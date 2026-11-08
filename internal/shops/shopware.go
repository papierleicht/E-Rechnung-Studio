package shops

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/shopspring/decimal"
	"papierleicht.de/e-rechnung-studio/internal/model"
)

// Shopware 6.4.12+ exposes direct associations with Accept: application/json.
// Reference: developer.shopware.com/docs/guides/development/integrations-api/.
// Price semantics follow Shopware's CartPrice, CalculatedPrice and TaxCalculator:
// totalPrice is context-dependent on lines, but order.amountTotal is always gross.
const (
	shopwarePageSize         = 25
	shopwareLineLimit        = 101
	shopwareAssociationLimit = 11
)

type shopwareState struct {
	TechnicalName string `json:"technicalName"`
}
type shopwareTax struct {
	Tax   json.Number `json:"tax"`
	Rate  json.Number `json:"taxRate"`
	Price json.Number `json:"price"`
}
type shopwareTaxRule struct {
	Rate       json.Number `json:"taxRate"`
	Percentage json.Number `json:"percentage"`
}
type shopwarePrice struct {
	Total     json.Number       `json:"totalPrice"`
	Net       json.Number       `json:"netPrice"`
	Unit      json.Number       `json:"unitPrice"`
	Quantity  json.Number       `json:"quantity"`
	Taxes     []shopwareTax     `json:"calculatedTaxes"`
	Rules     []shopwareTaxRule `json:"taxRules"`
	TaxStatus string            `json:"taxStatus"`
}
type shopwareLine struct {
	ID          string         `json:"id"`
	ParentID    string         `json:"parentId"`
	Type        string         `json:"type"`
	Label       string         `json:"label"`
	Description string         `json:"description"`
	Quantity    json.Number    `json:"quantity"`
	Total       json.Number    `json:"totalPrice"`
	Price       *shopwarePrice `json:"price"`
}
type shopwareRefund struct {
	ID    string         `json:"id"`
	State *shopwareState `json:"stateMachineState"`
}
type shopwareCapture struct {
	ID      string           `json:"id"`
	Refunds []shopwareRefund `json:"refunds"`
}
type shopwareTransaction struct {
	ID       string            `json:"id"`
	Created  string            `json:"createdAt"`
	State    *shopwareState    `json:"stateMachineState"`
	Amount   *shopwarePrice    `json:"amount"`
	Captures []shopwareCapture `json:"captures"`
}
type shopwareDelivery struct {
	ID    string         `json:"id"`
	State *shopwareState `json:"stateMachineState"`
}
type shopwareBilling struct {
	First       string `json:"firstName"`
	Last        string `json:"lastName"`
	Company     string `json:"company"`
	Department  string `json:"department"`
	Street      string `json:"street"`
	Additional1 string `json:"additionalAddressLine1"`
	Additional2 string `json:"additionalAddressLine2"`
	Postcode    string `json:"zipcode"`
	City        string `json:"city"`
	VATID       string `json:"vatId"`
	Country     *struct {
		ISO string `json:"iso"`
	} `json:"country"`
	Region *struct {
		Name string `json:"name"`
	} `json:"countryState"`
}
type shopwareCustomer struct {
	ID         string   `json:"id"`
	CustomerID string   `json:"customerId"`
	Number     string   `json:"customerNumber"`
	Email      string   `json:"email"`
	VATIDs     []string `json:"vatIds"`
}
type shopwareOrder struct {
	ID        string         `json:"id"`
	Number    string         `json:"orderNumber"`
	Created   string         `json:"createdAt"`
	Updated   string         `json:"updatedAt"`
	Total     json.Number    `json:"amountTotal"`
	Net       json.Number    `json:"amountNet"`
	TaxStatus string         `json:"taxStatus"`
	Price     *shopwarePrice `json:"price"`
	Shipping  *shopwarePrice `json:"shippingCosts"`
	Currency  *struct {
		ISO string `json:"isoCode"`
	} `json:"currency"`
	Customer           *shopwareCustomer     `json:"orderCustomer"`
	Billing            *shopwareBilling      `json:"billingAddress"`
	State              *shopwareState        `json:"stateMachineState"`
	PrimaryTransaction string                `json:"primaryOrderTransactionId"`
	Transactions       []shopwareTransaction `json:"transactions"`
	Deliveries         []shopwareDelivery    `json:"deliveries"`
	Lines              []shopwareLine        `json:"lineItems"`
}

func shopwareCriteria(page int, since time.Time) map[string]any {
	stamp := since.UTC().Format(time.RFC3339Nano)
	filters := []any{}
	// A new order normally has updatedAt:null. Payment and refund writes can
	// update only their child entity, so those timestamps also participate.
	for _, field := range []string{"createdAt", "updatedAt", "transactions.createdAt", "transactions.updatedAt", "transactions.captures.createdAt", "transactions.captures.updatedAt", "transactions.captures.refunds.createdAt", "transactions.captures.refunds.updatedAt", "deliveries.createdAt", "deliveries.updatedAt"} {
		filters = append(filters, map[string]any{"type": "range", "field": field, "parameters": map[string]string{"gte": stamp}})
	}
	state := map[string]any{"stateMachineState": map[string]any{}}
	return map[string]any{
		"page": page, "limit": shopwarePageSize, "total-count-mode": 1,
		"sort":   []any{map[string]string{"field": "createdAt", "order": "ASC"}, map[string]string{"field": "id", "order": "ASC"}},
		"filter": []any{map[string]any{"type": "multi", "operator": "or", "queries": filters}},
		"associations": map[string]any{
			"currency": map[string]any{}, "orderCustomer": map[string]any{}, "stateMachineState": map[string]any{},
			"billingAddress": map[string]any{"associations": map[string]any{"country": map[string]any{}, "countryState": map[string]any{}}},
			"lineItems":      map[string]any{"limit": shopwareLineLimit, "sort": []any{map[string]string{"field": "position", "order": "ASC"}, map[string]string{"field": "id", "order": "ASC"}}},
			"deliveries":     map[string]any{"limit": shopwareAssociationLimit, "associations": state},
			"transactions": map[string]any{"limit": shopwareAssociationLimit, "sort": []any{map[string]string{"field": "createdAt", "order": "DESC"}, map[string]string{"field": "id", "order": "DESC"}}, "associations": map[string]any{
				"stateMachineState": map[string]any{},
				"captures":          map[string]any{"limit": shopwareAssociationLimit, "associations": map[string]any{"refunds": map[string]any{"limit": shopwareAssociationLimit, "associations": state}}},
			}},
		},
	}
}

func (c *Client) fetchShopware(ctx context.Context, conn Connection, credentials Credentials, since time.Time) ([]Order, error) {
	data, _ := json.Marshal(map[string]string{"grant_type": "client_credentials", "client_id": strings.TrimSpace(credentials.ClientID), "client_secret": credentials.ClientSecret})
	var auth struct {
		Token string `json:"access_token"`
		Type  string `json:"token_type"`
	}
	if _, err := c.request(ctx, http.MethodPost, conn.URL+"/api/oauth/token", data, map[string]string{"Content-Type": "application/json"}, nil, &auth); err != nil {
		return nil, err
	}
	if auth.Token == "" || len(auth.Token) > 8192 || strings.ContainsAny(auth.Token, "\r\n\x00") || (auth.Type != "" && !strings.EqualFold(auth.Type, "Bearer")) {
		return nil, errors.New("Shopware hat keinen gültigen Zugriffstoken ausgestellt. Bitte Zugangs-ID und Sicherheitsschlüssel der Integration prüfen.")
	}
	headers := map[string]string{"Content-Type": "application/json", "Authorization": "Bearer " + auth.Token}
	orders := []Order{}
	seen := map[string]bool{}
	expectedCount := -1
	for page := 1; page <= maxPages; page++ {
		body, _ := json.Marshal(shopwareCriteria(page, since))
		var result struct {
			Data   []shopwareOrder   `json:"data"`
			Total  *int              `json:"total"`
			Errors []json.RawMessage `json:"errors"`
		}
		if _, err := c.request(ctx, http.MethodPost, conn.URL+"/api/search/order", body, headers, nil, &result); err != nil {
			return nil, err
		}
		if len(result.Errors) > 0 || result.Total == nil || *result.Total < 0 || len(result.Data) > shopwarePageSize {
			return nil, errors.New("Shopware hat keine vollständige Bestellliste geliefert. Bitte Shopware-Version und Leserechte der Integration prüfen.")
		}
		if *result.Total > maxPages*shopwarePageSize {
			return nil, errors.New("Der Shopware-Import überschreitet 25.000 Bestellungen. Bitte einen späteren Importbeginn wählen.")
		}
		if expectedCount < 0 {
			expectedCount = *result.Total
		} else if expectedCount != *result.Total {
			return nil, errors.New("Die Shopware-Bestellliste hat sich während des Abrufs geändert. Bitte erneut synchronisieren.")
		}
		if len(orders)+len(result.Data) > *result.Total {
			return nil, errors.New("Die Shopware-Seitenfolge widerspricht der angegebenen Bestellanzahl.")
		}
		if len(result.Data) == 0 && len(orders) < *result.Total {
			return nil, errors.New("Die Shopware-Seitenfolge ist unvollständig. Es wurden keine Bestellungen übernommen.")
		}
		for _, raw := range result.Data {
			if raw.ID == "" || seen[raw.ID] {
				return nil, errors.New("Die Shopware-Seitenfolge enthält fehlende oder doppelte Kennungen. Bitte erneut synchronisieren.")
			}
			seen[raw.ID] = true
			orders = append(orders, normalizeShopware(raw))
		}
		if len(orders) >= *result.Total {
			return orders, nil
		}
	}
	return nil, errors.New("Der Shopware-Import überschreitet 25.000 Bestellungen. Bitte einen späteren Importbeginn wählen.")
}

func shopwareNumber(o *Order, n json.Number, money bool) decimal.Decimal {
	d, err := boundedDecimal(n.String())
	if err != nil {
		review(o, "Shopware enthält fehlende oder ungültige Mengen, Steuerwerte oder Geldbeträge.")
		return decimal.Zero
	}
	if money && !d.Equal(d.Round(2)) {
		review(o, "Shopware-Beträge mit mehr als zwei Nachkommastellen benötigen eine manuelle Prüfung.")
	}
	return d
}

type shopwarePart struct {
	net, tax, rate decimal.Decimal
	known          bool
}

func shopwareParts(o *Order, p *shopwarePrice, mode string, split bool) []shopwarePart {
	if p == nil {
		review(o, "Eine Shopware-Position enthält keine vollständige Preisberechnung.")
		return nil
	}
	total := shopwareNumber(o, p.Total, true)
	taxes, basis := decimal.Zero, decimal.Zero
	parts := []shopwarePart{}
	for _, t := range p.Taxes {
		tax, rate, price := shopwareNumber(o, t.Tax, true), shopwareNumber(o, t.Rate, false), shopwareNumber(o, t.Price, false)
		known := rate.GreaterThanOrEqual(decimal.Zero) && rate.LessThanOrEqual(decimal.NewFromInt(100))
		if !known {
			review(o, "Der Shopware-Steuersatz ist nicht eindeutig.")
		}
		net := price
		if mode == "gross" {
			net = net.Sub(tax)
		}
		parts = append(parts, shopwarePart{net: net.Round(2), tax: tax, rate: rate, known: known})
		taxes = taxes.Add(tax)
		basis = basis.Add(price)
	}
	net := total
	if mode == "gross" {
		net = net.Sub(taxes)
	}
	if len(parts) == 0 {
		rate := decimal.Zero
		if len(p.Rules) == 1 {
			rate = shopwareNumber(o, p.Rules[0].Rate, false)
		}
		return []shopwarePart{{net: net, tax: decimal.Zero, rate: rate, known: false}}
	}
	if len(parts) == 1 {
		if !basis.Round(2).Equal(total.Round(2)) {
			review(o, "Die Shopware-Steuerbasis passt nicht zur Positionssumme.")
		}
		parts[0].net = net
		return parts
	}
	if !split || !basis.Round(2).Equal(total.Round(2)) {
		review(o, "Mehrere oder zusammengesetzte Shopware-Steuern je Produktposition müssen geprüft werden.")
		return []shopwarePart{{net: net, tax: taxes, rate: decimal.Zero, known: false}}
	}
	netSum := decimal.Zero
	for _, part := range parts {
		netSum = netSum.Add(part.net)
	}
	if !netSum.Equal(net.Round(2)) {
		review(o, "Die Aufteilung der Shopware-Steuerbeträge enthält eine nicht darstellbare Rundungsdifferenz.")
	}
	return parts
}

func normalizeShopware(raw shopwareOrder) Order {
	o := Order{ExternalID: raw.ID, Number: raw.Number, CreatedAt: raw.Created, UpdatedAt: raw.Updated, Gross: raw.Total.String()}
	if o.UpdatedAt == "" {
		o.UpdatedAt = o.CreatedAt
	}
	if raw.Currency != nil {
		o.Currency = strings.ToUpper(raw.Currency.ISO)
	}
	if raw.State != nil {
		o.Status = raw.State.TechnicalName
	}
	switch o.Status {
	case "open", "in_progress", "completed":
	case "cancelled":
		review(&o, "Die Shopware-Bestellung wurde storniert.")
	default:
		review(&o, "Der Shopware-Bestellstatus benötigt eine manuelle Prüfung.")
	}
	if raw.Customer != nil {
		o.CustomerExternalID = raw.Customer.CustomerID
		if o.CustomerExternalID == "" {
			o.CustomerExternalID = raw.Customer.Number
		}
		if o.CustomerExternalID == "" {
			o.CustomerExternalID = raw.Customer.ID
		}
	}
	if b := raw.Billing; b != nil {
		email := ""
		if raw.Customer != nil {
			email = raw.Customer.Email
		}
		additional := addressAdditional(b.Additional1, b.Additional2)
		additional = addressAdditional(additional, b.Department)
		if b.Region != nil {
			additional = addressAdditional(additional, b.Region.Name)
		}
		country := ""
		if b.Country != nil {
			country = b.Country.ISO
		}
		o.Customer = customer(b.First, b.Last, b.Company, email, b.Street, additional, b.Postcode, b.City, country)
		o.Customer.VATID = strings.TrimSpace(b.VATID)
		if o.Customer.VATID == "" && raw.Customer != nil {
			if len(raw.Customer.VATIDs) == 1 {
				o.Customer.VATID = strings.TrimSpace(raw.Customer.VATIDs[0])
			} else if len(raw.Customer.VATIDs) > 1 {
				review(&o, "Mehrere Umsatzsteuer-IDs in der Shopware-Bestellung müssen zugeordnet werden.")
			}
		}
	}
	mode := strings.ToLower(raw.TaxStatus)
	if mode == "" && raw.Price != nil {
		mode = strings.ToLower(raw.Price.TaxStatus)
	}
	if mode != "gross" && mode != "net" {
		review(&o, "Der steuerfreie oder unbekannte Shopware-Steuerfall benötigt eine manuelle Prüfung.")
		if mode != "tax-free" {
			mode = "gross"
		}
	}
	if raw.Price != nil && raw.Price.TaxStatus != "" && raw.TaxStatus != "" && !strings.EqualFold(raw.Price.TaxStatus, raw.TaxStatus) {
		review(&o, "Die Shopware-Steuerkennzeichen widersprechen sich.")
	}
	if len(raw.Lines) >= shopwareLineLimit || len(raw.Transactions) >= shopwareAssociationLimit || len(raw.Deliveries) >= shopwareAssociationLimit {
		review(&o, "Die Shopware-Bestellung überschreitet das Positions- oder Transaktionslimit; vollständige Daten im Shop prüfen.")
	}
	lines := []normalizedLine{}
	sourceNet, sourceTax := decimal.Zero, decimal.Zero
	seenLines := map[string]bool{}
	for _, l := range raw.Lines {
		if l.ID == "" || seenLines[l.ID] {
			review(&o, "Shopware-Positionen haben fehlende oder doppelte Kennungen.")
		}
		seenLines[l.ID] = true
		if l.ParentID != "" || l.Type == "container" {
			review(&o, "Verschachtelte Shopware-Positionen oder Bundles benötigen eine manuelle Prüfung.")
		}
		switch l.Type {
		case "product", "custom", "promotion", "discount":
		case "credit":
			review(&o, "Shopware-Guthabenpositionen benötigen eine manuelle steuerliche Prüfung.")
		default:
			review(&o, "Ein besonderer Shopware-Positionstyp benötigt eine manuelle Prüfung.")
		}
		if l.Price != nil && l.Total != "" && !shopwareNumber(&o, l.Total, true).Equal(shopwareNumber(&o, l.Price.Total, true)) {
			review(&o, "Die Shopware-Positionssummen widersprechen sich.")
		}
		adjustment := l.Type == "promotion" || l.Type == "discount" || l.Type == "credit"
		parts := shopwareParts(&o, l.Price, mode, adjustment)
		quantity := shopwareNumber(&o, l.Quantity, false)
		if !quantity.IsPositive() {
			review(&o, "Eine Shopware-Position enthält eine ungültige Menge.")
			quantity = decimal.NewFromInt(1)
		}
		for i, part := range parts {
			sourceNet = sourceNet.Add(part.net)
			sourceTax = sourceTax.Add(part.tax)
			if !part.known && (!part.net.IsZero() || !part.tax.IsZero()) {
				review(&o, "Die steuerliche Behandlung einer Shopware-Position ist nicht eindeutig.")
			}
			if adjustment || part.net.IsNegative() {
				if !adjustment {
					review(&o, "Negative Shopware-Produktpositionen benötigen eine manuelle Prüfung.")
				}
				if !part.net.Mul(part.rate).Div(decimal.NewFromInt(100)).Round(2).Equal(part.tax.Round(2)) {
					review(&o, "Der Shopware-Rabatt lässt sich mit seinem Steuerbetrag nicht verlustfrei darstellen.")
				}
				category := "S"
				if part.rate.IsZero() {
					category = "Z"
				}
				if !part.net.IsZero() {
					o.Adjustments = append(o.Adjustments, model.InvoiceAdjustment{ID: fmt.Sprintf("%s-%d", l.ID, i+1), Charge: part.net.IsPositive(), Amount: part.net.Abs().String(), Reason: l.Label, TaxCategory: category, TaxRate: part.rate.String()})
				}
			} else {
				lines = append(lines, normalizedLine{id: l.ID, name: l.Label, description: l.Description, quantity: quantity.String(), net: part.net.String(), tax: part.tax.String(), rate: part.rate.String(), knownRate: part.known})
			}
		}
	}
	if raw.Shipping == nil {
		review(&o, "Die Shopware-Versandkosten fehlen.")
	} else {
		for i, part := range shopwareParts(&o, raw.Shipping, mode, true) {
			sourceNet = sourceNet.Add(part.net)
			sourceTax = sourceTax.Add(part.tax)
			if !part.net.IsZero() || !part.tax.IsZero() {
				if part.net.IsNegative() {
					review(&o, "Negative Shopware-Versandkosten müssen geprüft werden.")
				}
				lines = append(lines, normalizedLine{id: fmt.Sprintf("shipping-%d", i+1), name: "Versand", quantity: "1", net: part.net.String(), tax: part.tax.String(), rate: part.rate.String(), knownRate: part.known})
			}
		}
	}
	expectedTax := decimal.Zero
	if raw.Price == nil {
		review(&o, "Die vollständige Shopware-Bestellberechnung fehlt.")
	} else {
		for _, tax := range raw.Price.Taxes {
			expectedTax = expectedTax.Add(shopwareNumber(&o, tax.Tax, true))
		}
		if !shopwareNumber(&o, raw.Price.Total, true).Equal(shopwareNumber(&o, raw.Total, true)) {
			review(&o, "Die Shopware-Gesamtsummen widersprechen sich.")
		}
		if !shopwareNumber(&o, raw.Price.Net, true).Equal(sourceNet.Round(2)) {
			review(&o, "Die Shopware-Nettosumme weicht von den importierten Positionen und Rabatten ab.")
		}
	}
	if raw.Net != "" && !shopwareNumber(&o, raw.Net, true).Equal(sourceNet.Round(2)) {
		review(&o, "Die gespeicherte Shopware-Nettosumme weicht von den importierten Beträgen ab.")
	}
	if !sourceTax.Round(2).Equal(expectedTax.Round(2)) {
		review(&o, "Nicht alle Shopware-Steuerbeträge konnten vollständig zugeordnet werden.")
	}
	if !sourceNet.Add(sourceTax).Round(2).Equal(shopwareNumber(&o, raw.Total, true).Round(2)) {
		review(&o, "Die Shopware-Gesamtsumme weicht von Positionen, Versand und Rabatten ab.")
	}
	shopwarePayment(&o, raw)
	finishOrder(&o, lines, expectedTax.String())
	return o
}

func shopwarePayment(o *Order, raw shopwareOrder) {
	var selected *shopwareTransaction
	paidCount := 0
	financialReview := false
	for i := range raw.Transactions {
		t := &raw.Transactions[i]
		if t.State == nil {
			review(o, "Ein Shopware-Zahlungsstatus fehlt.")
			continue
		}
		state := t.State.TechnicalName
		switch state {
		case "paid":
			paidCount++
		case "open", "authorized", "in_progress", "reminded", "unconfirmed", "cancelled", "failed":
		case "paid_partially":
			review(o, "Shopware-Teilzahlungen benötigen eine manuelle Prüfung der bereits gezahlten Beträge.")
			financialReview = true
		case "refunded", "refunded_partially", "chargeback":
			review(o, "Shopware-Erstattungen oder Rückbuchungen benötigen eine manuelle Rechnungskorrektur.")
			financialReview = true
		default:
			review(o, "Ein besonderer Shopware-Zahlungsstatus benötigt eine manuelle Prüfung.")
			financialReview = true
		}
		if raw.PrimaryTransaction != "" && t.ID == raw.PrimaryTransaction {
			selected = t
		} else if raw.PrimaryTransaction == "" {
			if selected == nil {
				selected = t
			} else {
				created, err := time.Parse(time.RFC3339, t.Created)
				previous, previousErr := time.Parse(time.RFC3339, selected.Created)
				if err != nil || previousErr != nil || created.Equal(previous) {
					review(o, "Die Reihenfolge der Shopware-Zahlungsversuche ist nicht eindeutig.")
					financialReview = true
				} else if created.After(previous) {
					selected = t
				}
			}
		}
		if len(t.Captures) >= shopwareAssociationLimit {
			review(o, "Die Shopware-Zahlung überschreitet das Transaktionslimit; vollständige Zahlungsdaten im Shop prüfen.")
		}
		for _, capture := range t.Captures {
			if len(capture.Refunds) > 0 {
				review(o, "Shopware-Erstattungen benötigen eine manuelle Prüfung und gegebenenfalls eine Rechnungskorrektur.")
				financialReview = true
			}
			if len(capture.Refunds) >= shopwareAssociationLimit {
				review(o, "Die Shopware-Erstattungen überschreiten das Importlimit.")
			}
		}
	}
	if raw.PrimaryTransaction != "" && selected == nil {
		review(o, "Die primäre Shopware-Zahlung fehlt in den importierten Transaktionen.")
	}
	if paidCount > 1 {
		review(o, "Mehrere bezahlte Shopware-Transaktionen müssen auf Doppel- oder Teilzahlungen geprüft werden.")
		financialReview = true
	}
	if selected != nil && selected.State != nil && selected.State.TechnicalName == "paid" && !financialReview {
		if selected.Amount == nil || selected.Amount.Total == "" {
			review(o, "Der bezahlte Shopware-Transaktionsbetrag fehlt.")
			return
		}
		amount := shopwareNumber(o, selected.Amount.Total, true)
		gross := shopwareNumber(o, raw.Total, true)
		if !amount.Equal(gross) {
			review(o, "Der bezahlte Shopware-Betrag weicht von der Bestellsumme ab; Teilzahlungen prüfen.")
			return
		}
		o.Paid = true
	}
	for _, d := range raw.Deliveries {
		if d.State != nil && (d.State.TechnicalName == "returned" || d.State.TechnicalName == "returned_partially") {
			review(o, "Shopware-Retouren benötigen eine manuelle Prüfung.")
		}
	}
}
