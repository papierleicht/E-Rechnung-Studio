package store

import (
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/shopspring/decimal"
	_ "modernc.org/sqlite"
	"papierleicht.de/e-rechnung-studio/internal/model"
	"papierleicht.de/e-rechnung-studio/internal/shops"
)

type Store struct {
	db *sql.DB
	// version is the running program, noted in the change log.
	version string
	// scope is set while an auditor looks at the data, see OpenAuditView.
	scope atomic.Pointer[auditScope]
}

type backupSetting struct {
	Key   string          `json:"key"`
	Value json.RawMessage `json:"value"`
}

type backupNamed struct {
	ID        string          `json:"id"`
	Name      string          `json:"name"`
	Data      json.RawMessage `json:"data"`
	CreatedAt string          `json:"createdAt"`
	UpdatedAt string          `json:"updatedAt"`
}

type backupInvoice struct {
	ID           string          `json:"id"`
	Number       string          `json:"number"`
	CustomerName string          `json:"customerName"`
	Status       string          `json:"status"`
	IssueDate    string          `json:"issueDate"`
	Gross        string          `json:"gross"`
	Data         json.RawMessage `json:"data"`
	CreatedAt    string          `json:"createdAt"`
	UpdatedAt    string          `json:"updatedAt"`
}

type backupArtifact struct {
	Encoding  string          `json:"encoding,omitempty"`
	ID        int64           `json:"id"`
	InvoiceID string          `json:"invoiceId"`
	Kind      string          `json:"kind"`
	Data      json.RawMessage `json:"data,omitempty"`
	// Blob is the SHA-256 of the file when a snapshot keeps it separately.
	Blob      string `json:"blob,omitempty"`
	CreatedAt string `json:"createdAt"`
}

type backupCounter struct {
	Name  string `json:"name"`
	Value int    `json:"value"`
}

type backupDocument struct {
	Received   []backupReceived `json:"received"`
	Format     string           `json:"format"`
	Version    int              `json:"version"`
	ExportedAt string           `json:"exportedAt"`
	Settings   []backupSetting  `json:"settings"`
	Customers  []backupNamed    `json:"customers"`
	Products   []backupNamed    `json:"products"`
	Invoices   []backupInvoice  `json:"invoices"`
	Artifacts  []backupArtifact `json:"artifacts"`
	Counters   []backupCounter  `json:"counters"`
	Reminders  []backupReminder `json:"reminders,omitempty"`
	Quotes     []backupQuote    `json:"quotes,omitempty"`
	// Recurring exists from version 8.
	Recurring []backupRecurring `json:"recurringInvoices,omitempty"`
	// Deleted is the log of deleted invoices (version 5). Older versions
	// would drop it and could hand out its numbers again.
	Deleted []backupDeleted `json:"deletedInvoices,omitempty"`
	// ExpenseFiles and BackupLog exist from version 6. Before that, files
	// were part of the expense rows in Settings.
	ExpenseFiles []backupExpenseFile `json:"expenseFiles,omitempty"`
	BackupLog    []BackupEvent       `json:"backupLog,omitempty"`
	// AuditLog, AuditAnchors and PeriodLocks exist from version 7.
	AuditLog     []AuditEntry  `json:"auditLog,omitempty"`
	AuditAnchors []AuditAnchor `json:"auditAnchors,omitempty"`
	PeriodLocks  []PeriodLock  `json:"periodLocks,omitempty"`
	// ShopConnections and ShopOrders exist from version 9. Shop credentials
	// stay in the keychain.
	ShopConnections []shops.Connection `json:"shopConnections,omitempty"`
	ShopOrders      []shops.Order      `json:"shopOrders,omitempty"`
}

func Open(path string) (*Store, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, err
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	if _, err = db.Exec(`PRAGMA journal_mode=WAL; PRAGMA foreign_keys=ON; PRAGMA busy_timeout=5000;`); err != nil {
		db.Close()
		return nil, err
	}
	// The database also holds bank data; Windows ignores the mode bits.
	if err = os.Chmod(path, 0o600); err != nil {
		db.Close()
		return nil, err
	}
	s := &Store{db: db}
	if err = s.migrate(); err != nil {
		db.Close()
		return nil, err
	}
	if err = s.migrateRecurring(); err != nil {
		db.Close()
		return nil, err
	}
	if err = s.migrateQuotes(); err != nil {
		db.Close()
		return nil, err
	}
	if err = s.baselineAudit(); err != nil {
		db.Close()
		return nil, err
	}
	return s, nil
}

func (s *Store) Close() error { return s.db.Close() }

// DB is the shared application database, which also holds the banking tables.
// The pool has a single connection: callers must not keep rows or a
// transaction open while calling into another component.
func (s *Store) DB() *sql.DB { return s.db }

func (s *Store) migrate() error {
	_, err := s.db.Exec(`
CREATE TABLE IF NOT EXISTS settings (key TEXT PRIMARY KEY, value BLOB NOT NULL);
CREATE TABLE IF NOT EXISTS customers (id TEXT PRIMARY KEY, name TEXT NOT NULL, data BLOB NOT NULL, created_at TEXT NOT NULL, updated_at TEXT NOT NULL);
CREATE TABLE IF NOT EXISTS products (id TEXT PRIMARY KEY, name TEXT NOT NULL, data BLOB NOT NULL, created_at TEXT NOT NULL, updated_at TEXT NOT NULL);
CREATE TABLE IF NOT EXISTS invoices (id TEXT PRIMARY KEY, number TEXT NOT NULL DEFAULT '', customer_name TEXT NOT NULL DEFAULT '', status TEXT NOT NULL, issue_date TEXT NOT NULL DEFAULT '', gross TEXT NOT NULL DEFAULT '0.00', data BLOB NOT NULL, created_at TEXT NOT NULL, updated_at TEXT NOT NULL);
CREATE UNIQUE INDEX IF NOT EXISTS invoices_number_issued ON invoices(number) WHERE status <> 'draft' AND number <> '';
CREATE TABLE IF NOT EXISTS artifacts (id INTEGER PRIMARY KEY AUTOINCREMENT, invoice_id TEXT NOT NULL REFERENCES invoices(id), kind TEXT NOT NULL, data BLOB NOT NULL, created_at TEXT NOT NULL);
CREATE TABLE IF NOT EXISTS counters (name TEXT PRIMARY KEY, value INTEGER NOT NULL);
CREATE TABLE IF NOT EXISTS received_invoices (id TEXT PRIMARY KEY, data BLOB NOT NULL, original BLOB NOT NULL);
CREATE TABLE IF NOT EXISTS shop_connections (id TEXT PRIMARY KEY, provider TEXT NOT NULL, url TEXT NOT NULL, data BLOB NOT NULL, UNIQUE(provider,url));
CREATE UNIQUE INDEX IF NOT EXISTS shop_connections_wix_site ON shop_connections(json_extract(data,'$.siteId')) WHERE provider='wix' AND COALESCE(json_extract(data,'$.siteId'),'')<>'';
CREATE TABLE IF NOT EXISTS shop_orders (id TEXT PRIMARY KEY, connection_id TEXT NOT NULL REFERENCES shop_connections(id), external_id TEXT NOT NULL, invoice_id TEXT NOT NULL DEFAULT '', data BLOB NOT NULL, UNIQUE(connection_id,external_id));
CREATE INDEX IF NOT EXISTS shop_orders_invoice ON shop_orders(invoice_id) WHERE invoice_id <> '';
PRAGMA user_version=1;`)
	if err != nil {
		return err
	}
	if err = migrateReminders(s.db); err != nil {
		return err
	}
	if err = migrateDeleted(s.db); err != nil {
		return err
	}
	if _, err = s.db.Exec(snapshotTables); err != nil {
		return err
	}
	if err = upgradeExpenseFiles(s.db); err != nil {
		return err
	}
	if _, err = s.db.Exec(auditTables + lockTables); err != nil {
		return err
	}
	if err = migrateExpenseFiles(s.db); err != nil {
		return err
	}
	return s.migratePayments()
}

func encode(v any) ([]byte, error) { return json.Marshal(v) }
func now() string                  { return time.Now().UTC().Format(time.RFC3339) }

func (s *Store) Company() (model.Company, error) {
	var raw []byte
	err := s.db.QueryRow(`SELECT value FROM settings WHERE key='company'`).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return model.Company{Address: model.Address{Country: "DE"}}, nil
	}
	if err != nil {
		return model.Company{}, err
	}
	var c model.Company
	err = json.Unmarshal(raw, &c)
	return c, err
}

func (s *Store) SaveCompany(c model.Company) error {
	raw, err := encode(c)
	if err != nil {
		return err
	}
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err = s.writeTracked(tx, "company", raw); err != nil {
		return err
	}
	return tx.Commit()
}

// lastID keeps IDs unique with a coarse clock: on Windows, records saved back
// to back can read the same UnixNano value.
var lastID atomic.Int64

func newID(prefix string) string {
	n := time.Now().UnixNano()
	for {
		last := lastID.Load()
		if n <= last {
			n = last + 1
		}
		if lastID.CompareAndSwap(last, n) {
			return fmt.Sprintf("%s-%d", prefix, n)
		}
	}
}

func (s *Store) SaveCustomer(c model.Customer) (model.Customer, error) {
	t := now()
	if c.ID != "" && c.InvoiceSettings == nil {
		var raw []byte
		err := s.db.QueryRow("SELECT data FROM customers WHERE id=?", c.ID).Scan(&raw)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return c, err
		}
		if err == nil {
			var existing model.Customer
			if err = json.Unmarshal(raw, &existing); err != nil {
				return c, err
			}
			c.InvoiceSettings = existing.InvoiceSettings
		}
	}
	if c.ID == "" {
		c.ID = newID("cus")
		c.CreatedAt = t
	}
	c.UpdatedAt = t
	raw, err := encode(c)
	if err != nil {
		return c, err
	}
	_, err = s.db.Exec(`INSERT INTO customers(id,name,data,created_at,updated_at) VALUES(?,?,?,?,?) ON CONFLICT(id) DO UPDATE SET name=excluded.name,data=excluded.data,updated_at=excluded.updated_at`, c.ID, c.Name, raw, c.CreatedAt, c.UpdatedAt)
	return c, err
}

func (s *Store) Customers() ([]model.Customer, error) {
	rows, err := s.db.Query(`SELECT data FROM customers ORDER BY name COLLATE NOCASE`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []model.Customer{}
	for rows.Next() {
		var raw []byte
		var v model.Customer
		if err = rows.Scan(&raw); err != nil {
			return nil, err
		}
		if err = json.Unmarshal(raw, &v); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

func (s *Store) SaveProduct(p model.Product) (model.Product, error) {
	t := now()
	if p.ID == "" {
		p.ID = newID("prd")
		p.CreatedAt = t
	}
	p.UpdatedAt = t
	raw, err := encode(p)
	if err != nil {
		return p, err
	}
	_, err = s.db.Exec(`INSERT INTO products(id,name,data,created_at,updated_at) VALUES(?,?,?,?,?) ON CONFLICT(id) DO UPDATE SET name=excluded.name,data=excluded.data,updated_at=excluded.updated_at`, p.ID, p.Name, raw, p.CreatedAt, p.UpdatedAt)
	return p, err
}

func (s *Store) Products() ([]model.Product, error) {
	rows, err := s.db.Query(`SELECT data FROM products ORDER BY name COLLATE NOCASE`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []model.Product{}
	for rows.Next() {
		var raw []byte
		var v model.Product
		if err = rows.Scan(&raw); err != nil {
			return nil, err
		}
		if err = json.Unmarshal(raw, &v); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

func (s *Store) DeleteCustomer(id string) error {
	result, err := s.db.Exec(`DELETE FROM customers WHERE id=?`, id)
	if err != nil {
		return err
	}
	if n, _ := result.RowsAffected(); n != 1 {
		return errors.New("Der Kunde wurde nicht gefunden.")
	}
	return nil
}

func (s *Store) DeleteProduct(id string) error {
	result, err := s.db.Exec(`DELETE FROM products WHERE id=?`, id)
	if err != nil {
		return err
	}
	if n, _ := result.RowsAffected(); n != 1 {
		return errors.New("Der Artikel wurde nicht gefunden.")
	}
	return nil
}

func (s *Store) SaveInvoice(v model.InvoiceDraft, gross string) (model.InvoiceDraft, error) {
	t := now()
	if v.ID == "" {
		v.ID = newID("inv")
		v.CreatedAt = t
	}
	if v.Status != "" && v.Status != "draft" {
		return v, errors.New("Die ausgestellte Rechnung kann nicht mehr geändert werden.")
	}
	v.UpdatedAt = t
	v.Status = "draft"
	v.MarkedSentAt = ""
	raw, err := encode(v)
	if err != nil {
		return v, err
	}
	tx, err := s.db.Begin()
	if err != nil {
		return v, err
	}
	defer tx.Rollback()
	result, err := tx.Exec(`INSERT INTO invoices(id,number,customer_name,status,issue_date,gross,data,created_at,updated_at) VALUES(?,?,?,?,?,?,?,?,?) ON CONFLICT(id) DO UPDATE SET number=excluded.number,customer_name=excluded.customer_name,status=excluded.status,issue_date=excluded.issue_date,gross=excluded.gross,data=excluded.data,updated_at=excluded.updated_at WHERE invoices.status='draft'`, v.ID, v.Number, v.Customer.Name, v.Status, v.IssueDate, gross, raw, v.CreatedAt, v.UpdatedAt)
	if err != nil {
		return v, err
	}
	if n, _ := result.RowsAffected(); n != 1 {
		return v, errors.New("Die ausgestellte Rechnung kann nicht mehr geändert werden.")
	}
	if err := rememberPaymentTerm(tx, v); err != nil {
		return v, err
	}
	return v, tx.Commit()
}

func (s *Store) Invoice(id string) (model.InvoiceDraft, error) {
	var raw []byte
	if err := s.db.QueryRow(`SELECT data FROM invoices WHERE id=?`, id).Scan(&raw); err != nil {
		return model.InvoiceDraft{}, err
	}
	var v model.InvoiceDraft
	err := json.Unmarshal(raw, &v)
	return v, err
}

func (s *Store) Invoices() ([]model.InvoiceSummary, error) {
	rows, err := s.db.Query(`SELECT id,number,customer_name,issue_date,status,gross,updated_at,COALESCE(NULLIF(json_extract(data,'$.customerId'),''),json_extract(data,'$.customer.id'),''),COALESCE(json_extract(data,'$.markedSentAt'),'') FROM invoices ORDER BY created_at DESC,id DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []model.InvoiceSummary{}
	for rows.Next() {
		var v model.InvoiceSummary
		if err = rows.Scan(&v.ID, &v.Number, &v.CustomerName, &v.IssueDate, &v.Status, &v.Gross, &v.UpdatedAt, &v.CustomerID, &v.SentAt); err != nil {
			return nil, err
		}
		if v.SentAt != "" {
			v.SentVia = "manual"
		}
		if s.scope.Load().covers(v.IssueDate) {
			out = append(out, v)
		}
	}
	return out, rows.Err()
}

func (s *Store) Artifact(invoiceID, kind string) ([]byte, error) {
	var data []byte
	err := s.db.QueryRow(`SELECT data FROM artifacts WHERE invoice_id=? AND kind=? ORDER BY id DESC LIMIT 1`, invoiceID, kind).Scan(&data)
	return data, err
}

func (s *Store) DeleteDraft(id string) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	r, err := tx.Exec(`DELETE FROM invoices WHERE id=? AND status='draft'`, id)
	if err != nil {
		return err
	}
	n, _ := r.RowsAffected()
	if n == 0 {
		return errors.New("Du kannst nur Rechnungsentwürfe löschen.")
	}
	if err = markShopInvoiceMissing(tx, id); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) Issue(id string, draft model.InvoiceDraft, gross string, makeArtifacts func(model.InvoiceDraft) (xml, pdf []byte, err error)) (model.InvoiceDraft, error) {
	tx, err := s.db.Begin()
	if err != nil {
		return draft, err
	}
	defer tx.Rollback()
	date, err := time.Parse("2006-01-02", draft.IssueDate)
	if err != nil {
		return draft, err
	}
	if draft, err = assignNumber(tx, id, draft, date); err != nil {
		return draft, err
	}
	draft.MarkedSentAt = ""
	xml, pdf, err := makeArtifacts(draft)
	if err != nil {
		return draft, err
	}
	if len(xml) == 0 || len(pdf) == 0 {
		return draft, errors.New("Die Rechnungsdateien dürfen bei der Ausstellung nicht leer sein.")
	}
	draft.Status = "issued"
	draft.IssuedAt = now()
	draft.PaidAt = ""
	draft.GeneratedXML = string(xml)
	draft.GeneratedAt = draft.IssuedAt
	draft.UpdatedAt = draft.IssuedAt
	raw, err := encode(draft)
	if err != nil {
		return draft, err
	}
	res, err := tx.Exec(`UPDATE invoices SET number=?,customer_name=?,status=?,issue_date=?,gross=?,data=?,updated_at=? WHERE id=? AND status='draft'`, draft.Number, draft.Customer.Name, draft.Status, draft.IssueDate, gross, raw, draft.UpdatedAt, id)
	if err != nil {
		return draft, err
	}
	n, _ := res.RowsAffected()
	if n != 1 {
		return draft, errors.New("Die Rechnung wurde nicht gefunden oder ist bereits ausgestellt.")
	}
	// Keep the original internal artifact keys for backup/database compatibility;
	// draft.Profile and draft.Syntax describe the actual CII/UBL and PDF format.
	if _, err = tx.Exec(`INSERT INTO artifacts(invoice_id,kind,data,created_at) VALUES(?,?,?,?)`, id, "cii-xml", xml, draft.IssuedAt); err != nil {
		return draft, err
	}
	if _, err = tx.Exec(`INSERT INTO artifacts(invoice_id,kind,data,created_at) VALUES(?,?,?,?)`, id, "facturx-pdf", pdf, draft.IssuedAt); err != nil {
		return draft, err
	}
	if err := rememberPaymentTerm(tx, draft); err != nil {
		return draft, err
	}
	summary := map[string]string{"number": draft.Number, "kind": draft.Kind, "customer": draft.Customer.Name, "issueDate": draft.IssueDate, "gross": gross, "currency": draft.Currency}
	if draft.AdvancePayment {
		summary["advancePayment"] = "true"
	}
	if err = s.logChange(tx, AuditEntry{Action: AuditIssued, Object: AuditInvoice, ObjectID: id, Label: "Rechnung " + draft.Number, After: stateJSON(summary), Sum: invoiceSum(draft.Number, draft.IssueDate, gross, xml, pdf)}); err != nil {
		return draft, err
	}
	if fullyPrepaidInvoice(draft, gross) {
		if err = s.recordPrepayment(tx, id, draft); err != nil {
			return draft, err
		}
		var paid []byte
		if err = tx.QueryRow(`SELECT data FROM invoices WHERE id=?`, id).Scan(&paid); err != nil {
			return draft, err
		}
		var settled model.InvoiceDraft
		if err = json.Unmarshal(paid, &settled); err != nil {
			return draft, err
		}
		draft = settled
	}
	return draft, tx.Commit()
}

func fullyPrepaidInvoice(draft model.InvoiceDraft, gross string) bool {
	if draft.Kind != "invoice" && draft.Kind != "corrected-invoice" {
		return false
	}
	prepaid, prepaidErr := decimal.NewFromString(strings.ReplaceAll(draft.PrepaidAmount, ",", "."))
	invoiceGross, grossErr := decimal.NewFromString(gross)
	return prepaidErr == nil && grossErr == nil && prepaid.IsPositive() && prepaid.Equal(invoiceGross)
}

// recordPrepayment books what a shop already collected for an invoice that
// states it as paid (BT-113). The payment ledger then shows the invoice as paid
// and the cash-basis figures count it like any other payment.
func (s *Store) recordPrepayment(tx *sql.Tx, id string, d model.InvoiceDraft) error {
	date, source, reason := d.IssueDate, "manual", "Laut Rechnung bereits bezahlt"
	var raw []byte
	err := tx.QueryRow(`SELECT data FROM shop_orders WHERE invoice_id=? ORDER BY id LIMIT 1`, id).Scan(&raw)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	if err == nil {
		var order shops.Order
		if err = json.Unmarshal(raw, &order); err != nil {
			return err
		}
		source, reason = "shop", "Bestellung "+order.Number+" im Shop bezahlt"
		if paid, parseErr := time.Parse(time.RFC3339, order.PaidAt); parseErr == nil {
			date = paid.In(time.Local).Format(time.DateOnly)
		} else {
			reason += ". Der Shop nennt kein Zahlungsdatum, eingetragen ist das Rechnungsdatum. Weicht es ab, korrigiere die Zahlung."
		}
	}
	scale, err := model.CurrencyScale(d.Currency)
	if err != nil {
		return err
	}
	amount, err := decimal.NewFromString(strings.ReplaceAll(d.PrepaidAmount, ",", "."))
	if err != nil {
		return err
	}
	payments, err := readPayments(tx)
	if err != nil {
		return err
	}
	payments = append(payments, Payment{ID: newID("pay"), InvoiceID: id, Direction: "outgoing", Amount: amount.StringFixed(scale), Currency: d.Currency, Date: date, Source: source, Reason: reason, CreatedAt: now()})
	if err = s.writePayments(tx, payments); err != nil {
		return err
	}
	return syncPaymentStatus(tx, "outgoing", id, payments)
}

func (s *Store) SetPaid(id string, paid bool) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	b, err := paymentTarget(tx, "outgoing", id)
	if err != nil {
		return err
	}
	payments, err := readPayments(tx)
	if err != nil {
		return err
	}
	if !paid {
		var raw []byte
		var gross string
		if err = tx.QueryRow(`SELECT gross,data FROM invoices WHERE id=?`, id).Scan(&gross, &raw); err != nil {
			return err
		}
		var v model.InvoiceDraft
		if err = json.Unmarshal(raw, &v); err != nil {
			return err
		}
		if fullyPrepaidInvoice(v, gross) {
			return errors.New("Laut dieser Rechnung ist schon alles bezahlt, deshalb kannst du sie nicht als offen markieren. Stimmt die Zahlung nicht, korrigiere sie unter den Zahlungsdetails oder erstelle eine Korrekturgutschrift.")
		}
	}
	if paid {
		if b, err = settledBalance(tx, b, payments); err != nil {
			return err
		}
		if b.State != "bezahlt" && b.State != "verrechnet" {
			payments = append(payments, Payment{ID: newID("pay"), InvoiceID: id, Direction: "outgoing", Amount: b.Remaining, Currency: b.Currency, Date: time.Now().Format(time.DateOnly), Source: "manual", Reason: "Als bezahlt markiert", CreatedAt: now()})
		}
	} else {
		for i, p := range payments {
			if p.InvoiceID == id && p.Direction == "outgoing" && p.RevokedAt == "" {
				payments[i].RevokedAt = now()
				payments[i].RevocationReason = "Zahlungsstatus manuell zurückgesetzt"
			}
		}
	}
	if err = s.writePayments(tx, payments); err != nil {
		return err
	}
	if err = syncPaymentStatus(tx, "outgoing", id, payments); err != nil {
		return err
	}
	return tx.Commit()
}

// ExportBackup returns one consistent state with all documents inline. It is
// meant for small data sets and tests; ExportSnapshot scales with the data.
func (s *Store) ExportBackup() (string, error) {
	tx, err := s.db.Begin()
	if err != nil {
		return "", err
	}
	defer tx.Rollback()
	doc, err := readDocument(tx, nil)
	if err != nil {
		return "", err
	}
	raw, err := json.MarshalIndent(doc, "", "  ")
	return string(raw), err
}

func readDocument(q sqlRunner, c *blobCollector) (backupDocument, error) {
	doc := backupDocument{Format: "e-rechnung-studio-backup", ExportedAt: now()}
	rows, err := q.Query(`SELECT key,value FROM settings ORDER BY key`)
	if err != nil {
		return doc, err
	}
	for rows.Next() {
		var v backupSetting
		if err = rows.Scan(&v.Key, &v.Value); err != nil {
			rows.Close()
			return doc, err
		}
		doc.Settings = append(doc.Settings, v)
	}
	if err = rows.Close(); err != nil {
		return doc, err
	}

	readNamed := func(table string) ([]backupNamed, error) {
		query := `SELECT id,name,data,created_at,updated_at FROM ` + table + ` ORDER BY id`
		r, queryErr := q.Query(query)
		if queryErr != nil {
			return nil, queryErr
		}
		defer r.Close()
		var out []backupNamed
		for r.Next() {
			var v backupNamed
			if queryErr = r.Scan(&v.ID, &v.Name, &v.Data, &v.CreatedAt, &v.UpdatedAt); queryErr != nil {
				return nil, queryErr
			}
			out = append(out, v)
		}
		return out, r.Err()
	}
	if doc.Customers, err = readNamed("customers"); err != nil {
		return doc, err
	}
	if doc.Products, err = readNamed("products"); err != nil {
		return doc, err
	}

	rows, err = q.Query(`SELECT id,number,customer_name,status,issue_date,gross,data,created_at,updated_at FROM invoices ORDER BY id`)
	if err != nil {
		return doc, err
	}
	for rows.Next() {
		var v backupInvoice
		if err = rows.Scan(&v.ID, &v.Number, &v.CustomerName, &v.Status, &v.IssueDate, &v.Gross, &v.Data, &v.CreatedAt, &v.UpdatedAt); err != nil {
			rows.Close()
			return doc, err
		}
		doc.Invoices = append(doc.Invoices, v)
	}
	if err = rows.Close(); err != nil {
		return doc, err
	}

	if doc.Artifacts, err = backupArtifacts(q, c); err != nil {
		return doc, err
	}

	rows, err = q.Query(`SELECT name,value FROM counters ORDER BY name`)
	if err != nil {
		return doc, err
	}
	for rows.Next() {
		var v backupCounter
		if err = rows.Scan(&v.Name, &v.Value); err != nil {
			rows.Close()
			return doc, err
		}
		doc.Counters = append(doc.Counters, v)
	}
	if err = rows.Close(); err != nil {
		return doc, err
	}
	if doc.Received, err = backupReceivedRows(q, c); err != nil {
		return doc, err
	}
	if doc.Reminders, err = backupReminders(q, c); err != nil {
		return doc, err
	}
	if doc.Quotes, err = backupQuotes(q); err != nil {
		return doc, err
	}
	if doc.Recurring, err = backupRecurringRows(q); err != nil {
		return doc, err
	}
	if doc.Deleted, err = backupDeletedRows(q, c); err != nil {
		return doc, err
	}
	if doc.ExpenseFiles, err = backupExpenseFiles(q, c); err != nil {
		return doc, err
	}
	if doc.BackupLog, err = readBackupEvents(q); err != nil {
		return doc, err
	}
	if doc.AuditLog, err = readAudit(q); err != nil {
		return doc, err
	}
	if doc.AuditAnchors, err = readAnchors(q); err != nil {
		return doc, err
	}
	if doc.PeriodLocks, err = readLocks(q); err != nil {
		return doc, err
	}
	if doc.ShopConnections, err = shopConnections(q); err != nil {
		return doc, err
	}
	if doc.ShopOrders, err = shopOrders(q, ""); err != nil {
		return doc, err
	}
	doc.Version = 9
	return doc, nil
}

func backupArtifacts(q sqlRunner, c *blobCollector) ([]backupArtifact, error) {
	rows, err := q.Query(`SELECT id,invoice_id,kind,length(data),created_at FROM artifacts ORDER BY id`)
	if err != nil {
		return nil, err
	}
	var items []backupArtifact
	var sizes []int64
	for rows.Next() {
		var v backupArtifact
		var size int64
		if err = rows.Scan(&v.ID, &v.InvoiceID, &v.Kind, &size, &v.CreatedAt); err != nil {
			rows.Close()
			return nil, err
		}
		items, sizes = append(items, v), append(sizes, size)
	}
	if err = rows.Close(); err != nil {
		return nil, err
	}
	for i := range items {
		v := &items[i]
		if c != nil {
			if v.Blob, err = c.reference(artifactSource(v.ID), sizes[i]); err != nil {
				return nil, err
			}
			continue
		}
		data, err := readBlob(q, artifactSource(v.ID))
		if err != nil {
			return nil, err
		}
		v.Encoding = "base64"
		v.Data = json.RawMessage(strconv.QuoteToASCII(base64.StdEncoding.EncodeToString(data)))
	}
	return items, nil
}

// RestoreBackup replaces the data with a JSON backup that holds its documents.
func (s *Store) RestoreBackup(data string) error {
	return s.restore([]byte(data), nil, BackupEvent{Kind: BackupEventRestore, OK: true, Summary: "Aus einer Sicherungsdatei wiederhergestellt."})
}

// RestoreSnapshot replaces the data with a snapshot document, whose documents
// open supplies. event is added to the protocol in the same transaction.
func (s *Store) RestoreSnapshot(document []byte, open BlobOpener, event BackupEvent) error {
	return s.restore(document, open, event)
}

func (s *Store) restore(data []byte, open BlobOpener, event BackupEvent) error {
	var doc backupDocument
	if err := json.Unmarshal(data, &doc); err != nil {
		return fmt.Errorf("Die Datensicherung konnte nicht gelesen werden: %w", err)
	}
	if doc.Format != "e-rechnung-studio-backup" || doc.Version < 1 || doc.Version > 9 {
		return errors.New("Das Format oder die Version der Datensicherung wird nicht unterstützt.")
	}
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	local, err := readCounters(tx)
	if err != nil {
		return err
	}
	for _, table := range []string{"shop_orders", "shop_connections", "received_invoices", "artifacts", "invoices", "customers", "products", "settings", "counters", "blob_index"} {
		if _, err = tx.Exec(`DELETE FROM ` + table); err != nil {
			return err
		}
	}
	for _, v := range doc.Received {
		original := v.Original
		if v.Blob != "" {
			if original, err = openBlob(open, v.Blob); err != nil {
				return err
			}
			if _, err = tx.Exec(`INSERT OR REPLACE INTO blob_index(source,sum) VALUES(?,?)`, sourceReceived+v.ID, v.Blob); err != nil {
				return err
			}
		}
		if original == nil {
			original = []byte{}
		}
		if _, err = tx.Exec(`INSERT INTO received_invoices(id,data,original) VALUES(?,?,?)`, v.ID, []byte(v.Data), original); err != nil {
			return err
		}
	}
	for _, v := range doc.Settings {
		if _, err = tx.Exec(`INSERT INTO settings(key,value) VALUES(?,?)`, v.Key, []byte(v.Value)); err != nil {
			return err
		}
	}
	writeNamed := func(table string, values []backupNamed) error {
		query := `INSERT INTO ` + table + `(id,name,data,created_at,updated_at) VALUES(?,?,?,?,?)`
		for _, v := range values {
			if _, insertErr := tx.Exec(query, v.ID, v.Name, []byte(v.Data), v.CreatedAt, v.UpdatedAt); insertErr != nil {
				return insertErr
			}
		}
		return nil
	}
	if err = writeNamed("customers", doc.Customers); err != nil {
		return err
	}
	if err = writeNamed("products", doc.Products); err != nil {
		return err
	}
	for _, v := range doc.Invoices {
		if _, err = tx.Exec(`INSERT INTO invoices(id,number,customer_name,status,issue_date,gross,data,created_at,updated_at) VALUES(?,?,?,?,?,?,?,?,?)`, v.ID, v.Number, v.CustomerName, v.Status, v.IssueDate, v.Gross, []byte(v.Data), v.CreatedAt, v.UpdatedAt); err != nil {
			return err
		}
	}
	for _, v := range doc.Artifacts {
		artifactBytes, err := artifactData(v, open)
		if err != nil {
			return err
		}
		if _, err = tx.Exec(`INSERT INTO artifacts(id,invoice_id,kind,data,created_at) VALUES(?,?,?,?,?)`, v.ID, v.InvoiceID, v.Kind, artifactBytes, v.CreatedAt); err != nil {
			return err
		}
		if v.Blob != "" {
			if _, err = tx.Exec(`INSERT OR REPLACE INTO blob_index(source,sum) VALUES(?,?)`, artifactSource(v.ID), v.Blob); err != nil {
				return err
			}
		}
	}
	for _, v := range doc.Counters {
		if _, err = tx.Exec(`INSERT INTO counters(name,value) VALUES(?,?)`, v.Name, v.Value); err != nil {
			return err
		}
	}
	// A number issued here after the backup was made must not be issued again,
	// even if the restore removes that invoice.
	for name, value := range local {
		if _, err = tx.Exec(`INSERT INTO counters(name,value) VALUES(?,?) ON CONFLICT(name) DO UPDATE SET value=MAX(value,excluded.value)`, name, value); err != nil {
			return err
		}
	}
	if err = replaceReminders(tx, doc.Reminders, open); err != nil {
		return err
	}
	if err = restoreQuotes(tx, doc.Quotes); err != nil {
		return err
	}
	if err = restoreRecurring(tx, doc.Recurring); err != nil {
		return err
	}
	if err = mergeDeleted(tx, doc.Deleted, open); err != nil {
		return err
	}
	if err = migrateExpenseFiles(tx); err != nil {
		return err
	}
	if err = restoreExpenseFiles(tx, doc.ExpenseFiles, open); err != nil {
		return err
	}
	if err = restoreShops(tx, doc.ShopConnections, doc.ShopOrders); err != nil {
		return err
	}
	if err = s.migratePaymentsIn(tx); err != nil {
		return err
	}
	// The protocols are the one thing a restore never takes back.
	for _, e := range doc.BackupLog {
		if err = insertBackupEvent(tx, e); err != nil {
			return err
		}
	}
	event.ID, event.At = newID("evt"), now()
	if err = insertBackupEvent(tx, event); err != nil {
		return err
	}
	for _, l := range doc.PeriodLocks {
		if _, err = tx.Exec("INSERT OR IGNORE INTO period_locks(id,from_date,to_date,at) VALUES(?,?,?,?)", l.ID, l.From, l.To, l.At); err != nil {
			return err
		}
	}
	if err = s.mergeAudit(tx, doc.AuditLog, doc.AuditAnchors, event); err != nil {
		return err
	}
	return tx.Commit()
}

func readCounters(q sqlRunner) (map[string]int, error) {
	rows, err := q.Query(`SELECT name,value FROM counters`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]int{}
	for rows.Next() {
		var name string
		var value int
		if err = rows.Scan(&name, &value); err != nil {
			return nil, err
		}
		out[name] = value
	}
	return out, rows.Err()
}

func artifactData(v backupArtifact, open BlobOpener) ([]byte, error) {
	if v.Blob != "" {
		return openBlob(open, v.Blob)
	}
	var artifact string
	if err := json.Unmarshal(v.Data, &artifact); err != nil {
		return nil, fmt.Errorf("Die Rechnungsdatei %d ist ungültig: %w", v.ID, err)
	}
	switch v.Encoding {
	case "":
		return []byte(artifact), nil
	case "base64":
		data, err := base64.StdEncoding.DecodeString(artifact)
		if err != nil {
			return nil, fmt.Errorf("Die Kodierung der Rechnungsdatei ist ungültig: %w", err)
		}
		return data, nil
	}
	return nil, errors.New("Die Kodierung der Rechnungsdatei wird nicht unterstützt.")
}

// Update only the customer preference, preserving current contact details.
func rememberPaymentTerm(tx *sql.Tx, d model.InvoiceDraft) error {
	if d.CustomerID == "" {
		return nil
	}
	var raw []byte
	err := tx.QueryRow("SELECT data FROM customers WHERE id=?", d.CustomerID).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	var c model.Customer
	if err = json.Unmarshal(raw, &c); err != nil {
		return err
	}
	issue, issueErr := time.Parse("2006-01-02", d.IssueDate)
	due, dueErr := time.Parse("2006-01-02", d.DueDate)
	if issueErr == nil && dueErr == nil && !due.Before(issue) {
		days := int(due.Sub(issue).Hours() / 24)
		c.PaymentTermDays = &days
	}
	kind := d.Kind
	// A correction references one particular invoice; it is never a customer default.
	if kind == "corrected-invoice" || d.CorrectsInvoiceID != "" {
		kind = "invoice"
	}
	series := ""
	if c.InvoiceSettings != nil {
		series = c.InvoiceSettings.NumberSeries
	}
	c.InvoiceSettings = &model.InvoiceSettings{NumberSeries: series, TaxSource: d.TaxSource, DisablePaymentQR: d.DisablePaymentQR, HidePaymentQRDetails: d.HidePaymentQRDetails, Profile: d.Profile, Kind: kind, PaymentMeansCode: d.PaymentMeansCode,
		TaxTreatment: d.TaxTreatment, TaxExemptionReason: d.TaxExemptionReason, BusinessProcess: d.BusinessProcess}
	c.Endpoint, c.EndpointScheme, c.BuyerReference = d.Customer.Endpoint, d.Customer.EndpointScheme, d.BuyerReference
	c.UpdatedAt = now()
	raw, err = encode(c)
	if err != nil {
		return err
	}
	_, err = tx.Exec("UPDATE customers SET data=?,updated_at=? WHERE id=?", raw, c.UpdatedAt, c.ID)
	return err
}

// CacheArtifact preserves the first generated container. Its source XML stays immutable.
func (s *Store) CacheArtifact(id, kind string, data []byte) ([]byte, error) {
	tx, err := s.db.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	var status string
	if err = tx.QueryRow("SELECT status FROM invoices WHERE id=?", id).Scan(&status); err != nil {
		return nil, err
	}
	if status == "draft" {
		return nil, errors.New("Stelle die Rechnung zuerst aus.")
	}
	var existing []byte
	err = tx.QueryRow("SELECT data FROM artifacts WHERE invoice_id=? AND kind=? ORDER BY id LIMIT 1", id, kind).Scan(&existing)
	if err == nil {
		return existing, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}
	if _, err = tx.Exec("INSERT INTO artifacts(invoice_id,kind,data,created_at) VALUES(?,?,?,?)", id, kind, data, now()); err != nil {
		return nil, err
	}
	return data, tx.Commit()
}
