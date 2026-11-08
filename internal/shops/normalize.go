package shops

import (
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/shopspring/decimal"
	"papierleicht.de/e-rechnung-studio/internal/model"
)

type normalizedLine struct {
	id, name, description, quantity, net, tax, rate string
	knownRate                                       bool
}

var plainDecimal = regexp.MustCompile(`^-?[0-9]+(\.[0-9]+)?$`)

func boundedDecimal(s string) (decimal.Decimal, error) {
	if len(s) > 64 || !plainDecimal.MatchString(s) {
		return decimal.Zero, errors.New("invalid decimal")
	}
	return decimal.NewFromString(s)
}

func review(o *Order, reason string) {
	if !strings.Contains(o.ReviewReason, reason) {
		if o.ReviewReason != "" {
			o.ReviewReason += " "
		}
		o.ReviewReason += reason
	}
}

func parseMoney(o *Order, s string) decimal.Decimal {
	d, err := boundedDecimal(s)
	if err != nil || d.IsNegative() {
		review(o, "Ungültige oder negative Beträge müssen geprüft werden.")
		return decimal.Zero
	}
	if !d.Equal(d.Round(2)) {
		review(o, "Beträge mit mehr als zwei Nachkommastellen benötigen eine manuelle Prüfung.")
	}
	return d
}

func finishOrder(o *Order, lines []normalizedLine, expectedTax string) {
	if len(o.Currency) != 3 {
		review(o, "Die Bestellwährung fehlt oder ist ungültig.")
	}
	a := o.Customer.Address
	if strings.TrimSpace(o.Customer.Name) == "" || strings.TrimSpace(a.Street) == "" || strings.TrimSpace(a.City) == "" || strings.TrimSpace(a.Postcode) == "" || len(a.Country) != 2 {
		review(o, "Die Rechnungsadresse ist unvollständig.")
	}
	for _, value := range []string{o.CreatedAt, o.UpdatedAt} {
		if _, err := time.Parse(time.RFC3339, value); err != nil {
			review(o, "Die Bestelldaten fehlen oder sind ungültig.")
		}
	}
	netSum, taxSum := decimal.Zero, decimal.Zero
	bases, taxes := map[string]decimal.Decimal{}, map[string]decimal.Decimal{}
	counts := map[string]int64{}
	for i, l := range lines {
		net, tax := parseMoney(o, l.net), parseMoney(o, l.tax)
		q, err := boundedDecimal(l.quantity)
		if err != nil || !q.IsPositive() {
			review(o, "Ungültige Mengen müssen geprüft werden.")
			q = decimal.NewFromInt(1)
		}
		rate, err := boundedDecimal(l.rate)
		if !l.knownRate && tax.IsZero() && (strings.TrimSpace(l.rate) == "" || (err == nil && rate.IsZero())) {
			// A line without any tax, e.g. from a shop under § 19 UStG. Whether
			// that fits is decided against the company settings, not here.
			rate = decimal.Zero
		} else if !l.knownRate || err != nil || rate.IsNegative() || rate.GreaterThan(decimal.NewFromInt(100)) {
			if !net.IsZero() || !tax.IsZero() {
				review(o, "Der Steuersatz ist nicht eindeutig; bitte steuerliche Behandlung prüfen.")
			}
			rate = decimal.Zero
		}
		category := "S"
		if rate.IsZero() {
			category = "Z"
		}
		id := l.id
		if id == "" {
			id = fmt.Sprintf("line-%d", i+1)
		}
		name := strings.TrimSpace(l.name)
		if name == "" {
			name = "Shop-Position"
			review(o, "Eine Positionsbezeichnung fehlt.")
		}
		price := net.DivRound(q, 16)
		if !price.Mul(q).Round(2).Equal(net.Round(2)) {
			review(o, "Die Positionssumme lässt sich nicht verlustfrei übernehmen.")
		}
		o.Lines = append(o.Lines, model.InvoiceLine{ID: id, Name: name, Description: l.description, Quantity: q.String(), Unit: "C62", UnitPrice: price.String(), TaxRate: rate.String(), TaxCategory: category, TaxTreatment: "standard"})
		netSum = netSum.Add(net.Round(2))
		taxSum = taxSum.Add(tax.Round(2))
		key := rate.String()
		bases[key] = bases[key].Add(net.Round(2))
		taxes[key] = taxes[key].Add(tax.Round(2))
		counts[key]++
	}
	for _, adjustment := range o.Adjustments {
		amount := parseMoney(o, adjustment.Amount)
		rate, err := boundedDecimal(adjustment.TaxRate)
		if err != nil || rate.IsNegative() || rate.GreaterThan(decimal.NewFromInt(100)) ||
			(adjustment.TaxCategory != "S" && adjustment.TaxCategory != "Z") {
			review(o, "Die Steuergruppe eines Rabatts oder Zuschlags ist ungültig.")
			continue
		}
		if !amount.IsPositive() || (rate.IsZero() != (adjustment.TaxCategory == "Z")) {
			review(o, "Rabatte und Zuschläge benötigen einen positiven Betrag und einen passenden Steuersatz.")
		}
		key := rate.String()
		if _, exists := bases[key]; !exists {
			review(o, "Ein Rabatt oder Zuschlag hat keine zugehörige Positionssteuergruppe.")
		}
		tax := amount.Mul(rate).Div(decimal.NewFromInt(100)).Round(2)
		if !adjustment.Charge {
			amount, tax = amount.Neg(), tax.Neg()
		}
		netSum, taxSum = netSum.Add(amount), taxSum.Add(tax)
		bases[key], taxes[key] = bases[key].Add(amount), taxes[key].Add(tax)
		counts[key]++
		if bases[key].IsNegative() {
			review(o, "Die Rabatte übersteigen den Betrag ihrer Steuergruppe.")
		}
	}
	if len(lines) == 0 {
		review(o, "Die Bestellung enthält keine Rechnungspositionen.")
	}
	o.SourceTaxes = nil
	for rate, base := range bases {
		r, _ := decimal.NewFromString(rate)
		difference := taxes[rate].Round(2).Sub(base.Mul(r).Div(decimal.NewFromInt(100)).Round(2))
		if difference.IsZero() {
			continue
		}
		// Shops round the tax of each line, the invoice that of each rate: up to
		// about 0.6 cents per line. The shop's amount then stays on the invoice.
		tolerance := decimal.NewFromInt(counts[rate]).Mul(decimal.RequireFromString("0.6")).Ceil().Div(decimal.NewFromInt(100))
		if r.IsPositive() && difference.Abs().LessThanOrEqual(tolerance) {
			o.SourceTaxes = append(o.SourceTaxes, model.SourceTax{Rate: r.String(), Basis: base.StringFixed(2), Amount: taxes[rate].StringFixed(2)})
			continue
		}
		review(o, "Die Steuerberechnung des Shops weicht von der Rechnungsberechnung ab.")
	}
	slices.SortFunc(o.SourceTaxes, func(a, b model.SourceTax) int { return strings.Compare(a.Rate, b.Rate) })
	// A free line without tax data joins the order's only rate, so the invoice
	// shows no zero-rated group for it.
	rates := map[string]bool{}
	for _, line := range o.Lines {
		if line.TaxCategory == "S" {
			rates[line.TaxRate] = true
		}
	}
	if len(rates) == 1 {
		for i := range o.Lines {
			line := &o.Lines[i]
			if price, err := decimal.NewFromString(line.UnitPrice); err == nil && price.IsZero() && line.TaxCategory == "Z" {
				for rate := range rates {
					line.TaxRate, line.TaxCategory = rate, "S"
				}
			}
		}
	}
	wantTax := parseMoney(o, expectedTax)
	if !taxSum.Round(2).Equal(wantTax.Round(2)) {
		review(o, "Nicht alle Steuerbeträge konnten eindeutig zugeordnet werden.")
	}
	gross := parseMoney(o, o.Gross)
	if !netSum.Add(taxSum).Round(2).Equal(gross.Round(2)) {
		review(o, "Die Bestellsumme weicht von der Summe der importierten Positionen ab; Gebühren, Rabatte oder Rundungen prüfen.")
	}
	if gross.Equal(gross.Round(2)) {
		o.Gross = gross.StringFixed(2)
	} else {
		o.Gross = gross.String()
	}
}

func customer(first, last, company, email, street, additional, postcode, city, country string) model.Customer {
	name := strings.TrimSpace(first + " " + last)
	contact := ""
	if strings.TrimSpace(company) != "" {
		contact = name
		name = strings.TrimSpace(company)
	}
	return model.Customer{Name: name, ContactName: contact, Email: strings.TrimSpace(email), Address: model.Address{Street: strings.TrimSpace(street), Additional: strings.TrimSpace(additional), Postcode: strings.TrimSpace(postcode), City: strings.TrimSpace(city), Country: strings.ToUpper(strings.TrimSpace(country))}}
}

func addressAdditional(line, region string) string {
	line, region = strings.TrimSpace(line), strings.TrimSpace(region)
	if region == "" {
		return line
	}
	if line == "" {
		return region
	}
	return line + ", " + region
}
