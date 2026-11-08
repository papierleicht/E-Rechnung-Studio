package app

import (
	"fmt"
	"strings"
	"time"

	"papierleicht.de/e-rechnung-studio/internal/model"
	"papierleicht.de/e-rechnung-studio/internal/store"
)

// NumberSeriesOption is a Nummernkreis the number dialog offers. Value is
// what the draft stores in NumberSeries to use it.
type NumberSeriesOption struct {
	Value   string           `json:"value"`
	Kind    string           `json:"kind"` // standard, customer or recent
	Label   string           `json:"label"`
	Series  string           `json:"series"`
	Pattern string           `json:"pattern"`
	Next    string           `json:"next"`
	Last    *store.NumberUse `json:"last,omitempty"`
}

// InvoiceNumbering describes the choices for the number of one draft.
type InvoiceNumbering struct {
	Options []NumberSeriesOption `json:"options"`
	// Selected is the Value of the effective option, or "manual".
	Selected       string `json:"selected"`
	CustomerID     string `json:"customerId"`
	CustomerName   string `json:"customerName"`
	CustomerSeries string `json:"customerSeries"`
}

// NumberCheck is the live check of a number the user types.
type NumberCheck struct {
	Number  string           `json:"number"`
	State   string           `json:"state"` // ok, invalid or taken
	Message string           `json:"message"`
	Hints   []string         `json:"hints"`
	Series  string           `json:"series"` // derived template, "" if none
	Pattern string           `json:"pattern"`
	Counter int64            `json:"counter"`
	Next    string           `json:"next"` // next free number of that series
	Last    *store.NumberUse `json:"last,omitempty"`
	// CanContinue: the series may count on from this number (it is free and
	// above every number already in the series).
	CanContinue bool `json:"canContinue"`
}

// NumberSeriesStart continues a series at Next (0 keeps its count) and makes
// it the customer's series, or the standard one without CustomerID.
type NumberSeriesStart struct {
	Series     string `json:"series"`
	IssueDate  string `json:"issueDate"`
	Next       int64  `json:"next"`
	CustomerID string `json:"customerId"`
}

// CustomerNumberSeries lists a customer with an own Nummernkreis.
type CustomerNumberSeries struct {
	CustomerID string `json:"customerId"`
	Name       string `json:"name"`
	Series     string `json:"series"`
	Pattern    string `json:"pattern"`
	Next       string `json:"next"`
}

// NumberSettings is the standard series and where it continues today.
// MinValue is the lowest counter still free; Next shows NextValue formatted.
type NumberSettings struct {
	Series    string                 `json:"series"`
	Pattern   string                 `json:"pattern"`
	Next      string                 `json:"next"`
	NextValue int64                  `json:"nextValue"`
	MinValue  int64                  `json:"minValue"`
	Last      *store.NumberUse       `json:"last,omitempty"`
	Customers []CustomerNumberSeries `json:"customers"`
}

func numberDate(value string) time.Time {
	if date, err := time.Parse(time.DateOnly, value); err == nil {
		return date
	}
	return time.Now()
}

func (s *InvoiceService) seriesOption(value, kind, label, series string, date time.Time) (NumberSeriesOption, error) {
	next, last, err := s.store.SeriesNext(series, date)
	return NumberSeriesOption{Value: value, Kind: kind, Label: label, Series: series, Pattern: model.SeriesPattern(series), Next: next, Last: last}, err
}

// InvoiceNumberOptions lists the standard series, the customer's own series
// and series of recent invoices with their next free numbers.
func (s *InvoiceService) InvoiceNumberOptions(d model.InvoiceDraft) (InvoiceNumbering, error) {
	date := numberDate(d.IssueDate)
	result := InvoiceNumbering{Options: []NumberSeriesOption{}, CustomerID: d.CustomerID, CustomerName: d.Customer.Name}
	standard, err := s.store.StandardSeries()
	if err != nil {
		return result, err
	}
	if result.CustomerSeries, err = s.store.CustomerSeries(d.CustomerID); err != nil {
		return result, err
	}
	standardValue := model.SeriesAuto
	if result.CustomerSeries != "" {
		standardValue = model.SeriesStandard
		option, err := s.seriesOption(model.SeriesAuto, "customer", fallbackText(d.Customer.Name, "Dieser Kunde"), result.CustomerSeries, date)
		if err != nil {
			return result, err
		}
		result.Options = append(result.Options, option)
	}
	option, err := s.seriesOption(standardValue, "standard", "Standard", standard, date)
	if err != nil {
		return result, err
	}
	result.Options = append([]NumberSeriesOption{option}, result.Options...)
	known := map[string]bool{standard: true, result.CustomerSeries: true}
	add := func(series string) error {
		if known[series] || model.ValidateSeries(series) != nil || len(result.Options) >= 5 {
			return nil
		}
		known[series] = true
		option, err := s.seriesOption(series, "recent", "Zuletzt verwendet", series, date)
		if err == nil {
			result.Options = append(result.Options, option)
		}
		return err
	}
	if d.AutomaticNumber && d.NumberSeries != model.SeriesAuto && d.NumberSeries != model.SeriesStandard {
		if err = add(d.NumberSeries); err != nil {
			return result, err
		}
	}
	recent, err := s.store.RecentNumbers(40)
	if err != nil {
		return result, err
	}
	for _, use := range recent {
		if series, ok := model.SeriesFromNumber(use.Number, numberDate(use.IssueDate)); ok {
			// A series with the invoice year continues in the draft's year.
			if err = add(series); err != nil {
				return result, err
			}
		}
	}
	result.Selected = "manual"
	if d.AutomaticNumber || d.Number == "" {
		result.Selected = d.NumberSeries
		if d.NumberSeries == model.SeriesStandard && result.CustomerSeries == "" {
			result.Selected = model.SeriesAuto
		}
	}
	return result, nil
}

// CheckInvoiceNumber explains whether number can be used on the draft and
// what it continues. Taken numbers block issuing; gaps only get a hint.
func (s *InvoiceService) CheckInvoiceNumber(d model.InvoiceDraft, number string) (NumberCheck, error) {
	number = strings.TrimSpace(number)
	check := NumberCheck{Number: number, State: "ok", Message: "Die Nummer ist frei.", Hints: []string{}}
	if err := model.ValidateInvoiceNumber(number); err != nil {
		check.State, check.Message = "invalid", err.Error()
		return check, nil
	}
	date := numberDate(d.IssueDate)
	uses, err := s.store.NumberUses(number, d.ID)
	if err != nil {
		return check, err
	}
	for _, use := range uses {
		if use.Draft {
			check.Hints = append(check.Hints, fmt.Sprintf("Der Entwurf an %s nutzt dieselbe Nummer. Sie geht an die Rechnung, die du zuerst ausstellst.", fallbackText(use.CustomerName, "einen Kunden")))
		} else if check.State == "ok" {
			check.State = "taken"
			if use.Deleted {
				check.Message = "Die Nummer gehörte zu einer gelöschten Rechnung. Sie bleibt gesperrt, damit jede Nummer einmalig ist."
			} else {
				check.Message = fmt.Sprintf("Die Nummer ist schon vergeben: Rechnung an %s vom %s.", fallbackText(use.CustomerName, "einen Kunden"), germanDate(use.IssueDate))
			}
		}
	}
	series, ok := model.SeriesFromNumber(number, date)
	if !ok {
		if strings.Trim(number, "0123456789") == number {
			check.Hints = append(check.Hints, "Die Nummer enthält keine Ziffer. Das ist erlaubt, solange sie einmalig ist. Weiterzählen kann die App so eine Nummer aber nicht.")
		}
		return check, nil
	}
	parts, err := model.ResolveSeries(series, date)
	if err != nil {
		return check, nil
	}
	check.Series, check.Pattern = series, model.SeriesPattern(series)
	check.Counter, _ = parts.Counter(number)
	if check.Next, check.Last, err = s.store.SeriesNext(series, date); err != nil {
		return check, err
	}
	if check.State != "ok" {
		return check, nil
	}
	if check.Last == nil {
		check.CanContinue = true
		check.Hints = append(check.Hints, fmt.Sprintf("Damit beginnt der Nummernkreis %s. Mehrere Kreise sind erlaubt, solange jede Nummer eindeutig bleibt.", check.Pattern))
		return check, nil
	}
	last, _ := parts.Counter(check.Last.Number)
	check.CanContinue = check.Counter > last
	switch {
	case check.Counter < last:
		check.Hints = append(check.Hints, fmt.Sprintf("In diesem Kreis ist schon %s vergeben. Neue Rechnungen sollten höhere Nummern bekommen, damit die Reihenfolge nachvollziehbar bleibt.", check.Last.Number))
	case check.Counter > last+1:
		check.Hints = append(check.Hints, fmt.Sprintf("Nach %s bleiben %d Nummern frei. Lücken sind erlaubt, du solltest sie aber erklären können, z. B. beim Wechsel aus einem anderen Programm.", check.Last.Number, check.Counter-last-1))
	}
	return check, nil
}

// numberIssue blocks issuing a manual number that is invalid or taken.
func (s *InvoiceService) numberIssue(d model.InvoiceDraft) (*model.ValidationIssue, error) {
	if d.AutomaticNumber || d.Number == "" || (d.Status != "" && d.Status != "draft") {
		return nil, nil
	}
	issue := &model.ValidationIssue{Field: "number", Stage: "input", RuleID: "NUMBER", Severity: "error"}
	if err := model.ValidateInvoiceNumber(strings.TrimSpace(d.Number)); err != nil {
		issue.Message = err.Error()
		return issue, nil
	}
	use, err := s.store.NumberTaken(strings.TrimSpace(d.Number), d.ID)
	if err != nil || use == nil {
		return nil, err
	}
	issue.Message = store.TakenError(*use).Error()
	return issue, nil
}

// StartNumberSeries continues a series at a chosen number for one customer
// or as the standard series.
func (s *InvoiceService) StartNumberSeries(start NumberSeriesStart) error {
	start.Series = strings.TrimSpace(start.Series)
	if err := model.ValidateSeries(start.Series); err != nil {
		return err
	}
	if start.Next > 0 {
		if err := s.store.StartSeries(start.Series, numberDate(start.IssueDate), start.Next); err != nil {
			return err
		}
	}
	if start.CustomerID != "" {
		return s.store.SetCustomerSeries(start.CustomerID, start.Series)
	}
	return s.store.SaveStandardSeries(start.Series)
}

// RemoveCustomerNumberSeries returns a customer to the standard series.
func (s *InvoiceService) RemoveCustomerNumberSeries(customerID string) error {
	return s.store.SetCustomerSeries(customerID, "")
}

// PreviewNumberSeries shows the number a series would give today, continued
// at next (0: where it stands now).
func (s *InvoiceService) PreviewNumberSeries(series string, next int64) (NumberSettings, error) {
	series = strings.TrimSpace(series)
	result := NumberSettings{Series: series, Pattern: model.SeriesPattern(series), Customers: []CustomerNumberSeries{}}
	if err := model.ValidateSeries(series); err != nil {
		return result, err
	}
	today := time.Now()
	natural, last, err := s.store.SeriesNext(series, today)
	if err != nil {
		return result, err
	}
	parts, err := model.ResolveSeries(series, today)
	if err != nil {
		return result, err
	}
	result.Last = last
	result.MinValue, _ = parts.Counter(natural)
	result.NextValue = next
	if next <= 0 {
		result.NextValue = result.MinValue
	}
	result.Next = parts.Format(result.NextValue)
	if result.NextValue < result.MinValue {
		return result, fmt.Errorf("Die nächste Nummer muss mindestens %d sein. Bis %s ist dieser Nummernkreis schon belegt.", result.MinValue, parts.Format(result.MinValue-1))
	}
	return result, nil
}

func (s *InvoiceService) InvoiceNumberSettings() (NumberSettings, error) {
	series, err := s.store.StandardSeries()
	if err != nil {
		return NumberSettings{}, err
	}
	result, err := s.PreviewNumberSeries(series, 0)
	if err != nil {
		return result, err
	}
	customers, err := s.store.Customers()
	if err != nil {
		return result, err
	}
	for _, c := range customers {
		if c.InvoiceSettings == nil || c.InvoiceSettings.NumberSeries == "" {
			continue
		}
		entry := CustomerNumberSeries{CustomerID: c.ID, Name: c.Name, Series: c.InvoiceSettings.NumberSeries, Pattern: model.SeriesPattern(c.InvoiceSettings.NumberSeries)}
		entry.Next, _, _ = s.store.SeriesNext(entry.Series, time.Now())
		result.Customers = append(result.Customers, entry)
	}
	return result, nil
}

// SaveInvoiceNumberSettings changes the standard series and where it
// continues. A count never goes back to a number in use.
func (s *InvoiceService) SaveInvoiceNumberSettings(series string, next int64) (NumberSettings, error) {
	preview, err := s.PreviewNumberSeries(series, next)
	if err != nil {
		return preview, err
	}
	if preview.NextValue == preview.MinValue {
		next = 0
	}
	if err = s.StartNumberSeries(NumberSeriesStart{Series: preview.Series, IssueDate: time.Now().Format(time.DateOnly), Next: next}); err != nil {
		return preview, err
	}
	return s.InvoiceNumberSettings()
}

func fallbackText(value, other string) string {
	if strings.TrimSpace(value) == "" {
		return other
	}
	return value
}
