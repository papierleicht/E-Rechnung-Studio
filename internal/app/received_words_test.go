package app

import (
	"encoding/base64"
	"regexp"
	"strings"
	"testing"
)

// The findings of a received document describe the supplier's invoice. The
// German texts of the check speak to the issuer ("Ergänze die Käuferreferenz,
// die dir der Empfänger mitgeteilt hat") and would send the recipient to fix it.
// § 27 Abs. 38 UStG: until the end of 2026, and in 2027 for a small turnover, an
// invoice may still come as a PDF with the recipient's consent.
func TestReceivedPDFWithoutInvoiceDataNamesTheTransitionRule(t *testing.T) {
	s := receivedService(t)
	got, err := s.InspectInvoiceFile("beleg.pdf", base64.StdEncoding.EncodeToString(receivedTestPDF(false)))
	if err != nil {
		t.Fatal(err)
	}
	issue := issueOf(got, "PDF-NO-XML")
	if issue == nil {
		t.Fatalf("got %+v", got.Validation)
	}
	for _, want := range []string{"nicht als strukturierte E-Rechnung", "Ende 2026", "800.000 €", "§ 27 Abs. 38 UStG"} {
		if !strings.Contains(issue.Message, want) {
			t.Errorf("the notice lacks %q: %q", want, issue.Message)
		}
	}
}

func TestReceivedFindingsAreWordedForTheRecipient(t *testing.T) {
	s := receivedService(t)
	valid := issuedXML(t, s, "xrechnung-cii", "cii")

	t.Run("a business rule", func(t *testing.T) {
		got := inspect(t, s, replaceOnce(t, valid, "<ram:BuyerReference>CUSTOMER-1</ram:BuyerReference>", ""))
		issue := issueOf(got, "BR-DE-15")
		if issue == nil || issue.Message != "Die Rechnung enthält keine Käuferreferenz (Leitweg-ID)." {
			t.Fatalf("got %+v", issue)
		}
		if issue.Details == "" || issue.Class != "business" || issue.Severity != "fatal" {
			t.Errorf("the original message, class and severity stay: %+v", issue)
		}
	})

	t.Run("a check of the app", func(t *testing.T) {
		xml := regexp.MustCompile(`<ram:IBANID>[^<]*</ram:IBANID>`).ReplaceAllString(valid, "<ram:IBANID>DE00123</ram:IBANID>")
		issue := issueOf(inspect(t, s, xml), "APP-IBAN")
		if issue == nil || issue.Message != "Die IBAN auf der Rechnung ist ungültig: Aufbau, Länge oder Prüfziffer stimmen nicht. Prüfe sie, bevor du überweist." {
			t.Fatalf("got %+v", issue)
		}
	})

	t.Run("the stored document keeps the wording", func(t *testing.T) {
		xml := replaceOnce(t, valid, "<ram:BuyerReference>CUSTOMER-1</ram:BuyerReference>", "")
		imported, err := s.ImportReceivedInvoice("lieferant.xml", base64.StdEncoding.EncodeToString([]byte(xml)))
		if err != nil {
			t.Fatal(err)
		}
		list, err := s.ListReceivedInvoices()
		if err != nil || len(list) == 0 {
			t.Fatal(list, err)
		}
		for _, item := range list {
			if item.ID != imported.Invoice.ID {
				continue
			}
			if issue := issueOf(item.Inspection, "BR-DE-15"); issue == nil || issue.Message != "Die Rechnung enthält keine Käuferreferenz (Leitweg-ID)." {
				t.Fatalf("got %+v", issue)
			}
		}
	})

	t.Run("issuing keeps speaking to the issuer", func(t *testing.T) {
		draft := testDraft()
		draft.Profile, draft.Syntax = "xrechnung-cii", "cii"
		draft.Customer.Endpoint, draft.Customer.EndpointScheme = draft.Customer.Email, "EM"
		draft.BuyerReference = ""
		view, err := s.ValidateInvoice(draft)
		if err != nil {
			t.Fatal(err)
		}
		found := false
		for _, issue := range view.Validation.Issues {
			found = found || issue.Message == "Ergänze die Käuferreferenz bzw. Leitweg-ID, die dir der Empfänger mitgeteilt hat."
		}
		if !found {
			t.Fatalf("the issuer's own text changed: %+v", view.Validation.Issues)
		}
	})
}
