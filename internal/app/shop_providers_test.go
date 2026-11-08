package app

import (
	"encoding/json"
	"strings"
	"testing"

	"papierleicht.de/e-rechnung-studio/internal/shops"
)

func additionalShopCredentials(provider string) shops.Credentials {
	switch provider {
	case "shopware":
		return shops.Credentials{ClientID: "private-shopware-id", ClientSecret: "private-shopware-secret"}
	case "gambio":
		return shops.Credentials{Username: "api@example.com", Password: " private-gambio-password "}
	default:
		return shops.Credentials{APIKey: "private-" + provider + "-key"}
	}
}

func additionalShopConnection(provider string) shops.Connection {
	c := shopConnectionFixture()
	c.Provider = provider
	if provider == "wix" {
		c.SiteID = "12345678-1234-1234-1234-123456789abc"
	}
	return c
}

func TestAdditionalShopProvidersUseInvoiceWorkflowAndSecretVault(t *testing.T) {
	s, f, _, _ := shopServiceFixture(t)
	f.orders = []shops.Order{shopOrderFixture()}
	for _, provider := range []string{"shopware", "gambio", "wix", "prestashop"} {
		t.Run(provider, func(t *testing.T) {
			credentials := additionalShopCredentials(provider)
			connection, err := s.Connect(additionalShopConnection(provider), credentials)
			if err != nil {
				t.Fatal(err)
			}
			stored, err := s.credentials(connection)
			if err != nil || stored != credentials {
				t.Fatalf("credentials did not round-trip: %v", err)
			}
			orders, err := s.invoices.store.ShopOrders(connection.ID)
			if err != nil || len(orders) != 1 || orders[0].InvoiceID == "" {
				t.Fatalf("orders=%+v err=%v", orders, err)
			}
			draft, err := s.CreateInvoice(orders[0].ID)
			if err != nil || draft.CustomerID == "" || draft.Currency != "EUR" {
				t.Fatalf("draft=%+v err=%v", draft, err)
			}
			if result, err := s.Sync(connection.ID); err != nil || result.Imported != 0 || result.Drafts != 0 {
				t.Fatalf("duplicate sync: %+v %v", result, err)
			}
			reconnected, err := s.Connect(additionalShopConnection(provider), credentials)
			if err != nil || reconnected.ID != connection.ID {
				t.Fatal("reconnect duplicated shop", err)
			}
		})
	}
	lists, err := s.invoices.ApplicationLists()
	if err != nil || len(lists.Invoices) != 4 || len(lists.Customers) != 4 {
		t.Fatalf("provider namespaces not isolated: %+v %v", lists, err)
	}
	overview, _ := s.Overview()
	metadata, _ := json.Marshal(overview)
	backup, err := s.invoices.ExportBackup()
	if err != nil {
		t.Fatal(err)
	}
	for _, secret := range []string{"private-shopware-id", "private-shopware-secret", "private-gambio-password", "private-wix-key", "private-prestashop-key"} {
		if strings.Contains(backup, secret) || strings.Contains(string(metadata), secret) {
			t.Fatal("shop secrets exposed")
		}
	}
	if !strings.Contains(backup, "12345678-1234-1234-1234-123456789abc") {
		t.Fatal("nonsecret Wix identity missing from backup")
	}
}

func TestWixSiteIdentityCannotChangeOrBeConnectedTwice(t *testing.T) {
	s, f, _, _ := shopServiceFixture(t)
	f.orders = []shops.Order{shopOrderFixture()}
	c, err := s.Connect(additionalShopConnection("wix"), additionalShopCredentials("wix"))
	if err != nil {
		t.Fatal(err)
	}
	before := f.calls
	changed := c
	changed.SiteID = "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa"
	if _, err := s.Connect(changed, additionalShopCredentials("wix")); err == nil {
		t.Fatal("Wix site identity changed")
	}
	alias := additionalShopConnection("wix")
	alias.URL = "https://other-domain.example"
	if _, err := s.Connect(alias, additionalShopCredentials("wix")); err == nil {
		t.Fatal("same Wix site connected twice")
	}
	if f.calls != before {
		t.Fatal("rebound connection reached remote fetcher")
	}
	alias.ID = "second-connection"
	if err := s.invoices.store.SaveShopConnection(alias); err == nil {
		t.Fatal("Wix site uniqueness not enforced by database")
	}
	connections, _ := s.invoices.store.ShopConnections()
	if len(connections) != 1 || connections[0].SiteID != c.SiteID {
		t.Fatal("existing identity overwritten")
	}
}

func TestWixRestoreCannotRebindCredentialSite(t *testing.T) {
	s, f, _, _ := shopServiceFixture(t)
	c, err := s.Connect(additionalShopConnection("wix"), additionalShopCredentials("wix"))
	if err != nil {
		t.Fatal(err)
	}
	backup, err := s.invoices.ExportBackup()
	if err != nil {
		t.Fatal(err)
	}
	changed := strings.ReplaceAll(backup, c.SiteID, "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa")
	if err := s.invoices.RestoreBackup(changed); err != nil {
		t.Fatal(err)
	}
	before := f.calls
	if _, err := s.Sync(c.ID); err == nil {
		t.Fatal("restored site reused other site's keychain secret")
	}
	if f.calls != before {
		t.Fatal("restored site reached remote fetcher")
	}
	metadata, err := s.Overview()
	if err != nil || metadata.Connections[0].HasCredentials {
		t.Fatal("restored site incorrectly authenticated", err)
	}
}
