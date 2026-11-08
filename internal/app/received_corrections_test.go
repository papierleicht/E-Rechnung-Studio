package app

import (
	"bytes"
	"encoding/base64"
	"slices"
	"strings"
	"testing"
	"time"

	"papierleicht.de/e-rechnung-studio/internal/einvoice"
	"papierleicht.de/e-rechnung-studio/internal/invoicexml"
	"papierleicht.de/e-rechnung-studio/internal/model"
)

func receivedCorrection(id, supplier, number string, documentType int, refs ...model.PrecedingInvoice) model.ReceivedInvoice {
	v := receivedFor(id, model.ValidationResult{State: "valid"}, model.Totals{Net: "100.00", Tax: "19.00", Gross: "119.00"})
	v.Inspection.InvoiceNumber, v.Inspection.SellerName = number, supplier
	v.Inspection.DocumentType, v.Inspection.PrecedingInvoices = documentType, refs
	return v
}

func ref(number string) model.PrecedingInvoice { return model.PrecedingInvoice{Number: number} }

// BMF letter of 15.10.2026, Rn. 20 and 21: a correction may replace the invoice or
// add to it, and the file does not say which. The app only finds the pair.
func TestReceivedOriginalsLinkACorrectionToItsInvoice(t *testing.T) {
	original := receivedCorrection("a", "Büro Nord GmbH", "RE-1", 380)
	for name, tc := range map[string]struct {
		correction model.ReceivedInvoice
		want       string
	}{
		"type 384 with the number of the invoice":    {receivedCorrection("c", "Büro Nord GmbH", "RE-1K", 384, ref("RE-1")), "a"},
		"another type that names the invoice":        {receivedCorrection("c", "Büro Nord GmbH", "RE-1K", 380, ref("RE-1")), "a"},
		"case and blanks do not matter":              {receivedCorrection("c", "  büro  nord gmbh", "RE-1K", 384, ref(" re-1 ")), "a"},
		"the second reference is the one received":   {receivedCorrection("c", "Büro Nord GmbH", "RE-1K", 384, ref("RE-0"), ref("RE-1")), "a"},
		"a correction that names no invoice":         {receivedCorrection("c", "Büro Nord GmbH", "RE-1K", 384), ""},
		"the invoice of another supplier":            {receivedCorrection("c", "Anderes Büro", "RE-1K", 384, ref("RE-1")), ""},
		"a credit note counts negative anyway":       {receivedCorrection("c", "Büro Nord GmbH", "G-1", 381, ref("RE-1")), ""},
		"an invoice that was not received":           {receivedCorrection("c", "Büro Nord GmbH", "RE-9K", 384, ref("RE-9")), ""},
		"an ordinary invoice refers to nothing":      {receivedCorrection("c", "Büro Nord GmbH", "RE-2", 380), ""},
		"a document does not correct itself":         {receivedCorrection("c", "Büro Nord GmbH", "RE-5", 384, ref("RE-5")), ""},
		"a supplier without a name cannot be traced": {receivedCorrection("c", "", "RE-1K", 384, ref("RE-1")), ""},
	} {
		got := receivedOriginals([]model.ReceivedInvoice{original, tc.correction})
		if tc.want == "" {
			if len(got) != 0 {
				t.Errorf("%s: got %+v", name, got)
			}
		} else if got["c"].ID != tc.want || len(got) != 1 {
			t.Errorf("%s: got %+v", name, got)
		}
	}
	// A misimported invoice is no invoice that counts.
	gone := original
	gone.Status = "misimport"
	if got := receivedOriginals([]model.ReceivedInvoice{gone, receivedCorrection("c", "Büro Nord GmbH", "RE-1K", 384, ref("RE-1"))}); len(got) != 0 {
		t.Errorf("misimport: got %+v", got)
	}
}

func TestVorsteuerAsksWhichOfACorrectionAndItsInvoiceApplies(t *testing.T) {
	in := baseInput()
	in.Received = []model.ReceivedInvoice{
		receivedCorrection("a", "Büro Nord GmbH", "RE-1", 380),
		receivedCorrection("c", "Büro Nord GmbH", "RE-1K", 384, ref("RE-1")),
	}
	r := buildFinanceReport(in)
	assertTraceable(t, r)
	if r.InputTax.Amount != "38.00" {
		t.Fatalf("both documents count, the app does not net them: %+v", r.InputTax)
	}
	note, ok := entryNote(r.Issues, "received_correction_review", "c")
	if !ok || !strings.Contains(note, "Berichtigung zu RE-1") {
		t.Fatalf("the correction carries the hint: %q %v", note, ok)
	}
	if _, ok := entryNote(r.Issues, "received_correction_review", "a"); ok {
		t.Error("the invoice itself needs no hint")
	}
	for _, issue := range r.Issues {
		if issue.Code == "received_correction_review" && issue.Message != correctionMessage {
			t.Errorf("the hint says what may not happen, not how to book it: %q", issue.Message)
		}
	}
	if !strings.Contains(correctionMessage, "doppelt") || !strings.Contains(correctionMessage, "entscheide es im Beleg") || strings.Contains(correctionMessage, "Voranmeldung") {
		t.Errorf("the app names the risk and leaves the decision to the user: %q", correctionMessage)
	}
	for _, entry := range r.InputTax.Entries {
		if entry.ID == "c" && entry.Note != "Berichtigung zu RE-1" {
			t.Errorf("the entry names the invoice: %q", entry.Note)
		}
	}

	// A decision on either document settles the question.
	for _, id := range []string{"a", "c"} {
		in.InputTaxChoices = map[string]model.InputTaxChoice{id: {ReceivedID: id, Mode: model.InputTaxNone}}
		decided := buildFinanceReport(in)
		if hasIssue(decided, "received_correction_review") || decided.InputTax.Amount != "19.00" {
			t.Errorf("decided on %s: %+v %+v", id, decided.InputTax, decided.Issues)
		}
		if _, _, issues := vatRecords(in); len(issues) != 0 {
			t.Errorf("decided on %s: %+v", id, issues)
		}
	}
	in.InputTaxChoices = nil

	in.Received = in.Received[1:]
	if hasIssue(buildFinanceReport(in), "received_correction_review") {
		t.Error("a correction without its invoice in the list needs no hint")
	}
}

func TestVATReturnAsksWhichOfACorrectionAndItsInvoiceApplies(t *testing.T) {
	in := baseInput()
	in.Received = []model.ReceivedInvoice{
		receivedCorrection("a", "Büro Nord GmbH", "RE-1", 380),
		receivedCorrection("c", "Büro Nord GmbH", "RE-1K", 384, ref("RE-1")),
	}
	_, purchases, issues := vatRecords(in)
	if len(purchases) != 2 {
		t.Fatalf("both documents count: %+v", purchases)
	}
	for _, issue := range issues {
		if issue.Code == "received_correction_review" {
			if len(issue.Entries) != 1 || issue.Entries[0].ID != "c" || !strings.Contains(issue.Entries[0].Note, "Berichtigung zu RE-1") {
				t.Fatalf("got %+v", issue)
			}
			return
		}
	}
	t.Fatalf("no hint in %+v", issues)
}

// asCorrection turns a document the app issued into a corrected invoice (type 384)
// that refers to an earlier invoice, the way a supplier's software would.
func asCorrection(t *testing.T, xml, number, preceding string, date time.Time) string {
	t.Helper()
	invoice, err := invoicexml.Parse([]byte(xml))
	if err != nil {
		t.Fatal(err)
	}
	invoice.InvoiceNumber, invoice.InvoiceTypeCode = number, 384
	invoice.InvoiceReferencedDocument = []einvoice.ReferencedDocument{{ID: preceding, Date: date}}
	var out bytes.Buffer
	if err := invoice.Write(&out); err != nil {
		t.Fatal(err)
	}
	return out.String()
}

func TestReceivedCorrectionNamesTheInvoiceItRefersTo(t *testing.T) {
	for _, f := range attachmentFormats {
		t.Run(f.syntax, func(t *testing.T) {
			s := receivedService(t)
			valid := issuedXML(t, s, f.profile, f.syntax)
			plain := inspect(t, s, valid)
			if plain.DocumentType != 380 || len(plain.PrecedingInvoices) != 0 || plain.InvoiceNumber == "" {
				t.Fatalf("an ordinary invoice: %+v", plain)
			}
			correction := asCorrection(t, valid, plain.InvoiceNumber+"-K", plain.InvoiceNumber, time.Date(2026, 3, 12, 0, 0, 0, 0, time.UTC))
			got := inspect(t, s, correction)
			if got.DocumentType != 384 || !slices.Equal(got.PrecedingInvoices, []model.PrecedingInvoice{{Number: plain.InvoiceNumber, Date: "2026-03-12"}}) {
				t.Fatalf("the correction: type %d, refers to %+v", got.DocumentType, got.PrecedingInvoices)
			}

			// Stored, listed and matched: both documents are found as a pair.
			for name, xml := range map[string]string{"rechnung.xml": valid, "berichtigung.xml": correction} {
				if _, err := s.ImportReceivedInvoice(name, base64.StdEncoding.EncodeToString([]byte(xml))); err != nil {
					t.Fatal(err)
				}
			}
			list, err := s.ListReceivedInvoices()
			if err != nil || len(list) != 2 {
				t.Fatal(list, err)
			}
			pairs := receivedOriginals(list)
			if len(pairs) != 1 {
				t.Fatalf("got %+v", pairs)
			}
			for correctionID, original := range pairs {
				if original.Inspection.InvoiceNumber != plain.InvoiceNumber || correctionID == original.ID {
					t.Fatalf("got %s -> %s", correctionID, original.Inspection.InvoiceNumber)
				}
			}
		})
	}
}
