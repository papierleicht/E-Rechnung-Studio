package app

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"papierleicht.de/e-rechnung-studio/internal/invoicexml"
	"papierleicht.de/e-rechnung-studio/internal/model"
	"papierleicht.de/e-rechnung-studio/internal/shops"
	"papierleicht.de/e-rechnung-studio/internal/store"
)

// shopSimulation is one emulated shop: a local HTTPS server that answers the
// provider's read API like the real system, usually with one fully paid order
// and one open order, each from another customer. The simulation drives the
// real client, shop service, invoice engine, payment ledger and backup, so
// only the remote shop is simulated.
type shopSimulation struct {
	connection  shops.Connection
	credentials shops.Credentials
	handler     http.Handler
	orders      []simulatedOrder
	// company replaces the regularly taxed test company, e.g. for § 19 UStG.
	company *model.Company
}

type simulatedOrder struct {
	number, gross string
	paid          bool
	// paidOn is the payment day the shop reports, empty if it reports none.
	paidOn string
	// tax is the VAT the invoice has to show, if the test checks it.
	tax string
}

// shopSimulations is filled by the shop_simulation_<provider>_test.go files.
var shopSimulations = map[string]func(t *testing.T) shopSimulation{}

// rewriteToServer sends every request to the emulator, whatever host the
// connection names, and trusts the emulator's test certificate.
type rewriteToServer struct {
	base   http.RoundTripper
	target *url.URL
}

func (r rewriteToServer) RoundTrip(req *http.Request) (*http.Response, error) {
	copy := req.Clone(req.Context())
	u := *req.URL
	u.Scheme, u.Host = r.target.Scheme, r.target.Host
	copy.URL = &u
	return r.base.RoundTrip(copy)
}

func TestShopSimulations(t *testing.T) {
	for _, provider := range []string{"shopify", "woocommerce", "shopware", "gambio", "wix", "prestashop"} {
		t.Run(provider, func(t *testing.T) {
			simulate, ok := shopSimulations[provider]
			if !ok {
				t.Fatal("no simulation")
			}
			sim := simulate(t)
			if sim.connection.Provider != provider {
				t.Fatalf("simulation for %s connects %s", provider, sim.connection.Provider)
			}
			runShopSimulation(t, sim)
		})
	}
}

func runShopSimulation(t *testing.T, sim shopSimulation) {
	server := httptest.NewTLSServer(sim.handler)
	defer server.Close()
	target, _ := url.Parse(server.URL)
	// The shop client uses the default transport; tests in this package do
	// not run in parallel.
	previous := http.DefaultTransport
	http.DefaultTransport = rewriteToServer{base: server.Client().Transport, target: target}
	defer func() { http.DefaultTransport = previous }()

	// SHOP_SIMULATION_DIR keeps the databases, e.g. to look at them in the app.
	dir := t.TempDir()
	if keep := os.Getenv("SHOP_SIMULATION_DIR"); keep != "" {
		dir = filepath.Join(keep, sim.connection.Provider)
		if err := os.RemoveAll(dir); err != nil {
			t.Fatal(err)
		}
	}
	invoices, err := NewInvoiceService(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer invoices.Close()
	company := testCompany()
	if sim.company != nil {
		company = *sim.company
	}
	if _, err = invoices.SaveCompany(company); err != nil {
		t.Fatal(err)
	}
	s := NewShopService(invoices)
	defer s.Close()
	s.vault = &memoryShopVault{values: map[string]string{}}

	connection := sim.connection
	connection.AutoCreateDrafts = true
	connection.ImportFrom = "2026-10-01"
	connected, err := s.Connect(connection, sim.credentials)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	if !connected.Enabled || connected.LastSync == "" || connected.LastError != "" {
		t.Fatalf("connection: %+v", connected)
	}
	overview, err := s.Overview()
	if err != nil || len(overview.Orders) != len(sim.orders) {
		t.Fatalf("imported orders: %+v, %v", overview.Orders, err)
	}
	for _, expected := range sim.orders {
		var order *shops.Order
		for i := range overview.Orders {
			if overview.Orders[i].Number == expected.number {
				order = &overview.Orders[i]
			}
		}
		if order == nil {
			t.Fatalf("order %s missing: %+v", expected.number, overview.Orders)
		}
		paid := expected.paid
		if order.Paid != paid || order.Gross != expected.gross || order.ReviewReason != "" || order.ConversionError != "" || order.InvoiceID == "" {
			t.Fatalf("order %s: %+v", expected.number, *order)
		}
		draft, err := invoices.store.Invoice(order.InvoiceID)
		if err != nil || draft.Status != "draft" || draft.Number != "" || draft.DeliveryDate != "" {
			t.Fatalf("draft of %s: %+v, %v", expected.number, draft, err)
		}
		if paid != (draft.PrepaidAmount == expected.gross) {
			t.Fatalf("draft of %s prepaid %q", expected.number, draft.PrepaidAmount)
		}
		// The merchant states the date of supply; the order date is no proof of it.
		draft.DeliveryDate = order.CreatedAt[:10]
		issued, err := invoices.IssueInvoice(draft)
		if err != nil {
			t.Fatalf("issue %s: %v", expected.number, err)
		}
		if issued.Validation.State != "valid" || issued.Totals.Gross != expected.gross {
			t.Fatalf("issued %s: %+v %+v", expected.number, issued.Validation, issued.Totals)
		}
		parsed, err := invoicexml.Parse([]byte(issued.XML))
		if err != nil || parsed.GrandTotal.StringFixed(2) != expected.gross {
			t.Fatalf("XML of %s: %v", expected.number, err)
		}
		if expected.tax != "" && issued.Totals.Tax != expected.tax {
			t.Fatalf("VAT of %s: %s, want %s", expected.number, issued.Totals.Tax, expected.tax)
		}
		for _, group := range parsed.TradeTaxes {
			if company.SmallBusiness != (group.CategoryCode == "E") {
				t.Fatalf("VAT group of %s: %+v", expected.number, group)
			}
		}
		balance, payments := shopSimulationLedger(t, invoices, issued.Draft.ID)
		if paid {
			if issued.Draft.Status != "paid" || issued.Totals.Due != "0.00" || parsed.TotalPrepaid.StringFixed(2) != expected.gross || !parsed.DuePayableAmount.IsZero() {
				t.Fatalf("paid order %s: status %s, due %s", expected.number, issued.Draft.Status, issued.Totals.Due)
			}
			day := expected.paidOn
			if day == "" {
				day = issued.Draft.IssueDate
			}
			if balance != "bezahlt" || len(payments) != 1 || payments[0].Source != "shop" || payments[0].Amount != expected.gross || payments[0].Date != day || !strings.Contains(payments[0].Reason, expected.number) {
				t.Fatalf("ledger of %s: %s %+v", expected.number, balance, payments)
			}
		} else {
			if issued.Draft.Status != "issued" || issued.Totals.Due != expected.gross || !parsed.TotalPrepaid.IsZero() {
				t.Fatalf("open order %s: status %s, due %s", expected.number, issued.Draft.Status, issued.Totals.Due)
			}
			if (balance != "offen" && balance != "überfällig") || len(payments) != 0 {
				t.Fatalf("ledger of %s: %s %+v", expected.number, balance, payments)
			}
		}
	}

	// A second poll returns the same orders and must not create anything.
	result, err := s.Sync(connected.ID)
	if err != nil || result.Imported != 0 || result.Drafts != 0 || result.Review != 0 {
		t.Fatalf("second sync: %+v, %v", result, err)
	}
	lists, err := invoices.ApplicationLists()
	if err != nil || len(lists.Invoices) != len(sim.orders) || len(lists.Customers) != len(sim.orders) {
		t.Fatalf("after second sync: %d invoices, %d customers, %v", len(lists.Invoices), len(lists.Customers), err)
	}
	backup, err := invoices.ExportBackup()
	if err != nil || !strings.Contains(backup, `"shopOrders"`) {
		t.Fatalf("backup: %v", err)
	}
	for _, secret := range []string{sim.credentials.AccessToken, sim.credentials.ClientSecret, sim.credentials.ConsumerSecret, sim.credentials.APIKey, sim.credentials.Password} {
		if secret != "" && strings.Contains(backup, secret) {
			t.Fatal("backup contains a shop credential")
		}
	}
}

func shopSimulationLedger(t *testing.T, invoices *InvoiceService, id string) (string, []store.Payment) {
	t.Helper()
	overview, err := invoices.store.PaymentOverview()
	if err != nil {
		t.Fatal(err)
	}
	state := ""
	for _, b := range overview.Invoices {
		if b.ID == id && b.Direction == "outgoing" {
			state = b.State
		}
	}
	payments := []store.Payment{}
	for _, p := range overview.Payments {
		if p.InvoiceID == id && p.RevokedAt == "" {
			payments = append(payments, p)
		}
	}
	return state, payments
}

// TestShopSimulationSmallBusiness runs a shop that charges no VAT for a
// company under § 19 UStG.
func TestShopSimulationSmallBusiness(t *testing.T) {
	runShopSimulation(t, simulateSmallBusinessWooCommerce(t))
}
