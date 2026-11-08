package shops

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/shopspring/decimal"
	"papierleicht.de/e-rechnung-studio/internal/model"
)

// The legacy Webservice is available in PrestaShop 8/9. Dates are shop wall
// clock values without an offset. Scan from the fixed import date every time;
// a moving UTC checkpoint would miss edits and related payment/refund changes.
// Docs: https://devdocs.prestashop-project.org/9/webservice/resources/orders/
type prestaScalar string

func (s *prestaScalar) UnmarshalJSON(raw []byte) error {
	if string(raw) == "null" {
		*s = ""
		return nil
	}
	if len(raw) > 0 && raw[0] == '"' {
		var value string
		if err := json.Unmarshal(raw, &value); err != nil {
			return err
		}
		*s = prestaScalar(value)
		return nil
	}
	if string(raw) == "true" {
		*s = "1"
		return nil
	}
	if string(raw) == "false" {
		*s = "0"
		return nil
	}
	var value json.Number
	if err := json.Unmarshal(raw, &value); err != nil {
		return err
	}
	*s = prestaScalar(value.String())
	return nil
}

type prestaOrder struct {
	ID            prestaScalar `json:"id"`
	Reference     string       `json:"reference"`
	AddressID     prestaScalar `json:"id_address_invoice"`
	CustomerID    prestaScalar `json:"id_customer"`
	CurrencyID    prestaScalar `json:"id_currency"`
	StateID       prestaScalar `json:"current_state"`
	Created       string       `json:"date_add"`
	Updated       string       `json:"date_upd"`
	Gross         prestaScalar `json:"total_paid_tax_incl"`
	Net           prestaScalar `json:"total_paid_tax_excl"`
	PaidReal      prestaScalar `json:"total_paid_real"`
	ProductsNet   prestaScalar `json:"total_products"`
	ProductsGross prestaScalar `json:"total_products_wt"`
	DiscountNet   prestaScalar `json:"total_discounts_tax_excl"`
	DiscountGross prestaScalar `json:"total_discounts_tax_incl"`
	ShippingNet   prestaScalar `json:"total_shipping_tax_excl"`
	ShippingGross prestaScalar `json:"total_shipping_tax_incl"`
	ShippingRate  prestaScalar `json:"carrier_tax_rate"`
	WrappingNet   prestaScalar `json:"total_wrapping_tax_excl"`
	WrappingGross prestaScalar `json:"total_wrapping_tax_incl"`
	Associations  struct {
		Rows []struct {
			ID prestaScalar `json:"id"`
		} `json:"order_rows"`
	} `json:"associations"`
}

type prestaDetail struct {
	ID           prestaScalar `json:"id"`
	OrderID      prestaScalar `json:"id_order"`
	Name         string       `json:"product_name"`
	Reference    string       `json:"product_reference"`
	Quantity     prestaScalar `json:"product_quantity"`
	Returned     prestaScalar `json:"product_quantity_return"`
	Refunded     prestaScalar `json:"product_quantity_refunded"`
	RefundNet    prestaScalar `json:"total_refunded_tax_excl"`
	RefundGross  prestaScalar `json:"total_refunded_tax_incl"`
	Ecotax       prestaScalar `json:"ecotax"`
	Net          prestaScalar `json:"total_price_tax_excl"`
	Gross        prestaScalar `json:"total_price_tax_incl"`
	Associations struct {
		Taxes []struct {
			ID prestaScalar `json:"id"`
		} `json:"taxes"`
	} `json:"associations"`
}

type prestaAddress struct {
	ID         prestaScalar `json:"id"`
	First      string       `json:"firstname"`
	Last       string       `json:"lastname"`
	Company    string       `json:"company"`
	Street     string       `json:"address1"`
	Additional string       `json:"address2"`
	Postcode   string       `json:"postcode"`
	City       string       `json:"city"`
	CountryID  prestaScalar `json:"id_country"`
	VATID      string       `json:"vat_number"`
}

type prestaState struct {
	ID      prestaScalar    `json:"id"`
	Paid    prestaScalar    `json:"paid"`
	Deleted prestaScalar    `json:"deleted"`
	Name    json.RawMessage `json:"name"`
}

type prestaPayment struct {
	ID         prestaScalar `json:"id"`
	Reference  string       `json:"order_reference"`
	CurrencyID prestaScalar `json:"id_currency"`
	Amount     prestaScalar `json:"amount"`
}

func prestaID(value prestaScalar) bool {
	id, err := strconv.ParseUint(string(value), 10, 63)
	return err == nil && id > 0
}

func shopWallClock(value string) string {
	if parsed, err := time.Parse(time.RFC3339, value); err == nil {
		return parsed.Format(time.RFC3339)
	}
	if parsed, err := time.Parse("2006-01-02 15:04:05", value); err == nil {
		return parsed.Format(time.RFC3339)
	}
	return value
}

func beforeImportDate(created, importFrom string) bool {
	// Compare calendar dates in the source timezone, never the moving `since`.
	if len(created) < 10 || len(importFrom) != 10 {
		return false
	}
	_, sourceErr := time.Parse("2006-01-02", created[:10])
	_, importErr := time.Parse("2006-01-02", importFrom)
	return sourceErr == nil && importErr == nil && created[:10] < importFrom
}

func prestaEndpoint(conn Connection, resource string, query url.Values) string {
	query.Set("output_format", "JSON")
	return conn.URL + "/api/" + resource + "?" + query.Encode()
}

func prestaCollection[T any](ctx context.Context, c *Client, conn Connection, auth *Credentials, resource string, filter url.Values) ([]T, error) {
	out := []T{}
	previous := int64(0)
	key := resource
	if resource == "order_slip" {
		key = "order_slips"
	}
	for page := 0; page < maxPages; page++ {
		query := url.Values{"display": {"full"}, "sort": {"[id_ASC]"}, "limit": {fmt.Sprintf("%d,100", page*100)}}
		for key, values := range filter {
			query[key] = append([]string(nil), values...)
		}
		var body json.RawMessage
		if _, err := c.request(ctx, http.MethodGet, prestaEndpoint(conn, resource, query), nil, nil, auth, &body); err != nil {
			return nil, err
		}
		var batch []json.RawMessage
		// The Webservice answers an empty list with a bare [] instead of an envelope.
		if string(bytes.TrimSpace(body)) != "[]" {
			var envelope map[string]json.RawMessage
			if err := json.Unmarshal(body, &envelope); err != nil {
				return nil, errors.New("PrestaShop hat eine ungültige Ressourcenliste geliefert.")
			}
			raw, ok := envelope[key]
			if !ok {
				return nil, errors.New("PrestaShop hat eine unvollständige Ressourcenliste geliefert. Bitte Webservice-Leserechte prüfen.")
			}
			if err := json.Unmarshal(raw, &batch); err != nil {
				return nil, errors.New("PrestaShop hat eine ungültige Ressourcenliste geliefert.")
			}
		}
		for _, row := range batch {
			var identity struct {
				ID prestaScalar `json:"id"`
			}
			var value T
			if json.Unmarshal(row, &identity) != nil || json.Unmarshal(row, &value) != nil || !prestaID(identity.ID) {
				return nil, errors.New("PrestaShop hat eine Ressource ohne gültige Kennung geliefert.")
			}
			id, _ := strconv.ParseInt(string(identity.ID), 10, 64)
			if id <= previous {
				return nil, errors.New("Die PrestaShop-Seitenfolge hat sich geändert oder ist unvollständig. Bitte erneut abrufen.")
			}
			previous = id
			out = append(out, value)
		}
		if len(batch) < 100 {
			return out, nil
		}
	}
	return nil, errors.New("Der PrestaShop-Abruf überschreitet die Grenze von 100.000 Ressourcen. Bitte Importumfang prüfen.")
}

func prestaResource[T any](ctx context.Context, c *Client, conn Connection, auth *Credentials, cache map[string]json.RawMessage, resource, singular string, id prestaScalar) (T, error) {
	var result T
	if !prestaID(id) {
		return result, errors.New("Eine erforderliche PrestaShop-Ressourcenkennung fehlt.")
	}
	key := resource + "/" + string(id)
	raw, ok := cache[key]
	if !ok {
		var envelope map[string]json.RawMessage
		if _, err := c.request(ctx, http.MethodGet, prestaEndpoint(conn, key, url.Values{}), nil, nil, auth, &envelope); err != nil {
			return result, err
		}
		raw, ok = envelope[singular]
		if !ok {
			return result, errors.New("PrestaShop hat eine erforderliche Ressource nicht vollständig geliefert.")
		}
		var identity struct {
			ID prestaScalar `json:"id"`
		}
		if json.Unmarshal(raw, &identity) != nil || identity.ID != id {
			return result, errors.New("PrestaShop hat eine Ressource mit abweichender Kennung geliefert.")
		}
		cache[key] = raw
	}
	if json.Unmarshal(raw, &result) != nil {
		return result, errors.New("PrestaShop hat ungültige Ressourcendaten geliefert.")
	}
	return result, nil
}

func optionalShopMoney(o *Order, value prestaScalar) decimal.Decimal {
	if value == "" {
		return decimal.Zero
	}
	return parseMoney(o, string(value))
}

func sourceStateNeedsReview(raw json.RawMessage) bool {
	// State IDs and translations are merchant configurable; never infer payment
	// from numeric IDs or a shipped flag. Recognize explicit cancellation labels.
	text := strings.ToLower(string(raw))
	for _, word := range []string{"cancel", "storn", "refund", "erstatt", "annul", "rembours", "fehler", "error"} {
		if strings.Contains(text, word) {
			return true
		}
	}
	return false
}

func finishDiscountedShopOrder(o *Order, lines []normalizedLine, expectedTax, discountNet, discountGross decimal.Decimal) {
	if discountNet.IsZero() && discountGross.IsZero() {
		finishOrder(o, lines, expectedTax.String())
		return
	}
	rate := ""
	for _, line := range lines {
		if line.net == "0" {
			continue
		}
		lineRate, rateErr := boundedDecimal(line.rate)
		if !line.knownRate || rateErr != nil || (rate != "" && rate != lineRate.String()) {
			review(o, "Bestellrabatte mit mehreren oder uneindeutigen Steuersätzen müssen geprüft werden.")
			finishOrder(o, lines, expectedTax.String())
			return
		}
		rate = lineRate.String()
	}
	r, err := boundedDecimal(rate)
	if err != nil || !r.IsPositive() || discountNet.IsNegative() || discountGross.IsNegative() || !discountNet.Mul(r).Div(decimal.NewFromInt(100)).Round(2).Equal(discountGross.Sub(discountNet).Round(2)) {
		review(o, "Die Steueraufteilung des Bestellrabatts ist nicht eindeutig.")
		finishOrder(o, lines, expectedTax.String())
		return
	}
	// The source's tax-inclusive/exclusive allowance establishes its tax group;
	// finishOrder validates that allowance against all final source totals.
	o.Adjustments = append(o.Adjustments, model.InvoiceAdjustment{ID: "shop-discount", Amount: discountNet.StringFixed(2), Reason: "Shop-Rabatt", TaxCategory: "S", TaxRate: r.String()})
	finishOrder(o, lines, expectedTax.String())
}

func (c *Client) fetchPrestaShop(ctx context.Context, conn Connection, credentials Credentials, since time.Time) ([]Order, error) {
	_ = since // Related-resource updates may not touch orders.date_upd.
	auth := &Credentials{ConsumerKey: credentials.APIKey}
	filter := url.Values{}
	if _, err := time.Parse("2006-01-02", conn.ImportFrom); err == nil {
		// This fixed source-wallclock range is independent of the checkpoint.
		// DATETIME's maximum value also covers any shop timezone or clock skew.
		filter.Set("date", "1")
		filter.Set("filter[date_add]", "["+conn.ImportFrom+" 00:00:00,9999-12-31 23:59:59]")
	}
	rows, err := prestaCollection[prestaOrder](ctx, c, conn, auth, "orders", filter)
	if err != nil {
		return nil, err
	}
	cache := map[string]json.RawMessage{}
	references := map[string]int{}
	for _, row := range rows {
		references[row.Reference]++
	}
	out := []Order{}
	for _, raw := range rows {
		if beforeImportDate(raw.Created, conn.ImportFrom) {
			continue
		}
		if strings.TrimSpace(raw.Reference) == "" || len(raw.Reference) > 64 || strings.ContainsAny(raw.Reference, "[]|%,") {
			return nil, errors.New("PrestaShop hat keine eindeutige Bestellreferenz geliefert.")
		}
		o := Order{ExternalID: string(raw.ID), Number: raw.Reference, CustomerExternalID: string(raw.CustomerID), Gross: string(raw.Gross), CreatedAt: shopWallClock(raw.Created), UpdatedAt: shopWallClock(raw.Updated), Status: string(raw.StateID)}
		if references[raw.Reference] > 1 {
			review(&o, "Mehrere PrestaShop-Teilbestellungen teilen dieselbe Zahlungsreferenz. Bitte Zahlungen einzeln zuordnen.")
		}
		address, err := prestaResource[prestaAddress](ctx, c, conn, auth, cache, "addresses", "address", raw.AddressID)
		if err != nil {
			return nil, err
		}
		buyer, err := prestaResource[struct {
			Email string `json:"email"`
		}](ctx, c, conn, auth, cache, "customers", "customer", raw.CustomerID)
		if err != nil {
			return nil, err
		}
		currency, err := prestaResource[struct {
			ISO string `json:"iso_code"`
		}](ctx, c, conn, auth, cache, "currencies", "currency", raw.CurrencyID)
		if err != nil {
			return nil, err
		}
		country, err := prestaResource[struct {
			ISO string `json:"iso_code"`
		}](ctx, c, conn, auth, cache, "countries", "country", address.CountryID)
		if err != nil {
			return nil, err
		}
		state, err := prestaResource[prestaState](ctx, c, conn, auth, cache, "order_states", "order_state", raw.StateID)
		if err != nil {
			return nil, err
		}
		// From PrestaShop 8 a single record leaves out empty and "0" fields.
		if state.Paid == "" {
			state.Paid = "0"
		}
		o.Currency = strings.ToUpper(currency.ISO)
		o.Customer = customer(address.First, address.Last, address.Company, buyer.Email, address.Street, address.Additional, address.Postcode, address.City, country.ISO)
		o.Customer.VATID = address.VATID
		if state.Deleted == "1" || sourceStateNeedsReview(state.Name) {
			review(&o, "Die Bestellung ist storniert, erstattet oder hat einen problematischen Status. Bitte im Shop prüfen.")
		}
		payments, err := prestaCollection[prestaPayment](ctx, c, conn, auth, "order_payments", url.Values{"filter[order_reference]": {"[" + raw.Reference + "]"}})
		if err != nil {
			return nil, err
		}
		paid := decimal.Zero
		for _, payment := range payments {
			if payment.Reference != raw.Reference || payment.CurrencyID != raw.CurrencyID {
				review(&o, "Zahlungen lassen sich der Bestellung oder Währung nicht eindeutig zuordnen.")
				continue
			}
			paid = paid.Add(parseMoney(&o, string(payment.Amount)))
		}
		gross, paidReal := parseMoney(&o, o.Gross), optionalShopMoney(&o, raw.PaidReal)
		if raw.PaidReal == "" || (state.Paid != "0" && state.Paid != "1") {
			review(&o, "Die Zahlungsinformationen aus PrestaShop sind unvollständig.")
		}
		if !paid.Equal(paidReal) || (paid.IsPositive() && !paid.Equal(gross)) || (state.Paid == "1" && !paid.Equal(gross)) {
			review(&o, "Teilzahlungen oder abweichende Zahlungsangaben müssen im Shop geprüft werden.")
		}
		o.Paid = gross.IsPositive() && paid.Equal(gross) && paidReal.Equal(gross)
		slips, err := prestaCollection[struct {
			ID      prestaScalar `json:"id"`
			OrderID prestaScalar `json:"id_order"`
		}](ctx, c, conn, auth, "order_slip", url.Values{"filter[id_order]": {"[" + string(raw.ID) + "]"}})
		if err != nil {
			return nil, err
		}
		if len(slips) > 0 {
			review(&o, "Erstattungen und Gutschriften benötigen eine manuelle Prüfung.")
		}
		details, err := prestaCollection[prestaDetail](ctx, c, conn, auth, "order_details", url.Values{"filter[id_order]": {"[" + string(raw.ID) + "]"}})
		if err != nil {
			return nil, err
		}
		if len(raw.Associations.Rows) != len(details) {
			return nil, errors.New("Die PrestaShop-Bestellpositionen wurden nicht vollständig geliefert.")
		}
		expectedIDs := map[prestaScalar]bool{}
		for _, row := range raw.Associations.Rows {
			if !prestaID(row.ID) || expectedIDs[row.ID] {
				return nil, errors.New("PrestaShop hat ungültige Bestellpositionskennungen geliefert.")
			}
			expectedIDs[row.ID] = true
		}
		lines := []normalizedLine{}
		productNet, productGross := decimal.Zero, decimal.Zero
		for _, detail := range details {
			if detail.OrderID != raw.ID || !expectedIDs[detail.ID] {
				return nil, errors.New("PrestaShop hat nicht zugehörige Bestellpositionen geliefert.")
			}
			for _, value := range []prestaScalar{detail.Returned, detail.Refunded, detail.RefundNet, detail.RefundGross, detail.Ecotax} {
				if optionalShopMoney(&o, value).IsPositive() {
					review(&o, "Rücksendungen, Erstattungen oder Ökosteuern benötigen eine manuelle Prüfung.")
				}
			}
			net, inc := parseMoney(&o, string(detail.Net)), parseMoney(&o, string(detail.Gross))
			productNet, productGross = productNet.Add(net), productGross.Add(inc)
			rate, known := "0", false
			if len(detail.Associations.Taxes) == 1 {
				tax, err := prestaResource[struct {
					Rate prestaScalar `json:"rate"`
				}](ctx, c, conn, auth, cache, "taxes", "tax", detail.Associations.Taxes[0].ID)
				if err != nil {
					return nil, err
				}
				rate, known = string(tax.Rate), true
			} else if len(detail.Associations.Taxes) > 1 {
				review(&o, "Zusammengesetzte Steuern müssen geprüft werden.")
			}
			lines = append(lines, normalizedLine{id: string(detail.ID), name: detail.Name, description: detail.Reference, quantity: string(detail.Quantity), net: net.String(), tax: inc.Sub(net).String(), rate: rate, knownRate: known})
		}
		if !productNet.Equal(parseMoney(&o, string(raw.ProductsNet))) || !productGross.Equal(parseMoney(&o, string(raw.ProductsGross))) {
			review(&o, "Die Produktzwischensummen stimmen nicht mit den Bestellpositionen überein.")
		}
		shipNet, shipGross := optionalShopMoney(&o, raw.ShippingNet), optionalShopMoney(&o, raw.ShippingGross)
		if !shipNet.IsZero() || !shipGross.IsZero() {
			lines = append(lines, normalizedLine{id: "shipping", name: "Versand", quantity: "1", net: shipNet.String(), tax: shipGross.Sub(shipNet).String(), rate: string(raw.ShippingRate), knownRate: true})
		}
		if optionalShopMoney(&o, raw.WrappingNet).IsPositive() || optionalShopMoney(&o, raw.WrappingGross).IsPositive() {
			review(&o, "Geschenkverpackungen benötigen eine manuelle steuerliche Prüfung.")
		}
		net := parseMoney(&o, string(raw.Net))
		discountNet, discountGross := optionalShopMoney(&o, raw.DiscountNet), optionalShopMoney(&o, raw.DiscountGross)
		finishDiscountedShopOrder(&o, lines, gross.Sub(net), discountNet, discountGross)
		out = append(out, o)
	}
	return out, nil
}
