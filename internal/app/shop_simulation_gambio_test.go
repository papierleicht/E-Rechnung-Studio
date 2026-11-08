package app

import (
	"encoding/json"
	"fmt"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"papierleicht.de/e-rechnung-studio/internal/shops"
)

func init() { shopSimulations["gambio"] = simulateGambio }

// gambioSimPage serves one page of a v2 list: page and per_page select the
// rows, sort=-id reverses them, and the Link header names the neighbours.
func gambioSimPage(w http.ResponseWriter, r *http.Request, rows []json.RawMessage) {
	query := r.URL.Query()
	page, perPage := 1, 50
	if value := query.Get("page"); value != "" {
		page, _ = strconv.Atoi(value)
	}
	if value := query.Get("per_page"); value != "" {
		perPage, _ = strconv.Atoi(value)
	}
	if page < 1 || perPage < 1 {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	if query.Get("sort") == "-id" {
		rows = slices.Clone(rows)
		slices.Reverse(rows)
	}
	last := max(1, (len(rows)+perPage-1)/perPage)
	links := []string{}
	link := func(target int, rel string) {
		query.Set("page", strconv.Itoa(target))
		links = append(links, fmt.Sprintf(`<https://%s%s?%s>; rel="%s"`, r.Host, r.URL.Path, query.Encode(), rel))
	}
	if page > 1 {
		link(1, "first")
		link(page-1, "previous")
	}
	if page < last {
		link(page+1, "next")
		link(last, "last")
	}
	if len(links) > 0 {
		w.Header().Set("Link", strings.Join(links, ", "))
	}
	_ = json.NewEncoder(w).Encode(rows[min((page-1)*perPage, len(rows)):min(page*perPage, len(rows))])
}

// simulateGambio answers the REST API v2 like a shop in a subdirectory: Basic
// authentication, an order list paged by Link headers, order details and the
// billing country by ID. 120 summer orders before the import date push the two
// open October orders onto the second page. Gambio reports no payments, so
// both orders stay open.
func simulateGambio(t *testing.T) shopSimulation {
	const user, password = "api@gambio-simulation.example", "Gambio-Simulation-2026!"
	row := func(id, status int, statusName, sum, purchased string, customer int, name, email string, payment map[string]string) json.RawMessage {
		raw, _ := json.Marshal(map[string]any{"id": id, "statusId": status, "statusName": statusName, "totalSum": sum, "purchaseDate": purchased, "comment": "",
			"withdrawalIds": []int{}, "mailStatus": true, "customerId": customer, "customerName": name, "customerEmail": email, "customerStatusId": 2,
			"customerStatusName": "Neuer Kunde", "customerMemos": []any{}, "paymentType": payment,
			"shippingType": map[string]string{"title": "DHL Paket", "module": "flat_flat"}, "trackingLinks": []string{}})
		return raw
	}
	invoice, prepayment := map[string]string{"title": "Rechnung", "module": "invoice"}, map[string]string{"title": "Vorkasse", "module": "moneyorder"}
	payments := []map[string]string{prepayment, invoice, {"title": "PayPal", "module": "gambio_hub"}}
	names := []string{"Anna Keller", "Ben Krüger", "Clara Vogel", "David Hahn", "Eva Lorenz", "Felix Roth"}
	rows := []json.RawMessage{}
	for i := range 120 {
		status, statusName := 3, "Versendet"
		if i%25 == 7 {
			status, statusName = 99, "Storniert"
		}
		customer := 3001 + i%len(names)
		purchased := time.Date(2026, 6, 1, 10, 15, 0, 0, time.UTC).AddDate(0, 0, i).Format(time.DateTime)
		rows = append(rows, row(1001+i, status, statusName, fmt.Sprintf("%d,90 EUR", 19+i%30), purchased, customer, names[i%len(names)], fmt.Sprintf("kunde%d@example.org", customer), payments[i%len(payments)]))
	}
	rows = append(rows,
		row(1121, 1, "Offen", "66,64 EUR", "2026-10-14 09:11:47", 4711, "Paul Schneider", "paul.schneider@example.net", invoice),
		row(1122, 1, "Offen", "56,71 EUR", "2026-10-21 16:38:05", 5288, "Marie Braun", "marie.braun@example.com", prepayment))
	// Prices include tax for these customer groups; finalPrice is the line total.
	paul := `{"gender":"m","firstname":"Paul","lastname":"Schneider","company":"","street":"Bahnhofstraße","houseNumber":"7","additionalAddressInfo":"","suburb":"","postcode":"80335","city":"München","countryId":81,"zoneId":0,"b2bStatus":false}`
	cafe := `{"gender":"f","firstname":"Marie","lastname":"Braun","company":"Café Sonnenschein GmbH","street":"Marktplatz","houseNumber":"3","additionalAddressInfo":"","suburb":"","postcode":"69117","city":"Heidelberg","countryId":81,"zoneId":0,"b2bStatus":true}`
	details := map[string]string{
		"orders/1121": `{"id":1121,"statusId":1,"purchaseDate":"2026-10-14 09:11:47","currencyCode":"EUR","languageCode":"DE","comment":"","totalWeight":0.48,
		 "paymentType":{"title":"Rechnung","module":"invoice"},"shippingType":{"title":"DHL Paket","module":"flat_flat"},
		 "customer":{"id":4711,"number":"","email":"paul.schneider@example.net","phone":"","vatId":"","status":{"id":2,"name":"Neuer Kunde","image":"","discount":0,"isGuest":false}},
		 "addresses":{"customer":` + paul + `,"billing":` + paul + `,"delivery":` + paul + `},
		 "items":[
		  {"id":2301,"model":"NB-A5-L","name":"Notizbuch A5 liniert","quantity":3,"price":8.33,"finalPrice":24.99,"tax":19,"isTaxAllowed":true,"discount":0,"shippingTimeInformation":"ca. 2-3 Tage","checkoutInformation":"","attributes":[],"downloadInformation":[],"addonValues":{"productId":"41","quantityUnitId":"0"},"quantityUnitName":""},
		  {"id":2302,"model":"FH-CL-01","name":"Füllfederhalter Classic","quantity":1,"price":35.7,"finalPrice":35.7,"tax":19,"isTaxAllowed":true,"discount":0,"shippingTimeInformation":"ca. 2-3 Tage","checkoutInformation":"","attributes":[],"downloadInformation":[],"addonValues":{"productId":"57","quantityUnitId":"0"},"quantityUnitName":""}],
		 "totals":[
		  {"id":5501,"title":"Zwischensumme:","value":60.69,"valueText":"60,69 EUR","class":"ot_subtotal","sortOrder":10},
		  {"id":5502,"title":"DHL Paket:","value":5.95,"valueText":"5,95 EUR","class":"ot_shipping","sortOrder":30},
		  {"id":5503,"title":"inkl. 19% MwSt.:","value":10.64,"valueText":"10,64 EUR","class":"ot_tax","sortOrder":98},
		  {"id":5504,"title":"<b>Summe</b>:","value":66.64,"valueText":"<b>66,64 EUR</b>","class":"ot_total","sortOrder":99}],
		 "statusHistory":[{"id":3301,"statusId":1,"dateAdded":"2026-10-14 09:11:47","comment":"","customerNotified":true}],
		 "addonValues":{"customerIp":"","downloadAbandonmentStatus":"0","serviceAbandonmentStatus":"0"}}`,
		"orders/1122": `{"id":1122,"statusId":1,"purchaseDate":"2026-10-21 16:38:05","currencyCode":"EUR","languageCode":"DE","comment":"Bitte an der Theke abgeben.","totalWeight":2.6,
		 "paymentType":{"title":"Vorkasse","module":"moneyorder"},"shippingType":{"title":"DHL Paket","module":"flat_flat"},
		 "customer":{"id":5288,"number":"","email":"marie.braun@example.com","phone":"","vatId":"","status":{"id":2,"name":"Neuer Kunde","image":"","discount":0,"isGuest":false}},
		 "addresses":{"customer":` + cafe + `,"billing":` + cafe + `,"delivery":` + cafe + `},
		 "items":[
		  {"id":2311,"model":"KB-ESP-1000","name":"Espresso Bohnen 1 kg","quantity":2,"price":21.4,"finalPrice":42.8,"tax":7,"isTaxAllowed":true,"discount":0,"shippingTimeInformation":"ca. 2-3 Tage","checkoutInformation":"","attributes":[],"downloadInformation":[],"addonValues":{"productId":"12","quantityUnitId":"0"},"quantityUnitName":""},
		  {"id":2312,"model":"KF-FIL-500","name":"Filterkaffee gemahlen 500 g","quantity":1,"price":8.56,"finalPrice":8.56,"tax":7,"isTaxAllowed":true,"discount":0,"shippingTimeInformation":"ca. 2-3 Tage","checkoutInformation":"","attributes":[],"downloadInformation":[],"addonValues":{"productId":"15","quantityUnitId":"0"},"quantityUnitName":""}],
		 "totals":[
		  {"id":5521,"title":"Zwischensumme:","value":51.36,"valueText":"51,36 EUR","class":"ot_subtotal","sortOrder":10},
		  {"id":5522,"title":"DHL Paket:","value":5.35,"valueText":"5,35 EUR","class":"ot_shipping","sortOrder":30},
		  {"id":5523,"title":"inkl. 7% MwSt.:","value":3.71,"valueText":"3,71 EUR","class":"ot_tax","sortOrder":98},
		  {"id":5524,"title":"<b>Summe</b>:","value":56.71,"valueText":"<b>56,71 EUR</b>","class":"ot_total","sortOrder":99}],
		 "statusHistory":[{"id":3342,"statusId":1,"dateAdded":"2026-10-21 16:38:05","comment":"","customerNotified":true}],
		 "addonValues":{"customerIp":"","downloadAbandonmentStatus":"0","serviceAbandonmentStatus":"0"}}`,
	}
	for path, detail := range details {
		if !json.Valid([]byte(detail)) {
			t.Fatalf("invalid Gambio %s", path)
		}
	}
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if u, p, ok := r.BasicAuth(); !ok || u != user || p != password {
			w.Header().Set("WWW-Authenticate", `Basic realm="Gambio API"`)
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		// Other methods write to the shop; the client only reads.
		if r.Method != http.MethodGet {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		resource, _ := strings.CutPrefix(r.URL.Path, "/shop/api.php/v2/")
		switch {
		case resource == "orders":
			gambioSimPage(w, r, rows)
		case resource == "countries/81":
			_, _ = w.Write([]byte(`{"id":81,"name":"Deutschland","iso2":"DE","iso3":"DEU","addressFormatId":5,"status":true}`))
		case details[resource] != "":
			_, _ = w.Write([]byte(details[resource]))
		default:
			// Orders before the import date must not be fetched in detail.
			t.Errorf("unexpected Gambio request %s", r.URL)
			w.WriteHeader(http.StatusNotFound)
		}
	})
	return shopSimulation{
		connection:  shops.Connection{Provider: "gambio", Label: "Gambio-Simulation", URL: "https://gambio.shop-simulation.example/shop"},
		credentials: shops.Credentials{Username: user, Password: password},
		handler:     handler,
		orders: []simulatedOrder{
			{number: "1121", gross: "66.64"},
			{number: "1122", gross: "56.71"},
		},
	}
}
