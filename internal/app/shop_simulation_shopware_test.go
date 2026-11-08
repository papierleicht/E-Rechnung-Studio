package app

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"papierleicht.de/e-rechnung-studio/internal/shops"
)

func init() { shopSimulations["shopware"] = simulateShopware }

// shopwareSimCriteria is the part of a search criteria the emulator applies:
// paging, the total count and the associations to load.
type shopwareSimCriteria struct {
	Page         *int                           `json:"page"`
	Limit        *int                           `json:"limit"`
	CountMode    any                            `json:"total-count-mode"`
	Associations map[string]shopwareSimCriteria `json:"associations"`
}

// shopwareSimLazy lists the associations Shopware loads only on request;
// states, the order customer and the address country are autoloaded.
var shopwareSimLazy = map[string][]string{
	"order":                     {"currency", "billingAddress", "lineItems", "deliveries", "transactions"},
	"order_transaction":         {"captures"},
	"order_transaction_capture": {"refunds"},
}

// shopwareSimLoad empties the lazy associations a search did not request and
// applies the requested association limits.
func shopwareSimLoad(entity map[string]any, requested map[string]shopwareSimCriteria) {
	alias, _ := entity["apiAlias"].(string)
	for _, name := range shopwareSimLazy[alias] {
		nested, ok := requested[name]
		if !ok {
			entity[name] = nil
			continue
		}
		children, _ := entity[name].([]any)
		if nested.Limit != nil && *nested.Limit > 0 && *nested.Limit < len(children) {
			children = children[:*nested.Limit]
			entity[name] = children
		}
		for _, child := range children {
			if c, ok := child.(map[string]any); ok {
				shopwareSimLoad(c, nested.Associations)
			}
		}
	}
}

// simulateShopware answers the Admin API like Shopware 6.7: the integration
// gets a bearer token by client credentials, then searches orders in the plain
// JSON format with the associations it requests. The date filter is not
// evaluated, so every poll returns the same orders.
func simulateShopware(t *testing.T) shopSimulation {
	const accessKey, secretKey = "SWIAQUDKRVNRMWPOEFXUXZHMDG", "ZWJmZDNhMmQtc2ltdWxhdGlvbi1zZWNyZXQtYWNjZXNzLWtleQ"
	const token = "eyJ0eXAiOiJKV1QiLCJhbGciOiJSUzI1NiJ9.eyJhdWQiOiJTV0lBUVVES1JWTlJNV1BPRUZYVVhaSE1ERyJ9.c2ltdWxhdGVkLXNpZ25hdHVyZQ"
	// 10421 is paid by PayPal and shipped, 10422 is an open invoice purchase
	// at 7 %; Shopware taxes the shipping at the rate of the cart.
	orders := `[
	 {"id":"3a1a989d6cc64c938fdb18654b86e3a4","versionId":"0fa91ce3e96a4bc2be4bd9ce752c3425","orderNumber":"10421","autoIncrement":421,
	  "orderDateTime":"2026-10-07T08:41:12.000+00:00","orderDate":"2026-10-07","createdAt":"2026-10-07T08:41:12.512+00:00","updatedAt":"2026-10-08T14:03:27.104+00:00",
	  "salesChannelId":"3fcaccaf30484ff591c9957c0c069f2e","languageId":"2fbb5fe2e29a4d70aa5854ce7ce3e20b","currencyId":"b7d2554b0ce847cd82f3ac9bd1c0dfca","currencyFactor":1,
	  "amountTotal":50.17,"amountNet":42.16,"positionPrice":45.22,"shippingTotal":4.95,"taxStatus":"gross",
	  "price":{"netPrice":42.16,"totalPrice":50.17,"positionPrice":45.22,"rawTotal":50.17,"taxStatus":"gross","calculatedTaxes":[{"tax":8.01,"taxRate":19,"price":50.17,"apiAlias":"cart_tax_calculated"}],"taxRules":[{"taxRate":19,"percentage":100,"apiAlias":"cart_tax_rule"}],"apiAlias":"cart_price"},
	  "shippingCosts":{"unitPrice":4.95,"quantity":1,"totalPrice":4.95,"calculatedTaxes":[{"tax":0.79,"taxRate":19,"price":4.95,"apiAlias":"cart_tax_calculated"}],"taxRules":[{"taxRate":19,"percentage":100,"apiAlias":"cart_tax_rule"}],"referencePrice":null,"listPrice":null,"regulationPrice":null,"apiAlias":"calculated_price"},
	  "stateId":"92f3ac0779ae436bb2e8975699450fdf","stateMachineState":{"technicalName":"in_progress","name":"In Bearbeitung","id":"92f3ac0779ae436bb2e8975699450fdf","apiAlias":"state_machine_state"},
	  "currency":{"isoCode":"EUR","symbol":"€","shortName":"EUR","name":"Euro","factor":1,"isSystemDefault":true,"id":"b7d2554b0ce847cd82f3ac9bd1c0dfca","apiAlias":"currency"},
	  "orderCustomer":{"email":"lena.wagner@example.org","firstName":"Lena","lastName":"Wagner","company":null,"customerNumber":"10037","customerId":"d9914fff0a04494d96ce48be8cb2f993","vatIds":null,"remoteAddress":"203.0.113.0","id":"b4af04023edc4b92a82c6c792eff79af","apiAlias":"order_customer"},
	  "billingAddressId":"6e5a138f89924033af9654e7e3478991",
	  "billingAddress":{"firstName":"Lena","lastName":"Wagner","company":null,"department":null,"street":"Lindenstraße 12","additionalAddressLine1":null,"additionalAddressLine2":null,"zipcode":"50674","city":"Köln","phoneNumber":null,"vatId":null,
	   "countryId":"83eef7e392264b1c81a83386a82b943c","countryStateId":null,"country":{"iso":"DE","iso3":"DEU","name":"Deutschland","id":"83eef7e392264b1c81a83386a82b943c","apiAlias":"country"},"countryState":null,"id":"6e5a138f89924033af9654e7e3478991","apiAlias":"order_address"},
	  "lineItems":[
	   {"id":"295022d8b2c5429a9b9fe44a2708b93f","type":"product","label":"Wandkalender 2027","description":null,"quantity":1,"unitPrice":23.8,"totalPrice":23.8,"position":1,"parentId":null,
	    "productId":"b1aea5fc0122404a86a44dae50482552","referencedId":"b1aea5fc0122404a86a44dae50482552","good":true,"states":["is-physical"],"payload":{"productNumber":"SW10012"},
	    "price":{"unitPrice":23.8,"quantity":1,"totalPrice":23.8,"calculatedTaxes":[{"tax":3.8,"taxRate":19,"price":23.8,"apiAlias":"cart_tax_calculated"}],"taxRules":[{"taxRate":19,"percentage":100,"apiAlias":"cart_tax_rule"}],"referencePrice":null,"listPrice":null,"regulationPrice":null,"apiAlias":"calculated_price"},"apiAlias":"order_line_item"},
	   {"id":"dbcb8ded89f7411abd3eea6eb5387de3","type":"product","label":"Notizbuch A5 liniert","description":null,"quantity":3,"unitPrice":7.14,"totalPrice":21.42,"position":2,"parentId":null,
	    "productId":"de4d2038977941a0b52aa56b29ba9f29","referencedId":"de4d2038977941a0b52aa56b29ba9f29","good":true,"states":["is-physical"],"payload":{"productNumber":"SW10031"},
	    "price":{"unitPrice":7.14,"quantity":3,"totalPrice":21.42,"calculatedTaxes":[{"tax":3.42,"taxRate":19,"price":21.42,"apiAlias":"cart_tax_calculated"}],"taxRules":[{"taxRate":19,"percentage":100,"apiAlias":"cart_tax_rule"}],"referencePrice":null,"listPrice":null,"regulationPrice":null,"apiAlias":"calculated_price"},"apiAlias":"order_line_item"}],
	  "primaryOrderDeliveryId":"1c8de774c4d146c883229c6fea7bbd9a",
	  "deliveries":[{"id":"1c8de774c4d146c883229c6fea7bbd9a","createdAt":"2026-10-07T08:41:12.541+00:00","updatedAt":"2026-10-08T14:03:27.090+00:00","shippingOrderAddressId":"6e5a138f89924033af9654e7e3478991","shippingMethodId":"007f85829f8f448bb5032d430ea83c0f",
	   "trackingCodes":[],"shippingDateEarliest":"2026-10-08T00:00:00.000+00:00","shippingDateLatest":"2026-10-10T00:00:00.000+00:00",
	   "shippingCosts":{"unitPrice":4.95,"quantity":1,"totalPrice":4.95,"calculatedTaxes":[{"tax":0.79,"taxRate":19,"price":4.95,"apiAlias":"cart_tax_calculated"}],"taxRules":[{"taxRate":19,"percentage":100,"apiAlias":"cart_tax_rule"}],"referencePrice":null,"listPrice":null,"regulationPrice":null,"apiAlias":"calculated_price"},
	   "stateId":"8f5a43f96d874f41b237fd3cd31ec1e5","stateMachineState":{"technicalName":"shipped","name":"Versandt","id":"8f5a43f96d874f41b237fd3cd31ec1e5","apiAlias":"state_machine_state"},"apiAlias":"order_delivery"}],
	  "primaryOrderTransactionId":"d3bceeb8a668416fad911cdcca19e624",
	  "transactions":[{"id":"d3bceeb8a668416fad911cdcca19e624","createdAt":"2026-10-07T08:41:12.530+00:00","updatedAt":"2026-10-07T08:41:44.218+00:00","paymentMethodId":"7d3cc14d72794186a9c7190c6a8799cb",
	   "amount":{"unitPrice":50.17,"quantity":1,"totalPrice":50.17,"calculatedTaxes":[{"tax":8.01,"taxRate":19,"price":50.17,"apiAlias":"cart_tax_calculated"}],"taxRules":[{"taxRate":19,"percentage":100,"apiAlias":"cart_tax_rule"}],"referencePrice":null,"listPrice":null,"regulationPrice":null,"apiAlias":"calculated_price"},
	   "stateId":"ad48afd3ea8e47e3a3ee10b06ad136fb","stateMachineState":{"technicalName":"paid","name":"Bezahlt","id":"ad48afd3ea8e47e3a3ee10b06ad136fb","apiAlias":"state_machine_state"},"captures":[],"apiAlias":"order_transaction"}],
	  "customerComment":null,"customFields":null,"apiAlias":"order"},
	 {"id":"e4a2315bbf9e4f2cb090847a7b77f764","versionId":"0fa91ce3e96a4bc2be4bd9ce752c3425","orderNumber":"10422","autoIncrement":422,
	  "orderDateTime":"2026-10-12T15:27:03.000+00:00","orderDate":"2026-10-12","createdAt":"2026-10-12T15:27:03.287+00:00","updatedAt":null,
	  "salesChannelId":"3fcaccaf30484ff591c9957c0c069f2e","languageId":"2fbb5fe2e29a4d70aa5854ce7ce3e20b","currencyId":"b7d2554b0ce847cd82f3ac9bd1c0dfca","currencyFactor":1,
	  "amountTotal":54.17,"amountNet":50.63,"positionPrice":49.22,"shippingTotal":4.95,"taxStatus":"gross",
	  "price":{"netPrice":50.63,"totalPrice":54.17,"positionPrice":49.22,"rawTotal":54.17,"taxStatus":"gross","calculatedTaxes":[{"tax":3.54,"taxRate":7,"price":54.17,"apiAlias":"cart_tax_calculated"}],"taxRules":[{"taxRate":7,"percentage":100,"apiAlias":"cart_tax_rule"}],"apiAlias":"cart_price"},
	  "shippingCosts":{"unitPrice":4.95,"quantity":1,"totalPrice":4.95,"calculatedTaxes":[{"tax":0.32,"taxRate":7,"price":4.95,"apiAlias":"cart_tax_calculated"}],"taxRules":[{"taxRate":7,"percentage":100,"apiAlias":"cart_tax_rule"}],"referencePrice":null,"listPrice":null,"regulationPrice":null,"apiAlias":"calculated_price"},
	  "stateId":"6bec349d0cef43f5b6960ba24b248b14","stateMachineState":{"technicalName":"open","name":"Offen","id":"6bec349d0cef43f5b6960ba24b248b14","apiAlias":"state_machine_state"},
	  "currency":{"isoCode":"EUR","symbol":"€","shortName":"EUR","name":"Euro","factor":1,"isSystemDefault":true,"id":"b7d2554b0ce847cd82f3ac9bd1c0dfca","apiAlias":"currency"},
	  "orderCustomer":{"email":"j.hoffmann@example.com","firstName":"Jonas","lastName":"Hoffmann","company":"Kanzlei Hoffmann & Partner mbB","customerNumber":"10052","customerId":"ea8697c260f54317964443b519575d48","vatIds":null,"remoteAddress":"198.51.100.0","id":"d35e51715af04f799f25634433b4cbcc","apiAlias":"order_customer"},
	  "billingAddressId":"623fae1bb04444ae81a034ff897058ca",
	  "billingAddress":{"firstName":"Jonas","lastName":"Hoffmann","company":"Kanzlei Hoffmann & Partner mbB","department":"Sekretariat","street":"Königsallee 14","additionalAddressLine1":null,"additionalAddressLine2":null,"zipcode":"40212","city":"Düsseldorf","phoneNumber":null,"vatId":null,
	   "countryId":"83eef7e392264b1c81a83386a82b943c","countryStateId":null,"country":{"iso":"DE","iso3":"DEU","name":"Deutschland","id":"83eef7e392264b1c81a83386a82b943c","apiAlias":"country"},"countryState":null,"id":"623fae1bb04444ae81a034ff897058ca","apiAlias":"order_address"},
	  "lineItems":[
	   {"id":"ed648c38c06a4f2fbf12c89247cb3e57","type":"product","label":"E-Rechnung in der Praxis (Buch)","description":null,"quantity":2,"unitPrice":24.61,"totalPrice":49.22,"position":1,"parentId":null,
	    "productId":"7f4dea64464847bfbf823c3778f6bef0","referencedId":"7f4dea64464847bfbf823c3778f6bef0","good":true,"states":["is-physical"],"payload":{"productNumber":"SW10058"},
	    "price":{"unitPrice":24.61,"quantity":2,"totalPrice":49.22,"calculatedTaxes":[{"tax":3.22,"taxRate":7,"price":49.22,"apiAlias":"cart_tax_calculated"}],"taxRules":[{"taxRate":7,"percentage":100,"apiAlias":"cart_tax_rule"}],"referencePrice":null,"listPrice":null,"regulationPrice":null,"apiAlias":"calculated_price"},"apiAlias":"order_line_item"}],
	  "primaryOrderDeliveryId":"23af4eea6f0143a69002762001d0a7eb",
	  "deliveries":[{"id":"23af4eea6f0143a69002762001d0a7eb","createdAt":"2026-10-12T15:27:03.309+00:00","updatedAt":null,"shippingOrderAddressId":"623fae1bb04444ae81a034ff897058ca","shippingMethodId":"007f85829f8f448bb5032d430ea83c0f",
	   "trackingCodes":[],"shippingDateEarliest":"2026-10-13T00:00:00.000+00:00","shippingDateLatest":"2026-10-15T00:00:00.000+00:00",
	   "shippingCosts":{"unitPrice":4.95,"quantity":1,"totalPrice":4.95,"calculatedTaxes":[{"tax":0.32,"taxRate":7,"price":4.95,"apiAlias":"cart_tax_calculated"}],"taxRules":[{"taxRate":7,"percentage":100,"apiAlias":"cart_tax_rule"}],"referencePrice":null,"listPrice":null,"regulationPrice":null,"apiAlias":"calculated_price"},
	   "stateId":"c94a4443507440678d641e2e8ae6c9da","stateMachineState":{"technicalName":"open","name":"Offen","id":"c94a4443507440678d641e2e8ae6c9da","apiAlias":"state_machine_state"},"apiAlias":"order_delivery"}],
	  "primaryOrderTransactionId":"a59131fa975f44a2a06c03a86650faca",
	  "transactions":[{"id":"a59131fa975f44a2a06c03a86650faca","createdAt":"2026-10-12T15:27:03.301+00:00","updatedAt":null,"paymentMethodId":"064804e8f62c47768ea0eccfff871953",
	   "amount":{"unitPrice":54.17,"quantity":1,"totalPrice":54.17,"calculatedTaxes":[{"tax":3.54,"taxRate":7,"price":54.17,"apiAlias":"cart_tax_calculated"}],"taxRules":[{"taxRate":7,"percentage":100,"apiAlias":"cart_tax_rule"}],"referencePrice":null,"listPrice":null,"regulationPrice":null,"apiAlias":"calculated_price"},
	   "stateId":"65de8b1159c34d7c80a46e86b5c50f2f","stateMachineState":{"technicalName":"open","name":"Offen","id":"65de8b1159c34d7c80a46e86b5c50f2f","apiAlias":"state_machine_state"},"captures":[],"apiAlias":"order_transaction"}],
	  "customerComment":"Bitte mit Rechnung an die Kanzlei.","customFields":null,"apiAlias":"order"}
	]`
	if !json.Valid([]byte(orders)) {
		t.Fatal("invalid Shopware orders")
	}
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		isJSON := strings.HasPrefix(r.Header.Get("Content-Type"), "application/json")
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/oauth/token":
			if r.Method != http.MethodPost {
				w.WriteHeader(http.StatusMethodNotAllowed)
				return
			}
			var grant struct {
				Type   string `json:"grant_type"`
				ID     string `json:"client_id"`
				Secret string `json:"client_secret"`
			}
			// The token route takes JSON or form fields.
			if isJSON {
				_ = json.NewDecoder(r.Body).Decode(&grant)
			} else {
				grant.Type, grant.ID, grant.Secret = r.PostFormValue("grant_type"), r.PostFormValue("client_id"), r.PostFormValue("client_secret")
			}
			switch {
			case grant.Type != "client_credentials":
				w.WriteHeader(http.StatusBadRequest)
				_, _ = w.Write([]byte(`{"errors":[{"code":"2","status":"400","title":"Bad Request","detail":"The authorization grant type is not supported by the authorization server."}]}`))
			case grant.ID != accessKey || grant.Secret != secretKey:
				w.WriteHeader(http.StatusUnauthorized)
				_, _ = w.Write([]byte(`{"errors":[{"code":"4","status":"401","title":"Unauthorized","detail":"Client authentication failed"}]}`))
			default:
				_, _ = w.Write([]byte(`{"token_type":"Bearer","expires_in":600,"access_token":"` + token + `"}`))
			}
		case "/api/search/order":
			if r.Method != http.MethodPost {
				w.WriteHeader(http.StatusMethodNotAllowed)
				return
			}
			if r.Header.Get("Authorization") != "Bearer "+token {
				w.WriteHeader(http.StatusUnauthorized)
				_, _ = w.Write([]byte(`{"errors":[{"code":"9","status":"401","title":"Unauthorized","detail":"The resource owner or authorization server denied the request."}]}`))
				return
			}
			// Only the plain JSON format is emulated; Shopware answers other
			// Accept values with JSON:API documents.
			if r.Header.Get("Accept") != "application/json" {
				w.WriteHeader(http.StatusNotAcceptable)
				return
			}
			var criteria shopwareSimCriteria
			if !isJSON || json.NewDecoder(r.Body).Decode(&criteria) != nil || (criteria.Page != nil && *criteria.Page < 1) || (criteria.Limit != nil && *criteria.Limit < 1) {
				w.WriteHeader(http.StatusBadRequest)
				_, _ = w.Write([]byte(`{"errors":[{"code":"FRAMEWORK__INVALID_CRITERIA","status":"400","title":"Bad Request","detail":"Invalid search criteria."}]}`))
				return
			}
			var all []map[string]any
			decoder := json.NewDecoder(strings.NewReader(orders))
			decoder.UseNumber()
			_ = decoder.Decode(&all)
			data := all
			if criteria.Limit != nil {
				offset := 0
				if criteria.Page != nil {
					offset = (*criteria.Page - 1) * *criteria.Limit
				}
				data = all[min(offset, len(all)):min(offset+*criteria.Limit, len(all))]
			}
			for _, order := range data {
				shopwareSimLoad(order, criteria.Associations)
			}
			total := len(data)
			if mode := criteria.CountMode; mode == float64(1) || mode == "exact" {
				total = len(all)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"total": total, "data": data, "aggregations": []any{}})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	})
	return shopSimulation{
		connection:  shops.Connection{Provider: "shopware", Label: "Shopware-Simulation", URL: "https://shopware.shop-simulation.example"},
		credentials: shops.Credentials{ClientID: accessKey, ClientSecret: secretKey},
		handler:     handler,
		orders: []simulatedOrder{
			{number: "10421", gross: "50.17", paid: true},
			{number: "10422", gross: "54.17"},
		},
	}
}
