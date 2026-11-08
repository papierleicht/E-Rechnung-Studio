package shops

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/shopspring/decimal"
)

type shopMoney struct {
	Amount   string `json:"amount"`
	Currency string `json:"currencyCode"`
}
type moneyBag struct {
	Presentment shopMoney `json:"presentmentMoney"`
}
type pageInfo struct {
	HasNext bool   `json:"hasNextPage"`
	End     string `json:"endCursor"`
}
type shopTax struct {
	Rate  json.Number `json:"rate"`
	Price moneyBag    `json:"priceSet"`
}
type shopLine struct {
	ID              string    `json:"id"`
	Name            string    `json:"name"`
	SKU             string    `json:"sku"`
	Quantity        int64     `json:"quantity"`
	CurrentQuantity int64     `json:"currentQuantity"`
	Net             moneyBag  `json:"priceAfterAllDiscountsBeforeTaxesSet"`
	TaxLines        []shopTax `json:"taxLines"`
	GiftCard        bool      `json:"isGiftCard"`
}
type shopShipping struct {
	ID      string    `json:"id"`
	Title   string    `json:"title"`
	Price   moneyBag  `json:"currentDiscountedPriceSet"`
	Taxes   []shopTax `json:"taxLines"`
	Removed bool      `json:"isRemoved"`
}
type shopOrder struct {
	ID            string   `json:"id"`
	Name          string   `json:"name"`
	Email         string   `json:"email"`
	Created       string   `json:"createdAt"`
	Updated       string   `json:"updatedAt"`
	Status        string   `json:"displayFinancialStatus"`
	Cancelled     *string  `json:"cancelledAt"`
	TaxesIncluded bool     `json:"taxesIncluded"`
	Total         moneyBag `json:"currentTotalPriceSet"`
	Tax           moneyBag `json:"currentTotalTaxSet"`
	Refunded      moneyBag `json:"totalRefundedSet"`
	Tips          moneyBag `json:"totalTipReceivedSet"`
	Customer      *struct {
		ID string `json:"id"`
	} `json:"customer"`
	Billing *struct {
		First      string `json:"firstName"`
		Last       string `json:"lastName"`
		Company    string `json:"company"`
		Street     string `json:"address1"`
		Additional string `json:"address2"`
		City       string `json:"city"`
		Province   string `json:"province"`
		Postcode   string `json:"zip"`
		Country    string `json:"countryCodeV2"`
	} `json:"billingAddress"`
	Lines struct {
		Nodes []shopLine `json:"nodes"`
		Page  pageInfo   `json:"pageInfo"`
	} `json:"lineItems"`
	Shipping struct {
		Nodes []shopShipping `json:"nodes"`
		Page  pageInfo       `json:"pageInfo"`
	} `json:"shippingLines"`
}

// Bounds all three connections. Oversized nested connections remain visible as
// review orders; no draft is generated from incomplete financial data.
const shopifyOrdersQuery = `query Orders($after: String, $search: String!) {
 currentAppInstallation { accessScopes { handle } }
 orders(first: 1, after: $after, sortKey: UPDATED_AT, query: $search) {
  pageInfo { hasNextPage endCursor }
  nodes {
   id name email createdAt updatedAt displayFinancialStatus cancelledAt taxesIncluded
   customer { id }
   billingAddress { firstName lastName company address1 address2 city province zip countryCodeV2 }
   currentTotalPriceSet { presentmentMoney { amount currencyCode } }
   currentTotalTaxSet { presentmentMoney { amount currencyCode } }
   totalRefundedSet { presentmentMoney { amount currencyCode } }
   totalTipReceivedSet { presentmentMoney { amount currencyCode } }
   lineItems(first: 100) { pageInfo { hasNextPage endCursor } nodes {
    id name sku quantity currentQuantity isGiftCard
    priceAfterAllDiscountsBeforeTaxesSet { presentmentMoney { amount currencyCode } }
    taxLines { rate priceSet { presentmentMoney { amount currencyCode } } }
   } }
   shippingLines(first: 10) { pageInfo { hasNextPage endCursor } nodes {
    id title isRemoved currentDiscountedPriceSet { presentmentMoney { amount currencyCode } }
    taxLines { rate priceSet { presentmentMoney { amount currencyCode } } }
   } }
  }
 }
}`

func (c *Client) fetchShopify(ctx context.Context, conn Connection, credentials Credentials, since time.Time) ([]Order, error) {
	token := strings.TrimSpace(credentials.AccessToken)
	if token == "" {
		form := url.Values{"grant_type": {"client_credentials"}, "client_id": {strings.TrimSpace(credentials.ClientID)}, "client_secret": {credentials.ClientSecret}}
		var result struct {
			Token string `json:"access_token"`
		}
		if _, err := c.request(ctx, http.MethodPost, conn.URL+"/admin/oauth/access_token", []byte(form.Encode()), map[string]string{"Content-Type": "application/x-www-form-urlencoded"}, nil, &result); err != nil {
			return nil, err
		}
		if result.Token == "" || len(result.Token) > 8192 || strings.ContainsAny(result.Token, "\r\n\x00") {
			return nil, errors.New("Shopify hat keinen gültigen Zugriffstoken ausgestellt. Bitte App-Installation und Organisationszuordnung prüfen.")
		}
		token = result.Token
	}
	search := "updated_at:>='" + since.UTC().Format(time.RFC3339) + "'"
	var after *string
	orders := []Order{}
	seenCursors := map[string]bool{}
	for page := 0; page < maxPages; page++ {
		body, _ := json.Marshal(map[string]any{"query": shopifyOrdersQuery, "variables": map[string]any{"after": after, "search": search}})
		var result struct {
			Data *struct {
				Installation *struct {
					Scopes []struct {
						Handle string `json:"handle"`
					} `json:"accessScopes"`
				} `json:"currentAppInstallation"`
				Orders *struct {
					Nodes []shopOrder `json:"nodes"`
					Page  pageInfo    `json:"pageInfo"`
				} `json:"orders"`
			} `json:"data"`
			Errors []json.RawMessage `json:"errors"`
		}
		if _, err := c.request(ctx, http.MethodPost, conn.URL+"/admin/api/"+shopifyVersion+"/graphql.json", body, map[string]string{"Content-Type": "application/json", "X-Shopify-Access-Token": token}, nil, &result); err != nil {
			return nil, err
		}
		if len(result.Errors) > 0 || result.Data == nil || result.Data.Orders == nil {
			return nil, errors.New("Shopify konnte die Bestellungen nicht vollständig liefern. Bitte API-Leserechte (read_orders, für ältere Bestellungen read_all_orders) und Zugriff auf Kundendaten prüfen.")
		}
		// Shopify otherwise silently hides orders older than 60 days. Do not
		// advance the local checkpoint after an incomplete historical import.
		if since.Before(time.Now().UTC().AddDate(0, 0, -59)) {
			allOrders := false
			if result.Data.Installation != nil {
				for _, scope := range result.Data.Installation.Scopes {
					if scope.Handle == "read_all_orders" {
						allOrders = true
					}
				}
			}
			if !allOrders {
				return nil, errors.New("Für diesen Importbeginn benötigt die Shopify-App zusätzlich read_all_orders. Bitte die Berechtigung erteilen oder einen Importbeginn innerhalb der letzten 59 Tage wählen.")
			}
		}
		batch := result.Data.Orders
		for _, raw := range batch.Nodes {
			if raw.ID == "" {
				return nil, errors.New("Shopify hat eine Bestellung ohne Kennung geliefert.")
			}
			orders = append(orders, normalizeShopify(raw))
		}
		if !batch.Page.HasNext {
			return orders, nil
		}
		if batch.Page.End == "" || seenCursors[batch.Page.End] || len(batch.Nodes) == 0 {
			return nil, errors.New("Die Shopify-Seitenfolge ist unvollständig. Es wurden keine Bestellungen übernommen.")
		}
		seenCursors[batch.Page.End] = true
		cursor := batch.Page.End
		after = &cursor
	}
	return nil, errors.New("Der Shopify-Import überschreitet 1.000 Bestellungen. Bitte einen späteren Importbeginn wählen.")
}

func money(o *Order, b moneyBag) string {
	if b.Presentment.Currency != o.Currency {
		review(o, "Beträge in unterschiedlichen Währungen müssen geprüft werden.")
	}
	return b.Presentment.Amount
}

func shopTaxes(o *Order, ts []shopTax) (string, string, bool) {
	total := decimal.Zero
	rate := decimal.Zero
	known := len(ts) == 1
	for _, t := range ts {
		total = total.Add(parseMoney(o, money(o, t.Price)))
		r, err := boundedDecimal(string(t.Rate))
		if err != nil {
			known = false
		} else {
			rate = rate.Add(r.Mul(decimal.NewFromInt(100)))
		}
	}
	if len(ts) > 1 {
		review(o, "Mehrere oder zusammengesetzte Steuern je Position werden nicht automatisch übernommen.")
	}
	return total.String(), rate.String(), known
}

func normalizeShopify(raw shopOrder) Order {
	o := Order{ExternalID: raw.ID, Number: raw.Name, Currency: raw.Total.Presentment.Currency, Gross: raw.Total.Presentment.Amount, CreatedAt: raw.Created, UpdatedAt: raw.Updated, Status: raw.Status, Paid: raw.Status == "PAID"}
	if raw.Customer != nil {
		o.CustomerExternalID = raw.Customer.ID
	}
	if b := raw.Billing; b != nil {
		o.Customer = customer(b.First, b.Last, b.Company, raw.Email, b.Street, addressAdditional(b.Additional, b.Province), b.Postcode, b.City, b.Country)
	}
	switch raw.Status {
	case "PENDING", "AUTHORIZED", "PAID", "PARTIALLY_REFUNDED", "REFUNDED", "VOIDED":
	case "PARTIALLY_PAID":
		review(&o, "Teilzahlungen benötigen eine manuelle Prüfung der bereits gezahlten Beträge.")
	default:
		review(&o, "Der Zahlungsstatus der Bestellung benötigt eine manuelle Prüfung.")
	}
	if raw.Cancelled != nil || raw.Status == "VOIDED" {
		review(&o, "Die Bestellung wurde storniert.")
	}
	if parseMoney(&o, money(&o, raw.Refunded)).IsPositive() || raw.Status == "REFUNDED" || raw.Status == "PARTIALLY_REFUNDED" {
		review(&o, "Erstattungen benötigen eine manuelle Prüfung und gegebenenfalls eine Rechnungskorrektur.")
	}
	if parseMoney(&o, money(&o, raw.Tips)).IsPositive() {
		review(&o, "Trinkgelder benötigen eine manuelle steuerliche Prüfung.")
	}
	if raw.Lines.Page.HasNext || raw.Shipping.Page.HasNext {
		review(&o, "Die Bestellung enthält mehr Positionen als automatisch übernommen werden können; vollständige Bestellung im Shop prüfen.")
	}
	lines := []normalizedLine{}
	for _, l := range raw.Lines.Nodes {
		if l.GiftCard {
			review(&o, "Gutscheine benötigen eine manuelle steuerliche Prüfung.")
		}
		if l.Quantity != l.CurrentQuantity {
			review(&o, "Geänderte oder erstattete Bestellmengen müssen geprüft werden.")
		}
		tax, rate, known := shopTaxes(&o, l.TaxLines)
		lines = append(lines, normalizedLine{id: l.ID, name: l.Name, description: l.SKU, quantity: decimal.NewFromInt(l.CurrentQuantity).String(), net: money(&o, l.Net), tax: tax, rate: rate, knownRate: known})
	}
	for _, s := range raw.Shipping.Nodes {
		if s.Removed {
			review(&o, "Entfernte Versandpositionen müssen geprüft werden.")
		}
		tax, rate, known := shopTaxes(&o, s.Taxes)
		amount := parseMoney(&o, money(&o, s.Price))
		if raw.TaxesIncluded {
			amount = amount.Sub(parseMoney(&o, tax))
		}
		// Free shipping has no tax line; as an invoice line it would be zero-rated.
		if amount.IsZero() && parseMoney(&o, tax).IsZero() {
			continue
		}
		lines = append(lines, normalizedLine{id: s.ID, name: "Versand: " + s.Title, quantity: "1", net: amount.String(), tax: tax, rate: rate, knownRate: known})
	}
	finishOrder(&o, lines, money(&o, raw.Tax))
	return o
}
