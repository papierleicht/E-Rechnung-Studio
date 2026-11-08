package store

import (
	"slices"
	"strings"
	"testing"

	"papierleicht.de/e-rechnung-studio/internal/model"
)

func receivedDocument(t *testing.T, s *Store, id, date string) {
	t.Helper()
	doc := model.ReceivedInvoice{ID: id, Filename: id + ".xml", ImportedAt: now(), Status: "active"}
	doc.Inspection.IssueDate = date
	if _, err := s.SaveReceived(doc, []byte("original "+id)); err != nil {
		t.Fatal(err)
	}
}

func TestInputTaxChoiceIsKeptLoggedAndCanBeTakenBack(t *testing.T) {
	s := openTest(t)
	receivedDocument(t, s, "r1", "2026-05-02")

	saved, err := s.SaveInputTaxChoice(model.InputTaxChoice{ReceivedID: "r1", Mode: model.InputTaxAmount, Amount: "94,2", Note: "  Anzahlung war schon abgezogen "})
	if err != nil {
		t.Fatal(err)
	}
	if saved.Amount != "94.20" || saved.Note != "Anzahlung war schon abgezogen" || saved.DecidedAt == "" {
		t.Fatalf("%+v", saved)
	}
	choices, err := s.InputTaxChoices()
	if err != nil || choices["r1"] != saved {
		t.Fatalf("%+v %v", choices, err)
	}

	// The same decision again changes nothing and keeps its date.
	if again, err := s.SaveInputTaxChoice(model.InputTaxChoice{ReceivedID: "r1", Mode: model.InputTaxAmount, Amount: "94.20", Note: "Anzahlung war schon abgezogen"}); err != nil || again != saved {
		t.Fatalf("%+v %v", again, err)
	}
	if got := actionsOf(t, s, AuditInputTax, "r1"); !slices.Equal(got, []string{AuditCreated}) {
		t.Fatalf("%v", got)
	}

	if _, err = s.SaveInputTaxChoice(model.InputTaxChoice{ReceivedID: "r1", Mode: model.InputTaxNone}); err != nil {
		t.Fatal(err)
	}
	if err = s.ClearInputTaxChoice("r1"); err != nil {
		t.Fatal(err)
	}
	if choices, _ = s.InputTaxChoices(); len(choices) != 0 {
		t.Fatalf("%+v", choices)
	}
	if err = s.ClearInputTaxChoice("r1"); err != nil {
		t.Fatalf("nothing to take back is no error: %v", err)
	}
	if got := actionsOf(t, s, AuditInputTax, "r1"); !slices.Equal(got, []string{AuditDeleted, AuditChanged, AuditCreated}) && !slices.Equal(got, []string{AuditCreated, AuditChanged, AuditDeleted}) {
		t.Fatalf("every step stays in the log: %v", got)
	}
	if report := verified(t, s); !report.OK {
		t.Fatalf("%+v", report)
	}
}

func TestInputTaxChoiceChecksItsInput(t *testing.T) {
	s := openTest(t)
	receivedDocument(t, s, "r1", "2026-05-02")
	for name, c := range map[string]model.InputTaxChoice{
		"no mode":         {ReceivedID: "r1"},
		"unknown mode":    {ReceivedID: "r1", Mode: "half"},
		"no amount":       {ReceivedID: "r1", Mode: model.InputTaxAmount},
		"negative amount": {ReceivedID: "r1", Mode: model.InputTaxAmount, Amount: "-1"},
		"long reason":     {ReceivedID: "r1", Mode: model.InputTaxNone, Note: strings.Repeat("x", 501)},
		"unknown receipt": {ReceivedID: "r2", Mode: model.InputTaxNone},
	} {
		if _, err := s.SaveInputTaxChoice(c); err == nil {
			t.Errorf("%s: must be refused", name)
		}
	}
	for _, mode := range []string{model.InputTaxFull, model.InputTaxOnPayment} {
		if saved, err := s.SaveInputTaxChoice(model.InputTaxChoice{ReceivedID: "r1", Mode: mode, Amount: "5"}); err != nil || saved.Amount != "" {
			t.Fatalf("an amount belongs to the mode amount only: %+v %v", saved, err)
		}
	}
}

func TestInputTaxChoiceStaysInAClosedPeriod(t *testing.T) {
	s := openTest(t)
	receivedDocument(t, s, "r1", "2026-02-02")
	if _, err := s.SaveInputTaxChoice(model.InputTaxChoice{ReceivedID: "r1", Mode: model.InputTaxNone}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.LockPeriod("", "2026-03-31"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.SaveInputTaxChoice(model.InputTaxChoice{ReceivedID: "r1", Mode: model.InputTaxFull}); err == nil {
		t.Fatal("a closed period keeps its decision")
	}
	if err := s.ClearInputTaxChoice("r1"); err == nil {
		t.Fatal("a closed period keeps its decision")
	}
	if choices, _ := s.InputTaxChoices(); choices["r1"].Mode != model.InputTaxNone {
		t.Fatalf("%+v", choices)
	}
}

func TestInputTaxChoiceGoesWithADeletedMisimport(t *testing.T) {
	s := openTest(t)
	receivedDocument(t, s, "r1", "2026-05-02")
	if _, err := s.SaveInputTaxChoice(model.InputTaxChoice{ReceivedID: "r1", Mode: model.InputTaxNone}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ChangeReceivedStatus("r1", "misimport", "falsche Datei"); err != nil {
		t.Fatal(err)
	}
	if err := s.DeleteReceivedMisimport("r1", "r1.xml", true); err != nil {
		t.Fatal(err)
	}
	if choices, _ := s.InputTaxChoices(); len(choices) != 0 {
		t.Fatalf("%+v", choices)
	}
	if report := verified(t, s); !report.OK {
		t.Fatalf("%+v", report)
	}
}
