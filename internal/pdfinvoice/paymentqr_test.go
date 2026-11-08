package pdfinvoice

import (
	"bytes"
	"github.com/shopspring/decimal"
	"html/template"
	"image/png"
	"os"
	"papierleicht.de/e-rechnung-studio/internal/einvoice"
	"strings"
	"testing"

	"golang.org/x/net/html"
)

func TestLocalImageSourcePreservesFilesystemPath(t *testing.T) {
	for _, path := range []string{
		`C:\Users\Max Müller\AppData\Local\Temp\ers-payment-qr-123.png`,
		`C:\Temp\a&b#100%.png`,
		`/tmp/a "quoted" & special/qr.png`,
		`\\server\share\qr.png`,
	} {
		t.Run(path, func(t *testing.T) {
			var out bytes.Buffer
			tmpl := template.Must(template.New("image").Parse(`<img {{.}}/>`))
			if err := tmpl.Execute(&out, localImageSource(path)); err != nil {
				t.Fatal(err)
			}
			parser := html.NewTokenizer(&out)
			if parser.Next() != html.SelfClosingTagToken {
				t.Fatal("missing image element")
			}
			token := parser.Token()
			if len(token.Attr) != 1 || token.Attr[0].Key != "src" || token.Attr[0].Val != path {
				t.Fatalf("image source changed: %+v", token.Attr)
			}
		})
	}
	if localImageSource("") != "" {
		t.Fatal("absent QR must not produce an image source")
	}
}

func qrInvoice() *einvoice.Invoice {
	inv := &einvoice.Invoice{InvoiceCurrencyCode: "EUR", InvoiceTypeCode: 380, InvoiceNumber: "RE-2026-001", DuePayableAmount: decimal.RequireFromString("238"), GrandTotal: decimal.RequireFromString("500")}
	inv.Seller.Name = "Muster GmbH"
	inv.PaymentMeans = []einvoice.PaymentMeans{{TypeCode: 58, PayeePartyCreditorFinancialAccountIBAN: "de89 3704 0044 0532 0130 00", PayeeSpecifiedCreditorFinancialInstitutionBIC: "COBADEFFXXX"}}
	return inv
}

func TestPaymentQRPayload(t *testing.T) {
	inv := qrInvoice()
	want := "BCD\n002\n1\nSCT\nCOBADEFFXXX\nMuster GmbH\nDE89370400440532013000\nEUR238.00\n\n\nRE-2026-001"
	if got := paymentQRPayload(inv); got != want {
		t.Fatalf("payload = %q; want %q", got, want)
	}
	inv.PaymentReference = "Kunde Müller 42"
	if !strings.HasSuffix(paymentQRPayload(inv), "\nKunde Müller 42") {
		t.Fatal("explicit reference lost")
	}
	inv.PaymentMeans[0].PayeeSpecifiedCreditorFinancialInstitutionBIC = ""
	if paymentQRPayload(inv) == "" {
		t.Fatal("EEA BIC must be optional")
	}
}

func TestPaymentQRIneligible(t *testing.T) {
	tests := map[string]func(*einvoice.Invoice){
		"cash":             func(i *einvoice.Invoice) { i.PaymentMeans[0].TypeCode = 10 },
		"generic transfer": func(i *einvoice.Invoice) { i.PaymentMeans[0].TypeCode = 30 },
		"credit":           func(i *einvoice.Invoice) { i.InvoiceTypeCode = 381 },
		"currency":         func(i *einvoice.Invoice) { i.InvoiceCurrencyCode = "USD" },
		"zero":             func(i *einvoice.Invoice) { i.DuePayableAmount = decimal.Zero },
		"negative":         func(i *einvoice.Invoice) { i.DuePayableAmount = decimal.NewFromInt(-1) },
		"too large":        func(i *einvoice.Invoice) { i.DuePayableAmount = decimal.NewFromInt(1000000000) },
		"fraction":         func(i *einvoice.Invoice) { i.DuePayableAmount = decimal.RequireFromString("1.001") },
		"IBAN": func(i *einvoice.Invoice) {
			i.PaymentMeans[0].PayeePartyCreditorFinancialAccountIBAN = "DE00370400440532013000"
		},
		"missing":    func(i *einvoice.Invoice) { i.PaymentMeans = nil },
		"name":       func(i *einvoice.Invoice) { i.Seller.Name = strings.Repeat("a", 71) },
		"reference":  func(i *einvoice.Invoice) { i.PaymentReference = strings.Repeat("a", 141) },
		"injection":  func(i *einvoice.Invoice) { i.Seller.Name = "Muster\nOther" },
		"UTF8 bytes": func(i *einvoice.Invoice) { i.PaymentReference = strings.Repeat("界", 140) },
		"nonEEA needs BIC": func(i *einvoice.Invoice) {
			i.PaymentMeans[0].PayeePartyCreditorFinancialAccountIBAN = "GB82WEST12345698765432"
			i.PaymentMeans[0].PayeeSpecifiedCreditorFinancialInstitutionBIC = ""
		},
	}
	for name, change := range tests {
		t.Run(name, func(t *testing.T) {
			inv := qrInvoice()
			change(inv)
			if got := paymentQRPayload(inv); got != "" {
				t.Fatalf("unexpected payload: %q", got)
			}
		})
	}
}

func TestPaymentQRImageQuietZone(t *testing.T) {
	path, err := paymentQRImage(qrInvoice())
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(path)
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	img, err := png.Decode(f)
	if err != nil {
		t.Fatal(err)
	}
	if img.Bounds().Dx() > (69+8)*8 {
		t.Fatal("exceeds EPC version 13")
	}
	for y := 0; y < img.Bounds().Dy(); y++ {
		for x := 0; x < img.Bounds().Dx(); x++ {
			if x < 32 || y < 32 || x >= img.Bounds().Dx()-32 || y >= img.Bounds().Dy()-32 {
				r, g, b, _ := img.At(x, y).RGBA()
				if r != 65535 || g != 65535 || b != 65535 {
					t.Fatal("quiet zone not white")
				}
			}
		}
	}
}
