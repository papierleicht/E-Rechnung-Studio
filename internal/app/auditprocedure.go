package app

import (
	"bytes"
	_ "embed"
	"errors"
	"fmt"
	"html/template"
	"os"
	"strings"
	"time"

	"papierleicht.de/e-rechnung-studio/internal/gdpdu"
	"papierleicht.de/e-rechnung-studio/internal/pdfinvoice"
	"papierleicht.de/e-rechnung-studio/internal/store"
)

// The app writes the Verfahrensdokumentation from what it knows about itself
// and a few answers of the business. It describes data and procedures, never
// hosts or the technology behind the service. The business reads it, adapts it
// where its process differs and keeps it (GoBD Rz. 151 ff.).

//go:embed auditprocedure.html
var procedureHTML string

var procedureTemplate = template.Must(template.New("procedure").Parse(procedureHTML))

type procedureProgram struct{ Version, Since string }

type procedureTable struct {
	Name, Description string
	Columns           string
}

type procedureHistory struct {
	Version int
	At      string
	Program string
}

type procedureData struct {
	Company    string
	Address    string
	TaxID      string
	Version    int
	Date       string
	Program    string
	Profile    store.ProcedureProfile
	PaperLabel string
	Scanning   bool
	Destroying bool
	Programs   []procedureProgram
	Tables     []procedureTable
	History    []procedureHistory
	Backup     bool
}

var paperLabels = map[string]string{
	"none":         "Es fallen keine Papierbelege an.",
	"keep":         "Papierbelege werden geordnet im Original aufbewahrt und in der App erfasst.",
	"scan-keep":    "Papierbelege werden gescannt oder fotografiert und zusätzlich im Original aufbewahrt.",
	"scan-destroy": "Papierbelege werden gescannt oder fotografiert und nach der Prüfung des Bildes vernichtet, soweit sie nicht im Original aufzubewahren sind.",
}

// ProcedureProfile returns the answers of the business.
func (s *AuditService) ProcedureProfile() (store.ProcedureProfile, error) {
	return s.invoices.store.ProcedureProfile()
}

// SaveProcedureProfile keeps the answers.
func (s *AuditService) SaveProcedureProfile(p store.ProcedureProfile) error {
	if _, ok := paperLabels[p.PaperReceipts]; !ok && p.PaperReceipts != "" {
		return errors.New("Wähle aus, wie du mit Papierbelegen umgehst.")
	}
	for _, v := range []string{p.Responsible, p.Advisor, p.OtherSystems, p.Devices, p.BackupRoutine} {
		if len(v) > 2000 {
			return errors.New("Fasse deine Angaben kürzer.")
		}
	}
	return s.invoices.store.SaveProcedureProfile(p)
}

func germanStamp(stamp string) string {
	t, err := time.Parse(time.RFC3339, stamp)
	if err != nil {
		return stamp
	}
	return t.Local().Format("02.01.2006")
}

func (s *AuditService) procedureBody(version int) (string, error) {
	st := s.invoices.store
	company, err := st.Company()
	if err != nil {
		return "", err
	}
	profile, err := st.ProcedureProfile()
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(profile.Responsible) == "" || profile.PaperReceipts == "" {
		return "", errors.New("Trage zuerst ein, wer die Belege erfasst und wie du mit Papierbelegen umgehst.")
	}
	a := company.Address
	data := procedureData{Company: company.Name, Address: strings.TrimSpace(strings.Join([]string{a.Street, strings.TrimSpace(a.Postcode + " " + a.City)}, ", ")), TaxID: firstNonEmpty(company.VATID, company.TaxNumber),
		Version: version, Date: s.now().Format("02.01.2006"), Program: currentVersion, Profile: profile, PaperLabel: paperLabels[profile.PaperReceipts],
		Scanning: strings.HasPrefix(profile.PaperReceipts, "scan"), Destroying: profile.PaperReceipts == "scan-destroy"}
	programs, err := st.ProgramVersions()
	if err != nil {
		return "", err
	}
	for _, e := range programs {
		data.Programs = append(data.Programs, procedureProgram{e.ObjectID, germanStamp(e.At)})
	}
	if len(data.Programs) == 0 {
		data.Programs = []procedureProgram{{currentVersion, data.Date}}
	}
	for _, t := range (auditRecords{}).tables(fileNames{}) {
		names := make([]string, 0, len(t.Columns))
		for _, c := range t.Columns {
			names = append(names, c.Name)
		}
		data.Tables = append(data.Tables, procedureTable{strings.ReplaceAll(t.Name, "_", " "), t.Description, strings.Join(names, ", ")})
	}
	history, err := st.ProcedureVersions()
	if err != nil {
		return "", err
	}
	for i := len(history) - 1; i >= 0; i-- {
		v := history[i]
		data.History = append(data.History, procedureHistory{v.Version, germanStamp(v.At), v.AppVersion})
	}
	var out bytes.Buffer
	if err = procedureTemplate.Execute(&out, data); err != nil {
		return "", err
	}
	return out.String(), nil
}

// CreateProcedure writes a new version. Old versions stay stored, so the
// history of the description remains traceable (GoBD Rz. 154).
func (s *AuditService) CreateProcedure() (store.ProcedureVersion, error) {
	versions, err := s.invoices.store.ProcedureVersions()
	if err != nil {
		return store.ProcedureVersion{}, err
	}
	body, err := s.procedureBody(len(versions) + 1)
	if err != nil {
		return store.ProcedureVersion{}, err
	}
	v, err := s.invoices.store.AddProcedureVersion(body)
	v.HTML = ""
	return v, err
}

func renderProcedure(v store.ProcedureVersion, company string) ([]byte, error) {
	return pdfinvoice.RenderPlain(fmt.Sprintf("Verfahrensdokumentation Version %d", v.Version), company, v.HTML)
}

//go:embed auditreport.html
var auditReportHTML string

var auditReportTemplate = template.Must(template.New("report").Parse(auditReportHTML))

// receiptReport explains the receipts of a period in plain words, so nobody
// has to verify a signature to understand them. It runs a fresh check first.
// Without receipts for the period there is nothing to report.
func (s *AuditService) receiptReport(from, to string) ([]byte, error) {
	records, err := s.invoices.auditRecords(from, to)
	if err != nil {
		return nil, err
	}
	anchors, err := s.invoices.store.AuditAnchors()
	if err != nil {
		return nil, err
	}
	rows := records.anchorTable(anchors).Rows
	if len(rows) == 0 {
		return nil, nil
	}
	company, err := s.invoices.store.Company()
	if err != nil {
		return nil, err
	}
	health := s.check()
	data := struct {
		Company, From, To, Created, Program, First, Last, Compared, Key string
		LogOK                                                           bool
		Problems, Count, Matched, Conflicts                             int
	}{Company: company.Name, From: gdpdu.Day(from), To: gdpdu.Day(to), Created: s.now().Format("02.01.2006 15:04"), Program: currentVersion,
		First: germanStamp(rows[0][2]), Last: germanStamp(rows[len(rows)-1][2]), Key: licensePublicKey,
		LogOK: health.OK, Problems: health.Problems, Count: len(rows), Matched: health.Matched, Conflicts: health.Conflicts}
	if health.ComparedAt != "" {
		data.Compared = germanStamp(health.ComparedAt)
	}
	var out bytes.Buffer
	if err := auditReportTemplate.Execute(&out, data); err != nil {
		return nil, err
	}
	return pdfinvoice.RenderPlain("Nachweisbericht", company.Name, out.String())
}

// SaveProcedure writes a stored version as a PDF file.
func (s *AuditService) SaveProcedure(path string, version int) error {
	v, err := s.invoices.store.ProcedureVersionText(version)
	if err != nil {
		return err
	}
	if v.HTML == "" {
		return errors.New("Diese Version gibt es nicht.")
	}
	company, err := s.invoices.store.Company()
	if err != nil {
		return err
	}
	data, err := renderProcedure(v, company.Name)
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o600)
}
