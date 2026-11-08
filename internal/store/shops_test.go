package store

import (
	"encoding/json"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"papierleicht.de/e-rechnung-studio/internal/model"
	"papierleicht.de/e-rechnung-studio/internal/shops"
)

func openShopTestStore(t *testing.T) *Store {
	t.Helper()
	s, err := Open(filepath.Join(t.TempDir(), "shops.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func seedShopOrder(t *testing.T, s *Store) shops.Order {
	t.Helper()
	connection := shops.Connection{ID: "connection-1", Provider: "shopify", URL: "https://example.myshopify.com", Label: "Mein Shop", Enabled: true, AutoCreateDrafts: true, HasCredentials: true}
	if err := s.SaveShopConnection(connection); err != nil {
		t.Fatal(err)
	}
	order := shops.Order{
		ConnectionID: connection.ID, ExternalID: "1001", Number: "#1001", CustomerExternalID: "customer-1",
		Customer: model.Customer{Name: "Erika Beispiel", Email: "erika@example.com", Address: model.Address{Street: "Hauptstraße 1", Postcode: "10115", City: "Berlin", Country: "DE"}},
		Currency: "EUR", Gross: "119.00", CreatedAt: "2026-10-14T12:00:00Z", UpdatedAt: "2026-10-14T12:00:00Z", Status: "paid", Paid: true,
		Lines: []model.InvoiceLine{{ID: "line-1", Name: "Artikel", Quantity: "1", Unit: "C62", UnitPrice: "100", TaxRate: "19", TaxCategory: "S", TaxTreatment: "standard"}},
	}
	order, created, err := s.UpsertShopOrder(order)
	if err != nil || !created {
		t.Fatalf("seed order: created=%v, err=%v", created, err)
	}
	return order
}

func shopDraft(order shops.Order) model.InvoiceDraft {
	draft := model.NewDraft()
	draft.Customer, draft.Currency = order.Customer, order.Currency
	draft.Lines, draft.Adjustments = order.Lines, order.Adjustments
	if order.Paid {
		draft.PrepaidAmount = order.Gross
	}
	return draft
}

func TestShopConnectionsUniqueAndCredentialFree(t *testing.T) {
	s := openShopTestStore(t)
	order := seedShopOrder(t, s)
	connection, err := s.ShopConnection(order.ConnectionID)
	if err != nil {
		t.Fatal(err)
	}
	if connection.HasCredentials {
		t.Fatal("keychain availability persisted")
	}
	duplicate := connection
	duplicate.ID = "another-id"
	if err = s.SaveShopConnection(duplicate); err == nil {
		t.Fatal("duplicate provider and URL accepted")
	}
	connection.URL = "https://different.myshopify.com"
	if err = s.SaveShopConnection(connection); err == nil {
		t.Fatal("source identity changed while orders existed")
	}
	for _, address := range []string{"https://key:secret@shop.example", "https://shop.example?consumer_secret=secret", "http://shop.example"} {
		connection.ID, connection.URL = "unsafe", address
		if err = s.SaveShopConnection(connection); err == nil {
			t.Fatalf("unsafe connection URL persisted: %s", address)
		}
	}
}

func TestShopOrderStableIDsAndDedupe(t *testing.T) {
	s := openShopTestStore(t)
	order := seedShopOrder(t, s)
	duplicate := order
	duplicate.ID, duplicate.ImportedAt, duplicate.InvoiceID = "caller-supplied-id", "replacement-import-date", "untrusted-invoice"
	duplicate, created, err := s.UpsertShopOrder(duplicate)
	if err != nil || created || duplicate.ID != order.ID || duplicate.ImportedAt != order.ImportedAt || duplicate.InvoiceID != "" {
		t.Fatalf("order not deduplicated: %+v, %v, %v", duplicate, created, err)
	}
	if order.ID != shopStableID("shop-order", order.ConnectionID, order.ExternalID) {
		t.Fatal("unstable generated order ID")
	}
	all, err := s.ShopOrders("")
	if err != nil || len(all) != 1 {
		t.Fatalf("orders=%+v, err=%v", all, err)
	}
	other, err := s.ShopOrders("other-connection")
	if err != nil || len(other) != 0 {
		t.Fatalf("filtered orders=%+v, err=%v", other, err)
	}
}

func TestShopInvoiceCreationIsAtomicAndIdempotent(t *testing.T) {
	s := openShopTestStore(t)
	order := seedShopOrder(t, s)
	draft := shopDraft(order)
	draft.Customer.ID = "shop-stable-customer"
	draft.CustomerID = draft.Customer.ID
	invoice, err := s.CreateShopInvoice(order.ID, draft, "119.0", order.UpdatedAt)
	if err != nil {
		t.Fatal(err)
	}
	if invoice.ID == "" || invoice.CustomerID != "shop-stable-customer" || invoice.Customer.ID != invoice.CustomerID {
		t.Fatalf("invoice=%+v", invoice)
	}
	stored, err := s.ShopOrder(order.ID)
	if err != nil || stored.InvoiceID != invoice.ID {
		t.Fatalf("source order=%+v, err=%v", stored, err)
	}
	issued, err := s.Issue(invoice.ID, invoice, "119", func(model.InvoiceDraft) ([]byte, []byte, error) {
		return []byte("immutable XML"), []byte("immutable PDF"), nil
	})
	if err != nil {
		t.Fatal(err)
	}
	draft.Customer.Name = "attempted overwrite"
	repeated, err := s.CreateShopInvoice(order.ID, draft, "0", "stale")
	if err != nil || !reflect.DeepEqual(issued, repeated) {
		t.Fatalf("existing invoice overwritten: %+v, %v", repeated, err)
	}
	customers, err := s.Customers()
	if err != nil || len(customers) != 1 || customers[0].Name != order.Customer.Name {
		t.Fatalf("customers=%+v, err=%v", customers, err)
	}
	invoices, err := s.Invoices()
	if err != nil || len(invoices) != 1 {
		t.Fatalf("invoices=%+v, err=%v", invoices, err)
	}
}

func TestShopInvoiceRollbackWhenOrderLinkFails(t *testing.T) {
	s := openShopTestStore(t)
	order := seedShopOrder(t, s)
	if _, err := s.db.Exec(`CREATE TRIGGER reject_shop_invoice BEFORE UPDATE ON shop_orders WHEN NEW.invoice_id<>'' BEGIN SELECT RAISE(ABORT,'test link failure'); END;`); err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateShopInvoice(order.ID, shopDraft(order), order.Gross, order.UpdatedAt); err == nil {
		t.Fatal("expected link failure")
	}
	customers, err := s.Customers()
	if err != nil || len(customers) != 0 {
		t.Fatalf("customer leaked from failed transaction: %+v, %v", customers, err)
	}
	invoices, err := s.Invoices()
	if err != nil || len(invoices) != 0 {
		t.Fatalf("invoice leaked from failed transaction: %+v, %v", invoices, err)
	}
	stored, err := s.ShopOrder(order.ID)
	if err != nil || stored.InvoiceID != "" {
		t.Fatalf("source link changed on failure: %+v, %v", stored, err)
	}
}

func TestShopInvoiceRejectsStaleOrDifferentSnapshot(t *testing.T) {
	for _, field := range []string{"timestamp", "customer", "lines", "adjustments", "currency", "gross", "review", "existing invoice ID"} {
		t.Run(field, func(t *testing.T) {
			s := openShopTestStore(t)
			order := seedShopOrder(t, s)
			draft := shopDraft(order)
			gross, timestamp := order.Gross, order.UpdatedAt
			switch field {
			case "timestamp":
				timestamp = "old"
			case "customer":
				draft.Customer.Name = "Other customer"
			case "lines":
				draft.Lines = append([]model.InvoiceLine(nil), draft.Lines...)
				draft.Lines[0].Quantity = "2"
			case "adjustments":
				draft.Adjustments = []model.InvoiceAdjustment{{Amount: "1", Reason: "Unexpected"}}
			case "currency":
				draft.Currency = "USD"
			case "gross":
				gross = "118.99"
			case "review":
				order.ReviewReason = "Bitte prüfen"
				if _, _, err := s.UpsertShopOrder(order); err != nil {
					t.Fatal(err)
				}
			case "existing invoice ID":
				draft.ID = "existing-invoice"
			}
			if _, err := s.CreateShopInvoice(order.ID, draft, gross, timestamp); err == nil {
				t.Fatal("unsafe creation accepted")
			}
			invoices, err := s.Invoices()
			if err != nil || len(invoices) != 0 {
				t.Fatalf("unsafe draft created: %+v, %v", invoices, err)
			}
		})
	}
}

func TestShopInvoicePrepaidMatchesPaymentSnapshot(t *testing.T) {
	cases := []struct {
		name    string
		paid    bool
		prepaid string
		accept  bool
	}{
		{name: "paid full", paid: true, prepaid: "119.00", accept: true},
		{name: "paid equivalent decimal", paid: true, prepaid: "119.0", accept: true},
		{name: "paid missing", paid: true, prepaid: ""},
		{name: "paid zero", paid: true, prepaid: "0"},
		{name: "paid partial", paid: true, prepaid: "50.00"},
		{name: "paid excessive", paid: true, prepaid: "120.00"},
		{name: "paid invalid", paid: true, prepaid: "invalid"},
		{name: "unpaid empty", prepaid: "", accept: true},
		{name: "unpaid zero", prepaid: "0.00", accept: true},
		{name: "unpaid nonzero", prepaid: "119.00"},
		{name: "unpaid negative", prepaid: "-1.00"},
		{name: "unpaid invalid", prepaid: "invalid"},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			s := openShopTestStore(t)
			order := seedShopOrder(t, s)
			order.Paid = test.paid
			order, _, err := s.UpsertShopOrder(order)
			if err != nil {
				t.Fatal(err)
			}
			draft := shopDraft(order)
			draft.PrepaidAmount = test.prepaid
			_, err = s.CreateShopInvoice(order.ID, draft, order.Gross, order.UpdatedAt)
			if (err == nil) != test.accept {
				t.Fatalf("paid=%v, prepaid=%q: accept=%v, err=%v", test.paid, test.prepaid, test.accept, err)
			}
			invoices, err := s.Invoices()
			if err != nil {
				t.Fatal(err)
			}
			expectedCount := 0
			if test.accept {
				expectedCount = 1
			}
			if len(invoices) != expectedCount {
				t.Fatalf("invoice count=%d, want=%d", len(invoices), expectedCount)
			}
		})
	}
}

func TestShopConversionErrorAllowsRetryAndClearsOnSuccess(t *testing.T) {
	s := openShopTestStore(t)
	order := seedShopOrder(t, s)
	if err := s.RecordShopOrderReview(order.ID, "Bitte Unternehmenseinstellungen prüfen"); err != nil {
		t.Fatal(err)
	}
	stored, err := s.ShopOrder(order.ID)
	if err != nil || stored.ConversionError == "" || stored.ReviewReason != "" {
		t.Fatalf("local error treated as source review: %+v, %v", stored, err)
	}
	invoice, err := s.CreateShopInvoice(order.ID, shopDraft(order), order.Gross, order.UpdatedAt)
	if err != nil {
		t.Fatal(err)
	}
	stored, err = s.ShopOrder(order.ID)
	if err != nil || stored.ConversionError != "" || stored.ReviewReason != "" || stored.InvoiceID != invoice.ID {
		t.Fatalf("successful conversion did not clear error: %+v, %v", stored, err)
	}
}

func TestShopPollClearsConversionErrorsWithoutChangingSource(t *testing.T) {
	for _, stale := range []bool{false, true} {
		t.Run(map[bool]string{false: "current snapshot", true: "older snapshot"}[stale], func(t *testing.T) {
			s := openShopTestStore(t)
			order := seedShopOrder(t, s)
			if err := s.RecordShopOrderReview(order.ID, "Lokaler Konvertierungsfehler"); err != nil {
				t.Fatal(err)
			}
			incoming := order
			incoming.ConversionError = "Remote fields must never inject a local error"
			if stale {
				incoming.UpdatedAt = "2026-10-13T12:00:00Z"
				incoming.Customer.Name = "Obsolete name"
			}
			polled, added, err := s.UpsertShopOrder(incoming)
			if err != nil || added || polled.ConversionError != "" || polled.ReviewReason != "" || polled.Customer.Name != order.Customer.Name {
				t.Fatalf("poll did not reset local error safely: %+v, %v, %v", polled, added, err)
			}
			stored, err := s.ShopOrder(order.ID)
			if err != nil || stored.ConversionError != "" {
				t.Fatalf("cleared error not persisted: %+v, %v", stored, err)
			}
		})
	}
}

func TestShopLinkedConversionErrorDoesNotFlagSourceChange(t *testing.T) {
	s := openShopTestStore(t)
	order := seedShopOrder(t, s)
	invoice, err := s.CreateShopInvoice(order.ID, shopDraft(order), order.Gross)
	if err != nil {
		t.Fatal(err)
	}
	if err = s.RecordShopOrderReview(order.ID, "Local error"); err != nil {
		t.Fatal(err)
	}
	polled, _, err := s.UpsertShopOrder(order)
	if err != nil || polled.ConversionError != "" || polled.ReviewReason != "" || polled.InvoiceID != invoice.ID {
		t.Fatalf("local error flagged source change: %+v, %v", polled, err)
	}
}

func TestShopOrderChangePreservesInvoiceAndReview(t *testing.T) {
	s := openShopTestStore(t)
	order := seedShopOrder(t, s)
	invoice, err := s.CreateShopInvoice(order.ID, shopDraft(order), order.Gross)
	if err != nil {
		t.Fatal(err)
	}
	changed := order
	changed.Gross, changed.UpdatedAt = "238.00", "2026-10-15T12:00:00Z"
	changed.Lines = append([]model.InvoiceLine(nil), changed.Lines...)
	changed.Lines[0].Quantity = "2"
	changed, _, err = s.UpsertShopOrder(changed)
	if err != nil || changed.InvoiceID != invoice.ID || changed.ReviewReason != shopOrderChangedReason {
		t.Fatalf("changed source=%+v, err=%v", changed, err)
	}
	changed.ReviewReason = ""
	repeated, _, err := s.UpsertShopOrder(changed)
	if err != nil || repeated.ReviewReason != shopOrderChangedReason {
		t.Fatalf("review lost on identical sync: %+v, %v", repeated, err)
	}
	stale, _, err := s.UpsertShopOrder(order)
	if err != nil || stale.Gross != changed.Gross || stale.ReviewReason != shopOrderChangedReason {
		t.Fatalf("older poll replaced newer source: %+v, %v", stale, err)
	}
	stored, err := s.Invoice(invoice.ID)
	if err != nil || !reflect.DeepEqual(stored, invoice) {
		t.Fatalf("source modification changed invoice: %+v, %v", stored, err)
	}
}

func TestShopDeletedDraftKeepsDuplicateProtection(t *testing.T) {
	s := openShopTestStore(t)
	order := seedShopOrder(t, s)
	invoice, err := s.CreateShopInvoice(order.ID, shopDraft(order), order.Gross)
	if err != nil {
		t.Fatal(err)
	}
	if err = s.DeleteDraft(invoice.ID); err != nil {
		t.Fatal(err)
	}
	stored, err := s.ShopOrder(order.ID)
	if err != nil || stored.InvoiceID != invoice.ID || stored.ReviewReason != shopInvoiceMissingReason {
		t.Fatalf("deleted source link lost: %+v, %v", stored, err)
	}
	if _, err = s.CreateShopInvoice(order.ID, shopDraft(order), order.Gross); err == nil {
		t.Fatal("deleted draft silently recreated")
	}
	resynced, _, err := s.UpsertShopOrder(order)
	if err != nil || resynced.ReviewReason != shopInvoiceMissingReason || resynced.InvoiceID != invoice.ID {
		t.Fatalf("resync lost deletion protection: %+v, %v", resynced, err)
	}
}

func TestShopBackupRoundtripAndLegacyRestore(t *testing.T) {
	s := openShopTestStore(t)
	order := seedShopOrder(t, s)
	invoice, err := s.CreateShopInvoice(order.ID, shopDraft(order), order.Gross)
	if err != nil {
		t.Fatal(err)
	}
	backup, err := s.ExportBackup()
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(backup, `"hasCredentials": true`) || strings.Contains(backup, "consumerSecret") || strings.Contains(backup, "accessToken") {
		t.Fatal("backup contains credential data")
	}
	restored := openShopTestStore(t)
	if err = restored.RestoreBackup(backup); err != nil {
		t.Fatal(err)
	}
	connections, err := restored.ShopConnections()
	if err != nil || len(connections) != 1 || connections[0].HasCredentials {
		t.Fatalf("restored connections=%+v, %v", connections, err)
	}
	orders, err := restored.ShopOrders("")
	if err != nil || len(orders) != 1 || orders[0].InvoiceID != invoice.ID || orders[0].ImportedAt != order.ImportedAt {
		t.Fatalf("restored orders=%+v, %v", orders, err)
	}
	repeated, err := restored.CreateShopInvoice(order.ID, shopDraft(order), order.Gross)
	if err != nil || repeated.ID != invoice.ID {
		t.Fatalf("restore lost dedupe: %+v, %v", repeated, err)
	}
	legacy := `{"format":"e-rechnung-studio-backup","version":1}`
	if err = restored.RestoreBackup(legacy); err != nil {
		t.Fatal(err)
	}
	orders, err = restored.ShopOrders("")
	if err != nil || len(orders) != 0 {
		t.Fatalf("legacy restore retained orders=%+v, %v", orders, err)
	}
	connections, err = restored.ShopConnections()
	if err != nil || len(connections) != 0 {
		t.Fatalf("legacy restore retained connections=%+v, %v", connections, err)
	}
}

func TestShopInvalidBackupRollsBackAllTables(t *testing.T) {
	s := openShopTestStore(t)
	order := seedShopOrder(t, s)
	invoice, err := s.CreateShopInvoice(order.ID, shopDraft(order), order.Gross)
	if err != nil {
		t.Fatal(err)
	}
	backup, err := s.ExportBackup()
	if err != nil {
		t.Fatal(err)
	}
	var doc backupDocument
	if err = json.Unmarshal([]byte(backup), &doc); err != nil {
		t.Fatal(err)
	}
	doc.ShopOrders[0].ConnectionID = "missing-connection"
	invalid, err := json.Marshal(doc)
	if err != nil {
		t.Fatal(err)
	}
	if err = s.RestoreBackup(string(invalid)); err == nil {
		t.Fatal("orphan source accepted")
	}
	stored, err := s.Invoice(invoice.ID)
	if err != nil || !reflect.DeepEqual(stored, invoice) {
		t.Fatalf("invoice lost after rollback: %+v, %v", stored, err)
	}
	storedOrder, err := s.ShopOrder(order.ID)
	if err != nil || storedOrder.InvoiceID != invoice.ID {
		t.Fatalf("order lost after rollback: %+v, %v", storedOrder, err)
	}
	customers, err := s.Customers()
	if err != nil || len(customers) != 1 {
		t.Fatalf("customers lost after rollback: %+v, %v", customers, err)
	}
}
