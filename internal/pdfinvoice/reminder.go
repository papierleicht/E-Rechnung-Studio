package pdfinvoice

import (
	"bytes"
	_ "embed"
	"fmt"
	"html/template"
	"os"
	"strings"
	"time"

	pdf "github.com/boxesandglue/baseline-pdf"
	"github.com/boxesandglue/boxesandglue/backend/document"
	"github.com/boxesandglue/boxesandglue/frontend"
	"github.com/boxesandglue/htmlbag"
	"github.com/shopspring/decimal"
	"papierleicht.de/e-rechnung-studio/internal/einvoice"
	"papierleicht.de/e-rechnung-studio/internal/model"
)

//go:embed reminder.html
var reminderHTML string

const reminderCSS = `.letter { margin: 0 0 4mm; } .letter .paragraph { line-height: 1.55; }`

// The reminder shares the invoice's template functions and stylesheet.
var reminderTemplate = template.Must(template.Must(invoiceTemplate.Clone()).Funcs(template.FuncMap{"eyebrow": eyebrowText}).New("pdf-reminder").Parse(reminderHTML))

// eyebrowText keeps the word gaps of a letter-spaced title such as
// „2. Mahnung“ as wide as before: htmlbag now widens a space by the
// letter-spacing, as CSS does; pdfCSS resets it on the gap.
func eyebrowText(title string) template.HTML {
	words := strings.Split(title, " ")
	for i, word := range words {
		words[i] = template.HTMLEscapeString(word)
	}
	return template.HTML(strings.Join(words, `<span class="eyebrow-gap"> </span>`))
}

// ReminderLetter is the already resolved content of a payment reminder.
// Amounts come from the payment ledger; the invoice XML supplies parties and
// bank details only.
type ReminderLetter struct {
	Title        string
	Text         string
	Date         time.Time
	DueSince     time.Time
	PaymentDue   time.Time
	Currency     string
	Amount       decimal.Decimal
	Fee          decimal.Decimal
	Interest     decimal.Decimal
	Total        decimal.Decimal
	InterestRate string
	DaysOverdue  int
	Installment  bool
	PaymentQR    bool
}

type reminderTemplateData struct {
	*einvoice.Invoice
	Letter      ReminderLetter
	PaymentHint string
	Reference   string
	PaymentQR   template.HTMLAttr
	Logo        template.HTMLAttr
}

// RenderReminder creates the reminder PDF for an issued invoice. Pass the
// invoice's issued snapshots so sender, logo and bank details match the invoice.
func RenderReminder(xml []byte, issuer *model.Company, buyer *model.Customer, letter ReminderLetter) ([]byte, error) {
	renderMu.Lock()
	defer renderMu.Unlock()
	inv, err := presentationInvoice(xml, issuer, buyer)
	if err != nil {
		return nil, err
	}
	reference := inv.PaymentReference
	if reference == "" {
		reference = inv.InvoiceNumber
	}
	amount := strings.ReplaceAll(letter.Total.StringFixed(2), ".", ",") + " " + letter.Currency
	hint := fmt.Sprintf("Bitte überweisen Sie %s unter Angabe von „%s“.", amount, reference)
	for _, means := range inv.PaymentMeans {
		if means.PayeePartyCreditorFinancialAccountIBAN != "" {
			hint = fmt.Sprintf("Bitte überweisen Sie %s unter Angabe von „%s“ auf das Konto IBAN %s.", amount, reference, displayIBAN(means.PayeePartyCreditorFinancialAccountIBAN))
			break
		}
	}
	var qrPath string
	if letter.PaymentQR {
		// The QR code requests exactly the reminder's amount, never the invoice total.
		qrInvoice := *inv
		qrInvoice.DuePayableAmount = letter.Total
		qrInvoice.PaymentReference = reference
		if letter.Currency != inv.InvoiceCurrencyCode {
			qrInvoice.InvoiceCurrencyCode = ""
		}
		qrPath, err = paymentQRImage(&qrInvoice)
		if err != nil {
			return nil, err
		}
		if qrPath != "" {
			defer os.Remove(qrPath)
		}
	}
	logo, err := issuerLogo(issuer)
	if err != nil {
		return nil, err
	}
	logoPath, err := logoImageFile(logo)
	if err != nil {
		return nil, err
	}
	if logoPath != "" {
		defer os.Remove(logoPath)
	}
	var html bytes.Buffer
	if err = reminderTemplate.ExecuteTemplate(&html, "pdf-reminder", reminderTemplateData{
		Invoice: inv, Letter: letter, PaymentHint: hint, Reference: reference,
		PaymentQR: localImageSource(qrPath), Logo: logoAttributes(localImageSource(logoPath), logo),
	}); err != nil {
		return nil, err
	}
	var out bytes.Buffer
	fe, err := frontend.NewForWriter(&out)
	if err != nil {
		return nil, err
	}
	var missingGlyph rune
	fe.MissingGlyphFunc = func(_ *pdf.Face, r rune) {
		if missingGlyph == 0 {
			missingGlyph = r
		}
	}
	fe.Doc.Format = document.FormatPDFA3b
	fe.Doc.Title = letter.Title + " zur Rechnung " + inv.InvoiceNumber
	fe.Doc.Author = inv.Seller.Name
	fe.Doc.Creator = creatorName
	if _, err = fe.Doc.LoadSRGBColorprofile(); err != nil {
		return nil, err
	}
	cb, err := htmlbag.New(fe, htmlbag.NewCSSParserWithDefaults())
	if err != nil {
		return nil, err
	}
	if err = cb.AddCSS(invoiceCSS + pdfCSS + reminderCSS); err != nil {
		return nil, err
	}
	if err = cb.InitPage(); err != nil {
		return nil, err
	}
	text, err := cb.HTMLToText(html.String())
	if err != nil {
		return nil, err
	}
	if err = cb.OutputPagesFromText(text); err != nil {
		return nil, err
	}
	if missingGlyph != 0 {
		return nil, fmt.Errorf("Das Zeichen %q kann in der PDF-Schrift nicht dargestellt werden. Ersetze es und erstelle die Erinnerung noch einmal.", missingGlyph)
	}
	if err = fe.Finish(); err != nil {
		return nil, err
	}
	if err = fe.Doc.PDFWriter.FinishAndClose(); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}
