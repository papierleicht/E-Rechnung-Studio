package app

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"testing"

	"papierleicht.de/e-rechnung-studio/internal/shops"
)

func init() { shopSimulations["wix"] = simulateWix }

// simulateWix answers the eCommerce Orders API v1 like a German site with
// tax-inclusive prices: Search Orders with an API key and the site ID header,
// cursor paging with at most 100 orders per page.
func simulateWix(t *testing.T) shopSimulation {
	const apiKey, siteID = "IST.simulation-api-key", "4b8f3c2e-9d61-4a7f-8e25-c0d9b7a6f531"
	// Sorted by updatedDate. Wix omits empty fields, numbers line items per
	// order and writes amounts with varying decimals ("0", "0.00", "50.65").
	orders := []string{
		`{"id":"9a3c7e51-2f4b-4d8e-b6a1-7c5e0d9f2b14","number":"10001","createdDate":"2026-10-08T09:14:22.512Z","updatedDate":"2026-10-08T09:15:03.108Z",
		 "lineItems":[
		  {"id":"00000000-0000-0000-0000-000000000001","productName":{"original":"Notizbuch A5 Leinen"},"catalogReference":{"catalogItemId":"5e2f8a41-93c7-4b0d-a6e2-1f7d9c3b8e50","appId":"215238eb-22a5-4c36-9e7b-e7c08025e04e"},
		   "quantity":2,"physicalProperties":{"sku":"NB-A5-LEINEN","shippable":true},"itemType":{"preset":"PHYSICAL"},
		   "price":{"amount":"17.90"},"totalPriceBeforeTax":{"amount":"30.08"},"totalPriceAfterTax":{"amount":"35.80"},"paymentOption":"FULL_PAYMENT_ONLINE",
		   "taxInfo":{"taxAmount":{"amount":"5.72"},"taxableAmount":{"amount":"30.08"},"taxRate":"0.19","taxIncludedInPrice":true,
		    "taxBreakdown":[{"jurisdiction":"Germany","rate":"0.19","taxAmount":{"amount":"5.72"},"taxableAmount":{"amount":"30.08"},"taxType":"VAT","taxName":"MwSt.","jurisdictionType":"COUNTRY"}]},
		   "priceUndetermined":false,"fixedQuantity":false},
		  {"id":"00000000-0000-0000-0000-000000000002","productName":{"original":"Washi-Tape-Set (5 Rollen)"},"catalogReference":{"catalogItemId":"b81d4c2a-6e0f-4f3b-9a5d-7c2e8f1a6b39","appId":"215238eb-22a5-4c36-9e7b-e7c08025e04e"},
		   "quantity":1,"physicalProperties":{"sku":"WT-SET-5","shippable":true},"itemType":{"preset":"PHYSICAL"},
		   "price":{"amount":"9.90"},"totalPriceBeforeTax":{"amount":"8.32"},"totalPriceAfterTax":{"amount":"9.90"},"paymentOption":"FULL_PAYMENT_ONLINE",
		   "taxInfo":{"taxAmount":{"amount":"1.58"},"taxableAmount":{"amount":"8.32"},"taxRate":"0.19","taxIncludedInPrice":true,
		    "taxBreakdown":[{"jurisdiction":"Germany","rate":"0.19","taxAmount":{"amount":"1.58"},"taxableAmount":{"amount":"8.32"},"taxType":"VAT","taxName":"MwSt.","jurisdictionType":"COUNTRY"}]},
		   "priceUndetermined":false,"fixedQuantity":false}],
		 "buyerInfo":{"contactId":"2d7e9b13-4a8c-4f61-8e03-b5c9a1d7e264","email":"lena.hoffmann@example.com","memberId":"7f1a3c95-0b2e-4d84-a6f7-9e3b5c8d1a20"},
		 "paymentStatus":"PAID","fulfillmentStatus":"FULFILLED","buyerLanguage":"de","weightUnit":"KG","currency":"EUR","taxIncludedInPrices":true,"siteLanguage":"de",
		 "priceSummary":{"subtotal":{"amount":"45.70"},"shipping":{"amount":"4.95"},"tax":{"amount":"8.09"},"discount":{"amount":"0"},"total":{"amount":"50.65"},"totalAdditionalFees":{"amount":"0"}},
		 "billingInfo":{"address":{"country":"DE","city":"Köln","postalCode":"50667","addressLine":"Breite Straße 21","countryFullname":"Germany"},"contactDetails":{"firstName":"Lena","lastName":"Hoffmann","phone":"+49 221 5550123"}},
		 "shippingInfo":{"code":"ed5bbce2-9533-dff4-7db0-13702fd139c5","title":"Standardversand (DHL)","logistics":{"deliveryTime":"2-3 Werktage"},
		  "cost":{"price":{"amount":"4.95"},"totalPriceBeforeTax":{"amount":"4.16"},"totalPriceAfterTax":{"amount":"4.95"},
		   "taxInfo":{"taxAmount":{"amount":"0.79"},"taxableAmount":{"amount":"4.16"},"taxRate":"0.19","taxIncludedInPrice":true,
		    "taxBreakdown":[{"jurisdiction":"Germany","rate":"0.19","taxAmount":{"amount":"0.79"},"taxableAmount":{"amount":"4.16"},"taxType":"VAT","taxName":"MwSt.","jurisdictionType":"COUNTRY"}]},
		   "discount":{"amount":"0"}}},
		 "status":"APPROVED","archived":false,
		 "activities":[{"id":"6c1f0a9e-3b7d-4e52-8a14-d9e2c7b0f365","createdDate":"2026-10-08T09:14:22.530Z","activityType":"ORDER_PLACED"},
		  {"id":"a2e8d4b6-5c31-4f97-b0e3-7d1a9c6f2e48","createdDate":"2026-10-08T09:15:02.871Z","activityType":"ORDER_PAID"}],
		 "taxInfo":{"totalTax":{"amount":"8.09"},"taxBreakdown":[{"taxName":"MwSt.","taxType":"VAT","jurisdiction":"Germany","jurisdictionType":"COUNTRY","rate":"0.19","aggregatedTaxAmount":{"amount":"8.09"}}]},
		 "channelInfo":{"type":"WEB"},
		 "balanceSummary":{"balance":{"amount":"0.00"},"paid":{"amount":"50.65"},"refunded":{"amount":"0"},"authorized":{"amount":"0"},"pendingRefund":{"amount":"0"},"pending":{"amount":"0"},"chargeback":{"amount":"0"},"chargebackReversal":{"amount":"0"}}}`,
		// Guest checkout with an offline payment method: approved, not yet paid.
		`{"id":"c41d8e27-6b3a-4f19-9e52-1a7f3b8d6c05","number":"10002","createdDate":"2026-10-09T15:42:10.077Z","updatedDate":"2026-10-09T15:42:11.240Z",
		 "lineItems":[
		  {"id":"00000000-0000-0000-0000-000000000001","productName":{"original":"Kraftpapier-Umschläge DIN lang (100 Stück)"},"catalogReference":{"catalogItemId":"3c9e1b7d-5a24-4e8f-b061-d2f7a9c4e815","appId":"215238eb-22a5-4c36-9e7b-e7c08025e04e"},
		   "quantity":3,"physicalProperties":{"sku":"UM-DL-KRAFT-100","shippable":true},"itemType":{"preset":"PHYSICAL"},
		   "price":{"amount":"12.95"},"totalPriceBeforeTax":{"amount":"32.65"},"totalPriceAfterTax":{"amount":"38.85"},"paymentOption":"FULL_PAYMENT_ONLINE",
		   "taxInfo":{"taxAmount":{"amount":"6.20"},"taxableAmount":{"amount":"32.65"},"taxRate":"0.19","taxIncludedInPrice":true,
		    "taxBreakdown":[{"jurisdiction":"Germany","rate":"0.19","taxAmount":{"amount":"6.20"},"taxableAmount":{"amount":"32.65"},"taxType":"VAT","taxName":"MwSt.","jurisdictionType":"COUNTRY"}]},
		   "priceUndetermined":false,"fixedQuantity":false}],
		 "buyerInfo":{"contactId":"e83f2a6c-1d95-4b07-8c4e-6a2b9f0d3c71","email":"m.schulz@example.com","visitorId":"5b0c8e3a-7f29-4d16-9a4b-c1e6d8f2a937"},
		 "paymentStatus":"NOT_PAID","fulfillmentStatus":"NOT_FULFILLED","buyerLanguage":"de","weightUnit":"KG","currency":"EUR","taxIncludedInPrices":true,"siteLanguage":"de",
		 "priceSummary":{"subtotal":{"amount":"38.85"},"shipping":{"amount":"4.95"},"tax":{"amount":"6.99"},"discount":{"amount":"0"},"total":{"amount":"43.80"},"totalAdditionalFees":{"amount":"0"}},
		 "billingInfo":{"address":{"country":"DE","subdivision":"DE-HH","city":"Hamburg","postalCode":"22767","streetAddress":{"name":"Holstenstraße","number":"88"},"addressLine2":"Haus B","countryFullname":"Germany","subdivisionFullname":"Hamburg"},
		  "contactDetails":{"firstName":"Mareike","lastName":"Schulz","company":"Hafenkontor Schulz GmbH"}},
		 "shippingInfo":{"code":"ed5bbce2-9533-dff4-7db0-13702fd139c5","title":"Standardversand (DHL)","logistics":{"deliveryTime":"2-3 Werktage"},
		  "cost":{"price":{"amount":"4.95"},"totalPriceBeforeTax":{"amount":"4.16"},"totalPriceAfterTax":{"amount":"4.95"},
		   "taxInfo":{"taxAmount":{"amount":"0.79"},"taxableAmount":{"amount":"4.16"},"taxRate":"0.19","taxIncludedInPrice":true,
		    "taxBreakdown":[{"jurisdiction":"Germany","rate":"0.19","taxAmount":{"amount":"0.79"},"taxableAmount":{"amount":"4.16"},"taxType":"VAT","taxName":"MwSt.","jurisdictionType":"COUNTRY"}]},
		   "discount":{"amount":"0"}}},
		 "status":"APPROVED","archived":false,
		 "activities":[{"id":"f3b9c2d7-8e14-4a65-9c07-2b5e1d8a4f93","createdDate":"2026-10-09T15:42:10.095Z","activityType":"ORDER_PLACED"}],
		 "taxInfo":{"totalTax":{"amount":"6.99"},"taxBreakdown":[{"taxName":"MwSt.","taxType":"VAT","jurisdiction":"Germany","jurisdictionType":"COUNTRY","rate":"0.19","aggregatedTaxAmount":{"amount":"6.99"}}]},
		 "channelInfo":{"type":"WEB"},
		 "balanceSummary":{"balance":{"amount":"43.80"},"paid":{"amount":"0.00"},"refunded":{"amount":"0.00"},"authorized":{"amount":"0.00"}}}`,
	}
	for i, o := range orders {
		if !json.Valid([]byte(o)) {
			t.Fatalf("invalid order JSON %d", i)
		}
	}
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Header.Get("Authorization") != apiKey:
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"message":"UNAUTHENTICATED","details":{}}`))
			return
		case r.Header.Get("wix-site-id") != siteID:
			// A valid key without access to the requested site.
			w.WriteHeader(http.StatusForbidden)
			_, _ = w.Write([]byte(`{"message":"PERMISSION_DENIED","details":{}}`))
			return
		case r.Method != http.MethodPost || r.URL.Path != "/ecom/v1/orders/search":
			w.WriteHeader(http.StatusNotFound)
			return
		}
		var request struct {
			Search struct {
				Paging struct {
					Limit  *int   `json:"limit"`
					Cursor string `json:"cursor"`
				} `json:"cursorPaging"`
			} `json:"search"`
		}
		limit, start := 100, 0
		err := json.NewDecoder(r.Body).Decode(&request)
		if request.Search.Paging.Limit != nil {
			limit = *request.Search.Paging.Limit
		}
		if cursor := request.Search.Paging.Cursor; err == nil && cursor != "" {
			start, err = strconv.Atoi(strings.TrimPrefix(cursor, "simulation-cursor-"))
		}
		if err != nil || limit < 1 || limit > 100 || start < 0 || start > len(orders) {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"message":"INVALID_ARGUMENT","details":{}}`))
			return
		}
		end := min(start+limit, len(orders))
		page, cursors := []json.RawMessage{}, map[string]string{}
		for _, o := range orders[start:end] {
			page = append(page, json.RawMessage(o))
		}
		if end < len(orders) {
			cursors["next"] = "simulation-cursor-" + strconv.Itoa(end)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"orders": page, "metadata": map[string]any{"count": len(page), "cursors": cursors, "hasNext": end < len(orders)}})
	})
	return shopSimulation{
		connection:  shops.Connection{Provider: "wix", Label: "Wix-Simulation", URL: "https://simulation.wixsite.com/papeterie", SiteID: siteID},
		credentials: shops.Credentials{APIKey: apiKey},
		handler:     handler,
		orders: []simulatedOrder{
			// The client does not read the ORDER_PAID activity, so no paidOn.
			{number: "10001", gross: "50.65", paid: true},
			{number: "10002", gross: "43.80"},
		},
	}
}
