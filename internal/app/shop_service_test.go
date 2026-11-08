package app

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"papierleicht.de/e-rechnung-studio/internal/invoicexml"
	"papierleicht.de/e-rechnung-studio/internal/model"
	"papierleicht.de/e-rechnung-studio/internal/profile"
	"papierleicht.de/e-rechnung-studio/internal/shops"
)

type memoryShopVault struct {
	values              map[string]string
	failSet, failDelete bool
}

func (v *memoryShopVault) Get(id string) (string, error) {
	if value, ok := v.values[id]; ok {
		return value, nil
	}
	return "", errors.New("not found")
}
func (v *memoryShopVault) Set(id, value string) error {
	if v.failSet {
		return errors.New("vault unavailable")
	}
	v.values[id] = value
	return nil
}
func (v *memoryShopVault) Delete(id string) error {
	if v.failDelete {
		return errors.New("vault unavailable")
	}
	delete(v.values, id)
	return nil
}

type fakeShopFetcher struct {
	orders []shops.Order
	err    error
	since  time.Time
	calls  int
	block  chan struct{}
	once   sync.Once
}

func (f *fakeShopFetcher) FetchOrders(ctx context.Context, c shops.Connection, credentials shops.Credentials, since time.Time) ([]shops.Order, error) {
	f.since, f.calls = since, f.calls+1
	if f.block != nil {
		f.once.Do(func() { close(f.block) })
		<-ctx.Done()
		return nil, ctx.Err()
	}
	return f.orders, f.err
}

func shopServiceFixture(t *testing.T) (*ShopService, *fakeShopFetcher, *memoryShopVault, string) {
	t.Helper()
	dir := t.TempDir()
	invoices, err := NewInvoiceService(dir)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := invoices.SaveCompany(testCompany()); err != nil {
		t.Fatal(err)
	}
	s := NewShopService(invoices)
	f := &fakeShopFetcher{}
	v := &memoryShopVault{values: map[string]string{}}
	s.fetcher, s.vault = f, v
	t.Cleanup(func() { s.Close(); invoices.Close() })
	return s, f, v, dir
}

func shopConnectionFixture() shops.Connection {
	return shops.Connection{Provider: "woocommerce", Label: "Mein Shop", URL: "https://shop.example", ImportFrom: "2026-01-01", AutoCreateDrafts: true}
}

func shopCredentialsFixture() shops.Credentials {
	return shops.Credentials{ConsumerKey: "ck-private", ConsumerSecret: "cs-private"}
}

func shopOrderFixture() shops.Order {
	d := testDraft()
	return shops.Order{ExternalID: "42", Number: "BEST-42", CustomerExternalID: "123", Customer: d.Customer, Currency: "EUR", Gross: "238.00", CreatedAt: "2026-09-09T10:00:00Z", UpdatedAt: "2026-09-09T10:00:00Z", Status: "processing", Lines: d.Lines, Adjustments: []model.InvoiceAdjustment{}}
}

func TestShopConnectSyncInvoiceAndPrepayment(t *testing.T) {
	s, f, _, _ := shopServiceFixture(t)
	o := shopOrderFixture()
	o.Paid = true
	f.orders = []shops.Order{o}
	c, err := s.Connect(shopConnectionFixture(), shopCredentialsFixture())
	if err != nil {
		t.Fatal(err)
	}
	if !c.Enabled || c.LastSync == "" || !c.HasCredentials {
		t.Fatalf("connection: %+v", c)
	}
	overview, err := s.Overview()
	if err != nil || len(overview.Orders) != 1 {
		t.Fatalf("overview: %+v %v", overview, err)
	}
	order := overview.Orders[0]
	if order.InvoiceID == "" || order.ReviewReason != "" {
		t.Fatalf("order: %+v", order)
	}
	draft, err := s.CreateInvoice(order.ID)
	if err != nil || draft.PrepaidAmount != "238.00" || draft.Status != "draft" || draft.Number != "" || !draft.AutomaticNumber || draft.DeliveryDate != "" || draft.TaxSource != "manual" {
		t.Fatalf("draft: %+v %v", draft, err)
	}
	// The importer leaves fulfillment facts to the merchant; the order creation
	// timestamp is never silently presented as the service date.
	draft.DeliveryDate = "2026-09-09"
	view, err := s.invoices.ValidateInvoice(draft)
	if err != nil || view.Totals.Due != "0.00" || view.Validation.State != "valid" {
		t.Fatalf("prepaid: %+v %v", view, err)
	}
	parsed, err := invoicexml.Parse([]byte(view.XML))
	if err != nil || parsed.TotalPrepaid.StringFixed(2) != "238.00" || !parsed.DuePayableAmount.IsZero() {
		t.Fatalf("prepaid XML: %+v %v", parsed, err)
	}
	if !strings.Contains(view.PreviewHTML, "Bereits bezahlt") || strings.Contains(view.PreviewHTML, "SEPA-Überweisung per QR-Code") {
		t.Fatal("prepaid preview payment mismatch")
	}
	result, err := s.Sync(c.ID)
	if err != nil || result.Imported != 0 || result.Drafts != 0 {
		t.Fatalf("duplicate sync: %+v %v", result, err)
	}
	checkpoint, _ := time.Parse(time.RFC3339, c.LastSync)
	if !f.since.Equal(checkpoint.Add(-5 * time.Minute)) {
		t.Fatalf("overlap missing: %v", f.since)
	}
	list, _ := s.invoices.ApplicationLists()
	if len(list.Invoices) != 1 || len(list.Customers) != 1 {
		t.Fatalf("duplicates: %+v", list)
	}
	backup, err := s.invoices.ExportBackup()
	if err != nil || strings.Contains(backup, "ck-private") || strings.Contains(backup, "cs-private") {
		t.Fatal("secret in backup", err)
	}
	issued, err := s.invoices.IssueInvoice(draft)
	if err != nil || issued.Draft.Status != "paid" || issued.Draft.PaidAt == "" || issued.Totals.Due != "0.00" {
		t.Fatalf("issue: %+v %v", issued, err)
	}
	correction, err := s.invoices.CreateCorrectionDraft(issued.Draft.ID)
	if err != nil || correction.Draft.PrepaidAmount != "" || correction.Totals.Due != "238.00" {
		t.Fatalf("correction prepayment carried: %+v %v", correction, err)
	}
}

func TestShopReviewAndManualConversion(t *testing.T) {
	s, f, _, _ := shopServiceFixture(t)
	cfg := shopConnectionFixture()
	cfg.AutoCreateDrafts = false
	f.orders = []shops.Order{shopOrderFixture()}
	c, err := s.Connect(cfg, shopCredentialsFixture())
	if err != nil {
		t.Fatal(err)
	}
	orders, _ := s.invoices.store.ShopOrders(c.ID)
	if orders[0].InvoiceID != "" {
		t.Fatal("draft created despite disabled option")
	}
	d, err := s.CreateInvoice(orders[0].ID)
	if err != nil || d.ID == "" {
		t.Fatal("manual conversion failed", err)
	}
	updated := shopOrderFixture()
	updated.Gross, updated.UpdatedAt = "250.00", "2026-09-10T10:00:00Z"
	f.orders = []shops.Order{updated}
	if _, err := s.Sync(c.ID); err != nil {
		t.Fatal(err)
	}
	orders, _ = s.invoices.store.ShopOrders(c.ID)
	if orders[0].ReviewReason == "" || orders[0].InvoiceID != d.ID {
		t.Fatal("updated source did not keep link and review")
	}
	persisted, _ := s.invoices.store.Invoice(d.ID)
	if persisted.Lines[0].UnitPrice != "100.00" {
		t.Fatal("existing draft overwritten")
	}
}

func TestShopAmountMismatchRemainsReviewable(t *testing.T) {
	s, f, _, _ := shopServiceFixture(t)
	o := shopOrderFixture()
	o.Gross = "239.00"
	f.orders = []shops.Order{o}
	c, err := s.Connect(shopConnectionFixture(), shopCredentialsFixture())
	if err != nil {
		t.Fatal(err)
	}
	orders, _ := s.invoices.store.ShopOrders(c.ID)
	if len(orders) != 1 || orders[0].ConversionError == "" || orders[0].InvoiceID != "" {
		t.Fatalf("mismatch accepted: %+v", orders)
	}
	f.orders = []shops.Order{shopOrderFixture()}
	result, err := s.Sync(c.ID)
	if err != nil || result.Drafts != 1 {
		t.Fatalf("corrected order not retried: %+v %v", result, err)
	}
}

func TestShopFailedFetchAndVaultLeaveConnectionUnchanged(t *testing.T) {
	s, f, v, _ := shopServiceFixture(t)
	f.err = errors.New("remote body includes cs-private")
	if _, err := s.Connect(shopConnectionFixture(), shopCredentialsFixture()); err == nil || strings.Contains(err.Error(), "cs-private") {
		t.Fatal("unsafe failure", err)
	}
	connections, _ := s.invoices.store.ShopConnections()
	if len(connections) != 0 || len(v.values) != 0 {
		t.Fatal("failed connection persisted")
	}
	f.err = nil
	v.failSet = true
	if _, err := s.Connect(shopConnectionFixture(), shopCredentialsFixture()); err == nil {
		t.Fatal("vault failure ignored")
	}
	connections, _ = s.invoices.store.ShopConnections()
	if len(connections) != 0 {
		t.Fatal("connection saved without credentials")
	}
	v.failSet = false
	c, err := s.Connect(shopConnectionFixture(), shopCredentialsFixture())
	if err != nil {
		t.Fatal(err)
	}
	f.err = errors.New("credential cs-private")
	if _, err := s.Sync(c.ID); err == nil || strings.Contains(err.Error(), "cs-private") {
		t.Fatal("unsafe sync failure")
	}
	after, _ := s.invoices.store.ShopConnection(c.ID)
	if after.LastSync != c.LastSync || after.LastError == "" {
		t.Fatal("failed fetch advanced checkpoint")
	}
}

func TestShopRestoreCannotRebindKeychainOrigin(t *testing.T) {
	s, _, _, _ := shopServiceFixture(t)
	c, err := s.Connect(shopConnectionFixture(), shopCredentialsFixture())
	if err != nil {
		t.Fatal(err)
	}
	backup, _ := s.invoices.ExportBackup()
	changed := strings.ReplaceAll(backup, "https://shop.example", "https://other.example")
	if err := s.invoices.RestoreBackup(changed); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Sync(c.ID); err == nil {
		t.Fatal("restored URL reused keychain secret")
	}
	overview, _ := s.Overview()
	if overview.Connections[0].HasCredentials {
		t.Fatal("credential availability trusted after restore")
	}
}

func TestShopStorageFailureDoesNotCheckpointAndRetries(t *testing.T) {
	s, f, _, dir := shopServiceFixture(t)
	c, err := s.Connect(shopConnectionFixture(), shopCredentialsFixture())
	if err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("sqlite", filepath.Join(dir, "e-rechnung-studio.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec(`CREATE TRIGGER reject_shop_invoice BEFORE INSERT ON invoices BEGIN SELECT RAISE(ABORT,'test failure'); END;`); err != nil {
		t.Fatal(err)
	}
	f.orders = []shops.Order{shopOrderFixture()}
	if _, err := s.Sync(c.ID); err == nil {
		t.Fatal("storage failure ignored")
	}
	after, _ := s.invoices.store.ShopConnection(c.ID)
	if after.LastSync != c.LastSync {
		t.Fatal("storage failure advanced checkpoint")
	}
	orders, _ := s.invoices.store.ShopOrders(c.ID)
	if orders[0].InvoiceID != "" || orders[0].ReviewReason != "" {
		t.Fatal("storage failure created permanent review")
	}
	if _, err := db.Exec(`DROP TRIGGER reject_shop_invoice`); err != nil {
		t.Fatal(err)
	}
	result, err := s.Sync(c.ID)
	if err != nil || result.Drafts != 1 {
		t.Fatalf("retry: %+v %v", result, err)
	}
}

func TestShopLocalConversionFailureCanBeRetried(t *testing.T) {
	s, f, _, _ := shopServiceFixture(t)
	company := testCompany()
	company.SmallBusiness = true
	if _, err := s.invoices.SaveCompany(company); err != nil {
		t.Fatal(err)
	}
	f.orders = []shops.Order{shopOrderFixture()}
	c, err := s.Connect(shopConnectionFixture(), shopCredentialsFixture())
	if err != nil {
		t.Fatal(err)
	}
	orders, _ := s.invoices.store.ShopOrders(c.ID)
	if orders[0].ConversionError == "" || orders[0].ReviewReason != "" {
		t.Fatalf("conversion review lost: %+v", orders[0])
	}
	if _, err := s.invoices.SaveCompany(testCompany()); err != nil {
		t.Fatal(err)
	}
	d, err := s.CreateInvoice(orders[0].ID)
	if err != nil || d.ID == "" {
		t.Fatalf("local fix could not retry: %v", err)
	}
	order, _ := s.invoices.store.ShopOrder(orders[0].ID)
	if order.ConversionError != "" {
		t.Fatal("conversion error retained after success")
	}
}

func TestShopForeignCurrencyIsPreservedWithoutEuroInvoice(t *testing.T) {
	s, f, _, _ := shopServiceFixture(t)
	o := shopOrderFixture()
	o.Currency = "USD"
	f.orders = []shops.Order{o}
	c, err := s.Connect(shopConnectionFixture(), shopCredentialsFixture())
	if err != nil {
		t.Fatal(err)
	}
	orders, _ := s.invoices.store.ShopOrders(c.ID)
	if orders[0].Currency != "USD" || orders[0].ConversionError == "" || orders[0].InvoiceID != "" {
		t.Fatalf("foreign currency mislabeled: %+v", orders[0])
	}
}

func TestPrepaidAmountsInSupportedProfiles(t *testing.T) {
	s, _, _, _ := shopServiceFixture(t)
	company := testCompany()
	company.Phone, company.Endpoint, company.EndpointScheme = "+49 30 1234567", company.Email, "EM"
	if _, err := s.invoices.SaveCompany(company); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"facturx-basic", "facturx-en16931", "facturx-extended", "xrechnung-cii", "xrechnung-ubl"} {
		t.Run(id, func(t *testing.T) {
			format, _ := profile.Get(id)
			d := testDraft()
			d.Profile, d.Syntax, d.PrepaidAmount = id, format.Syntax, "238.00"
			d.Customer.Endpoint, d.Customer.EndpointScheme, d.BuyerReference = d.Customer.Email, "EM", "04011000-12345-03"
			view, err := s.invoices.ValidateInvoice(d)
			if err != nil || view.Validation.State != "valid" || view.Totals.Due != "0.00" {
				t.Fatalf("validation=%+v err=%v", view.Validation, err)
			}
			inv, err := invoicexml.Parse([]byte(view.XML))
			if err != nil || inv.TotalPrepaid.StringFixed(2) != "238.00" || !inv.DuePayableAmount.IsZero() {
				t.Fatal("prepayment XML mismatch", err)
			}
		})
	}
	for _, invalid := range []string{"-1", "239.00", "1.001", "invalid"} {
		d := testDraft()
		d.PrepaidAmount = invalid
		if _, err := s.invoices.engine.Build(d, company); err == nil {
			t.Fatalf("invalid prepaid amount %q accepted", invalid)
		}
	}
}

func TestShopTaxAmountsInSupportedProfiles(t *testing.T) {
	s, _, _, _ := shopServiceFixture(t)
	company := testCompany()
	company.Phone, company.Endpoint, company.EndpointScheme = "+49 30 1234567", company.Email, "EM"
	if _, err := s.invoices.SaveCompany(company); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"facturx-basic", "facturx-en16931", "facturx-extended", "xrechnung-cii", "xrechnung-ubl"} {
		t.Run(id, func(t *testing.T) {
			format, _ := profile.Get(id)
			d := testDraft()
			d.Profile, d.Syntax = id, format.Syntax
			d.Customer.Endpoint, d.Customer.EndpointScheme, d.BuyerReference = d.Customer.Email, "EM", "04011000-12345-03"
			// A cent of line rounding on 200.00 at 19 %, as a shop would send it.
			d.SourceTaxes = []model.SourceTax{{Rate: "19", Basis: "200.00", Amount: "38.01"}}
			view, err := s.invoices.ValidateInvoice(d)
			if err != nil || view.Validation.State != "valid" || view.Totals.Tax != "38.01" || view.Totals.Gross != "238.01" {
				t.Fatalf("validation=%+v totals=%+v err=%v", view.Validation, view.Totals, err)
			}
			inv, err := invoicexml.Parse([]byte(view.XML))
			if err != nil || len(inv.TradeTaxes) != 1 || inv.TradeTaxes[0].CalculatedAmount.StringFixed(2) != "38.01" || inv.GrandTotal.StringFixed(2) != "238.01" {
				t.Fatal("tax amount XML mismatch", err)
			}
		})
	}
}

func TestShopReconnectDisconnectAndPause(t *testing.T) {
	s, f, v, _ := shopServiceFixture(t)
	f.orders = []shops.Order{shopOrderFixture()}
	c, err := s.Connect(shopConnectionFixture(), shopCredentialsFixture())
	if err != nil {
		t.Fatal(err)
	}
	reconnected, err := s.Connect(shopConnectionFixture(), shopCredentialsFixture())
	if err != nil || reconnected.ID != c.ID {
		t.Fatal("reconnect duplicated shop", err)
	}
	if err := s.SetEnabled(c.ID, false); err != nil {
		t.Fatal(err)
	}
	before := f.calls
	if _, err := s.Sync(c.ID); err == nil || f.calls != before {
		t.Fatal("paused connection fetched")
	}
	v.failDelete = true
	if err := s.Disconnect(c.ID); err == nil {
		t.Fatal("keychain deletion failure ignored")
	}
	persisted, _ := s.invoices.store.ShopConnection(c.ID)
	if persisted.Enabled {
		t.Fatal("failed disconnect left shop active")
	}
	v.failDelete = false
	if err := s.Disconnect(c.ID); err != nil {
		t.Fatal(err)
	}
	if len(v.values) != 0 {
		t.Fatal("secret retained after disconnect")
	}
	if err := s.SetEnabled(c.ID, true); err == nil {
		t.Fatal("disconnected shop enabled without credentials")
	}
	orders, _ := s.invoices.store.ShopOrders(c.ID)
	if len(orders) != 1 || orders[0].InvoiceID == "" {
		t.Fatal("disconnect erased import history")
	}
}

func TestShopCloseCancelsAndJoinsInFlightSync(t *testing.T) {
	s, f, _, _ := shopServiceFixture(t)
	c, err := s.Connect(shopConnectionFixture(), shopCredentialsFixture())
	if err != nil {
		t.Fatal(err)
	}
	f.block = make(chan struct{})
	done := make(chan struct{})
	go func() { defer close(done); _, _ = s.Sync(c.ID) }()
	<-f.block
	if _, err := s.Sync(c.ID); err == nil {
		t.Fatal("concurrent sync allowed")
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("in-flight sync not joined")
	}
	if err := s.SetEnabled(c.ID, true); err == nil {
		t.Fatal("closed service accepted mutation")
	}
}

func TestShopMetadataContainsNoSecrets(t *testing.T) {
	s, _, _, _ := shopServiceFixture(t)
	if _, err := s.Connect(shopConnectionFixture(), shopCredentialsFixture()); err != nil {
		t.Fatal(err)
	}
	o, err := s.Overview()
	raw, _ := json.Marshal(o)
	if err != nil || strings.Contains(string(raw), "cs-private") || strings.Contains(string(raw), "ck-private") {
		t.Fatal("overview exposed secrets")
	}
}

func TestShopWaitsWhileTheAuditorViewIsOpen(t *testing.T) {
	s, f, _, _ := shopServiceFixture(t)
	f.orders = []shops.Order{shopOrderFixture()}
	c, err := s.Connect(shopConnectionFixture(), shopCredentialsFixture())
	if err != nil {
		t.Fatal(err)
	}
	if err = s.invoices.store.OpenAuditView("2026-01-01", "2026-12-31"); err != nil {
		t.Fatal(err)
	}
	calls := f.calls
	if _, err = s.Sync(c.ID); err == nil || !strings.Contains(err.Error(), "Prüferansicht") || f.calls != calls {
		t.Fatalf("sync during the auditor view: %v, calls %d", err, f.calls-calls)
	}
	if err = s.invoices.store.CloseAuditView(); err != nil {
		t.Fatal(err)
	}
	if _, err = s.Sync(c.ID); err != nil {
		t.Fatal(err)
	}
}

func TestShopOrdersFollowTheSmallBusinessRule(t *testing.T) {
	untaxed := shopOrderFixture()
	untaxed.Gross = "200.00"
	untaxed.Lines = append([]model.InvoiceLine(nil), untaxed.Lines...)
	untaxed.Lines[0].TaxRate, untaxed.Lines[0].TaxCategory = "0", "Z"
	t.Run("small business, untaxed order", func(t *testing.T) {
		s, f, _, _ := shopServiceFixture(t)
		company := testCompany()
		company.SmallBusiness = true
		if _, err := s.invoices.SaveCompany(company); err != nil {
			t.Fatal(err)
		}
		f.orders = []shops.Order{untaxed}
		if _, err := s.Connect(shopConnectionFixture(), shopCredentialsFixture()); err != nil {
			t.Fatal(err)
		}
		overview, err := s.Overview()
		if err != nil || len(overview.Orders) != 1 || overview.Orders[0].InvoiceID == "" || overview.Orders[0].ConversionError != "" {
			t.Fatalf("orders: %+v, %v", overview.Orders, err)
		}
		draft, err := s.invoices.store.Invoice(overview.Orders[0].InvoiceID)
		if err != nil || draft.TaxTreatment != "small-business" {
			t.Fatalf("draft: %+v, %v", draft, err)
		}
		draft.DeliveryDate = "2026-09-09"
		view, err := s.invoices.ValidateInvoice(draft)
		if err != nil || view.Validation.State != "valid" || view.Totals.Tax != "0.00" || view.Totals.Gross != "200.00" {
			t.Fatalf("view: %+v %+v, %v", view.Validation, view.Totals, err)
		}
		parsed, err := invoicexml.Parse([]byte(view.XML))
		if err != nil || len(parsed.TradeTaxes) != 1 || parsed.TradeTaxes[0].CategoryCode != "E" {
			t.Fatalf("tax groups: %+v, %v", parsed.TradeTaxes, err)
		}
	})
	t.Run("small business, order with VAT", func(t *testing.T) {
		s, f, _, _ := shopServiceFixture(t)
		company := testCompany()
		company.SmallBusiness = true
		if _, err := s.invoices.SaveCompany(company); err != nil {
			t.Fatal(err)
		}
		f.orders = []shops.Order{shopOrderFixture()}
		if _, err := s.Connect(shopConnectionFixture(), shopCredentialsFixture()); err != nil {
			t.Fatal(err)
		}
		overview, _ := s.Overview()
		if len(overview.Orders) != 1 || overview.Orders[0].InvoiceID != "" || !strings.Contains(overview.Orders[0].ConversionError, "Kleinunternehmer") {
			t.Fatalf("orders: %+v", overview.Orders)
		}
	})
	t.Run("standard taxation, untaxed order", func(t *testing.T) {
		s, f, _, _ := shopServiceFixture(t)
		f.orders = []shops.Order{untaxed}
		if _, err := s.Connect(shopConnectionFixture(), shopCredentialsFixture()); err != nil {
			t.Fatal(err)
		}
		overview, _ := s.Overview()
		if len(overview.Orders) != 1 || overview.Orders[0].InvoiceID != "" || !strings.Contains(overview.Orders[0].ConversionError, "ohne Umsatzsteuer") {
			t.Fatalf("orders: %+v", overview.Orders)
		}
	})
}

func TestShopInvoiceKeepsTheShopsLineRounding(t *testing.T) {
	s, f, _, _ := shopServiceFixture(t)
	o := shopOrderFixture()
	o.Gross = "19.98"
	o.Lines = []model.InvoiceLine{
		{ID: "a", Name: "Notizbuch", Quantity: "1", Unit: "C62", UnitPrice: "8.39", TaxRate: "19", TaxCategory: "S", TaxTreatment: "standard"},
		{ID: "b", Name: "Stiftset", Quantity: "1", Unit: "C62", UnitPrice: "8.39", TaxRate: "19", TaxCategory: "S", TaxTreatment: "standard"},
	}
	o.SourceTaxes = []model.SourceTax{{Rate: "19", Basis: "16.78", Amount: "3.20"}}
	o.Paid = true
	f.orders = []shops.Order{o}
	if _, err := s.Connect(shopConnectionFixture(), shopCredentialsFixture()); err != nil {
		t.Fatal(err)
	}
	overview, _ := s.Overview()
	if len(overview.Orders) != 1 || overview.Orders[0].InvoiceID == "" {
		t.Fatalf("orders: %+v", overview.Orders)
	}
	draft, err := s.invoices.store.Invoice(overview.Orders[0].InvoiceID)
	if err != nil || len(draft.SourceTaxes) != 1 {
		t.Fatalf("draft: %+v, %v", draft, err)
	}
	draft.DeliveryDate = "2026-09-09"
	issued, err := s.invoices.IssueInvoice(draft)
	if err != nil || issued.Validation.State != "valid" || issued.Totals.Tax != "3.20" || issued.Totals.Gross != "19.98" || issued.Totals.Due != "0.00" || issued.Draft.Status != "paid" {
		t.Fatalf("issued: %+v %+v %s, %v", issued.Validation, issued.Totals, issued.Draft.Status, err)
	}
	// A full credit note reverses exactly the VAT of the invoice.
	correction, err := s.invoices.CreateCorrectionDraft(issued.Draft.ID)
	if err != nil || correction.Totals.Tax != "3.20" {
		t.Fatalf("correction: %+v, %v", correction.Totals, err)
	}
}
