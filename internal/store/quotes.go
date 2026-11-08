package store

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"papierleicht.de/e-rechnung-studio/internal/model"
)

// Quotes have their own table and number range (AN-YYYY-NNNN). They never
// share the invoice numbering, artifacts or uniqueness index.
func (s *Store) migrateQuotes() error {
	_, err := s.db.Exec(`
CREATE TABLE IF NOT EXISTS quotes (id TEXT PRIMARY KEY, number TEXT NOT NULL DEFAULT '', customer_name TEXT NOT NULL DEFAULT '', status TEXT NOT NULL, issue_date TEXT NOT NULL DEFAULT '', valid_until TEXT NOT NULL DEFAULT '', gross TEXT NOT NULL DEFAULT '0.00', invoice_id TEXT NOT NULL DEFAULT '', data BLOB NOT NULL, created_at TEXT NOT NULL, updated_at TEXT NOT NULL);
CREATE UNIQUE INDEX IF NOT EXISTS quotes_number_final ON quotes(number) WHERE status <> 'draft' AND number <> '';`)
	return err
}

const quoteColumns = `id,number,customer_name,status,issue_date,valid_until,gross,invoice_id,data,created_at,updated_at`

func quotePrefix(year int) string { return fmt.Sprintf("AN-%d-", year) }

// Mirrors nextSequence for invoices: the stored counter and every final number
// with the standard prefix are considered, so manual numbers are never reused.
func nextQuoteSequence(db numberReader, year int) (int64, error) {
	var highest int64
	if err := db.QueryRow(`SELECT COALESCE((SELECT value FROM counters WHERE name=?),0)`, fmt.Sprintf("quote-%d", year)).Scan(&highest); err != nil {
		return 0, err
	}
	prefix := quotePrefix(year)
	rows, err := db.Query(`SELECT number FROM quotes WHERE status <> 'draft' AND number LIKE ?`, prefix+"%")
	if err != nil {
		return 0, err
	}
	defer rows.Close()
	for rows.Next() {
		var number string
		if err = rows.Scan(&number); err != nil {
			return 0, err
		}
		suffix := strings.TrimPrefix(number, prefix)
		if suffix == "" || strings.Trim(suffix, "0123456789") != "" {
			continue
		}
		n, e := strconv.ParseInt(suffix, 10, 64)
		if e != nil {
			return 0, fmt.Errorf("Angebotsnummer überschreitet den unterstützten Nummernbereich")
		}
		highest = max(highest, n)
	}
	if err = rows.Err(); err != nil {
		return 0, err
	}
	if highest == int64(1<<63-1) {
		return 0, fmt.Errorf("Angebotsnummernbereich ist ausgeschöpft")
	}
	return highest + 1, nil
}

// RefreshQuoteNumber is read-only, like RefreshDraftNumber. Drafts show the
// next free number; it is only reserved when the quote is marked as sent.
func (s *Store) RefreshQuoteNumber(q model.Quote) (model.Quote, error) {
	if q.Status != "" && q.Status != "draft" {
		return q, nil
	}
	if q.Number != "" && !q.AutomaticNumber {
		return q, nil
	}
	date, err := time.Parse(time.DateOnly, q.Document.IssueDate)
	if err != nil {
		return q, nil
	} // The editor reports the invalid date.
	n, err := nextQuoteSequence(s.db, date.Year())
	if err != nil {
		return q, err
	}
	q.Number = fmt.Sprintf("%s%04d", quotePrefix(date.Year()), n)
	q.AutomaticNumber = true
	return q, nil
}

func writeQuote(tx *sql.Tx, q model.Quote, gross string, condition string) error {
	raw, err := encode(q)
	if err != nil {
		return err
	}
	result, err := tx.Exec(`INSERT INTO quotes(`+quoteColumns+`) VALUES(?,?,?,?,?,?,?,?,?,?,?) ON CONFLICT(id) DO UPDATE SET number=excluded.number,customer_name=excluded.customer_name,status=excluded.status,issue_date=excluded.issue_date,valid_until=excluded.valid_until,gross=excluded.gross,invoice_id=excluded.invoice_id,data=excluded.data,updated_at=excluded.updated_at WHERE `+condition,
		q.ID, q.Number, q.Document.Customer.Name, q.Status, q.Document.IssueDate, q.ValidUntil, gross, q.InvoiceID, raw, q.CreatedAt, q.UpdatedAt)
	if err != nil {
		return err
	}
	if n, _ := result.RowsAffected(); n != 1 {
		return errors.New("Das Angebot wurde inzwischen geändert. Öffne es bitte noch einmal.")
	}
	return nil
}

// SaveQuote stores a draft. Sent quotes must be reopened explicitly first.
func (s *Store) SaveQuote(q model.Quote, gross string) (model.Quote, error) {
	t := now()
	if q.ID == "" {
		q.ID = newID("quo")
		q.CreatedAt = t
	}
	if q.Status != "" && q.Status != "draft" {
		return q, errors.New("Ein versendetes Angebot kannst du erst nach „Wieder bearbeiten“ ändern.")
	}
	q.Status, q.UpdatedAt = "draft", t
	q.InvoiceID, q.SentAt, q.DecidedAt, q.ConvertedAt = "", "", "", ""
	tx, err := s.db.Begin()
	if err != nil {
		return q, err
	}
	defer tx.Rollback()
	if err = writeQuote(tx, q, gross, `quotes.status='draft'`); err != nil {
		return q, err
	}
	return q, tx.Commit()
}

func readQuote(db numberReader, id string) (model.Quote, error) {
	var raw []byte
	var q model.Quote
	if err := db.QueryRow(`SELECT data FROM quotes WHERE id=?`, id).Scan(&raw); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return q, errors.New("Das Angebot wurde nicht gefunden.")
		}
		return q, err
	}
	if err := json.Unmarshal(raw, &q); err != nil {
		return q, err
	}
	// Deleting the generated invoice draft releases the quote again.
	if q.Status == "converted" {
		var exists int
		if err := db.QueryRow(`SELECT COUNT(*) FROM invoices WHERE id=?`, q.InvoiceID).Scan(&exists); err != nil {
			return q, err
		}
		if exists == 0 {
			q.Status, q.InvoiceID, q.ConvertedAt = "accepted", "", ""
		}
	}
	return q, nil
}

func (s *Store) Quote(id string) (model.Quote, error) { return readQuote(s.db, id) }

// QuoteInvoiceNumber names the linked invoice, which may still be a draft.
func (s *Store) QuoteInvoiceNumber(invoiceID string) (string, error) {
	var number string
	err := s.db.QueryRow(`SELECT number FROM invoices WHERE id=?`, invoiceID).Scan(&number)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	return number, err
}

// mailedQuotes maps quotes to their latest delivery; the mail service owns the table.
func (s *Store) mailedQuotes() (map[string]string, error) {
	mailed := map[string]string{}
	var tables int
	if err := s.db.QueryRow("SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='mail_log'").Scan(&tables); err != nil || tables == 0 {
		return mailed, err
	}
	rows, err := s.db.Query("SELECT document_id,MAX(sent_at) FROM mail_log WHERE document_type='quote' AND status='sent' GROUP BY document_id")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var id, at string
		if err = rows.Scan(&id, &at); err != nil {
			return nil, err
		}
		mailed[id] = at
	}
	return mailed, rows.Err()
}

func (s *Store) Quotes() ([]model.QuoteSummary, error) {
	rows, err := s.db.Query(`SELECT q.id,q.number,q.customer_name,q.issue_date,q.valid_until,q.status,q.gross,q.updated_at,COALESCE(json_extract(q.data,'$.document.customerId'),''),COALESCE(i.id,''),COALESCE(i.number,'') FROM quotes q LEFT JOIN invoices i ON i.id=q.invoice_id AND q.invoice_id<>'' ORDER BY q.created_at DESC,q.id DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []model.QuoteSummary{}
	for rows.Next() {
		var v model.QuoteSummary
		if err = rows.Scan(&v.ID, &v.Number, &v.CustomerName, &v.IssueDate, &v.ValidUntil, &v.Status, &v.Gross, &v.UpdatedAt, &v.CustomerID, &v.InvoiceID, &v.InvoiceNumber); err != nil {
			return nil, err
		}
		if v.Status == "converted" && v.InvoiceID == "" {
			v.Status = "accepted"
		}
		out = append(out, v)
	}
	if err = rows.Err(); err != nil {
		return nil, err
	}
	mailed, err := s.mailedQuotes()
	if err != nil {
		return nil, err
	}
	for i := range out {
		out[i].MailedAt = mailed[out[i].ID]
	}
	return out, nil
}

func (s *Store) DeleteQuote(id string) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var number string
	err = tx.QueryRow(`SELECT number FROM quotes WHERE id=? AND status='draft'`, id).Scan(&number)
	if errors.Is(err, sql.ErrNoRows) {
		return errors.New("Du kannst nur Angebotsentwürfe löschen.")
	}
	if err != nil {
		return err
	}
	if _, err = tx.Exec(`DELETE FROM quotes WHERE id=?`, id); err != nil {
		return err
	}
	// A reopened quote was sent once; its log entries explain where it went.
	var sent int
	if err = tx.QueryRow(`SELECT COUNT(*) FROM audit_log WHERE object=? AND object_id=?`, AuditQuote, id).Scan(&sent); err != nil {
		return err
	}
	if sent > 0 {
		if err = s.logChange(tx, AuditEntry{Action: AuditDeleted, Object: AuditQuote, ObjectID: id, Label: "Angebot " + number}); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// MarkQuoteSent reserves the number and stores the content as sent. The
// issuer snapshot keeps later PDFs identical to the version sent out.
func (s *Store) MarkQuoteSent(q model.Quote, gross string) (model.Quote, error) {
	tx, err := s.db.Begin()
	if err != nil {
		return q, err
	}
	defer tx.Rollback()
	stored, err := readQuote(tx, q.ID)
	if err != nil {
		return q, err
	}
	if stored.Status != "draft" {
		return q, errors.New("Das Angebot wurde bereits versendet.")
	}
	date, err := time.Parse(time.DateOnly, q.Document.IssueDate)
	if err != nil {
		return q, errors.New("Wähle ein gültiges Angebotsdatum.")
	}
	if q.Number == "" || q.AutomaticNumber {
		n, err := nextQuoteSequence(tx, date.Year())
		if err != nil {
			return q, err
		}
		if _, err = tx.Exec(`INSERT INTO counters(name,value) VALUES(?,?) ON CONFLICT(name) DO UPDATE SET value=excluded.value`, fmt.Sprintf("quote-%d", date.Year()), n); err != nil {
			return q, err
		}
		q.Number = fmt.Sprintf("%s%04d", quotePrefix(date.Year()), n)
	}
	q.AutomaticNumber = false
	q.Status, q.SentAt, q.UpdatedAt = "sent", now(), now()
	q.DecidedAt, q.InvoiceID, q.ConvertedAt = "", "", ""
	if err = writeQuote(tx, q, gross, `quotes.status='draft'`); err != nil {
		if strings.Contains(err.Error(), "UNIQUE") {
			return q, fmt.Errorf("Die Angebotsnummer %s ist bereits vergeben.", q.Number)
		}
		return q, err
	}
	// A sent quote is a business letter. Its content stays in the log, also if
	// the quote is reopened and sent again.
	if err = s.logChange(tx, AuditEntry{Action: AuditSent, Object: AuditQuote, ObjectID: q.ID, Label: "Angebot " + q.Number, After: stateJSON(q)}); err != nil {
		return q, err
	}
	return q, tx.Commit()
}

// ChangeQuoteStatus records the customer's decision or reopens a quote.
// Converted quotes are final while their invoice exists.
func (s *Store) ChangeQuoteStatus(id, status string) (model.Quote, error) {
	tx, err := s.db.Begin()
	if err != nil {
		return model.Quote{}, err
	}
	defer tx.Rollback()
	q, err := readQuote(tx, id)
	if err != nil {
		return q, err
	}
	allowed := map[string][]string{
		"accepted": {"sent", "rejected"},
		"rejected": {"sent", "accepted"},
		"sent":     {"accepted", "rejected"},
		"draft":    {"sent", "accepted", "rejected"},
	}[status]
	valid := false
	for _, from := range allowed {
		valid = valid || q.Status == from
	}
	if !valid {
		if q.Status == "converted" {
			return q, errors.New("Das Angebot wurde bereits in eine Rechnung umgewandelt.")
		}
		return q, errors.New("Dieser Statuswechsel ist für das Angebot nicht möglich.")
	}
	switch status {
	case "accepted", "rejected":
		q.DecidedAt = now()
	case "sent":
		q.DecidedAt = ""
	case "draft":
		// Reopening keeps the reserved number; the next sending stores new content.
		q.DecidedAt, q.SentAt = "", ""
		q.Document.IssuerSnapshot = nil
	}
	q.Status, q.UpdatedAt = status, now()
	var gross string
	if err = tx.QueryRow(`SELECT gross FROM quotes WHERE id=?`, id).Scan(&gross); err != nil {
		return q, err
	}
	if err = writeQuote(tx, q, gross, `quotes.status<>'draft'`); err != nil {
		return q, err
	}
	if err = s.logChange(tx, AuditEntry{Action: AuditChanged, Object: AuditQuote, ObjectID: q.ID, Label: "Angebot " + q.Number, After: stateJSON(map[string]string{"status": status})}); err != nil {
		return q, err
	}
	return q, tx.Commit()
}

// ConvertQuote creates the invoice draft and links it in one transaction, so
// a repeated click can never produce a second invoice for the same quote.
func (s *Store) ConvertQuote(id string, invoice model.InvoiceDraft, gross string) (model.InvoiceDraft, error) {
	tx, err := s.db.Begin()
	if err != nil {
		return invoice, err
	}
	defer tx.Rollback()
	q, err := readQuote(tx, id)
	if err != nil {
		return invoice, err
	}
	if q.Status == "converted" {
		return invoice, errors.New("Das Angebot wurde bereits in eine Rechnung umgewandelt.")
	}
	if q.Status != "sent" && q.Status != "accepted" {
		return invoice, errors.New("Du kannst nur versendete oder angenommene Angebote in eine Rechnung umwandeln.")
	}
	t := now()
	invoice.ID, invoice.Status, invoice.CreatedAt, invoice.UpdatedAt = newID("inv"), "draft", t, t
	invoice.QuoteID, invoice.QuoteNumber = q.ID, q.Number
	raw, err := encode(invoice)
	if err != nil {
		return invoice, err
	}
	if _, err = tx.Exec(`INSERT INTO invoices(id,number,customer_name,status,issue_date,gross,data,created_at,updated_at) VALUES(?,?,?,?,?,?,?,?,?)`, invoice.ID, invoice.Number, invoice.Customer.Name, invoice.Status, invoice.IssueDate, gross, raw, invoice.CreatedAt, invoice.UpdatedAt); err != nil {
		return invoice, err
	}
	if q.DecidedAt == "" {
		q.DecidedAt = t
	}
	q.Status, q.InvoiceID, q.ConvertedAt, q.UpdatedAt = "converted", invoice.ID, t, t
	var quoteGross string
	if err = tx.QueryRow(`SELECT gross FROM quotes WHERE id=?`, id).Scan(&quoteGross); err != nil {
		return invoice, err
	}
	if err = writeQuote(tx, q, quoteGross, `quotes.status<>'draft'`); err != nil {
		return invoice, err
	}
	if err = s.logChange(tx, AuditEntry{Action: AuditChanged, Object: AuditQuote, ObjectID: q.ID, Label: "Angebot " + q.Number, After: stateJSON(map[string]string{"status": q.Status, "invoiceId": invoice.ID})}); err != nil {
		return invoice, err
	}
	return invoice, tx.Commit()
}

type backupQuote struct {
	ID           string          `json:"id"`
	Number       string          `json:"number"`
	CustomerName string          `json:"customerName"`
	Status       string          `json:"status"`
	IssueDate    string          `json:"issueDate"`
	ValidUntil   string          `json:"validUntil"`
	Gross        string          `json:"gross"`
	InvoiceID    string          `json:"invoiceId"`
	Data         json.RawMessage `json:"data"`
	CreatedAt    string          `json:"createdAt"`
	UpdatedAt    string          `json:"updatedAt"`
}

func backupQuotes(q sqlRunner) ([]backupQuote, error) {
	rows, err := q.Query(`SELECT ` + quoteColumns + ` FROM quotes ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []backupQuote{}
	for rows.Next() {
		var v backupQuote
		var raw []byte
		if err = rows.Scan(&v.ID, &v.Number, &v.CustomerName, &v.Status, &v.IssueDate, &v.ValidUntil, &v.Gross, &v.InvoiceID, &raw, &v.CreatedAt, &v.UpdatedAt); err != nil {
			return nil, err
		}
		v.Data = raw
		items = append(items, v)
	}
	return items, rows.Err()
}

// Backups before version 4 contain no quotes; restoring them clears the table
// like every other restored table.
func restoreQuotes(tx *sql.Tx, items []backupQuote) error {
	if _, err := tx.Exec(`DELETE FROM quotes`); err != nil {
		return err
	}
	for _, v := range items {
		if !json.Valid(v.Data) {
			return fmt.Errorf("Das Angebot %s in der Datensicherung ist ungültig.", v.ID)
		}
		if _, err := tx.Exec(`INSERT INTO quotes(`+quoteColumns+`) VALUES(?,?,?,?,?,?,?,?,?,?,?)`, v.ID, v.Number, v.CustomerName, v.Status, v.IssueDate, v.ValidUntil, v.Gross, v.InvoiceID, []byte(v.Data), v.CreatedAt, v.UpdatedAt); err != nil {
			return err
		}
	}
	return nil
}
