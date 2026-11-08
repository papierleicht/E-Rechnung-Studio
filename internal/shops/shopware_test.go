package shops

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"
)

func validShopwareOrder() shopwareOrder {
	var raw shopwareOrder
	err := json.Unmarshal([]byte(`{
 "id":"order-1","orderNumber":"SW-1001","createdAt":"2026-10-01T09:00:00.000+00:00","updatedAt":null,
 "amountTotal":220.15,"amountNet":185,"taxStatus":"gross",
 "price":{"netPrice":185,"totalPrice":220.15,"taxStatus":"gross","calculatedTaxes":[{"tax":35.15,"taxRate":19,"price":220.15}]},
 "shippingCosts":{"unitPrice":5.95,"totalPrice":5.95,"quantity":1,"calculatedTaxes":[{"tax":0.95,"taxRate":19,"price":5.95}],"taxRules":[{"taxRate":19,"percentage":100}]},
 "currency":{"isoCode":"EUR"},"stateMachineState":{"technicalName":"completed"},
 "orderCustomer":{"id":"order-customer-1","customerNumber":"C100","email":"ada@example.com","vatIds":["DE123456789"]},
 "billingAddress":{"firstName":"Ada","lastName":"Lovelace","company":"Ada GmbH","department":"Einkauf","street":"Testweg 1","zipcode":"10115","city":"Berlin","additionalAddressLine1":"2. OG","additionalAddressLine2":"Tür 3","country":{"iso":"DE"},"countryState":{"name":"Berlin"}},
 "lineItems":[
  {"id":"product-1","type":"product","label":"Produkt","description":"Variante Blau","quantity":2,"totalPrice":238,"price":{"unitPrice":119,"totalPrice":238,"quantity":2,"calculatedTaxes":[{"tax":38,"taxRate":19,"price":238}],"taxRules":[{"taxRate":19,"percentage":100}]}},
  {"id":"promo-1","type":"promotion","label":"10 Prozent Rabatt","quantity":1,"totalPrice":-23.8,"price":{"unitPrice":-23.8,"totalPrice":-23.8,"quantity":1,"calculatedTaxes":[{"tax":-3.8,"taxRate":19,"price":-23.8}],"taxRules":[{"taxRate":19,"percentage":100}]}}
 ],
 "primaryOrderTransactionId":"payment-1",
 "transactions":[{"id":"payment-1","createdAt":"2026-10-01T09:00:00.000+00:00","amount":{"totalPrice":220.15},"stateMachineState":{"technicalName":"paid"},"captures":[]}],
 "deliveries":[{"id":"delivery-1","stateMachineState":{"technicalName":"shipped"}}]
}`), &raw)
	if err != nil {
		panic(err)
	}
	return raw
}

func shopwareConn() Connection {
	return Connection{Provider: "shopware", URL: "https://shop.example/subdir"}
}

func TestShopwareGrossOrderWithDiscountAndShipping(t *testing.T) {
	o := normalizeShopware(validShopwareOrder())
	if o.ReviewReason != "" {
		t.Fatal(o.ReviewReason)
	}
	if o.Gross != "220.15" || o.Currency != "EUR" || !o.Paid || o.InvoiceID != "" || len(o.Lines) != 2 || len(o.Adjustments) != 1 {
		t.Fatalf("order=%+v", o)
	}
	if o.Lines[0].Quantity != "2" || o.Lines[0].UnitPrice != "100" || o.Lines[1].UnitPrice != "5" || o.Adjustments[0].Charge || o.Adjustments[0].Amount != "20" || o.Adjustments[0].TaxRate != "19" {
		t.Fatalf("lines=%+v adjustments=%+v", o.Lines, o.Adjustments)
	}
	if o.Customer.Name != "Ada GmbH" || o.Customer.ContactName != "Ada Lovelace" || o.Customer.VATID != "DE123456789" || o.CustomerExternalID != "C100" || o.Customer.Address.Additional != "2. OG, Tür 3, Einkauf, Berlin" || o.UpdatedAt != o.CreatedAt {
		t.Fatalf("billing/dates lost: %+v", o)
	}
}

func TestShopwareNetPricesAndUnpaidOrder(t *testing.T) {
	raw := validShopwareOrder()
	raw.TaxStatus = "net"
	raw.Price.TaxStatus = "net"
	raw.Lines[0].Total = "200"
	raw.Lines[0].Price.Total = "200"
	raw.Lines[0].Price.Unit = "100"
	raw.Lines[0].Price.Taxes[0].Price = "200"
	raw.Lines[1].Total = "-20"
	raw.Lines[1].Price.Total = "-20"
	raw.Lines[1].Price.Unit = "-20"
	raw.Lines[1].Price.Taxes[0].Price = "-20"
	raw.Shipping.Total = "5"
	raw.Shipping.Unit = "5"
	raw.Shipping.Taxes[0].Price = "5"
	raw.Transactions[0].State.TechnicalName = "open"
	o := normalizeShopware(raw)
	if o.ReviewReason != "" || o.Paid || o.Lines[0].UnitPrice != "100" || o.Lines[1].UnitPrice != "5" || o.Adjustments[0].Amount != "20" {
		t.Fatalf("net order=%+v", o)
	}
}

func TestShopwareMixedRatePromotionsAndShipping(t *testing.T) {
	raw := validShopwareOrder()
	raw.Lines[0].Quantity = "1"
	raw.Lines[0].Total = "119"
	raw.Lines[0].Price.Total = "119"
	raw.Lines[0].Price.Taxes = []shopwareTax{{Tax: "19", Rate: "19", Price: "119"}}
	second := shopwareLine{ID: "product-2", Type: "product", Label: "Buch", Quantity: "1", Total: "107", Price: &shopwarePrice{Total: "107", Taxes: []shopwareTax{{Tax: "7", Rate: "7", Price: "107"}}}}
	promo := raw.Lines[1]
	promo.Total = "-22.6"
	promo.Price.Total = "-22.6"
	promo.Price.Taxes = []shopwareTax{{Tax: "-1.9", Rate: "19", Price: "-11.9"}, {Tax: "-0.7", Rate: "7", Price: "-10.7"}}
	raw.Lines = []shopwareLine{raw.Lines[0], second, promo}
	raw.Shipping.Total = "11.3"
	raw.Shipping.Taxes = []shopwareTax{{Tax: "0.95", Rate: "19", Price: "5.95"}, {Tax: "0.35", Rate: "7", Price: "5.35"}}
	raw.Total = "214.7"
	raw.Net = "190"
	raw.Price.Total = "214.7"
	raw.Price.Net = "190"
	raw.Price.Taxes = []shopwareTax{{Tax: "18.05", Rate: "19", Price: "113.05"}, {Tax: "6.65", Rate: "7", Price: "101.65"}}
	raw.Transactions[0].Amount.Total = "214.7"
	o := normalizeShopware(raw)
	if o.ReviewReason != "" || len(o.Lines) != 4 || len(o.Adjustments) != 2 || o.Adjustments[0].Amount != "10" || o.Adjustments[1].Amount != "10" || o.Lines[2].UnitPrice != "5" || o.Lines[3].UnitPrice != "5" {
		t.Fatalf("mixed-tax order=%+v", o)
	}
}

func TestShopwareAuthReadOnlyPaginationAndInclusiveFilter(t *testing.T) {
	calls, pages := 0, 0
	since := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.Method != "POST" || r.Header.Get("Accept") != "application/json" || r.Header.Get("Content-Type") != "application/json" {
			t.Error("wrong API headers/method")
		}
		if r.URL.Path == "/subdir/api/oauth/token" {
			var body map[string]string
			json.NewDecoder(r.Body).Decode(&body)
			if body["grant_type"] != "client_credentials" || body["client_id"] != "integration-access" || body["client_secret"] != "integration-secret" {
				t.Errorf("wrong OAuth fields")
			}
			fmt.Fprint(w, `{"access_token":"short-lived-token","token_type":"Bearer"}`)
			return
		}
		if r.URL.Path != "/subdir/api/search/order" || r.Header.Get("Authorization") != "Bearer short-lived-token" || r.URL.RawQuery != "" {
			t.Error("wrong read-only endpoint/auth")
		}
		pages++
		var body struct {
			Page   int `json:"page"`
			Limit  int `json:"limit"`
			Count  int `json:"total-count-mode"`
			Filter []struct {
				Type     string `json:"type"`
				Operator string `json:"operator"`
				Queries  []struct {
					Field      string            `json:"field"`
					Parameters map[string]string `json:"parameters"`
				} `json:"queries"`
			} `json:"filter"`
			Associations map[string]json.RawMessage `json:"associations"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if body.Page != pages || body.Limit != shopwarePageSize || body.Count != 1 || len(body.Filter) != 1 || body.Filter[0].Type != "multi" || body.Filter[0].Operator != "or" {
			t.Errorf("incorrect search criteria: %+v", body)
		}
		fields := map[string]bool{}
		for _, q := range body.Filter[0].Queries {
			fields[q.Field] = true
			if q.Parameters["gte"] != since.Format(time.RFC3339) {
				t.Error("exclusive or incorrect date filter")
			}
		}
		for _, field := range []string{"createdAt", "updatedAt", "transactions.updatedAt", "transactions.captures.createdAt", "transactions.captures.updatedAt", "transactions.captures.refunds.updatedAt", "deliveries.createdAt", "deliveries.updatedAt"} {
			if !fields[field] {
				t.Errorf("missing date field %s", field)
			}
		}
		for _, assoc := range []string{"lineItems", "currency", "orderCustomer", "billingAddress", "transactions", "deliveries", "stateMachineState"} {
			if body.Associations[assoc] == nil {
				t.Errorf("missing association %s", assoc)
			}
		}
		raw := validShopwareOrder()
		raw.ID = fmt.Sprintf("order-%d", pages)
		json.NewEncoder(w).Encode(map[string]any{"data": []shopwareOrder{raw}, "total": 2})
	})
	orders, err := c.fetchShopware(context.Background(), shopwareConn(), Credentials{ClientID: "integration-access", ClientSecret: "integration-secret"}, since)
	if err != nil || calls != 3 || pages != 2 || len(orders) != 2 || orders[0].ReviewReason != "" {
		t.Fatalf("orders=%+v calls=%d pages=%d err=%v", orders, calls, pages, err)
	}
}

func TestShopwareFinancialAndUnsupportedFeaturesNeedReview(t *testing.T) {
	for _, tc := range []struct {
		name, reason string
		mutate       func(*shopwareOrder)
	}{
		{"partial-payment", "Teilzahlungen", func(r *shopwareOrder) { r.Transactions[0].State.TechnicalName = "paid_partially" }},
		{"refund-state", "Erstattungen", func(r *shopwareOrder) { r.Transactions[0].State.TechnicalName = "refunded_partially" }},
		{"refund-capture", "Erstattungen", func(r *shopwareOrder) {
			r.Transactions[0].Captures = []shopwareCapture{{ID: "capture", Refunds: []shopwareRefund{{ID: "refund"}}}}
		}},
		{"cancelled", "storniert", func(r *shopwareOrder) { r.State.TechnicalName = "cancelled" }},
		{"billing", "Rechnungsadresse", func(r *shopwareOrder) { r.Billing = nil }},
		{"unknown-status", "Bestellstatus", func(r *shopwareOrder) { r.State.TechnicalName = "custom-status" }},
		{"tax-free", "steuerfrei", func(r *shopwareOrder) { r.TaxStatus = "tax-free"; r.Price.TaxStatus = "tax-free" }},
		{"mixed-product", "zusammengesetzte", func(r *shopwareOrder) {
			r.Lines[0].Price.Taxes = append(r.Lines[0].Price.Taxes, shopwareTax{Tax: "2", Rate: "1", Price: "238"})
		}},
		{"bundle", "Bundles", func(r *shopwareOrder) { r.Lines[0].ParentID = "parent" }},
		{"unknown-line", "Positionstyp", func(r *shopwareOrder) { r.Lines[0].Type = "deposit" }},
		{"total-mismatch", "Gesamtsumme", func(r *shopwareOrder) { r.Total = "221" }},
		{"missing-price", "Preisberechnung", func(r *shopwareOrder) { r.Lines[0].Price = nil }},
		{"negative-refund-line", "Negative", func(r *shopwareOrder) { r.Lines[1].Type = "product" }},
		{"payment-amount", "Teilzahlungen", func(r *shopwareOrder) { r.Transactions[0].Amount.Total = "1" }},
		{"unbounded-decimal", "ungültige", func(r *shopwareOrder) { r.Lines[0].Price.Total = "1e99999999" }},
		{"partial-transactions", "Transaktionslimit", func(r *shopwareOrder) { r.Transactions = make([]shopwareTransaction, shopwareAssociationLimit) }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			raw := validShopwareOrder()
			tc.mutate(&raw)
			o := normalizeShopware(raw)
			if !strings.Contains(o.ReviewReason, tc.reason) || o.ExternalID == "" || o.Gross == "" {
				t.Fatalf("review=%q order=%+v", o.ReviewReason, o)
			}
		})
	}
}

func TestShopwarePaidStatusComesFromFullTransaction(t *testing.T) {
	for _, state := range []string{"open", "authorized", "in_progress", "reminded", "unconfirmed"} {
		raw := validShopwareOrder()
		raw.Transactions[0].State.TechnicalName = state
		o := normalizeShopware(raw)
		if o.Paid || o.ReviewReason != "" {
			t.Fatalf("%s => paid=%v review=%q", state, o.Paid, o.ReviewReason)
		}
	}
	raw := validShopwareOrder()
	old := raw.Transactions[0]
	old.ID = "old-attempt"
	old.Created = "2026-09-30T09:00:00Z"
	old.State = &shopwareState{TechnicalName: "failed"}
	raw.Transactions = append(raw.Transactions, old)
	if o := normalizeShopware(raw); !o.Paid || o.ReviewReason != "" {
		t.Fatalf("failed previous attempt should not block successful payment: %+v", o)
	}
	raw.Transactions[1].State.TechnicalName = "paid"
	if o := normalizeShopware(raw); o.Paid || !strings.Contains(o.ReviewReason, "Mehrere bezahlte") {
		t.Fatalf("duplicate paid transaction accepted: %+v", o)
	}
}

func TestShopwareAssociationLimitDoesNotSilentlyCreateDraft(t *testing.T) {
	raw := validShopwareOrder()
	raw.Lines = append(raw.Lines, make([]shopwareLine, shopwareLineLimit-len(raw.Lines))...)
	o := normalizeShopware(raw)
	if !strings.Contains(o.ReviewReason, "Positions- oder Transaktionslimit") || o.Gross != "220.15" {
		t.Fatalf("partial nested list accepted: %+v", o)
	}
}

func TestShopwareFailedPagesNeverReturnPartialOrdersOrSecrets(t *testing.T) {
	for _, kind := range []string{"http", "api-error", "duplicate", "missing", "changed-total", "missing-total"} {
		t.Run(kind, func(t *testing.T) {
			pages := 0
			c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
				if strings.HasSuffix(r.URL.Path, "/oauth/token") {
					fmt.Fprint(w, `{"access_token":"private-token"}`)
					return
				}
				pages++
				raw := validShopwareOrder()
				if pages == 1 {
					json.NewEncoder(w).Encode(map[string]any{"data": []shopwareOrder{raw}, "total": 2})
					return
				}
				switch kind {
				case "http":
					w.WriteHeader(401)
					fmt.Fprint(w, "private-token integration-secret")
				case "api-error":
					fmt.Fprint(w, `{"data":[],"total":2,"errors":[{"detail":"integration-secret"}]}`)
				case "duplicate":
					json.NewEncoder(w).Encode(map[string]any{"data": []shopwareOrder{raw}, "total": 2})
				case "missing":
					fmt.Fprint(w, `{"data":[],"total":2}`)
				case "changed-total":
					fmt.Fprint(w, `{"data":[],"total":1}`)
				case "missing-total":
					fmt.Fprint(w, `{"data":[]}`)
				}
			})
			orders, err := c.fetchShopware(context.Background(), shopwareConn(), Credentials{ClientID: "id", ClientSecret: "integration-secret"}, time.Now())
			if err == nil || orders != nil || strings.Contains(err.Error(), "private-token") || strings.Contains(err.Error(), "integration-secret") {
				t.Fatalf("orders=%+v err=%v", orders, err)
			}
		})
	}
}

func TestShopwareTokenValidationAndDateParsing(t *testing.T) {
	for _, token := range []string{"", "unsafe\r\nvalue"} {
		c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
			json.NewEncoder(w).Encode(map[string]string{"access_token": token})
		})
		orders, err := c.fetchShopware(context.Background(), shopwareConn(), Credentials{}, time.Now())
		if err == nil || orders != nil {
			t.Fatalf("invalid token accepted: %q", token)
		}
	}
	raw := validShopwareOrder()
	raw.Created = "missing"
	o := normalizeShopware(raw)
	if !strings.Contains(o.ReviewReason, "Bestelldaten") {
		t.Fatal("invalid source dates accepted")
	}
}
