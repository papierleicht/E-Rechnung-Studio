package app

import (
	"strings"
	"sync"
	"testing"

	"papierleicht.de/e-rechnung-studio/internal/einvoice"
	"papierleicht.de/e-rechnung-studio/internal/model"
)

func testCompany() model.Company {
	return model.Company{Name: "Muster GmbH", ContactName: "Max Mustermann", Email: "rechnung@example.de", VATID: "DE123456789", IBAN: "DE89370400440532013000", BIC: "COBADEFFXXX", BankName: "Musterbank", Address: model.Address{Street: "Musterweg 1", Postcode: "10115", City: "Berlin", Country: "DE"}}
}

func testDraft() model.InvoiceDraft {
	d := model.NewDraft()
	d.Number = "TEST-2026-001"
	d.IssueDate = "2026-09-09"
	d.DeliveryDate = "2026-09-09"
	d.DueDate = "2026-09-23"
	d.PaymentTerms = "Zahlbar innerhalb von 14 Tagen."
	d.Customer = model.Customer{Name: "Beispielkunde AG", Email: "kunde@example.eu", VATID: "FR40303265045", Address: model.Address{Street: "Rue Exemple 2", Postcode: "75001", City: "Paris", Country: "FR"}}
	d.Lines = []model.InvoiceLine{{ID: "1", Name: "Beratung", Quantity: "2", Unit: "HUR", UnitPrice: "100.00", TaxRate: "19", TaxCategory: "S"}}
	return d
}

func TestCompanySetupIsCompletedOnceAndRestored(t *testing.T) {
	root := t.TempDir()
	s, err := NewInvoiceService(root)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.CompleteSetup(model.Company{}); err == nil {
		s.Close()
		t.Fatal("empty setup accepted")
	}
	initial, err := s.Bootstrap()
	if err != nil || initial.Company.SetupCompleted {
		s.Close()
		t.Fatalf("failed setup was persisted: %+v %v", initial.Company, err)
	}
	c := testCompany()
	c.SmallBusiness = true
	c.TaxNumber = "12/345/67890"
	c.ExemptionReason = "Umsatzsteuerbefreit nach § 19 UStG"
	c.Address.Country = "de"
	if _, err = s.CompleteSetup(c); err != nil {
		s.Close()
		t.Fatal(err)
	}
	if err = s.Close(); err != nil {
		t.Fatal(err)
	}
	s, err = NewInvoiceService(root)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	b, err := s.Bootstrap()
	if err != nil {
		t.Fatal(err)
	}
	if !b.Company.SetupCompleted || b.Company.Name != c.Name || b.Company.Address.Country != "DE" || b.Draft.TaxTreatment != "small-business" || b.Draft.TaxExemptionReason != c.ExemptionReason {
		t.Fatalf("setup not restored: %+v", b)
	}
	view, err := s.ValidateInvoice(testDraft())
	if err != nil || !strings.Contains(view.XML, c.TaxNumber) {
		t.Fatalf("configured tax number missing from XML: %v", err)
	}
}

func TestDraftToValidatedXMLAndIssue(t *testing.T) {
	s, err := NewInvoiceService(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if _, err = s.SaveCompany(testCompany()); err != nil {
		t.Fatal(err)
	}
	previewDraft := testDraft()
	previewDraft.Number = ""
	preview, err := s.PreviewInvoice(previewDraft)
	if err != nil {
		t.Fatal(err)
	}
	if preview.Validation.State != "valid" || preview.Draft.Number != "RE-2026-0001" || !preview.Draft.AutomaticNumber {
		t.Fatalf("unnumbered draft readiness=%s number=%q issues=%+v", preview.Validation.State, preview.Draft.Number, preview.Validation.Issues)
	}

	validated, err := s.ValidateInvoice(testDraft())
	if err != nil {
		t.Fatal(err)
	}
	if validated.Validation.State != "valid" {
		t.Fatalf("validation: %#v", validated.Validation.Issues)
	}
	if validated.Totals.Gross != "238.00" {
		t.Fatalf("gross=%s", validated.Totals.Gross)
	}
	if !strings.Contains(validated.XML, "TEST-2026-001") {
		t.Fatal("XML does not contain invoice number")
	}

	d := testDraft()
	d.Number = ""
	saved, err := s.SaveInvoice(d)
	if err != nil {
		t.Fatal(err)
	}
	issued, err := s.IssueInvoice(saved.Draft)
	if err != nil {
		t.Fatal(err)
	}
	if issued.Draft.Status != "issued" || !strings.HasPrefix(issued.Draft.Number, "RE-2026-") {
		t.Fatalf("issued=%+v", issued.Draft)
	}
	if !strings.Contains(issued.XML, issued.Draft.Number) {
		t.Fatal("issued artifact and number disagree")
	}
	artifact, err := s.store.Artifact(issued.Draft.ID, "cii-xml")
	if err != nil {
		t.Fatal(err)
	}
	if issued.XML != string(artifact) {
		t.Fatal("opened XML is not the immutable issued artifact")
	}

	issued.Draft.Customer.Name = "Changed after issue"
	if _, err = s.SaveInvoice(issued.Draft); err == nil {
		t.Fatal("issued invoice was mutable")
	}
	if err = s.SetPaid(issued.Draft.ID, true); err != nil {
		t.Fatal(err)
	}
	reopened, err := s.OpenInvoice(issued.Draft.ID)
	if err != nil {
		t.Fatal(err)
	}
	if reopened.Draft.Status != "paid" || reopened.Totals.Gross != "238.00" {
		t.Fatalf("paid invoice=%+v totals=%+v", reopened.Draft, reopened.Totals)
	}
	if reopened.Draft.Customer.Name == "Changed after issue" {
		t.Fatal("immutable invoice content changed")
	}
}

func TestConcurrentIssueOnlyCommitsOnce(t *testing.T) {
	s, err := NewInvoiceService(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if _, err = s.SaveCompany(testCompany()); err != nil {
		t.Fatal(err)
	}
	draft := testDraft()
	draft.Number = ""
	saved, err := s.SaveInvoice(draft)
	if err != nil {
		t.Fatal(err)
	}

	start := make(chan struct{})
	errs := make(chan error, 2)
	var wg sync.WaitGroup
	for range 2 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			_, issueErr := s.IssueInvoice(saved.Draft)
			errs <- issueErr
		}()
	}
	close(start)
	wg.Wait()
	close(errs)

	succeeded := 0
	for issueErr := range errs {
		if issueErr == nil {
			succeeded++
		}
	}
	if succeeded != 1 {
		t.Fatalf("concurrent issue succeeded %d times, want exactly once", succeeded)
	}
	opened, err := s.OpenInvoice(saved.Draft.ID)
	if err != nil || opened.Draft.Status != "issued" || opened.Draft.Number == "" {
		t.Fatalf("committed invoice: status=%q number=%q err=%v", opened.Draft.Status, opened.Draft.Number, err)
	}
}

func TestIncompleteDraftIsNotValid(t *testing.T) {
	s, err := NewInvoiceService(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	view, err := s.ValidateInvoice(model.NewDraft())
	if err != nil {
		t.Fatal(err)
	}
	if view.Validation.State == "valid" {
		t.Fatal("empty draft unexpectedly valid")
	}
	fields := map[string]bool{}
	for _, issue := range view.Validation.Issues {
		fields[issue.Field] = true
	}
	for _, field := range []string{"company.name", "customer.name", "customer.address.country", "lines.0.name", "lines.0.unitPrice"} {
		if !fields[field] {
			t.Errorf("incomplete draft does not explain %s", field)
		}
	}
}

func TestOfficialRulesRejectMissingBuyerName(t *testing.T) {
	s, err := NewInvoiceService(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if _, err = s.SaveCompany(testCompany()); err != nil {
		t.Fatal(err)
	}
	draft := testDraft()
	draft.Customer.Name = ""
	view, err := s.ValidateInvoice(draft)
	if err != nil {
		t.Fatal(err)
	}
	if view.Validation.State != "invalid" {
		t.Fatalf("missing buyer name validation=%+v", view.Validation)
	}
	foundRuleIssue := false
	for _, issue := range view.Validation.Issues {
		if issue.Stage == "rules" {
			foundRuleIssue = true
			if issue.RuleID == "BR-07" && (issue.Field != "customer.name" || issue.Details == "" || !strings.Contains(issue.Message, "Rechnungsempfängers")) {
				t.Fatalf("missing German buyer guidance: %+v", issue)
			}
			break
		}
	}
	if !foundRuleIssue {
		t.Fatalf("expected an official rule issue, got %+v", view.Validation.Issues)
	}
}

func TestBackupRoundTripPreservesIssuedArtifact(t *testing.T) {
	s, err := NewInvoiceService(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.SaveCompany(testCompany()); err != nil {
		t.Fatal(err)
	}
	customer, err := s.SaveCustomer(testDraft().Customer)
	if err != nil {
		t.Fatal(err)
	}
	draft := testDraft()
	draft.Number = ""
	draft.CustomerID = customer.ID
	saved, err := s.SaveInvoice(draft)
	if err != nil {
		t.Fatal(err)
	}
	issued, err := s.IssueInvoice(saved.Draft)
	if err != nil {
		t.Fatal(err)
	}
	backup, err := s.ExportBackup()
	if err != nil {
		t.Fatal(err)
	}
	if err = s.Close(); err != nil {
		t.Fatal(err)
	}

	restored, err := NewInvoiceService(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer restored.Close()
	if err = restored.RestoreBackup(backup); err != nil {
		t.Fatal(err)
	}
	state, err := restored.Bootstrap()
	if err != nil {
		t.Fatal(err)
	}
	if len(state.Customers) != 1 || len(state.Invoices) != 1 {
		t.Fatalf("restored state=%+v", state)
	}
	reopened, err := restored.OpenInvoice(issued.Draft.ID)
	if err != nil {
		t.Fatal(err)
	}
	if reopened.Draft.Number != issued.Draft.Number || reopened.XML != issued.XML {
		t.Fatal("restored issued snapshot or XML differs")
	}
}

func TestInspectXMLValidatesOriginalBytes(t *testing.T) {
	s, err := NewInvoiceService(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if _, err = s.SaveCompany(testCompany()); err != nil {
		t.Fatal(err)
	}
	generated, err := s.ValidateInvoice(testDraft())
	if err != nil {
		t.Fatal(err)
	}
	inspection, err := s.InspectXML(generated.XML)
	if err != nil {
		t.Fatal(err)
	}
	if inspection.Validation.State != "valid" || inspection.InvoiceNumber != "TEST-2026-001" || inspection.Syntax != "cii" {
		t.Fatalf("inspection=%+v", inspection)
	}

	doctype, err := s.InspectXML(`<!DOCTYPE invoice SYSTEM "https://example.invalid/invoice.dtd"><invoice/>`)
	if err != nil {
		t.Fatal(err)
	}
	if doctype.Validation.State == "valid" || len(doctype.Validation.Issues) == 0 || doctype.Validation.Issues[0].RuleID != "DOCTYPE" {
		t.Fatalf("doctype inspection=%+v", doctype)
	}
}

func TestSmallBusinessInvoiceUsesZeroTaxAndVisibleReason(t *testing.T) {
	s, err := NewInvoiceService(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	company := testCompany()
	company.SmallBusiness = true
	company.ExemptionReason = "Umsatzsteuerbefreit nach § 19 UStG"
	if _, err = s.SaveCompany(company); err != nil {
		t.Fatal(err)
	}
	draft := testDraft()
	draft.TaxTreatment = "small-business"
	view, err := s.ValidateInvoice(draft)
	if err != nil {
		t.Fatal(err)
	}
	if view.Validation.State != "valid" || view.Totals.Tax != "0.00" || view.Totals.Gross != "200.00" {
		t.Fatalf("small business view=%+v", view)
	}
	if !strings.Contains(view.PreviewHTML, "§ 19 UStG") {
		t.Fatal("visible invoice omits the tax exemption reason")
	}
	state, err := s.Bootstrap()
	if err != nil {
		t.Fatal(err)
	}
	if state.Draft.TaxTreatment != "small-business" {
		t.Fatal("new drafts do not inherit the company tax treatment")
	}
}

func TestNegativeLineAmountIsRejected(t *testing.T) {
	s, err := NewInvoiceService(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	draft := testDraft()
	draft.Lines[0].UnitPrice = "-1.00"
	view, err := s.ValidateInvoice(draft)
	if err != nil {
		t.Fatal(err)
	}
	if view.Validation.State == "valid" || len(view.Validation.Issues) == 0 {
		t.Fatalf("negative line amount validation=%+v", view.Validation)
	}
}

func TestTaxNumberWithoutVATIDValidatesAndIssues(t *testing.T) {
	for _, small := range []bool{false, true} {
		name := "standard"
		if small {
			name = "small-business"
		}
		t.Run(name, func(t *testing.T) {
			s, err := NewInvoiceService(t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			defer s.Close()
			c := testCompany()
			c.VATID, c.TaxNumber, c.SmallBusiness = "", "12/345/67890", small
			if _, err = s.SaveCompany(c); err != nil {
				t.Fatal(err)
			}
			b, err := s.Bootstrap()
			if err != nil {
				t.Fatal(err)
			}
			d := testDraft()
			d.Number = ""
			d.TaxTreatment, d.TaxExemptionReason = b.Draft.TaxTreatment, b.Draft.TaxExemptionReason
			v, err := s.ValidateInvoice(d)
			if err != nil || v.Validation.State != "valid" {
				t.Fatalf("tax number only: %v %+v", err, v.Validation.Issues)
			}
			inv, err := einvoice.ParseReader(strings.NewReader(v.XML))
			if err != nil {
				t.Fatal(err)
			}
			if inv.Seller.FCTaxRegistration != c.TaxNumber || inv.Seller.VATaxRegistration != "" || len(inv.Seller.ID) != 1 || inv.Seller.ID[0] != c.TaxNumber {
				t.Fatalf("incorrect seller identifiers: %+v", inv.Seller)
			}
			if small && (v.Totals.Tax != "0.00" || inv.InvoiceLines[0].TaxCategoryCode != "E" || !strings.Contains(v.PreviewHTML, "§ 19 UStG")) {
				t.Fatalf("incorrect exemption: %+v", v)
			}
			if !strings.Contains(v.PreviewHTML, c.TaxNumber) {
				t.Fatal("tax number missing from readable invoice")
			}
			issued, err := s.IssueInvoice(d)
			if err != nil || issued.Draft.Status != "issued" {
				t.Fatalf("issue failed: %v", err)
			}
			c.TaxNumber = ""
			if _, err = s.SaveCompany(c); err != nil {
				t.Fatal(err)
			}
			invalid, err := s.ValidateInvoice(d)
			if err != nil {
				t.Fatal(err)
			}
			found := false
			for _, issue := range invalid.Validation.Issues {
				if issue.RuleID == "BR-CO-26" {
					found = true
				}
			}
			if !found {
				t.Fatal("missing identifier must still fail BR-CO-26")
			}
		})
	}
}

func TestCustomerHistoryPreservesIssuedSnapshot(t *testing.T) {
	s, err := NewInvoiceService(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if _, err = s.SaveCompany(testCompany()); err != nil {
		t.Fatal(err)
	}
	c, err := s.SaveCustomer(testDraft().Customer)
	if err != nil {
		t.Fatal(err)
	}
	other, err := s.SaveCustomer(testDraft().Customer)
	if err != nil {
		t.Fatal(err)
	}
	d := testDraft()
	d.Customer = c
	d.CustomerID = c.ID
	issued, err := s.IssueInvoice(d)
	if err != nil {
		t.Fatal(err)
	}
	c.Name = "Renamed customer"
	c.Address.Street = "New address"
	if _, err = s.SaveCustomer(c); err != nil {
		t.Fatal(err)
	}
	d = testDraft()
	d.Number = ""
	d.Customer = other
	d.CustomerID = other.ID
	if _, err = s.SaveInvoice(d); err != nil {
		t.Fatal(err)
	}
	list, err := s.ListInvoices()
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 2 || list[0].CustomerID != other.ID || list[1].CustomerID != c.ID {
		t.Fatalf("customer association/order: %+v", list)
	}
	reopened, err := s.OpenInvoice(issued.Draft.ID)
	if err != nil {
		t.Fatal(err)
	}
	if reopened.Draft.Customer.Name != issued.Draft.Customer.Name || reopened.Draft.Customer.Address.Street != issued.Draft.Customer.Address.Street || reopened.XML != issued.XML {
		t.Fatal("customer edit changed issued invoice")
	}
}
