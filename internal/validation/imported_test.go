package validation

import (
	"papierleicht.de/e-rechnung-studio/internal/model"
	"testing"
)

func TestSoftenLocalIssues(t *testing.T) {
	local := model.ValidationIssue{Stage: "input", RuleID: "APP-BIC", Severity: "error"}
	official := model.ValidationIssue{Stage: "rules", RuleID: "BR-CO-15", Severity: "error"}
	t.Run("only local safeguards fail", func(t *testing.T) {
		got := softenLocalIssues(model.ValidationResult{State: "invalid", Issues: []model.ValidationIssue{local}})
		if got.State != "warning" || got.Issues[0].Severity != "warning" {
			t.Fatalf("got %+v", got)
		}
	})
	t.Run("official rule still blocks", func(t *testing.T) {
		got := softenLocalIssues(model.ValidationResult{State: "invalid", Issues: []model.ValidationIssue{local, official}})
		if got.State != "invalid" || got.Issues[0].Severity != "warning" || got.Issues[1].Severity != "error" {
			t.Fatalf("got %+v", got)
		}
	})
	t.Run("valid stays valid", func(t *testing.T) {
		if got := softenLocalIssues(model.ValidationResult{State: "valid", Issues: []model.ValidationIssue{}}); got.State != "valid" {
			t.Fatalf("got %+v", got)
		}
	})
}
