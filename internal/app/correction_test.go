package app

import (
	"bytes"
	"papierleicht.de/e-rechnung-studio/internal/invoicexml"
	"papierleicht.de/e-rechnung-studio/internal/profile"
	"testing"
)

func TestCorrectionPreservesIssuedOriginalAndReferencesIt(t *testing.T) {
	s, err := NewInvoiceService(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	t.Setenv("E_RECHNUNG_STUDIO_EXPORT_DIR", t.TempDir())
	company := testCompany()
	company.Endpoint = company.Email
	company.EndpointScheme = "EM"
	company.Phone = "030123456"
	if _, err := s.SaveCompany(company); err != nil {
		t.Fatal(err)
	}
	for _, format := range []string{"facturx-en16931", "facturx-basic", "facturx-extended", "xrechnung-cii", "xrechnung-ubl"} {
		t.Run(format, func(t *testing.T) {
			draft := testDraft()
			draft.Number = ""
			draft.Profile = format
			p, _ := profile.Get(format)
			draft.Syntax = p.Syntax
			draft.Customer.Endpoint, draft.Customer.EndpointScheme = draft.Customer.Email, "EM"
			draft.BuyerReference = "CUSTOMER-1"
			issued, err := s.IssueInvoice(draft)
			if err != nil {
				t.Fatal(err)
			}
			originalPDF, err := s.invoicePDF(issued.Draft.ID)
			if err != nil {
				t.Fatal(err)
			}
			correction, err := s.CreateCorrectionDraft(issued.Draft.ID)
			if err != nil {
				t.Fatal(err)
			}
			if correction.Draft.ID == issued.Draft.ID || correction.Draft.Kind != "credit-note" || correction.Draft.Status != "draft" || correction.Draft.Number == "" || !correction.Draft.AutomaticNumber {
				t.Fatal("correction isn't an independent draft")
			}
			final, err := s.IssueInvoice(correction.Draft)
			if err != nil {
				t.Fatalf("%v: %+v", err, correction.Validation)
			}
			parsed, err := invoicexml.Parse([]byte(final.XML))
			if err != nil {
				t.Fatal(err)
			}
			if parsed.InvoiceTypeCode != 381 || len(parsed.InvoiceReferencedDocument) != 1 || parsed.InvoiceReferencedDocument[0].ID != issued.Draft.Number {
				t.Fatal("missing reference to original")
			}
			after, err := s.OpenInvoice(issued.Draft.ID)
			if err != nil {
				t.Fatal(err)
			}
			if after.XML != issued.XML || after.Draft.Status != "issued" || after.Draft.Number != issued.Draft.Number {
				t.Fatal("correction mutated issued original")
			}
			afterPDF, err := s.invoicePDF(issued.Draft.ID)
			if err != nil || !bytes.Equal(originalPDF, afterPDF) {
				t.Fatal("correction changed original PDF", err)
			}
			if err := s.DeleteDraft(issued.Draft.ID); err == nil {
				t.Fatal("issued original deleted")
			}
		})
	}
	draft, err := s.SaveInvoice(testDraft())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateCorrectionDraft(draft.Draft.ID); err == nil {
		t.Fatal("accepted correction of draft")
	}
	if err := s.DeleteDraft(draft.Draft.ID); err != nil {
		t.Fatal("could not delete draft", err)
	}
}
