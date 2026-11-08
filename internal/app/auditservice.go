package app

import (
	"context"
	"errors"
	"fmt"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"
	"papierleicht.de/e-rechnung-studio/internal/account"
	"papierleicht.de/e-rechnung-studio/internal/entitlement"
	"papierleicht.de/e-rechnung-studio/internal/gdpdu"
	"papierleicht.de/e-rechnung-studio/internal/store"
)

// AuditService bundles what keeps the books provable and what a tax audit asks
// for. The change log, closing periods and the check work for everyone, because
// the rules apply to everyone who keeps records (GoBD Rz. 28). The tools for the
// audit itself belong to the paid plans; a business that is audited can book a
// single month.

const (
	// anchorInterval is how often a new end of the log is sent to the server.
	anchorInterval = 24 * time.Hour
	anchorCheck    = 30 * time.Minute
	anchorTimeout  = 30 * time.Second
	// healthInterval is how often Plus checks the log without being asked.
	healthInterval = 24 * time.Hour
	// staleAfter is how long the log may move on without a new receipt.
	staleAfter = 7 * 24 * time.Hour
)

type auditRemote interface {
	HasFeature(string) bool
	AnchorAudit(context.Context, int64, string) (account.AuditReceipt, error)
	CheckAuditReceipt(string) (entitlement.Anchor, error)
	AuditReceipts(context.Context) ([]account.StoredReceipt, error)
	DeviceID() string
}

type AuditService struct {
	invoices *InvoiceService
	remote   auditRemote
	now      func() time.Time
	stop     chan struct{}
	done     chan struct{}
	onHealth func(AuditHealth)

	mu     sync.Mutex
	health AuditHealth
}

// AuditHealth is the last automatic check of the log and of the receipts
// against the server's list. ComparedAt is empty until a comparison succeeded.
type AuditHealth struct {
	CheckedAt  string `json:"checkedAt"`
	OK         bool   `json:"ok"`
	Problems   int    `json:"problems"`
	ComparedAt string `json:"comparedAt"`
	Matched    int    `json:"matched"`
	Restored   int    `json:"restored"`
	Conflicts  int    `json:"conflicts"`
	// Stale: the log moved on, but no receipt came for a week.
	Stale bool `json:"stale"`
}

func NewAuditService(invoices *InvoiceService, remote auditRemote) *AuditService {
	return &AuditService{invoices: invoices, remote: remote, now: time.Now, stop: make(chan struct{})}
}

// ServiceStartup starts the receipts for the change log.
//
//wails:ignore
func (s *AuditService) ServiceStartup(context.Context, application.ServiceOptions) error {
	s.done = make(chan struct{})
	s.onHealth = func(h AuditHealth) {
		if app := application.Get(); app != nil {
			app.Event.Emit("audit:health", h)
		}
	}
	go s.anchorLoop()
	return nil
}

// ServiceShutdown stops the loop.
//
//wails:ignore
func (s *AuditService) ServiceShutdown() error {
	close(s.stop)
	if s.done != nil {
		<-s.done
	}
	return nil
}

func (s *AuditService) anchorLoop() {
	defer close(s.done)
	ticker := time.NewTicker(anchorCheck)
	defer ticker.Stop()
	s.anchor()
	s.watch()
	for {
		select {
		case <-s.stop:
			return
		case <-ticker.C:
			s.anchor()
			s.watch()
		}
	}
}

// watch runs the daily check of Plus, so a damaged log does not wait until
// somebody opens the settings.
func (s *AuditService) watch() {
	if !s.remote.HasFeature(entitlement.FeatureAudit) {
		return
	}
	s.mu.Lock()
	last, _ := time.Parse(time.RFC3339, s.health.CheckedAt)
	s.mu.Unlock()
	if s.now().Sub(last) >= healthInterval {
		s.check()
	}
}

// check verifies the log and compares the receipts with the server. Offline
// the last comparison stays; the next run catches up.
func (s *AuditService) check() AuditHealth {
	st := s.invoices.store
	s.mu.Lock()
	health := s.health
	s.mu.Unlock()
	health.CheckedAt = s.now().UTC().Format(time.RFC3339)
	if _, _, open := st.AuditView(); open {
		return health
	}
	report, err := s.Verify()
	health.OK, health.Problems = err == nil && report.OK, len(report.Problems)
	health.Stale = s.stale()
	ctx, cancel := context.WithTimeout(context.Background(), anchorTimeout)
	defer cancel()
	if list, err := s.remote.AuditReceipts(ctx); err == nil {
		if health, err = s.compareReceipts(health, list); err != nil {
			health.ComparedAt = ""
		}
	}
	s.mu.Lock()
	s.health = health
	s.mu.Unlock()
	if s.onHealth != nil {
		s.onHealth(health)
	}
	return health
}

// stale tells that changes are waiting for a receipt that does not come.
func (s *AuditService) stale() bool {
	st := s.invoices.store
	anchors, err := st.AuditAnchors()
	if err != nil || len(anchors) == 0 || !s.remote.HasFeature(entitlement.FeatureAudit) {
		return false
	}
	last := anchors[len(anchors)-1]
	seq, _, err := st.AuditHead()
	at, atErr := time.Parse(time.RFC3339, last.At)
	return err == nil && atErr == nil && seq > last.Seq && s.now().Sub(at) >= staleAfter
}

// compareReceipts holds the server's list against the log. Entries that match
// are fine and bring back receipts lost here. A device's own entry that matches
// nothing means the log changed after it was confirmed; other devices keep logs
// of their own, unless their data was restored here.
func (s *AuditService) compareReceipts(h AuditHealth, list []account.StoredReceipt) (AuditHealth, error) {
	st := s.invoices.store
	entries, err := st.AuditEntries(store.AuditFilter{})
	if err != nil {
		return h, err
	}
	log := make(map[int64]string, len(entries))
	for _, e := range entries {
		log[e.Seq] = e.Hash
	}
	anchors, err := st.AuditAnchors()
	if err != nil {
		return h, err
	}
	known := make(map[int64]bool, len(anchors))
	for _, a := range anchors {
		known[a.Seq] = true
	}
	h.Matched, h.Restored, h.Conflicts = 0, 0, 0
	device := s.remote.DeviceID()
	for _, r := range list {
		signed, err := s.remote.CheckAuditReceipt(r.Receipt)
		if err != nil || signed.Seq != r.Seq || signed.Hash != r.Hash {
			h.Conflicts++
			continue
		}
		switch {
		case log[r.Seq] == r.Hash:
			h.Matched++
			if !known[r.Seq] {
				if err := st.AddAuditAnchor(store.AuditAnchor{Seq: r.Seq, Hash: r.Hash, At: r.At.UTC().Format(time.RFC3339), Receipt: r.Receipt}); err != nil {
					return h, err
				}
				known[r.Seq] = true
				h.Restored++
			}
		case r.DeviceID == device:
			h.Conflicts++
		}
	}
	h.ComparedAt = s.now().UTC().Format(time.RFC3339)
	return h, nil
}

// anchor sends the end of the log to the server, once a day and only if it
// moved. Being offline is no problem: the next run catches up.
func (s *AuditService) anchor() {
	if !s.remote.HasFeature(entitlement.FeatureAudit) {
		return
	}
	st := s.invoices.store
	if _, _, open := st.AuditView(); open {
		return
	}
	seq, hash, err := st.AuditHead()
	if err != nil || seq == 0 {
		return
	}
	anchors, err := st.AuditAnchors()
	if err != nil {
		return
	}
	if len(anchors) > 0 {
		last := anchors[len(anchors)-1]
		at, err := time.Parse(time.RFC3339, last.At)
		if last.Seq == seq || (err == nil && s.now().Sub(at) < anchorInterval) {
			return
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), anchorTimeout)
	defer cancel()
	receipt, err := s.remote.AnchorAudit(ctx, seq, hash)
	if err != nil {
		return
	}
	_ = st.AddAuditAnchor(store.AuditAnchor{Seq: seq, Hash: hash, At: receipt.At.UTC().Format(time.RFC3339), Receipt: receipt.Receipt})
}

// AuditViewState tells whether the read-only view is open.
type AuditViewState struct {
	Open bool   `json:"open"`
	From string `json:"from"`
	To   string `json:"to"`
}

// AuditStatus is the head of the settings page.
type AuditStatus struct {
	Entries     int64                    `json:"entries"`
	Plan        bool                     `json:"plan"`
	Anchors     int                      `json:"anchors"`
	FirstAnchor string                   `json:"firstAnchor"`
	LastAnchor  string                   `json:"lastAnchor"`
	Health      AuditHealth              `json:"health"`
	View        AuditViewState           `json:"view"`
	Locks       []store.PeriodLock       `json:"locks"`
	Procedure   []store.ProcedureVersion `json:"procedure"`
}

func (s *AuditService) Status() (AuditStatus, error) {
	st := s.invoices.store
	status := AuditStatus{Plan: s.remote.HasFeature(entitlement.FeatureAudit)}
	s.mu.Lock()
	status.Health = s.health
	s.mu.Unlock()
	var err error
	if status.Entries, _, err = st.AuditHead(); err != nil {
		return status, err
	}
	anchors, err := st.AuditAnchors()
	if err != nil {
		return status, err
	}
	status.Anchors = len(anchors)
	if len(anchors) > 0 {
		status.FirstAnchor = anchors[0].At
		status.LastAnchor = anchors[len(anchors)-1].At
	}
	status.View.From, status.View.To, status.View.Open = st.AuditView()
	if status.Locks, err = st.PeriodLocks(); err != nil {
		return status, err
	}
	status.Procedure, err = st.ProcedureVersions()
	return status, err
}

// Verify checks the log, all documents and the receipts.
func (s *AuditService) Verify() (store.AuditReport, error) {
	return s.invoices.store.VerifyAudit(func(a store.AuditAnchor) error {
		got, err := s.remote.CheckAuditReceipt(a.Receipt)
		if err != nil {
			return err
		}
		if got.Seq != a.Seq || got.Hash != a.Hash {
			return errors.New("receipt does not match")
		}
		return nil
	})
}

// Entries lists the log. In the read-only view it stays within the period.
func (s *AuditService) Entries(f store.AuditFilter) ([]store.AuditEntry, error) {
	if from, to, open := s.invoices.store.AuditView(); open {
		f.From = max(f.From, from)
		if f.To == "" || f.To > to {
			f.To = to
		}
	}
	return s.invoices.store.AuditEntries(f)
}

// LockPeriod closes a period for good.
func (s *AuditService) LockPeriod(from, to string) (store.PeriodLock, error) {
	return s.invoices.store.LockPeriod(from, to)
}

// AuditFinding is a result of the completeness check.
type AuditFinding struct {
	// Level is "warning" or "info".
	Level  string   `json:"level"`
	Title  string   `json:"title"`
	Detail string   `json:"detail"`
	Items  []string `json:"items"`
}

var trailingNumber = regexp.MustCompile(`^(.*?)(\d+)$`)

// numberGaps finds numbers that are missing between the first and the last of
// each series, for example RE-2026-0003 between 0002 and 0004.
func numberGaps(used, explained map[string]bool) []string {
	type series struct {
		width int
		seen  map[int]bool
	}
	groups := map[string]*series{}
	for number := range used {
		m := trailingNumber.FindStringSubmatch(number)
		if m == nil {
			continue
		}
		n, err := strconv.Atoi(m[2])
		if err != nil {
			continue
		}
		g := groups[m[1]]
		if g == nil {
			g = &series{width: len(m[2]), seen: map[int]bool{}}
			groups[m[1]] = g
		}
		g.seen[n] = true
	}
	missing := []string{}
	for prefix, g := range groups {
		low, high := 1<<62, 0
		for n := range g.seen {
			low, high = min(low, n), max(high, n)
		}
		for n := low + 1; n < high && len(missing) < 200; n++ {
			number := fmt.Sprintf("%s%0*d", prefix, g.width, n)
			if !g.seen[n] && !explained[number] {
				missing = append(missing, number)
			}
		}
	}
	sort.Strings(missing)
	return missing
}

// Findings looks for what an auditor would ask about (GoBD Rz. 40).
func (s *AuditService) Findings(from, to string) ([]AuditFinding, error) {
	r, err := s.invoices.auditRecords(from, to)
	if err != nil {
		return nil, err
	}
	findings := []AuditFinding{}
	add := func(level, title, detail string, items []string) {
		if len(items) > 0 {
			findings = append(findings, AuditFinding{level, title, detail, items})
		}
	}

	invoices, explained := map[string]bool{}, map[string]bool{}
	for _, rec := range r.Issued {
		invoices[rec.Draft.Number] = true
	}
	deleted, err := s.invoices.store.DeletedInvoices()
	if err != nil {
		return nil, err
	}
	for _, d := range deleted {
		explained[d.Number] = true
		if inRange(d.IssueDate, from, to) {
			invoices[d.Number] = true
		}
	}
	add("warning", "Lücken bei den Rechnungsnummern", "Diese Nummern fehlen und stehen auch nicht im Protokoll gelöschter Rechnungen. Lücken sind erlaubt, du solltest sie aber erklären können.", numberGaps(invoices, explained))

	receipts := map[string]bool{}
	for _, e := range r.Expenses {
		receipts[e.Number] = true
	}
	for _, v := range r.Received {
		receipts[v.ReceiptNumber] = true
	}
	log, err := s.invoices.store.AuditEntries(store.AuditFilter{Object: store.AuditReceived})
	if err != nil {
		return nil, err
	}
	for _, e := range log {
		if e.Action == store.AuditDeleted {
			if m := regexp.MustCompile(`B-\d{4}-\d+`).FindString(e.Before); m != "" {
				explained[m] = true
			}
		}
	}
	add("warning", "Lücken bei den Belegnummern", "Diese Belegnummern fehlen im Zeitraum. Die App vergibt sie fortlaufend, ein Beleg kann also fehlen oder außerhalb des Zeitraums liegen.", numberGaps(receipts, explained))

	withoutFile, duplicates, late := []string{}, []string{}, []string{}
	seen := map[string]string{}
	for _, e := range r.Expenses {
		if e.CancelledAt != "" {
			continue
		}
		label := e.Number + " " + e.Supplier
		if e.FileSum == "" && e.ReceivedID == "" {
			withoutFile = append(withoutFile, label)
		}
		key := e.Supplier + "\x00" + e.Date + "\x00" + e.Gross
		if first, ok := seen[key]; ok {
			duplicates = append(duplicates, first+" und "+e.Number)
		}
		seen[key] = e.Number
		date, err1 := time.Parse(time.DateOnly, e.Date)
		created, err2 := time.Parse(time.RFC3339, e.CreatedAt)
		if err1 == nil && err2 == nil && created.Sub(date) > 11*24*time.Hour && e.PaidWith != "cash" {
			late = append(late, label)
		}
	}
	add("warning", "Ausgaben ohne Belegdatei", "Jede Buchung braucht einen Beleg. Hänge die Datei an oder bewahre das Papier auf und halte fest, wo es liegt.", withoutFile)
	add("warning", "Mögliche Doppelerfassungen", "Gleicher Lieferant, gleiches Datum, gleicher Betrag.", duplicates)
	add("info", "Spät erfasste Belege", "Mehr als zehn Tage zwischen Belegdatum und Erfassung. Das ist kein Fehler, aber zeitnahes Erfassen ist die Regel.", late)

	cash := []string{}
	for _, e := range r.Expenses {
		if e.CancelledAt == "" && e.PaidWith == "cash" {
			cash = append(cash, e.Number+" "+e.Supplier)
		}
	}
	add("info", "Barzahlungen", "Die App führt kein Kassenbuch. Wenn du eine Kasse führst, gehören diese Belege auch dorthin.", cash)
	return findings, nil
}

// AuditTableInfo names a table of the data view.
type AuditTableInfo struct {
	Name  string `json:"name"`
	Title string `json:"title"`
	Rows  int    `json:"rows"`
}

// Tables lists the tables of a period.
func (s *AuditService) Tables(from, to string) ([]AuditTableInfo, error) {
	r, err := s.invoices.auditRecords(from, to)
	if err != nil {
		return nil, err
	}
	out := []AuditTableInfo{}
	for _, t := range r.tables(fileNames{}) {
		v := viewOf(t)
		out = append(out, AuditTableInfo{v.Name, v.Title, len(v.Rows)})
	}
	return out, nil
}

func (s *AuditService) table(name, from, to string) (gdpdu.Table, error) {
	r, err := s.invoices.auditRecords(from, to)
	if err != nil {
		return gdpdu.Table{}, err
	}
	for _, t := range r.tables(fileNames{}) {
		if strings.TrimSuffix(t.File, ".csv") == name {
			t.Rows = t.Filled()
			return t, nil
		}
	}
	return gdpdu.Table{}, errors.New("Diese Tabelle gibt es nicht.")
}

// Table returns all rows of a table for filtering and sorting in the app.
func (s *AuditService) Table(name, from, to string) (auditView, error) {
	t, err := s.table(name, from, to)
	if err != nil {
		return auditView{}, err
	}
	return viewOf(t), nil
}

// SaveTable writes a filtered table as a CSV file, the result of an evaluation
// the auditor asked for (Z2, GoBD Rz. 166).
func (s *AuditService) SaveTable(path, name, from, to, query string) (int, error) {
	t, err := s.table(name, from, to)
	if err != nil {
		return 0, err
	}
	if query = strings.ToLower(strings.TrimSpace(query)); query != "" {
		kept := [][]string{}
		for _, row := range t.Rows {
			if strings.Contains(strings.ToLower(strings.Join(row, " ")), query) {
				kept = append(kept, row)
			}
		}
		t.Rows = kept
	}
	return len(t.Rows), os.WriteFile(path, t.CSV(), 0o600)
}

// Export writes the package for the tax office. The newest
// Verfahrensdokumentation goes along when there is one.
func (s *AuditService) Export(path, from, to string) (AuditExportResult, error) {
	var procedure []byte
	if version, err := s.invoices.store.ProcedureVersionText(0); err == nil {
		company, err := s.invoices.store.Company()
		if err != nil {
			return AuditExportResult{}, err
		}
		if procedure, err = renderProcedure(version, company.Name); err != nil {
			return AuditExportResult{}, err
		}
	}
	report, err := s.receiptReport(from, to)
	if err != nil {
		return AuditExportResult{}, err
	}
	return s.invoices.writeAuditPackage(path, from, to, procedure, report)
}

// OpenView switches to the read-only view of a period for the auditor (Z1).
func (s *AuditService) OpenView(from, to string) (AuditViewState, error) {
	if err := s.invoices.store.OpenAuditView(from, to); err != nil {
		return AuditViewState{}, err
	}
	if s.invoices.mail != nil {
		if err := s.invoices.mail.SetReadOnly(true); err != nil {
			_ = s.invoices.store.CloseAuditView()
			return AuditViewState{}, err
		}
	}
	return AuditViewState{true, from, to}, nil
}

// CloseView ends the read-only view.
func (s *AuditService) CloseView() error {
	if s.invoices.mail != nil {
		if err := s.invoices.mail.SetReadOnly(false); err != nil {
			return err
		}
	}
	return s.invoices.store.CloseAuditView()
}

// ReadOnly tells background work to stay away from the database.
//
//wails:ignore
func (s *InvoiceService) ReadOnly() bool {
	_, _, open := s.store.AuditView()
	return open
}
