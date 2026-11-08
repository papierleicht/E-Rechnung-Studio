package shops

import (
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
)

type wooTaxAmount struct {
	ID    int64  `json:"id"`
	Total string `json:"total"`
}
type wooLine struct {
	ID          int64          `json:"id"`
	Name        string         `json:"name"`
	MethodTitle string         `json:"method_title"`
	SKU         string         `json:"sku"`
	Quantity    json.Number    `json:"quantity"`
	Total       string         `json:"total"`
	Tax         string         `json:"total_tax"`
	Taxes       []wooTaxAmount `json:"taxes"`
}
type wooTaxLine struct {
	RateID   int64           `json:"rate_id"`
	Percent  json.RawMessage `json:"rate_percent"`
	Compound bool            `json:"compound"`
}
type wooOrder struct {
	ID         int64   `json:"id"`
	Number     string  `json:"number"`
	CustomerID int64   `json:"customer_id"`
	Currency   string  `json:"currency"`
	Created    string  `json:"date_created_gmt"`
	Updated    string  `json:"date_modified_gmt"`
	Paid       *string `json:"date_paid_gmt"`
	Status     string  `json:"status"`
	Total      string  `json:"total"`
	Tax        string  `json:"total_tax"`
	Billing    struct {
		First      string `json:"first_name"`
		Last       string `json:"last_name"`
		Company    string `json:"company"`
		Street     string `json:"address_1"`
		Additional string `json:"address_2"`
		City       string `json:"city"`
		State      string `json:"state"`
		Postcode   string `json:"postcode"`
		Country    string `json:"country"`
		Email      string `json:"email"`
	} `json:"billing"`
	Lines    []wooLine         `json:"line_items"`
	Shipping []wooLine         `json:"shipping_lines"`
	Fees     []wooLine         `json:"fee_lines"`
	TaxLines []wooTaxLine      `json:"tax_lines"`
	Refunds  []json.RawMessage `json:"refunds"`
}
type wooRate struct {
	rate     string
	known    bool
	compound bool
}

func percent(raw json.RawMessage) string {
	if len(raw) == 0 || string(raw) == "null" {
		return ""
	}
	var s string
	if json.Unmarshal(raw, &s) == nil {
		return s
	}
	return string(raw)
}

func (c *Client) fetchWoo(ctx context.Context, conn Connection, credentials Credentials, since time.Time) ([]Order, error) {
	orders := []Order{}
	rates := map[int64]wooRate{}
	seen := map[int64]bool{}
	for page := 1; page <= maxPages; page++ {
		u, _ := url.Parse(conn.URL + "/wp-json/wc/v3/orders")
		// Woo's modified_after is exclusive. Subtract one second so the
		// Fetcher contract remains inclusive even for same-second updates.
		q := url.Values{"per_page": {"100"}, "page": {strconv.Itoa(page)}, "orderby": {"modified"}, "order": {"asc"}, "status": {"any"}, "dates_are_gmt": {"true"}, "modified_after": {since.UTC().Add(-time.Second).Format(time.RFC3339)}}
		u.RawQuery = q.Encode()
		var batch []wooOrder
		headers, err := c.request(ctx, http.MethodGet, u.String(), nil, nil, &credentials, &batch)
		if err != nil {
			return nil, err
		}
		pages := 0
		if value := headers.Get("X-WP-TotalPages"); value != "" {
			pages, err = strconv.Atoi(value)
			if err != nil || pages < 0 {
				return nil, errors.New("WooCommerce hat eine ungültige Seitenanzahl geliefert.")
			}
			if pages > maxPages {
				return nil, errors.New("Der WooCommerce-Import überschreitet 100.000 Bestellungen. Bitte einen späteren Importbeginn wählen.")
			}
		}
		if len(batch) == 0 && pages >= page {
			return nil, errors.New("Die WooCommerce-Seitenfolge ist unvollständig. Es wurden keine Bestellungen übernommen.")
		}
		for _, raw := range batch {
			if raw.ID <= 0 {
				return nil, errors.New("WooCommerce hat eine Bestellung ohne Kennung geliefert.")
			}
			if seen[raw.ID] {
				return nil, errors.New("Die WooCommerce-Seitenfolge hat sich während des Abrufs geändert. Bitte erneut synchronisieren.")
			}
			seen[raw.ID] = true
			orderRates := map[int64]wooRate{}
			for _, line := range raw.TaxLines {
				rate := percent(line.Percent)
				entry := wooRate{rate: rate, known: rate != "", compound: line.Compound}
				if rate == "" {
					var ok bool
					entry, ok = rates[line.RateID]
					if !ok {
						var result struct {
							Rate     string `json:"rate"`
							Compound bool   `json:"compound"`
						}
						_, rateErr := c.request(ctx, http.MethodGet, conn.URL+"/wp-json/wc/v3/taxes/"+strconv.FormatInt(line.RateID, 10), nil, nil, &credentials, &result)
						entry = wooRate{rate: result.Rate, known: rateErr == nil && result.Rate != "", compound: result.Compound}
						rates[line.RateID] = entry
					}
					entry.compound = entry.compound || line.Compound
				}
				orderRates[line.RateID] = entry
			}
			orders = append(orders, normalizeWoo(raw, orderRates))
		}
		if pages > 0 && page >= pages {
			return orders, nil
		}
		if len(batch) < 100 && pages == 0 {
			return orders, nil
		}
	}
	return nil, errors.New("Der WooCommerce-Import überschreitet 100.000 Bestellungen. Bitte einen späteren Importbeginn wählen.")
}

func wooDate(s string) string {
	if s == "" {
		return ""
	}
	for _, layout := range []string{time.RFC3339, "2006-01-02T15:04:05"} {
		if value, err := time.Parse(layout, s); err == nil {
			return value.UTC().Format(time.RFC3339)
		}
	}
	return s
}

func normalizeWoo(raw wooOrder, rates map[int64]wooRate) Order {
	b := raw.Billing
	o := Order{ExternalID: strconv.FormatInt(raw.ID, 10), Number: raw.Number, Currency: strings.ToUpper(raw.Currency), Gross: raw.Total, CreatedAt: wooDate(raw.Created), UpdatedAt: wooDate(raw.Updated), Status: raw.Status, Paid: raw.Paid != nil && *raw.Paid != ""}
	if o.Paid {
		o.PaidAt = wooDate(*raw.Paid)
	}
	if raw.CustomerID > 0 {
		o.CustomerExternalID = strconv.FormatInt(raw.CustomerID, 10)
	}
	o.Customer = customer(b.First, b.Last, b.Company, b.Email, b.Street, addressAdditional(b.Additional, b.State), b.Postcode, b.City, b.Country)
	switch raw.Status {
	case "pending", "processing", "on-hold", "completed", "cancelled", "refunded", "failed", "trash":
	default:
		review(&o, "Der besondere Bestellstatus benötigt eine manuelle Prüfung, insbesondere auf Anzahlungen oder Teilzahlungen.")
	}
	if raw.Status == "cancelled" || raw.Status == "failed" || raw.Status == "trash" {
		review(&o, "Die Bestellung ist storniert, fehlgeschlagen oder gelöscht.")
	}
	if len(raw.Refunds) > 0 || raw.Status == "refunded" {
		review(&o, "Erstattungen benötigen eine manuelle Prüfung und gegebenenfalls eine Rechnungskorrektur.")
	}
	lines := []normalizedLine{}
	add := func(l wooLine, kind string) {
		q := string(l.Quantity)
		name := l.Name
		if kind == "shipping" {
			q = "1"
			name = "Versand: " + l.MethodTitle
		}
		if kind == "fee" {
			q = "1"
		}
		rate := decimal.Zero
		known := len(l.Taxes) == 1
		for _, tax := range l.Taxes {
			entry, ok := rates[tax.ID]
			r, err := boundedDecimal(entry.rate)
			if !ok || !entry.known || err != nil || entry.compound {
				known = false
			} else {
				rate = rate.Add(r)
			}
			if entry.compound {
				review(&o, "Zusammengesetzte Steuern werden nicht automatisch übernommen.")
			}
		}
		if len(l.Taxes) > 1 {
			known = false
			review(&o, "Mehrere Steuern je Position benötigen eine manuelle Prüfung.")
		}
		lines = append(lines, normalizedLine{id: fmt.Sprintf("%s-%d", kind, l.ID), name: name, description: l.SKU, quantity: q, net: l.Total, tax: l.Tax, rate: rate.String(), knownRate: known})
	}
	for _, line := range raw.Lines {
		add(line, "item")
	}
	for _, line := range raw.Shipping {
		// Free shipping has no tax line; as an invoice line it would be zero-rated.
		if parseMoney(&o, line.Total).IsZero() && parseMoney(&o, line.Tax).IsZero() {
			continue
		}
		add(line, "shipping")
	}
	for _, line := range raw.Fees {
		add(line, "fee")
	}
	finishOrder(&o, lines, raw.Tax)
	return o
}
