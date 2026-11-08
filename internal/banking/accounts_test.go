package banking

import (
	"context"
	"errors"
	"strings"
	"testing"

	fints "github.com/BergerAPI/go-fints"
)

func TestSameLoginCannotBeConnectedTwice(t *testing.T) {
	s, _ := testService(t)
	testAccount(t, s)
	again := testConnection()
	again.UserID = " USER "
	_, e := s.Connect(again, "pin", true)
	if e == nil || !strings.Contains(e.Error(), "bereits eingerichtet") {
		t.Fatalf("second connection was not refused: %v", e)
	}
	o, _ := s.Overview()
	if len(o.Connections) != 1 || len(o.Accounts) != 1 {
		t.Fatalf("duplicate was stored: %+v", o)
	}
}

func TestOtherLoginAtSameBankIsAllowed(t *testing.T) {
	s, _ := testService(t)
	testAccount(t, s)
	other := testConnection()
	other.UserID = "second-user"
	s.factory = func(c fints.Config) (readClient, error) {
		return &fakeClient{cfg: c}, nil
	}
	if _, e := s.Connect(other, "pin", false); e != nil {
		t.Fatal(e)
	}
}

func TestAccountOfAnotherConnectionIsNotStoredTwice(t *testing.T) {
	s, v := testService(t)
	testAccount(t, s)
	// The same account reached through a second login (the fake bank returns
	// one IBAN for every login) must neither duplicate it nor leave the login.
	other := testConnection()
	other.UserID = "second-user"
	op, e := s.Connect(other, "pin", true)
	if e != nil {
		t.Fatal(e)
	}
	r := awaitTerminal(t, s, op.ID)
	if r.Status != "error" || !strings.Contains(r.Message, "bereits eingerichtet") {
		t.Fatalf("expected the known-accounts refusal: %+v", r)
	}
	o, _ := s.Overview()
	if len(o.Connections) != 1 || len(o.Accounts) != 1 {
		t.Fatalf("second login left traces: %+v", o)
	}
	if len(v.pins) != 0 {
		t.Fatalf("PIN of the dropped login stayed in the vault: %v", v.pins)
	}
}

func TestPartlyKnownAccountsAreSkipped(t *testing.T) {
	s, _ := testService(t)
	testAccount(t, s)
	s.factory = func(c fints.Config) (readClient, error) {
		return &accountsClient{fakeClient: fakeClient{cfg: c}, accounts: []fints.Account{
			{IBAN: "DE02 1203 0000 0000 2020 51", Currency: "EUR"},
			{IBAN: "DE89370400440532013000", Currency: "EUR"},
		}}, nil
	}
	other := testConnection()
	other.UserID = "second-user"
	op, e := s.Connect(other, "pin", false)
	if e != nil {
		t.Fatal(e)
	}
	r := awaitTerminal(t, s, op.ID)
	if r.Status != "done" || !strings.Contains(r.Message, "nicht doppelt") {
		t.Fatalf("unexpected result: %+v", r)
	}
	o, _ := s.Overview()
	if len(o.Accounts) != 2 {
		t.Fatalf("expected the known and one new account, got %+v", o.Accounts)
	}
}

type accountsClient struct {
	fakeClient
	accounts []fints.Account
}

func (c *accountsClient) Accounts(context.Context) ([]fints.Account, error) { return c.accounts, nil }

func TestAccountLabel(t *testing.T) {
	s, _ := testService(t)
	a := testAccount(t, s)
	if e := s.SetAccountLabel(a.ID, "  Geschäfts   konto "); e != nil {
		t.Fatal(e)
	}
	o, _ := s.Overview()
	if o.Accounts[0].Label != "Geschäfts konto" {
		t.Fatalf("label: %q", o.Accounts[0].Label)
	}
	// Fetching accounts again keeps the name.
	if _, e := s.store.saveAccounts(Connection{ID: a.ConnectionID}, []fints.Account{a.Data}); e != nil {
		t.Fatal(e)
	}
	if got, _ := s.store.account(a.ID); got.Label != "Geschäfts konto" {
		t.Fatalf("label lost: %q", got.Label)
	}
	if e := s.SetAccountLabel(a.ID, ""); e != nil {
		t.Fatal(e)
	}
	if got, _ := s.store.account(a.ID); got.Label != "" {
		t.Fatalf("label not cleared: %q", got.Label)
	}
	if e := s.SetAccountLabel(a.ID, strings.Repeat("ä", maxAccountLabel+1)); e == nil {
		t.Fatal("overlong label accepted")
	}
	if e := s.SetAccountLabel("unknown", "x"); e == nil {
		t.Fatal("unknown account accepted")
	}
}

func TestStoreAddsLabelColumnToOlderDatabase(t *testing.T) {
	db := openTestDB(t)
	if _, e := db.Exec(`CREATE TABLE bank_connections (id TEXT PRIMARY KEY, data BLOB NOT NULL, system_id TEXT NOT NULL DEFAULT '');
 CREATE TABLE bank_accounts (id TEXT PRIMARY KEY, connection_id TEXT NOT NULL REFERENCES bank_connections(id), data BLOB NOT NULL, last_sync TEXT NOT NULL DEFAULT '');
 INSERT INTO bank_connections(id,data) VALUES('c','{}');
 INSERT INTO bank_accounts(id,connection_id,data) VALUES('a','c','{}');`); e != nil {
		t.Fatal(e)
	}
	st, e := newStore(db)
	if e != nil {
		t.Fatal(e)
	}
	if e = st.setAccountLabel("a", "Alt"); e != nil {
		t.Fatal(e)
	}
	if _, e = newStore(db); e != nil {
		t.Fatalf("second start failed: %v", e)
	}
}

func TestRemoveAccountTakesBookingsAndLastConnection(t *testing.T) {
	s, v := testService(t)
	a := testAccount(t, s)
	op, e := s.Sync(a.ID, "pin", "2026-01-01", "2026-01-31")
	if e != nil {
		t.Fatal(e)
	}
	awaitTerminal(t, s, op.ID)
	s.vault = v
	c, _ := s.store.connection(a.ConnectionID)
	c.RememberPIN = true
	_ = s.store.saveConnection(c)
	v.pins[c.ID] = "secret"
	if e = s.SetAccountLabel(a.ID, "Altes Konto"); e != nil {
		t.Fatal(e)
	}
	removed, e := s.RemoveAccount(a.ID)
	if e != nil {
		t.Fatal(e)
	}
	if removed.Name != "Altes Konto" || removed.IBAN != a.Data.IBAN || !removed.ConnectionRemoved {
		t.Fatalf("unexpected report: %+v", removed)
	}
	o, _ := s.Overview()
	if len(o.Accounts) != 0 || len(o.Connections) != 0 {
		t.Fatalf("traces left: %+v", o)
	}
	if _, ok := v.pins[c.ID]; ok {
		t.Fatal("PIN stayed in the vault")
	}
	var rows int
	_ = s.store.db.QueryRow("SELECT COUNT(*) FROM bank_bookings").Scan(&rows)
	if rows != 0 {
		t.Fatalf("%d bookings left", rows)
	}
	if _, e = s.RemoveAccount(a.ID); !errors.Is(e, ErrAccountNotFound) {
		t.Fatalf("second removal: %v", e)
	}
	// The same login can be set up again.
	if _, e = s.Connect(testConnection(), "pin", false); e != nil {
		t.Fatalf("set up again: %v", e)
	}
}

func TestRemoveAccountKeepsConnectionWithOtherAccounts(t *testing.T) {
	s, _ := testService(t)
	s.factory = func(c fints.Config) (readClient, error) {
		return &accountsClient{fakeClient: fakeClient{cfg: c}, accounts: []fints.Account{
			{IBAN: "DE02120300000000202051", Currency: "EUR"},
			{IBAN: "DE89370400440532013000", Currency: "EUR"},
		}}, nil
	}
	op, e := s.Connect(testConnection(), "pin", false)
	if e != nil {
		t.Fatal(e)
	}
	awaitTerminal(t, s, op.ID)
	o, _ := s.Overview()
	if len(o.Accounts) != 2 {
		t.Fatalf("accounts: %+v", o.Accounts)
	}
	removed, e := s.RemoveAccount(o.Accounts[0].ID)
	if e != nil || removed.ConnectionRemoved {
		t.Fatalf("%+v %v", removed, e)
	}
	if o, _ = s.Overview(); len(o.Accounts) != 1 || len(o.Connections) != 1 {
		t.Fatalf("expected one account on the connection: %+v", o)
	}
}
