package app

import (
	"net/http"
	"strings"
	"testing"

	"papierleicht.de/e-rechnung-studio/internal/shops"
)

func init() { shopSimulations["woocommerce"] = simulateWooCommerce }

// simulateWooCommerce answers the REST API v3 like a shop in a subdirectory:
// orders with Basic authentication, one page, and the tax rate looked up by ID.
func simulateWooCommerce(t *testing.T) shopSimulation {
	const key, secret = "ck_simulation_key", "cs_simulation_secret"
	orders := `[
	 {"id":501,"number":"501","customer_id":7,"currency":"EUR","date_created_gmt":"2026-10-05T07:00:00","date_modified_gmt":"2026-10-05T07:31:00","date_paid_gmt":"2026-10-05T07:30:00","status":"processing","total":"65.45","total_tax":"10.45",
	  "billing":{"first_name":"Ada","last_name":"Lovelace","company":"","address_1":"Testweg 1","address_2":"","city":"Berlin","state":"","postcode":"10115","country":"DE","email":"ada@example.com"},
	  "line_items":[{"id":1,"name":"Notizbuch A5","sku":"NB-A5","quantity":2,"total":"50.00","total_tax":"9.50","taxes":[{"id":9,"total":"9.50"}]}],
	  "shipping_lines":[{"id":2,"method_title":"DHL Paket","total":"5.00","total_tax":"0.95","taxes":[{"id":9,"total":"0.95"}]}],
	  "fee_lines":[],"tax_lines":[{"rate_id":9,"rate_percent":"","compound":false}],"refunds":[]},
	 {"id":503,"number":"503","customer_id":9,"currency":"EUR","date_created_gmt":"2026-10-07T08:00:00","date_modified_gmt":"2026-10-07T08:02:00","date_paid_gmt":"2026-10-07T08:01:00","status":"processing","total":"19.98","total_tax":"3.20",
	  "billing":{"first_name":"Katherine","last_name":"Johnson","company":"","address_1":"Sternwarte 2","address_2":"","city":"Potsdam","state":"","postcode":"14473","country":"DE","email":"katherine@example.com"},
	  "line_items":[{"id":5,"name":"Notizbuch A6","sku":"NB-A6","quantity":1,"total":"8.39","total_tax":"1.60","taxes":[{"id":9,"total":"1.60"}]},{"id":6,"name":"Lesezeichen-Set","sku":"LZ-5","quantity":1,"total":"8.39","total_tax":"1.60","taxes":[{"id":9,"total":"1.60"}]}],
	  "shipping_lines":[{"id":7,"method_title":"Kostenloser Versand","total":"0.00","total_tax":"0.00","taxes":[]}],"fee_lines":[],"tax_lines":[{"rate_id":9,"rate_percent":"19","compound":false}],"refunds":[]},
	 {"id":502,"number":"502","customer_id":0,"currency":"EUR","date_created_gmt":"2026-10-06T12:00:00","date_modified_gmt":"2026-10-06T12:00:00","date_paid_gmt":null,"status":"on-hold","total":"35.70","total_tax":"5.70",
	  "billing":{"first_name":"Grace","last_name":"Hopper","company":"Hopper Werkstatt GmbH","address_1":"Hafenstraße 3","address_2":"","city":"Hamburg","state":"","postcode":"20457","country":"DE","email":"grace@example.com"},
	  "line_items":[{"id":3,"name":"Stiftset","sku":"ST-3","quantity":3,"total":"30.00","total_tax":"5.70","taxes":[{"id":9,"total":"5.70"}]}],
	  "shipping_lines":[],"fee_lines":[],"tax_lines":[{"rate_id":9,"rate_percent":"19","compound":false}],"refunds":[]}
	]`
	return shopSimulation{
		connection:  shops.Connection{Provider: "woocommerce", Label: "Woo-Simulation", URL: "https://woo.shop-simulation.example/shop"},
		credentials: shops.Credentials{ConsumerKey: key, ConsumerSecret: secret},
		handler:     wooSimulationHandler(key, secret, orders),
		orders: []simulatedOrder{
			{number: "501", gross: "65.45", paid: true, paidOn: "2026-10-05"},
			{number: "502", gross: "35.70"},
			// Two items at 9.99 € gross, rounded per line: 3.20 VAT, not 3.19.
			{number: "503", gross: "19.98", paid: true, paidOn: "2026-10-07", tax: "3.20"},
		},
	}
}

// simulateSmallBusinessWooCommerce is a shop without VAT, as a business under
// § 19 UStG sets it up: no tax lines at all.
func simulateSmallBusinessWooCommerce(t *testing.T) shopSimulation {
	const key, secret = "ck_small_business", "cs_small_business"
	orders := `[
	 {"id":601,"number":"601","customer_id":11,"currency":"EUR","date_created_gmt":"2026-10-08T09:00:00","date_modified_gmt":"2026-10-08T09:05:00","date_paid_gmt":"2026-10-08T09:04:00","status":"completed","total":"54.90","total_tax":"0.00",
	  "billing":{"first_name":"Hedy","last_name":"Lamarr","company":"","address_1":"Funkweg 5","address_2":"","city":"Leipzig","state":"","postcode":"04109","country":"DE","email":"hedy@example.com"},
	  "line_items":[{"id":1,"name":"Keramikbecher","sku":"KB-1","quantity":2,"total":"49.90","total_tax":"0.00","taxes":[]}],
	  "shipping_lines":[{"id":2,"method_title":"Paket","total":"5.00","total_tax":"0.00","taxes":[]}],"fee_lines":[],"tax_lines":[],"refunds":[]},
	 {"id":602,"number":"602","customer_id":12,"currency":"EUR","date_created_gmt":"2026-10-09T15:00:00","date_modified_gmt":"2026-10-09T15:00:00","date_paid_gmt":null,"status":"on-hold","total":"24.95","total_tax":"0.00",
	  "billing":{"first_name":"Rosalind","last_name":"Franklin","company":"Labor Franklin","address_1":"Helixstraße 1","address_2":"","city":"Halle","state":"","postcode":"06108","country":"DE","email":"labor@example.com"},
	  "line_items":[{"id":3,"name":"Keramikschale","sku":"KS-1","quantity":1,"total":"24.95","total_tax":"0.00","taxes":[]}],
	  "shipping_lines":[],"fee_lines":[],"tax_lines":[],"refunds":[]}
	]`
	company := testCompany()
	company.SmallBusiness = true
	return shopSimulation{
		connection:  shops.Connection{Provider: "woocommerce", Label: "Woo-Simulation § 19", URL: "https://kleinunternehmen.shop-simulation.example"},
		credentials: shops.Credentials{ConsumerKey: key, ConsumerSecret: secret},
		handler:     wooSimulationHandler(key, secret, orders),
		company:     &company,
		orders: []simulatedOrder{
			{number: "601", gross: "54.90", paid: true, paidOn: "2026-10-08", tax: "0.00"},
			{number: "602", gross: "24.95", tax: "0.00"},
		},
	}
}

// wooSimulationHandler answers the REST API v3 with Basic authentication,
// one page of orders and the tax rate looked up by ID, in any subdirectory.
func wooSimulationHandler(key, secret, orders string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user, password, ok := r.BasicAuth()
		if !ok || user != key || password != secret || r.Method != http.MethodGet {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, route, _ := strings.Cut(r.URL.Path, "/wp-json/wc/v3/")
		switch route {
		case "taxes/9":
			_, _ = w.Write([]byte(`{"id":9,"rate":"19.0000","compound":false}`))
		case "orders":
			if r.URL.Query().Get("page") != "1" {
				_, _ = w.Write([]byte(`[]`))
				return
			}
			w.Header().Set("X-WP-TotalPages", "1")
			_, _ = w.Write([]byte(orders))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	})
}
