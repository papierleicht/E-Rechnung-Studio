package shops

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/shopspring/decimal"
	"papierleicht.de/e-rechnung-studio/internal/engine"
	"papierleicht.de/e-rechnung-studio/internal/model"
)

type rewriteTransport struct {
	base   http.RoundTripper
	target *url.URL
}

func (r rewriteTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	copy := req.Clone(req.Context())
	u := *req.URL
	u.Scheme = r.target.Scheme
	u.Host = r.target.Host
	copy.URL = &u
	return r.base.RoundTrip(copy)
}

func testClient(t *testing.T, h http.HandlerFunc) *Client {
	t.Helper()
	server := httptest.NewTLSServer(h)
	t.Cleanup(server.Close)
	u, _ := url.Parse(server.URL)
	c := NewClient()
	c.http.Transport = rewriteTransport{base: server.Client().Transport, target: u}
	return c
}

func testShopConn() Connection {
	return Connection{Provider: "shopify", URL: "https://demo.myshopify.com"}
}
func testWooConn() Connection {
	return Connection{Provider: "woocommerce", URL: "https://store.example/shop"}
}
func bag(amount string) moneyBag {
	return moneyBag{Presentment: shopMoney{Amount: amount, Currency: "EUR"}}
}
func validShopOrder() shopOrder {
	r := shopOrder{ID: "gid://shopify/Order/42", Name: "#1001", Created: "2026-10-01T09:00:00Z", Updated: "2026-10-15T09:00:00Z", Status: "PAID", TaxesIncluded: true, Total: bag("113.05"), Tax: bag("18.05"), Refunded: bag("0"), Tips: bag("0")}
	r.Billing = &struct {
		First      string `json:"firstName"`
		Last       string `json:"lastName"`
		Company    string `json:"company"`
		Street     string `json:"address1"`
		Additional string `json:"address2"`
		City       string `json:"city"`
		Province   string `json:"province"`
		Postcode   string `json:"zip"`
		Country    string `json:"countryCodeV2"`
	}{First: "Ada", Last: "Lovelace", Street: "Testweg 1", City: "Berlin", Postcode: "10115", Country: "DE"}
	r.Lines.Nodes = []shopLine{{ID: "line-1", Name: "Produkt nach Rabatt", Quantity: 3, CurrentQuantity: 3, Net: bag("90"), TaxLines: []shopTax{{Rate: "0.19", Price: bag("17.10")}}}}
	r.Shipping.Nodes = []shopShipping{{ID: "ship-1", Title: "Paket", Price: bag("5.95"), Taxes: []shopTax{{Rate: "0.19", Price: bag("0.95")}}}}
	return r
}

func validWooOrder() wooOrder {
	var r wooOrder
	err := json.Unmarshal([]byte(`{"id":42,"number":"42","customer_id":7,"currency":"EUR","date_created_gmt":"2026-10-01T09:00:00","date_modified_gmt":"2026-10-15T09:00:00","date_paid_gmt":"2026-10-01T10:00:00","status":"processing","total":"115.43","total_tax":"18.43","billing":{"first_name":"Ada","last_name":"Lovelace","company":"Ada GmbH","address_1":"Testweg 1","address_2":"2. OG","city":"Berlin","postcode":"10115","country":"DE","email":"ada@example.com"},"line_items":[{"id":1,"name":"Produkt nach Rabatt","quantity":3,"total":"90.00","total_tax":"17.10","taxes":[{"id":9,"total":"17.10"}]}],"shipping_lines":[{"id":2,"method_title":"Paket","total":"5.00","total_tax":"0.95","taxes":[{"id":9,"total":"0.95"}]}],"fee_lines":[{"id":3,"name":"Verpackung","total":"2.00","total_tax":"0.38","taxes":[{"id":9,"total":"0.38"}]}],"tax_lines":[{"rate_id":9,"rate_percent":"19","compound":false}]}`), &r)
	if err != nil {
		panic(err)
	}
	return r
}

func TestNormalizeConnection(t *testing.T) {
	for _, tc := range []struct {
		provider, raw, want string
		bad                 bool
	}{
		{"shopify", " Demo.myshopify.com/ ", "https://demo.myshopify.com", false},
		{"woocommerce", "https://Shop.Example:443/wordpress/", "https://shop.example/wordpress", false},
		{"shopify", "https://demo.myshopify.com.evil.test", "", true},
		{"shopify", "https://evil.myshopify.com@evil.example", "", true},
		{"shopify", "https://demo.myshopify.com/admin", "", true},
		{"woocommerce", "http://shop.example", "", true},
		{"woocommerce", "https://user:pass@shop.example", "", true},
		{"woocommerce", "https://shop.example/?token=secret", "", true},
		{"woocommerce", "https://shop.example/#secret", "", true},
		{"woocommerce", "https://shop.example:8443", "", true},
		{"woocommerce", "https://shop.example/a/../b", "", true},
		{"woocommerce", "https://shop.example/%2fsecret", "", true},
	} {
		t.Run(tc.raw, func(t *testing.T) {
			got, err := NormalizeConnection(Connection{Provider: tc.provider, URL: tc.raw})
			if (err != nil) != tc.bad {
				t.Fatalf("error=%v", err)
			}
			if !tc.bad && got.URL != tc.want {
				t.Fatalf("URL=%q", got.URL)
			}
		})
	}
}

func TestValidateCredentials(t *testing.T) {
	if err := ValidateCredentials("shopify", Credentials{ClientID: "app", ClientSecret: "secret"}); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		provider string
		c        Credentials
	}{{"shopify", Credentials{}}, {"shopify", Credentials{AccessToken: "bad\nvalue", ClientID: "app", ClientSecret: "secret"}}, {"woocommerce", Credentials{ConsumerKey: "key"}}} {
		if ValidateCredentials(tc.provider, tc.c) == nil {
			t.Fatal("invalid credentials accepted")
		}
	}
}

func TestShopifyTokenAndPagination(t *testing.T) {
	var calls int
	since := time.Now().UTC().Truncate(time.Second).Add(-24 * time.Hour)
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.Method != "POST" || r.URL.Path != "/admin/api/2026-10/graphql.json" || r.Header.Get("X-Shopify-Access-Token") != "private-token" {
			t.Errorf("incorrect Shopify auth/endpoint")
		}
		var body struct {
			Query     string `json:"query"`
			Variables struct {
				After  *string `json:"after"`
				Search string  `json:"search"`
			} `json:"variables"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if strings.Contains(body.Query, "mutation") || !strings.Contains(body.Query, "orders(first: 1,") || !strings.Contains(body.Variables.Search, "updated_at:>='"+since.Format(time.RFC3339)+"'") {
			t.Error("wrong read-only/inclusive query")
		}
		if calls == 1 && body.Variables.After != nil {
			t.Error("unexpected first cursor")
		}
		if calls == 2 && (body.Variables.After == nil || *body.Variables.After != "next") {
			t.Error("missing cursor")
		}
		raw := validShopOrder()
		raw.ID = fmt.Sprint(calls)
		json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{"orders": map[string]any{"nodes": []shopOrder{raw}, "pageInfo": map[string]any{"hasNextPage": calls == 1, "endCursor": "next"}}}})
	})
	orders, err := c.FetchOrders(context.Background(), testShopConn(), Credentials{AccessToken: "private-token"}, since)
	if err != nil || len(orders) != 2 || calls != 2 {
		t.Fatalf("orders=%d calls=%d err=%v", len(orders), calls, err)
	}
	if orders[0].ReviewReason != "" {
		t.Fatal(orders[0].ReviewReason)
	}
}

func TestShopifyClientCredentials(t *testing.T) {
	calls := 0
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		calls++
		if calls == 1 {
			if r.URL.Path != "/admin/oauth/access_token" {
				t.Error("wrong token endpoint")
			}
			if err := r.ParseForm(); err != nil {
				t.Fatal(err)
			}
			if r.Form.Get("grant_type") != "client_credentials" || r.Form.Get("client_id") != "client" || r.Form.Get("client_secret") != "secret" {
				t.Error("wrong grant")
			}
			fmt.Fprint(w, `{"access_token":"ephemeral"}`)
			return
		}
		if r.Header.Get("X-Shopify-Access-Token") != "ephemeral" {
			t.Error("missing exchanged token")
		}
		fmt.Fprint(w, `{"data":{"orders":{"nodes":[],"pageInfo":{"hasNextPage":false}}}}`)
	})
	_, err := c.FetchOrders(context.Background(), testShopConn(), Credentials{ClientID: "client", ClientSecret: "secret"}, time.Now())
	if err != nil || calls != 2 {
		t.Fatalf("calls=%d err=%v", calls, err)
	}
}

func TestShopifyFinancialMapping(t *testing.T) {
	o := normalizeShopify(validShopOrder())
	if o.ReviewReason != "" || len(o.Lines) != 2 || o.Lines[0].UnitPrice != "30" || o.Lines[1].UnitPrice != "5" || !o.Paid || o.InvoiceID != "" || o.Currency != "EUR" {
		t.Fatalf("unexpected order: %+v", o)
	}
	for _, tc := range []struct {
		name, reason string
		mutate       func(*shopOrder)
	}{
		{"refund", "Erstattungen", func(r *shopOrder) { r.Refunded = bag("1") }},
		{"partial-payment", "Teilzahlungen", func(r *shopOrder) { r.Status = "PARTIALLY_PAID" }},
		{"cancelled", "storniert", func(r *shopOrder) { v := "2026-10-15T10:00:00Z"; r.Cancelled = &v }},
		{"billing", "Rechnungsadresse", func(r *shopOrder) { r.Billing = nil }},
		{"missing-lines", "mehr Positionen", func(r *shopOrder) { r.Lines.Page.HasNext = true }},
		{"mismatch", "Bestellsumme", func(r *shopOrder) { r.Total = bag("200") }},
		{"currency", "Währungen", func(r *shopOrder) { r.Lines.Nodes[0].Net.Presentment.Currency = "USD" }},
		{"mixed-tax", "Mehrere", func(r *shopOrder) {
			r.Lines.Nodes[0].TaxLines = append(r.Lines.Nodes[0].TaxLines, shopTax{Rate: "0.01", Price: bag("0.90")})
		}},
		{"gift-card", "Gutscheine", func(r *shopOrder) { r.Lines.Nodes[0].GiftCard = true }},
		// Without tax data the line counts as untaxed; the order's tax total then disagrees.
		{"zero-tax", "Steuerbeträge", func(r *shopOrder) { r.Lines.Nodes[0].TaxLines = nil }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			raw := validShopOrder()
			tc.mutate(&raw)
			o := normalizeShopify(raw)
			if !strings.Contains(o.ReviewReason, tc.reason) || len(o.Lines) == 0 {
				t.Fatalf("review=%q", o.ReviewReason)
			}
		})
	}
}

func TestWooReadOnlyAuthPaginationAndRate(t *testing.T) {
	var pages, rates int
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		key, secret, ok := r.BasicAuth()
		if !ok || key != "ck_private" || secret != "cs_private" || r.Method != "GET" || r.URL.Query().Has("consumer_secret") {
			t.Error("wrong Woo authentication")
		}
		if strings.HasSuffix(r.URL.Path, "/taxes/9") {
			rates++
			fmt.Fprint(w, `{"rate":"19.0000","compound":false}`)
			return
		}
		if r.URL.Path != "/shop/wp-json/wc/v3/orders" {
			t.Errorf("subdirectory lost: %s", r.URL.Path)
		}
		pages++
		if r.URL.Query().Get("page") != fmt.Sprint(pages) || r.URL.Query().Get("modified_after") != "2026-10-01T08:59:59Z" || r.URL.Query().Get("dates_are_gmt") != "true" {
			t.Error("wrong pagination/modified bound")
		}
		w.Header().Set("X-WP-TotalPages", "2")
		raw := validWooOrder()
		raw.ID = int64(pages)
		raw.TaxLines[0].Percent = nil
		json.NewEncoder(w).Encode([]wooOrder{raw})
	})
	orders, err := c.FetchOrders(context.Background(), testWooConn(), Credentials{ConsumerKey: "ck_private", ConsumerSecret: "cs_private"}, time.Date(2026, 10, 1, 9, 0, 0, 0, time.UTC))
	if err != nil || len(orders) != 2 || pages != 2 || rates != 1 {
		t.Fatalf("orders=%d pages=%d rates=%d err=%v", len(orders), pages, rates, err)
	}
	o := orders[0]
	if o.ReviewReason != "" || len(o.Lines) != 3 || o.Gross != "115.43" || o.Customer.Name != "Ada GmbH" || o.Customer.ContactName != "Ada Lovelace" || o.Customer.Address.Additional != "2. OG" || !o.Paid {
		t.Fatalf("unexpected order: %+v", o)
	}
}

func TestWooUnavailableTaxRateKeepsReviewOrder(t *testing.T) {
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/taxes/") {
			w.WriteHeader(403)
			fmt.Fprint(w, `{"message":"cs_private"}`)
			return
		}
		raw := validWooOrder()
		raw.TaxLines[0].Percent = nil
		json.NewEncoder(w).Encode([]wooOrder{raw})
	})
	orders, err := c.FetchOrders(context.Background(), testWooConn(), Credentials{ConsumerKey: "ck_private", ConsumerSecret: "cs_private"}, time.Now())
	if err != nil || len(orders) != 1 || orders[0].ReviewReason == "" || len(orders[0].Lines) != 3 || strings.Contains(orders[0].ReviewReason, "cs_private") {
		t.Fatalf("orders=%+v err=%v", orders, err)
	}
}

func TestWooRefundAndMissingAddressStayVisible(t *testing.T) {
	raw := validWooOrder()
	raw.Refunds = []json.RawMessage{json.RawMessage(`{"total":"-1"}`)}
	raw.Billing.Street = ""
	o := normalizeWoo(raw, map[int64]wooRate{9: {rate: "19", known: true}})
	if len(o.Lines) != 3 || !strings.Contains(o.ReviewReason, "Erstattungen") || !strings.Contains(o.ReviewReason, "Rechnungsadresse") {
		t.Fatalf("order=%+v", o)
	}
}

func TestWooCustomPaymentStatusNeedsReview(t *testing.T) {
	raw := validWooOrder()
	raw.Status = "partially-paid"
	o := normalizeWoo(raw, map[int64]wooRate{9: {rate: "19", known: true}})
	if !strings.Contains(o.ReviewReason, "Anzahlungen") || len(o.Lines) != 3 {
		t.Fatalf("partial payment accepted: %+v", o)
	}
}

func TestBillingRegionPreserved(t *testing.T) {
	raw := validShopOrder()
	raw.Billing.Province = "California"
	raw.Billing.Additional = "Suite 5"
	if got := normalizeShopify(raw).Customer.Address.Additional; got != "Suite 5, California" {
		t.Fatalf("region lost: %s", got)
	}
	w := validWooOrder()
	w.Billing.State = "CA"
	w.Billing.Additional = "Suite 5"
	if got := normalizeWoo(w, map[int64]wooRate{9: {rate: "19", known: true}}).Customer.Address.Additional; got != "Suite 5, CA" {
		t.Fatalf("region lost: %s", got)
	}
}

func TestExactDecimalMappingAndTaxRounding(t *testing.T) {
	o := normalizeShopify(validShopOrder())
	o.Lines = nil
	o.ReviewReason = ""
	o.Gross = "119.00"
	finishOrder(&o, []normalizedLine{{name: "Drittel", quantity: "3", net: "100.00", tax: "19.00", rate: "19", knownRate: true}}, "19.00")
	if o.ReviewReason != "" {
		t.Fatal(o.ReviewReason)
	}
	price := decimal.RequireFromString(o.Lines[0].UnitPrice)
	if !price.Mul(decimal.NewFromInt(3)).Round(2).Equal(decimal.NewFromInt(100)) {
		t.Fatal("money lost")
	}
	// Rounding each line (0.03 € gross is 0.03 net and no tax) differs from the
	// per-rate calculation by a cent; the invoice keeps the shop's amount.
	o.Lines = nil
	o.ReviewReason = ""
	o.Gross = "0.06"
	finishOrder(&o, []normalizedLine{{name: "A", quantity: "1", net: "0.03", tax: "0", rate: "19", knownRate: true}, {name: "B", quantity: "1", net: "0.03", tax: "0", rate: "19", knownRate: true}}, "0")
	if o.ReviewReason != "" || len(o.SourceTaxes) != 1 || o.SourceTaxes[0] != (model.SourceTax{Rate: "19", Basis: "0.06", Amount: "0.00"}) {
		t.Fatalf("line rounding: %q %+v", o.ReviewReason, o.SourceTaxes)
	}
	// More than line rounding explains is still a mismatch.
	o.Lines, o.SourceTaxes = nil, nil
	o.ReviewReason = ""
	o.Gross = "1.00"
	finishOrder(&o, []normalizedLine{{name: "A", quantity: "1", net: "1.00", tax: "0", rate: "19", knownRate: true}}, "0")
	if !strings.Contains(o.ReviewReason, "Steuerberechnung") || len(o.SourceTaxes) != 0 {
		t.Fatalf("tax mismatch accepted: %q %+v", o.ReviewReason, o.SourceTaxes)
	}
}

func TestOrdersWithoutTaxAndLineRounding(t *testing.T) {
	// A shop under § 19 UStG sends no tax data; the lines are untaxed, not unclear.
	o := Order{Currency: "EUR", Gross: "45.00", CreatedAt: "2026-10-01T09:00:00Z", UpdatedAt: "2026-10-01T09:00:00Z", Customer: model.Customer{Name: "Ada", Address: model.Address{Street: "Testweg 1", Postcode: "10115", City: "Berlin", Country: "DE"}}}
	finishOrder(&o, []normalizedLine{{id: "a", name: "Kurs", quantity: "1", net: "40.00", tax: "0"}, {id: "s", name: "Versand", quantity: "1", net: "5.00", tax: "0"}}, "0")
	if o.ReviewReason != "" || len(o.Lines) != 2 || o.Lines[0].TaxCategory != "Z" || o.Lines[0].TaxRate != "0" || len(o.SourceTaxes) != 0 {
		t.Fatalf("untaxed order: %q %+v", o.ReviewReason, o.Lines)
	}
	// Two items at 9.99 € gross: 8.39 net and 1.60 tax each, 3.20 in total,
	// while 16.78 at 19 % gives 3.19. A free gift joins the 19 % group.
	o = Order{Currency: "EUR", Gross: "19.98", CreatedAt: "2026-10-01T09:00:00Z", UpdatedAt: "2026-10-01T09:00:00Z", Customer: o.Customer}
	finishOrder(&o, []normalizedLine{{id: "a", name: "A", quantity: "1", net: "8.39", tax: "1.60", rate: "19", knownRate: true}, {id: "b", name: "B", quantity: "1", net: "8.39", tax: "1.60", rate: "19", knownRate: true}, {id: "g", name: "Geschenk", quantity: "1", net: "0", tax: "0"}}, "3.20")
	if o.ReviewReason != "" || len(o.SourceTaxes) != 1 || o.SourceTaxes[0] != (model.SourceTax{Rate: "19", Basis: "16.78", Amount: "3.20"}) {
		t.Fatalf("line rounding: %q %+v", o.ReviewReason, o.SourceTaxes)
	}
	if gift := o.Lines[2]; gift.TaxCategory != "S" || gift.TaxRate != "19" {
		t.Fatalf("free line: %+v", gift)
	}
	d := model.NewDraft()
	d.Customer, d.Currency, d.Lines, d.Adjustments, d.SourceTaxes = o.Customer, o.Currency, o.Lines, o.Adjustments, o.SourceTaxes
	d.TaxTreatment = "mixed"
	invoice, err := engine.New().Build(d, model.Company{})
	if err != nil || invoice.GrandTotal.StringFixed(2) != "19.98" || invoice.TaxTotal.StringFixed(2) != "3.20" {
		t.Fatalf("invoice=%+v, err=%v", invoice, err)
	}
	// Changing a line changes the basis; the shop's amount no longer applies.
	d.Lines[0].UnitPrice = "8.40"
	invoice, err = engine.New().Build(d, model.Company{})
	if err != nil || invoice.TaxTotal.StringFixed(2) != "3.19" {
		t.Fatalf("edited invoice=%+v, err=%v", invoice, err)
	}
}

func TestFailedPagesReturnNoPartialOrdersAndRedactErrors(t *testing.T) {
	for _, kind := range []string{"http", "graphql", "cursor"} {
		t.Run(kind, func(t *testing.T) {
			calls := 0
			c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
				calls++
				if calls == 1 || kind == "cursor" {
					json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{"orders": map[string]any{"nodes": []shopOrder{validShopOrder()}, "pageInfo": map[string]any{"hasNextPage": true, "endCursor": "same"}}}})
					return
				}
				if kind == "http" {
					w.WriteHeader(401)
					fmt.Fprint(w, "private-token")
					return
				}
				fmt.Fprint(w, `{"errors":[{"message":"private-token"}],"data":null}`)
			})
			orders, err := c.FetchOrders(context.Background(), testShopConn(), Credentials{AccessToken: "private-token"}, time.Now())
			if err == nil || orders != nil || strings.Contains(err.Error(), "private-token") || strings.Contains(SafeError(err), "private-token") {
				t.Fatalf("orders=%d err=%v", len(orders), err)
			}
		})
	}
	if strings.Contains(SafeError(errors.New("private-token")), "private-token") {
		t.Fatal("unknown error leaked")
	}
}

func TestRedirectDoesNotSendCredentials(t *testing.T) {
	var redirected atomic.Int32
	receiver := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { redirected.Add(1) }))
	defer receiver.Close()
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, receiver.URL, http.StatusTemporaryRedirect)
	})
	_, err := c.FetchOrders(context.Background(), testShopConn(), Credentials{AccessToken: "private-token"}, time.Now())
	if err == nil || redirected.Load() != 0 {
		t.Fatalf("redirect followed: %v", err)
	}
}

func TestResponseLimitAndCancellation(t *testing.T) {
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, strings.Repeat(" ", maxResponseBytes+1)) })
	orders, err := c.FetchOrders(context.Background(), testShopConn(), Credentials{AccessToken: "private-token"}, time.Now())
	if err == nil || orders != nil {
		t.Fatal("oversized response accepted")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = c.FetchOrders(ctx, testShopConn(), Credentials{AccessToken: "private-token"}, time.Now())
	if err == nil {
		t.Fatal("cancel ignored")
	}
}

func TestShopifyHistoricalScopePreventsSilentOmission(t *testing.T) {
	for _, allowed := range []bool{false, true} {
		t.Run(fmt.Sprint(allowed), func(t *testing.T) {
			c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
				scopes := []map[string]string{{"handle": "read_orders"}}
				if allowed {
					scopes = append(scopes, map[string]string{"handle": "read_all_orders"})
				}
				json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{"currentAppInstallation": map[string]any{"accessScopes": scopes}, "orders": map[string]any{"nodes": []shopOrder{}, "pageInfo": map[string]bool{"hasNextPage": false}}}})
			})
			_, err := c.FetchOrders(context.Background(), testShopConn(), Credentials{AccessToken: "private-token"}, time.Now().AddDate(0, 0, -90))
			if (err == nil) != allowed {
				t.Fatalf("scope accepted=%v err=%v", allowed, err)
			}
		})
	}
}

func TestPrecisionAndUnboundedDecimalsRequireReview(t *testing.T) {
	raw := validShopOrder()
	raw.Total = bag("113.051")
	o := normalizeShopify(raw)
	if o.Gross != "113.051" || !strings.Contains(o.ReviewReason, "Nachkommastellen") {
		t.Fatalf("precision lost: %+v", o)
	}
	raw = validShopOrder()
	raw.Lines.Nodes[0].Net = bag("1e999999999")
	o = normalizeShopify(raw)
	if !strings.Contains(o.ReviewReason, "Ungültige") {
		t.Fatal("unbounded amount accepted")
	}
}

func TestFreeShippingAddsNoZeroRatedLine(t *testing.T) {
	shop := validShopOrder()
	shop.Total, shop.Tax = bag("107.1"), bag("17.1")
	shop.Shipping.Nodes = []shopShipping{{ID: "ship-free", Title: "Kostenloser Versand", Price: bag("0.0")}}
	woo := validWooOrder()
	woo.Total, woo.Tax, woo.Fees = "107.10", "17.10", nil
	woo.Shipping = []wooLine{{ID: 2, MethodTitle: "Kostenloser Versand", Total: "0.00", Tax: "0.00"}}
	for name, o := range map[string]Order{"shopify": normalizeShopify(shop), "woocommerce": normalizeWoo(woo, map[int64]wooRate{9: {rate: "19", known: true}})} {
		if o.ReviewReason != "" || o.Gross != "107.10" || len(o.Lines) != 1 {
			t.Fatalf("%s: %+v", name, o)
		}
		for _, line := range o.Lines {
			if line.TaxCategory != "S" {
				t.Fatalf("%s: zero-rated line %+v", name, line)
			}
		}
	}
}
