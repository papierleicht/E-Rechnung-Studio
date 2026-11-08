package shops

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/shopspring/decimal"
)

// Orders remain a REST v2 resource in the current Gambio specification:
// https://developers.gambio.de/docs/API/v2/openapi.json
// `modified` returns change-history entries for a date, not complete orders
// changed since an instant. Use a full bounded scan from the fixed import date.
type gambioListOrder struct {
	ID            prestaScalar   `json:"id"`
	StatusID      prestaScalar   `json:"statusId"`
	StatusName    string         `json:"statusName"`
	PurchaseDate  string         `json:"purchaseDate"`
	WithdrawalIDs []prestaScalar `json:"withdrawalIds"`
}

type gambioOrder struct {
	ID           prestaScalar `json:"id"`
	StatusID     prestaScalar `json:"statusId"`
	PurchaseDate string       `json:"purchaseDate"`
	Currency     string       `json:"currencyCode"`
	Payment      struct {
		Module string `json:"module"`
	} `json:"paymentType"`
	Customer struct {
		ID    prestaScalar `json:"id"`
		Email string       `json:"email"`
		VATID string       `json:"vatId"`
	} `json:"customer"`
	Addresses struct {
		Billing struct {
			First      string       `json:"firstname"`
			Last       string       `json:"lastname"`
			Company    string       `json:"company"`
			Street     string       `json:"street"`
			House      string       `json:"houseNumber"`
			Additional string       `json:"additionalAddressInfo"`
			Suburb     string       `json:"suburb"`
			Postcode   string       `json:"postcode"`
			City       string       `json:"city"`
			CountryID  prestaScalar `json:"countryId"`
		} `json:"billing"`
	} `json:"addresses"`
	Items []struct {
		ID         prestaScalar `json:"id"`
		Name       string       `json:"name"`
		Model      string       `json:"model"`
		Quantity   prestaScalar `json:"quantity"`
		FinalPrice prestaScalar `json:"finalPrice"`
		Tax        prestaScalar `json:"tax"`
		TaxAllowed prestaScalar `json:"isTaxAllowed"`
	} `json:"items"`
	Totals []struct {
		Class string       `json:"class"`
		Value prestaScalar `json:"value"`
	} `json:"totals"`
	History []struct {
		Date string `json:"dateAdded"`
	} `json:"statusHistory"`
}

func gambioSingleton[T any](raw json.RawMessage) (T, error) {
	var result T
	if len(raw) > 0 && raw[0] == '[' {
		var rows []T
		if json.Unmarshal(raw, &rows) != nil || len(rows) != 1 {
			return result, errors.New("Gambio hat eine Ressource nicht eindeutig geliefert.")
		}
		return rows[0], nil
	}
	if len(raw) == 0 || raw[0] != '{' || json.Unmarshal(raw, &result) != nil {
		return result, errors.New("Gambio hat eine ungültige Ressource geliefert.")
	}
	return result, nil
}

func gambioUnpaidState(statusName, module string) bool {
	// IDs and status labels can be customized. Only explicit open-payment
	// labels with a conventional unpaid payment method are unambiguous here.
	status := strings.ToLower(strings.TrimSpace(statusName))
	switch status {
	case "offen", "pending", "unbezahlt", "unpaid", "awaiting payment", "zahlung ausstehend", "warten auf zahlung":
	default:
		return false
	}
	switch strings.ToLower(module) {
	case "invoice", "moneyorder", "banktransfer", "prepayment", "cod", "cash":
		return true
	default:
		return false
	}
}

func gambioAmount(o *Order, value prestaScalar) decimal.Decimal {
	amount, err := boundedDecimal(string(value))
	if err != nil || amount.IsNegative() {
		review(o, "Ungültige Gambio-Beträge müssen geprüft werden.")
		return decimal.Zero
	}
	return amount.Round(2)
}

func normalizeGambio(raw gambioOrder, list gambioListOrder, country string) Order {
	o := Order{ExternalID: string(raw.ID), Number: string(raw.ID), CustomerExternalID: string(raw.Customer.ID), Currency: strings.ToUpper(raw.Currency), CreatedAt: shopWallClock(raw.PurchaseDate), UpdatedAt: shopWallClock(raw.PurchaseDate), Status: string(raw.StatusID) + ": " + list.StatusName}
	for _, history := range raw.History {
		candidate := shopWallClock(history.Date)
		if candidate > o.UpdatedAt {
			o.UpdatedAt = candidate
		}
	}
	b := raw.Addresses.Billing
	o.Customer = customer(b.First, b.Last, b.Company, raw.Customer.Email, strings.TrimSpace(b.Street+" "+b.House), addressAdditional(b.Additional, b.Suburb), b.Postcode, b.City, country)
	o.Customer.VATID = raw.Customer.VATID
	if list.StatusID == "" || raw.StatusID != list.StatusID {
		review(&o, "Der Gambio-Bestellstatus hat sich während des Abrufs geändert oder fehlt. Bitte erneut abrufen.")
	}
	if sourceStateNeedsReview(json.RawMessage(strconv.Quote(list.StatusName))) || len(list.WithdrawalIDs) > 0 {
		review(&o, "Die Gambio-Bestellung ist storniert, erstattet oder widerrufen. Bitte im Shop prüfen.")
	}
	if !gambioUnpaidState(list.StatusName, raw.Payment.Module) {
		review(&o, "Gambio v2 meldet für diese Zahlungsart und diesen Status keinen eindeutigen Zahlungsbetrag. Zahlung und mögliche Erstattungen bitte im Shop prüfen.")
	}
	// Gambio's configurable status is not a payment receipt. In particular,
	// shipping and processing never imply that the order has been paid.
	o.Paid = false
	gross, taxTotal, shipping, discount := decimal.Zero, decimal.Zero, decimal.Zero, decimal.Zero
	hasGross, hasTax := false, false
	for _, total := range raw.Totals {
		switch total.Class {
		case "ot_total":
			if hasGross {
				review(&o, "Mehrere Gambio-Gesamtsummen müssen geprüft werden.")
			}
			gross = gambioAmount(&o, total.Value)
			hasGross = true
		case "ot_tax":
			taxTotal = taxTotal.Add(gambioAmount(&o, total.Value))
			hasTax = true
		case "ot_shipping":
			shipping = shipping.Add(gambioAmount(&o, total.Value))
		case "ot_discount":
			amount, err := boundedDecimal(string(total.Value))
			if err != nil {
				review(&o, "Der Gambio-Rabatt ist ungültig.")
			} else {
				discount = discount.Add(amount.Abs().Round(2))
			}
		case "ot_subtotal", "ot_subtotal_no_tax", "ot_total_net", "ot_total_netto":
			// Informational subtotals are independently checked against ot_total
			// and the aggregated source tax, never added a second time.
		default:
			amount, err := boundedDecimal(string(total.Value))
			if err != nil || !amount.IsZero() {
				review(&o, "Gutscheine, zusätzliche Gebühren oder besondere Gambio-Summen benötigen eine manuelle steuerliche Prüfung.")
			}
		}
	}
	if !hasGross || !hasTax {
		review(&o, "Die Gambio-Gesamt- oder Steuersumme fehlt.")
	}
	o.Gross = gross.StringFixed(2)
	// Whether finalPrice is gross or net depends on the customer-group price
	// display. Accept a representation only if every tax group and the final
	// source total reconcile; never guess from localized amount text.
	candidates := []Order{}
	for _, pricesIncludeTax := range []bool{true, false} {
		candidate := o
		candidate.ReviewReason = ""
		candidate.Lines, candidate.Adjustments = nil, nil
		lines := []normalizedLine{}
		shippingRate := ""
		for _, item := range raw.Items {
			rate, err := boundedDecimal(string(item.Tax))
			known := err == nil && rate.IsPositive() && rate.LessThanOrEqual(decimal.NewFromInt(100)) && (item.TaxAllowed == "1" || strings.EqualFold(string(item.TaxAllowed), "true"))
			if !known {
				review(&candidate, "Die Gambio-Steuerbehandlung einer Position ist nicht eindeutig.")
				rate = decimal.Zero
			}
			amount := gambioAmount(&candidate, item.FinalPrice)
			net := amount
			var tax decimal.Decimal
			if pricesIncludeTax {
				net = amount.Div(decimal.NewFromInt(1).Add(rate.Div(decimal.NewFromInt(100)))).Round(2)
				tax = amount.Sub(net)
			} else {
				tax = net.Mul(rate).Div(decimal.NewFromInt(100)).Round(2)
			}
			lines = append(lines, normalizedLine{id: string(item.ID), name: item.Name, description: item.Model, quantity: string(item.Quantity), net: net.String(), tax: tax.String(), rate: rate.String(), knownRate: known})
			if shippingRate == "" {
				shippingRate = rate.String()
			} else if shippingRate != rate.String() {
				shippingRate = "mixed"
			}
		}
		rate, rateErr := boundedDecimal(shippingRate)
		if shipping.IsPositive() {
			net, tax := shipping, decimal.Zero
			if rateErr == nil && rate.IsPositive() {
				if pricesIncludeTax {
					net = shipping.Div(decimal.NewFromInt(1).Add(rate.Div(decimal.NewFromInt(100)))).Round(2)
					tax = shipping.Sub(net)
				} else {
					tax = shipping.Mul(rate).Div(decimal.NewFromInt(100)).Round(2)
				}
			}
			lines = append(lines, normalizedLine{id: "shipping", name: "Versand", quantity: "1", net: net.String(), tax: tax.String(), rate: shippingRate, knownRate: rateErr == nil})
		}
		discountNet, discountGross := discount, discount
		if discount.IsPositive() && rateErr == nil && rate.IsPositive() {
			if pricesIncludeTax {
				discountNet = discount.Div(decimal.NewFromInt(1).Add(rate.Div(decimal.NewFromInt(100)))).Round(2)
			} else {
				discountGross = discount.Add(discount.Mul(rate).Div(decimal.NewFromInt(100)).Round(2))
			}
		}
		finishDiscountedShopOrder(&candidate, lines, taxTotal, discountNet, discountGross)
		if candidate.ReviewReason == "" {
			candidates = append(candidates, candidate)
		}
		// Keep an informative but blocked snapshot when neither interpretation
		// reconciles, so merchants can inspect the order rather than lose it.
		if pricesIncludeTax {
			o.Lines, o.Adjustments = candidate.Lines, candidate.Adjustments
		}
	}
	if len(candidates) == 1 {
		candidates[0].ReviewReason = o.ReviewReason
		return candidates[0]
	}
	if len(candidates) > 1 {
		review(&o, "Die Netto-/Bruttopreisbehandlung ist nicht eindeutig.")
	} else {
		review(&o, "Gambio-Positionen, Versand, Rabatte und Steuergruppen lassen sich nicht eindeutig mit den Bestellsummen abgleichen.")
	}
	return o
}

func (c *Client) fetchGambio(ctx context.Context, conn Connection, credentials Credentials, since time.Time) ([]Order, error) {
	_ = since
	auth := &Credentials{ConsumerKey: credentials.Username, ConsumerSecret: credentials.Password}
	lists := []gambioListOrder{}
	previous := int64(0)
	complete := false
	for page := 1; page <= maxPages; page++ {
		query := url.Values{"page": {strconv.Itoa(page)}, "per_page": {"100"}, "sort": {"+id"}}
		var batch []gambioListOrder
		headers, err := c.request(ctx, http.MethodGet, conn.URL+"/api.php/v2/orders?"+query.Encode(), nil, nil, auth, &batch)
		if err != nil {
			return nil, err
		}
		for _, row := range batch {
			id, err := strconv.ParseInt(string(row.ID), 10, 64)
			if err != nil || id <= previous {
				return nil, errors.New("Die Gambio-Seitenfolge hat sich geändert oder enthält ungültige Bestellkennungen. Bitte erneut abrufen.")
			}
			previous = id
			lists = append(lists, row)
		}
		hasNext := strings.Contains(headers.Get("Link"), `rel="next"`) || strings.Contains(headers.Get("Link"), "rel=next")
		if len(batch) == 0 && hasNext {
			return nil, errors.New("Gambio hat eine unvollständige Seitenfolge geliefert.")
		}
		if len(batch) < 100 && !hasNext {
			complete = true
			break
		}
	}
	if !complete {
		return nil, errors.New("Der vollständige Gambio-Bestellbestand erreicht die Abrufgrenze von 1.000 Seiten mit je 100 Bestellungen. Ein späterer Importbeginn verkleinert diese Liste nicht; bitte Shop-Bestand oder API-Einstellungen prüfen.")
	}
	countries := map[prestaScalar]string{}
	out := []Order{}
	for _, row := range lists {
		if beforeImportDate(row.PurchaseDate, conn.ImportFrom) {
			continue
		}
		var data json.RawMessage
		if _, err := c.request(ctx, http.MethodGet, conn.URL+"/api.php/v2/orders/"+string(row.ID), nil, nil, auth, &data); err != nil {
			return nil, err
		}
		raw, err := gambioSingleton[gambioOrder](data)
		if err != nil || raw.ID != row.ID {
			return nil, errors.New("Gambio hat eine Bestellung nicht vollständig oder mit abweichender Kennung geliefert.")
		}
		countryID := raw.Addresses.Billing.CountryID
		iso, ok := countries[countryID]
		if !ok {
			if !prestaID(countryID) {
				return nil, errors.New("Die Gambio-Rechnungsadresse enthält keine gültige Länderkennung.")
			}
			if _, err := c.request(ctx, http.MethodGet, conn.URL+"/api.php/v2/countries/"+string(countryID), nil, nil, auth, &data); err != nil {
				return nil, err
			}
			country, err := gambioSingleton[struct {
				ISO string `json:"iso2"`
			}](data)
			if err != nil {
				return nil, err
			}
			iso = country.ISO
			countries[countryID] = iso
		}
		out = append(out, normalizeGambio(raw, row, iso))
	}
	return out, nil
}
