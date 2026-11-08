package shops

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"papierleicht.de/e-rechnung-studio/internal/engine"
	"papierleicht.de/e-rechnung-studio/internal/model"
)

func gambioTestOrder(t *testing.T, netPrices bool) map[string]any {
	t.Helper()
	// Official GXOrder shape; finalPrice is the complete line amount, not its
	// unit price. Both customer-group price-display modes must reconcile.
	raw := `{"id":1,"statusId":1,"purchaseDate":"2026-03-10 12:00:00","currencyCode":"EUR","paymentType":{"module":"invoice"},"customer":{"id":4,"email":"erika@example.com","vatId":""},"addresses":{"billing":{"firstname":"Erika","lastname":"Beispiel","company":"","street":"Testweg","houseNumber":"1","additionalAddressInfo":"","suburb":"","postcode":"10115","city":"Berlin","countryId":81}},"items":[{"id":10,"model":"SKU-1","name":"Artikel","quantity":1,"price":119,"finalPrice":119,"tax":19,"isTaxAllowed":true,"discount":0}],"totals":[{"class":"ot_subtotal","value":119},{"class":"ot_shipping","value":5.95},{"class":"ot_discount","value":11.90},{"class":"ot_tax","value":18.05},{"class":"ot_total","value":113.05}],"statusHistory":[{"statusId":1,"dateAdded":"2026-03-10 12:01:00"}]}`
	var result map[string]any
	if err := json.Unmarshal([]byte(raw), &result); err != nil {
		t.Fatal(err)
	}
	if netPrices {
		result["items"].([]any)[0].(map[string]any)["finalPrice"] = 100
		// GXOrder's embedded official example serializes this flag as a string;
		// GXOrderItem's own example uses a JSON boolean. Both are supported.
		result["items"].([]any)[0].(map[string]any)["isTaxAllowed"] = "true"
		totals := result["totals"].([]any)
		totals[0].(map[string]any)["value"], totals[1].(map[string]any)["value"], totals[2].(map[string]any)["value"] = 100, 5, 10
	}
	return result
}

func gambioTestList(id int, status string) map[string]any {
	return map[string]any{"id": id, "statusId": 1, "statusName": status, "purchaseDate": "2026-03-10 12:00:00", "withdrawalIds": []int{}}
}

func TestGambioPriceModesShippingDiscountAndReadOnlyAuth(t *testing.T) {
	for _, netPrices := range []bool{false, true} {
		t.Run(map[bool]string{false: "gross customer", true: "net customer"}[netPrices], func(t *testing.T) {
			order := gambioTestOrder(t, netPrices)
			c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
				user, pass, ok := r.BasicAuth()
				if r.Method != http.MethodGet || !ok || user != "api-user" || pass != "private-password" || strings.Contains(r.URL.RawQuery, "private") {
					t.Error("unsafe merchant request")
				}
				w.Header().Set("Content-Type", "application/json")
				switch r.URL.Path {
				case "/shop/api.php/v2/orders":
					if r.URL.Query().Get("sort") != "+id" || r.URL.Query().Get("modified") != "" {
						t.Error("unsafe mutable-history filter")
					}
					json.NewEncoder(w).Encode([]any{gambioTestList(1, "Offen")})
				case "/shop/api.php/v2/orders/1":
					json.NewEncoder(w).Encode(order)
				case "/shop/api.php/v2/countries/81":
					json.NewEncoder(w).Encode([]any{map[string]any{"id": 81, "iso2": "de"}})
				default:
					t.Errorf("unexpected endpoint %s", r.URL.Path)
					http.NotFound(w, r)
				}
			})
			orders, err := c.fetchGambio(context.Background(), Connection{Provider: "gambio", URL: "https://store.example/shop", ImportFrom: "2026-01-01"}, Credentials{Username: "api-user", Password: "private-password"}, time.Date(2026, 10, 15, 0, 0, 0, 0, time.UTC))
			if err != nil || len(orders) != 1 {
				t.Fatalf("orders=%+v, err=%v", orders, err)
			}
			o := orders[0]
			if o.ReviewReason != "" || o.Paid || o.Gross != "113.05" || len(o.Lines) != 2 || len(o.Adjustments) != 1 || o.Customer.Address.Country != "DE" || o.Customer.Address.Street != "Testweg 1" {
				t.Fatalf("mapping=%+v", o)
			}
			d := model.NewDraft()
			d.Customer = o.Customer
			d.Currency = o.Currency
			d.Lines = o.Lines
			d.Adjustments = o.Adjustments
			d.TaxTreatment = "mixed"
			invoice, err := engine.New().Build(d, model.Company{})
			if err != nil || invoice.GrandTotal.StringFixed(2) != o.Gross {
				t.Fatalf("invoice=%+v, err=%v", invoice, err)
			}
		})
	}
}

func TestGambioAmbiguousStatesAndFinancialDataStayReviewable(t *testing.T) {
	for _, scenario := range []string{"shipped", "cancelled", "unknown payment", "unknown total", "mismatch", "mixed shipping", "withdrawal", "changed status during fetch"} {
		t.Run(scenario, func(t *testing.T) {
			order := gambioTestOrder(t, false)
			list := gambioTestList(1, "Offen")
			switch scenario {
			case "shipped":
				list["statusName"] = "Versendet"
				order["paymentType"] = map[string]any{"module": "paypal"}
			case "cancelled":
				list["statusName"] = "Storniert"
			case "unknown payment":
				order["paymentType"] = map[string]any{"module": "custom-payment"}
			case "unknown total":
				order["totals"] = append(order["totals"].([]any), map[string]any{"class": "ot_gv", "value": 10})
			case "mismatch":
				order["totals"].([]any)[4].(map[string]any)["value"] = 114.05
			case "mixed shipping":
				order["items"] = append(order["items"].([]any), map[string]any{"id": 11, "name": "Ermäßigt", "quantity": 1, "finalPrice": 107, "tax": 7, "isTaxAllowed": true})
			case "withdrawal":
				list["withdrawalIds"] = []int{2}
			case "changed status during fetch":
				order["statusId"] = 2
			}
			encoded, _ := json.Marshal(order)
			raw, err := gambioSingleton[gambioOrder](encoded)
			if err != nil {
				t.Fatal(err)
			}
			encoded, _ = json.Marshal(list)
			var row gambioListOrder
			if err = json.Unmarshal(encoded, &row); err != nil {
				t.Fatal(err)
			}
			mapped := normalizeGambio(raw, row, "DE")
			if mapped.ReviewReason == "" || mapped.Paid {
				t.Fatalf("unsafe automatic mapping=%+v", mapped)
			}
		})
	}
}

func TestGambioPaginationHonorsLinksAndFailsWholeFetch(t *testing.T) {
	for _, fail := range []bool{false, true} {
		t.Run(map[bool]string{false: "complete", true: "detail failure"}[fail], func(t *testing.T) {
			c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				switch r.URL.Path {
				case "/api.php/v2/orders":
					id := 1
					if r.URL.Query().Get("page") == "1" {
						w.Header().Set("Link", `<https://store.example/api.php/v2/orders?page=2>; rel="next"`)
					} else {
						id = 2
					}
					json.NewEncoder(w).Encode([]any{gambioTestList(id, "Offen")})
				case "/api.php/v2/orders/1", "/api.php/v2/orders/2":
					if fail && strings.HasSuffix(r.URL.Path, "/2") {
						http.Error(w, "private-password", 500)
						return
					}
					order := gambioTestOrder(t, false)
					if strings.HasSuffix(r.URL.Path, "/2") {
						order["id"] = 2
					}
					json.NewEncoder(w).Encode([]any{order})
				case "/api.php/v2/countries/81":
					json.NewEncoder(w).Encode(map[string]any{"id": 81, "iso2": "DE"})
				default:
					t.Errorf("unexpected endpoint %s", r.URL.Path)
					http.NotFound(w, r)
				}
			})
			orders, err := c.fetchGambio(context.Background(), Connection{URL: "https://store.example", ImportFrom: "2026-01-01"}, Credentials{Username: "u", Password: "private-password"}, time.Now())
			if fail {
				if err == nil || orders != nil || strings.Contains(err.Error(), "private-password") {
					t.Fatalf("partial result or unsafe error: %+v, %v", orders, err)
				}
			} else if err != nil || len(orders) != 2 {
				t.Fatalf("pagination=%+v, %v", orders, err)
			}
		})
	}
}
