package store

import (
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"papierleicht.de/e-rechnung-studio/internal/model"
	"strings"
)

// Deleting an issued invoice keeps it traceable (GoBD): the document, its
// files and the reason move into this log, which can only grow, and the
// number stays blocked. Only invoices nobody has received may be deleted.

const deletedTriggers = `
CREATE TRIGGER IF NOT EXISTS deleted_invoices_immutable_update BEFORE UPDATE ON deleted_invoices BEGIN SELECT RAISE(ABORT, 'Das Protokoll gelöschter Rechnungen ist unveränderlich.'); END;
CREATE TRIGGER IF NOT EXISTS deleted_invoices_immutable_delete BEFORE DELETE ON deleted_invoices BEGIN SELECT RAISE(ABORT, 'Das Protokoll gelöschter Rechnungen ist unveränderlich.'); END;`

func migrateDeleted(db interface {
	Exec(string, ...any) (sql.Result, error)
}) error {
	_, err := db.Exec(`
CREATE TABLE IF NOT EXISTS deleted_invoices (id TEXT PRIMARY KEY, number TEXT NOT NULL, customer_name TEXT NOT NULL, issue_date TEXT NOT NULL, gross TEXT NOT NULL, data BLOB NOT NULL, xml BLOB NOT NULL, pdf BLOB NOT NULL, reason TEXT NOT NULL, deleted_at TEXT NOT NULL);
CREATE UNIQUE INDEX IF NOT EXISTS deleted_invoices_number ON deleted_invoices(number);` + deletedTriggers)
	return err
}

// DeletedInvoice is one entry of the log of deleted invoices.
type DeletedInvoice struct {
	ID           string `json:"id"`
	Number       string `json:"number"`
	Kind         string `json:"kind"`
	CustomerName string `json:"customerName"`
	IssueDate    string `json:"issueDate"`
	IssuedAt     string `json:"issuedAt"`
	Gross        string `json:"gross"`
	Currency     string `json:"currency"`
	Reason       string `json:"reason"`
	DeletedAt    string `json:"deletedAt"`
}

// InvoiceLinks are the traces an issued invoice left. Each one means the
// invoice reached someone or entered the books and must stay.
type InvoiceLinks struct {
	Status       string   `json:"status"`
	MailedAt     string   `json:"mailedAt"`
	MarkedSentAt string   `json:"markedSentAt"`
	Payments     int      `json:"payments"`
	Installments bool     `json:"installments"`
	Reminders    int      `json:"reminders"`
	Corrections  []string `json:"corrections"`
}

func invoiceLinks(q numberReader, id string) (InvoiceLinks, model.InvoiceDraft, error) {
	var links InvoiceLinks
	var d model.InvoiceDraft
	var raw []byte
	if err := q.QueryRow("SELECT status,data FROM invoices WHERE id=?", id).Scan(&links.Status, &raw); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return links, d, errors.New("Die Rechnung wurde nicht gefunden.")
		}
		return links, d, err
	}
	if err := json.Unmarshal(raw, &d); err != nil {
		return links, d, err
	}
	links.MarkedSentAt = d.MarkedSentAt
	var mailLog int
	if err := q.QueryRow("SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='mail_log'").Scan(&mailLog); err != nil {
		return links, d, err
	}
	if mailLog > 0 {
		if err := q.QueryRow("SELECT COALESCE(MAX(sent_at),'') FROM mail_log WHERE document_type='invoice' AND document_id=? AND status='sent'", id).Scan(&links.MailedAt); err != nil {
			return links, d, err
		}
	}
	payments, err := readPayments(q)
	if err != nil {
		return links, d, err
	}
	for _, p := range payments {
		// Revoked entries count too: the invoice was treated as paid once.
		if p.InvoiceID == id && p.Direction == "outgoing" {
			links.Payments++
		}
	}
	var plans int
	if err = q.QueryRow("SELECT COUNT(*) FROM settings WHERE key=?", "installments:"+id).Scan(&plans); err != nil {
		return links, d, err
	}
	links.Installments = plans > 0
	if err = q.QueryRow("SELECT COUNT(*) FROM invoice_reminders WHERE invoice_id=?", id).Scan(&links.Reminders); err != nil {
		return links, d, err
	}
	rows, err := q.Query(`SELECT number,status FROM invoices WHERE json_extract(data,'$.correctsInvoiceId')=? ORDER BY created_at`, id)
	if err != nil {
		return links, d, err
	}
	defer rows.Close()
	links.Corrections = []string{}
	for rows.Next() {
		var number, status string
		if err = rows.Scan(&number, &status); err != nil {
			return links, d, err
		}
		if status == "draft" {
			number = "Entwurf"
		}
		links.Corrections = append(links.Corrections, number)
	}
	return links, d, rows.Err()
}

// InvoiceLinks reads the traces of an issued invoice.
func (s *Store) InvoiceLinks(id string) (InvoiceLinks, model.InvoiceDraft, error) {
	return invoiceLinks(s.db, id)
}

// DeleteIssuedInvoice moves an issued invoice into the log. check decides
// within the transaction whether its traces allow that. The automatic export
// paths are returned so the caller can remove those copies.
func (s *Store) DeleteIssuedInvoice(id, reason string, check func(model.InvoiceDraft, InvoiceLinks) error) (DeletedInvoice, model.InvoiceExport, error) {
	reason = strings.TrimSpace(reason)
	if reason == "" {
		return DeletedInvoice{}, model.InvoiceExport{}, errors.New("Gib an, warum du die Rechnung löschst.")
	}
	if len([]rune(reason)) > 500 {
		return DeletedInvoice{}, model.InvoiceExport{}, errors.New("Fasse den Grund in höchstens 500 Zeichen zusammen.")
	}
	tx, err := s.db.Begin()
	if err != nil {
		return DeletedInvoice{}, model.InvoiceExport{}, err
	}
	defer tx.Rollback()
	links, d, err := invoiceLinks(tx, id)
	if err != nil {
		return DeletedInvoice{}, model.InvoiceExport{}, err
	}
	if links.Status == "draft" {
		return DeletedInvoice{}, model.InvoiceExport{}, errors.New("Das ist ein Entwurf. Lösche ihn über „Entwurf löschen“.")
	}
	if err = check(d, links); err != nil {
		return DeletedInvoice{}, model.InvoiceExport{}, err
	}
	var entry DeletedInvoice
	var raw []byte
	if err = tx.QueryRow("SELECT number,customer_name,issue_date,gross,data FROM invoices WHERE id=?", id).Scan(&entry.Number, &entry.CustomerName, &entry.IssueDate, &entry.Gross, &raw); err != nil {
		return entry, model.InvoiceExport{}, err
	}
	artifact := func(kind string) ([]byte, error) {
		var data []byte
		err := tx.QueryRow("SELECT data FROM artifacts WHERE invoice_id=? AND kind=? ORDER BY id LIMIT 1", id, kind).Scan(&data)
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("Die gespeicherte Datei der Rechnung %s fehlt. Sie wird deshalb nicht gelöscht.", entry.Number)
		}
		return data, err
	}
	xml, err := artifact("cii-xml")
	if err != nil {
		return entry, model.InvoiceExport{}, err
	}
	pdf, err := artifact("facturx-pdf")
	if err != nil {
		return entry, model.InvoiceExport{}, err
	}
	entry.ID, entry.Reason, entry.DeletedAt = id, reason, now()
	entry.Kind, entry.IssuedAt, entry.Currency = d.Kind, d.IssuedAt, d.Currency
	// A restored invoice may already be in the log; that entry stays as it is,
	// and the new deletion only adds its reason to the change log.
	if _, err = tx.Exec(`INSERT OR IGNORE INTO deleted_invoices(id,number,customer_name,issue_date,gross,data,xml,pdf,reason,deleted_at) VALUES(?,?,?,?,?,?,?,?,?,?)`,
		id, entry.Number, entry.CustomerName, entry.IssueDate, entry.Gross, raw, xml, pdf, reason, entry.DeletedAt); err != nil {
		return entry, model.InvoiceExport{}, err
	}
	var logged struct {
		number, date, gross, reason string
		xml, pdf                    []byte
	}
	if err = tx.QueryRow("SELECT number,issue_date,gross,reason,xml,pdf FROM deleted_invoices WHERE id=?", id).Scan(&logged.number, &logged.date, &logged.gross, &logged.reason, &logged.xml, &logged.pdf); err != nil {
		return entry, model.InvoiceExport{}, fmt.Errorf("Die Rechnung %s ließ sich nicht ins Protokoll übernehmen. Sie wird deshalb nicht gelöscht.", entry.Number)
	}
	var export model.InvoiceExport
	var exportRaw []byte
	if err = tx.QueryRow("SELECT value FROM settings WHERE key=?", "export:"+id).Scan(&exportRaw); err == nil {
		_ = json.Unmarshal(exportRaw, &export)
	} else if !errors.Is(err, sql.ErrNoRows) {
		return entry, export, err
	}
	for _, statement := range []string{"DELETE FROM artifacts WHERE invoice_id=?", "DELETE FROM invoices WHERE id=?", "DELETE FROM settings WHERE key='export:'||?"} {
		if _, err = tx.Exec(statement, id); err != nil {
			return entry, export, err
		}
	}
	if err = markShopInvoiceMissing(tx, id); err != nil {
		return entry, export, err
	}
	if err = s.logChange(tx, AuditEntry{Action: AuditDeleted, Object: AuditInvoice, ObjectID: id, Label: "Rechnung " + entry.Number, Reason: reason, Sum: deletedSum(logged.number, logged.date, logged.gross, logged.reason, logged.xml, logged.pdf)}); err != nil {
		return entry, export, err
	}
	return entry, export, tx.Commit()
}

const deletedColumns = `id,number,customer_name,issue_date,gross,COALESCE(json_extract(data,'$.kind'),''),COALESCE(json_extract(data,'$.issuedAt'),''),COALESCE(json_extract(data,'$.currency'),''),reason,deleted_at`

func scanDeleted(row interface{ Scan(...any) error }) (DeletedInvoice, error) {
	var v DeletedInvoice
	err := row.Scan(&v.ID, &v.Number, &v.CustomerName, &v.IssueDate, &v.Gross, &v.Kind, &v.IssuedAt, &v.Currency, &v.Reason, &v.DeletedAt)
	return v, err
}

// DeletedInvoices lists the log, newest deletion first.
func (s *Store) DeletedInvoices() ([]DeletedInvoice, error) {
	list, err := s.deletedInvoices()
	if err != nil {
		return nil, err
	}
	kept := make([]DeletedInvoice, 0, len(list))
	for _, v := range list {
		if s.scope.Load().covers(v.IssueDate) {
			kept = append(kept, v)
		}
	}
	return kept, nil
}

func (s *Store) deletedInvoices() ([]DeletedInvoice, error) {
	rows, err := s.db.Query(`SELECT ` + deletedColumns + ` FROM deleted_invoices ORDER BY deleted_at DESC, id DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []DeletedInvoice{}
	for rows.Next() {
		v, err := scanDeleted(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

// DeletedInvoice returns a logged invoice with its draft data and files.
func (s *Store) DeletedInvoice(id string) (DeletedInvoice, model.InvoiceDraft, []byte, []byte, error) {
	v, err := scanDeleted(s.db.QueryRow(`SELECT `+deletedColumns+` FROM deleted_invoices WHERE id=?`, id))
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			err = errors.New("Die gelöschte Rechnung wurde nicht gefunden.")
		}
		return v, model.InvoiceDraft{}, nil, nil, err
	}
	var raw, xml, pdf []byte
	if err = s.db.QueryRow(`SELECT data,xml,pdf FROM deleted_invoices WHERE id=?`, id).Scan(&raw, &xml, &pdf); err != nil {
		return v, model.InvoiceDraft{}, nil, nil, err
	}
	var d model.InvoiceDraft
	err = json.Unmarshal(raw, &d)
	return v, d, xml, pdf, err
}

// MarkSent notes (or withdraws the note) that an issued invoice was sent
// outside the app. The document itself stays unchanged.
func (s *Store) MarkSent(id string, sent bool) error {
	at := ""
	if sent {
		at = now()
	}
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	res, err := tx.Exec(`UPDATE invoices SET data=CAST(json_set(data,'$.markedSentAt',?) AS BLOB) WHERE id=? AND status<>'draft'`, at, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n != 1 {
		return errors.New("Nur ausgestellte Rechnungen kannst du als verschickt markieren.")
	}
	var number string
	if err = tx.QueryRow(`SELECT number FROM invoices WHERE id=?`, id).Scan(&number); err != nil {
		return err
	}
	action := AuditUnsent
	if sent {
		action = AuditSent
	}
	if err = s.logChange(tx, AuditEntry{Action: action, Object: AuditInvoice, ObjectID: id, Label: "Rechnung " + number}); err != nil {
		return err
	}
	return tx.Commit()
}

type backupDeleted struct {
	ID           string          `json:"id"`
	Number       string          `json:"number"`
	CustomerName string          `json:"customerName"`
	IssueDate    string          `json:"issueDate"`
	Gross        string          `json:"gross"`
	Data         json.RawMessage `json:"data"`
	XML          string          `json:"xml,omitempty"`
	PDF          string          `json:"pdf,omitempty"`
	// XMLBlob and PDFBlob are the hashes when a snapshot keeps the files apart.
	XMLBlob   string `json:"xmlBlob,omitempty"`
	PDFBlob   string `json:"pdfBlob,omitempty"`
	Reason    string `json:"reason"`
	DeletedAt string `json:"deletedAt"`
}

func backupDeletedRows(q sqlRunner, c *blobCollector) ([]backupDeleted, error) {
	rows, err := q.Query(`SELECT id,number,customer_name,issue_date,gross,data,length(xml),length(pdf),reason,deleted_at FROM deleted_invoices ORDER BY deleted_at,id`)
	if err != nil {
		return nil, err
	}
	out := []backupDeleted{}
	type sizes struct{ xml, pdf int64 }
	var lengths []sizes
	for rows.Next() {
		var v backupDeleted
		var data []byte
		var size sizes
		if err = rows.Scan(&v.ID, &v.Number, &v.CustomerName, &v.IssueDate, &v.Gross, &data, &size.xml, &size.pdf, &v.Reason, &v.DeletedAt); err != nil {
			rows.Close()
			return nil, err
		}
		v.Data = json.RawMessage(data)
		out, lengths = append(out, v), append(lengths, size)
	}
	if err = rows.Close(); err != nil {
		return nil, err
	}
	for i := range out {
		v := &out[i]
		if c != nil {
			if v.XMLBlob, err = c.reference(sourceDeletedXML+v.ID, lengths[i].xml); err != nil {
				return nil, err
			}
			if v.PDFBlob, err = c.reference(sourceDeletedPDF+v.ID, lengths[i].pdf); err != nil {
				return nil, err
			}
			continue
		}
		xml, err := readBlob(q, sourceDeletedXML+v.ID)
		if err != nil {
			return nil, err
		}
		pdf, err := readBlob(q, sourceDeletedPDF+v.ID)
		if err != nil {
			return nil, err
		}
		v.XML, v.PDF = base64.StdEncoding.EncodeToString(xml), base64.StdEncoding.EncodeToString(pdf)
	}
	return out, nil
}

func deletedFile(inline, blob string, open BlobOpener) ([]byte, error) {
	if blob != "" {
		return openBlob(open, blob)
	}
	return base64.StdEncoding.DecodeString(inline)
}

// mergeDeleted adds the entries of a backup. The log only grows: an older backup
// must not bring a deleted invoice's number back into use or lose its files.
func mergeDeleted(tx *sql.Tx, values []backupDeleted, open BlobOpener) error {
	for _, v := range values {
		xml, err := deletedFile(v.XML, v.XMLBlob, open)
		if err != nil {
			return fmt.Errorf("Die gelöschte Rechnung %s ist ungültig: %w", v.Number, err)
		}
		pdf, err := deletedFile(v.PDF, v.PDFBlob, open)
		if err != nil {
			return fmt.Errorf("Die gelöschte Rechnung %s ist ungültig: %w", v.Number, err)
		}
		if !json.Valid(v.Data) {
			return fmt.Errorf("Die gelöschte Rechnung %s ist ungültig.", v.Number)
		}
		if _, err = tx.Exec(`INSERT OR IGNORE INTO deleted_invoices(id,number,customer_name,issue_date,gross,data,xml,pdf,reason,deleted_at) VALUES(?,?,?,?,?,?,?,?,?,?)`,
			v.ID, v.Number, v.CustomerName, v.IssueDate, v.Gross, []byte(v.Data), xml, pdf, v.Reason, v.DeletedAt); err != nil {
			return err
		}
	}
	return nil
}
