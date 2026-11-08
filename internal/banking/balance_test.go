package banking

import (
	"context"
	"errors"
	"strings"
	"testing"

	fints "github.com/BergerAPI/go-fints"
)

// balanceFake adds the optional balance query to the shared fake client.
type balanceFake struct {
	fakeClient
	balance        *fints.AccountBalance
	err            error
	needsTAN       bool
	closing        *fints.DatedBalance
	balanceQueries int
}

func (f *balanceFake) StatementBalance() *fints.DatedBalance { return f.closing }

func (f *balanceFake) Balance(ctx context.Context, _ fints.Account) (*fints.AccountBalance, error) {
	f.balanceQueries++
	if f.needsTAN {
		if _, err := f.cfg.TANHandler(ctx, fints.TANChallenge{Challenge: "Saldo freigeben"}); err != nil {
			return nil, err
		}
	}
	return f.balance, f.err
}

func syncWith(t *testing.T, s *Service, a Account, client *balanceFake) Operation {
	t.Helper()
	s.factory = func(c fints.Config) (readClient, error) {
		client.cfg = c
		client.transactions = []fints.Transaction{{BookingDate: "2026-10-01", Amount: 1000, Currency: "EUR", Description: "Zahlung"}}
		return client, nil
	}
	op, err := s.Sync(a.ID, "pin", "2026-10-01", "2026-10-31")
	if err != nil {
		t.Fatal(err)
	}
	return awaitTerminal(t, s, op.ID)
}

func storedBalance(t *testing.T, s *Service) *Balance {
	t.Helper()
	o, err := s.Overview()
	if err != nil || len(o.Accounts) != 1 {
		t.Fatal(o, err)
	}
	return o.Accounts[0].Balance
}

func TestSyncStoresBankBalance(t *testing.T) {
	s, _ := testService(t)
	a := testAccount(t, s)
	r := syncWith(t, s, a, &balanceFake{balance: &fints.AccountBalance{Booked: fints.DatedBalance{Money: fints.Money{Amount: 842017, Currency: "EUR"}, Date: "2026-10-26"}}})
	if r.Status != "done" || r.Added != 1 || !strings.Contains(r.Message, "Kontostand aktualisiert") {
		t.Fatalf("%+v", r)
	}
	b := storedBalance(t, s)
	if b == nil || b.Booked.Amount != 842017 || b.Booked.Date != "2026-10-26" || b.FetchedAt == "" || b.Source != BalanceSourceQuery {
		t.Fatalf("balance %+v", b)
	}
}

func TestStatementClosingBalanceAvoidsBalanceQuery(t *testing.T) {
	s, _ := testService(t)
	a := testAccount(t, s)
	client := &balanceFake{closing: &fints.DatedBalance{Money: fints.Money{Amount: -1234, Currency: "EUR"}, Date: "2026-10-25"}, needsTAN: true}
	r := syncWith(t, s, a, client)
	if r.Status != "done" || !strings.Contains(r.Message, "Kontostand aktualisiert") || client.balanceQueries != 0 {
		t.Fatalf("closing balance must be used without a balance query: %+v, queries %d", r, client.balanceQueries)
	}
	if b := storedBalance(t, s); b == nil || b.Booked.Amount != -1234 || b.Source != BalanceSourceStatement {
		t.Fatalf("balance %+v", b)
	}
}

func TestBalanceNeverAsksForSecondApproval(t *testing.T) {
	s, _ := testService(t)
	a := testAccount(t, s)
	r := syncWith(t, s, a, &balanceFake{needsTAN: true, balance: &fints.AccountBalance{}})
	if r.Status != "done" || r.Added != 1 || r.Challenge != nil || !strings.Contains(r.Message, "nicht übermittelt") {
		t.Fatalf("a balance requiring a TAN must be skipped, not prompted: %+v", r)
	}
	if storedBalance(t, s) != nil {
		t.Fatal("balance stored although the bank required approval")
	}
}

func TestFailedBalanceKeepsBookingsAndPreviousBalance(t *testing.T) {
	s, _ := testService(t)
	a := testAccount(t, s)
	syncWith(t, s, a, &balanceFake{balance: &fints.AccountBalance{Booked: fints.DatedBalance{Money: fints.Money{Amount: 500, Currency: "EUR"}, Date: "2026-10-20"}}})
	r := syncWith(t, s, a, &balanceFake{err: errors.New("HKSAL not supported")})
	if r.Status != "done" {
		t.Fatalf("balance failure must not fail the import: %+v", r)
	}
	if b := storedBalance(t, s); b == nil || b.Booked.Amount != 500 {
		t.Fatalf("previous balance lost: %+v", b)
	}
	if bookings, _ := s.Bookings(a.ID); len(bookings) != 1 {
		t.Fatalf("bookings %d", len(bookings))
	}
}
