package store

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
)

// A snapshot is one consistent state of all business data, in which every
// document (PDF, XML, photo) is replaced by the SHA-256 of its bytes. Documents
// never change, so a backup stores each of them once and a new state only adds
// what is new. Secrets stay in the keychain and are never part of a snapshot.

const expenseFileTable = `
CREATE TABLE IF NOT EXISTS expense_files (expense_id TEXT NOT NULL, sha256 TEXT NOT NULL, data BLOB NOT NULL, added_at TEXT NOT NULL, PRIMARY KEY(expense_id,sha256));`

// expenseFileTriggers keep every file a receipt ever had.
const expenseFileTriggers = `
CREATE TRIGGER IF NOT EXISTS expense_files_immutable_update BEFORE UPDATE ON expense_files BEGIN SELECT RAISE(ABORT, 'Belegdateien sind unveränderlich.'); END;
CREATE TRIGGER IF NOT EXISTS expense_files_immutable_delete BEFORE DELETE ON expense_files BEGIN SELECT RAISE(ABORT, 'Belegdateien sind unveränderlich.'); END;`

const snapshotTables = expenseFileTable + `
CREATE TABLE IF NOT EXISTS blob_index (source TEXT PRIMARY KEY, sum TEXT NOT NULL);
CREATE TABLE IF NOT EXISTS backup_log (id TEXT PRIMARY KEY, at TEXT NOT NULL, kind TEXT NOT NULL, data BLOB NOT NULL);
CREATE TRIGGER IF NOT EXISTS backup_log_immutable_update BEFORE UPDATE ON backup_log BEGIN SELECT RAISE(ABORT, 'Das Sicherungsprotokoll ist unveränderlich.'); END;
CREATE TRIGGER IF NOT EXISTS backup_log_immutable_delete BEFORE DELETE ON backup_log BEGIN SELECT RAISE(ABORT, 'Das Sicherungsprotokoll ist unveränderlich.'); END;`

// sqlRunner is a *sql.DB or a *sql.Tx.
type sqlRunner interface {
	Exec(string, ...any) (sql.Result, error)
	Query(string, ...any) (*sql.Rows, error)
	QueryRow(string, ...any) *sql.Row
}

func sumOf(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

// SnapshotBlob is a document a snapshot refers to. Source tells where its
// bytes are stored (see ReadBlob).
type SnapshotBlob struct {
	Sum    string
	Size   int64
	Source string
}

// SnapshotCounts summarize a state for the restore preview and the protocol.
type SnapshotCounts struct {
	Customers int `json:"customers"`
	Products  int `json:"products"`
	Invoices  int `json:"invoices"`
	Received  int `json:"received"`
	Expenses  int `json:"expenses"`
}

// Snapshot is a backup document plus the documents it refers to.
type Snapshot struct {
	Document []byte
	Blobs    []SnapshotBlob
	Counts   SnapshotCounts
}

// BlobOpener returns a document's bytes by its SHA-256 during a restore.
type BlobOpener func(sum string) ([]byte, error)

// Sources of documents, as "<kind>:<row id>".
const (
	sourceArtifact   = "artifact:"
	sourceReceived   = "received:"
	sourceReminder   = "reminder:"
	sourceDeletedXML = "deleted-xml:"
	sourceDeletedPDF = "deleted-pdf:"
	sourceExpense    = "expense:"
)

func readBlob(q sqlRunner, source string) ([]byte, error) {
	kind, id, ok := strings.Cut(source, ":")
	if !ok {
		return nil, errors.New("Unbekannte Dokumentquelle.")
	}
	query := map[string]string{
		"artifact":    "SELECT data FROM artifacts WHERE id=?",
		"received":    "SELECT original FROM received_invoices WHERE id=?",
		"reminder":    "SELECT pdf FROM invoice_reminders WHERE id=?",
		"deleted-xml": "SELECT xml FROM deleted_invoices WHERE id=?",
		"deleted-pdf": "SELECT pdf FROM deleted_invoices WHERE id=?",
		"expense":     "SELECT data FROM expense_files WHERE expense_id||':'||sha256=?",
	}[kind]
	if query == "" {
		return nil, errors.New("Unbekannte Dokumentquelle.")
	}
	var data []byte
	err := q.QueryRow(query, id).Scan(&data)
	return data, err
}

// ReadBlob returns the bytes of a snapshot's document and checks that they
// still match the hash the snapshot recorded.
func (s *Store) ReadBlob(b SnapshotBlob) ([]byte, error) {
	data, err := readBlob(s.db, b.Source)
	if err != nil {
		return nil, fmt.Errorf("Das Dokument %s ließ sich nicht lesen: %w", b.Source, err)
	}
	if sumOf(data) != b.Sum {
		return nil, fmt.Errorf("Das Dokument %s hat sich verändert. Die Sicherung wird beim nächsten Lauf neu aufgebaut.", b.Source)
	}
	return data, nil
}

// blobCollector turns documents into references while a snapshot is read. A
// nil collector keeps them inline, as the JSON backup does.
type blobCollector struct {
	q     sqlRunner
	known map[string]string
	blobs []SnapshotBlob
	seen  map[string]bool
}

func (c *blobCollector) reference(source string, size int64) (string, error) {
	if size == 0 {
		return "", nil
	}
	sum := c.known[source]
	if sum == "" {
		data, err := readBlob(c.q, source)
		if err != nil {
			return "", err
		}
		sum = sumOf(data)
	}
	if !c.seen[sum] {
		c.seen[sum] = true
		c.blobs = append(c.blobs, SnapshotBlob{Sum: sum, Size: size, Source: source})
	}
	return sum, nil
}

// indexBlobs hashes new documents once, so later snapshots need not read them.
// Rows that are only ever added are safe to remember by their id.
func (s *Store) indexBlobs() error {
	rows, err := s.db.Query(`
SELECT source FROM (
  SELECT 'artifact:'||id AS source FROM artifacts
  UNION ALL SELECT 'received:'||id FROM received_invoices
  UNION ALL SELECT 'reminder:'||id FROM invoice_reminders
  UNION ALL SELECT 'deleted-xml:'||id FROM deleted_invoices
  UNION ALL SELECT 'deleted-pdf:'||id FROM deleted_invoices
) WHERE source NOT IN (SELECT source FROM blob_index)`)
	if err != nil {
		return err
	}
	var sources []string
	for rows.Next() {
		var source string
		if err = rows.Scan(&source); err != nil {
			rows.Close()
			return err
		}
		sources = append(sources, source)
	}
	if err = rows.Close(); err != nil {
		return err
	}
	for _, source := range sources {
		data, err := readBlob(s.db, source)
		if err != nil {
			return err
		}
		if _, err = s.db.Exec("INSERT OR REPLACE INTO blob_index(source,sum) VALUES(?,?)", source, sumOf(data)); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) blobIndex() (map[string]string, error) {
	rows, err := s.db.Query("SELECT source,sum FROM blob_index")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	known := map[string]string{}
	for rows.Next() {
		var source, sum string
		if err = rows.Scan(&source, &sum); err != nil {
			return nil, err
		}
		known[source] = sum
	}
	return known, rows.Err()
}

// ExportSnapshot reads one consistent state without loading its documents.
func (s *Store) ExportSnapshot() (*Snapshot, error) {
	if err := s.indexBlobs(); err != nil {
		return nil, err
	}
	known, err := s.blobIndex()
	if err != nil {
		return nil, err
	}
	tx, err := s.db.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	c := &blobCollector{q: tx, known: known, seen: map[string]bool{}}
	doc, err := readDocument(tx, c)
	if err != nil {
		return nil, err
	}
	raw, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return nil, err
	}
	counts := SnapshotCounts{Customers: len(doc.Customers), Products: len(doc.Products), Invoices: len(doc.Invoices), Received: len(doc.Received)}
	for _, v := range doc.Settings {
		if strings.HasPrefix(v.Key, "expense:") {
			counts.Expenses++
		}
	}
	return &Snapshot{Document: raw, Blobs: c.blobs, Counts: counts}, nil
}

type backupExpenseFile struct {
	ExpenseID string `json:"expenseId"`
	SHA256    string `json:"sha256"`
	AddedAt   string `json:"addedAt,omitempty"`
	Data      []byte `json:"data,omitempty"`
	Blob      string `json:"blob,omitempty"`
}

func backupExpenseFiles(q sqlRunner, c *blobCollector) ([]backupExpenseFile, error) {
	rows, err := q.Query(`SELECT expense_id,sha256,added_at,length(data) FROM expense_files ORDER BY expense_id,added_at,sha256`)
	if err != nil {
		return nil, err
	}
	var items []backupExpenseFile
	var sizes []int64
	for rows.Next() {
		var v backupExpenseFile
		var size int64
		if err = rows.Scan(&v.ExpenseID, &v.SHA256, &v.AddedAt, &size); err != nil {
			rows.Close()
			return nil, err
		}
		items, sizes = append(items, v), append(sizes, size)
	}
	if err = rows.Close(); err != nil {
		return nil, err
	}
	for i := range items {
		source := sourceExpense + items[i].ExpenseID + ":" + items[i].SHA256
		if c == nil {
			if items[i].Data, err = readBlob(q, source); err != nil {
				return nil, err
			}
			continue
		}
		c.known[source] = items[i].SHA256
		if items[i].Blob, err = c.reference(source, sizes[i]); err != nil {
			return nil, err
		}
	}
	return items, nil
}

func restoreExpenseFiles(tx sqlRunner, items []backupExpenseFile, open BlobOpener) error {
	for _, v := range items {
		data := v.Data
		if v.Blob != "" {
			var err error
			if data, err = openBlob(open, v.Blob); err != nil {
				return err
			}
		}
		if len(data) == 0 {
			continue
		}
		if _, err := saveExpenseFile(tx, v.ExpenseID, data, v.AddedAt); err != nil {
			return err
		}
	}
	return nil
}

// openBlob fetches a referenced document and rejects anything but the exact
// bytes that were backed up.
func openBlob(open BlobOpener, sum string) ([]byte, error) {
	if open == nil {
		return nil, errors.New("Die Sicherung verweist auf Dokumente, die nicht vorliegen.")
	}
	data, err := open(sum)
	if err != nil {
		return nil, err
	}
	if sumOf(data) != sum {
		return nil, errors.New("Ein Dokument der Sicherung ist beschädigt oder wurde verändert.")
	}
	return data, nil
}

// BackupEvent is an entry of the backup protocol. It records what happened to
// the data as a whole, above all every restore, which replaces the current
// state. The protocol survives restores and can only grow.
type BackupEvent struct {
	ID string `json:"id"`
	At string `json:"at"`
	// Kind is BackupEventRestore, BackupEventVerify or BackupEventExport.
	Kind string `json:"kind"`
	OK   bool   `json:"ok"`
	// Summary is the sentence shown to the customer.
	Summary string `json:"summary"`
	// BackupAt is the time of the backup concerned.
	BackupAt   string `json:"backupAt,omitempty"`
	AppVersion string `json:"appVersion,omitempty"`
}

const (
	BackupEventRestore = "restore"
	BackupEventVerify  = "verify"
	BackupEventExport  = "export"
)

func insertBackupEvent(q sqlRunner, e BackupEvent) error {
	raw, err := json.Marshal(e)
	if err != nil {
		return err
	}
	_, err = q.Exec("INSERT OR IGNORE INTO backup_log(id,at,kind,data) VALUES(?,?,?,?)", e.ID, e.At, e.Kind, raw)
	return err
}

// AddBackupEvent appends to the protocol.
func (s *Store) AddBackupEvent(e BackupEvent) error {
	e.ID, e.At = newID("evt"), now()
	return insertBackupEvent(s.db, e)
}

func readBackupEvents(q sqlRunner) ([]BackupEvent, error) {
	rows, err := q.Query("SELECT data FROM backup_log ORDER BY at DESC,id DESC")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []BackupEvent{}
	for rows.Next() {
		var raw []byte
		var e BackupEvent
		if err = rows.Scan(&raw); err != nil {
			return nil, err
		}
		if err = json.Unmarshal(raw, &e); err != nil {
			return nil, err
		}
		items = append(items, e)
	}
	return items, rows.Err()
}

// BackupEvents lists the protocol, newest first.
func (s *Store) BackupEvents() ([]BackupEvent, error) { return readBackupEvents(s.db) }

// RestoreImpact names what a restore would remove from the current data.
type RestoreImpact struct {
	// Invoices are the numbers of issued invoices missing from the backup.
	Invoices []string `json:"invoices"`
	Received int      `json:"received"`
	Expenses int      `json:"expenses"`
}

// RestoreImpact compares a backup document with the current data.
func (s *Store) RestoreImpact(document []byte) (RestoreImpact, error) {
	impact := RestoreImpact{Invoices: []string{}}
	var doc struct {
		Invoices []struct {
			ID string `json:"id"`
		} `json:"invoices"`
		Received []struct {
			ID string `json:"id"`
		} `json:"received"`
		Settings []struct {
			Key string `json:"key"`
		} `json:"settings"`
	}
	if err := json.Unmarshal(document, &doc); err != nil {
		return impact, err
	}
	backedUp := map[string]bool{}
	for _, v := range doc.Invoices {
		backedUp["invoice:"+v.ID] = true
	}
	for _, v := range doc.Received {
		backedUp["received:"+v.ID] = true
	}
	for _, v := range doc.Settings {
		backedUp[v.Key] = true
	}
	rows, err := s.db.Query(`SELECT id,number FROM invoices WHERE status<>'draft' ORDER BY number`)
	if err != nil {
		return impact, err
	}
	for rows.Next() {
		var id, number string
		if err = rows.Scan(&id, &number); err != nil {
			rows.Close()
			return impact, err
		}
		if !backedUp["invoice:"+id] {
			impact.Invoices = append(impact.Invoices, number)
		}
	}
	if err = rows.Close(); err != nil {
		return impact, err
	}
	count := func(query, prefix string) (int, error) {
		rows, err := s.db.Query(query)
		if err != nil {
			return 0, err
		}
		defer rows.Close()
		n := 0
		for rows.Next() {
			var id string
			if err = rows.Scan(&id); err != nil {
				return 0, err
			}
			if !backedUp[prefix+id] {
				n++
			}
		}
		return n, rows.Err()
	}
	if impact.Received, err = count(`SELECT id FROM received_invoices`, "received:"); err != nil {
		return impact, err
	}
	impact.Expenses, err = count(`SELECT key FROM settings WHERE key LIKE 'expense:%'`, "")
	return impact, err
}

func artifactSource(id int64) string { return sourceArtifact + strconv.FormatInt(id, 10) }
