package shops

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"papierleicht.de/e-rechnung-studio/internal/engine"
	"papierleicht.de/e-rechnung-studio/internal/model"
)

func prestaTestResources(t *testing.T) map[string]any {
	t.Helper()
	// The Webservice returns scalar strings, numeric IDs and multilingual
	// state names. Detail tax associations refer to /taxes, not product prices.
	raw := `{
"orders":[{"id":1,"reference":"ABCDEFGHI","id_address_invoice":"4","id_customer":"9","id_currency":"1","current_state":"2","date_add":"2026-03-10 12:00:00","date_upd":"2026-03-10 12:01:00","total_paid_tax_incl":"113.050000","total_paid_tax_excl":"95.000000","total_paid_real":"113.050000","total_products":"100.000000","total_products_wt":"119.000000","total_discounts_tax_excl":"10.000000","total_discounts_tax_incl":"11.900000","total_shipping_tax_excl":"5.000000","total_shipping_tax_incl":"5.950000","carrier_tax_rate":"19.000","total_wrapping_tax_excl":"0.000000","total_wrapping_tax_incl":"0.000000","associations":{"order_rows":[{"id":"11"}]}}],
"addresses/4":{"id":4,"firstname":"Erika","lastname":"Beispiel","company":"","address1":"Testweg 1","address2":"","postcode":"10115","city":"Berlin","id_country":"1","vat_number":"DE123456789"},
"customers/9":{"id":9,"email":"erika@example.com"},
"currencies/1":{"id":1,"iso_code":"EUR"},
"countries/1":{"id":1,"iso_code":"DE"},
"order_states/2":{"id":2,"paid":"1","shipped":"0","deleted":"0","name":[{"id":"1","value":"Payment accepted"}]},
"taxes/1":{"id":1,"rate":"19.000000"},
"order_payments":[{"id":15,"order_reference":"ABCDEFGHI","id_currency":"1","amount":"113.050000"}],
"order_slip":[],
"order_details":[{"id":11,"id_order":"1","product_name":"Artikel","product_reference":"SKU-1","product_quantity":"1","product_quantity_return":"0","product_quantity_refunded":"0","total_refunded_tax_excl":"0.000000","total_refunded_tax_incl":"0.000000","ecotax":"0.000000","total_price_tax_excl":"100.000000","total_price_tax_incl":"119.000000","associations":{"taxes":[{"id":"1"}]}}]
}`
	var resources map[string]any
	if err := json.Unmarshal([]byte(raw), &resources); err != nil {
		t.Fatal(err)
	}
	return resources
}

func prestaTestHandler(t *testing.T, resources map[string]any, failResource string) http.HandlerFunc {
	t.Helper()
	return func(w http.ResponseWriter, r *http.Request) {
		user, password, ok := r.BasicAuth()
		if r.Method != http.MethodGet || !ok || user != "private-api-key" || password != "" || strings.Contains(r.URL.RawQuery, "private") {
			t.Error("unsafe PrestaShop authentication or write request")
		}
		if r.URL.Query().Get("output_format") != "JSON" {
			t.Error("missing JSON output format")
		}
		resource := strings.TrimPrefix(r.URL.Path, "/shop/api/")
		if resource == failResource {
			http.Error(w, "private-api-key", http.StatusForbidden)
			return
		}
		value, ok := resources[resource]
		if !ok {
			t.Errorf("unexpected resource %q", resource)
			http.NotFound(w, r)
			return
		}
		key := resource
		if strings.Contains(resource, "/") {
			key = map[string]string{"addresses": "address", "customers": "customer", "currencies": "currency", "countries": "country", "order_states": "order_state", "taxes": "tax"}[strings.Split(resource, "/")[0]]
		} else {
			if r.URL.Query().Get("display") != "full" || r.URL.Query().Get("sort") != "[id_ASC]" || r.URL.Query().Get("limit") != "0,100" {
				t.Error("incomplete resource request")
			}
			if resource == "orders" && (r.URL.Query().Get("date") != "1" || r.URL.Query().Get("filter[date_add]") != "[2026-01-01 00:00:00,9999-12-31 23:59:59]") {
				t.Error("creation-window filter is not fixed at ImportFrom")
			}
			if resource == "order_details" || resource == "order_slip" {
				if r.URL.Query().Get("filter[id_order]") != "[1]" {
					t.Error("missing related order filter")
				}
			}
			if resource == "order_payments" && r.URL.Query().Get("filter[order_reference]") != "[ABCDEFGHI]" {
				t.Error("missing payment reference filter")
			}
			if resource == "order_slip" {
				key = "order_slips"
			}
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{key: value})
	}
}

func fetchTestPresta(t *testing.T, resources map[string]any, failResource string) ([]Order, error) {
	t.Helper()
	c := testClient(t, prestaTestHandler(t, resources, failResource))
	return c.fetchPrestaShop(context.Background(), Connection{Provider: "prestashop", URL: "https://store.example/shop", ImportFrom: "2026-01-01"}, Credentials{APIKey: "private-api-key"}, time.Date(2026, 10, 15, 0, 0, 0, 0, time.UTC))
}

func TestPrestaShopInvoiceUsesNetPricesDiscountShippingAndReceipts(t *testing.T) {
	orders, err := fetchTestPresta(t, prestaTestResources(t), "")
	if err != nil || len(orders) != 1 {
		t.Fatalf("orders=%+v, err=%v", orders, err)
	}
	o := orders[0]
	if o.ReviewReason != "" || !o.Paid || o.Gross != "113.05" || len(o.Lines) != 2 || len(o.Adjustments) != 1 || o.Customer.Address.Country != "DE" || o.Customer.VATID != "DE123456789" {
		t.Fatalf("mapping=%+v", o)
	}
	d := model.NewDraft()
	d.Customer = o.Customer
	d.Currency = o.Currency
	d.Lines = o.Lines
	d.Adjustments = o.Adjustments
	d.TaxTreatment = "mixed"
	d.PrepaidAmount = o.Gross
	invoice, err := engine.New().Build(d, model.Company{})
	if err != nil || invoice.GrandTotal.StringFixed(2) != o.Gross || !invoice.DuePayableAmount.IsZero() {
		t.Fatalf("invoice=%+v, err=%v", invoice, err)
	}
}

func TestPrestaShopDoesNotInferPaymentFromShipping(t *testing.T) {
	resources := prestaTestResources(t)
	resources["orders"].([]any)[0].(map[string]any)["total_paid_real"] = "0.000000"
	state := resources["order_states/2"].(map[string]any)
	state["paid"], state["shipped"], state["name"] = "0", "1", []any{map[string]any{"id": 1, "value": "Shipped"}}
	resources["order_payments"] = []any{}
	orders, err := fetchTestPresta(t, resources, "")
	if err != nil || len(orders) != 1 || orders[0].Paid || orders[0].ReviewReason != "" {
		t.Fatalf("unpaid shipping=%+v, %v", orders, err)
	}
}

// prestaShop8Handler answers like PrestaShop 8 and later: an empty list is a
// bare [] and a single record leaves out its empty and "0" fields.
func prestaShop8Handler(t *testing.T, resources map[string]any) http.HandlerFunc {
	inner := prestaTestHandler(t, resources, "")
	return func(w http.ResponseWriter, r *http.Request) {
		recorder := httptest.NewRecorder()
		inner(recorder, r)
		var envelope map[string]any
		if err := json.Unmarshal(recorder.Body.Bytes(), &envelope); err != nil {
			t.Error(err)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		for _, value := range envelope {
			switch v := value.(type) {
			case []any:
				if len(v) == 0 {
					_, _ = w.Write([]byte("[]"))
					return
				}
			case map[string]any:
				for field, item := range v {
					if item == "" || item == "0" {
						delete(v, field)
					}
				}
			}
		}
		_ = json.NewEncoder(w).Encode(envelope)
	}
}

func TestPrestaShop8EmptyListsAndUnpaidState(t *testing.T) {
	resources := prestaTestResources(t)
	resources["orders"].([]any)[0].(map[string]any)["total_paid_real"] = "0.000000"
	resources["order_states/2"].(map[string]any)["paid"] = "0"
	resources["order_payments"] = []any{}
	c := testClient(t, prestaShop8Handler(t, resources))
	orders, err := c.fetchPrestaShop(context.Background(), Connection{Provider: "prestashop", URL: "https://store.example/shop", ImportFrom: "2026-01-01"}, Credentials{APIKey: "private-api-key"}, time.Date(2026, 10, 15, 0, 0, 0, 0, time.UTC))
	if err != nil || len(orders) != 1 || orders[0].Paid || orders[0].ReviewReason != "" {
		t.Fatalf("unpaid order from PrestaShop 8=%+v, %v", orders, err)
	}
}

func TestPrestaShopAmbiguousPaymentsRefundsAndTaxesNeedReview(t *testing.T) {
	for _, scenario := range []string{"partial payment", "wrong payment currency", "cancelled", "refund slip", "refunded quantity", "changed tax definition", "compound taxes", "missing tax association", "missing payment amount", "shipping tax mismatch", "wrapping"} {
		t.Run(scenario, func(t *testing.T) {
			resources := prestaTestResources(t)
			order := resources["orders"].([]any)[0].(map[string]any)
			detail := resources["order_details"].([]any)[0].(map[string]any)
			switch scenario {
			case "partial payment":
				order["total_paid_real"] = "50"
				resources["order_payments"].([]any)[0].(map[string]any)["amount"] = "50"
			case "wrong payment currency":
				resources["order_payments"].([]any)[0].(map[string]any)["id_currency"] = "2"
			case "cancelled":
				resources["order_states/2"].(map[string]any)["name"] = []any{map[string]any{"id": 1, "value": "Storniert"}}
			case "refund slip":
				resources["order_slip"] = []any{map[string]any{"id": 10, "id_order": 1}}
			case "refunded quantity":
				detail["product_quantity_refunded"] = "1"
			case "changed tax definition":
				resources["taxes/1"].(map[string]any)["rate"] = "20.00"
			case "compound taxes":
				detail["associations"] = map[string]any{"taxes": []any{map[string]any{"id": 1}, map[string]any{"id": 2}}}
			case "missing tax association":
				detail["associations"] = map[string]any{"taxes": []any{}}
			case "missing payment amount":
				delete(order, "total_paid_real")
			case "shipping tax mismatch":
				order["carrier_tax_rate"] = "7"
			case "wrapping":
				order["total_wrapping_tax_incl"] = "1.19"
				order["total_wrapping_tax_excl"] = "1.00"
			}
			orders, err := fetchTestPresta(t, resources, "")
			if err != nil || len(orders) != 1 || orders[0].ReviewReason == "" {
				t.Fatalf("unsafe mapping=%+v, %v", orders, err)
			}
		})
	}
}

func TestPrestaShopMissingPermissionsAndIncompleteRowsAbortFetch(t *testing.T) {
	for _, resource := range []string{"taxes/1", "order_payments", "order_details"} {
		t.Run(resource, func(t *testing.T) {
			orders, err := fetchTestPresta(t, prestaTestResources(t), resource)
			if err == nil || orders != nil || strings.Contains(err.Error(), "private-api-key") {
				t.Fatalf("unsafe partial result=%+v, %v", orders, err)
			}
		})
	}
	resources := prestaTestResources(t)
	resources["order_details"] = []any{}
	if orders, err := fetchTestPresta(t, resources, ""); err == nil || orders != nil {
		t.Fatalf("missing positions accepted=%+v, %v", orders, err)
	}
}

func TestPrestaShopPaginationRescansHistoryAndRejectsRepeats(t *testing.T) {
	for _, repeat := range []bool{false, true} {
		t.Run(map[bool]string{false: "complete", true: "repeated page"}[repeat], func(t *testing.T) {
			resources := prestaTestResources(t)
			fullHandler := prestaTestHandler(t, resources, "")
			pages := 0
			c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/shop/api/orders" {
					fullHandler(w, r)
					return
				}
				pages++
				if r.URL.Query().Get("date") != "1" || strings.Contains(r.URL.Query().Get("filter[date_add]"), "2026-10-15") {
					t.Error("moving checkpoint creation filter")
				}
				limit := r.URL.Query().Get("limit")
				batch := []any{}
				if limit == "0,100" || repeat {
					for i := 1; i <= 100; i++ {
						batch = append(batch, map[string]any{"id": i, "reference": "OLD" + strconv.Itoa(i), "date_add": "2025-01-01 00:00:00"})
					}
				} else if limit == "100,100" {
					row := resources["orders"].([]any)[0].(map[string]any)
					row["id"] = 101
					batch = append(batch, row)
					resources["order_details"].([]any)[0].(map[string]any)["id_order"] = "101"
					// This test exercises paging only; map the linked collection
					// filter back to the one-order shared handler after assertion.
				} else {
					t.Fatalf("unexpected page %s", limit)
				}
				json.NewEncoder(w).Encode(map[string]any{"orders": batch})
			})
			// For complete pagination avoid unrelated requests by filtering all
			// returned source dates before the configured import date.
			orders, err := c.fetchPrestaShop(context.Background(), Connection{URL: "https://store.example/shop", ImportFrom: "2027-01-01"}, Credentials{APIKey: "private-api-key"}, time.Now())
			if repeat {
				if err == nil || orders != nil {
					t.Fatalf("repeat accepted=%+v, %v", orders, err)
				}
			} else if err != nil || len(orders) != 0 || pages != 2 {
				t.Fatalf("paging=%+v, %v, pages=%d", orders, err, pages)
			}
		})
	}
}
