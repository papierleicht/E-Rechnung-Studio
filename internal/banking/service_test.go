package banking

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	fints "github.com/BergerAPI/go-fints"
)

type fakeVault struct {
	pins    map[string]string
	fail    bool
	deletes int
}

func (v *fakeVault) Get(id string) (string, error) { return v.pins[id], nil }
func (v *fakeVault) Set(id, pin string) error {
	if v.fail {
		return errors.New("vault unavailable")
	}
	v.pins[id] = pin
	return nil
}
func (v *fakeVault) Delete(id string) error {
	v.deletes++
	if v.fail {
		return errors.New("vault unavailable")
	}
	delete(v.pins, id)
	return nil
}

type fakeClient struct {
	cfg          fints.Config
	transactions []fints.Transaction
	err          error
	tan          bool
}

func (f *fakeClient) Accounts(ctx context.Context) ([]fints.Account, error) {
	if f.tan {
		_, e := f.cfg.TANHandler(ctx, fints.TANChallenge{Challenge: "Freigeben"})
		if e != nil {
			return nil, e
		}
	}
	return []fints.Account{{IBAN: "DE02120300000000202051", Currency: "EUR"}}, f.err
}
func (f *fakeClient) TANMethods(context.Context) ([]fints.TANMethod, error) {
	return []fints.TANMethod{}, nil
}
func (f *fakeClient) Transactions(context.Context, fints.Account, time.Time, time.Time) ([]fints.Transaction, error) {
	return f.transactions, f.err
}
func testService(t *testing.T) (*Service, *fakeVault) {
	t.Helper()
	s, e := New(openTestDB(t), t.TempDir(), "test-product")
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { s.Close() })
	v := &fakeVault{pins: map[string]string{}}
	s.vault = v
	s.factory = func(c fints.Config) (readClient, error) { return &fakeClient{cfg: c}, nil }
	return s, v
}
func testConnection() Connection {
	return Connection{Label: "Testbank", Endpoint: "https://bank.example/fints", BankCode: "12345678", UserID: "user"}
}
func awaitTerminal(t *testing.T, s *Service, id string) Operation {
	t.Helper()
	deadline := time.After(2 * time.Second)
	for {
		r, e := s.Status(id)
		if e != nil {
			t.Fatal(e)
		}
		if r.Status == "done" || r.Status == "error" || r.Status == "cancelled" {
			s.mu.Lock()
			done := s.active.done
			s.mu.Unlock()
			<-done
			return r
		}
		select {
		case <-deadline:
			t.Fatal("operation timed out")
		case <-time.After(time.Millisecond):
		}
	}
}
func testAccount(t *testing.T, s *Service) Account {
	t.Helper()
	op, e := s.Connect(testConnection(), "sensitive-pin", false)
	if e != nil {
		t.Fatal(e)
	}
	if r := awaitTerminal(t, s, op.ID); r.Status != "done" {
		t.Fatal(r)
	}
	o, e := s.Overview()
	if e != nil || len(o.Accounts) != 1 {
		t.Fatalf("overview: %+v %v", o, e)
	}
	return o.Accounts[0]
}
func TestUnsavedPINDoesNotRequireVault(t *testing.T) {
	s, v := testService(t)
	v.fail = true
	testAccount(t, s)
	if v.deletes != 0 || len(v.pins) != 0 {
		t.Fatal("unexpected vault access")
	}
}

func TestConnectCreatesProtocolValidClientConfiguration(t *testing.T) {
	s, _ := testService(t)
	s.factory = func(config fints.Config) (readClient, error) {
		// Use the production validator without any network requests. This
		// catches invalid HKVVB metadata before a bank rejects initialization.
		if _, err := fints.NewClient(config); err != nil {
			return nil, err
		}
		return &fakeClient{cfg: config}, nil
	}
	testAccount(t, s)
}

func TestPINPersistenceIsOptInAndCanBeRemoved(t *testing.T) {
	s, v := testService(t)
	op, e := s.Connect(testConnection(), "sensitive-pin", true)
	if e != nil {
		t.Fatal(e)
	}
	awaitTerminal(t, s, op.ID)
	if v.pins[op.ConnectionID] != "sensitive-pin" {
		t.Fatal("missing credential")
	}
	rows, e := s.store.db.Query("SELECT data FROM bank_connections")
	if e != nil {
		t.Fatal(e)
	}
	for rows.Next() {
		var b []byte
		rows.Scan(&b)
		if strings.Contains(string(b), "sensitive-pin") {
			t.Fatal("PIN in SQLite")
		}
	}
	rows.Close()
	if e = s.ForgetPIN(op.ConnectionID); e != nil {
		t.Fatal(e)
	}
	if len(v.pins) != 0 {
		t.Fatal("PIN retained")
	}
	c, _ := s.store.connection(op.ConnectionID)
	if c.RememberPIN {
		t.Fatal("stale persistence flag")
	}
}
func TestFailedPageDoesNotImportPartialTransactions(t *testing.T) {
	s, _ := testService(t)
	a := testAccount(t, s)
	s.factory = func(c fints.Config) (readClient, error) {
		return &fakeClient{cfg: c, transactions: []fints.Transaction{{BookingDate: "2026-08-01", Currency: "EUR", Amount: 100}}, err: errors.New("failed second page secret")}, nil
	}
	op, e := s.Sync(a.ID, "pin", "2026-08-01", "2026-08-31")
	if e != nil {
		t.Fatal(e)
	}
	r := awaitTerminal(t, s, op.ID)
	if r.Status != "error" || strings.Contains(r.Message, "secret") {
		t.Fatal(r)
	}
	b, _ := s.Bookings(a.ID)
	if len(b) != 0 {
		t.Fatal("partial transactions persisted")
	}
	a, _ = s.store.account(a.ID)
	if a.LastSync != "" {
		t.Fatal("failed sync marked successful")
	}
}
func TestTANOperationCanBeCancelled(t *testing.T) {
	s, _ := testService(t)
	s.factory = func(c fints.Config) (readClient, error) { return &fakeClient{cfg: c, tan: true}, nil }
	op, e := s.Connect(testConnection(), "pin", false)
	if e != nil {
		t.Fatal(e)
	}
	deadline := time.Now().Add(time.Second)
	for {
		r, _ := s.Status(op.ID)
		if r.Status == "tan" {
			if e = s.SubmitTAN(op.ID, "old-challenge", "123456"); e == nil {
				t.Fatal("accepted stale challenge")
			}
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("no TAN challenge")
		}
		time.Sleep(time.Millisecond)
	}
	if _, e = s.Connect(testConnection(), "pin", false); e == nil {
		t.Fatal("allowed concurrent dialog")
	}
	s.Cancel(op.ID)
	r := awaitTerminal(t, s, op.ID)
	if r.Status != "cancelled" {
		t.Fatal(r)
	}
}
func TestSuccessfulTANAndVaultFailure(t *testing.T) {
	s, v := testService(t)
	v.fail = true
	if _, e := s.Connect(testConnection(), "pin", true); e == nil {
		t.Fatal("silently ignored vault error")
	}
	s.factory = func(c fints.Config) (readClient, error) { return &fakeClient{cfg: c, tan: true}, nil }
	op, e := s.Connect(testConnection(), "pin", false)
	if e != nil {
		t.Fatal(e)
	}
	deadline := time.Now().Add(time.Second)
	for {
		r, _ := s.Status(op.ID)
		if r.Challenge != nil {
			if e = s.SubmitTAN(op.ID, r.Challenge.ID, "123456"); e != nil {
				t.Fatal(e)
			}
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("no challenge")
		}
		time.Sleep(time.Millisecond)
	}
	if r := awaitTerminal(t, s, op.ID); r.Status != "done" {
		t.Fatal(r)
	}
}
func TestDuplicateMultiplicityAndAtomicImport(t *testing.T) {
	s, _ := testService(t)
	a := testAccount(t, s)
	b := fints.Transaction{BookingDate: "2026-08-01", Amount: 100, Currency: "EUR", Description: "Identical legitimate payments"}
	for i := 0; i < 2; i++ {
		n, e := s.store.importBookings(context.Background(), a, []fints.Transaction{b, b})
		want := 2
		if i > 0 {
			want = 0
		}
		if e != nil || n != want {
			t.Fatalf("import %d: %d %v", i, n, e)
		}
	}
	third := b
	third.BookingDate = "2026-08-02"
	invalid := b
	invalid.BookingDate = "invalid"
	if _, e := s.store.importBookings(context.Background(), a, []fints.Transaction{third, invalid}); e == nil {
		t.Fatal("accepted invalid booking")
	}
	stored, _ := s.Bookings(a.ID)
	if len(stored) != 2 {
		t.Fatalf("got %d bookings", len(stored))
	}
}
func TestConnectionAndErrorSanitization(t *testing.T) {
	for _, endpoint := range []string{"http://bank.example", "https://user:pin@bank.example", "https://bank.example?pin=secret", "https://bank.example#pin"} {
		c := testConnection()
		c.Endpoint = endpoint
		if validateConnection(c) == nil {
			t.Fatal(endpoint)
		}
	}
	if strings.Contains(safeError(&fints.BankError{Code: "PIN!", Message: "secret"}), "PIN!") {
		t.Fatal("untrusted bank code leaked")
	}
}
