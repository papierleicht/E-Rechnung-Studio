package app

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPaymentQROptOut(t *testing.T) {
	s, err := NewInvoiceService(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if _, err = s.SaveCompany(testCompany()); err != nil {
		t.Fatal(err)
	}
	for _, disabled := range []bool{false, true} {
		d := testDraft()
		if disabled {
			d.Number = "QR-OFF-2026-001"
		}
		d.DisablePaymentQR = disabled
		issued, err := s.IssueInvoice(d)
		if err != nil {
			t.Fatal(err)
		}
		stored, err := s.store.Invoice(issued.Draft.ID)
		if err != nil {
			t.Fatal(err)
		}
		if stored.DisablePaymentQR != disabled {
			t.Fatal("opt-out not persisted")
		}
		raw, err := s.invoicePDF(stored.ID)
		if err != nil {
			t.Fatal(err)
		}
		if bytes.Contains(raw, []byte("/Subtype /Image")) == disabled {
			t.Fatalf("issued image presence, disabled=%v", disabled)
		}
	}
}

func TestPaymentQRDisplaySettings(t *testing.T) {
	s, err := NewInvoiceService(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if _, err = s.SaveCompany(testCompany()); err != nil {
		t.Fatal(err)
	}
	customer, err := s.SaveCustomer(testDraft().Customer)
	if err != nil {
		t.Fatal(err)
	}
	dir := os.Getenv("E_RECHNUNG_STUDIO_QR_TEST_OUTPUT")
	if dir != "" {
		if err = os.MkdirAll(dir, 0700); err != nil {
			t.Fatal(err)
		}
		if err = os.WriteFile(filepath.Join(dir, "fonts.css"), []byte(s.InvoicePreviewFonts()), 0600); err != nil {
			t.Fatal(err)
		}
	}
	for _, mode := range []struct {
		name                  string
		disabled, hideDetails bool
	}{{"info", false, false}, {"code", false, true}, {"off-info", true, false}, {"off-code", true, true}} {
		t.Run(mode.name, func(t *testing.T) {
			d := testDraft()
			d.Number = fmt.Sprintf("QR-%s", mode.name)
			d.CustomerID, d.Customer = customer.ID, customer
			d.PaymentMeansCode = "58"
			d.DisablePaymentQR, d.HidePaymentQRDetails = mode.disabled, mode.hideDetails
			saved, err := s.SaveInvoice(d)
			if err != nil {
				t.Fatal(err)
			}
			opened, err := s.OpenInvoice(saved.Draft.ID)
			if err != nil || opened.Draft.DisablePaymentQR != mode.disabled || opened.Draft.HidePaymentQRDetails != mode.hideDetails {
				t.Fatal("draft settings not persisted", err)
			}
			boot, err := s.Bootstrap()
			if err != nil {
				t.Fatal(err)
			}
			defaults := boot.Customers[0].InvoiceSettings
			if defaults == nil || defaults.DisablePaymentQR != mode.disabled || defaults.HidePaymentQRDetails != mode.hideDetails {
				t.Fatal("customer QR defaults lost")
			}
			preview, err := s.InvoiceDocumentHTML(opened.Draft)
			if err != nil {
				t.Fatal(err)
			}
			if strings.Contains(preview, `class="payment-qr"`) == mode.disabled {
				t.Fatal("wrong HTML QR visibility")
			}
			if strings.Contains(preview, "SEPA-Überweisung per QR-Code") != (!mode.disabled && !mode.hideDetails) {
				t.Fatal("wrong HTML QR details visibility")
			}
			issued, err := s.IssueInvoice(opened.Draft)
			if err != nil {
				t.Fatal(err)
			}
			changed := issued.Draft
			changed.DisablePaymentQR, changed.HidePaymentQRDetails = !mode.disabled, !mode.hideDetails
			issuedHTML, err := s.InvoiceDocumentHTML(changed)
			if err != nil || preview != issuedHTML {
				t.Fatal("issued QR settings are not immutable", err)
			}
			pdf, err := s.invoicePDF(issued.Draft.ID)
			if err != nil {
				t.Fatal(err)
			}
			if bytes.Contains(pdf, []byte("/Subtype /Image")) == mode.disabled {
				t.Fatal("wrong PDF QR visibility")
			}
			if dir != "" {
				stem := filepath.Join(dir, "qr-"+mode.name)
				draftJSON, err := json.Marshal(opened.Draft)
				if err != nil {
					t.Fatal(err)
				}
				for ext, data := range map[string][]byte{".html": []byte(preview), ".pdf": pdf, ".json": draftJSON} {
					if err = os.WriteFile(stem+ext, data, 0600); err != nil {
						t.Fatal(err)
					}
				}
			}
		})
	}
}
