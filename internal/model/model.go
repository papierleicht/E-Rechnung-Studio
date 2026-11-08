package model

import "time"

type Address struct {
	Street     string `json:"street"`
	Additional string `json:"additional"`
	Postcode   string `json:"postcode"`
	City       string `json:"city"`
	Country    string `json:"country"`
}

type Company struct {
	InvoiceTaxPreset    *string       `json:"invoiceTaxPreset,omitempty"`
	TaxStatusUnclear    bool          `json:"taxStatusUnclear,omitempty"`
	DefaultTaxTreatment string        `json:"defaultTaxTreatment,omitempty"`
	AdditionalAccounts  []BankAccount `json:"additionalAccounts,omitempty"`
	PaymentMeansCode    string        `json:"paymentMeansCode,omitempty"`
	SetupCompleted      bool          `json:"setupCompleted,omitempty"`
	Name                string        `json:"name"`
	ContactName         string        `json:"contactName"`
	Email               string        `json:"email"`
	Phone               string        `json:"phone"`
	VATID               string        `json:"vatId"`
	TaxNumber           string        `json:"taxNumber"`
	Endpoint            string        `json:"endpoint"`
	EndpointScheme      string        `json:"endpointScheme"`
	IBAN                string        `json:"iban"`
	BIC                 string        `json:"bic"`
	BankName            string        `json:"bankName"`
	SmallBusiness       bool          `json:"smallBusiness"`
	ExemptionReason     string        `json:"exemptionReason"`
	Address             Address       `json:"address"`
	// Logo is a normalized base64 PNG (see invoicelogo), printed on invoices.
	Logo string `json:"logo,omitempty"`
}

// BankAccount is an additional credit-transfer destination, never a payer account.
type BankAccount struct {
	IBAN string `json:"iban"`
	BIC  string `json:"bic"`
	Name string `json:"name"`
}

type Customer struct {
	InvoiceSettings *InvoiceSettings `json:"invoiceSettings,omitempty"`
	PaymentTermDays *int             `json:"paymentTermDays"`
	ID              string           `json:"id"`
	Name            string           `json:"name"`
	ContactName     string           `json:"contactName"`
	Email           string           `json:"email"`
	VATID           string           `json:"vatId"`
	Endpoint        string           `json:"endpoint"`
	EndpointScheme  string           `json:"endpointScheme"`
	BuyerReference  string           `json:"buyerReference"`
	Address         Address          `json:"address"`
	CreatedAt       string           `json:"createdAt"`
	UpdatedAt       string           `json:"updatedAt"`
}

// Defaults for future invoices only; issued invoice snapshots remain unchanged.
type InvoiceSettings struct {
	TaxSource            string `json:"taxSource,omitempty"`
	DisablePaymentQR     bool   `json:"disablePaymentQR,omitempty"`
	HidePaymentQRDetails bool   `json:"hidePaymentQRDetails,omitempty"`
	PaymentMeansCode     string `json:"paymentMeansCode,omitempty"`
	Profile              string `json:"profile"`
	Kind                 string `json:"kind"`
	TaxTreatment         string `json:"taxTreatment"`
	TaxExemptionReason   string `json:"taxExemptionReason"`
	BusinessProcess      string `json:"businessProcess"`
	// NumberSeries is the customer's own Nummernkreis; empty uses the standard.
	// It changes only on request, never as a side effect of an invoice.
	NumberSeries string `json:"numberSeries,omitempty"`
}

type AppSettings struct {
	AutomaticUpdates bool   `json:"automaticUpdates"`
	ExportDirectory  string `json:"exportDirectory"`
	Appearance       string `json:"appearance,omitempty"`
	// Sections shows (true) or hides (false) optional areas of the menu,
	// see MenuSections. A missing key keeps the default: "shops" shows only
	// with a connected shop, "products" always.
	Sections map[string]bool `json:"sections,omitempty"`
}

// MenuSections are the areas of the menu a customer may hide.
var MenuSections = []string{"shops", "products"}

type InvoiceExport struct {
	PDFPath string `json:"pdfPath"`
	XMLPath string `json:"xmlPath"`
	Error   string `json:"error,omitempty"`
}

type Product struct {
	ID string `json:"id"`
	// ArticleNumber is the seller's own number (BT-155). The ID is internal.
	ArticleNumber string `json:"articleNumber,omitempty"`
	Name          string `json:"name"`
	Description   string `json:"description"`
	Unit          string `json:"unit"`
	UnitPrice     string `json:"unitPrice"`
	TaxRate       string `json:"taxRate"`
	TaxCategory   string `json:"taxCategory"`
	CreatedAt     string `json:"createdAt"`
	UpdatedAt     string `json:"updatedAt"`
}

type InvoiceLine struct {
	TaxTreatment    string `json:"taxTreatment,omitempty"`
	ExemptionCode   string `json:"exemptionCode,omitempty"`
	ExemptionReason string `json:"exemptionReason,omitempty"`
	ID              string `json:"id"`
	ProductID       string `json:"productId"`
	ArticleNumber   string `json:"articleNumber,omitempty"`
	Name            string `json:"name"`
	Description     string `json:"description"`
	Quantity        string `json:"quantity"`
	Unit            string `json:"unit"`
	UnitPrice       string `json:"unitPrice"`
	TaxRate         string `json:"taxRate"`
	TaxCategory     string `json:"taxCategory"`
}

// InvoiceAdjustment is a net document-level allowance or charge (BG-20/21).
// Amount is always positive; Charge determines whether it adds to the total.
type InvoiceAdjustment struct {
	ID          string `json:"id"`
	Charge      bool   `json:"charge"`
	Amount      string `json:"amount"`
	Reason      string `json:"reason"`
	TaxCategory string `json:"taxCategory"`
	TaxRate     string `json:"taxRate"`
}

// SourceTax is the VAT of one rate as a shop computed it, rounding each line.
// Basis is the taxable amount the shop's figure belongs to.
type SourceTax struct {
	Rate   string `json:"rate"`
	Basis  string `json:"basis"`
	Amount string `json:"amount"`
}

type InvoiceDraft struct {
	PrepaidAmount string `json:"prepaidAmount,omitempty"`
	// SourceTaxes keep a shop's VAT per rate when its line rounding differs
	// from the per-rate calculation by a few cents, so the invoice matches
	// what the customer paid. The engine uses an amount only while its rate
	// group keeps the same basis.
	SourceTaxes            []SourceTax         `json:"sourceTaxes,omitempty"`
	TaxSource              string              `json:"taxSource,omitempty"`
	AutomaticNumber        bool                `json:"automaticNumber,omitempty"`
	DisablePaymentQR       bool                `json:"disablePaymentQR,omitempty"`
	HidePaymentQRDetails   bool                `json:"hidePaymentQRDetails,omitempty"`
	Adjustments            []InvoiceAdjustment `json:"adjustments,omitempty"`
	PaymentMeansCode       string              `json:"paymentMeansCode,omitempty"`
	CorrectsInvoiceID      string              `json:"correctsInvoiceId,omitempty"`
	PrecedingInvoiceNumber string              `json:"precedingInvoiceNumber,omitempty"`
	PrecedingInvoiceDate   string              `json:"precedingInvoiceDate,omitempty"`
	// CorrectionReason of a credit note or corrected invoice decides its
	// Voranmeldungszeitraum: CorrectionChange or CorrectionReplace.
	CorrectionReason string `json:"correctionReason,omitempty"`
	// AdvancePayment marks an invoice over a payment before the supply. Only the
	// user sets it; under Soll-Versteuerung its tax then follows the payments.
	AdvancePayment bool `json:"advancePayment,omitempty"`
	// NumberSeries chooses the Nummernkreis of an automatic number: empty
	// follows the customer's series, "standard" the standard one, anything
	// else is a series template such as "RE-{JJJJ}-{NNNN}".
	NumberSeries string `json:"numberSeries,omitempty"`
	// MarkedSentAt notes that the user sent an issued invoice without the
	// app's e-mail, e.g. by post. A sent invoice can no longer be deleted.
	MarkedSentAt string `json:"markedSentAt,omitempty"`
	// Set when the draft was created from a quote; display reference only.
	QuoteID     string `json:"quoteId,omitempty"`
	QuoteNumber string `json:"quoteNumber,omitempty"`
	// Set when the draft came from a recurring invoice; RecurringID also lets
	// the issued invoice carry a new price over to that recurring invoice.
	RecurringID   string `json:"recurringId,omitempty"`
	RecurringName string `json:"recurringName,omitempty"`
	// Captured by the service at issue time. BASIC has no structured contacts;
	// this immutable snapshot preserves the issuer contact for PDF and reopening.
	IssuerSnapshot      *Company      `json:"issuerSnapshot,omitempty"`
	ID                  string        `json:"id"`
	Number              string        `json:"number"`
	Kind                string        `json:"kind"`
	Status              string        `json:"status"`
	CustomerID          string        `json:"customerId"`
	Customer            Customer      `json:"customer"`
	IssueDate           string        `json:"issueDate"`
	DeliveryDate        string        `json:"deliveryDate"`
	DeliveryCountry     string        `json:"deliveryCountry"`
	DeliveryCity        string        `json:"deliveryCity"`
	DeliveryPostcode    string        `json:"deliveryPostcode"`
	PeriodStart         string        `json:"periodStart"`
	PeriodEnd           string        `json:"periodEnd"`
	DueDate             string        `json:"dueDate"`
	Currency            string        `json:"currency"`
	BuyerReference      string        `json:"buyerReference"`
	BusinessProcess     string        `json:"businessProcess"`
	PaymentReference    string        `json:"paymentReference"`
	Notes               string        `json:"notes"`
	PaymentTerms        string        `json:"paymentTerms"`
	Profile             string        `json:"profile"`
	Syntax              string        `json:"syntax"`
	TaxTreatment        string        `json:"taxTreatment"`
	TaxExemptionReason  string        `json:"taxExemptionReason"`
	Lines               []InvoiceLine `json:"lines"`
	CreatedAt           string        `json:"createdAt"`
	UpdatedAt           string        `json:"updatedAt"`
	IssuedAt            string        `json:"issuedAt"`
	PaidAt              string        `json:"paidAt"`
	OriginalXML         string        `json:"originalXml,omitempty"`
	GeneratedXML        string        `json:"generatedXml,omitempty"`
	GeneratedAt         string        `json:"generatedAt,omitempty"`
	LastValidationState string        `json:"lastValidationState,omitempty"`
}

type Totals struct {
	Net   string `json:"net"`
	Tax   string `json:"tax"`
	Gross string `json:"gross"`
	Due   string `json:"due"`
	// Prepaid is the paid amount already deducted from Gross (BT-113), set only
	// when the document deducts one, e.g. a final invoice.
	Prepaid string `json:"prepaid,omitempty"`
}

type ValidationIssue struct {
	Field    string `json:"field,omitempty"`
	Details  string `json:"details,omitempty"`
	Stage    string `json:"stage"`
	RuleID   string `json:"ruleId"`
	Severity string `json:"severity"`
	Message  string `json:"message"`
	Location string `json:"location,omitempty"`
	// Class tells received documents' findings apart: "format", "business",
	// "hint" or "unchecked" (see the validation package).
	Class string `json:"class,omitempty"`
}

type ValidationResult struct {
	State  string            `json:"state"`
	Issues []ValidationIssue `json:"issues"`
}

type InvoiceView struct {
	Export      InvoiceExport    `json:"export"`
	Draft       InvoiceDraft     `json:"draft"`
	Company     Company          `json:"company"`
	Totals      Totals           `json:"totals"`
	LineTotals  []string         `json:"lineTotals"`
	Validation  ValidationResult `json:"validation"`
	XML         string           `json:"xml,omitempty"`
	PreviewHTML string           `json:"previewHtml"`
}

type InvoiceSummary struct {
	CustomerID   string `json:"customerId"`
	ID           string `json:"id"`
	Number       string `json:"number"`
	CustomerName string `json:"customerName"`
	IssueDate    string `json:"issueDate"`
	Status       string `json:"status"`
	Gross        string `json:"gross"`
	UpdatedAt    string `json:"updatedAt"`
	// SentAt is the latest successful e-mail delivery or, without one, the
	// day the user marked the invoice as sent (empty if neither).
	SentAt string `json:"sentAt,omitempty"`
	// SentVia is "mail" or "manual".
	SentVia string `json:"sentVia,omitempty"`
}

type Bootstrap struct {
	Settings  AppSettings      `json:"settings"`
	Company   Company          `json:"company"`
	Customers []Customer       `json:"customers"`
	Products  []Product        `json:"products"`
	Invoices  []InvoiceSummary `json:"invoices"`
	Draft     InvoiceDraft     `json:"draft"`
}

type ImportInspection struct {
	DueDate       string           `json:"dueDate"`
	IBAN          string           `json:"iban"`
	Syntax        string           `json:"syntax"`
	Profile       string           `json:"profile"`
	InvoiceNumber string           `json:"invoiceNumber"`
	IssueDate     string           `json:"issueDate"`
	SellerName    string           `json:"sellerName"`
	BuyerName     string           `json:"buyerName"`
	Currency      string           `json:"currency"`
	Totals        Totals           `json:"totals"`
	Validation    ValidationResult `json:"validation"`
	// InputTax says whether the document counts for the Vorsteuer. It is derived
	// from Validation whenever a received document is shown.
	InputTax *InputTaxStatus `json:"inputTax,omitempty"`
	// DocumentType is the type code of the document (BT-3): 380 invoice, 381
	// credit note, 384 corrected invoice.
	DocumentType int `json:"documentType,omitempty"`
	// PrecedingInvoices are the invoices the document refers to (BG-3).
	PrecedingInvoices []PrecedingInvoice `json:"precedingInvoices,omitempty"`
}

func NewDraft() InvoiceDraft {
	today := time.Now().Format("2006-01-02")
	return InvoiceDraft{
		Kind: "invoice", Status: "draft", IssueDate: today, DeliveryDate: today, DueDate: time.Now().AddDate(0, 0, 14).Format("2006-01-02"),
		Currency: "EUR", Profile: "facturx-en16931", Syntax: "cii",
		TaxTreatment: "standard", Lines: []InvoiceLine{{Quantity: "1", Unit: "C62", TaxRate: "19", TaxCategory: "S"}},
	}
}

// ApplicationLists is the data needed after mutations to invoices or master data.
type ApplicationLists struct {
	Customers []Customer       `json:"customers"`
	Products  []Product        `json:"products"`
	Invoices  []InvoiceSummary `json:"invoices"`
}
