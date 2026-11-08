package app

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/beevik/etree"
	"papierleicht.de/e-rechnung-studio/internal/invoicexml"
	"papierleicht.de/e-rechnung-studio/internal/model"
)

func TestBasicDetailsAndImmutablePresentation(t *testing.T) {
	s, err := NewInvoiceService(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	c := testCompany()
	c.Phone = "+49 30 1234567"
	if _, err := s.SaveCompany(c); err != nil {
		t.Fatal(err)
	}
	d := testDraft()
	d.Profile = "facturx-basic"
	d.Number = "BASIC-DETAILS"
	d.Customer.ContactName = "Erika Beispiel"
	d.BuyerReference = "BESTELLUNG-2026-42"
	d.PaymentReference = "ZAHLUNG-42"
	d.PeriodStart, d.PeriodEnd = "2026-09-01", "2026-09-09"
	d.Notes = "Zusätzliche Vereinbarung bleibt erhalten."
	d.Lines[0].ArticleNumber = "ART-42"
	d.Lines[0].Description = "Ausführliche Leistungsbeschreibung mit Umlauten: äöü.\nZweite Zeile bleibt vollständig erhalten."
	d.IssuerSnapshot = &model.Company{Name: "Falscher Absender", Email: "falsch@example.de"}
	saved, err := s.SaveInvoice(d)
	if err != nil || saved.Draft.IssuerSnapshot != nil {
		t.Fatalf("caller snapshot persisted: %v", err)
	}
	d = saved.Draft
	issued, err := s.IssueInvoice(d)
	if err != nil {
		t.Fatal(err)
	}
	if issued.Draft.IssuerSnapshot == nil || issued.Draft.IssuerSnapshot.Email != c.Email {
		t.Fatal("issuer snapshot missing or not authoritative")
	}
	xml := []byte(issued.XML)
	parsed, err := invoicexml.Parse(xml)
	if err != nil {
		t.Fatal(err)
	}
	if len(parsed.InvoiceLines) != 1 || parsed.InvoiceLines[0].Note != d.Lines[0].Description+"\nArt.-Nr.: ART-42" {
		t.Fatal("description or article ID lost")
	}
	if parsed.BuyerReference != d.BuyerReference || parsed.PaymentReference != d.PaymentReference || parsed.BillingSpecifiedPeriodEnd.Format("2006-01-02") != d.PeriodEnd {
		t.Fatal("BASIC references or period lost")
	}
	if strings.Contains(issued.XML, "DefinedTradeContact") || strings.Contains(issued.XML, "AccountName") || strings.Contains(issued.XML, "FinancialInstitution") {
		t.Fatal("unsupported BASIC elements emitted")
	}
	pdf, err := s.invoicePDF(d.ID)
	if err != nil {
		t.Fatal(err)
	}
	changed := c
	changed.Name, changed.Email, changed.Phone, changed.BankName = "Neuer Absender", "neu@example.de", "999999", "Neue Bank"
	if _, err := s.SaveCompany(changed); err != nil {
		t.Fatal(err)
	}
	opened, err := s.OpenInvoice(d.ID)
	if err != nil || opened.Company.Name != c.Name || opened.Company.Email != c.Email || opened.Company.Phone != c.Phone || opened.Company.BankName != c.BankName || opened.Company.BIC != c.BIC {
		t.Fatalf("issued company changed: %v %+v", err, opened.Company)
	}
	if _, err := s.SaveInvoice(opened.Draft); err == nil {
		t.Fatal("issued BASIC invoice editable")
	}
	// Regeneration after a backup without the cached PDF must use its issuer
	// snapshot, including contact/bank fields that BASIC cannot store in XML.
	backup, err := s.ExportBackup()
	if err != nil {
		t.Fatal(err)
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(backup), &payload); err != nil {
		t.Fatal(err)
	}
	var artifacts []any
	for _, raw := range payload["artifacts"].([]any) {
		if raw.(map[string]any)["kind"] != "facturx-pdf" {
			artifacts = append(artifacts, raw)
		}
	}
	payload["artifacts"] = artifacts
	withoutPDF, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	restored, err := NewInvoiceService(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer restored.Close()
	if err := restored.RestoreBackup(string(withoutPDF)); err != nil {
		t.Fatal(err)
	}
	regenerated, err := restored.invoicePDF(d.ID)
	if err != nil || !bytes.HasPrefix(regenerated, []byte("%PDF-")) {
		t.Fatalf("regeneration failed: %v", err)
	}
	writeFormatFixture(t, "facturx-basic-details", xml, issued.Validation)
	if root := os.Getenv("E_RECHNUNG_STUDIO_FORMAT_TEST_OUTPUT"); root != "" {
		for name, data := range map[string][]byte{"facturx-basic-details.pdf": pdf, "facturx-basic-details-regenerated.pdf": regenerated, "facturx-basic-details-embedded.xml": xml, "facturx-basic-details-regenerated-embedded.xml": xml} {
			if err := os.WriteFile(filepath.Join(root, name), data, 0600); err != nil {
				t.Fatal(err)
			}
		}
	}
	for _, m := range []struct{ name, query, value, rule string }{
		{"bad-total", "//ram:GrandTotalAmount", "999.99", "BR-CO-15"},
		{"missing-number", "//rsm:ExchangedDocument/ram:ID", "", "BR-02"},
		{"bad-tax", "//ram:TaxTotalAmount", "99.99", "BR-CO-14"},
	} {
		t.Run(m.name, func(t *testing.T) {
			doc := etree.NewDocument()
			if err := doc.ReadFromBytes(xml); err != nil {
				t.Fatal(err)
			}
			node := doc.FindElement(m.query)
			if node == nil {
				t.Fatal("mutation target missing")
			}
			node.SetText(m.value)
			raw, _ := doc.WriteToBytes()
			result := s.validator.Validate(raw)
			found := false
			for _, i := range result.Issues {
				if i.RuleID == m.rule {
					found = true
				}
			}
			if result.State != "invalid" || !found {
				t.Fatalf("expected %s: %+v", m.rule, result)
			}
			writeFormatFixture(t, "facturx-basic-"+m.name, raw, result)
		})
	}
}
