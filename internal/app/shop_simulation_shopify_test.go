package app

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"testing"

	"papierleicht.de/e-rechnung-studio/internal/shops"
)

func init() { shopSimulations["shopify"] = simulateShopify }

// simulateShopify answers the Admin GraphQL API 2026-10 like a German store
// with tax-inclusive prices: a token from the client credentials grant, then
// the orders query paged by its "first" argument and opaque cursors.
func simulateShopify(t *testing.T) shopSimulation {
	const clientID, clientSecret = "simulation-client-id", "simulation-client-secret"
	// Sorted by updatedAt (sortKey UPDATED_AT). Shopify writes decimals without
	// trailing zeros and null for empty address fields.
	orders := []struct {
		id      int64
		updated string
		node    string
	}{
		{6904817350721, "2026-10-05 07:31:02", `{"id":"gid://shopify/Order/6904817350721","name":"#1001","email":"katharina.weber@example.com",
		 "createdAt":"2026-10-05T07:28:15Z","updatedAt":"2026-10-05T07:31:02Z","displayFinancialStatus":"PAID","cancelledAt":null,"taxesIncluded":true,
		 "customer":{"id":"gid://shopify/Customer/8461029384756"},
		 "billingAddress":{"firstName":"Katharina","lastName":"Weber","company":null,"address1":"Kastanienallee 12","address2":null,"city":"Berlin","province":null,"zip":"10435","countryCodeV2":"DE"},
		 "currentTotalPriceSet":{"presentmentMoney":{"amount":"73.7","currencyCode":"EUR"}},
		 "currentTotalTaxSet":{"presentmentMoney":{"amount":"11.77","currencyCode":"EUR"}},
		 "totalRefundedSet":{"presentmentMoney":{"amount":"0.0","currencyCode":"EUR"}},
		 "totalTipReceivedSet":{"presentmentMoney":{"amount":"0.0","currencyCode":"EUR"}},
		 "lineItems":{"pageInfo":{"hasNextPage":false,"endCursor":"eyJsYXN0X2lkIjoxNjIxMTgzOTc4Mjk3NywibGFzdF92YWx1ZSI6MTYyMTE4Mzk3ODI5Nzd9"},"nodes":[
		  {"id":"gid://shopify/LineItem/16211839750209","name":"Notizbuch A5 - Punktraster","sku":"NB-A5-DOT","quantity":2,"currentQuantity":2,"isGiftCard":false,
		   "priceAfterAllDiscountsBeforeTaxesSet":{"presentmentMoney":{"amount":"25.04","currencyCode":"EUR"}},
		   "taxLines":[{"rate":0.19,"priceSet":{"presentmentMoney":{"amount":"4.76","currencyCode":"EUR"}}}]},
		  {"id":"gid://shopify/LineItem/16211839782977","name":"Füllfederhalter Classic - Blau","sku":"FH-CL-BLAU","quantity":1,"currentQuantity":1,"isGiftCard":false,
		   "priceAfterAllDiscountsBeforeTaxesSet":{"presentmentMoney":{"amount":"32.77","currencyCode":"EUR"}},
		   "taxLines":[{"rate":0.19,"priceSet":{"presentmentMoney":{"amount":"6.23","currencyCode":"EUR"}}}]}]},
		 "shippingLines":{"pageInfo":{"hasNextPage":false,"endCursor":"eyJsYXN0X2lkIjo1NDk4MjE3NzU4Nzg1LCJsYXN0X3ZhbHVlIjo1NDk4MjE3NzU4Nzg1fQ=="},"nodes":[
		  {"id":"gid://shopify/ShippingLine/5498217758785","title":"DHL Paket","isRemoved":false,
		   "currentDiscountedPriceSet":{"presentmentMoney":{"amount":"4.9","currencyCode":"EUR"}},
		   "taxLines":[{"rate":0.19,"priceSet":{"presentmentMoney":{"amount":"0.78","currencyCode":"EUR"}}}]}]}}`},
		// Books at 7 %, shipping follows the rate of the goods; paid later by bank transfer.
		{6905022578753, "2026-10-06 12:00:41", `{"id":"gid://shopify/Order/6905022578753","name":"#1002","email":"jonas.becker@example.com",
		 "createdAt":"2026-10-06T12:00:39Z","updatedAt":"2026-10-06T12:00:41Z","displayFinancialStatus":"PENDING","cancelledAt":null,"taxesIncluded":true,
		 "customer":{"id":"gid://shopify/Customer/8462210978884"},
		 "billingAddress":{"firstName":"Jonas","lastName":"Becker","company":"Atelier Becker GmbH","address1":"Leopoldstraße 44","address2":"Rückgebäude","city":"München","province":null,"zip":"80802","countryCodeV2":"DE"},
		 "currentTotalPriceSet":{"presentmentMoney":{"amount":"41.9","currencyCode":"EUR"}},
		 "currentTotalTaxSet":{"presentmentMoney":{"amount":"2.74","currencyCode":"EUR"}},
		 "totalRefundedSet":{"presentmentMoney":{"amount":"0.0","currencyCode":"EUR"}},
		 "totalTipReceivedSet":{"presentmentMoney":{"amount":"0.0","currencyCode":"EUR"}},
		 "lineItems":{"pageInfo":{"hasNextPage":false,"endCursor":"eyJsYXN0X2lkIjoxNjIxMjMwNDAxMTMyOSwibGFzdF92YWx1ZSI6MTYyMTIzMDQwMTEzMjl9"},"nodes":[
		  {"id":"gid://shopify/LineItem/16212304011329","name":"Kalligrafie-Handbuch - Taschenbuch","sku":"BU-KALLI-TB","quantity":3,"currentQuantity":3,"isGiftCard":false,
		   "priceAfterAllDiscountsBeforeTaxesSet":{"presentmentMoney":{"amount":"36.17","currencyCode":"EUR"}},
		   "taxLines":[{"rate":0.07,"priceSet":{"presentmentMoney":{"amount":"2.53","currencyCode":"EUR"}}}]}]},
		 "shippingLines":{"pageInfo":{"hasNextPage":false,"endCursor":"eyJsYXN0X2lkIjo1NDk4NDE5ODcyODMzLCJsYXN0X3ZhbHVlIjo1NDk4NDE5ODcyODMzfQ=="},"nodes":[
		  {"id":"gid://shopify/ShippingLine/5498419872833","title":"Büchersendung","isRemoved":false,
		   "currentDiscountedPriceSet":{"presentmentMoney":{"amount":"3.2","currencyCode":"EUR"}},
		   "taxLines":[{"rate":0.07,"priceSet":{"presentmentMoney":{"amount":"0.21","currencyCode":"EUR"}}}]}]}}`},
	}
	for _, o := range orders {
		if !json.Valid([]byte(o.node)) {
			t.Fatalf("invalid order JSON %d", o.id)
		}
	}
	cursor := func(i int) string {
		return base64.StdEncoding.EncodeToString(fmt.Appendf(nil, `{"last_id":%d,"last_value":"%s.000000"}`, orders[i].id, orders[i].updated))
	}
	pageSize := regexp.MustCompile(`\borders\(first: (\d+),`)
	var mu sync.Mutex
	tokens := map[string]bool{}
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		switch r.URL.Path {
		case "/admin/oauth/access_token":
			if r.Method != http.MethodPost {
				w.WriteHeader(http.StatusMethodNotAllowed)
				return
			}
			if r.ParseForm() != nil || r.PostForm.Get("grant_type") != "client_credentials" || r.PostForm.Get("client_id") != clientID || r.PostForm.Get("client_secret") != clientSecret {
				w.WriteHeader(http.StatusUnauthorized)
				_, _ = w.Write([]byte(`{"error":"invalid_client","error_description":"Client authentication failed."}`))
				return
			}
			// Tokens live 24 hours; every sync requests a new one.
			mu.Lock()
			token := fmt.Sprintf("simulation-access-token-%d", len(tokens)+1)
			tokens[token] = true
			mu.Unlock()
			_, _ = fmt.Fprintf(w, `{"access_token":%q,"scope":"read_orders,read_all_orders","expires_in":86399}`, token)
		case "/admin/api/2026-10/graphql.json":
			mu.Lock()
			known := tokens[r.Header.Get("X-Shopify-Access-Token")]
			mu.Unlock()
			if !known {
				w.WriteHeader(http.StatusUnauthorized)
				_, _ = w.Write([]byte(`{"errors":"[API] Invalid API key or access token (unrecognized login or wrong password)"}`))
				return
			}
			if r.Method != http.MethodPost {
				w.WriteHeader(http.StatusMethodNotAllowed)
				return
			}
			var request struct {
				Query     string `json:"query"`
				Variables struct {
					After *string `json:"after"`
				} `json:"variables"`
			}
			if json.NewDecoder(r.Body).Decode(&request) != nil {
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			w.Header().Set("X-Shopify-API-Version", "2026-10")
			first := pageSize.FindStringSubmatch(request.Query)
			start := 0
			if request.Variables.After != nil {
				start = -1
				for i := range orders {
					if cursor(i) == *request.Variables.After {
						start = i + 1
					}
				}
			}
			// GraphQL errors arrive with status 200.
			if !strings.HasPrefix(strings.TrimSpace(request.Query), "query") || first == nil || start < 0 {
				_, _ = w.Write([]byte(`{"errors":[{"message":"Invalid orders query or cursor."}]}`))
				return
			}
			size, _ := strconv.Atoi(first[1])
			end := min(start+size, len(orders))
			nodes := []json.RawMessage{}
			var endCursor *string
			for i := start; i < end; i++ {
				nodes = append(nodes, json.RawMessage(orders[i].node))
				c := cursor(i)
				endCursor = &c
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"data": map[string]any{
					"currentAppInstallation": map[string]any{"accessScopes": []map[string]string{{"handle": "read_orders"}, {"handle": "read_all_orders"}}},
					"orders":                 map[string]any{"pageInfo": map[string]any{"hasNextPage": end < len(orders), "endCursor": endCursor}, "nodes": nodes},
				},
				"extensions": json.RawMessage(`{"cost":{"requestedQueryCost":113,"actualQueryCost":8,"throttleStatus":{"maximumAvailable":2000.0,"currentlyAvailable":1992,"restoreRate":100.0}}}`),
			})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	})
	return shopSimulation{
		connection:  shops.Connection{Provider: "shopify", Label: "Shopify-Simulation", URL: "https://papeterie-simulation.myshopify.com"},
		credentials: shops.Credentials{ClientID: clientID, ClientSecret: clientSecret},
		handler:     handler,
		orders: []simulatedOrder{
			// The orders query reads no payment time, so no paidOn.
			{number: "#1001", gross: "73.70", paid: true},
			{number: "#1002", gross: "41.90"},
		},
	}
}
