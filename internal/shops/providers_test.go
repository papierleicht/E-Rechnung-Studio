package shops

import (
	"strings"
	"testing"

	"papierleicht.de/e-rechnung-studio/internal/engine"
	"papierleicht.de/e-rechnung-studio/internal/model"
)

func TestAdditionalProvidersValidateCredentialsAndOrigins(t *testing.T) {
	for _, provider := range []string{"shopware", "gambio", "wix", "prestashop"} {
		t.Run(provider, func(t *testing.T) {
			connection := Connection{Provider: strings.ToUpper(provider), URL: "https://Shop.Example:443/subdir/"}
			if provider == "wix" {
				connection.SiteID = "12345678-1234-ABCD-1234-123456789ABC"
			}
			normalized, err := NormalizeConnection(connection)
			if err != nil || normalized.Provider != provider || normalized.URL != "https://shop.example/subdir" {
				t.Fatalf("normalization: %+v %v", normalized, err)
			}
			valid := Credentials{}
			switch provider {
			case "shopware":
				valid.ClientID, valid.ClientSecret = "access-id", "secret"
			case "gambio":
				valid.Username, valid.Password = "api@example.com", " password with spaces "
			default:
				valid.APIKey = "private-key"
			}
			if err := ValidateCredentials(provider, valid); err != nil {
				t.Fatal(err)
			}
			if err := ValidateCredentials(provider, Credentials{}); err == nil {
				t.Fatal("empty credentials accepted")
			}
			invalid := valid
			switch provider {
			case "shopware":
				invalid.ClientSecret = "bad\nsecret"
			case "gambio":
				invalid.Username = "user:password"
			default:
				invalid.APIKey = "bad\nkey"
			}
			if err := ValidateCredentials(provider, invalid); err == nil {
				t.Fatal("ambiguous credentials accepted")
			}
			for _, unsafe := range []string{"http://shop.example", "https://user:secret@shop.example", "https://shop.example?key=secret", "https://shop.example/#secret", "https://shop.example:8443", "https://shop.example/%2fsecret"} {
				connection.URL = unsafe
				if _, err := NormalizeConnection(connection); err == nil {
					t.Fatalf("unsafe origin accepted: %s", unsafe)
				}
			}
		})
	}
	for _, connection := range []Connection{
		{Provider: "wix", URL: "https://shop.example"},
		{Provider: "wix", URL: "https://shop.example", SiteID: "site\r\nheader"},
		{Provider: "woocommerce", URL: "https://shop.example", SiteID: "12345678-1234-1234-1234-123456789abc"},
	} {
		if _, err := NormalizeConnection(connection); err == nil {
			t.Fatal("invalid site identity accepted")
		}
	}
}

func TestDocumentAdjustmentsReconcileSourceAndInvoiceEngine(t *testing.T) {
	o := Order{Currency: "EUR", Gross: "107.10", CreatedAt: "2026-10-15T10:00:00Z", UpdatedAt: "2026-10-15T10:00:00Z",
		Customer:    customer("Ada", "Lovelace", "", "ada@example.com", "Testweg 1", "", "10115", "Berlin", "DE"),
		Adjustments: []model.InvoiceAdjustment{{ID: "discount", Amount: "5.00", Reason: "Shop-Rabatt", TaxRate: "19", TaxCategory: "S"}},
	}
	lines := []normalizedLine{
		{id: "product", name: "Artikel", quantity: "3", net: "90", tax: "17.10", rate: "19", knownRate: true},
		{id: "shipping", name: "Versand", quantity: "1", net: "5", tax: "0.95", rate: "19", knownRate: true},
	}
	finishOrder(&o, lines, "17.10")
	if o.ReviewReason != "" {
		t.Fatal(o.ReviewReason)
	}
	d := model.NewDraft()
	d.TaxTreatment, d.Customer, d.Lines, d.Adjustments = "mixed", o.Customer, o.Lines, o.Adjustments
	inv, err := engine.New().Build(d, model.Company{})
	if err != nil || inv.GrandTotal.StringFixed(2) != o.Gross {
		t.Fatalf("invoice/source mismatch: %v %v", inv, err)
	}
	for _, adjustment := range []model.InvoiceAdjustment{
		{Amount: "1000", TaxRate: "19", TaxCategory: "S"},
		{Amount: "5", TaxRate: "7", TaxCategory: "S"},
		{Amount: "0", TaxRate: "19", TaxCategory: "S"},
		{Amount: "5", TaxRate: "0", TaxCategory: "S"},
	} {
		bad := o
		bad.Lines, bad.Adjustments, bad.ReviewReason = nil, []model.InvoiceAdjustment{adjustment}, ""
		finishOrder(&bad, lines, "17.10")
		if bad.ReviewReason == "" {
			t.Fatalf("unsafe adjustment accepted: %+v", adjustment)
		}
	}
}
