package banking

import (
	"context"
	"errors"
	fints "github.com/BergerAPI/go-fints"
	"testing"
	"time"
)

func TestBankReferenceAllowsMetadataRefresh(t *testing.T) {
	s, _ := testService(t)
	a := testAccount(t, s)
	b := fints.Transaction{BookingDate: "2026-08-01", Amount: 100, Currency: "EUR", BankReference: "bank-id-1", Description: "Initial"}
	_, e := s.store.importBookings(context.Background(), a, []fints.Transaction{b, b})
	if e != nil {
		t.Fatal(e)
	}
	b.Description = "Enriched"
	b.CounterpartyName = "Sender"
	n, e := s.store.importBookings(context.Background(), a, []fints.Transaction{b, b})
	if e != nil || n != 0 {
		t.Fatalf("metadata duplicated: %d %v", n, e)
	}
	stored, _ := s.Bookings(a.ID)
	if len(stored) != 2 || stored[0].Data.Description != "Enriched" {
		t.Fatal(stored)
	}
}
func TestReferenceFreeChangesBlockInsteadOfDoubleCounting(t *testing.T) {
	for _, reference := range []string{"", "NONREF", "NOTPROVIDED"} {
		t.Run(reference, func(t *testing.T) {
			s, _ := testService(t)
			a := testAccount(t, s)
			b := fints.Transaction{BookingDate: "2026-08-01", Amount: 100, Currency: "EUR", BankReference: reference, Description: "Initial"}
			s.store.importBookings(context.Background(), a, []fints.Transaction{b})
			b.Description = "Changed"
			_, e := s.store.importBookings(context.Background(), a, []fints.Transaction{b})
			var conflict *ImportConflictError
			if !errors.As(e, &conflict) {
				t.Fatalf("expected review conflict: %v", e)
			}
			stored, _ := s.Bookings(a.ID)
			if len(stored) != 1 || stored[0].Data.Description != "Initial" {
				t.Fatal(stored)
			}
		})
	}
}
func TestMissingPreviouslyBookedRowsBlockAtomicWindow(t *testing.T) {
	s, _ := testService(t)
	a := testAccount(t, s)
	b := fints.Transaction{BookingDate: "2026-08-01", Amount: 100, Currency: "EUR", BankReference: "bank-id-1"}
	s.store.importBookings(context.Background(), a, []fints.Transaction{b})
	from, _ := time.Parse(time.DateOnly, "2026-08-01")
	to, _ := time.Parse(time.DateOnly, "2026-08-31")
	_, e := s.store.importBookings(context.Background(), a, nil, from, to)
	var conflict *ImportConflictError
	if !errors.As(e, &conflict) {
		t.Fatal("empty response accepted", e)
	}
	b.Amount = 200
	_, e = s.store.importBookings(context.Background(), a, []fints.Transaction{b}, from, to)
	if !errors.As(e, &conflict) {
		t.Fatal("changed amount accepted", e)
	}
	stored, _ := s.Bookings(a.ID)
	if len(stored) != 1 || stored[0].Data.Amount != 100 {
		t.Fatal(stored)
	}
}
func TestCompleteOverlappingWindowsAndNewRows(t *testing.T) {
	s, _ := testService(t)
	a := testAccount(t, s)
	one := fints.Transaction{BookingDate: "2026-08-01", Amount: 100, Currency: "EUR"}
	two := one
	two.BookingDate = "2026-08-02"
	three := one
	three.BookingDate = "2026-08-03"
	s.store.importBookings(context.Background(), a, []fints.Transaction{one, two})
	from, _ := time.Parse(time.DateOnly, "2026-08-02")
	to, _ := time.Parse(time.DateOnly, "2026-08-03")
	n, e := s.store.importBookings(context.Background(), a, []fints.Transaction{two, three}, from, to)
	if e != nil || n != 1 {
		t.Fatal(n, e)
	}
	stored, _ := s.Bookings(a.ID)
	if len(stored) != 3 {
		t.Fatal(stored)
	}
}
