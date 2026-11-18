package store

import (
	"database/sql"
	"encoding/json"
	"errors"
	"sort"
	"strings"

	"github.com/shopspring/decimal"
	"papierleicht.de/e-rechnung-studio/internal/model"
)

// The legacy archive keeps what a customer brings along from another program
// ("Umzug"): the original files exactly as delivered, with the little the app
// can read from them. Nothing here ever changes or disappears, a restore only
// adds to it, and none of it counts in the figures of this app.

const legacyTables = `
CREATE TABLE IF NOT EXISTS legacy_documents (id TEXT PRIMARY KEY, source TEXT NOT NULL, source_id TEXT NOT NULL, direction TEXT NOT NULL, kind TEXT NOT NULL, number TEXT NOT NULL, date TEXT NOT NULL, partner TEXT NOT NULL, gross TEXT NOT NULL, data BLOB NOT NULL, imported_at TEXT NOT NULL, UNIQUE(source,source_id));
CREATE INDEX IF NOT EXISTS legacy_documents_date ON legacy_documents(date);
CREATE TABLE IF NOT EXISTS legacy_files (document_id TEXT NOT NULL REFERENCES legacy_documents(id), sha256 TEXT NOT NULL, name TEXT NOT NULL, mime TEXT NOT NULL, data BLOB NOT NULL, added_at TEXT NOT NULL, PRIMARY KEY(document_id,sha256));
CREATE INDEX IF NOT EXISTS legacy_files_sum ON legacy_files(sha256);
CREATE TRIGGER IF NOT EXISTS legacy_documents_immutable_update BEFORE UPDATE ON legacy_documents BEGIN SELECT RAISE(ABORT, 'Der Altbestand ist unveränderlich.'); END;
CREATE TRIGGER IF NOT EXISTS legacy_documents_immutable_delete BEFORE DELETE ON legacy_documents BEGIN SELECT RAISE(ABORT, 'Der Altbestand ist unveränderlich.'); END;
CREATE TRIGGER IF NOT EXISTS legacy_files_immutable_update BEFORE UPDATE ON legacy_files BEGIN SELECT RAISE(ABORT, 'Dateien im Altbestand sind unveränderlich.'); END;
CREATE TRIGGER IF NOT EXISTS legacy_files_immutable_delete BEFORE DELETE ON legacy_files BEGIN SELECT RAISE(ABORT, 'Dateien im Altbestand sind unveränderlich.'); END;`

// AuditLegacy is the change-log kind of a document in the legacy archive.
const AuditLegacy = "legacy"

const sourceLegacy = "legacy:"

// LegacyFilter narrows the archive list. Empty fields do not filter.
type LegacyFilter struct {
	Year      string `json:"year"`
	Direction string `json:"direction"`
	Source    string `json:"source"`
	Query     string `json:"query"`
}

func legacyLabel(d model.LegacyDocument) string {
	label := "Altbestand"
	if d.Number != "" {
		label += " " + d.Number
	} else if len(d.Files) > 0 {
		label += " " + d.Files[0].Name
	}
	return label
}

// legacySum covers the stored data and the hash of every file.
func legacySum(raw []byte, sums []string) string {
	sorted := append([]string(nil), sums...)
	sort.Strings(sorted)
	return sumOf([]byte(jsonSum(raw) + strings.Join(sorted, ",")))
}

// HasLegacyFile tells whether a file with these bytes is already archived.
func (s *Store) HasLegacyFile(sum string) (bool, error) {
	var n int
	err := s.db.QueryRow(`SELECT COUNT(*) FROM legacy_files WHERE sha256=?`, sum).Scan(&n)
	return n > 0, err
}

// AddLegacyDocument archives a document with its files in one transaction. It
// reports false without error when the document or one of its files is
// already in the archive, so an import can run again and only adds what is new.
func (s *Store) AddLegacyDocument(d model.LegacyDocument) (model.LegacyDocument, bool, error) {
	if len(d.Files) == 0 {
		return d, false, errors.New("Ein Dokument im Altbestand braucht mindestens eine Datei.")
	}
	if d.Source == "" || d.SourceID == "" {
		return d, false, errors.New("Die Herkunft des Dokuments fehlt.")
	}
	tx, err := s.db.Begin()
	if err != nil {
		return d, false, err
	}
	defer tx.Rollback()
	var n int
	if err = tx.QueryRow(`SELECT COUNT(*) FROM legacy_documents WHERE source=? AND source_id=?`, d.Source, d.SourceID).Scan(&n); err != nil || n > 0 {
		return d, false, err
	}
	sums := make([]string, len(d.Files))
	for i := range d.Files {
		f := &d.Files[i]
		f.SHA256, f.Size = sumOf(f.Data), len(f.Data)
		sums[i] = f.SHA256
		if err = tx.QueryRow(`SELECT COUNT(*) FROM legacy_files WHERE sha256=?`, f.SHA256).Scan(&n); err != nil || n > 0 {
			return d, false, err
		}
	}
	if d.ID == "" {
		d.ID = "leg-" + sums[0][:24]
	}
	if d.ImportedAt == "" {
		d.ImportedAt = now()
	}
	raw, err := encode(d)
	if err != nil {
		return d, false, err
	}
	if _, err = tx.Exec(`INSERT INTO legacy_documents(id,source,source_id,direction,kind,number,date,partner,gross,data,imported_at) VALUES(?,?,?,?,?,?,?,?,?,?,?)`,
		d.ID, d.Source, d.SourceID, d.Direction, d.Kind, d.Number, d.Date, d.Partner, d.Gross, raw, d.ImportedAt); err != nil {
		return d, false, err
	}
	for _, f := range d.Files {
		if _, err = tx.Exec(`INSERT INTO legacy_files(document_id,sha256,name,mime,data,added_at) VALUES(?,?,?,?,?,?)`, d.ID, f.SHA256, f.Name, f.MIME, f.Data, d.ImportedAt); err != nil {
			return d, false, err
		}
	}
	summary := d
	summary.Files = nil
	for _, f := range d.Files {
		summary.Files = append(summary.Files, model.LegacyFile{Name: f.Name, MIME: f.MIME, SHA256: f.SHA256, Size: f.Size})
	}
	if err = s.logChange(tx, AuditEntry{Action: AuditCreated, Object: AuditLegacy, ObjectID: d.ID, Label: legacyLabel(d), Reason: "Umzug aus " + d.Source, After: stateJSON(summary), Sum: legacySum(raw, sums)}); err != nil {
		return d, false, err
	}
	if err = tx.Commit(); err != nil {
		return d, false, err
	}
	for i := range d.Files {
		d.Files[i].Data = nil
	}
	return d, true, nil
}

// LegacyDocuments lists the archive, newest first, without file contents.
func (s *Store) LegacyDocuments(f LegacyFilter) ([]model.LegacyDocument, error) {
	query := `SELECT data FROM legacy_documents WHERE 1=1`
	var args []any
	if f.Year != "" {
		query += ` AND substr(date,1,4)=?`
		args = append(args, f.Year)
	}
	if f.Direction != "" {
		query += ` AND direction=?`
		args = append(args, f.Direction)
	}
	if f.Source != "" {
		query += ` AND source=?`
		args = append(args, f.Source)
	}
	if q := strings.TrimSpace(f.Query); q != "" {
		query += ` AND (number LIKE ? ESCAPE '\' OR partner LIKE ? ESCAPE '\' OR id IN (SELECT document_id FROM legacy_files WHERE name LIKE ? ESCAPE '\'))`
		like := "%" + strings.NewReplacer(`\`, `\\`, "%", `\%`, "_", `\_`).Replace(q) + "%"
		args = append(args, like, like, like)
	}
	query += ` ORDER BY date DESC, number DESC, id`
	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []model.LegacyDocument{}
	for rows.Next() {
		var raw []byte
		if err = rows.Scan(&raw); err != nil {
			return nil, err
		}
		var d model.LegacyDocument
		if err = json.Unmarshal(raw, &d); err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

// LegacyFile returns one file of a legacy document with its bytes.
func (s *Store) LegacyFile(documentID, sum string) (model.LegacyFile, error) {
	f := model.LegacyFile{SHA256: sum}
	err := s.db.QueryRow(`SELECT name,mime,data FROM legacy_files WHERE document_id=? AND sha256=?`, documentID, sum).Scan(&f.Name, &f.MIME, &f.Data)
	if errors.Is(err, sql.ErrNoRows) {
		return f, errors.New("Diese Datei gibt es im Altbestand nicht.")
	}
	f.Size = len(f.Data)
	return f, err
}

// LegacyYears sums up the archive per year and direction. Only documents with
// e-invoice data have amounts; the others are counted.
func (s *Store) LegacyYears() ([]model.LegacyYear, error) {
	docs, err := s.LegacyDocuments(LegacyFilter{})
	if err != nil {
		return nil, err
	}
	type key struct{ year, direction string }
	type sums struct {
		count, structured int
		net, tax, gross   decimal.Decimal
	}
	totals := map[key]*sums{}
	for _, d := range docs {
		year := "ohne Datum"
		if len(d.Date) >= 4 {
			year = d.Date[:4]
		}
		k := key{year, d.Direction}
		t := totals[k]
		if t == nil {
			t = &sums{}
			totals[k] = t
		}
		t.count++
		if !d.EInvoice {
			continue
		}
		t.structured++
		sign := decimal.NewFromInt(1)
		if d.Kind == "credit-note" {
			sign = sign.Neg()
		}
		for _, part := range []struct {
			to    *decimal.Decimal
			value string
		}{{&t.net, d.Net}, {&t.tax, d.Tax}, {&t.gross, d.Gross}} {
			if v, parseErr := decimal.NewFromString(part.value); parseErr == nil {
				*part.to = part.to.Add(v.Mul(sign))
			}
		}
	}
	out := make([]model.LegacyYear, 0, len(totals))
	for k, t := range totals {
		out = append(out, model.LegacyYear{Year: k.year, Direction: k.direction, Count: t.count, Structured: t.structured, Net: t.net.StringFixed(2), Tax: t.tax.StringFixed(2), Gross: t.gross.StringFixed(2)})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Year != out[j].Year {
			return out[i].Year > out[j].Year
		}
		return out[i].Direction < out[j].Direction
	})
	return out, nil
}

// legacyObjects lists the archive for the change log check.
func legacyObjects(q sqlRunner) ([]auditObject, error) {
	rows, err := q.Query(`SELECT d.id,d.data,GROUP_CONCAT(f.sha256) FROM legacy_documents d LEFT JOIN legacy_files f ON f.document_id=d.id GROUP BY d.id ORDER BY d.id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []auditObject
	for rows.Next() {
		var id string
		var raw []byte
		var files sql.NullString
		if err = rows.Scan(&id, &raw, &files); err != nil {
			return nil, err
		}
		var d model.LegacyDocument
		if err = json.Unmarshal(raw, &d); err != nil {
			return nil, err
		}
		var sums []string
		if files.String != "" {
			sums = strings.Split(files.String, ",")
		}
		out = append(out, auditObject{AuditLegacy, id, legacyLabel(d), legacySum(raw, sums)})
	}
	return out, rows.Err()
}

// verifyLegacyFiles checks that every archived file still has its bytes.
func verifyLegacyFiles(q sqlRunner) (fileCheck, error) {
	var result fileCheck
	rows, err := q.Query(`SELECT document_id,sha256 FROM legacy_files ORDER BY document_id,sha256`)
	if err != nil {
		return result, err
	}
	type file struct{ id, sum string }
	var files []file
	for rows.Next() {
		var f file
		if err = rows.Scan(&f.id, &f.sum); err != nil {
			rows.Close()
			return result, err
		}
		files = append(files, f)
	}
	if err = rows.Close(); err != nil {
		return result, err
	}
	for _, f := range files {
		var data []byte
		if err = q.QueryRow(`SELECT data FROM legacy_files WHERE document_id=? AND sha256=?`, f.id, f.sum).Scan(&data); err != nil {
			return result, err
		}
		result.Count++
		if sumOf(data) != f.sum {
			result.Problems = append(result.Problems, AuditProblem{AuditLegacy, f.id, "Datei im Altbestand", "Die Datei wurde verändert."})
		}
	}
	return result, nil
}

type backupLegacyDocument struct {
	ID         string          `json:"id"`
	Source     string          `json:"source"`
	SourceID   string          `json:"sourceId"`
	Data       json.RawMessage `json:"data"`
	ImportedAt string          `json:"importedAt"`
}

type backupLegacyFile struct {
	DocumentID string `json:"documentId"`
	SHA256     string `json:"sha256"`
	Name       string `json:"name"`
	MIME       string `json:"mime"`
	AddedAt    string `json:"addedAt"`
	Data       []byte `json:"data,omitempty"`
	Blob       string `json:"blob,omitempty"`
}

func backupLegacy(q sqlRunner, c *blobCollector) ([]backupLegacyDocument, []backupLegacyFile, error) {
	rows, err := q.Query(`SELECT id,source,source_id,data,imported_at FROM legacy_documents ORDER BY id`)
	if err != nil {
		return nil, nil, err
	}
	var docs []backupLegacyDocument
	for rows.Next() {
		var v backupLegacyDocument
		var raw []byte
		if err = rows.Scan(&v.ID, &v.Source, &v.SourceID, &raw, &v.ImportedAt); err != nil {
			rows.Close()
			return nil, nil, err
		}
		v.Data = raw
		docs = append(docs, v)
	}
	if err = rows.Close(); err != nil {
		return nil, nil, err
	}
	rows, err = q.Query(`SELECT document_id,sha256,name,mime,added_at,length(data) FROM legacy_files ORDER BY document_id,sha256`)
	if err != nil {
		return nil, nil, err
	}
	var files []backupLegacyFile
	var sizes []int64
	for rows.Next() {
		var v backupLegacyFile
		var size int64
		if err = rows.Scan(&v.DocumentID, &v.SHA256, &v.Name, &v.MIME, &v.AddedAt, &size); err != nil {
			rows.Close()
			return nil, nil, err
		}
		files, sizes = append(files, v), append(sizes, size)
	}
	if err = rows.Close(); err != nil {
		return nil, nil, err
	}
	for i := range files {
		source := sourceLegacy + files[i].DocumentID + ":" + files[i].SHA256
		if c == nil {
			if files[i].Data, err = readBlob(q, source); err != nil {
				return nil, nil, err
			}
			continue
		}
		c.known[source] = files[i].SHA256
		if files[i].Blob, err = c.reference(source, sizes[i]); err != nil {
			return nil, nil, err
		}
	}
	return docs, files, nil
}

// restoreLegacy only adds: the archive of the current data stays as it is.
func restoreLegacy(tx sqlRunner, docs []backupLegacyDocument, files []backupLegacyFile, open BlobOpener) error {
	for _, v := range docs {
		var d model.LegacyDocument
		if err := json.Unmarshal(v.Data, &d); err != nil {
			return errors.New("Ein Dokument des Altbestands in der Sicherung ist ungültig.")
		}
		if _, err := tx.Exec(`INSERT OR IGNORE INTO legacy_documents(id,source,source_id,direction,kind,number,date,partner,gross,data,imported_at) VALUES(?,?,?,?,?,?,?,?,?,?,?)`,
			v.ID, v.Source, v.SourceID, d.Direction, d.Kind, d.Number, d.Date, d.Partner, d.Gross, []byte(v.Data), v.ImportedAt); err != nil {
			return err
		}
	}
	for _, v := range files {
		data := v.Data
		if v.Blob != "" {
			var err error
			if data, err = openBlob(open, v.Blob); err != nil {
				return err
			}
		}
		if sumOf(data) != v.SHA256 {
			return errors.New("Eine Datei des Altbestands in der Sicherung ist beschädigt.")
		}
		if _, err := tx.Exec(`INSERT OR IGNORE INTO legacy_files(document_id,sha256,name,mime,data,added_at) VALUES(?,?,?,?,?,?)`, v.DocumentID, v.SHA256, v.Name, v.MIME, data, v.AddedAt); err != nil {
			return err
		}
	}
	return nil
}
