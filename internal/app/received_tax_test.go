package app

import (
	"encoding/base64"
	"strings"
	"testing"

	"papierleicht.de/e-rechnung-studio/internal/model"
	"papierleicht.de/e-rechnung-studio/internal/profile"
	"papierleicht.de/e-rechnung-studio/internal/store"
)

// issuedXML returns a valid document the app generates for the given profile.
func issuedXML(t *testing.T, s *InvoiceService, profile, syntax string) string {
	t.Helper()
	draft := testDraft()
	draft.Profile, draft.Syntax = profile, syntax
	if strings.HasPrefix(profile, "xrechnung-") {
		draft.Customer.Endpoint, draft.Customer.EndpointScheme = draft.Customer.Email, "EM"
		draft.BuyerReference = "CUSTOMER-1"
	}
	view, err := s.ValidateInvoice(draft)
	if err != nil || view.Validation.State != "valid" {
		t.Fatalf("%v %+v", err, view.Validation)
	}
	return view.XML
}

func replaceOnce(t *testing.T, xml, old, replacement string) string {
	t.Helper()
	if !strings.Contains(xml, old) {
		t.Fatalf("test document has no %q", old)
	}
	return strings.Replace(xml, old, replacement, 1)
}

func inspect(t *testing.T, s *InvoiceService, xml string) model.ImportInspection {
	t.Helper()
	got, err := s.InspectInvoiceFile("lieferant.xml", base64.StdEncoding.EncodeToString([]byte(xml)))
	if err != nil {
		t.Fatal(err)
	}
	return got
}

func issueOf(inspection model.ImportInspection, rule string) *model.ValidationIssue {
	for i := range inspection.Validation.Issues {
		if inspection.Validation.Issues[i].RuleID == rule {
			return &inspection.Validation.Issues[i]
		}
	}
	return nil
}

func receivedService(t *testing.T) *InvoiceService {
	t.Helper()
	s, err := NewInvoiceService(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	company := testCompany()
	company.Endpoint, company.EndpointScheme, company.Phone = company.Email, "EM", "030123456"
	if _, err := s.SaveCompany(company); err != nil {
		t.Fatal(err)
	}
	return s
}

// BMF letter of 15.10.2026, Rn. 22 and 23: only a finding that concerns a
// Pflichtangabe keeps a received e-invoice out of the Vorsteuer.
func TestReceivedFindingsAreClassifiedAndDecideTheInputTax(t *testing.T) {
	s := receivedService(t)
	valid := issuedXML(t, s, "xrechnung-cii", "cii")
	if got := inspect(t, s, valid); got.Validation.State != "valid" || got.InputTax == nil || !got.InputTax.Allowed || got.InputTax.Reason != "" {
		t.Fatalf("a passed check needs no note: %+v", got)
	}

	t.Run("missing buyer reference (BT-10) is a business-rule error without tax effect", func(t *testing.T) {
		got := inspect(t, s, replaceOnce(t, valid, "<ram:BuyerReference>CUSTOMER-1</ram:BuyerReference>", ""))
		issue := issueOf(got, "BR-DE-15")
		if got.Validation.State != "invalid" || issue == nil || issue.Class != "business" {
			t.Fatalf("expected BR-DE-15 as business-rule error: %+v", got.Validation)
		}
		if !got.InputTax.Allowed || got.InputTax.Reason == "" {
			t.Fatalf("BT-10 must not block the Vorsteuer: %+v", got.InputTax)
		}
	})

	t.Run("a wrong tax amount is a business-rule error that blocks", func(t *testing.T) {
		got := inspect(t, s, replaceOnce(t, valid, "<ram:CalculatedAmount>38.00</ram:CalculatedAmount>", "<ram:CalculatedAmount>39.00</ram:CalculatedAmount>"))
		if got.Validation.State != "invalid" || got.InputTax.Allowed || got.InputTax.Blocker != "business" {
			t.Fatalf("got %+v %+v", got.Validation, got.InputTax)
		}
	})

	t.Run("a schema violation is a format error", func(t *testing.T) {
		got := inspect(t, s, replaceOnce(t, valid, "<rsm:ExchangedDocument>", "<rsm:ExchangedDocument><ram:Bogus/>"))
		if got.Validation.State != "invalid" || got.InputTax.Allowed || got.InputTax.Blocker != "format" {
			t.Fatalf("got %+v %+v", got.Validation, got.InputTax)
		}
		if len(got.Validation.Issues) == 0 || got.Validation.Issues[0].Class != "format" {
			t.Fatalf("the finding needs its class: %+v", got.Validation.Issues)
		}
	})

	t.Run("a PDF without invoice data cannot be checked", func(t *testing.T) {
		got, err := s.InspectInvoiceFile("beleg.PDF", base64.StdEncoding.EncodeToString(receivedTestPDF(false)))
		if err != nil || got.InputTax == nil || got.InputTax.Allowed || got.InputTax.Blocker != "unchecked" {
			t.Fatalf("got %+v %v", got.InputTax, err)
		}
	})
}

func TestStoredReceivedDocumentsCarryClassAndStatus(t *testing.T) {
	s := receivedService(t)
	xml := issuedXML(t, s, "xrechnung-cii", "cii")
	xml = replaceOnce(t, xml, "<ram:BuyerReference>CUSTOMER-1</ram:BuyerReference>", "")
	imported, err := s.ImportReceivedInvoice("lieferant.xml", base64.StdEncoding.EncodeToString([]byte(xml)))
	if err != nil {
		t.Fatal(err)
	}
	list, err := s.ListReceivedInvoices()
	if err != nil || len(list) != 1 {
		t.Fatal(list, err)
	}
	if list[0].ID != imported.Invoice.ID || list[0].Inspection.InputTax == nil || !list[0].Inspection.InputTax.Allowed {
		t.Fatalf("status missing from the list: %+v", list[0].Inspection.InputTax)
	}
	if issue := issueOf(list[0].Inspection, "BR-DE-15"); issue == nil || issue.Class != "business" {
		t.Fatalf("class missing from the list: %+v", list[0].Inspection.Validation.Issues)
	}
}

// BMF letter of 15.10.2026, Rn. 3: a final invoice may deduct the payments made
// before as a sum in BT-113. The open amount is then the amount due (BT-115).
func TestFinalInvoiceWithPaidAmountOpensTheAmountDue(t *testing.T) {
	s := receivedService(t)
	xml := issuedXML(t, s, "facturx-en16931", "cii")
	xml = replaceOnce(t, xml, "<ram:DuePayableAmount>238.00</ram:DuePayableAmount>", "<ram:TotalPrepaidAmount>100.00</ram:TotalPrepaidAmount><ram:DuePayableAmount>138.00</ram:DuePayableAmount>")
	imported, err := s.ImportReceivedInvoice("endrechnung.xml", base64.StdEncoding.EncodeToString([]byte(xml)))
	if err != nil {
		t.Fatal(err)
	}
	inspection := imported.Invoice.Inspection
	if inspection.Validation.State != "valid" || inspection.Totals.Gross != "238.00" || inspection.Totals.Prepaid != "100.00" || inspection.Totals.Due != "138.00" {
		t.Fatalf("%+v", inspection)
	}
	balance := func() store.PaymentBalance {
		overview, err := s.Payments()
		if err != nil {
			t.Fatal(err)
		}
		for _, b := range overview.Invoices {
			if b.ID == imported.Invoice.ID && b.Direction == "incoming" {
				return b
			}
		}
		t.Fatal("no balance for the final invoice")
		return store.PaymentBalance{}
	}
	// The test invoice's due date has passed, so the state is "überfällig", not "offen".
	if b := balance(); b.Remaining != "138.00" || b.State == "bezahlt" {
		t.Fatalf("open amount must be the amount due: %+v", b)
	}
	if _, err := s.RecordPayment(store.Payment{InvoiceID: imported.Invoice.ID, Direction: "incoming", Currency: "EUR", Amount: "138.00", Date: "2026-10-20", Source: "manual", Reason: "Restzahlung"}); err != nil {
		t.Fatal(err)
	}
	if b := balance(); b.Remaining != "0.00" || b.State != "bezahlt" {
		t.Fatalf("paying the amount due must settle the invoice: %+v", b)
	}
}

func receivedFor(id string, validation model.ValidationResult, totals model.Totals) model.ReceivedInvoice {
	return model.ReceivedInvoice{ID: id, Status: "active", Inspection: model.ImportInspection{InvoiceNumber: id, SellerName: "Lieferant " + id, IssueDate: "2026-10-02", Currency: "EUR", Totals: totals, Validation: validation}}
}

func entryNote(issues []FinanceIssue, code, id string) (string, bool) {
	for _, issue := range issues {
		if issue.Code != code {
			continue
		}
		for _, e := range issue.Entries {
			if e.ID == id {
				return e.Note, true
			}
		}
	}
	return "", false
}

func TestVorsteuerCountsReceivedDocumentsByTheKindOfFinding(t *testing.T) {
	totals := model.Totals{Net: "100.00", Tax: "19.00", Gross: "119.00"}
	rule := func(id string) model.ValidationResult {
		return model.ValidationResult{State: "invalid", Issues: []model.ValidationIssue{{Stage: "rules", RuleID: id, Severity: "error"}}}
	}
	in := baseInput()
	in.Received = []model.ReceivedInvoice{
		receivedFor("passed", model.ValidationResult{State: "valid"}, totals),
		receivedFor("buyer-reference", rule("BR-DE-15"), totals),
		receivedFor("amounts", rule("BR-CO-15"), totals),
		receivedFor("format", model.ValidationResult{State: "invalid", Issues: []model.ValidationIssue{{Stage: "xsd", RuleID: "XSD", Severity: "error"}}}, totals),
	}
	r := buildFinanceReport(in)
	assertTraceable(t, r)
	if r.InputTax.Amount != "38.00" || len(r.InputTax.Entries) != 2 {
		t.Fatalf("passed and BT-10 documents count, the others stay out: %+v", r.InputTax)
	}
	if note, ok := entryNote(r.Issues, "received_needs_review", "amounts"); !ok || note != "Regelverstoß prüfen" {
		t.Fatalf("amounts: %q %v", note, ok)
	}
	if note, ok := entryNote(r.Issues, "received_needs_review", "format"); !ok || note != "Formatfehler" {
		t.Fatalf("format: %q %v", note, ok)
	}
}

func TestVorsteuerOfAFinalInvoiceAsksToCheckTheDownPayments(t *testing.T) {
	in := baseInput()
	in.Received = []model.ReceivedInvoice{receivedFor("final", model.ValidationResult{State: "valid"}, model.Totals{Net: "100.00", Tax: "19.00", Gross: "119.00", Due: "59.50", Prepaid: "59.50"})}
	r := buildFinanceReport(in)
	if r.InputTax.Amount != "9.50" || !hasIssue(r, "received_prepaid_review") {
		t.Fatalf("the remaining payment counts, with a hint on the down payments: %+v %+v", r.InputTax, r.Issues)
	}
	if note, ok := entryNote(r.Issues, "received_prepaid_review", "final"); !ok || !strings.Contains(note, "Restzahlung") {
		t.Fatalf("note %q %v", note, ok)
	}
}

func TestDashboardCountsOnlyDocumentsWhoseFindingsCanMatterForTheVorsteuer(t *testing.T) {
	totals := model.Totals{Net: "100.00", Tax: "19.00", Gross: "119.00"}
	rule := func(id string) model.ValidationResult {
		return model.ValidationResult{State: "invalid", Issues: []model.ValidationIssue{{Stage: "rules", RuleID: id, Severity: "error"}}}
	}
	in := baseInput()
	in.Received = []model.ReceivedInvoice{
		receivedFor("passed", model.ValidationResult{State: "valid"}, totals),
		receivedFor("buyer-reference", rule("BR-DE-15"), totals),
		receivedFor("amounts", rule("BR-CO-15"), totals),
	}
	if got := buildDashboard(in, buildFinanceReport(in)).ReceivedToReview; got != 1 {
		t.Fatalf("only the document with an amounts finding needs review, got %d", got)
	}
}

// A PDF without invoice data has no date of its own; once its amounts are
// captured as a receipt, that receipt carries the date and the PDF is done.
func TestDashboardSkipsPDFsCapturedAsReceipts(t *testing.T) {
	in := baseInput()
	plain := receivedFor("plain", model.ValidationResult{State: "valid"}, model.Totals{})
	plain.Inspection.IssueDate = ""
	in.Received = []model.ReceivedInvoice{plain}
	if got := buildDashboard(in, buildFinanceReport(in)).ReceivedToReview; got != 1 {
		t.Fatalf("a PDF without date needs review, got %d", got)
	}
	in.Expenses = []store.Expense{{ID: "exp-1", ReceivedID: plain.ID, Date: "2026-10-15", Gross: "100.00"}}
	if got := buildDashboard(in, buildFinanceReport(in)).ReceivedToReview; got != 0 {
		t.Fatalf("a captured PDF still needs review, got %d", got)
	}
}

// BR-CO-26 asks for BT-29, BT-30 or BT-31. A supplier with only a tax number (BT-32)
// still meets the Pflichtangabe of § 14 Abs. 4 Satz 1 Nr. 2 UStG.
func TestSupplierWithOnlyATaxNumberStillCountsForTheVorsteuer(t *testing.T) {
	s := receivedService(t)
	company := testCompany()
	company.Endpoint, company.EndpointScheme, company.Phone = company.Email, "EM", "030123456"
	company.VATID, company.TaxNumber = "", "12/345/67890"
	if _, err := s.SaveCompany(company); err != nil {
		t.Fatal(err)
	}
	valid := issuedXML(t, s, "xrechnung-cii", "cii")
	// The app writes the tax number as BT-29 as well; strip that so only BT-32 is left.
	withoutIdentifier := replaceOnce(t, valid, "<ram:ID>12/345/67890</ram:ID>", "")

	got := inspect(t, s, withoutIdentifier)
	issue := issueOf(got, "BR-CO-26")
	if issue == nil || issue.Severity != "warning" || got.Validation.State != "warning" {
		t.Fatalf("BR-CO-26 must stay visible as a warning: %+v", got.Validation)
	}
	if !got.InputTax.Allowed {
		t.Fatalf("the tax number is the Pflichtangabe: %+v", got.InputTax)
	}

	// Without any tax identifier the document is still held back.
	bare := strings.ReplaceAll(withoutIdentifier, "12/345/67890", "")
	if got := inspect(t, s, bare); got.InputTax.Allowed {
		t.Fatalf("no tax identifier at all must not count: %+v %+v", got.InputTax, got.Validation)
	}
}

// BR-CO-26 applies to every format. The app writes a lone tax number as the
// seller identifier (BT-29) as well, so a business without a VAT ID can issue.
func TestIssuingWithOnlyATaxNumberWorksInEveryFormat(t *testing.T) {
	s := receivedService(t)
	company := testCompany()
	company.Endpoint, company.EndpointScheme, company.Phone = company.Email, "EM", "030123456"
	company.VATID, company.TaxNumber = "", "12/345/67890"
	if _, err := s.SaveCompany(company); err != nil {
		t.Fatal(err)
	}
	for _, format := range []string{"facturx-en16931", "facturx-basic", "facturx-extended", "xrechnung-cii", "xrechnung-ubl"} {
		t.Run(format, func(t *testing.T) {
			f, err := profile.Get(format)
			if err != nil {
				t.Fatal(err)
			}
			d := testDraft()
			d.Number, d.Profile, d.Syntax = "", format, f.Syntax
			d.Customer.Endpoint, d.Customer.EndpointScheme = d.Customer.Email, "EM"
			d.BuyerReference = "CUSTOMER-1"
			issued, err := s.IssueInvoice(d)
			if err != nil || issued.Draft.Status != "issued" || issued.Validation.State != "valid" {
				t.Fatalf("%v %+v", err, issued.Validation)
			}
		})
	}
}
