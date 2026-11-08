package app

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"

	"github.com/shopspring/decimal"
	"papierleicht.de/e-rechnung-studio/internal/banking"
	"papierleicht.de/e-rechnung-studio/internal/einvoice"
	"papierleicht.de/e-rechnung-studio/internal/engine"
	"papierleicht.de/e-rechnung-studio/internal/invoicexml"
	"papierleicht.de/e-rechnung-studio/internal/mail"
	"papierleicht.de/e-rechnung-studio/internal/model"
	"papierleicht.de/e-rechnung-studio/internal/store"
	validator "papierleicht.de/e-rechnung-studio/internal/validation"
)

type InvoiceService struct {
	// shopMu keeps a restore from interleaving with a shop import.
	shopMu      sync.Mutex
	paymentMu   sync.Mutex
	reconcileMu sync.Mutex
	bank        *banking.Service
	exportMu    sync.Mutex
	issueMu     sync.Mutex
	recurringMu sync.Mutex
	store       *store.Store
	engine      *engine.Engine
	validator   *validator.Validator
	close       closeGuard
	mail        *mail.Service
	// receivedTax caches the tax breakdown of received documents. An ID is
	// the hash of the stored file, so an entry never goes stale; nil Rates
	// mark a document without readable invoice data.
	receivedMu  sync.Mutex
	receivedTax map[string]receivedTax
}

// SetUnsavedChanges keeps native window-close handling in sync with the editor.
func (s *InvoiceService) SetUnsavedChanges(dirty bool) {
	s.close.setDirty(dirty)
}

// newValidator is replaced in tests, which share one compiled rule cache
// instead of unpacking and compiling the rules for every service.
var newValidator = validator.New

func NewInvoiceService(dataDir string) (*InvoiceService, error) {
	s, err := store.Open(filepath.Join(dataDir, "e-rechnung-studio.db"))
	if err != nil {
		return nil, err
	}
	if err = s.SetVersion(currentVersion); err != nil {
		s.Close()
		return nil, err
	}
	v, err := newValidator(filepath.Join(dataDir, "cache"))
	if err != nil {
		s.Close()
		return nil, err
	}
	return &InvoiceService{store: s, engine: engine.New(), validator: v}, nil
}

func defaultDataDir() (string, error) {
	if root := os.Getenv("E_RECHNUNG_STUDIO_DATA_DIR"); root != "" {
		return root, nil
	}
	base, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(base, "ERechnungStudio"), nil
}

// Close releases native resources during application shutdown. It is not part
// of the frontend API; exposing it would let webview code close the live store.
//
//wails:ignore
func (s *InvoiceService) Close() error { return s.store.Close() }

func (s *InvoiceService) Bootstrap() (model.Bootstrap, error) {
	c, err := s.store.Company()
	if err != nil {
		return model.Bootstrap{}, err
	}
	lists, err := s.ApplicationLists()
	if err != nil {
		return model.Bootstrap{}, err
	}

	draft := model.NewDraft()
	draft, err = s.store.RefreshDraftNumber(draft)
	if err != nil {
		return model.Bootstrap{}, err
	}
	draft.TaxTreatment, draft.TaxExemptionReason = c.InvoiceTaxTreatment(), c.ExemptionReason
	draft.TaxSource = "company"
	if draft.TaxTreatment == "mixed" {
		draft.TaxExemptionReason = ""
		for i := range draft.Lines {
			draft.Lines[i].TaxTreatment = "standard"
			if c.SmallBusiness {
				draft.Lines[i].TaxTreatment = "small-business"
			}
		}
	}
	settings, err := s.GetAppSettings()
	if err != nil {
		return model.Bootstrap{}, err
	}
	return model.Bootstrap{Settings: settings, Company: c, Customers: lists.Customers, Products: lists.Products, Invoices: lists.Invoices, Draft: draft}, nil
}

func (s *InvoiceService) SaveCompany(c model.Company) (model.Company, error) {
	c = model.NormalizeCompany(c)
	if issues := validator.CompanyIssues(c); len(issues) > 0 {
		return c, errors.New(issues[0].Message)
	}
	if err := s.store.SaveCompany(c); err != nil {
		return c, err
	}
	return c, nil
}

// ValidateCompanyStep checks a copy without persisting or rewriting displayed input.
func (s *InvoiceService) ValidateCompanyStep(c model.Company, step string) ([]model.ValidationIssue, error) {
	if step != "tax" && step != "payment" {
		return nil, errors.New("Dieser Einrichtungsschritt ist unbekannt.")
	}
	issues := []model.ValidationIssue{}
	for _, issue := range validator.CompanyIssues(c) {
		isTax := issue.Field == "company.vatId" || issue.Field == "company.taxNumber" || issue.Field == "company.defaultTaxTreatment" || issue.Field == "company.invoiceTaxPreset"
		isPayment := issue.Field == "company.iban" || issue.Field == "company.bic" || issue.Field == "company.paymentMeansCode" || strings.HasPrefix(issue.Field, "company.additionalAccounts.")
		if (step == "tax" && isTax) || (step == "payment" && isPayment) {
			issues = append(issues, issue)
		}
	}
	return issues, nil
}

// CompleteSetup persists the one-time onboarding marker together with the company.
func (s *InvoiceService) CompleteSetup(c model.Company) (model.Company, error) {
	for _, field := range []struct{ value, name string }{
		{c.Name, "Unternehmensname"}, {c.Address.Street, "Straße und Hausnummer"},
		{c.Address.City, "Ort"}, {c.Address.Country, "Land"},
	} {
		if strings.TrimSpace(field.value) == "" {
			return c, fmt.Errorf("Ergänze die Angabe „%s“.", field.name)
		}
	}
	c.Address.Country = strings.ToUpper(strings.TrimSpace(c.Address.Country))
	if len(c.Address.Country) != 2 {
		return c, errors.New("Gib einen zweistelligen Ländercode ein, z. B. DE.")
	}
	c.SetupCompleted = true
	return s.SaveCompany(c)
}

func (s *InvoiceService) SaveCustomer(c model.Customer) (model.Customer, error) {
	if c.PaymentTermDays != nil && (*c.PaymentTermDays < 0 || *c.PaymentTermDays > 3650) {
		return c, errors.New("Wähle ein Zahlungsziel zwischen 0 und 3650 Tagen.")
	}
	c = model.NormalizeCustomer(c)
	if issues := validator.CustomerIssues(c); len(issues) > 0 {
		return c, errors.New(issues[0].Message)
	}
	if c.Name == "" {
		return c, errors.New("Gib einen Kundennamen ein.")
	}
	return s.store.SaveCustomer(c)
}

func (s *InvoiceService) SaveProduct(p model.Product) (model.Product, error) {
	p.Name = strings.TrimSpace(p.Name)
	p.Description = strings.TrimSpace(p.Description)
	p.Unit = strings.ToUpper(strings.TrimSpace(p.Unit))
	p.UnitPrice = strings.TrimSpace(p.UnitPrice)
	p.TaxRate = strings.TrimSpace(p.TaxRate)
	p.TaxCategory = strings.ToUpper(strings.TrimSpace(p.TaxCategory))
	if p.Name == "" {
		return p, errors.New("Gib dem Artikel eine Bezeichnung.")
	}
	if p.Unit == "" {
		return p, errors.New("Wähle eine Einheit für den Artikel.")
	}
	price, err := decimal.NewFromString(strings.ReplaceAll(p.UnitPrice, ",", "."))
	if err != nil || price.IsNegative() {
		return p, errors.New("Gib einen gültigen Nettopreis ein, der nicht negativ ist.")
	}
	rate, err := decimal.NewFromString(strings.ReplaceAll(p.TaxRate, ",", "."))
	if err != nil || rate.IsNegative() {
		return p, errors.New("Gib einen gültigen Umsatzsteuersatz ein, der nicht negativ ist.")
	}
	if !strings.Contains("|S|Z|E|AE|K|G|O|", "|"+p.TaxCategory+"|") {
		return p, errors.New("Wähle eine unterstützte Steuerkategorie.")
	}
	if (p.TaxCategory == "S" && !rate.IsPositive()) || (p.TaxCategory != "S" && !rate.IsZero()) {
		return p, errors.New("Der Umsatzsteuersatz passt nicht zur gewählten Steuerkategorie.")
	}
	p.UnitPrice = price.String()
	p.TaxRate = rate.String()
	return s.store.SaveProduct(p)
}

func (s *InvoiceService) DeleteCustomer(id string) error { return s.store.DeleteCustomer(id) }

func (s *InvoiceService) DeleteProduct(id string) error { return s.store.DeleteProduct(id) }

// ExportBackup keeps all documents inline in one JSON text, so it only suits
// small data sets. The app saves backups through ExportSnapshot.
//
//wails:ignore
func (s *InvoiceService) ExportBackup() (string, error) {
	if _, err := s.PaymentBookings(); err != nil {
		return "", err
	}
	data, err := s.store.ExportBackup()
	if err != nil || s.mail == nil {
		return data, err
	}
	raw, err := s.withMailLog([]byte(data))
	return string(raw), err
}

// withMailLog adds mail settings and send log; the SMTP password stays in the
// keyring.
func (s *InvoiceService) withMailLog(data []byte) ([]byte, error) {
	var document map[string]json.RawMessage
	err := json.Unmarshal(data, &document)
	if err != nil {
		return nil, err
	}
	if document["mail"], err = s.mail.ExportBackup(); err != nil {
		return nil, err
	}
	return json.MarshalIndent(document, "", "  ")
}

//wails:ignore
func (s *InvoiceService) RestoreBackup(data string) error {
	// A restore must not interleave with a shop import and resurrect its cursor
	// or attach an imported order to an invoice from another database snapshot.
	s.shopMu.Lock()
	defer s.shopMu.Unlock()
	if err := s.store.RestoreBackup(data); err != nil || s.mail == nil {
		return err
	}
	return s.restoreMailLog([]byte(data))
}

func (s *InvoiceService) restoreMailLog(data []byte) error {
	var document map[string]json.RawMessage
	if err := json.Unmarshal(data, &document); err != nil {
		return err
	}
	return s.mail.RestoreBackup(document["mail"])
}

// ExportSnapshot reads one state; its documents stay in the store.
//
//wails:ignore
func (s *InvoiceService) ExportSnapshot() (*store.Snapshot, error) {
	if _, err := s.PaymentBookings(); err != nil {
		return nil, err
	}
	snapshot, err := s.store.ExportSnapshot()
	if err != nil || s.mail == nil {
		return snapshot, err
	}
	snapshot.Document, err = s.withMailLog(snapshot.Document)
	return snapshot, err
}

//wails:ignore
func (s *InvoiceService) ReadBlob(blob store.SnapshotBlob) ([]byte, error) {
	return s.store.ReadBlob(blob)
}

//wails:ignore
func (s *InvoiceService) RestoreSnapshot(document []byte, open store.BlobOpener, event store.BackupEvent) error {
	s.shopMu.Lock()
	defer s.shopMu.Unlock()
	if err := s.store.RestoreSnapshot(document, open, event); err != nil || s.mail == nil {
		return err
	}
	return s.restoreMailLog(document)
}

//wails:ignore
func (s *InvoiceService) RestoreImpact(document []byte) (store.RestoreImpact, error) {
	return s.store.RestoreImpact(document)
}

//wails:ignore
func (s *InvoiceService) AddBackupEvent(e store.BackupEvent) error { return s.store.AddBackupEvent(e) }

//wails:ignore
func (s *InvoiceService) BackupEvents() ([]store.BackupEvent, error) { return s.store.BackupEvents() }

// InspectXML is the shared backend implementation for raw XML inspection.
// The frontend uses InspectInvoiceFile so file type and size checks stay uniform.
//
//wails:ignore
func (s *InvoiceService) InspectXML(data string) (inspection model.ImportInspection, inspectionErr error) {
	defer func() { inspection.Validation = validator.German(inspection.Validation) }()
	const maxInvoiceSize = 20 << 20
	if len(data) == 0 {
		return model.ImportInspection{}, errors.New("Die XML-Datei ist leer. Wähle eine Rechnungsdatei aus.")
	}
	if len(data) > maxInvoiceSize {
		return model.ImportInspection{}, errors.New("Die XML-Datei ist zu groß. Dateien bis 20 MB werden unterstützt.")
	}
	result := model.ImportInspection{Validation: model.ValidationResult{State: "unsupported", Issues: []model.ValidationIssue{}}}
	if strings.Contains(strings.ToUpper(data), "<!DOCTYPE") {
		result.Validation = s.validator.Validate([]byte(data))
		return result, nil
	}
	inv, err := invoicexml.Parse([]byte(data))
	if err != nil {
		result.Validation = model.ValidationResult{State: "invalid", Issues: []model.ValidationIssue{{Stage: "syntax", RuleID: "PARSE", Severity: "error", Message: err.Error()}}}
		return result, nil
	}
	switch inv.SchemaType {
	case einvoice.CII:
		result.Syntax = "cii"
	case einvoice.UBL:
		result.Syntax = "ubl"
	default:
		result.Syntax = "unknown"
	}
	result.Profile = inv.GuidelineSpecifiedDocumentContextParameter
	result.InvoiceNumber = inv.InvoiceNumber
	if !inv.InvoiceDate.IsZero() {
		result.IssueDate = inv.InvoiceDate.Format("2006-01-02")
	}
	for _, term := range inv.SpecifiedTradePaymentTerms {
		if !term.DueDate.IsZero() {
			result.DueDate = term.DueDate.Format("2006-01-02")
			break
		}
	}
	for _, means := range inv.PaymentMeans {
		if means.PayeePartyCreditorFinancialAccountIBAN != "" {
			result.IBAN = means.PayeePartyCreditorFinancialAccountIBAN
			break
		}
	}
	result.SellerName = inv.Seller.Name
	result.BuyerName = inv.Buyer.Name
	result.Currency = inv.InvoiceCurrencyCode
	result.Totals = engine.Totals(inv)
	result.Validation = s.validator.ValidateImported([]byte(data))
	return result, nil
}

func (s *InvoiceService) view(d model.InvoiceDraft, includeXML bool) (model.InvoiceView, error) {
	d, numberErr := s.store.RefreshDraftNumber(d)
	if numberErr != nil {
		return model.InvoiceView{}, numberErr
	}
	c, err := s.store.Company()
	if err != nil {
		return model.InvoiceView{}, err
	}
	result := model.InvoiceView{Draft: d, Company: c, Validation: model.ValidationResult{State: "incomplete", Issues: []model.ValidationIssue{}}}
	inputIssues := append(validator.InputIssues(d, c), validator.InvoiceIssues(d, c)...)
	numberIssue, err := s.numberIssue(d)
	if err != nil {
		return model.InvoiceView{}, err
	}
	if numberIssue != nil {
		inputIssues = append(inputIssues, *numberIssue)
	}
	validationDraft := d
	if validationDraft.Number == "" {
		validationDraft.Number = "PREVIEW"
	}
	inv, err := s.engine.Build(validationDraft, c)
	if err != nil {
		result.Validation.Issues = inputIssues
		if len(inputIssues) == 0 {
			result.Validation = validator.German(model.ValidationResult{State: "incomplete", Issues: []model.ValidationIssue{{Stage: "input", RuleID: "INPUT", Severity: "error", Message: err.Error()}}})
		}
		return result, nil
	}
	result.Totals = engine.Totals(inv)
	result.LineTotals = engine.LineTotals(inv)
	result.PreviewHTML, _ = s.engine.Preview(d, c, inv)
	xmlData, err := s.engine.XML(inv)
	if err != nil {
		return result, err
	}
	result.Validation = s.validator.Validate(xmlData)
	// Preserve official results; add input hints for fields the schema did not reach.
	for _, input := range inputIssues {
		found := false
		for _, issue := range result.Validation.Issues {
			if issue.Field == input.Field {
				found = true
				break
			}
		}
		if !found {
			result.Validation.Issues = append(result.Validation.Issues, input)
		}
	}
	if len(inputIssues) > 0 {
		result.Validation.State = "invalid"
	}
	// Hints on the description of the supply never block; they show once the rest is valid.
	if result.Validation.State == "valid" {
		result.Validation.Issues = append(result.Validation.Issues, validator.AdviceIssues(d)...)
	}
	if includeXML {
		result.XML = string(xmlData)
	}
	return result, nil
}

func (s *InvoiceService) PreviewInvoice(d model.InvoiceDraft) (model.InvoiceView, error) {
	return s.view(d, false)
}

func (s *InvoiceService) ValidateInvoice(d model.InvoiceDraft) (model.InvoiceView, error) {
	return s.view(d, true)
}

func (s *InvoiceService) SaveInvoice(d model.InvoiceDraft) (model.InvoiceView, error) {
	d.IssuerSnapshot = nil
	d.CorrectionReason = model.NormalizeCorrectionReason(d)
	view, err := s.view(d, false)
	if err != nil {
		return view, err
	}
	saved, err := s.store.SaveInvoice(view.Draft, view.Totals.Gross)
	if err != nil {
		return view, err
	}
	view.Draft = saved
	return view, nil
}

func (s *InvoiceService) OpenInvoice(id string) (model.InvoiceView, error) {
	d, err := s.store.Invoice(id)
	if err != nil {
		return model.InvoiceView{}, err
	}
	if d.Status == "draft" {
		return s.view(d, false)
	}
	view := model.InvoiceView{Draft: d}
	view.Export, err = s.store.InvoiceExport(id)
	if err != nil {
		return view, err
	}
	artifact, err := s.store.Artifact(id, "cii-xml")
	if err != nil {
		return view, fmt.Errorf("Die gespeicherte Rechnungsdatei konnte nicht geladen werden: %w", err)
	}
	inv, err := invoicexml.Parse(artifact)
	if err != nil {
		return view, fmt.Errorf("Die gespeicherte Rechnungsdatei konnte nicht gelesen werden: %w", err)
	}
	// Issued documents must retain their original issuer as well as their buyer.
	p := inv.Seller
	c := model.Company{Name: p.Name, VATID: p.VATaxRegistration, TaxNumber: p.FCTaxRegistration,
		Endpoint: p.URIUniversalCommunication, EndpointScheme: p.URIUniversalCommunicationScheme,
		SmallBusiness: d.TaxTreatment == "small-business", ExemptionReason: d.TaxExemptionReason}
	if a := p.PostalAddress; a != nil {
		c.Address = model.Address{Street: a.Line1, Additional: a.Line2, Postcode: a.PostcodeCode, City: a.City, Country: a.CountryID}
	}
	if len(p.DefinedTradeContact) > 0 {
		contact := p.DefinedTradeContact[0]
		c.ContactName, c.Email, c.Phone = contact.PersonName, contact.EMail, contact.PhoneNumber
	}
	if len(inv.PaymentMeans) > 0 {
		payment := inv.PaymentMeans[0]
		c.IBAN, c.BIC, c.BankName = payment.PayeePartyCreditorFinancialAccountIBAN, payment.PayeeSpecifiedCreditorFinancialInstitutionBIC, payment.PayeePartyCreditorFinancialAccountName
		c.PaymentMeansCode = strconv.Itoa(payment.TypeCode)
		for _, payment := range inv.PaymentMeans[1:] {
			if payment.PayeePartyCreditorFinancialAccountIBAN != "" {
				c.AdditionalAccounts = append(c.AdditionalAccounts, model.BankAccount{IBAN: payment.PayeePartyCreditorFinancialAccountIBAN, BIC: payment.PayeeSpecifiedCreditorFinancialInstitutionBIC, Name: payment.PayeePartyCreditorFinancialAccountName})
			}
		}
	}
	view.Company = c
	if d.Profile == "facturx-basic" && d.IssuerSnapshot != nil {
		// All invoice amounts/identifiers still come from the immutable XML.
		// BASIC's optional contacts are retained by the presentation snapshot.
		c.ContactName, c.Email, c.Phone = d.IssuerSnapshot.ContactName, d.IssuerSnapshot.Email, d.IssuerSnapshot.Phone
		c.BIC, c.BankName = d.IssuerSnapshot.BIC, d.IssuerSnapshot.BankName
		for i := range c.AdditionalAccounts {
			for _, account := range d.IssuerSnapshot.BankAccounts() {
				if c.AdditionalAccounts[i].IBAN == model.CompactIdentifier(account.IBAN) {
					c.AdditionalAccounts[i].BIC, c.AdditionalAccounts[i].Name = account.BIC, account.Name
				}
			}
		}
		view.Company = c
	}
	view.Totals, view.LineTotals = engine.Totals(inv), engine.LineTotals(inv)
	view.PreviewHTML, err = s.engine.Preview(d, c, inv)
	if err != nil {
		return view, err
	}
	view.XML = string(artifact)
	view.Validation = s.validator.Validate(artifact)
	return view, nil
}

// ListInvoices supports native tests and internal workflows. The frontend uses
// the batched ApplicationLists response when it needs to synchronize its lists.
//
//wails:ignore
func (s *InvoiceService) ListInvoices() ([]model.InvoiceSummary, error) {
	return s.withMailStatus(s.store.Invoices())
}

// withMailStatus adds the latest successful e-mail delivery to each summary.
func (s *InvoiceService) withMailStatus(invoices []model.InvoiceSummary, err error) ([]model.InvoiceSummary, error) {
	if err != nil || s.mail == nil {
		return invoices, err
	}
	sent, err := s.mail.LastSent(mail.DocumentInvoice)
	if err != nil {
		return nil, err
	}
	for i := range invoices {
		if at := sent[invoices[i].ID]; at != "" {
			invoices[i].SentAt, invoices[i].SentVia = at, "mail"
		}
	}
	return invoices, nil
}

func (s *InvoiceService) DeleteDraft(id string) error { return s.store.DeleteDraft(id) }

func (s *InvoiceService) IssueInvoice(d model.InvoiceDraft) (model.InvoiceView, error) {
	// Bound methods may run concurrently. Keep the complete issuance workflow
	// single-flight in addition to the store transaction so duplicate UI calls do
	// not validate and render immutable artifacts at the same time.
	s.issueMu.Lock()
	defer s.issueMu.Unlock()
	d.CorrectionReason = model.NormalizeCorrectionReason(d)
	if d.ID == "" {
		view, err := s.SaveInvoice(d)
		if err != nil {
			return view, err
		}
		d = view.Draft
	}
	checked, err := s.view(d, true)
	if err != nil {
		return checked, err
	}
	if checked.Validation.State != "valid" {
		return checked, errors.New("Die Rechnung lässt sich noch nicht ausstellen. Behebe zuerst die angezeigten Hinweise.")
	}
	company, err := s.store.Company()
	if err != nil {
		return checked, err
	}
	// Never trust a snapshot supplied by the frontend. Capture actual settings
	// inside the issuance workflow before the artifacts and draft are committed.
	// Every profile keeps the snapshot so the printed logo stays reproducible;
	// only BASIC also uses its contact and bank fields for the presentation.
	d.IssuerSnapshot = &company
	issued, err := s.store.Issue(d.ID, d, checked.Totals.Gross, func(numbered model.InvoiceDraft) ([]byte, []byte, error) {
		inv, err := s.engine.Build(numbered, company)
		if err != nil {
			return nil, nil, err
		}
		xmlData, err := s.engine.XML(inv)
		if err != nil {
			return nil, nil, err
		}
		if result := s.validator.Validate(xmlData); result.State != "valid" {
			return nil, nil, errors.New("Die abschließende Prüfung ist fehlgeschlagen, deshalb wurde die Rechnung nicht ausgestellt.")
		}
		pdfData, err := s.renderInvoicePDF(xmlData, numbered)
		if err != nil {
			return nil, nil, err
		}
		return xmlData, pdfData, nil
	})
	if err != nil {
		return checked, err
	}
	view, err := s.OpenInvoice(issued.ID)
	if err != nil {
		return view, err
	}
	// Issuance is already committed. A filesystem failure must never suggest
	// that the user should issue the invoice a second time.
	view.Export, err = s.ExportIssuedInvoice(issued.ID)
	if err != nil && view.Export.Error == "" {
		view.Export.Error = err.Error()
	}
	return view, nil
}

func (s *InvoiceService) SetPaid(id string, paid bool) error { return s.store.SetPaid(id, paid) }

// ApplicationLists refreshes list data without constructing a new invoice draft.
func (s *InvoiceService) ApplicationLists() (model.ApplicationLists, error) {
	customers, err := s.store.Customers()
	if err != nil {
		return model.ApplicationLists{}, err
	}
	products, err := s.store.Products()
	if err != nil {
		return model.ApplicationLists{}, err
	}
	invoices, err := s.withMailStatus(s.store.Invoices())
	if err != nil {
		return model.ApplicationLists{}, err
	}
	return model.ApplicationLists{Customers: customers, Products: products, Invoices: invoices}, nil
}
