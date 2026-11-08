package app

import (
	"encoding/json"
	"fmt"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"testing"

	"papierleicht.de/e-rechnung-studio/internal/shops"
)

func init() { shopSimulations["prestashop"] = simulatePrestaShop }

// prestaShopSimulationRows is the shop as its Webservice exposes it: numeric
// IDs, every column as a string, multilingual fields per language (1 = de,
// 2 = en). Order 40 predates the import date; the date filter leaves it out.
const prestaShopSimulationRows = `{
"orders":[
 {"id":40,"id_address_delivery":"18","id_address_invoice":"18","id_cart":"52","id_currency":"1","id_lang":"1","id_customer":"9","id_carrier":"3","current_state":"5","module":"ps_checkout","invoice_number":"2","invoice_date":"2026-09-28 17:44:05","delivery_number":"1","delivery_date":"2026-09-30 10:02:11","valid":"1","date_add":"2026-09-28 17:44:02","date_upd":"2026-09-30 10:02:11","shipping_number":"","note":"","id_shop_group":"1","id_shop":"1","payment":"Kreditkarte","recyclable":"0","gift":"0","gift_message":"","mobile_theme":"0",
  "total_discounts":"0.000000","total_discounts_tax_incl":"0.000000","total_discounts_tax_excl":"0.000000","total_paid":"34.510000","total_paid_tax_incl":"34.510000","total_paid_tax_excl":"29.000000","total_paid_real":"34.510000","total_products":"24.000000","total_products_wt":"28.560000","total_shipping":"5.950000","total_shipping_tax_incl":"5.950000","total_shipping_tax_excl":"5.000000","carrier_tax_rate":"19.000","total_wrapping":"0.000000","total_wrapping_tax_incl":"0.000000","total_wrapping_tax_excl":"0.000000","round_mode":"2","round_type":"1","conversion_rate":"1.000000","reference":"WQBDNMSRA",
  "associations":{"order_rows":[{"id":"85","product_id":"14","product_attribute_id":"32","product_quantity":"2","product_name":"Notizbuch A5 Leinen - Farbe : Blau","product_reference":"NB-A5-L","product_ean13":"","product_isbn":"","product_upc":"","product_price":"12.000000","id_customization":"0","unit_price_tax_incl":"14.280000","unit_price_tax_excl":"12.000000"}]}},
 {"id":41,"id_address_delivery":"23","id_address_invoice":"23","id_cart":"57","id_currency":"1","id_lang":"1","id_customer":"12","id_carrier":"3","current_state":"2","module":"ps_checkout","invoice_number":"3","invoice_date":"2026-10-05 09:12:33","delivery_number":"0","delivery_date":"0000-00-00 00:00:00","valid":"1","date_add":"2026-10-05 09:12:31","date_upd":"2026-10-05 09:12:33","shipping_number":"","note":"","id_shop_group":"1","id_shop":"1","payment":"Kreditkarte","recyclable":"0","gift":"0","gift_message":"","mobile_theme":"0",
  "total_discounts":"0.000000","total_discounts_tax_incl":"0.000000","total_discounts_tax_excl":"0.000000","total_paid":"76.160000","total_paid_tax_incl":"76.160000","total_paid_tax_excl":"64.000000","total_paid_real":"76.160000","total_products":"59.000000","total_products_wt":"70.210000","total_shipping":"5.950000","total_shipping_tax_incl":"5.950000","total_shipping_tax_excl":"5.000000","carrier_tax_rate":"19.000","total_wrapping":"0.000000","total_wrapping_tax_incl":"0.000000","total_wrapping_tax_excl":"0.000000","round_mode":"2","round_type":"1","conversion_rate":"1.000000","reference":"KZRLWPQAE",
  "associations":{"order_rows":[{"id":"88","product_id":"14","product_attribute_id":"31","product_quantity":"2","product_name":"Notizbuch A5 Leinen - Farbe : Schwarz","product_reference":"NB-A5-L","product_ean13":"","product_isbn":"","product_upc":"","product_price":"12.000000","id_customization":"0","unit_price_tax_incl":"14.280000","unit_price_tax_excl":"12.000000"},
   {"id":"89","product_id":"21","product_attribute_id":"0","product_quantity":"1","product_name":"Füllfederhalter Classic","product_reference":"FH-CL","product_ean13":"","product_isbn":"","product_upc":"","product_price":"35.000000","id_customization":"0","unit_price_tax_incl":"41.650000","unit_price_tax_excl":"35.000000"}]}},
 {"id":42,"id_address_delivery":"27","id_address_invoice":"27","id_cart":"58","id_currency":"1","id_lang":"1","id_customer":"15","id_carrier":"3","current_state":"10","module":"ps_wirepayment","invoice_number":"0","invoice_date":"0000-00-00 00:00:00","delivery_number":"0","delivery_date":"0000-00-00 00:00:00","valid":"0","date_add":"2026-10-06 14:03:10","date_upd":"2026-10-06 14:03:10","shipping_number":"","note":"","id_shop_group":"1","id_shop":"1","payment":"Banküberweisung","recyclable":"0","gift":"0","gift_message":"","mobile_theme":"0",
  "total_discounts":"0.000000","total_discounts_tax_incl":"0.000000","total_discounts_tax_excl":"0.000000","total_paid":"51.360000","total_paid_tax_incl":"51.360000","total_paid_tax_excl":"48.000000","total_paid_real":"0.000000","total_products":"48.000000","total_products_wt":"51.360000","total_shipping":"0.000000","total_shipping_tax_incl":"0.000000","total_shipping_tax_excl":"0.000000","carrier_tax_rate":"19.000","total_wrapping":"0.000000","total_wrapping_tax_incl":"0.000000","total_wrapping_tax_excl":"0.000000","round_mode":"2","round_type":"1","conversion_rate":"1.000000","reference":"TNVHGBXSD",
  "associations":{"order_rows":[{"id":"90","product_id":"30","product_attribute_id":"0","product_quantity":"2","product_name":"Fachbuch E-Rechnung in der Praxis","product_reference":"BU-ER-26","product_ean13":"","product_isbn":"","product_upc":"","product_price":"24.000000","id_customization":"0","unit_price_tax_incl":"25.680000","unit_price_tax_excl":"24.000000"}]}}
],
"order_details":[
 {"id":85,"id_order":"40","product_id":"14","product_attribute_id":"32","id_order_invoice":"2","id_shop":"1","id_customization":"0","product_name":"Notizbuch A5 Leinen - Farbe : Blau","product_quantity":"2","product_quantity_in_stock":"2","product_quantity_return":"0","product_quantity_refunded":"0","product_quantity_reinjected":"0","product_price":"12.000000","reduction_percent":"0.00","reduction_amount":"0.000000","reduction_amount_tax_incl":"0.000000","reduction_amount_tax_excl":"0.000000","group_reduction":"0.00","product_quantity_discount":"0.000000","product_ean13":"","product_reference":"NB-A5-L","tax_computation_method":"0","id_tax_rules_group":"1","ecotax":"0.000000","ecotax_tax_rate":"0.000",
  "unit_price_tax_incl":"14.280000","unit_price_tax_excl":"12.000000","total_price_tax_incl":"28.560000","total_price_tax_excl":"24.000000","total_shipping_price_tax_excl":"0.000000","total_shipping_price_tax_incl":"0.000000","original_product_price":"12.000000","total_refunded_tax_excl":"0.000000","total_refunded_tax_incl":"0.000000","associations":{"taxes":[{"id":"1"}]}},
 {"id":88,"id_order":"41","product_id":"14","product_attribute_id":"31","id_order_invoice":"3","id_shop":"1","id_customization":"0","product_name":"Notizbuch A5 Leinen - Farbe : Schwarz","product_quantity":"2","product_quantity_in_stock":"2","product_quantity_return":"0","product_quantity_refunded":"0","product_quantity_reinjected":"0","product_price":"12.000000","reduction_percent":"0.00","reduction_amount":"0.000000","reduction_amount_tax_incl":"0.000000","reduction_amount_tax_excl":"0.000000","group_reduction":"0.00","product_quantity_discount":"0.000000","product_ean13":"","product_reference":"NB-A5-L","tax_computation_method":"0","id_tax_rules_group":"1","ecotax":"0.000000","ecotax_tax_rate":"0.000",
  "unit_price_tax_incl":"14.280000","unit_price_tax_excl":"12.000000","total_price_tax_incl":"28.560000","total_price_tax_excl":"24.000000","total_shipping_price_tax_excl":"0.000000","total_shipping_price_tax_incl":"0.000000","original_product_price":"12.000000","total_refunded_tax_excl":"0.000000","total_refunded_tax_incl":"0.000000","associations":{"taxes":[{"id":"1"}]}},
 {"id":89,"id_order":"41","product_id":"21","product_attribute_id":"0","id_order_invoice":"3","id_shop":"1","id_customization":"0","product_name":"Füllfederhalter Classic","product_quantity":"1","product_quantity_in_stock":"1","product_quantity_return":"0","product_quantity_refunded":"0","product_quantity_reinjected":"0","product_price":"35.000000","reduction_percent":"0.00","reduction_amount":"0.000000","reduction_amount_tax_incl":"0.000000","reduction_amount_tax_excl":"0.000000","group_reduction":"0.00","product_quantity_discount":"0.000000","product_ean13":"","product_reference":"FH-CL","tax_computation_method":"0","id_tax_rules_group":"1","ecotax":"0.000000","ecotax_tax_rate":"0.000",
  "unit_price_tax_incl":"41.650000","unit_price_tax_excl":"35.000000","total_price_tax_incl":"41.650000","total_price_tax_excl":"35.000000","total_shipping_price_tax_excl":"0.000000","total_shipping_price_tax_incl":"0.000000","original_product_price":"35.000000","total_refunded_tax_excl":"0.000000","total_refunded_tax_incl":"0.000000","associations":{"taxes":[{"id":"1"}]}},
 {"id":90,"id_order":"42","product_id":"30","product_attribute_id":"0","id_order_invoice":"0","id_shop":"1","id_customization":"0","product_name":"Fachbuch E-Rechnung in der Praxis","product_quantity":"2","product_quantity_in_stock":"2","product_quantity_return":"0","product_quantity_refunded":"0","product_quantity_reinjected":"0","product_price":"24.000000","reduction_percent":"0.00","reduction_amount":"0.000000","reduction_amount_tax_incl":"0.000000","reduction_amount_tax_excl":"0.000000","group_reduction":"0.00","product_quantity_discount":"0.000000","product_ean13":"","product_reference":"BU-ER-26","tax_computation_method":"0","id_tax_rules_group":"2","ecotax":"0.000000","ecotax_tax_rate":"0.000",
  "unit_price_tax_incl":"25.680000","unit_price_tax_excl":"24.000000","total_price_tax_incl":"51.360000","total_price_tax_excl":"48.000000","total_shipping_price_tax_excl":"0.000000","total_shipping_price_tax_incl":"0.000000","original_product_price":"24.000000","total_refunded_tax_excl":"0.000000","total_refunded_tax_incl":"0.000000","associations":{"taxes":[{"id":"2"}]}}
],
"order_payments":[
 {"id":17,"order_reference":"WQBDNMSRA","id_currency":"1","amount":"34.510000","payment_method":"Kreditkarte","conversion_rate":"1.000000","transaction_id":"pi_sim_0040","card_number":"","card_brand":"","card_expiration":"","card_holder":"","date_add":"2026-09-28 17:44:05"},
 {"id":19,"order_reference":"KZRLWPQAE","id_currency":"1","amount":"76.160000","payment_method":"Kreditkarte","conversion_rate":"1.000000","transaction_id":"pi_sim_0041","card_number":"","card_brand":"","card_expiration":"","card_holder":"","date_add":"2026-10-05 09:12:33"}
],
"order_slip":[],
"order_states":[
 {"id":2,"unremovable":"1","delivery":"0","hidden":"0","send_email":"1","module_name":"","invoice":"1","color":"#3498D8","logable":"1","shipped":"0","paid":"1","pdf_delivery":"0","pdf_invoice":"1","deleted":"0","name":[{"id":"1","value":"Zahlung eingegangen"},{"id":"2","value":"Payment accepted"}],"template":[{"id":"1","value":"payment"},{"id":"2","value":"payment"}]},
 {"id":10,"unremovable":"1","delivery":"0","hidden":"0","send_email":"1","module_name":"ps_wirepayment","invoice":"0","color":"#34209E","logable":"0","shipped":"0","paid":"0","pdf_delivery":"0","pdf_invoice":"0","deleted":"0","name":[{"id":"1","value":"Warten auf Zahlungseingang"},{"id":"2","value":"Awaiting bank wire payment"}],"template":[{"id":"1","value":"bankwire"},{"id":"2","value":"bankwire"}]}
],
"addresses":[
 {"id":23,"id_customer":"12","id_manufacturer":"0","id_supplier":"0","id_warehouse":"0","id_country":"1","id_state":"0","alias":"Meine Adresse","company":"","lastname":"Noether","firstname":"Emmy","vat_number":"","address1":"Lindenallee 7","address2":"","postcode":"37073","city":"Göttingen","other":"","phone":"","phone_mobile":"","dni":"","deleted":"0","date_add":"2026-10-05 09:05:12","date_upd":"2026-10-05 09:05:12"},
 {"id":27,"id_customer":"15","id_manufacturer":"0","id_supplier":"0","id_warehouse":"0","id_country":"1","id_state":"0","alias":"Firma","company":"Meitner Messtechnik GmbH","lastname":"Meitner","firstname":"Lise","vat_number":"","address1":"Werftstraße 21","address2":"Halle 3","postcode":"24143","city":"Kiel","other":"","phone":"","phone_mobile":"","dni":"","deleted":"0","date_add":"2026-10-06 13:58:40","date_upd":"2026-10-06 13:58:40"}
],
"customers":[
 {"id":12,"id_default_group":"3","id_lang":"1","newsletter_date_add":"0000-00-00 00:00:00","ip_registration_newsletter":"","last_passwd_gen":"2026-10-05 09:05:12","deleted":"0","lastname":"Noether","firstname":"Emmy","email":"emmy@example.com","id_gender":"2","birthday":"0000-00-00","newsletter":"0","optin":"0","website":"","company":"","siret":"","ape":"","outstanding_allow_amount":"0.000000","show_public_prices":"0","id_risk":"0","max_payment_days":"0","active":"1","note":"","is_guest":"0","id_shop":"1","id_shop_group":"1","date_add":"2026-10-05 09:05:12","date_upd":"2026-10-05 09:05:12","associations":{"groups":[{"id":"3"}]}},
 {"id":15,"id_default_group":"2","id_lang":"1","newsletter_date_add":"0000-00-00 00:00:00","ip_registration_newsletter":"","last_passwd_gen":"2026-10-06 13:58:40","deleted":"0","lastname":"Meitner","firstname":"Lise","email":"einkauf@meitner-messtechnik.example","id_gender":"2","birthday":"0000-00-00","newsletter":"0","optin":"0","website":"","company":"","siret":"","ape":"","outstanding_allow_amount":"0.000000","show_public_prices":"0","id_risk":"0","max_payment_days":"0","active":"1","note":"","is_guest":"1","id_shop":"1","id_shop_group":"1","date_add":"2026-10-06 13:58:40","date_upd":"2026-10-06 13:58:40","associations":{"groups":[{"id":"2"}]}}
],
"countries":[
 {"id":1,"id_zone":"1","id_currency":"0","call_prefix":"49","iso_code":"DE","active":"1","contains_states":"0","need_identification_number":"0","need_zip_code":"1","zip_code_format":"NNNNN","display_tax_label":"1","name":[{"id":"1","value":"Deutschland"},{"id":"2","value":"Germany"}]}
],
"currencies":[
 {"id":1,"names":[{"id":"1","value":"Euro"},{"id":"2","value":"Euro"}],"name":"Euro","symbol":[{"id":"1","value":"€"},{"id":"2","value":"€"}],"iso_code":"EUR","numeric_iso_code":"978","precision":"2","conversion_rate":"1.000000","deleted":"0","active":"1","unofficial":"0","modified":"0","pattern":[{"id":"1","value":""},{"id":"2","value":""}]}
],
"taxes":[
 {"id":1,"rate":"19.000","active":"1","deleted":"0","name":[{"id":"1","value":"USt. DE 19%"},{"id":"2","value":"VAT DE 19%"}]},
 {"id":2,"rate":"7.000","active":"1","deleted":"0","name":[{"id":"1","value":"USt. DE 7%"},{"id":"2","value":"VAT DE 7%"}]}
]
}`

// simulatePrestaShop answers the Webservice of a PrestaShop 9 shop in a
// subdirectory: the key as Basic user name, JSON output, and display, filter,
// date, sort and limit on lists. Like the real shop it encodes an empty list
// as [] and drops fields PHP deems empty ("", "0") from single resources
// (array_filter in WebserviceOutputJSON since PrestaShop 8.0).
func simulatePrestaShop(t *testing.T) shopSimulation {
	const key = "SIMKZ7Q2X9D4M8P1R6TW3YB5NC0HFJ8L"
	var rows map[string][]map[string]any
	decoder := json.NewDecoder(strings.NewReader(prestaShopSimulationRows))
	decoder.UseNumber()
	if err := decoder.Decode(&rows); err != nil {
		t.Fatal(err)
	}
	// The key may read these resources: single and list node names.
	nodes := map[string][2]string{
		"orders": {"order", "orders"}, "order_details": {"order_detail", "order_details"},
		"order_payments": {"order_payment", "order_payments"}, "order_slip": {"order_slip", "order_slips"},
		"order_states": {"order_state", "order_states"}, "addresses": {"address", "addresses"},
		"customers": {"customer", "customers"}, "countries": {"country", "countries"},
		"currencies": {"currency", "currencies"}, "taxes": {"tax", "taxes"},
	}
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reply := func(status int, body any) {
			w.Header().Set("Content-Type", "application/json;charset=utf-8")
			w.WriteHeader(status)
			_ = json.NewEncoder(w).Encode(body)
		}
		fail := func(status, code int, message string) {
			reply(status, map[string]any{"errors": []any{map[string]any{"code": code, "message": message}}})
		}
		path, api := strings.CutPrefix(r.URL.Path, "/shop/api/")
		if !api {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("PSWS-Version", "9.0.0")
		// The key is the Basic user name; PrestaShop ignores the password.
		if user, _, ok := r.BasicAuth(); user != key {
			code, message := 20, "Authentification key is not active"
			if !ok {
				code, message = 16, "Please enter the authentication key as the login. No password required"
			} else if len(user) != 32 {
				code, message = 18, "Invalid authentication key format"
			}
			w.Header().Set("WWW-Authenticate", `Basic realm="Welcome to PrestaShop Webservice, please enter the authentication key as the login. No password required."`)
			fail(http.StatusUnauthorized, code, message)
			return
		}
		resource, id, single := strings.Cut(path, "/")
		node, allowed := nodes[resource]
		if !allowed {
			fail(http.StatusUnauthorized, 26, fmt.Sprintf("Resource of type %q is not allowed with this authentication key", resource))
			return
		}
		if r.Method != http.MethodGet {
			fail(http.StatusMethodNotAllowed, 25, "Method "+r.Method+" is not allowed for the resource "+resource+" with this authentication key")
			return
		}
		query := r.URL.Query()
		if query.Get("output_format") != "JSON" {
			w.Header().Set("Content-Type", "text/xml;charset=utf-8")
			_, _ = w.Write([]byte(`<?xml version="1.0" encoding="UTF-8"?>` + "\n" + `<prestashop xmlns:xlink="http://www.w3.org/1999/xlink"/>`))
			return
		}
		list := slices.Clone(rows[resource])
		_, display := query["display"]
		if single {
			i := slices.IndexFunc(list, func(row map[string]any) bool { return fmt.Sprint(row["id"]) == id })
			if i < 0 {
				w.WriteHeader(http.StatusNotFound)
				return
			}
			list = list[i : i+1]
			if !display {
				// PHP's array_filter removes "", "0", null and empty arrays.
				entity := map[string]any{}
				for field, value := range list[0] {
					if text, ok := value.(string); ok && (text == "" || text == "0") {
						continue
					}
					if items, ok := value.([]any); value == nil || (ok && len(items) == 0) {
						continue
					}
					entity[field] = value
				}
				reply(http.StatusOK, map[string]any{node[0]: entity})
				return
			}
		} else {
			for param := range query {
				field, ok := strings.CutPrefix(param, "filter[")
				if !ok {
					continue
				}
				field = strings.TrimSuffix(field, "]")
				value := query.Get(param)
				// Date columns are filterable only with date=1.
				if (field == "date_add" || field == "date_upd") && query.Get("date") != "1" {
					fail(http.StatusBadRequest, 32, "This filter does not exist")
					return
				}
				if len(value) < 2 || value[0] != '[' || value[len(value)-1] != ']' {
					fail(http.StatusBadRequest, 32, "This filter does not exist")
					return
				}
				// [a|b] lists values, [a,b] is an interval; dates compare as text.
				list = slices.DeleteFunc(list, func(row map[string]any) bool {
					actual, present := row[field]
					text := fmt.Sprint(actual)
					for _, alternative := range strings.Split(value[1:len(value)-1], "|") {
						low, high, interval := strings.Cut(alternative, ",")
						if present && ((interval && text >= low && text <= high) || (!interval && text == alternative)) {
							return false
						}
					}
					return true
				})
			}
			switch query.Get("sort") {
			case "", "[id_ASC]":
			case "[id_DESC]":
				slices.Reverse(list)
			default:
				fail(http.StatusBadRequest, 38, "Unable to sort by this field")
				return
			}
			// limit is "offset,count" or "count".
			if limit := query.Get("limit"); limit != "" {
				offset, count, paged := strings.Cut(limit, ",")
				if !paged {
					offset, count = "0", limit
				}
				from, fromErr := strconv.Atoi(offset)
				n, countErr := strconv.Atoi(count)
				if fromErr != nil || countErr != nil || from < 0 || n < 0 {
					fail(http.StatusBadRequest, 39, "The limit has to be formed as 'offset,count' or 'count'")
					return
				}
				list = list[min(from, len(list)):min(from+n, len(list))]
			}
		}
		// Without display a list names only IDs; display=[a,b] picks fields.
		shown := make([]any, len(list))
		for i, row := range list {
			switch fields := query.Get("display"); {
			case fields == "full":
				shown[i] = row
			case !display:
				shown[i] = map[string]any{"id": row["id"]}
			default:
				picked := map[string]any{}
				for _, field := range strings.Split(strings.Trim(fields, "[]"), ",") {
					if value, ok := row[field]; ok {
						picked[field] = value
					}
				}
				shown[i] = picked
			}
		}
		if len(shown) == 0 {
			// Nothing was rendered, so PHP encodes the empty content array as [].
			reply(http.StatusOK, []any{})
			return
		}
		reply(http.StatusOK, map[string]any{node[1]: shown})
	})
	return shopSimulation{
		connection:  shops.Connection{Provider: "prestashop", Label: "PrestaShop-Simulation", URL: "https://prestashop.shop-simulation.example/shop"},
		credentials: shops.Credentials{APIKey: key},
		handler:     handler,
		orders: []simulatedOrder{
			{number: "KZRLWPQAE", gross: "76.16", paid: true},
			{number: "TNVHGBXSD", gross: "51.36"},
		},
	}
}
