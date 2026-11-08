package banking

import (
	"context"
	fints "github.com/BergerAPI/go-fints"
	"testing"
	"time"
)

type importBarrierClient struct {
	fakeClient
	arrived chan struct{}
	release chan struct{}
}

func (c *importBarrierClient) Transactions(context.Context, fints.Account, time.Time, time.Time) ([]fints.Transaction, error) {
	close(c.arrived)
	<-c.release
	return []fints.Transaction{{BookingDate: "2026-08-01", Amount: 100, Currency: "EUR"}}, nil
}
func TestCancelWhileImportWaitsForDatabaseDoesNotCommit(t *testing.T) {
	s, _ := testService(t)
	a := testAccount(t, s)
	cl := &importBarrierClient{arrived: make(chan struct{}), release: make(chan struct{})}
	s.factory = func(fints.Config) (readClient, error) { return cl, nil }
	op, e := s.Sync(a.ID, "pin", "2026-08-01", "2026-08-31")
	if e != nil {
		t.Fatal(e)
	}
	<-cl.arrived
	conn, e := s.store.db.Conn(context.Background())
	if e != nil {
		t.Fatal(e)
	}
	defer conn.Close()
	waits := s.store.db.Stats().WaitCount
	close(cl.release)
	deadline := time.Now().Add(time.Second)
	for s.store.db.Stats().WaitCount == waits {
		if time.Now().After(deadline) {
			t.Fatal("import did not wait for database")
		}
		time.Sleep(time.Millisecond)
	}
	if e = s.Cancel(op.ID); e != nil {
		t.Fatal(e)
	}
	// A context-aware BeginTx must end without waiting for the DB connection.
	result := awaitTerminal(t, s, op.ID)
	if result.Status != "cancelled" || result.Added != 0 {
		t.Fatalf("result=%+v", result)
	}
	conn.Close()
	bookings, _ := s.Bookings(a.ID)
	if len(bookings) != 0 {
		t.Fatal("cancelled import committed bookings")
	}
	fresh, _ := s.store.account(a.ID)
	if fresh.LastSync != "" {
		t.Fatal("cancelled import advanced last sync")
	}
}

type postCommitBarrierClient struct {
	fakeClient
	arrived, release chan struct{}
}

func (c *postCommitBarrierClient) State() fints.ClientState {
	close(c.arrived)
	<-c.release
	return fints.ClientState{SystemID: "synthetic-system"}
}
func TestCancelAfterSuccessfulCommitStillReportsSuccess(t *testing.T) {
	s, _ := testService(t)
	a := testAccount(t, s)
	cl := &postCommitBarrierClient{fakeClient: fakeClient{transactions: []fints.Transaction{{BookingDate: "2026-08-01", Amount: 100, Currency: "EUR"}}}, arrived: make(chan struct{}), release: make(chan struct{})}
	s.factory = func(fints.Config) (readClient, error) { return cl, nil }
	op, e := s.Sync(a.ID, "pin", "2026-08-01", "2026-08-31")
	if e != nil {
		t.Fatal(e)
	}
	<-cl.arrived
	if e = s.Cancel(op.ID); e != nil {
		t.Fatal(e)
	}
	close(cl.release)
	result := awaitTerminal(t, s, op.ID)
	if result.Status != "done" || result.Added != 1 {
		t.Fatalf("result=%+v", result)
	}
	bookings, _ := s.Bookings(a.ID)
	if len(bookings) != 1 {
		t.Fatal("successful import missing")
	}
}

func rejectConnectionWrites(t *testing.T, s *Service) {
	t.Helper()
	_, e := s.store.db.Exec(`CREATE TRIGGER reject_bank_connection BEFORE INSERT ON bank_connections BEGIN SELECT RAISE(FAIL,'synthetic write failure'); END;`)
	if e != nil {
		t.Fatal(e)
	}
}
func TestNewConnectionWriteFailureRemovesOrphanPIN(t *testing.T) {
	s, v := testService(t)
	rejectConnectionWrites(t, s)
	if _, e := s.Connect(testConnection(), "new-secret", true); e == nil {
		t.Fatal("expected metadata failure")
	}
	if len(v.pins) != 0 {
		t.Fatal("orphan PIN retained in vault")
	}
}
func TestExistingConnectionWriteFailureRestoresOldPIN(t *testing.T) {
	for _, remember := range []bool{true, false} {
		t.Run(map[bool]string{true: "replace", false: "remove"}[remember], func(t *testing.T) {
			s, v := testService(t)
			op, e := s.Connect(testConnection(), "old-secret", true)
			if e != nil {
				t.Fatal(e)
			}
			awaitTerminal(t, s, op.ID)
			c, e := s.store.connection(op.ConnectionID)
			if e != nil {
				t.Fatal(e)
			}
			rejectConnectionWrites(t, s)
			if _, e = s.Connect(c, "new-secret", remember); e == nil {
				t.Fatal("expected metadata failure")
			}
			if v.pins[c.ID] != "old-secret" {
				t.Fatal("prior credential not restored")
			}
		})
	}
}
func TestForgetPINDeletesStaleVaultEntry(t *testing.T) {
	s, v := testService(t)
	a := testAccount(t, s)
	v.pins[a.ConnectionID] = "stale-secret"
	if e := s.ForgetPIN(a.ConnectionID); e != nil {
		t.Fatal(e)
	}
	if _, exists := v.pins[a.ConnectionID]; exists {
		t.Fatal("stale credential retained")
	}
}
