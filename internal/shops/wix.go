package shops

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/shopspring/decimal"
)

const wixOrdersEndpoint = "https://www.wixapis.com/ecom/v1/orders/search"

type wixPrice struct {
	Amount string `json:"amount"`
}
type wixTax struct {
	Rate      string   `json:"taxRate"`
	Amount    wixPrice `json:"taxAmount"`
	Taxable   wixPrice `json:"taxableAmount"`
	Breakdown []struct {
		Rate   string   `json:"rate"`
		Amount wixPrice `json:"taxAmount"`
	} `json:"taxBreakdown"`
}
type wixLegacyTax struct {
	Rate   string   `json:"taxRate"`
	Amount wixPrice `json:"totalTax"`
}
type wixLine struct {
	ID   string `json:"id"`
	Name struct {
		Original string `json:"original"`
	} `json:"productName"`
	Quantity       json.Number   `json:"quantity"`
	Net            wixPrice      `json:"totalPriceBeforeTax"`
	Gross          wixPrice      `json:"totalPriceAfterTax"`
	Tax            *wixTax       `json:"taxInfo"`
	LegacyTax      *wixLegacyTax `json:"taxDetails"`
	RefundQuantity json.Number   `json:"refundQuantity"`
	PaymentOption  string        `json:"paymentOption"`
	Undetermined   bool          `json:"priceUndetermined"`
	Physical       struct {
		SKU string `json:"sku"`
	} `json:"physicalProperties"`
	ItemType struct {
		Preset string `json:"preset"`
	} `json:"itemType"`
	DecimalQuantity json.RawMessage `json:"decimalQuantity"`
}
type wixShippingCost struct {
	Net       wixPrice      `json:"totalPriceBeforeTax"`
	Gross     wixPrice      `json:"totalPriceAfterTax"`
	Tax       *wixTax       `json:"taxInfo"`
	LegacyTax *wixLegacyTax `json:"taxDetails"`
}
type wixOrder struct {
	ID            string      `json:"id"`
	Number        json.Number `json:"number"`
	Created       string      `json:"createdDate"`
	Updated       string      `json:"updatedDate"`
	Currency      string      `json:"currency"`
	Status        string      `json:"status"`
	PaymentStatus string      `json:"paymentStatus"`
	Lines         []wixLine   `json:"lineItems"`
	Buyer         struct {
		MemberID  string `json:"memberId"`
		ContactID string `json:"contactId"`
		Email     string `json:"email"`
	} `json:"buyerInfo"`
	Billing struct {
		Address struct {
			Line       string `json:"addressLine"`
			Additional string `json:"addressLine2"`
			City       string `json:"city"`
			Postcode   string `json:"postalCode"`
			Country    string `json:"country"`
			Region     string `json:"subdivision"`
			Street     struct {
				Name   string `json:"name"`
				Number string `json:"number"`
			} `json:"streetAddress"`
		} `json:"address"`
		Contact struct {
			First   string `json:"firstName"`
			Last    string `json:"lastName"`
			Company string `json:"company"`
		} `json:"contactDetails"`
	} `json:"billingInfo"`
	Summary struct {
		Total    wixPrice `json:"total"`
		Tax      wixPrice `json:"tax"`
		Shipping wixPrice `json:"shipping"`
	} `json:"priceSummary"`
	TaxInfo *struct {
		Total  wixPrice `json:"totalTax"`
		Exempt bool     `json:"taxExempt"`
	} `json:"taxInfo"`
	Shipping *struct {
		Title string          `json:"title"`
		Cost  wixShippingCost `json:"cost"`
	} `json:"shippingInfo"`
	Fees []struct {
		ID        string        `json:"id"`
		Name      string        `json:"name"`
		Net       wixPrice      `json:"priceBeforeTax"`
		Gross     wixPrice      `json:"priceAfterTax"`
		Tax       *wixTax       `json:"taxInfo"`
		LegacyTax *wixLegacyTax `json:"taxDetails"`
	} `json:"additionalFees"`
	Balance *struct {
		Paid          wixPrice `json:"paid"`
		Refunded      wixPrice `json:"refunded"`
		PendingRefund wixPrice `json:"pendingRefund"`
		Chargeback    wixPrice `json:"chargeback"`
	} `json:"balanceSummary"`
}

func (c *Client) fetchWix(ctx context.Context, conn Connection, credentials Credentials, since time.Time) ([]Order, error) {
	orders := []Order{}
	cursor := ""
	seenCursors, seenOrders := map[string]bool{}, map[string]bool{}
	for page := 0; page < maxPages; page++ {
		paging := map[string]any{"limit": 100}
		if cursor != "" {
			paging["cursor"] = cursor
		}
		search := map[string]any{"cursorPaging": paging}
		if cursor == "" {
			search["filter"] = map[string]any{
				"updatedDate": map[string]string{"$gte": since.UTC().Format(time.RFC3339)},
				// An explicit status filter also includes pending/rejected orders,
				// which Wix otherwise silently excludes from search results.
				"status": map[string]any{"$in": []string{"APPROVED", "CANCELED", "PENDING", "REJECTED"}},
			}
			search["sort"] = []map[string]string{{"fieldName": "updatedDate", "order": "ASC"}}
		}
		// The continuation cursor encodes the initial search criteria. Sending
		// new filters/sorting beside it can invalidate the opaque Wix cursor.
		body, _ := json.Marshal(map[string]any{"search": search})
		var result struct {
			Orders   *[]wixOrder `json:"orders"`
			Metadata *struct {
				Count   *int  `json:"count"`
				HasNext *bool `json:"hasNext"`
				Cursors struct {
					Next    string `json:"next"`
					HasNext *bool  `json:"hasNext"`
				} `json:"cursors"`
			} `json:"metadata"`
		}
		_, err := c.request(ctx, http.MethodPost, wixOrdersEndpoint, body, map[string]string{
			"Content-Type": "application/json", "Authorization": credentials.APIKey, "wix-site-id": conn.SiteID,
		}, nil, &result)
		if err != nil {
			return nil, err
		}
		if result.Orders == nil || result.Metadata == nil {
			return nil, errors.New("Wix hat keine vollständige Bestellantwort geliefert. Bitte API-Schlüssel, Website-ID und Bestell-Leserechte prüfen.")
		}
		batch, metadata := *result.Orders, result.Metadata
		if metadata.Count != nil && *metadata.Count != len(batch) {
			return nil, errors.New("Die Wix-Seitenfolge ist unvollständig. Bitte erneut abrufen.")
		}
		for _, raw := range batch {
			if raw.ID == "" || seenOrders[raw.ID] {
				return nil, errors.New("Die Wix-Bestellkennung oder Seitenfolge ist ungültig. Bitte erneut abrufen.")
			}
			seenOrders[raw.ID] = true
			orders = append(orders, normalizeWix(raw))
		}
		hasNext := metadata.HasNext
		if hasNext == nil {
			hasNext = metadata.Cursors.HasNext
		}
		if (hasNext != nil && !*hasNext) || (hasNext == nil && metadata.Cursors.Next == "") {
			return orders, nil
		}
		cursor = metadata.Cursors.Next
		if cursor == "" || len(cursor) > 16384 || seenCursors[cursor] || len(batch) == 0 {
			return nil, errors.New("Die Wix-Seitenfolge ist unvollständig. Bitte erneut abrufen.")
		}
		seenCursors[cursor] = true
	}
	return nil, errors.New("Der Wix-Import überschreitet 100.000 Bestellungen. Bitte einen späteren Importbeginn wählen.")
}

func wixLineTax(o *Order, tax *wixTax, legacy *wixLegacyTax) (string, string, bool) {
	if tax == nil && legacy == nil {
		return "0", "0", false
	}
	rate, amount := "", ""
	if tax != nil {
		rate, amount = tax.Rate, tax.Amount.Amount
		if len(tax.Breakdown) > 1 {
			review(o, "Mehrere oder zusammengesetzte Wix-Steuern je Position müssen geprüft werden.")
		}
		if len(tax.Breakdown) == 1 {
			part := tax.Breakdown[0]
			if rate == "" {
				rate = part.Rate
			}
			if amount == "" {
				amount = part.Amount.Amount
			}
			if part.Amount.Amount != "" && !parseMoney(o, part.Amount.Amount).Equal(parseMoney(o, amount)) {
				review(o, "Die Wix-Steueraufteilung passt nicht zum Positionssteuerbetrag.")
			}
		}
	} else {
		rate, amount = legacy.Rate, legacy.Amount.Amount
	}
	r, err := boundedDecimal(rate)
	if err != nil {
		return amount, "0", false
	}
	return amount, r.Mul(decimal.NewFromInt(100)).String(), true
}

func normalizeWix(raw wixOrder) Order {
	o := Order{ExternalID: raw.ID, Number: raw.Number.String(), CreatedAt: raw.Created, UpdatedAt: raw.Updated,
		Currency: strings.ToUpper(raw.Currency), Gross: raw.Summary.Total.Amount, Status: raw.Status + "/" + raw.PaymentStatus, Paid: raw.PaymentStatus == "PAID"}
	if raw.Buyer.MemberID != "" {
		o.CustomerExternalID = "member:" + raw.Buyer.MemberID
	} else if raw.Buyer.ContactID != "" {
		o.CustomerExternalID = "contact:" + raw.Buyer.ContactID
	}
	a, b := raw.Billing.Address, raw.Billing.Contact
	street := a.Line
	if street == "" {
		street = strings.TrimSpace(a.Street.Name + " " + a.Street.Number)
	}
	o.Customer = customer(b.First, b.Last, b.Company, raw.Buyer.Email, street, addressAdditional(a.Additional, a.Region), a.Postcode, a.City, a.Country)
	if raw.Status != "APPROVED" {
		review(&o, "Die Wix-Bestellung ist nicht freigegeben, wurde abgebrochen oder abgelehnt.")
	}
	switch raw.PaymentStatus {
	case "PAID", "NOT_PAID", "PENDING":
	default:
		review(&o, "Der Wix-Zahlungsstatus benötigt eine Prüfung auf Teilzahlungen, Erstattungen oder fehlgeschlagene Zahlungen.")
	}
	if raw.TaxInfo != nil && raw.TaxInfo.Exempt {
		review(&o, "Die Wix-Bestellung ist steuerbefreit; bitte den Steuerfall prüfen.")
	}
	if raw.Balance != nil {
		for _, amount := range []wixPrice{raw.Balance.Refunded, raw.Balance.PendingRefund, raw.Balance.Chargeback} {
			if amount.Amount != "" && parseMoney(&o, amount.Amount).IsPositive() {
				review(&o, "Wix-Erstattungen oder Rückbuchungen benötigen eine manuelle Prüfung.")
			}
		}
		if raw.Balance.Paid.Amount != "" {
			paid, gross := parseMoney(&o, raw.Balance.Paid.Amount), parseMoney(&o, o.Gross)
			if (o.Paid && !paid.Equal(gross)) || (!o.Paid && paid.IsPositive()) {
				review(&o, "Der Wix-Zahlbetrag passt nicht zum Zahlungsstatus. Bitte Anzahlungen oder Überzahlungen prüfen.")
			}
		}
	}
	lines := []normalizedLine{}
	add := func(id, name, description, quantity string, net, gross wixPrice, tax *wixTax, legacy *wixLegacyTax) {
		taxAmount, rate, known := wixLineTax(&o, tax, legacy)
		if gross.Amount != "" {
			if !parseMoney(&o, net.Amount).Add(parseMoney(&o, taxAmount)).Equal(parseMoney(&o, gross.Amount)) {
				review(&o, "Die Wix-Positionsbeträge stimmen nicht mit Netto und Steuer überein.")
			}
		}
		lines = append(lines, normalizedLine{id: id, name: name, description: description, quantity: quantity, net: net.Amount, tax: taxAmount, rate: rate, knownRate: known})
	}
	for _, line := range raw.Lines {
		if line.ItemType.Preset == "GIFT_CARD" {
			review(&o, "Wix-Gutscheine benötigen eine manuelle steuerliche Prüfung.")
		}
		if line.PaymentOption != "" && line.PaymentOption != "FULL_PAYMENT_ONLINE" && line.PaymentOption != "FULL_PAYMENT_OFFLINE" {
			review(&o, "Die Wix-Zahlungsoption enthält eine Anzahlung oder einen besonderen Zahlungsablauf.")
		}
		if line.RefundQuantity != "" {
			q, err := boundedDecimal(line.RefundQuantity.String())
			if err != nil || !q.IsZero() {
				review(&o, "Erstattete Wix-Positionen benötigen eine manuelle Prüfung.")
			}
		}
		if line.Undetermined || (len(line.DecimalQuantity) > 0 && string(line.DecimalQuantity) != "null") {
			review(&o, "Unbestimmte Preise oder besondere Mengeneinheiten bei Wix müssen geprüft werden.")
		}
		add(line.ID, line.Name.Original, line.Physical.SKU, line.Quantity.String(), line.Net, line.Gross, line.Tax, line.LegacyTax)
	}
	if raw.Shipping != nil {
		shipping := raw.Shipping.Cost
		if shipping.Net.Amount != "" && (!parseMoney(&o, shipping.Net.Amount).IsZero() || (shipping.Gross.Amount != "" && !parseMoney(&o, shipping.Gross.Amount).IsZero())) {
			add("wix-shipping", "Versand: "+raw.Shipping.Title, "", "1", shipping.Net, shipping.Gross, shipping.Tax, shipping.LegacyTax)
		}
	}
	for _, fee := range raw.Fees {
		add("wix-fee:"+fee.ID, fee.Name, "", "1", fee.Net, fee.Gross, fee.Tax, fee.LegacyTax)
	}
	tax := raw.Summary.Tax.Amount
	if raw.TaxInfo != nil && raw.TaxInfo.Total.Amount != "" {
		if tax != "" && !parseMoney(&o, tax).Equal(parseMoney(&o, raw.TaxInfo.Total.Amount)) {
			review(&o, "Die Wix-Gesamtsteuerangaben widersprechen sich.")
		}
		tax = raw.TaxInfo.Total.Amount
	}
	finishOrder(&o, lines, tax)
	return o
}
