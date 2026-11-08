package app

import (
	"papierleicht.de/e-rechnung-studio/internal/model"
	"reflect"
	"testing"
)

func TestCompanyStepValidation(t *testing.T) {
	s := &InvoiceService{} // validation must work without a store and never persist.
	c := testCompany()
	c.TaxNumber = "123"
	c.IBAN = "DE00370400440532013000"
	for step, field := range map[string]string{"tax": "company.taxNumber", "payment": "company.iban"} {
		issues, err := s.ValidateCompanyStep(c, step)
		if err != nil || len(issues) != 1 || issues[0].Field != field {
			t.Fatalf("%s: %v %v", step, issues, err)
		}
	}
	if _, err := s.ValidateCompanyStep(c, "unknown"); err == nil {
		t.Fatal("invalid step accepted")
	}
}

func TestStepValidationAcceptsIBANWhitespaceWithoutChangingInput(t *testing.T) {
	s := &InvoiceService{}
	c := testCompany()
	c.IBAN = "de89 3704\u00a00044 0532 0130 00"
	c.AdditionalAccounts = []model.BankAccount{{IBAN: "GB82 WEST 1234 5698 7654 32"}}
	original := c
	original.AdditionalAccounts = append([]model.BankAccount(nil), c.AdditionalAccounts...)
	issues, err := s.ValidateCompanyStep(c, "payment")
	if err != nil || len(issues) != 0 {
		t.Fatalf("spaced IBAN rejected: %v %v", issues, err)
	}
	if !reflect.DeepEqual(c, original) {
		t.Fatal("validation modified displayed input")
	}
	c.AdditionalAccounts[0].IBAN = "DE89 3704 0044 0532 0130 00"
	issues, _ = s.ValidateCompanyStep(c, "payment")
	if len(issues) != 1 || issues[0].RuleID != "APP-IBAN-DUPLICATE" {
		t.Fatalf("duplicate not detected: %v", issues)
	}
}
