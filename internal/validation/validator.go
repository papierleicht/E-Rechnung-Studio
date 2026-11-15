package validation

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"path/filepath"
	"sync"

	erechnung "github.com/papierleicht/go-e-rechnung-validator"
	"papierleicht.de/e-rechnung-studio/internal/einvoice"
	"papierleicht.de/e-rechnung-studio/internal/invoicexml"
	"papierleicht.de/e-rechnung-studio/internal/model"
	"papierleicht.de/e-rechnung-studio/internal/profile"
)

// Validator runs the official XSD and Schematron rules through
// go-e-rechnung-validator and adds what belongs to the app: its own input
// safeguards (stage "input"), German texts and a cache of recent results.
type Validator struct {
	rules *erechnung.Validator
	// recent holds the last results by content. Issuing checks the same final
	// XML before storing it, before rendering the PDF and when reopening it.
	recentMu sync.Mutex
	recent   []recentResult
}

// recentResults is enough for the drafts and documents of one workflow.
const recentResults = 16

type recentResult struct {
	sum    [sha256.Size]byte
	result model.ValidationResult
}

func New(cacheRoot string) (*Validator, error) {
	rules, err := erechnung.New(erechnung.Options{CacheDir: filepath.Join(cacheRoot, "rules")})
	if err != nil {
		return nil, err
	}
	return &Validator{rules: rules}, nil
}

// Validate checks an invoice against its profile. The rules depend only on the
// document, so a result is reused for the same bytes; engine errors such as a
// timeout are not, and run again.
func (v *Validator) Validate(data []byte) model.ValidationResult {
	sum := sha256.Sum256(data)
	v.recentMu.Lock()
	for _, r := range v.recent {
		if r.sum == sum {
			v.recentMu.Unlock()
			return cloneResult(r.result)
		}
	}
	v.recentMu.Unlock()
	result := v.validate(data)
	if result.State != "error" {
		v.recentMu.Lock()
		if len(v.recent) == recentResults {
			v.recent = v.recent[1:]
		}
		v.recent = append(v.recent, recentResult{sum, cloneResult(result)})
		v.recentMu.Unlock()
	}
	return result
}

// cloneResult keeps callers that adjust issues in place, like
// softenLocalIssues, from changing a remembered result.
func cloneResult(r model.ValidationResult) model.ValidationResult {
	if r.Issues != nil {
		r.Issues = append(make([]model.ValidationIssue, 0, len(r.Issues)), r.Issues...)
	}
	return r
}

func (v *Validator) validate(data []byte) (translated model.ValidationResult) {
	var invoiceContext *einvoice.Invoice
	defer func() {
		translated = German(translated)
		if invoiceContext != nil {
			translated = explainIntraCommunityIdentifiers(translated, invoiceContext)
		}
	}()
	official := v.rules.Validate(data)
	// The app reports unreadable input (DOCTYPE, malformed XML) as a failed
	// check, not as an invalid invoice.
	if official.State == erechnung.Invalid && len(official.Issues) == 1 && official.Issues[0].Stage == erechnung.StageSyntax {
		i := official.Issues[0]
		return failed(i.Stage, i.RuleID, errors.New(i.Message))
	}
	// The app's own reading of the document gives the context for its input
	// safeguards and explanations; a document it cannot read is not checked.
	inv, parseErr := invoicexml.Parse(data)
	if parseErr != nil {
		return failed("syntax", "XML", parseErr)
	}
	invoiceContext = inv
	if _, err := profile.Detect(inv); err != nil && official.State != erechnung.Error {
		return model.ValidationResult{State: "unsupported", Issues: []model.ValidationIssue{{Stage: "profile", RuleID: "UNSUPPORTED", Severity: "error", Message: err.Error()}}}
	}
	result := model.ValidationResult{State: string(official.State), Issues: []model.ValidationIssue{}}
	switch official.State {
	case erechnung.Error, erechnung.Unsupported:
		return withIssues(result, official.Issues)
	}
	if len(official.Issues) > 0 && official.Issues[0].Stage == erechnung.StageXSD {
		return withIssues(result, official.Issues)
	}
	// Input safeguards come after the schema and before the business rules,
	// and block issuing like an error of the official rules.
	if issues := SupplementalIssues(inv); len(issues) > 0 {
		result.Issues = append(result.Issues, issues...)
		result.State = "invalid"
	}
	return withIssues(result, official.Issues)
}

func withIssues(result model.ValidationResult, issues []erechnung.Issue) model.ValidationResult {
	for _, i := range issues {
		result.Issues = append(result.Issues, model.ValidationIssue{Stage: i.Stage, RuleID: i.RuleID, Severity: i.Severity, Message: i.Message, Location: i.Location})
	}
	return result
}

// ValidateImported validates a document received from a third party. The official
// rules stay binding; the app's own input safeguards (stage "input") only warn,
// because they are checks for what this app issues, not conformance requirements.
func (v *Validator) ValidateImported(data []byte) model.ValidationResult {
	return softenLocalIssues(v.Validate(data))
}

func softenLocalIssues(result model.ValidationResult) model.ValidationResult {
	blocking := false
	for n, issue := range result.Issues {
		if issue.Stage == "input" && (issue.Severity == "error" || issue.Severity == "fatal") {
			result.Issues[n].Severity = "warning"
		}
		blocking = blocking || result.Issues[n].Severity == "error" || result.Issues[n].Severity == "fatal"
	}
	if result.State == "invalid" && !blocking {
		result.State = "warning"
	}
	return result
}

func failed(stage, code string, err error) model.ValidationResult {
	return model.ValidationResult{State: "error", Issues: []model.ValidationIssue{{Stage: stage, RuleID: code, Severity: "error", Message: fmt.Sprint(err)}}}
}
