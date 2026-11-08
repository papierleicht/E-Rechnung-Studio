package validation

import (
	"bytes"
	"context"
	"crypto/sha256"
	"embed"
	"encoding/xml"
	"errors"
	"fmt"
	"io/fs"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/knroy/go-xml/xdm"
	"github.com/knroy/go-xml/xsd"
	"github.com/knroy/go-xml/xslt"
	"papierleicht.de/e-rechnung-studio/internal/einvoice"
	"papierleicht.de/e-rechnung-studio/internal/invoicexml"
	"papierleicht.de/e-rechnung-studio/internal/model"
	"papierleicht.de/e-rechnung-studio/internal/profile"
)

//go:embed all:rules
var bundled embed.FS

type Validator struct {
	root   string
	setsMu sync.Mutex
	runMu  sync.Mutex
	sets   map[string]*ruleSet
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

type ruleSet struct {
	once   sync.Once
	schema *xsd.Schema
	sheets []ruleSheet
	levels map[string]string
	err    error
}

type ruleSheet struct {
	sheet    *xslt.Stylesheet
	resolver *xslt.FileResolver
}

func New(cacheRoot string) (*Validator, error) {
	root := filepath.Join(cacheRoot, "rules")
	if err := materialize(root); err != nil {
		return nil, err
	}
	return &Validator{root: root, sets: map[string]*ruleSet{}}, nil
}

func materialize(root string) error {
	return fs.WalkDir(bundled, "rules", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel("rules", path)
		dest := filepath.Join(root, rel)
		if d.IsDir() {
			return os.MkdirAll(dest, 0o700)
		}
		data, err := fs.ReadFile(bundled, path)
		if err != nil {
			return err
		}
		return os.WriteFile(dest, data, 0o600)
	})
}

func fileURI(path string) string {
	abs, _ := filepath.Abs(path)
	if filepath.VolumeName(abs) != "" {
		abs = "/" + abs
	}
	return (&url.URL{Scheme: "file", Path: filepath.ToSlash(abs)}).String()
}

func (v *Validator) rules(f profile.Format, credit bool) *ruleSet {
	key := f.ID
	if f.Syntax == "ubl" && credit {
		key += "-credit"
	}
	v.setsMu.Lock()
	r := v.sets[key]
	if r == nil {
		r = &ruleSet{}
		v.sets[key] = r
	}
	v.setsMu.Unlock()
	r.once.Do(func() { r.err = r.compile(v.root, f, credit) })
	return r
}

func (r *ruleSet) compile(root string, f profile.Format, credit bool) error {
	var schema string
	var sheets []string
	if !f.XRechnung {
		level := strings.ReplaceAll(f.Conformance, " ", "")
		root = filepath.Join(root, f.ID)
		schema = "Factur-X_1.09.2_" + level + ".xsd"
		sheets = []string{"_XSLT_" + level + "/FACTUR-X_" + level + ".xslt"}
	} else {
		scenario := "EN16931 XRechnung (CII)"
		if f.Syntax == "ubl" {
			scenario = "EN16931 XRechnung (UBL Invoice)"
			if credit {
				scenario = "EN16931 XRechnung (UBL CreditNote)"
			}
		}
		var err error
		if r.levels, err = scenarioLevels(filepath.Join(root, "xrechnung", "scenarios.xml"), scenario); err != nil {
			return err
		}
		root = filepath.Join(root, "xrechnung", "resources")
		if f.Syntax == "cii" {
			schema = "cii/16b/xsd/CrossIndustryInvoice_100pD16B.xsd"
			sheets = []string{"cii/16b/xsl/EN16931-CII-validation.xsl", "xrechnung/3.0.2/xsl/XRechnung-CII-validation.xsl"}
		} else {
			schema = "ubl/2.1/xsd/maindoc/UBL-Invoice-2.1.xsd"
			if credit {
				schema = "ubl/2.1/xsd/maindoc/UBL-CreditNote-2.1.xsd"
			}
			sheets = []string{"ubl/2.1/xsl/EN16931-UBL-validation.xsl", "xrechnung/3.0.2/xsl/XRechnung-UBL-validation.xsl"}
		}
	}
	opts := xsd.Options{Resolver: &xsd.FileResolver{Root: root}}
	// Only the trusted, bundled UBL schemas need their internal DTD. Invoice
	// input still rejects DOCTYPE and external entities are never enabled.
	opts.ParseOptions.AllowDOCTYPE = f.Syntax == "ubl"
	var err error
	r.schema, err = xsd.LoadFile(filepath.Join(root, schema), opts)
	if err != nil {
		return err
	}
	for _, name := range sheets {
		path := filepath.Join(root, name)
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		// go-xml currently rejects this XSLT 2.0 pattern. We only transform
		// complete XML document nodes, where root() and / select the same node.
		// Original bundled files remain unchanged; assertions are not rewritten.
		if f.XRechnung {
			data = bytes.ReplaceAll(data, []byte(`match="root()"`), []byte(`match="/"`))
		}
		resolver, err := xslt.NewFileResolver(filepath.Dir(path))
		if err != nil {
			return err
		}
		tree, err := xdm.ParseString(string(data), xdm.ParseOptions{BaseURI: fileURI(path)})
		if err != nil {
			return err
		}
		sheet, err := xslt.Compile(tree.Root, xslt.CompileOptions{Resolver: resolver, BaseURI: fileURI(path)})
		if err != nil {
			return err
		}
		r.sheets = append(r.sheets, ruleSheet{sheet, resolver})
	}
	return nil
}

type svrlReport struct {
	Failed  []svrlIssue `xml:"failed-assert"`
	Reports []svrlIssue `xml:"successful-report"`
}
type svrlIssue struct {
	ID       string `xml:"id,attr"`
	Flag     string `xml:"flag,attr"`
	Location string `xml:"location,attr"`
	Text     string `xml:"text"`
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
	// Compiled schema and stylesheet objects are cached and shared by every
	// bound service call. Their third-party implementations do not promise safe
	// concurrent execution, so serialize use while retaining the cache.
	v.runMu.Lock()
	defer v.runMu.Unlock()
	var invoiceContext *einvoice.Invoice
	defer func() {
		translated = German(translated)
		if invoiceContext != nil {
			translated = explainIntraCommunityIdentifiers(translated, invoiceContext)
		}
	}()
	result := model.ValidationResult{State: "valid", Issues: []model.ValidationIssue{}}
	if strings.Contains(strings.ToUpper(string(data)), "<!DOCTYPE") {
		return failed("syntax", "DOCTYPE", errors.New("DOCTYPE declarations are not allowed in invoice XML"))
	}
	tree, err := xdm.Parse(bytes.NewReader(data), xdm.ParseOptions{TrackPositions: true})
	if err != nil {
		return failed("syntax", "XML", err)
	}
	inv, parseErr := invoicexml.Parse(data)
	if parseErr != nil {
		return failed("syntax", "XML", parseErr)
	}
	invoiceContext = inv
	f, err := profile.Detect(inv)
	if err != nil {
		return model.ValidationResult{State: "unsupported", Issues: []model.ValidationIssue{{Stage: "profile", RuleID: "UNSUPPORTED", Severity: "error", Message: err.Error()}}}
	}
	r := v.rules(f, inv.InvoiceTypeCode == 381)
	if r.err != nil {
		return failed("engine", "VALIDATOR-INIT", r.err)
	}
	if err = r.schema.Validate(tree.Root, xsd.ValidateOptions{}); err != nil {
		result.State = "invalid"
		result.Issues = append(result.Issues, model.ValidationIssue{Stage: "xsd", RuleID: "XSD", Severity: "error", Message: err.Error()})
		return result
	}
	if issues := SupplementalIssues(inv); len(issues) > 0 {
		result.Issues = append(result.Issues, issues...)
		result.State = "invalid"
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	for _, s := range r.sheets {
		out, err := s.sheet.Transform(ctx, tree.Root, xslt.TransformOptions{Documents: s.resolver, Texts: s.resolver})
		if err != nil {
			return failed("rules", "XSLT", err)
		}
		var buf bytes.Buffer
		if err = out.Serialize(&buf); err != nil {
			return failed("rules", "SVRL-SERIALIZE", err)
		}
		var report svrlReport
		if err = xml.Unmarshal(buf.Bytes(), &report); err != nil {
			return failed("rules", "SVRL-PARSE", err)
		}
		for _, i := range append(report.Failed, report.Reports...) {
			severity := i.Flag
			if level := r.levels[i.ID]; level != "" {
				severity = level
			}
			if severity == "" {
				severity = "error"
			}
			result.Issues = append(result.Issues, model.ValidationIssue{Stage: "rules", RuleID: i.ID, Severity: severity, Message: i.Text, Location: i.Location})
			if severity == "fatal" || severity == "error" {
				result.State = "invalid"
			} else if result.State == "valid" {
				result.State = "warning"
			}
		}
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

// scenarioLevels reads the customLevel severity overrides of one standard KoSIT
// scenario (extension and CVD scenarios are separate entries and not selected).
func scenarioLevels(path, name string) (map[string]string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var doc struct {
		Scenarios []struct {
			Name   string `xml:"name"`
			Levels []struct {
				Level string `xml:"level,attr"`
				ID    string `xml:",chardata"`
			} `xml:"createReport>customLevel"`
		} `xml:"scenario"`
	}
	if err = xml.Unmarshal(data, &doc); err != nil {
		return nil, err
	}
	for _, sc := range doc.Scenarios {
		if sc.Name != name {
			continue
		}
		levels := make(map[string]string, len(sc.Levels))
		for _, l := range sc.Levels {
			levels[strings.TrimSpace(l.ID)] = l.Level
		}
		return levels, nil
	}
	return nil, fmt.Errorf("KoSIT scenario %q not found in %s", name, path)
}

func failed(stage, code string, err error) model.ValidationResult {
	state := "error"
	if errors.Is(err, context.DeadlineExceeded) {
		code = "TIMEOUT"
	}
	return model.ValidationResult{State: state, Issues: []model.ValidationIssue{{Stage: stage, RuleID: code, Severity: "error", Message: fmt.Sprint(err)}}}
}
