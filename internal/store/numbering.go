package store

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"papierleicht.de/e-rechnung-studio/internal/model"
	"strings"
	"time"
)

type numberReader interface {
	QueryRow(string, ...any) *sql.Row
	Query(string, ...any) (*sql.Rows, error)
}

const numberingKey = "invoice-numbering-v1"

// NumberUse is an invoice that holds a number. Issued and deleted invoices
// block the number for good; drafts only preview it.
type NumberUse struct {
	ID           string `json:"id"`
	Number       string `json:"number"`
	CustomerName string `json:"customerName"`
	IssueDate    string `json:"issueDate"`
	Draft        bool   `json:"draft"`
	Deleted      bool   `json:"deleted"`
}

func standardSeries(q numberReader) (string, error) {
	var raw []byte
	err := q.QueryRow("SELECT value FROM settings WHERE key=?", numberingKey).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return model.DefaultNumberSeries, nil
	}
	if err != nil {
		return "", err
	}
	var v struct{ Series string }
	if err = json.Unmarshal(raw, &v); err != nil || v.Series == "" {
		return model.DefaultNumberSeries, err
	}
	return v.Series, nil
}

// StandardSeries is the series of every invoice without a customer series.
func (s *Store) StandardSeries() (string, error) { return standardSeries(s.db) }

func (s *Store) SaveStandardSeries(template string) error {
	if err := model.ValidateSeries(template); err != nil {
		return err
	}
	raw, err := json.Marshal(map[string]string{"series": template})
	if err != nil {
		return err
	}
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err = s.writeTracked(tx, numberingKey, raw); err != nil {
		return err
	}
	return tx.Commit()
}

// CustomerSeries returns the customer's own series, or "".
func customerSeries(q numberReader, customerID string) (string, error) {
	if customerID == "" {
		return "", nil
	}
	var series string
	err := q.QueryRow(`SELECT COALESCE(json_extract(data,'$.invoiceSettings.numberSeries'),'') FROM customers WHERE id=?`, customerID).Scan(&series)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	return series, err
}

func (s *Store) CustomerSeries(customerID string) (string, error) {
	return customerSeries(s.db, customerID)
}

// SetCustomerSeries gives a customer an own series; "" returns it to the standard.
func (s *Store) SetCustomerSeries(customerID, template string) error {
	if template != "" {
		if err := model.ValidateSeries(template); err != nil {
			return err
		}
	}
	var raw []byte
	if err := s.db.QueryRow("SELECT data FROM customers WHERE id=?", customerID).Scan(&raw); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return errors.New("Speichere den Kunden zuerst, damit er einen eigenen Nummernkreis bekommen kann.")
		}
		return err
	}
	var c model.Customer
	if err := json.Unmarshal(raw, &c); err != nil {
		return err
	}
	if c.InvoiceSettings == nil {
		c.InvoiceSettings = &model.InvoiceSettings{}
	}
	c.InvoiceSettings.NumberSeries = template
	c.UpdatedAt = now()
	raw, err := encode(c)
	if err != nil {
		return err
	}
	_, err = s.db.Exec("UPDATE customers SET data=?,updated_at=? WHERE id=?", raw, c.UpdatedAt, c.ID)
	return err
}

// DraftSeries resolves the template of a draft's automatic number.
func draftSeries(q numberReader, d model.InvoiceDraft) (string, error) {
	switch d.NumberSeries {
	case model.SeriesStandard:
		return standardSeries(q)
	case model.SeriesAuto:
		series, err := customerSeries(q, d.CustomerID)
		if err != nil || series != "" {
			return series, err
		}
		return standardSeries(q)
	}
	return d.NumberSeries, nil
}

func (s *Store) DraftSeries(d model.InvoiceDraft) (string, error) { return draftSeries(s.db, d) }

// counterName keeps the historical counters of the original RE-JJJJ series.
func counterName(p model.SeriesParts) string {
	year := strings.TrimSuffix(strings.TrimPrefix(p.Prefix, "RE-"), "-")
	if p.Suffix == "" && len(p.Prefix) == 8 && p.Prefix == "RE-"+year+"-" && strings.Trim(year, "0123456789") == "" {
		return "invoice-" + year
	}
	return "invoice-series:" + strings.ToUpper(p.Prefix) + "{N}" + strings.ToUpper(p.Suffix)
}

func likeEscape(s string) string {
	return strings.NewReplacer(`\`, `\\`, "%", `\%`, "_", `\_`).Replace(s)
}

// SeriesState reads the highest counter of a series: the stored counter and
// every issued or deleted number in it. Last is the invoice with the highest
// number; drafts never reserve one.
func seriesState(q numberReader, p model.SeriesParts) (int64, *NumberUse, error) {
	var highest int64
	err := q.QueryRow(`SELECT COALESCE((SELECT value FROM counters WHERE name=?),0)`, counterName(p)).Scan(&highest)
	if err != nil {
		return 0, nil, err
	}
	pattern := likeEscape(p.Prefix) + "%" + likeEscape(p.Suffix)
	rows, err := q.Query(`SELECT id,number,customer_name,issue_date,0 FROM invoices WHERE status<>'draft' AND number LIKE ? ESCAPE '\'
UNION ALL SELECT id,number,customer_name,issue_date,1 FROM deleted_invoices WHERE number LIKE ? ESCAPE '\'`, pattern, pattern)
	if err != nil {
		return 0, nil, err
	}
	defer rows.Close()
	var last *NumberUse
	var lastValue int64 = -1
	for rows.Next() {
		var use NumberUse
		if err = rows.Scan(&use.ID, &use.Number, &use.CustomerName, &use.IssueDate, &use.Deleted); err != nil {
			return 0, nil, err
		}
		n, ok := p.Counter(use.Number)
		if !ok {
			// Digits beyond the int64 range cannot continue a series.
			if digits := use.Number[len(p.Prefix) : len(use.Number)-len(p.Suffix)]; len(digits) > 18 && strings.Trim(digits, "0123456789") == "" {
				return 0, nil, errors.New("Rechnungsnummer überschreitet den unterstützten Nummernbereich")
			}
			continue
		}
		if n > lastValue {
			lastValue, last = n, &use
		}
		if n > highest {
			highest = n
		}
	}
	return highest, last, rows.Err()
}

func nextInSeries(q numberReader, p model.SeriesParts) (int64, *NumberUse, error) {
	highest, last, err := seriesState(q, p)
	if err != nil {
		return 0, nil, err
	}
	if highest >= 1<<62 {
		return 0, nil, errors.New("Rechnungsnummernbereich ist ausgeschöpft")
	}
	return highest + 1, last, nil
}

// SeriesNext returns the next free number of a template on a date and the
// invoice that holds the highest number so far.
func (s *Store) SeriesNext(template string, date time.Time) (string, *NumberUse, error) {
	if err := model.ValidateSeries(template); err != nil {
		return "", nil, err
	}
	p, err := model.ResolveSeries(template, date)
	if err != nil {
		return "", nil, err
	}
	n, last, err := nextInSeries(s.db, p)
	if err != nil {
		return "", nil, err
	}
	number := p.Format(n)
	for use, err := numberTaken(s.db, number, ""); use != nil || err != nil; use, err = numberTaken(s.db, number, "") {
		if err != nil {
			return "", nil, err
		}
		n++
		number = p.Format(n)
	}
	return number, last, nil
}

// StartSeries lets a series continue at next, e.g. after switching from
// another program. It never goes back below a number already in use.
func (s *Store) StartSeries(template string, date time.Time, next int64) error {
	if err := model.ValidateSeries(template); err != nil {
		return err
	}
	p, err := model.ResolveSeries(template, date)
	if err != nil {
		return err
	}
	if next < 1 {
		return errors.New("Die nächste Nummer muss mindestens 1 sein.")
	}
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	highest, last, err := seriesState(tx, p)
	if err != nil {
		return err
	}
	if next <= highest {
		if last != nil && next <= mustCounter(p, last.Number) {
			return fmt.Errorf("%s ist schon vergeben. Die nächste Nummer muss höher sein, damit jede Nummer einmalig bleibt.", p.Format(next))
		}
		return fmt.Errorf("Dieser Nummernkreis ist schon bis %s gezählt. Die nächste Nummer muss höher sein.", p.Format(highest))
	}
	if _, err = tx.Exec(`INSERT INTO counters(name,value) VALUES(?,?) ON CONFLICT(name) DO UPDATE SET value=excluded.value`, counterName(p), next-1); err != nil {
		return err
	}
	if err = s.logChange(tx, AuditEntry{Action: AuditChanged, Object: AuditSetting, ObjectID: "series:" + template, Label: "Nummernkreis " + template, After: stateJSON(map[string]int64{"next": next})}); err != nil {
		return err
	}
	return tx.Commit()
}

func mustCounter(p model.SeriesParts, number string) int64 {
	n, _ := p.Counter(number)
	return n
}

// numberTaken finds an issued or deleted invoice other than id with number.
// Letters compare case-insensitively: RE-1 and re-1 are one number.
func numberTaken(q numberReader, number, id string) (*NumberUse, error) {
	var use NumberUse
	err := q.QueryRow(`SELECT id,number,customer_name,issue_date,0 FROM invoices WHERE status<>'draft' AND number=? COLLATE NOCASE AND id<>?
UNION ALL SELECT id,number,customer_name,issue_date,1 FROM deleted_invoices WHERE number=? COLLATE NOCASE LIMIT 1`, number, id, number).
		Scan(&use.ID, &use.Number, &use.CustomerName, &use.IssueDate, &use.Deleted)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &use, nil
}

// NumberTaken finds an issued or deleted invoice other than id with number.
func (s *Store) NumberTaken(number, id string) (*NumberUse, error) {
	return numberTaken(s.db, number, id)
}

// NumberUses lists every other invoice, draft or deleted invoice with number.
func (s *Store) NumberUses(number, id string) ([]NumberUse, error) {
	rows, err := s.db.Query(`SELECT id,number,customer_name,issue_date,status='draft',0 FROM invoices WHERE number=? COLLATE NOCASE AND id<>?
UNION ALL SELECT id,number,customer_name,issue_date,0,1 FROM deleted_invoices WHERE number=? COLLATE NOCASE`, number, id, number)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []NumberUse{}
	for rows.Next() {
		var use NumberUse
		if err = rows.Scan(&use.ID, &use.Number, &use.CustomerName, &use.IssueDate, &use.Draft, &use.Deleted); err != nil {
			return nil, err
		}
		out = append(out, use)
	}
	return out, rows.Err()
}

// RecentNumbers lists the numbers of the latest issued invoices, newest first.
func (s *Store) RecentNumbers(limit int) ([]NumberUse, error) {
	rows, err := s.db.Query(`SELECT id,number,customer_name,issue_date FROM invoices WHERE status<>'draft' AND number<>'' ORDER BY issue_date DESC, created_at DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []NumberUse{}
	for rows.Next() {
		var use NumberUse
		if err = rows.Scan(&use.ID, &use.Number, &use.CustomerName, &use.IssueDate); err != nil {
			return nil, err
		}
		out = append(out, use)
	}
	return out, rows.Err()
}

func applyNumber(d model.InvoiceDraft, number string) model.InvoiceDraft {
	if d.PaymentReference == "" || (d.AutomaticNumber && d.PaymentReference == d.Number) {
		d.PaymentReference = number
	}
	d.Number = number
	return d
}

// nextDraftNumber is the next free number in the draft's series.
func nextDraftNumber(q numberReader, d model.InvoiceDraft, date time.Time) (string, model.SeriesParts, int64, error) {
	template, err := draftSeries(q, d)
	if err != nil {
		return "", model.SeriesParts{}, 0, err
	}
	if err = model.ValidateSeries(template); err != nil {
		return "", model.SeriesParts{}, 0, fmt.Errorf("Der Nummernkreis %s ist ungültig: %w", model.SeriesPattern(template), err)
	}
	p, err := model.ResolveSeries(template, date)
	if err != nil {
		return "", p, 0, err
	}
	n, _, err := nextInSeries(q, p)
	if err != nil {
		return "", p, 0, err
	}
	// Another series may already have produced the same text.
	for {
		use, err := numberTaken(q, p.Format(n), d.ID)
		if err != nil {
			return "", p, 0, err
		}
		if use == nil {
			return p.Format(n), p, n, nil
		}
		n++
	}
}

// RefreshDraftNumber is read-only. Existing manually numbered and issued
// invoices retain their numbers; old empty drafts migrate to automatic mode.
func (s *Store) RefreshDraftNumber(d model.InvoiceDraft) (model.InvoiceDraft, error) {
	if d.Status != "" && d.Status != "draft" {
		return d, nil
	}
	if d.Number != "" && !d.AutomaticNumber {
		return applyNumber(d, d.Number), nil
	}
	date, err := time.Parse("2006-01-02", d.IssueDate)
	if err != nil {
		return d, nil
	} // Preserve the ordinary date validation in the editor.
	number, _, _, err := nextDraftNumber(s.db, d, date)
	if err != nil {
		return d, err
	}
	d = applyNumber(d, number)
	d.AutomaticNumber = true
	return d, nil
}

// assignNumber fixes the number inside the issuance transaction.
func assignNumber(tx *sql.Tx, id string, d model.InvoiceDraft, date time.Time) (model.InvoiceDraft, error) {
	if d.Number == "" || d.AutomaticNumber {
		number, p, n, err := nextDraftNumber(tx, d, date)
		if err != nil {
			return d, err
		}
		if _, err = tx.Exec(`INSERT INTO counters(name,value) VALUES(?,?) ON CONFLICT(name) DO UPDATE SET value=MAX(value,excluded.value)`, counterName(p), n); err != nil {
			return d, err
		}
		return applyNumber(d, number), nil
	}
	d.Number = strings.TrimSpace(d.Number)
	if err := model.ValidateInvoiceNumber(d.Number); err != nil {
		return d, err
	}
	use, err := numberTaken(tx, d.Number, id)
	if err != nil {
		return d, err
	}
	if use != nil {
		return d, TakenError(*use)
	}
	return applyNumber(d, d.Number), nil
}

// TakenError explains why a number cannot be issued again.
func TakenError(use NumberUse) error {
	if use.Deleted {
		return fmt.Errorf("Die Rechnungsnummer %s gehörte zu einer gelöschten Rechnung. Sie bleibt gesperrt, damit jede Nummer einmalig ist.", use.Number)
	}
	return fmt.Errorf("Die Rechnungsnummer %s ist schon an die Rechnung an %s vergeben. Jede Nummer darf nur einmal vorkommen.", use.Number, fallback(use.CustomerName, "einen Kunden"))
}

func fallback(value, other string) string {
	if strings.TrimSpace(value) == "" {
		return other
	}
	return value
}
