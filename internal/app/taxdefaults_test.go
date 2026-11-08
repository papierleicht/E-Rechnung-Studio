package app

import "testing"

func TestPerPositionProfileDefault(t *testing.T) {
	s, e := NewInvoiceService(t.TempDir())
	if e != nil {
		t.Fatal(e)
	}
	defer s.Close()
	c := testCompany()
	c.SmallBusiness = true
	preset := "mixed"
	c.InvoiceTaxPreset = &preset
	if _, e = s.SaveCompany(c); e != nil {
		t.Fatal(e)
	}
	boot, e := s.Bootstrap()
	if e != nil {
		t.Fatal(e)
	}
	if boot.Draft.TaxTreatment != "mixed" || boot.Draft.Lines[0].TaxTreatment != "small-business" || !boot.Company.SmallBusiness {
		t.Fatal("per-position default did not retain status")
	}
}

func TestTaxDefaultsDoNotChangeExistingDrafts(t *testing.T) {
	s, e := NewInvoiceService(t.TempDir())
	if e != nil {
		t.Fatal(e)
	}
	defer s.Close()
	c := testCompany()
	if _, e = s.SaveCompany(c); e != nil {
		t.Fatal(e)
	}
	saved, e := s.SaveInvoice(testDraft())
	if e != nil {
		t.Fatal(e)
	}
	preset := "exempt"
	c.InvoiceTaxPreset = &preset
	c.ExemptionReason = "Test-Befreiungsgrund"
	if _, e = s.SaveCompany(c); e != nil {
		t.Fatal(e)
	}
	boot, e := s.Bootstrap()
	if e != nil {
		t.Fatal(e)
	}
	if boot.Draft.TaxTreatment != "exempt" || boot.Draft.TaxSource != "company" || boot.Draft.TaxExemptionReason != c.ExemptionReason {
		t.Fatal("new defaults not applied")
	}
	old, e := s.OpenInvoice(saved.Draft.ID)
	if e != nil {
		t.Fatal(e)
	}
	if old.Draft.TaxTreatment != "standard" || old.Draft.TaxExemptionReason != saved.Draft.TaxExemptionReason {
		t.Fatal("existing draft changed")
	}
	c.TaxStatusUnclear = true
	if _, e = s.SaveCompany(c); e != nil {
		t.Fatal(e)
	}
	if _, e = s.SaveInvoice(testDraft()); e != nil {
		t.Fatal("unclear status must allow drafts", e)
	}
	if _, e = s.IssueInvoice(testDraft()); e == nil {
		t.Fatal("unclear status allowed issuance")
	}
	backup, e := s.ExportBackup()
	if e != nil {
		t.Fatal(e)
	}
	if e = s.RestoreBackup(backup); e != nil {
		t.Fatal(e)
	}
	boot, e = s.Bootstrap()
	if e != nil {
		t.Fatal(e)
	}
	if boot.Company.InvoiceTaxPreset == nil || *boot.Company.InvoiceTaxPreset != "exempt" || !boot.Company.TaxStatusUnclear {
		t.Fatal("preset lost in backup")
	}
}
