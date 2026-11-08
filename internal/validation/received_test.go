package validation

import (
	"testing"

	"papierleicht.de/e-rechnung-studio/internal/einvoice"
	"papierleicht.de/e-rechnung-studio/internal/model"
)

func rule(id, severity string) model.ValidationIssue {
	return model.ValidationIssue{Stage: "rules", RuleID: id, Severity: severity}
}

func TestReceivedInputTax(t *testing.T) {
	cases := []struct {
		name    string
		result  model.ValidationResult
		allowed bool
		blocker string
	}{
		{"passed check", model.ValidationResult{State: "valid"}, true, ""},
		{"buyer reference only (BT-10)", model.ValidationResult{State: "invalid", Issues: []model.ValidationIssue{rule("BR-DE-15", "error")}}, true, ""},
		{"contact and payment rules only", model.ValidationResult{State: "invalid", Issues: []model.ValidationIssue{rule("BR-DE-2", "error"), rule("BR-DE-6", "fatal"), rule("BR-50", "error"), rule("PEPPOL-EN16931-R010", "error")}}, true, ""},
		{"amounts rule", model.ValidationResult{State: "invalid", Issues: []model.ValidationIssue{rule("BR-CO-15", "error")}}, false, ClassBusiness},
		{"tax rate rule next to buyer reference", model.ValidationResult{State: "invalid", Issues: []model.ValidationIssue{rule("BR-DE-15", "error"), rule("BR-S-05", "error")}}, false, ClassBusiness},
		{"seller identifier rule is not tax-irrelevant", model.ValidationResult{State: "invalid", Issues: []model.ValidationIssue{rule("BR-CO-26", "error")}}, false, ClassBusiness},
		{"address rule", model.ValidationResult{State: "invalid", Issues: []model.ValidationIssue{rule("BR-DE-4", "error")}}, false, ClassBusiness},
		{"rule warning does not violate a mandatory rule", model.ValidationResult{State: "warning", Issues: []model.ValidationIssue{rule("BR-CL-23", "warning")}}, true, ""},
		{"format error from the schema", model.ValidationResult{State: "invalid", Issues: []model.ValidationIssue{{Stage: "xsd", RuleID: "XSD", Severity: "error"}}}, false, ClassFormat},
		{"format error beats business rule", model.ValidationResult{State: "invalid", Issues: []model.ValidationIssue{rule("BR-DE-15", "error"), {Stage: "xsd", RuleID: "XSD", Severity: "error"}}}, false, ClassFormat},
		{"unreadable XML", model.ValidationResult{State: "error", Issues: []model.ValidationIssue{{Stage: "syntax", RuleID: "XML", Severity: "error"}}}, false, ClassFormat},
		{"syntax-binding rule", model.ValidationResult{State: "invalid", Issues: []model.ValidationIssue{rule("CII-SR-452", "error")}}, false, ClassFormat},
		{"app hint on IBAN", model.ValidationResult{State: "warning", Issues: []model.ValidationIssue{{Stage: "input", RuleID: "APP-IBAN", Severity: "warning", Field: "company.iban"}}}, true, ""},
		{"app hint on payment terms", model.ValidationResult{State: "warning", Issues: []model.ValidationIssue{{Stage: "input", RuleID: "APP-PAYMENT-TERMS", Severity: "warning", Field: "paymentTerms"}}}, true, ""},
		{"app hint on the supplier's VAT ID", model.ValidationResult{State: "warning", Issues: []model.ValidationIssue{{Stage: "input", RuleID: "APP-VAT-FORMAT", Severity: "warning", Field: "company.vatId"}}}, false, ClassHint},
		{"app hint on the recipient's VAT ID", model.ValidationResult{State: "warning", Issues: []model.ValidationIssue{{Stage: "input", RuleID: "APP-VAT-FORMAT", Severity: "warning", Field: "customer.vatId"}}}, true, ""},
		{"profile the app cannot check", model.ValidationResult{State: "unsupported", Issues: []model.ValidationIssue{{Stage: "profile", RuleID: "UNSUPPORTED", Severity: "error"}}}, false, ClassUnchecked},
		{"PDF without invoice data", model.ValidationResult{State: "unsupported", Issues: []model.ValidationIssue{{Stage: "import", RuleID: "PDF-NO-XML", Severity: "warning"}}}, false, ClassUnchecked},
		{"check could not finish", model.ValidationResult{State: "error", Issues: []model.ValidationIssue{{Stage: "rules", RuleID: "XSLT", Severity: "error"}}}, false, ClassUnchecked},
		{"failed state without findings", model.ValidationResult{State: "invalid"}, false, ClassUnchecked},
		{"unknown state", model.ValidationResult{State: ""}, false, ClassUnchecked},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := ReceivedInputTax(tc.result)
			if got.Allowed != tc.allowed || got.Blocker != tc.blocker {
				t.Fatalf("got %+v, want allowed=%v blocker=%q", got, tc.allowed, tc.blocker)
			}
			if !got.Allowed && (got.Reason == "" || got.Short == "") {
				t.Fatalf("a blocked document needs a reason and a short note: %+v", got)
			}
			if tc.result.State != "valid" && got.Allowed && got.Reason == "" {
				t.Fatalf("an allowed document with findings needs a note: %+v", got)
			}
		})
	}
}

func TestAnnotateReceivedClassifiesWithoutChangingTheInput(t *testing.T) {
	in := model.ImportInspection{Validation: model.ValidationResult{State: "invalid", Issues: []model.ValidationIssue{
		{Stage: "xsd", RuleID: "XSD", Severity: "error"},
		rule("BR-DE-15", "error"),
		{Stage: "input", RuleID: "APP-IBAN", Severity: "warning"},
		{Stage: "import", RuleID: "PDF-NO-XML", Severity: "warning"},
	}}}
	out := AnnotateReceived(in)
	want := []string{ClassFormat, ClassBusiness, ClassHint, ClassUnchecked}
	for i, class := range want {
		if out.Validation.Issues[i].Class != class {
			t.Errorf("issue %d: class %q, want %q", i, out.Validation.Issues[i].Class, class)
		}
		if in.Validation.Issues[i].Class != "" {
			t.Errorf("issue %d of the argument was changed", i)
		}
	}
	if out.InputTax == nil || out.InputTax.Allowed || out.InputTax.Blocker != ClassFormat {
		t.Fatalf("status %+v", out.InputTax)
	}
	if again := AnnotateReceived(out); again.InputTax.Blocker != ClassFormat || again.Validation.Issues[1].Class != ClassBusiness {
		t.Fatalf("annotating twice changed the result: %+v", again)
	}
}

func TestAnnotateReceivedKeepsAnEmptyListEmpty(t *testing.T) {
	out := AnnotateReceived(model.ImportInspection{Validation: model.ValidationResult{State: "valid"}})
	if out.Validation.Issues != nil || out.InputTax == nil || !out.InputTax.Allowed {
		t.Fatalf("got %+v", out)
	}
}

func TestRelaxSellerIdentifierWhenTheTaxNumberIsThere(t *testing.T) {
	finding := model.ValidationIssue{Stage: "rules", RuleID: "BR-CO-26", Severity: "error", Message: "Eine Verkäuferkennung fehlt.", Details: "[BR-CO-26] original"}
	inspection := func(issues ...model.ValidationIssue) model.ImportInspection {
		return model.ImportInspection{Validation: model.ValidationResult{State: "invalid", Issues: issues}}
	}
	withTaxNumber := &einvoice.Invoice{Seller: einvoice.Party{FCTaxRegistration: "12/345/67890"}}

	got := RelaxSellerIdentifier(inspection(finding), withTaxNumber)
	if got.Validation.State != "warning" || got.Validation.Issues[0].Severity != "warning" {
		t.Fatalf("the finding must become a warning: %+v", got.Validation)
	}
	if got.Validation.Issues[0].Details != finding.Details || got.Validation.Issues[0].Message == finding.Message {
		t.Fatalf("the original stays in the details, the message explains: %+v", got.Validation.Issues[0])
	}
	if status := ReceivedInputTax(got.Validation); !status.Allowed {
		t.Fatalf("a supplier with a tax number counts for the Vorsteuer: %+v", status)
	}

	// Every other case keeps the finding as it is.
	for name, c := range map[string]struct {
		in  model.ImportInspection
		inv *einvoice.Invoice
	}{
		"no tax number":           {inspection(finding), &einvoice.Invoice{}},
		"blank tax number":        {inspection(finding), &einvoice.Invoice{Seller: einvoice.Party{FCTaxRegistration: " "}}},
		"document not parsed":     {inspection(finding), nil},
		"another rule at its own": {inspection(rule("BR-CO-15", "error")), withTaxNumber},
	} {
		if got := RelaxSellerIdentifier(c.in, c.inv); got.Validation.State != "invalid" || got.Validation.Issues[0].Severity == "warning" {
			t.Errorf("%s: %+v", name, got.Validation)
		}
	}

	// A second finding that violates a rule keeps the state at invalid.
	mixed := RelaxSellerIdentifier(inspection(finding, rule("BR-CO-15", "error")), withTaxNumber)
	if mixed.Validation.State != "invalid" || mixed.Validation.Issues[0].Severity != "warning" || mixed.Validation.Issues[1].Severity != "error" {
		t.Fatalf("got %+v", mixed.Validation)
	}
	if finding.Severity != "error" {
		t.Fatal("the argument was changed")
	}
}
