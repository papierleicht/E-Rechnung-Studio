package store

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/shopspring/decimal"
)

// Expense is a receipt captured by hand: a paper or PDF receipt without
// e-invoice data, a bank fee or a cash purchase. Rows live in the settings
// table, so the regular backup includes them; the attached files live in
// expense_files, so listing receipts does not load every photo.
//
// A receipt is never deleted. Every change is in the change log, a mistake is
// cancelled with a reason, and a replaced file stays stored (GoBD Rz. 93, 108).
type Expense struct {
	ID string `json:"id"`
	// Number is assigned once when the receipt is recorded and never changes.
	Number string `json:"number"`
	// DocumentNumber is the number printed on the receipt, if it has one.
	DocumentNumber string `json:"documentNumber,omitempty"`
	// Date is the receipt date (Vorsteuer); PaidDate the payment (EÜR).
	Date     string `json:"date"`
	PaidDate string `json:"paidDate"`
	Supplier string `json:"supplier"`
	Note     string `json:"note"`
	Category string `json:"category"`
	// Gross is entered; Net and Tax follow from TaxRate ("19", "7", "0", "none",
	// RateMixed or a reverse-charge rate).
	Gross   string `json:"gross"`
	TaxRate string `json:"taxRate"`
	// Currency and OriginalGross describe a receipt in foreign currency; Gross
	// stays the amount in euro and ExchangeRate follows from the two.
	Currency      string `json:"currency,omitempty"`
	OriginalGross string `json:"originalGross,omitempty"`
	ExchangeRate  string `json:"exchangeRate,omitempty"`
	// ReducedGross is the part of Gross taxed at 7 % when TaxRate is RateMixed;
	// the rest is taxed at 19 %.
	ReducedGross string `json:"reducedGross,omitempty"`
	Net          string `json:"net"`
	Tax          string `json:"tax"`
	// PaidWith is "bank", "cash" or "private" (paid privately for the business).
	PaidWith string `json:"paidWith"`
	// Advance marks a payment ahead of the supply: its tax counts from PaidDate on.
	Advance bool `json:"advance,omitempty"`
	// BookingID links a bank booking; ReceivedID a stored PDF receipt.
	BookingID  string `json:"bookingId,omitempty"`
	ReceivedID string `json:"receivedId,omitempty"`
	// Years is the useful life of a purchase in the asset category.
	Years          int    `json:"years,omitempty"`
	AttachmentName string `json:"attachmentName,omitempty"`
	AttachmentType string `json:"attachmentType,omitempty"`
	// FileSum is the SHA-256 of the current file.
	FileSum string `json:"fileSum,omitempty"`
	// Attachment carries the file to and from SaveExpense and ExpenseAttachment;
	// it is never part of the stored row.
	Attachment []byte `json:"attachment,omitempty"`
	CreatedAt  string `json:"createdAt"`
	UpdatedAt  string `json:"updatedAt"`
	// CancelledAt marks a cancelled receipt, which no longer counts.
	CancelledAt  string `json:"cancelledAt,omitempty"`
	CancelReason string `json:"cancelReason,omitempty"`
	// Locked explains why the receipt can only be cancelled; it is worked out
	// when the receipt is read and never stored.
	Locked string `json:"locked,omitempty"`
}

func expenseLabel(e Expense) string {
	return strings.TrimSpace("Beleg " + e.Number + " " + e.Supplier)
}

// nextReceiptNumber numbers receipts per year of recording. Together with the
// log a missing number is noticed, and a number is never used twice.
func nextReceiptNumber(q sqlRunner) (string, error) {
	year := time.Now().Year()
	name := fmt.Sprintf("receipt-%d", year)
	if _, err := q.Exec(`INSERT INTO counters(name,value) VALUES(?,1) ON CONFLICT(name) DO UPDATE SET value=value+1`, name); err != nil {
		return "", err
	}
	var n int
	if err := q.QueryRow(`SELECT value FROM counters WHERE name=?`, name).Scan(&n); err != nil {
		return "", err
	}
	return fmt.Sprintf("B-%d-%04d", year, n), nil
}

// MaxExpenseAttachment bounds a photo or PDF stored with a receipt.
const MaxExpenseAttachment = 10 << 20

var expenseAttachmentTypes = map[string]bool{"application/pdf": true, "image/jpeg": true, "image/png": true, "image/heic": true}

// Receipts without VAT from a supplier whose tax the business owes itself
// (§ 13b UStG), split by where the supplier is based.
const (
	RateReverseChargeEU    = "reverse-charge-eu"
	RateReverseChargeOther = "reverse-charge"
)

// ReverseChargeRate reports a receipt on which the business owes the tax.
func ReverseChargeRate(rate string) bool {
	return rate == RateReverseChargeEU || rate == RateReverseChargeOther
}

// RateMixed marks a receipt with 19 % and 7 % items, e.g. a restaurant bill
// with drinks and food (food at 7 % since 2026) or a supermarket receipt.
const RateMixed = "mixed"

// SplitMixed returns net and tax of a gross amount of which reduced is taxed
// at 7 % and the rest at 19 %.
func SplitMixed(gross, reduced decimal.Decimal) (decimal.Decimal, decimal.Decimal) {
	net19, tax19, _ := SplitGross(gross.Sub(reduced), "19")
	net7, tax7, _ := SplitGross(reduced, "7")
	return net19.Add(net7), tax19.Add(tax7)
}

// SplitGross returns net and tax of a gross amount at rate ("none" = no VAT).
func SplitGross(gross decimal.Decimal, rate string) (decimal.Decimal, decimal.Decimal, error) {
	switch rate {
	case "none", "0", RateReverseChargeEU, RateReverseChargeOther:
		return gross, decimal.Zero, nil
	case "19", "7":
		pct := decimal.RequireFromString(rate)
		net := gross.Mul(decimal.NewFromInt(100)).Div(decimal.NewFromInt(100).Add(pct)).Round(2)
		return net, gross.Sub(net), nil
	}
	return decimal.Zero, decimal.Zero, errors.New("Wähle einen gültigen Steuersatz.")
}

func validDate(s string) bool {
	_, err := time.Parse(time.DateOnly, s)
	return err == nil
}

// SaveExpense validates and stores a receipt. validCategory checks the
// category against the form mapping, which the store does not know.
func (s *Store) SaveExpense(e Expense, validCategory func(string) bool, asset string) (Expense, error) {
	e.Supplier, e.Note = strings.TrimSpace(e.Supplier), strings.TrimSpace(e.Note)
	switch {
	case !validDate(e.Date):
		return e, errors.New("Gib das Belegdatum an.")
	case e.PaidDate != "" && !validDate(e.PaidDate):
		return e, errors.New("Gib ein gültiges Zahlungsdatum an.")
	case e.Supplier == "" || len(e.Supplier) > 200 || len(e.Note) > 2000:
		return e, errors.New("Gib an, bei wem du gekauft hast.")
	case !validCategory(e.Category):
		return e, errors.New("Wähle aus, wofür die Ausgabe war.")
	case e.PaidWith != "bank" && e.PaidWith != "cash" && e.PaidWith != "private":
		return e, errors.New("Gib an, wie du bezahlt hast.")
	}
	gross, err := decimal.NewFromString(strings.ReplaceAll(strings.TrimSpace(e.Gross), ",", "."))
	if err != nil || !gross.IsPositive() || !gross.Equal(gross.Round(2)) {
		return e, errors.New("Gib den Bruttobetrag mit höchstens zwei Nachkommastellen ein.")
	}
	var net, tax decimal.Decimal
	if e.TaxRate == RateMixed {
		reduced, err := decimal.NewFromString(strings.ReplaceAll(strings.TrimSpace(e.ReducedGross), ",", "."))
		if err != nil || !reduced.IsPositive() || !reduced.LessThan(gross) || !reduced.Equal(reduced.Round(2)) {
			return e, errors.New("Gib an, welcher Teil des Betrags mit 7 % besteuert ist. Er muss kleiner als der Gesamtbetrag sein.")
		}
		net, tax = SplitMixed(gross, reduced)
		e.ReducedGross = reduced.StringFixed(2)
	} else {
		if net, tax, err = SplitGross(gross, e.TaxRate); err != nil {
			return e, err
		}
		e.ReducedGross = ""
	}
	e.Gross, e.Net, e.Tax = gross.StringFixed(2), net.StringFixed(2), tax.StringFixed(2)
	if e.Category == asset {
		if e.Years < 1 || e.Years > 50 {
			return e, errors.New("Gib die Nutzungsdauer in Jahren an (1 bis 50).")
		}
	} else {
		e.Years = 0
	}
	if err = normalizeCurrency(&e, gross); err != nil {
		return e, err
	}
	if len(e.DocumentNumber) > 100 {
		return e, errors.New("Die Belegnummer darf höchstens 100 Zeichen lang sein.")
	}
	if len(e.Attachment) > MaxExpenseAttachment {
		return e, errors.New("Die Datei ist größer als 10 MB.")
	}
	if len(e.Attachment) > 0 && !expenseAttachmentTypes[e.AttachmentType] {
		return e, errors.New("Hänge ein PDF oder Foto an (JPG, PNG, HEIC).")
	}
	tx, err := s.db.Begin()
	if err != nil {
		return e, err
	}
	defer tx.Rollback()
	existing, err := readExpenses(tx)
	if err != nil {
		return e, err
	}
	var previous *Expense
	for i := range existing {
		other := existing[i]
		if other.ID == e.ID {
			previous = &existing[i]
			continue
		}
		if other.CancelledAt != "" {
			continue
		}
		if e.BookingID != "" && other.BookingID == e.BookingID {
			return e, errors.New("Für diesen Bankumsatz ist bereits ein Beleg erfasst.")
		}
		if e.ReceivedID != "" && other.ReceivedID == e.ReceivedID {
			return e, errors.New("Für dieses Dokument sind bereits Beträge erfasst.")
		}
	}
	if e.BookingID != "" {
		payments, err := readPayments(tx)
		if err != nil {
			return e, err
		}
		for _, p := range payments {
			if p.BookingID == e.BookingID && p.RevokedAt == "" {
				return e, errors.New("Dieser Bankumsatz ist bereits einer Rechnung zugeordnet.")
			}
		}
	}
	var before []byte
	switch {
	case e.ID == "":
		e.ID, e.CreatedAt = newID("exp"), now()
		if e.ReceivedID != "" {
			err = tx.QueryRow(`SELECT COALESCE(json_extract(data,'$.receiptNumber'),'') FROM received_invoices WHERE id=?`, e.ReceivedID).Scan(&e.Number)
			if err != nil && !errors.Is(err, sql.ErrNoRows) {
				return e, err
			}
		}
		if e.Number == "" {
			if e.Number, err = nextReceiptNumber(tx); err != nil {
				return e, err
			}
		}
	case previous == nil:
		return e, errors.New("Der Beleg wurde nicht gefunden.")
	default:
		if previous.CancelledAt != "" {
			return e, errors.New("Ein stornierter Beleg lässt sich nicht mehr ändern. Erfasse ihn neu.")
		}
		e.CreatedAt, e.Number = previous.CreatedAt, previous.Number
		// Keep the stored file unless a new one is attached.
		if len(e.Attachment) == 0 && e.AttachmentName == previous.AttachmentName {
			e.AttachmentType, e.FileSum = previous.AttachmentType, previous.FileSum
		}
		rules, err := lockRules(tx)
		if err != nil {
			return e, err
		}
		why := lockedBy(rules, previous.CreatedAt, previous.Date, previous.PaidDate)
		if onlyAddsPayment(*previous, e) {
			why = lockedBy(rules, now(), e.PaidDate)
		}
		if why != "" && !onlyAddsFile(*previous, e) {
			return e, fmt.Errorf("Dieser Beleg ist festgeschrieben. %s Storniere ihn und erfasse ihn neu.", why)
		}
		if before, err = json.Marshal(*previous); err != nil {
			return e, err
		}
	}
	if len(e.Attachment) > 0 {
		if e.FileSum, err = saveExpenseFile(tx, e.ID, e.Attachment, now()); err != nil {
			return e, err
		}
	} else if e.AttachmentName == "" {
		e.AttachmentType, e.FileSum = "", ""
	}
	e.UpdatedAt = now()
	stored := e
	stored.Attachment, stored.Locked = nil, ""
	raw, err := json.Marshal(stored)
	if err != nil {
		return e, err
	}
	if before != nil && sameExpense(before, raw) {
		return e, tx.Commit()
	}
	if _, err = tx.Exec("INSERT INTO settings(key,value) VALUES(?,?) ON CONFLICT(key) DO UPDATE SET value=excluded.value", "expense:"+e.ID, raw); err != nil {
		return e, err
	}
	action := AuditCreated
	if before != nil {
		action = AuditChanged
	}
	if err = s.logChange(tx, AuditEntry{Action: action, Object: AuditExpense, ObjectID: e.ID, Label: expenseLabel(e), Before: string(before), After: string(raw), Sum: jsonSum(raw)}); err != nil {
		return e, err
	}
	return e, tx.Commit()
}

// sameExpense tells whether a save changed anything but the time of saving.
func sameExpense(before, after []byte) bool {
	var a, b Expense
	if json.Unmarshal(before, &a) != nil || json.Unmarshal(after, &b) != nil {
		return false
	}
	a.UpdatedAt, b.UpdatedAt = "", ""
	return stateJSON(a) == stateJSON(b)
}

// onlyAddsFile allows the one change a closed period can take: the missing file
// of a receipt whose amounts stay as they are.
func onlyAddsFile(before, after Expense) bool {
	if before.FileSum != "" || after.FileSum == "" && len(after.Attachment) == 0 {
		return false
	}
	before.AttachmentName, before.AttachmentType, before.FileSum = "", "", ""
	after.AttachmentName, after.AttachmentType, after.FileSum, after.Attachment = "", "", "", nil
	before.UpdatedAt, after.UpdatedAt, after.Locked = "", "", ""
	return stateJSON(before) == stateJSON(after)
}

// onlyAddsPayment allows paying a receipt from a closed period later: the
// Vorsteuer stays with the receipt date, the payment belongs to its own date.
func onlyAddsPayment(before, after Expense) bool {
	if before.PaidDate != "" || after.PaidDate == "" {
		return false
	}
	before.PaidDate, before.PaidWith, before.BookingID = "", "", ""
	after.PaidDate, after.PaidWith, after.BookingID = "", "", ""
	before.UpdatedAt, after.UpdatedAt, after.Locked, after.Attachment = "", "", "", nil
	return stateJSON(before) == stateJSON(after)
}

// normalizeCurrency keeps foreign-currency receipts complete (GoBD Rz. 77).
func normalizeCurrency(e *Expense, gross decimal.Decimal) error {
	e.Currency = strings.ToUpper(strings.TrimSpace(e.Currency))
	if e.Currency == "" || e.Currency == "EUR" {
		e.Currency, e.OriginalGross, e.ExchangeRate = "", "", ""
		return nil
	}
	if len(e.Currency) != 3 || strings.Trim(e.Currency, "ABCDEFGHIJKLMNOPQRSTUVWXYZ") != "" {
		return errors.New("Gib die Währung als dreistelliges Kürzel an, zum Beispiel USD.")
	}
	original, err := decimal.NewFromString(strings.ReplaceAll(strings.TrimSpace(e.OriginalGross), ",", "."))
	if err != nil || !original.IsPositive() {
		return errors.New("Gib den Betrag in der Originalwährung an.")
	}
	e.OriginalGross = original.String()
	e.ExchangeRate = original.DivRound(gross, 6).String()
	return nil
}

// saveExpenseFile adds a file to a receipt. Earlier files stay stored.
func saveExpenseFile(q sqlRunner, id string, data []byte, at string) (string, error) {
	sum := sumOf(data)
	_, err := q.Exec("INSERT OR IGNORE INTO expense_files(expense_id,sha256,data,added_at) VALUES(?,?,?,?)", id, sum, data, at)
	return sum, err
}

// upgradeExpenseFiles moves development databases to the keeping table, in which
// a receipt can hold several files.
func upgradeExpenseFiles(q sqlRunner) error {
	var columns int
	if err := q.QueryRow(`SELECT COUNT(*) FROM pragma_table_info('expense_files') WHERE name='added_at'`).Scan(&columns); err != nil {
		return err
	}
	if columns > 0 {
		_, err := q.Exec(expenseFileTriggers)
		return err
	}
	if _, err := q.Exec(`ALTER TABLE expense_files RENAME TO expense_files_old;` + expenseFileTable + `
INSERT INTO expense_files(expense_id,sha256,data,added_at) SELECT expense_id,sha256,data,'' FROM expense_files_old;
DROP TABLE expense_files_old;` + expenseFileTriggers); err != nil {
		return err
	}
	rows, err := q.Query(`SELECT expense_id,sha256 FROM expense_files`)
	if err != nil {
		return err
	}
	sums := map[string]string{}
	for rows.Next() {
		var id, sum string
		if err = rows.Scan(&id, &sum); err != nil {
			rows.Close()
			return err
		}
		sums[id] = sum
	}
	if err = rows.Close(); err != nil {
		return err
	}
	for id, sum := range sums {
		if _, err = q.Exec(`UPDATE settings SET value=CAST(json_set(CAST(value AS TEXT),'$.fileSum',?) AS BLOB) WHERE key=?`, sum, "expense:"+id); err != nil {
			return err
		}
	}
	return nil
}

// migrateExpenseFiles moves files out of expense rows, which older versions
// and older backups kept inline.
func migrateExpenseFiles(q sqlRunner) error {
	rows, err := q.Query(`SELECT key FROM settings WHERE key LIKE 'expense:%' AND json_type(CAST(value AS TEXT),'$.attachment') IS NOT NULL`)
	if err != nil {
		return err
	}
	var keys []string
	for rows.Next() {
		var key string
		if err = rows.Scan(&key); err != nil {
			rows.Close()
			return err
		}
		keys = append(keys, key)
	}
	if err = rows.Close(); err != nil {
		return err
	}
	for _, key := range keys {
		var raw []byte
		if err = q.QueryRow("SELECT value FROM settings WHERE key=?", key).Scan(&raw); err != nil {
			return err
		}
		var e Expense
		if err = json.Unmarshal(raw, &e); err != nil {
			return err
		}
		if len(e.Attachment) > 0 {
			if e.FileSum, err = saveExpenseFile(q, e.ID, e.Attachment, e.UpdatedAt); err != nil {
				return err
			}
		}
		e.Attachment = nil
		if raw, err = json.Marshal(e); err != nil {
			return err
		}
		if _, err = q.Exec("UPDATE settings SET value=? WHERE key=?", raw, key); err != nil {
			return err
		}
	}
	return nil
}

func readExpenses(q interface {
	Query(string, ...any) (*sql.Rows, error)
}) ([]Expense, error) {
	out := []Expense{}
	rows, err := q.Query("SELECT value FROM settings WHERE key LIKE 'expense:%' ORDER BY key")
	if err != nil {
		return out, err
	}
	defer rows.Close()
	for rows.Next() {
		var raw []byte
		var e Expense
		if err = rows.Scan(&raw); err != nil {
			return out, err
		}
		if err = json.Unmarshal(raw, &e); err != nil {
			return out, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// Expenses lists all receipts with cancelled ones, newest first; attachments
// stay in the store.
func (s *Store) Expenses() ([]Expense, error) {
	list, err := readExpenses(s.db)
	if err != nil {
		return list, err
	}
	rules, err := lockRules(s.db)
	if err != nil {
		return list, err
	}
	for i := range list {
		if list[i].CancelledAt == "" {
			list[i].Locked = lockedBy(rules, list[i].CreatedAt, list[i].Date, list[i].PaidDate)
		}
	}
	sortExpenses(list)
	if scope := s.scope.Load(); scope != nil {
		kept := make([]Expense, 0, len(list))
		for _, e := range list {
			if scope.covers(e.Date, e.PaidDate) {
				kept = append(kept, e)
			}
		}
		list = kept
	}
	return list, nil
}

// ActiveExpenses lists the receipts that count for taxes and reports.
func (s *Store) ActiveExpenses() ([]Expense, error) {
	list, err := s.Expenses()
	if err != nil {
		return list, err
	}
	active := make([]Expense, 0, len(list))
	for _, e := range list {
		if e.CancelledAt == "" {
			active = append(active, e)
		}
	}
	return active, nil
}

func sortExpenses(list []Expense) {
	for i := 1; i < len(list); i++ {
		for j := i; j > 0 && list[j].Date > list[j-1].Date; j-- {
			list[j], list[j-1] = list[j-1], list[j]
		}
	}
}

// ExpenseAttachment returns the stored file of a receipt.
func (s *Store) ExpenseAttachment(id string) (Expense, error) {
	var raw []byte
	if err := s.db.QueryRow("SELECT value FROM settings WHERE key=?", "expense:"+id).Scan(&raw); err != nil {
		return Expense{}, errors.New("Der Beleg wurde nicht gefunden.")
	}
	var e Expense
	if err := json.Unmarshal(raw, &e); err != nil {
		return e, err
	}
	if e.FileSum == "" {
		return e, nil
	}
	err := s.db.QueryRow("SELECT data FROM expense_files WHERE expense_id=? AND sha256=?", id, e.FileSum).Scan(&e.Attachment)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return e, err
	}
	return e, nil
}

// CancelExpense takes a receipt out of all figures. It stays visible with its
// reason, also in a closed period: the return of that period then needs a
// correction, which the tax view points out.
func (s *Store) CancelExpense(id, reason string) (Expense, error) {
	var e Expense
	reason = strings.TrimSpace(reason)
	if reason == "" {
		return e, errors.New("Gib an, warum du den Beleg stornierst.")
	}
	if len([]rune(reason)) > 500 {
		return e, errors.New("Fasse den Grund in höchstens 500 Zeichen zusammen.")
	}
	tx, err := s.db.Begin()
	if err != nil {
		return e, err
	}
	defer tx.Rollback()
	var before []byte
	if err = tx.QueryRow("SELECT value FROM settings WHERE key=?", "expense:"+id).Scan(&before); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return e, errors.New("Der Beleg wurde nicht gefunden.")
		}
		return e, err
	}
	if err = json.Unmarshal(before, &e); err != nil {
		return e, err
	}
	if e.CancelledAt != "" {
		return e, errors.New("Dieser Beleg ist bereits storniert.")
	}
	e.CancelledAt, e.CancelReason, e.UpdatedAt = now(), reason, now()
	raw, err := json.Marshal(e)
	if err != nil {
		return e, err
	}
	if _, err = tx.Exec("UPDATE settings SET value=? WHERE key=?", raw, "expense:"+id); err != nil {
		return e, err
	}
	if err = s.logChange(tx, AuditEntry{Action: AuditCancelled, Object: AuditExpense, ObjectID: id, Label: expenseLabel(e), Reason: reason, Before: string(before), After: string(raw), Sum: jsonSum(raw)}); err != nil {
		return e, err
	}
	return e, tx.Commit()
}

// ExpenseFiles lists all files of a receipt, oldest first.
func (s *Store) ExpenseFiles(id string) ([]ExpenseFile, error) {
	rows, err := s.db.Query(`SELECT sha256,length(data),added_at FROM expense_files WHERE expense_id=? ORDER BY added_at,sha256`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []ExpenseFile{}
	for rows.Next() {
		v := ExpenseFile{ExpenseID: id}
		if err = rows.Scan(&v.Sum, &v.Size, &v.AddedAt); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

// ExpenseFile is one stored version of a receipt's file.
type ExpenseFile struct {
	ExpenseID string `json:"expenseId"`
	Sum       string `json:"sum"`
	Size      int64  `json:"size"`
	AddedAt   string `json:"addedAt"`
}

// ReadExpenseFile returns the bytes of one version.
func (s *Store) ReadExpenseFile(id, sum string) ([]byte, error) {
	var data []byte
	err := s.db.QueryRow("SELECT data FROM expense_files WHERE expense_id=? AND sha256=?", id, sum).Scan(&data)
	return data, err
}

type fileCheck struct {
	Count    int
	Problems []AuditProblem
}

// verifyExpenseFiles reads every stored file once and compares it with its
// name. It also finds files that are gone: each one a receipt uses or used.
func verifyExpenseFiles(q sqlRunner, entries []AuditEntry) (fileCheck, error) {
	var result fileCheck
	rows, err := q.Query(`SELECT expense_id,sha256 FROM expense_files ORDER BY expense_id,sha256`)
	if err != nil {
		return result, err
	}
	type file struct{ id, sum string }
	var files []file
	stored := map[file]bool{}
	for rows.Next() {
		var f file
		if err = rows.Scan(&f.id, &f.sum); err != nil {
			rows.Close()
			return result, err
		}
		files = append(files, f)
		stored[f] = true
	}
	if err = rows.Close(); err != nil {
		return result, err
	}
	for _, f := range files {
		var data []byte
		if err = q.QueryRow("SELECT data FROM expense_files WHERE expense_id=? AND sha256=?", f.id, f.sum).Scan(&data); err != nil {
			return result, err
		}
		result.Count++
		if sumOf(data) != f.sum {
			result.Problems = append(result.Problems, AuditProblem{AuditExpense, f.id, "Belegdatei", "Die Datei wurde verändert."})
		}
	}
	used := map[file]string{}
	for _, e := range entries {
		if e.Object != AuditExpense || e.After == "" {
			continue
		}
		var row Expense
		if err = json.Unmarshal([]byte(e.After), &row); err != nil {
			return result, err
		}
		if row.FileSum != "" {
			used[file{row.ID, row.FileSum}] = expenseLabel(row)
		}
	}
	current, err := readExpenses(q)
	if err != nil {
		return result, err
	}
	for _, e := range current {
		if e.FileSum != "" {
			used[file{e.ID, e.FileSum}] = expenseLabel(e)
		}
	}
	for f, label := range used {
		if !stored[f] {
			result.Problems = append(result.Problems, AuditProblem{AuditExpense, f.id, label, "Eine Belegdatei fehlt."})
		}
	}
	return result, nil
}

// ReceivedCategory classifies a stored e-invoice for the Anlage EÜR.
type ReceivedCategory struct {
	ReceivedID string `json:"receivedId"`
	Category   string `json:"category"`
	Years      int    `json:"years,omitempty"`
}

func (s *Store) SaveReceivedCategory(c ReceivedCategory, validCategory func(string) bool, asset string) error {
	if !validCategory(c.Category) {
		return errors.New("Wähle aus, wofür die Ausgabe war.")
	}
	if c.Category == asset && (c.Years < 1 || c.Years > 50) {
		return errors.New("Gib die Nutzungsdauer in Jahren an (1 bis 50).")
	}
	if c.Category != asset {
		c.Years = 0
	}
	raw, err := json.Marshal(c)
	if err != nil {
		return err
	}
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err = refuseLockedReceived(tx, c.ReceivedID); err != nil {
		return err
	}
	key := "received-category:" + c.ReceivedID
	var before []byte
	err = tx.QueryRow("SELECT value FROM settings WHERE key=?", key).Scan(&before)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	if string(before) == string(raw) {
		return nil
	}
	if _, err = tx.Exec("INSERT INTO settings(key,value) VALUES(?,?) ON CONFLICT(key) DO UPDATE SET value=excluded.value", key, raw); err != nil {
		return err
	}
	action := AuditCreated
	if before != nil {
		action = AuditChanged
	}
	if err = s.logChange(tx, AuditEntry{Action: action, Object: AuditCategory, ObjectID: c.ReceivedID, Label: "Zuordnung eines Eingangsbelegs", Before: string(before), After: string(raw), Sum: jsonSum(raw)}); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) ReceivedCategories() (map[string]ReceivedCategory, error) {
	out := map[string]ReceivedCategory{}
	rows, err := s.db.Query("SELECT value FROM settings WHERE key LIKE 'received-category:%'")
	if err != nil {
		return out, err
	}
	defer rows.Close()
	for rows.Next() {
		var raw []byte
		var c ReceivedCategory
		if err = rows.Scan(&raw); err != nil {
			return out, err
		}
		if err = json.Unmarshal(raw, &c); err != nil {
			return out, err
		}
		out[c.ReceivedID] = c
	}
	return out, rows.Err()
}

// VATFiling records that a return was transmitted, so its deadline no longer
// shows as open. Payable is the result at that time: Kz 83 of a Voranmeldung,
// the total of a Zusammenfassende Meldung or the profit of the EÜR.
type VATFiling struct {
	PeriodID    string `json:"periodId"`
	SubmittedOn string `json:"submittedOn"`
	Payable     string `json:"payable"`
	// SavedAt is when the note was made. Receipts recorded up to then are closed.
	SavedAt string `json:"savedAt,omitempty"`
}

// Filing kinds keep the transmission notes of each return apart.
const (
	FilingVAT    = "vat-filing:"
	FilingZM     = "zm-filing:"
	FilingAnnual = "annual-filing:"
)

func (s *Store) SaveVATFiling(f VATFiling) error           { return s.SaveFiling(FilingVAT, f) }
func (s *Store) DeleteVATFiling(id string) error           { return s.DeleteFiling(FilingVAT, id) }
func (s *Store) VATFilings() (map[string]VATFiling, error) { return s.Filings(FilingVAT) }

func (s *Store) SaveFiling(kind string, f VATFiling) error {
	if !validDate(f.SubmittedOn) || strings.TrimSpace(f.PeriodID) == "" {
		return errors.New("Gib das Übermittlungsdatum an.")
	}
	f.SavedAt = now()
	raw, err := json.Marshal(f)
	if err != nil {
		return err
	}
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	key := kind + f.PeriodID
	var before []byte
	err = tx.QueryRow("SELECT value FROM settings WHERE key=?", key).Scan(&before)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	if _, err = tx.Exec("INSERT INTO settings(key,value) VALUES(?,?) ON CONFLICT(key) DO UPDATE SET value=excluded.value", key, raw); err != nil {
		return err
	}
	if err = s.logChange(tx, AuditEntry{Action: AuditFiled, Object: AuditFiling, ObjectID: key, Label: filingLabel(key), Before: string(before), After: string(raw), Sum: jsonSum(raw)}); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) DeleteFiling(kind, periodID string) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	key := kind + periodID
	var before []byte
	err = tx.QueryRow("SELECT value FROM settings WHERE key=?", key).Scan(&before)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	if _, err = tx.Exec("DELETE FROM settings WHERE key=?", key); err != nil {
		return err
	}
	if err = s.logChange(tx, AuditEntry{Action: AuditUnfiled, Object: AuditFiling, ObjectID: key, Label: filingLabel(key), Before: string(before), Sum: auditGone}); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) Filings(kind string) (map[string]VATFiling, error) {
	out := map[string]VATFiling{}
	rows, err := s.db.Query("SELECT value FROM settings WHERE key LIKE ?", kind+"%")
	if err != nil {
		return out, err
	}
	defer rows.Close()
	for rows.Next() {
		var raw []byte
		var f VATFiling
		if err = rows.Scan(&raw); err != nil {
			return out, err
		}
		if err = json.Unmarshal(raw, &f); err != nil {
			return out, err
		}
		out[f.PeriodID] = f
	}
	return out, rows.Err()
}
