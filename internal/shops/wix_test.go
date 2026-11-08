package shops

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"
)

func wixConnectionFixture() Connection {
	return Connection{Provider: "wix", URL: "https://store.example", SiteID: "12345678-1234-1234-1234-123456789abc"}
}

func wixOrderFixture(t *testing.T) wixOrder {
	t.Helper()
	// Current REST wire fields from Wix's Order schema (taxInfo, not the
	// deprecated taxDetails). Source totals already include line discounts.
	var order wixOrder
	err := json.Unmarshal([]byte(`{
	 "id":"wix-order-1","number":"10001","createdDate":"2026-10-10T10:00:00Z","updatedDate":"2026-10-15T10:00:00Z",
	 "currency":"EUR","status":"APPROVED","paymentStatus":"PAID","taxIncludedInPrices":true,
	 "buyerInfo":{"memberId":"buyer-1","email":"ada@example.com"},
	 "billingInfo":{"address":{"country":"DE","city":"Berlin","postalCode":"10115","addressLine":"Testweg 1","addressLine2":"2. OG","subdivision":"DE-BE"},"contactDetails":{"firstName":"Ada","lastName":"Lovelace","company":"Ada GmbH"}},
	 "lineItems":[{"id":"item-1","productName":{"original":"Artikel nach Rabatt"},"quantity":3,"totalPriceBeforeTax":{"amount":"90.00"},"totalPriceAfterTax":{"amount":"107.10"},"totalDiscount":{"amount":"11.90"},"taxInfo":{"taxAmount":{"amount":"17.10"},"taxRate":"0.19"},"paymentOption":"FULL_PAYMENT_ONLINE"}],
	 "shippingInfo":{"title":"Paket","cost":{"totalPriceBeforeTax":{"amount":"5.00"},"totalPriceAfterTax":{"amount":"5.95"},"taxInfo":{"taxAmount":{"amount":"0.95"},"taxRate":"0.19"}}},
	 "additionalFees":[{"id":"fee-1","name":"Verpackung","priceBeforeTax":{"amount":"2.00"},"priceAfterTax":{"amount":"2.38"},"taxInfo":{"taxAmount":{"amount":"0.38"},"taxRate":"0.19"}}],
	 "priceSummary":{"total":{"amount":"115.43"},"tax":{"amount":"18.43"},"shipping":{"amount":"5.00"}},"taxInfo":{"totalTax":{"amount":"18.43"}},
	 "balanceSummary":{"paid":{"amount":"115.43"},"refunded":{"amount":"0"},"pendingRefund":{"amount":"0"}}
	}`), &order)
	if err != nil {
		t.Fatal(err)
	}
	return order
}

func TestWixAuthenticationPaginationAndReadOnlyEndpoint(t *testing.T) {
	since := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	calls := 0
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.Method != "POST" || r.URL.Path != "/ecom/v1/orders/search" || r.URL.RawQuery != "" || r.Header.Get("Authorization") != "private-wix-key" || r.Header.Get("wix-site-id") != wixConnectionFixture().SiteID {
			t.Errorf("wrong endpoint or authentication")
		}
		var body struct {
			Search struct {
				Filter map[string]json.RawMessage `json:"filter"`
				Sort   []struct {
					Field string `json:"fieldName"`
					Order string `json:"order"`
				} `json:"sort"`
				Paging struct {
					Limit  int    `json:"limit"`
					Cursor string `json:"cursor"`
				} `json:"cursorPaging"`
			} `json:"search"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
			return
		}
		if body.Search.Paging.Limit != 100 || (calls == 1 && (!strings.Contains(string(body.Search.Filter["updatedDate"]), since.Format(time.RFC3339)) || !strings.Contains(string(body.Search.Filter["status"]), "REJECTED") || len(body.Search.Sort) != 1 || body.Search.Sort[0].Field != "updatedDate")) {
			t.Errorf("invalid search criteria")
		}
		raw := wixOrderFixture(t)
		if calls == 1 {
			if body.Search.Paging.Cursor != "" {
				t.Error("unexpected initial cursor")
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"orders": []wixOrder{raw}, "metadata": map[string]any{"count": 1, "hasNext": true, "cursors": map[string]string{"next": "page-2"}}})
		} else {
			if len(body.Search.Filter) != 0 || len(body.Search.Sort) != 0 {
				t.Error("cursor continued with new filter or sorting")
			}
			if body.Search.Paging.Cursor != "page-2" {
				t.Error("cursor not continued")
			}
			raw.ID = "wix-order-2"
			_ = json.NewEncoder(w).Encode(map[string]any{"orders": []wixOrder{raw}, "metadata": map[string]any{"count": 1, "hasNext": false}})
		}
	})
	orders, err := c.FetchOrders(context.Background(), wixConnectionFixture(), Credentials{APIKey: "private-wix-key"}, since)
	if err != nil || len(orders) != 2 || calls != 2 {
		t.Fatalf("orders=%+v calls=%d err=%v", orders, calls, err)
	}
	for _, o := range orders {
		if o.ReviewReason != "" || !o.Paid || len(o.Lines) != 3 || o.Lines[0].UnitPrice != "30" || o.Gross != "115.43" || o.Customer.Name != "Ada GmbH" || o.Customer.ContactName != "Ada Lovelace" || o.Customer.Address.Additional != "2. OG, DE-BE" {
			t.Fatalf("mapping: %+v", o)
		}
	}
}

func TestWixRejectsIncompletePagination(t *testing.T) {
	for _, scenario := range []string{"missing metadata", "missing orders", "missing cursor", "repeated cursor", "duplicate order", "wrong count", "empty next page"} {
		t.Run(scenario, func(t *testing.T) {
			calls := 0
			c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
				calls++
				o := wixOrderFixture(t)
				o.ID += string(rune('a' + calls))
				orders := []wixOrder{o}
				meta := map[string]any{"count": 1, "hasNext": true, "cursors": map[string]string{"next": "same-cursor"}}
				response := map[string]any{"orders": orders, "metadata": meta}
				switch scenario {
				case "missing metadata":
					delete(response, "metadata")
				case "missing orders":
					delete(response, "orders")
				case "missing cursor":
					delete(meta, "cursors")
				case "duplicate order":
					response["orders"] = []wixOrder{wixOrderFixture(t)}
				case "wrong count":
					meta["count"] = 2
				case "empty next page":
					response["orders"], meta["count"] = []wixOrder{}, 0
				}
				_ = json.NewEncoder(w).Encode(response)
			})
			orders, err := c.FetchOrders(context.Background(), wixConnectionFixture(), Credentials{APIKey: "key"}, time.Now())
			if err == nil || len(orders) != 0 {
				t.Fatalf("partial import accepted: %+v %v", orders, err)
			}
		})
	}
}

func TestWixReviewCases(t *testing.T) {
	for _, scenario := range []string{"partial", "refund", "chargeback", "pending refund", "pending order", "cancelled", "unknown payment", "paid mismatch", "missing address", "mixed taxes", "wrong tax", "wrong gross", "gift card", "refunded quantity", "deposit", "undetermined", "decimal quantity", "exempt", "missing shipping"} {
		t.Run(scenario, func(t *testing.T) {
			raw := wixOrderFixture(t)
			switch scenario {
			case "partial":
				raw.PaymentStatus = "PARTIALLY_PAID"
			case "refund":
				raw.Balance.Refunded.Amount = "10"
			case "chargeback":
				raw.Balance.Chargeback.Amount = "10"
			case "pending refund":
				raw.Balance.PendingRefund.Amount = "10"
			case "pending order":
				raw.Status = "PENDING"
			case "cancelled":
				raw.Status = "CANCELED"
			case "unknown payment":
				raw.PaymentStatus = "MYSTERY"
			case "paid mismatch":
				raw.Balance.Paid.Amount = "100"
			case "missing address":
				raw.Billing.Address.Line = ""
			case "mixed taxes":
				raw.Lines[0].Tax.Breakdown = make([]struct {
					Rate   string   `json:"rate"`
					Amount wixPrice `json:"taxAmount"`
				}, 2)
			case "wrong tax":
				raw.Lines[0].Tax.Amount.Amount = "16.10"
			case "wrong gross":
				raw.Summary.Total.Amount = "115.44"
			case "gift card":
				raw.Lines[0].ItemType.Preset = "GIFT_CARD"
			case "refunded quantity":
				raw.Lines[0].RefundQuantity = "1"
			case "deposit":
				raw.Lines[0].PaymentOption = "DEPOSIT_ONLINE"
			case "undetermined":
				raw.Lines[0].Undetermined = true
			case "decimal quantity":
				raw.Lines[0].DecimalQuantity = json.RawMessage(`{"value":"2.5"}`)
			case "exempt":
				raw.TaxInfo.Exempt = true
			case "missing shipping":
				raw.Shipping = nil
			}
			o := normalizeWix(raw)
			if o.ReviewReason == "" {
				t.Fatalf("review missing: %+v", o)
			}
		})
	}
}

func TestWixLegacyTaxAndUnpaidOrder(t *testing.T) {
	raw := wixOrderFixture(t)
	raw.PaymentStatus, raw.Balance.Paid.Amount = "NOT_PAID", "0"
	for i := range raw.Lines {
		raw.Lines[i].LegacyTax = &wixLegacyTax{Rate: raw.Lines[i].Tax.Rate, Amount: raw.Lines[i].Tax.Amount}
		raw.Lines[i].Tax = nil
	}
	raw.Shipping.Cost.LegacyTax = &wixLegacyTax{Rate: "0.19", Amount: wixPrice{Amount: "0.95"}}
	raw.Shipping.Cost.Tax = nil
	o := normalizeWix(raw)
	if o.Paid || o.ReviewReason != "" {
		t.Fatalf("legacy unpaid mapping: %+v", o)
	}
}

func TestWixHTTPFailureDoesNotExposeSecrets(t *testing.T) {
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(403)
		_, _ = w.Write([]byte(`{"error":"private-wix-key"}`))
	})
	_, err := c.FetchOrders(context.Background(), wixConnectionFixture(), Credentials{APIKey: "private-wix-key"}, time.Now())
	if err == nil || strings.Contains(SafeError(err), "private-wix-key") {
		t.Fatal("unsafe API failure", err)
	}
}
