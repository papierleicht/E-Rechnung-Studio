package store

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"papierleicht.de/e-rechnung-studio/internal/model"
)

// The change log keeps every change to tax-relevant data traceable (GoBD
// Rz. 58 f., 107 ff.). An entry is written in the same transaction as the change
// and holds the state before and after it. Entries can only be added, and each
// one contains the hash of its predecessor, so editing or removing one breaks
// the chain. Receipts from the server (AuditAnchor) expose even a chain that was
// recalculated from scratch. Customers and products are not logged: documents
// carry their own snapshot, and deleted contacts must really disappear.

const auditTriggers = `
CREATE TRIGGER IF NOT EXISTS audit_log_immutable_update BEFORE UPDATE ON audit_log BEGIN SELECT RAISE(ABORT, 'Das Änderungsprotokoll ist unveränderlich.'); END;
CREATE TRIGGER IF NOT EXISTS audit_log_immutable_delete BEFORE DELETE ON audit_log BEGIN SELECT RAISE(ABORT, 'Das Änderungsprotokoll ist unveränderlich.'); END;`

const auditTables = `
CREATE TABLE IF NOT EXISTS audit_log (seq INTEGER PRIMARY KEY, at TEXT NOT NULL, action TEXT NOT NULL, object TEXT NOT NULL, object_id TEXT NOT NULL, label TEXT NOT NULL, reason TEXT NOT NULL, before_state TEXT NOT NULL, after_state TEXT NOT NULL, sum TEXT NOT NULL, version TEXT NOT NULL, previous TEXT NOT NULL, hash TEXT NOT NULL);
CREATE INDEX IF NOT EXISTS audit_log_object ON audit_log(object,object_id,seq);
CREATE TABLE IF NOT EXISTS audit_anchors (seq INTEGER PRIMARY KEY, hash TEXT NOT NULL, at TEXT NOT NULL, receipt TEXT NOT NULL);` + auditTriggers

// Actions of the change log.
const (
	AuditCreated   = "created"
	AuditChanged   = "changed"
	AuditCancelled = "cancelled"
	AuditDeleted   = "deleted"
	AuditIssued    = "issued"
	AuditSent      = "sent"
	AuditUnsent    = "unsent"
	AuditRecorded  = "recorded"
	AuditRevoked   = "revoked"
	AuditFiled     = "filed"
	AuditUnfiled   = "unfiled"
	AuditLocked    = "locked"
	AuditRestored  = "restored"
	AuditBaseline  = "baseline"
	AuditVersion   = "version"
	AuditExported  = "exported"
)

// Kinds of logged objects.
const (
	AuditExpense   = "expense"
	AuditReceived  = "received"
	AuditCategory  = "category"
	AuditInputTax  = "input-tax"
	AuditInvoice   = "invoice"
	AuditPayment   = "payment"
	AuditReminder  = "reminder"
	AuditFiling    = "filing"
	AuditSetting   = "setting"
	AuditBooking   = "booking"
	AuditLock      = "lock"
	AuditBackup    = "backup"
	AuditExport    = "export"
	AuditProcedure = "procedure"
	AuditQuote     = "quote"
	AuditProgram   = "program"
)

// auditGone in Sum says that the object no longer exists; an empty Sum says
// that the entry did not change the state the log tracks.
const auditGone = "gone"

// AuditEntry is one line of the change log.
type AuditEntry struct {
	Seq      int64  `json:"seq"`
	At       string `json:"at"`
	Action   string `json:"action"`
	Object   string `json:"object"`
	ObjectID string `json:"objectId"`
	Label    string `json:"label"`
	Reason   string `json:"reason,omitempty"`
	Before   string `json:"before,omitempty"`
	After    string `json:"after,omitempty"`
	Sum      string `json:"sum,omitempty"`
	Version  string `json:"version,omitempty"`
	Previous string `json:"previous"`
	Hash     string `json:"hash"`
}

// AuditAnchor is a server receipt that the log had this state at that time.
type AuditAnchor struct {
	Seq     int64  `json:"seq"`
	Hash    string `json:"hash"`
	At      string `json:"at"`
	Receipt string `json:"receipt"`
}

func auditHash(e AuditEntry) string {
	raw, err := json.Marshal([]any{e.Seq, e.At, e.Action, e.Object, e.ObjectID, e.Label, e.Reason, e.Before, e.After, e.Sum, e.Version, e.Previous})
	if err != nil {
		panic(err)
	}
	return sumOf(raw)
}

func insertAudit(q sqlRunner, e AuditEntry) error {
	_, err := q.Exec(`INSERT INTO audit_log(seq,at,action,object,object_id,label,reason,before_state,after_state,sum,version,previous,hash) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		e.Seq, e.At, e.Action, e.Object, e.ObjectID, e.Label, e.Reason, e.Before, e.After, e.Sum, e.Version, e.Previous, e.Hash)
	return err
}

// logChange appends to the chain. Call it inside the transaction of the change.
func (s *Store) logChange(q sqlRunner, e AuditEntry) error {
	var seq int64
	var previous string
	err := q.QueryRow("SELECT seq,hash FROM audit_log ORDER BY seq DESC LIMIT 1").Scan(&seq, &previous)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	e.Seq, e.Previous, e.Version = seq+1, previous, s.version
	if e.At == "" {
		e.At = now()
	}
	e.Hash = auditHash(e)
	return insertAudit(q, e)
}

func stateJSON(v any) string {
	raw, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return string(raw)
}

// SetVersion names the running program and notes every version in the log, so
// the program behind each state stays provable (GoBD Rz. 154).
func (s *Store) SetVersion(version string) error {
	s.version = version
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err = s.logVersion(tx); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) logVersion(q sqlRunner) error {
	var last string
	err := q.QueryRow("SELECT object_id FROM audit_log WHERE object=? ORDER BY seq DESC LIMIT 1", AuditProgram).Scan(&last)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	if s.version == "" || last == s.version {
		return nil
	}
	return s.logChange(q, AuditEntry{Action: AuditVersion, Object: AuditProgram, ObjectID: s.version, Label: "Programmversion " + s.version})
}

// AuditFilter narrows a listing of the log.
type AuditFilter struct {
	From   string `json:"from"`
	To     string `json:"to"`
	Object string `json:"object"`
	Query  string `json:"query"`
	// Limit bounds the newest entries returned; 0 returns all.
	Limit int `json:"limit"`
}

// AuditEntries lists the log, newest first.
func (s *Store) AuditEntries(f AuditFilter) ([]AuditEntry, error) {
	entries, err := readAudit(s.db)
	if err != nil {
		return nil, err
	}
	out := []AuditEntry{}
	query := strings.ToLower(strings.TrimSpace(f.Query))
	for i := len(entries) - 1; i >= 0; i-- {
		e := entries[i]
		day := e.At[:min(10, len(e.At))]
		if (f.From != "" && day < f.From) || (f.To != "" && day > f.To) || (f.Object != "" && e.Object != f.Object) {
			continue
		}
		if query != "" && !strings.Contains(strings.ToLower(e.Label+" "+e.Reason+" "+e.ObjectID), query) {
			continue
		}
		out = append(out, e)
		if f.Limit > 0 && len(out) == f.Limit {
			break
		}
	}
	return out, nil
}

func readAudit(q sqlRunner) ([]AuditEntry, error) {
	rows, err := q.Query(`SELECT seq,at,action,object,object_id,label,reason,before_state,after_state,sum,version,previous,hash FROM audit_log ORDER BY seq`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []AuditEntry{}
	for rows.Next() {
		var e AuditEntry
		if err = rows.Scan(&e.Seq, &e.At, &e.Action, &e.Object, &e.ObjectID, &e.Label, &e.Reason, &e.Before, &e.After, &e.Sum, &e.Version, &e.Previous, &e.Hash); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// AuditHead returns the newest entry of the log.
func (s *Store) AuditHead() (int64, string, error) {
	var seq int64
	var hash string
	err := s.db.QueryRow("SELECT seq,hash FROM audit_log ORDER BY seq DESC LIMIT 1").Scan(&seq, &hash)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, "", nil
	}
	return seq, hash, err
}

// AddAuditAnchor keeps a server receipt. A receipt never replaces an older one.
func (s *Store) AddAuditAnchor(a AuditAnchor) error {
	_, err := s.db.Exec("INSERT OR IGNORE INTO audit_anchors(seq,hash,at,receipt) VALUES(?,?,?,?)", a.Seq, a.Hash, a.At, a.Receipt)
	return err
}

// AuditAnchors lists the receipts, oldest first.
func (s *Store) AuditAnchors() ([]AuditAnchor, error) { return readAnchors(s.db) }

func readAnchors(q sqlRunner) ([]AuditAnchor, error) {
	rows, err := q.Query("SELECT seq,hash,at,receipt FROM audit_anchors ORDER BY seq")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []AuditAnchor{}
	for rows.Next() {
		var a AuditAnchor
		if err = rows.Scan(&a.Seq, &a.Hash, &a.At, &a.Receipt); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// auditObject is the current state of something the log tracks.
type auditObject struct {
	Object, ID, Label, Sum string
}

// invoiceSum covers the files and what the lists show of an issued invoice.
func invoiceSum(number, issueDate, gross string, xml, pdf []byte) string {
	return sumOf([]byte(stateJSON([]string{number, issueDate, gross, sumOf(xml), sumOf(pdf)})))
}

// deletedSum differs from invoiceSum, so a restore that brings a deleted invoice
// back shows up in the log.
func deletedSum(number, issueDate, gross, reason string, xml, pdf []byte) string {
	return sumOf([]byte("deleted:" + reason + ":" + invoiceSum(number, issueDate, gross, xml, pdf)))
}

// receivedSum covers the original and what the app read from it and decided.
func receivedSum(v model.ReceivedInvoice, original []byte) string {
	status := v.Status
	if status == "" {
		status = "active"
	}
	i := v.Inspection
	return sumOf([]byte(stateJSON([]string{sumOf(original), v.ReceiptNumber, v.Filename, v.ImportedAt, status, i.InvoiceNumber, i.IssueDate, i.SellerName, i.Currency, i.Totals.Net, i.Totals.Tax, i.Totals.Gross})))
}

func reminderSum(raw, pdf []byte) string { return sumOf([]byte(jsonSum(raw) + sumOf(pdf))) }

// jsonSum hashes a stored row independent of its formatting, because a backup
// document indents the rows it contains.
func jsonSum(raw []byte) string {
	var compact bytes.Buffer
	if err := json.Compact(&compact, raw); err != nil {
		return sumOf(raw)
	}
	return sumOf(compact.Bytes())
}

func paymentSum(p Payment) string { return sumOf([]byte(stateJSON(p))) }

func settingLabel(key string) string {
	switch key {
	case "company":
		return "Firmendaten"
	case financeKey:
		return "Steuer- und Finanzeinstellungen"
	case datevKey:
		return "DATEV-Einstellungen"
	case numberingKey:
		return "Rechnungsnummern"
	}
	return key
}

func filingLabel(key string) string {
	for prefix, name := range map[string]string{FilingVAT: "Umsatzsteuer-Voranmeldung", FilingZM: "Zusammenfassende Meldung", FilingAnnual: "Jahreserklärung"} {
		if id, ok := strings.CutPrefix(key, prefix); ok {
			if _, _, label, known := FilingPeriod(id); known {
				return name + " " + label
			}
			return name + " " + id
		}
	}
	return key
}

// auditObjects reads everything the log tracks. It reads each document once,
// one at a time, because the data can be larger than the memory of the device.
func auditObjects(q sqlRunner) ([]auditObject, error) {
	out := []auditObject{}
	rows, err := q.Query(`SELECT key,value FROM settings WHERE key LIKE 'expense:%' OR key LIKE 'received-category:%' OR key LIKE 'received-input-tax:%' OR key LIKE 'booking-treatment:%' OR key LIKE 'procedure-version:%' OR key LIKE ? OR key LIKE ? OR key LIKE ? OR key IN ('company',?,?,?)`,
		FilingVAT+"%", FilingZM+"%", FilingAnnual+"%", financeKey, datevKey, numberingKey)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var key string
		var raw []byte
		if err = rows.Scan(&key, &raw); err != nil {
			rows.Close()
			return nil, err
		}
		o := auditObject{Sum: jsonSum(raw)}
		switch {
		case strings.HasPrefix(key, "expense:"):
			var e Expense
			if err = json.Unmarshal(raw, &e); err != nil {
				rows.Close()
				return nil, err
			}
			o.Object, o.ID, o.Label = AuditExpense, e.ID, expenseLabel(e)
		case strings.HasPrefix(key, "received-category:"):
			o.Object, o.ID, o.Label = AuditCategory, strings.TrimPrefix(key, "received-category:"), "Zuordnung eines Eingangsbelegs"
		case strings.HasPrefix(key, inputTaxPrefix):
			o.Object, o.ID, o.Label = AuditInputTax, strings.TrimPrefix(key, inputTaxPrefix), inputTaxLabel
		case strings.HasPrefix(key, "booking-treatment:"):
			o.Object, o.ID, o.Label = AuditBooking, strings.TrimPrefix(key, "booking-treatment:"), "Einordnung eines Bankumsatzes"
		case strings.HasPrefix(key, "procedure-version:"):
			o.Object, o.ID, o.Label = AuditProcedure, strings.TrimPrefix(key, "procedure-version:"), "Verfahrensdokumentation"
		case strings.HasPrefix(key, FilingVAT), strings.HasPrefix(key, FilingZM), strings.HasPrefix(key, FilingAnnual):
			o.Object, o.ID, o.Label = AuditFiling, key, filingLabel(key)
		default:
			o.Object, o.ID, o.Label = AuditSetting, key, settingLabel(key)
		}
		out = append(out, o)
	}
	if err = rows.Close(); err != nil {
		return nil, err
	}

	var ids []string
	collect := func(query string) error {
		ids = nil
		r, err := q.Query(query)
		if err != nil {
			return err
		}
		defer r.Close()
		for r.Next() {
			var id string
			if err = r.Scan(&id); err != nil {
				return err
			}
			ids = append(ids, id)
		}
		return r.Err()
	}

	if err = collect(`SELECT id FROM received_invoices ORDER BY id`); err != nil {
		return nil, err
	}
	for _, id := range ids {
		var raw, original []byte
		if err = q.QueryRow(`SELECT data,original FROM received_invoices WHERE id=?`, id).Scan(&raw, &original); err != nil {
			return nil, err
		}
		var item model.ReceivedInvoice
		if err = json.Unmarshal(raw, &item); err != nil {
			return nil, err
		}
		out = append(out, auditObject{AuditReceived, id, receivedLabel(item), receivedSum(item, original)})
	}

	if err = collect(`SELECT id FROM invoices WHERE status<>'draft' ORDER BY id`); err != nil {
		return nil, err
	}
	issued := map[string]bool{}
	for _, id := range ids {
		o, err := issuedObject(q, id)
		if err != nil {
			return nil, err
		}
		issued[id] = true
		out = append(out, o)
	}

	if err = collect(`SELECT id FROM deleted_invoices ORDER BY id`); err != nil {
		return nil, err
	}
	for _, id := range ids {
		// A restore can bring back an invoice whose deletion stays in the log.
		if issued[id] {
			continue
		}
		var xml, pdf []byte
		var number, issueDate, gross, reason string
		if err = q.QueryRow(`SELECT number,issue_date,gross,reason,xml,pdf FROM deleted_invoices WHERE id=?`, id).Scan(&number, &issueDate, &gross, &reason, &xml, &pdf); err != nil {
			return nil, err
		}
		out = append(out, auditObject{AuditInvoice, id, "Rechnung " + number + " (gelöscht)", deletedSum(number, issueDate, gross, reason, xml, pdf)})
	}

	if err = collect(`SELECT id FROM invoice_reminders ORDER BY id`); err != nil {
		return nil, err
	}
	for _, id := range ids {
		var raw, pdf []byte
		if err = q.QueryRow(`SELECT data,pdf FROM invoice_reminders WHERE id=?`, id).Scan(&raw, &pdf); err != nil {
			return nil, err
		}
		out = append(out, auditObject{AuditReminder, id, "Zahlungserinnerung", reminderSum(raw, pdf)})
	}

	payments, err := readPayments(q)
	if err != nil {
		return nil, err
	}
	for _, p := range payments {
		out = append(out, auditObject{AuditPayment, p.ID, paymentLabel(p), paymentSum(p)})
	}

	locks, err := readLocks(q)
	if err != nil {
		return nil, err
	}
	for _, l := range locks {
		out = append(out, auditObject{AuditLock, l.ID, lockLabel(l), lockSum(l)})
	}
	return out, nil
}

// issuedObject reads an issued invoice with the files made when it was issued.
func issuedObject(q sqlRunner, id string) (auditObject, error) {
	var number, issueDate, gross string
	if err := q.QueryRow(`SELECT number,issue_date,gross FROM invoices WHERE id=?`, id).Scan(&number, &issueDate, &gross); err != nil {
		return auditObject{}, err
	}
	file := func(kind string) ([]byte, error) {
		var data []byte
		err := q.QueryRow("SELECT data FROM artifacts WHERE invoice_id=? AND kind=? ORDER BY id LIMIT 1", id, kind).Scan(&data)
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return data, err
	}
	xml, err := file("cii-xml")
	if err != nil {
		return auditObject{}, err
	}
	pdf, err := file("facturx-pdf")
	if err != nil {
		return auditObject{}, err
	}
	return auditObject{AuditInvoice, id, "Rechnung " + number, invoiceSum(number, issueDate, gross, xml, pdf)}, nil
}

// lastStates reads what the log says each object should look like.
func lastStates(entries []AuditEntry) map[string]AuditEntry {
	states := map[string]AuditEntry{}
	for _, e := range entries {
		if e.Sum == "" {
			continue
		}
		key := e.Object + "\x00" + e.ObjectID
		if e.Sum == auditGone {
			delete(states, key)
			continue
		}
		states[key] = e
	}
	return states
}

// reconcileAudit explains the current data in the log: it records every object
// whose state the log does not know yet, and every object that disappeared. It
// runs after a restore, so that a backup with edited data shows up object by
// object, and once for data from before the log existed.
func (s *Store) reconcileAudit(q sqlRunner, action, reason string) error {
	entries, err := readAudit(q)
	if err != nil {
		return err
	}
	states := lastStates(entries)
	objects, err := auditObjects(q)
	if err != nil {
		return err
	}
	for _, o := range objects {
		key := o.Object + "\x00" + o.ID
		last, known := states[key]
		delete(states, key)
		if known && last.Sum == o.Sum {
			continue
		}
		e := AuditEntry{Action: action, Object: o.Object, ObjectID: o.ID, Label: o.Label, Reason: reason, Sum: o.Sum}
		if known {
			e.Before = last.After
		}
		if err = s.logChange(q, e); err != nil {
			return err
		}
	}
	for _, id := range sortedKeys(states) {
		last := states[id]
		if err = s.logChange(q, AuditEntry{Action: action, Object: last.Object, ObjectID: last.ObjectID, Label: last.Label + " (nicht mehr vorhanden)", Reason: reason, Before: last.After, Sum: auditGone}); err != nil {
			return err
		}
	}
	return nil
}

func sortedKeys(m map[string]AuditEntry) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	for i := 1; i < len(keys); i++ {
		for j := i; j > 0 && keys[j] < keys[j-1]; j-- {
			keys[j], keys[j-1] = keys[j-1], keys[j]
		}
	}
	return keys
}

// baselineAudit records data from before the log existed, once.
func (s *Store) baselineAudit() error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var business int
	if err = tx.QueryRow("SELECT COUNT(*) FROM audit_log WHERE object<>?", AuditProgram).Scan(&business); err != nil {
		return err
	}
	if business > 0 {
		return nil
	}
	if err = s.reconcileAudit(tx, AuditBaseline, "Bestand bei Einführung des Änderungsprotokolls"); err != nil {
		return err
	}
	return tx.Commit()
}

// AuditProblem is something the check found.
type AuditProblem struct {
	Object   string `json:"object"`
	ObjectID string `json:"objectId"`
	Label    string `json:"label"`
	Message  string `json:"message"`
}

// AuditReport is the result of checking the log against the data.
type AuditReport struct {
	OK         bool           `json:"ok"`
	Entries    int            `json:"entries"`
	Objects    int            `json:"objects"`
	Files      int            `json:"files"`
	Head       string         `json:"head"`
	Anchors    int            `json:"anchors"`
	CheckedAt  string         `json:"checkedAt"`
	LastAnchor string         `json:"lastAnchor"`
	Problems   []AuditProblem `json:"problems"`
}

func verifyChain(entries []AuditEntry) *AuditProblem {
	previous := ""
	for i, e := range entries {
		if e.Seq != int64(i+1) {
			return &AuditProblem{Label: fmt.Sprintf("Eintrag %d", i+1), Message: "Im Protokoll fehlen Einträge."}
		}
		if e.Previous != previous || auditHash(e) != e.Hash {
			return &AuditProblem{Label: fmt.Sprintf("Eintrag %d", e.Seq), Message: "Dieser Protokolleintrag wurde nachträglich verändert."}
		}
		previous = e.Hash
	}
	return nil
}

// AnchorCheck verifies a server receipt.
type AnchorCheck func(AuditAnchor) error

// VerifyAudit checks the chain, every document against its hash, the data
// against the log and the receipts against the chain. Reading all documents can
// take a while, so it holds no transaction that would stop the app. Every tracked
// change adds a log entry, so an unchanged end of the log proves that nothing
// changed in between; otherwise the check starts again.
func (s *Store) VerifyAudit(check AnchorCheck) (AuditReport, error) {
	for attempt := 0; ; attempt++ {
		report, err := s.verifyOnce(check)
		if err != nil {
			return report, err
		}
		_, head, err := s.AuditHead()
		if err != nil {
			return report, err
		}
		if head == report.Head {
			return report, nil
		}
		if attempt == 2 {
			return report, errors.New("Während der Prüfung wurden Daten geändert. Prüfe noch einmal, wenn du fertig bist.")
		}
	}
}

func (s *Store) verifyOnce(check AnchorCheck) (AuditReport, error) {
	report := AuditReport{Problems: []AuditProblem{}, CheckedAt: now()}
	entries, err := readAudit(s.db)
	if err != nil {
		return report, err
	}
	report.Entries = len(entries)
	if len(entries) > 0 {
		report.Head = entries[len(entries)-1].Hash
	}
	if p := verifyChain(entries); p != nil {
		report.Problems = append(report.Problems, *p)
	}
	states := lastStates(entries)
	objects, err := auditObjects(s.db)
	if err != nil {
		return report, err
	}
	report.Objects = len(objects)
	for _, o := range objects {
		key := o.Object + "\x00" + o.ID
		last, known := states[key]
		delete(states, key)
		switch {
		case !known:
			report.Problems = append(report.Problems, AuditProblem{o.Object, o.ID, o.Label, "Dazu fehlt ein Protokolleintrag."})
		case last.Sum != o.Sum:
			report.Problems = append(report.Problems, AuditProblem{o.Object, o.ID, o.Label, "Der Inhalt weicht vom Protokoll ab."})
		}
	}
	for _, key := range sortedKeys(states) {
		last := states[key]
		report.Problems = append(report.Problems, AuditProblem{last.Object, last.ObjectID, last.Label, "Laut Protokoll müsste das noch vorhanden sein."})
	}
	files, err := verifyExpenseFiles(s.db, entries)
	report.Files = files.Count
	if err != nil {
		return report, err
	}
	report.Problems = append(report.Problems, files.Problems...)
	anchors, err := readAnchors(s.db)
	if err != nil {
		return report, err
	}
	report.Anchors = len(anchors)
	for _, a := range anchors {
		label := fmt.Sprintf("Nachweis vom %s", a.At[:min(10, len(a.At))])
		if a.Seq < 1 || a.Seq > int64(len(entries)) || entries[a.Seq-1].Hash != a.Hash {
			report.Problems = append(report.Problems, AuditProblem{"anchor", fmt.Sprint(a.Seq), label, "Das Protokoll passt nicht mehr zu diesem Nachweis des Servers."})
			continue
		}
		if check != nil {
			if err = check(a); err != nil {
				report.Problems = append(report.Problems, AuditProblem{"anchor", fmt.Sprint(a.Seq), label, "Die Signatur dieses Nachweises ist ungültig."})
				continue
			}
		}
		report.LastAnchor = a.At
	}
	report.OK = len(report.Problems) == 0
	return report, nil
}

// mergeAnchors keeps the receipts that belong to the log that stayed. A receipt
// of another history proves nothing here and would only raise an alarm.
func mergeAnchors(tx sqlRunner, backup []AuditAnchor) error {
	chain, err := readAudit(tx)
	if err != nil {
		return err
	}
	fits := func(a AuditAnchor) bool {
		return a.Seq >= 1 && a.Seq <= int64(len(chain)) && chain[a.Seq-1].Hash == a.Hash
	}
	local, err := readAnchors(tx)
	if err != nil {
		return err
	}
	for _, a := range local {
		if !fits(a) {
			if _, err = tx.Exec("DELETE FROM audit_anchors WHERE seq=?", a.Seq); err != nil {
				return err
			}
		}
	}
	for _, a := range backup {
		if fits(a) {
			if _, err = tx.Exec("INSERT OR IGNORE INTO audit_anchors(seq,hash,at,receipt) VALUES(?,?,?,?)", a.Seq, a.Hash, a.At, a.Receipt); err != nil {
				return err
			}
		}
	}
	return nil
}

// mergeAudit brings the log of a backup into the local one. The log is never
// taken back: a fresh install adopts the backup's history, a newer backup
// continues the local one, and an older or foreign backup only adds a note.
// What the restore changed is then recorded object by object.
func (s *Store) mergeAudit(tx sqlRunner, entries []AuditEntry, anchors []AuditAnchor, event BackupEvent) error {
	local, err := readAudit(tx)
	if err != nil {
		return err
	}
	business := 0
	for _, e := range local {
		if e.Object != AuditProgram {
			business++
		}
	}
	damaged := verifyChain(entries) != nil
	shared := 0
	for shared < len(local) && shared < len(entries) && local[shared].Hash == entries[shared].Hash {
		shared++
	}
	foreign := ""
	switch {
	case damaged:
		foreign = stateJSON(entries)
	case business == 0:
		if _, err = tx.Exec(`DROP TRIGGER IF EXISTS audit_log_immutable_update; DROP TRIGGER IF EXISTS audit_log_immutable_delete; DELETE FROM audit_log;`); err != nil {
			return err
		}
		for _, e := range entries {
			if err = insertAudit(tx, e); err != nil {
				return err
			}
		}
		if _, err = tx.Exec(auditTriggers); err != nil {
			return err
		}
	case shared == len(local):
		for _, e := range entries[shared:] {
			if err = insertAudit(tx, e); err != nil {
				return err
			}
		}
	case shared < len(entries):
		foreign = stateJSON(entries[shared:])
	}
	if err = mergeAnchors(tx, anchors); err != nil {
		return err
	}
	reason := "Sicherung vom " + event.BackupAt
	if event.BackupAt == "" {
		reason = "Wiederherstellung aus einer Sicherung"
	}
	if damaged {
		reason += ". Das Protokoll der Sicherung ist beschädigt oder wurde verändert."
	}
	label := event.Summary
	if label == "" {
		label = "Daten aus einer Sicherung wiederhergestellt"
	}
	if err = s.logChange(tx, AuditEntry{Action: AuditRestored, Object: AuditBackup, ObjectID: event.ID, Label: label, Reason: reason, Before: foreign}); err != nil {
		return err
	}
	if err = s.logVersion(tx); err != nil {
		return err
	}
	return s.reconcileAudit(tx, AuditRestored, reason)
}

// LogExport notes that data left the program for a tax audit.
func (s *Store) LogExport(label, summary, sum string) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err = s.logChange(tx, AuditEntry{Action: AuditExported, Object: AuditExport, ObjectID: sum, Label: label, After: summary}); err != nil {
		return err
	}
	return tx.Commit()
}

// auditScope limits the lists while a tax auditor looks at the data.
type auditScope struct {
	From, To string
}

func (a *auditScope) covers(dates ...string) bool {
	if a == nil {
		return true
	}
	for _, date := range dates {
		if date != "" && date >= a.From && date <= a.To {
			return true
		}
	}
	return false
}

// OpenAuditView makes the database read-only and limits the lists to a period
// (GoBD Rz. 159, 165, 174). The pool has one connection, so the switch covers
// every later statement of every component.
func (s *Store) OpenAuditView(from, to string) error {
	if !validDate(from) || !validDate(to) || from > to {
		return errors.New("Gib den Prüfungszeitraum an.")
	}
	if _, err := s.db.Exec("PRAGMA query_only=ON"); err != nil {
		return err
	}
	s.scope.Store(&auditScope{From: from, To: to})
	return nil
}

// CloseAuditView makes the database writable again.
func (s *Store) CloseAuditView() error {
	if _, err := s.db.Exec("PRAGMA query_only=OFF"); err != nil {
		return err
	}
	s.scope.Store(nil)
	return nil
}

// AuditView returns the open period, if any.
func (s *Store) AuditView() (from, to string, open bool) {
	if a := s.scope.Load(); a != nil {
		return a.From, a.To, true
	}
	return "", "", false
}
