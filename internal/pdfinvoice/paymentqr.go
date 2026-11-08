package pdfinvoice

import (
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"os"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/shopspring/decimal"
	"papierleicht.de/e-rechnung-studio/internal/einvoice"
	"papierleicht.de/e-rechnung-studio/internal/model"
	"papierleicht.de/e-rechnung-studio/internal/paymentqr"
	"papierleicht.de/e-rechnung-studio/internal/validation"
)

// EPC069-12 v3.1: UTF-8, payload version 002, error correction M.
// Never truncate payment data or produce a QR for a refund/non-EUR payment.
func paymentQRPayload(inv *einvoice.Invoice) string {
	if inv.InvoiceCurrencyCode != "EUR" || inv.InvoiceTypeCode == 381 || len(inv.PaymentMeans) == 0 {
		return ""
	}
	p := inv.PaymentMeans[0]
	if p.TypeCode != 58 {
		return ""
	}
	amount := inv.DuePayableAmount
	if amount.LessThan(decimal.RequireFromString("0.01")) || amount.GreaterThan(decimal.RequireFromString("999999999.99")) || !amount.Equal(amount.Round(2)) {
		return ""
	}
	iban, bic := strings.ToUpper(model.CompactIdentifier(p.PayeePartyCreditorFinancialAccountIBAN)), strings.ToUpper(model.CompactIdentifier(p.PayeeSpecifiedCreditorFinancialInstitutionBIC))
	if len(iban) < 2 || len(validation.CompanyIssues(model.Company{IBAN: iban, BIC: bic})) != 0 {
		return ""
	}
	// A BIC is required outside the EEA. Unknown countries are conservative.
	eea := " AT BE BG CY CZ DE DK EE ES FI FR GR HR HU IE IS IT LI LT LU LV MT NL NO PL PT RO SE SI SK "
	if bic == "" && !strings.Contains(eea, " "+iban[:2]+" ") {
		return ""
	}
	name, reference := inv.Seller.Name, inv.PaymentReference
	if reference == "" {
		reference = inv.InvoiceNumber
	}
	if strings.TrimSpace(name) == "" || utf8.RuneCountInString(name) > 70 || utf8.RuneCountInString(reference) > 140 {
		return ""
	}
	for _, value := range []string{name, reference} {
		if strings.IndexFunc(value, unicode.IsControl) >= 0 || !utf8.ValidString(value) {
			return ""
		}
	}
	payload := strings.TrimRight(strings.Join([]string{"BCD", "002", "1", "SCT", bic, name, iban, "EUR" + amount.StringFixed(2), "", "", reference}, "\n"), "\n")
	if len(payload) > 331 {
		return ""
	}
	return payload
}

// Use integer module scaling and a four-module white quiet zone. The temporary
// file is private and is removed by the renderer immediately after PDF creation.
func paymentQRImage(inv *einvoice.Invoice) (string, error) {
	payload := paymentQRPayload(inv)
	if payload == "" {
		return "", nil
	}
	code, err := paymentqr.Encode(payload)
	if err != nil {
		return "", err
	}
	const scale = 8
	size := code.Bounds().Dx()
	canvas := image.NewRGBA(image.Rect(0, 0, (size+8)*scale, (size+8)*scale))
	draw.Draw(canvas, canvas.Bounds(), image.NewUniform(color.White), image.Point{}, draw.Src)
	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			draw.Draw(canvas, image.Rect((x+4)*scale, (y+4)*scale, (x+5)*scale, (y+5)*scale), image.NewUniform(code.At(x, y)), image.Point{}, draw.Src)
		}
	}
	f, err := os.CreateTemp("", "ers-payment-qr-*.png")
	if err != nil {
		return "", err
	}
	err = png.Encode(f, canvas)
	closeErr := f.Close()
	if err == nil {
		err = closeErr
	}
	if err != nil {
		os.Remove(f.Name())
		return "", err
	}
	return f.Name(), nil
}
