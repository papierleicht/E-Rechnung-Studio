package app

import (
	"archive/zip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"hash"
	"io"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"papierleicht.de/e-rechnung-studio/internal/einvoice"
	"papierleicht.de/e-rechnung-studio/internal/gdpdu"
	"papierleicht.de/e-rechnung-studio/internal/invoicexml"
	"papierleicht.de/e-rechnung-studio/internal/model"
	"papierleicht.de/e-rechnung-studio/internal/store"
	"papierleicht.de/e-rechnung-studio/internal/taxforms"
)

// A tax auditor may ask for the data of a period to evaluate it with their own
// software (Z3, GoBD Rz. 167, 176). The export is one unencrypted ZIP: the tax
// office's upload platforms reject encrypted and split archives. It holds CSV
// files with index.xml in the Beschreibungsstandard and every document in its
// original format. The same tables feed the data view for Z1 and Z2.

const (
	receiptKindExpense  = "Beleg"
	receiptKindEInvoice = "E-Rechnung"
	receiptKindPDF      = "PDF-Rechnung"
)

// Names of the tables, also used by the data view.
const (
	tableInvoices  = "ausgangsrechnungen"
	tableLines     = "ausgangspositionen"
	tableTaxes     = "ausgangssteuern"
	tableDeleted   = "geloeschte_rechnungen"
	tableReceipts  = "eingangsbelege"
	tablePayments  = "zahlungen"
	tableReminders = "zahlungserinnerungen"
	tableFilings   = "steuermeldungen"
	tableLocks     = "festschreibungen"
	tableLog       = "aenderungsprotokoll"
	tableAnchors   = "nachweise"
	tableCustomers = "kunden"
	tableFiles     = "dateien"
)

type issuedRecord struct {
	Draft   model.InvoiceDraft
	Invoice *einvoice.Invoice
}

// auditRecords is what belongs to a period.
type auditRecords struct {
	From, To   string
	Issued     []issuedRecord
	Deleted    []store.DeletedInvoice
	Expenses   []store.Expense
	Received   []model.ReceivedInvoice
	Categories map[string]store.ReceivedCategory
	Payments   []store.Payment
	Reminders  []store.Reminder
	Filings    [][]string
	Locks      []store.PeriodLock
	Log        []store.AuditEntry
}

func inRange(date, from, to string) bool { return date != "" && date >= from && date <= to }

func dayOf(stamp string) string { return stamp[:min(10, len(stamp))] }

func validPeriod(from, to string) error {
	_, e1 := time.Parse(time.DateOnly, from)
	_, e2 := time.Parse(time.DateOnly, to)
	if e1 != nil || e2 != nil || from > to {
		return errors.New("Gib den Prüfungszeitraum an.")
	}
	return nil
}

func (s *InvoiceService) auditRecords(from, to string) (auditRecords, error) {
	if err := validPeriod(from, to); err != nil {
		return auditRecords{}, err
	}
	// The read-only view shows its period, whatever dates are asked for.
	if viewFrom, viewTo, open := s.store.AuditView(); open {
		from, to = max(from, viewFrom), min(to, viewTo)
		if from > to {
			return auditRecords{}, errors.New("Dieser Zeitraum liegt außerhalb der Nur-Lese-Ansicht.")
		}
	}
	r := auditRecords{From: from, To: to}
	issued, err := s.store.IssuedDocuments()
	if err != nil {
		return r, err
	}
	seen := map[string]bool{}
	for _, d := range issued {
		if !inRange(d.Draft.IssueDate, from, to) {
			continue
		}
		rec := issuedRecord{Draft: d.Draft}
		if rec.Invoice, err = invoicexml.Parse(d.XML); err != nil {
			rec.Invoice = nil
		}
		r.Issued = append(r.Issued, rec)
		seen[d.Draft.ID] = true
	}
	sort.Slice(r.Issued, func(i, j int) bool {
		a, b := r.Issued[i].Draft, r.Issued[j].Draft
		return a.IssueDate+a.Number < b.IssueDate+b.Number
	})
	deleted, err := s.store.DeletedInvoices()
	if err != nil {
		return r, err
	}
	for _, d := range deleted {
		if inRange(d.IssueDate, from, to) {
			r.Deleted = append(r.Deleted, d)
			seen[d.ID] = true
		}
	}
	expenses, err := s.store.Expenses()
	if err != nil {
		return r, err
	}
	for i := len(expenses) - 1; i >= 0; i-- {
		e := expenses[i]
		if inRange(e.Date, from, to) || inRange(e.PaidDate, from, to) {
			r.Expenses = append(r.Expenses, e)
			seen[e.ID] = true
		}
	}
	received, err := s.store.ListReceived()
	if err != nil {
		return r, err
	}
	for _, v := range received {
		if inRange(v.Inspection.IssueDate, from, to) || inRange(dayOf(v.ImportedAt), from, to) {
			r.Received = append(r.Received, v)
			seen[v.ID] = true
		}
	}
	if r.Categories, err = s.store.ReceivedCategories(); err != nil {
		return r, err
	}
	overview, err := s.store.PaymentOverview()
	if err != nil {
		return r, err
	}
	for _, p := range overview.Payments {
		if inRange(p.Date, from, to) {
			r.Payments = append(r.Payments, p)
			seen[p.ID] = true
		}
	}
	reminders, err := s.store.Reminders("")
	if err != nil {
		return r, err
	}
	for _, v := range reminders {
		if inRange(v.Date, from, to) {
			r.Reminders = append(r.Reminders, v)
			seen[v.ID] = true
		}
	}
	for _, kind := range []struct{ key, name string }{{store.FilingVAT, "Umsatzsteuer-Voranmeldung"}, {store.FilingZM, "Zusammenfassende Meldung"}, {store.FilingAnnual, "Jahreserklärung"}} {
		filings, err := s.store.Filings(kind.key)
		if err != nil {
			return r, err
		}
		for id, f := range filings {
			start, end, label, ok := store.FilingPeriod(id)
			if ok && start <= to && end >= from {
				r.Filings = append(r.Filings, []string{kind.key + id, kind.name, label, gdpdu.Day(f.SubmittedOn), gdpdu.Amount(f.Payable), f.SavedAt})
				seen[kind.key+id] = true
			}
		}
	}
	sort.Slice(r.Filings, func(i, j int) bool { return r.Filings[i][0] < r.Filings[j][0] })
	if r.Locks, err = s.store.PeriodLocks(); err != nil {
		return r, err
	}
	log, err := s.store.AuditEntries(store.AuditFilter{})
	if err != nil {
		return r, err
	}
	for i := len(log) - 1; i >= 0; i-- {
		e := log[i]
		// Later changes to records of the period belong to the period.
		if inRange(dayOf(e.At), from, to) || seen[e.ObjectID] {
			r.Log = append(r.Log, e)
		}
	}
	return r, nil
}

func text(name, description string) gdpdu.Column {
	return gdpdu.Column{Name: name, Description: description, Kind: gdpdu.Text}
}

func number(name, description string, accuracy int) gdpdu.Column {
	return gdpdu.Column{Name: name, Description: description, Kind: gdpdu.Number, Accuracy: accuracy}
}

func date(name, description string) gdpdu.Column {
	return gdpdu.Column{Name: name, Description: description, Kind: gdpdu.Date}
}

// amount writes a decimal with two places for the file.
func amount(v fmt.Stringer) string {
	return gdpdu.Amount(v.String())
}

func decimalText(s string) string { return gdpdu.Amount(s) }

var kindLabels = map[string]string{"invoice": "Rechnung", "credit-note": "Gutschrift", "corrected-invoice": "Rechnungskorrektur"}

func kindLabel(kind string) string {
	if label, ok := kindLabels[kind]; ok {
		return label
	}
	return kind
}

var paidLabels = map[string]string{"bank": "Bank", "cash": "Bar", "private": "Privat"}

func categoryLabel(id string) string {
	if c, ok := taxforms.CategoryByID(id); ok {
		return c.Label
	}
	return id
}

// fileNames maps each document of the package to its place.
type fileNames map[string]auditFile

// tables turns the records into the files of the package. files is empty for
// the data view, which shows the same rows without reading any document.
func (r auditRecords) tables(files fileNames) []gdpdu.Table {
	path := func(key string) string { return files[key].Path }
	sum := func(key string) string { return files[key].Sum }

	invoices := gdpdu.Table{File: tableInvoices + ".csv", Name: "Ausgangsrechnungen", Description: "Ausgestellte Rechnungen und Gutschriften", From: r.From, To: r.To, Keys: 1,
		Columns: []gdpdu.Column{text("Rechnungs_ID", "Eindeutiger Schlüssel"), text("Rechnungsnummer", ""), text("Art", ""), text("Status", ""), date("Rechnungsdatum", ""), date("Leistungsdatum", ""), date("Faellig_am", ""),
			text("Kunden_ID", ""), text("Kunde", ""), text("USt_IdNr_Kunde", ""), text("Waehrung", ""), number("Netto", "", 2), number("Umsatzsteuer", "", 2), number("Brutto", "", 2),
			text("Ausgestellt_am", "Zeitpunkt des Ausstellens (UTC)"), text("Bezahlt_am", ""), text("Verschickt_am", ""), text("Korrektur_zu", "Rechnungs_ID der korrigierten Rechnung"), text("Korrekturgrund", ""),
			text("Datei_PDF", ""), text("SHA256_PDF", ""), text("Datei_XML", ""), text("SHA256_XML", "")}}
	lines := gdpdu.Table{File: tableLines + ".csv", Name: "Ausgangspositionen", Description: "Positionen der Ausgangsrechnungen", From: r.From, To: r.To, Keys: 2,
		Columns: []gdpdu.Column{text("Rechnungs_ID", ""), text("Position", ""), text("Bezeichnung", ""), text("Beschreibung", ""), number("Menge", "", 6), text("Einheit", ""), number("Einzelpreis_Netto", "", 6), text("Steuerkategorie", ""), number("Steuersatz", "", 2), number("Netto", "", 2)},
		Foreign: []gdpdu.ForeignKey{{Column: "Rechnungs_ID", Table: "Ausgangsrechnungen"}}}
	taxes := gdpdu.Table{File: tableTaxes + ".csv", Name: "Ausgangssteuern", Description: "Steueraufschlüsselung der Ausgangsrechnungen", From: r.From, To: r.To, Keys: 2,
		Columns: []gdpdu.Column{text("Rechnungs_ID", ""), text("Lfd_Nr", ""), text("Steuerkategorie", ""), number("Steuersatz", "", 2), number("Bemessungsgrundlage", "", 2), number("Umsatzsteuer", "", 2)},
		Foreign: []gdpdu.ForeignKey{{Column: "Rechnungs_ID", Table: "Ausgangsrechnungen"}}}
	customers := gdpdu.Table{File: tableCustomers + ".csv", Name: "Kunden", Description: "Kundendaten, wie sie auf den Rechnungen stehen", From: r.From, To: r.To, Keys: 2,
		Columns: []gdpdu.Column{text("Kunden_ID", ""), text("Name", ""), text("Strasse", ""), text("PLZ", ""), text("Ort", ""), text("Land", ""), text("USt_IdNr", "")}}
	seenCustomer := map[string]bool{}
	for _, rec := range r.Issued {
		d := rec.Draft
		net, tax, gross := "", "", ""
		if rec.Invoice != nil {
			net, tax, gross = amount(rec.Invoice.TaxBasisTotal), amount(rec.Invoice.TaxTotal), amount(rec.Invoice.GrandTotal)
		}
		status := "ausgestellt"
		if d.Status == "paid" {
			status = "bezahlt"
		}
		invoices.Rows = append(invoices.Rows, []string{d.ID, d.Number, kindLabel(d.Kind), status, gdpdu.Day(d.IssueDate), gdpdu.Day(firstNonEmpty(d.DeliveryDate, d.PeriodEnd)), gdpdu.Day(d.DueDate),
			d.CustomerID, d.Customer.Name, d.Customer.VATID, firstNonEmpty(d.Currency, "EUR"), net, tax, gross, d.IssuedAt, d.PaidAt, d.MarkedSentAt, d.CorrectsInvoiceID, d.CorrectionReason,
			path("invoice-pdf:" + d.ID), sum("invoice-pdf:" + d.ID), path("invoice-xml:" + d.ID), sum("invoice-xml:" + d.ID)})
		if rec.Invoice != nil {
			for i, l := range rec.Invoice.InvoiceLines {
				lines.Rows = append(lines.Rows, []string{d.ID, firstNonEmpty(l.LineID, strconv.Itoa(i+1)), l.ItemName, l.Description, amount(l.BilledQuantity), l.BilledQuantityUnit, amount(l.NetPrice), l.TaxCategoryCode, amount(l.TaxRateApplicablePercent), amount(l.Total)})
			}
			for i, t := range rec.Invoice.TradeTaxes {
				taxes.Rows = append(taxes.Rows, []string{d.ID, strconv.Itoa(i + 1), t.CategoryCode, amount(t.Percent), amount(t.BasisAmount), amount(t.CalculatedAmount)})
			}
		}
		key := d.CustomerID + "\x00" + d.Customer.Name
		if !seenCustomer[key] {
			seenCustomer[key] = true
			a := d.Customer.Address
			customers.Rows = append(customers.Rows, []string{d.CustomerID, d.Customer.Name, strings.TrimSpace(a.Street + " " + a.Additional), a.Postcode, a.City, a.Country, d.Customer.VATID})
		}
	}

	deleted := gdpdu.Table{File: tableDeleted + ".csv", Name: "Geloeschte_Rechnungen", Description: "Rechnungen, die vor dem Versand gelöscht wurden; ihre Nummern bleiben gesperrt", From: r.From, To: r.To, Keys: 1,
		Columns: []gdpdu.Column{text("Rechnungs_ID", ""), text("Rechnungsnummer", ""), text("Art", ""), date("Rechnungsdatum", ""), text("Kunde", ""), text("Waehrung", ""), number("Brutto", "", 2), text("Ausgestellt_am", ""), text("Geloescht_am", ""), text("Grund", ""),
			text("Datei_PDF", ""), text("SHA256_PDF", ""), text("Datei_XML", ""), text("SHA256_XML", "")}}
	for _, d := range r.Deleted {
		deleted.Rows = append(deleted.Rows, []string{d.ID, d.Number, kindLabel(d.Kind), gdpdu.Day(d.IssueDate), d.CustomerName, firstNonEmpty(d.Currency, "EUR"), decimalText(d.Gross), d.IssuedAt, d.DeletedAt, d.Reason,
			path("deleted-pdf:" + d.ID), sum("deleted-pdf:" + d.ID), path("deleted-xml:" + d.ID), sum("deleted-xml:" + d.ID)})
	}

	receipts := gdpdu.Table{File: tableReceipts + ".csv", Name: "Eingangsbelege", Description: "Erfasste Ausgaben und eingegangene Rechnungen", From: r.From, To: r.To, Keys: 1,
		Columns: []gdpdu.Column{text("Beleg_ID", ""), text("Belegnummer", "Von der App vergeben, einmalig"), text("Fremdbelegnummer", "Nummer auf dem Beleg"), text("Art", ""), text("Status", ""), date("Belegdatum", ""), date("Zahlungsdatum", ""), text("Lieferant", ""),
			text("Kategorie", ""), text("Beschreibung", ""), text("Waehrung", ""), number("Betrag_Originalwaehrung", "", 4), number("Wechselkurs", "Fremdwährung je Euro", 6), number("Brutto_EUR", "", 2), number("Netto_EUR", "", 2), number("Vorsteuer_EUR", "", 2), text("Steuersatz", ""), text("Zahlart", ""),
			text("Eingangsdokument", "Beleg_ID des zugehörigen Eingangsdokuments"), text("Erfasst_am", ""), text("Geaendert_am", ""), text("Storniert_am", ""), text("Stornogrund", ""), text("Datei", ""), text("SHA256", "")}}
	for _, e := range r.Expenses {
		status := "aktiv"
		if e.CancelledAt != "" {
			status = "storniert"
		}
		receipts.Rows = append(receipts.Rows, []string{e.ID, e.Number, e.DocumentNumber, receiptKindExpense, status, gdpdu.Day(e.Date), gdpdu.Day(e.PaidDate), e.Supplier, categoryLabel(e.Category), e.Note,
			firstNonEmpty(e.Currency, "EUR"), decimalText(e.OriginalGross), decimalText(e.ExchangeRate), decimalText(e.Gross), decimalText(e.Net), decimalText(e.Tax), e.TaxRate, paidLabels[e.PaidWith],
			e.ReceivedID, e.CreatedAt, e.UpdatedAt, e.CancelledAt, e.CancelReason, path("expense:" + e.ID + ":" + e.FileSum), e.FileSum})
	}
	for _, v := range r.Received {
		kind := receiptKindPDF
		if v.Inspection.Syntax != "" && v.Inspection.Syntax != "unknown" {
			kind = receiptKindEInvoice
		}
		status := map[string]string{"": "aktiv", "active": "aktiv", "archived": "archiviert", "misimport": "Fehlimport"}[v.Status]
		totals := v.Inspection.Totals
		receipts.Rows = append(receipts.Rows, []string{v.ID, v.ReceiptNumber, v.Inspection.InvoiceNumber, kind, status, gdpdu.Day(v.Inspection.IssueDate), "", v.Inspection.SellerName, categoryLabel(r.Categories[v.ID].Category), v.Filename,
			firstNonEmpty(v.Inspection.Currency, "EUR"), "", "", decimalText(totals.Gross), decimalText(totals.Net), decimalText(totals.Tax), "", "", "", v.ImportedAt, "", "", "", path("received:" + v.ID), sum("received:" + v.ID)})
	}

	payments := gdpdu.Table{File: tablePayments + ".csv", Name: "Zahlungen", Description: "Zugeordnete Zahlungen. Korrekturen sind Aufhebungen, keine Änderungen.", From: r.From, To: r.To, Keys: 1,
		Columns: []gdpdu.Column{text("Zahlungs_ID", ""), text("Beleg_oder_Rechnungs_ID", ""), text("Richtung", ""), number("Betrag", "", 2), text("Waehrung", ""), date("Zahlungsdatum", ""), text("Quelle", ""), text("Bankumsatz_ID", ""), text("Grund", ""), text("Erfasst_am", ""), text("Aufgehoben_am", ""), text("Aufhebungsgrund", "")}}
	for _, p := range r.Payments {
		direction := "Eingang"
		if p.Direction == "incoming" {
			direction = "Ausgang"
		}
		payments.Rows = append(payments.Rows, []string{p.ID, p.InvoiceID, direction, decimalText(p.Amount), p.Currency, gdpdu.Day(p.Date), p.Source, p.BookingID, p.Reason, p.CreatedAt, p.RevokedAt, p.RevocationReason})
	}

	reminders := gdpdu.Table{File: tableReminders + ".csv", Name: "Zahlungserinnerungen", Description: "Zahlungserinnerungen und Mahnungen", From: r.From, To: r.To, Keys: 1,
		Columns: []gdpdu.Column{text("Mahnungs_ID", ""), text("Rechnungs_ID", ""), text("Rechnungsnummer", ""), number("Stufe", "", 0), date("Datum", ""), number("Offener_Betrag", "", 2), number("Gebuehr", "", 2), number("Zinsen", "", 2), number("Gesamt", "", 2), text("Waehrung", ""), text("Versandweg", ""), text("Erfasst_am", ""), text("Datei", ""), text("SHA256", "")}}
	for _, m := range r.Reminders {
		reminders.Rows = append(reminders.Rows, []string{m.ID, m.InvoiceID, m.Number, strconv.Itoa(m.Level), gdpdu.Day(m.Date), decimalText(m.Amount), decimalText(m.Fee), decimalText(m.Interest), decimalText(m.Total), m.Currency, m.Channel, m.CreatedAt, path("reminder:" + m.ID), m.DocumentSHA256})
	}

	filings := gdpdu.Table{File: tableFilings + ".csv", Name: "Steuermeldungen", Description: "Vermerke über übermittelte Steuermeldungen", From: r.From, To: r.To, Keys: 1,
		Columns: []gdpdu.Column{text("Vermerk_ID", ""), text("Art", ""), text("Zeitraum", ""), date("Uebermittelt_am", ""), number("Betrag", "", 2), text("Vermerkt_am", "")}, Rows: r.Filings}

	locks := gdpdu.Table{File: tableLocks + ".csv", Name: "Festschreibungen", Description: "Von Hand festgeschriebene Zeiträume. Übermittelte Steuermeldungen schreiben ihren Zeitraum zusätzlich fest.", Keys: 1,
		Columns: []gdpdu.Column{text("Festschreibung_ID", ""), date("Von", ""), date("Bis", ""), text("Festgeschrieben_am", "")}}
	for _, l := range r.Locks {
		locks.Rows = append(locks.Rows, []string{l.ID, gdpdu.Day(l.From), gdpdu.Day(l.To), l.At})
	}

	log := gdpdu.Table{File: tableLog + ".csv", Name: "Aenderungsprotokoll", Description: "Alle Änderungen an steuerlich relevanten Daten. Jeder Eintrag enthält den Hash seines Vorgängers.", From: r.From, To: r.To, Keys: 1,
		Columns: []gdpdu.Column{number("Lfd_Nr", "", 0), text("Zeitpunkt", "UTC"), text("Aktion", ""), text("Objektart", ""), text("Objekt_ID", ""), text("Bezeichnung", ""), text("Grund", ""), text("Inhalt_vorher", "JSON"), text("Inhalt_nachher", "JSON"), text("Inhalts_Hash", ""), text("Programmversion", ""), text("Hash_Vorgaenger", ""), text("Hash", "")}}
	for _, e := range r.Log {
		before := e.Before
		// A restore note can hold the log of another history, not part of this period.
		if e.Object == store.AuditBackup {
			before = ""
		}
		log.Rows = append(log.Rows, []string{strconv.FormatInt(e.Seq, 10), e.At, e.Action, e.Object, e.ObjectID, e.Label, e.Reason, before, e.After, e.Sum, e.Version, e.Previous, e.Hash})
	}

	return []gdpdu.Table{invoices, lines, taxes, customers, deleted, receipts, payments, reminders, filings, locks, log}
}

// auditFile is one document of the package.
type auditFile struct {
	Key, Path, Kind, Owner string
	Sum                    string
	Size                   int64
	Read                   func() ([]byte, error)
	// AddExtension completes the path once the content is known.
	AddExtension bool
}

var unsafeName = regexp.MustCompile(`[^A-Za-z0-9._-]+`)

func safeName(s string) string {
	s = strings.Trim(unsafeName.ReplaceAllString(s, "_"), "_.")
	if s == "" {
		return "ohne-nummer"
	}
	return s[:min(80, len(s))]
}

func short(id string) string { return safeName(id[max(0, len(id)-8):]) }

// auditDocuments lists every file of the period. Nothing is read yet, because
// the documents of several years do not fit into memory.
func (s *InvoiceService) auditDocuments(r auditRecords) ([]*auditFile, error) {
	files := []*auditFile{}
	for _, rec := range r.Issued {
		id, name := rec.Draft.ID, safeName(rec.Draft.Number)
		for _, f := range []struct{ key, kind, ext string }{{"invoice-pdf:", "facturx-pdf", ".pdf"}, {"invoice-xml:", "cii-xml", ".xml"}} {
			kind := f.kind
			files = append(files, &auditFile{Key: f.key + id, Path: "belege/ausgang/" + name + f.ext, Kind: "Ausgangsrechnung", Owner: id, Read: func() ([]byte, error) { return s.store.Artifact(id, kind) }})
		}
	}
	for _, d := range r.Deleted {
		id, name := d.ID, safeName(d.Number)
		files = append(files,
			&auditFile{Key: "deleted-pdf:" + id, Path: "belege/geloescht/" + name + ".pdf", Kind: "Gelöschte Rechnung", Owner: id, Read: func() ([]byte, error) {
				_, _, _, pdf, err := s.store.DeletedInvoice(id)
				return pdf, err
			}},
			&auditFile{Key: "deleted-xml:" + id, Path: "belege/geloescht/" + name + ".xml", Kind: "Gelöschte Rechnung", Owner: id, Read: func() ([]byte, error) {
				_, _, xml, _, err := s.store.DeletedInvoice(id)
				return xml, err
			}})
	}
	for _, e := range r.Expenses {
		versions, err := s.store.ExpenseFiles(e.ID)
		if err != nil {
			return nil, err
		}
		for _, v := range versions {
			id, sum := e.ID, v.Sum
			kind := "Belegdatei"
			if sum != e.FileSum {
				kind = "Ersetzte Belegdatei"
			}
			files = append(files, &auditFile{Key: "expense:" + id + ":" + sum, Path: "belege/ausgaben/" + safeName(e.Number) + "_" + sum[:8], Kind: kind, Owner: id, AddExtension: true,
				Read: func() ([]byte, error) { return s.store.ReadExpenseFile(id, sum) }})
		}
	}
	for _, v := range r.Received {
		id, ext := v.ID, "."+safeName(v.Kind)
		files = append(files, &auditFile{Key: "received:" + id, Path: "belege/eingang/" + safeName(v.ReceiptNumber) + "_" + short(id) + ext, Kind: "Eingangsdokument", Owner: id, Read: func() ([]byte, error) {
			_, original, err := s.store.GetReceived(id)
			return original, err
		}})
	}
	for _, m := range r.Reminders {
		id := m.ID
		files = append(files, &auditFile{Key: "reminder:" + id, Path: fmt.Sprintf("belege/mahnungen/%s_%d_%s.pdf", safeName(m.Number), m.Level, short(id)), Kind: "Zahlungserinnerung", Owner: m.InvoiceID, Read: func() ([]byte, error) {
			_, pdf, err := s.store.ReminderPDF(id)
			return pdf, err
		}})
	}
	return files, nil
}

// AuditExportResult describes a written package.
type AuditExportResult struct {
	Path  string `json:"path"`
	Files int    `json:"files"`
	Bytes int64  `json:"bytes"`
	// Sum is the SHA-256 of the archive; note it for the handover.
	Sum string `json:"sum"`
}

type countingWriter struct {
	w io.Writer
	h hash.Hash
	n int64
}

func (c *countingWriter) Write(p []byte) (int, error) {
	n, err := c.w.Write(p)
	c.h.Write(p[:n])
	c.n += int64(n)
	return n, err
}

// writeAuditPackage writes the ZIP next to target and moves it there when it is
// complete, so a failed export never leaves half a package behind.
func (s *InvoiceService) writeAuditPackage(target, from, to string, procedure, report []byte) (AuditExportResult, error) {
	result := AuditExportResult{Path: target}
	s.exportMu.Lock()
	defer s.exportMu.Unlock()
	records, err := s.auditRecords(from, to)
	if err != nil {
		return result, err
	}
	documents, err := s.auditDocuments(records)
	if err != nil {
		return result, err
	}
	tmp := target + ".tmp"
	file, err := os.OpenFile(tmp, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return result, err
	}
	defer func() {
		file.Close()
		if err != nil {
			os.Remove(tmp)
		}
	}()
	out := &countingWriter{w: file, h: sha256.New()}
	zw := zip.NewWriter(out)
	store := func(name string, data []byte) error {
		w, e := zw.CreateHeader(&zip.FileHeader{Name: name, Method: zip.Store, Modified: time.Now()})
		if e != nil {
			return e
		}
		_, e = w.Write(data)
		return e
	}
	compress := func(name string, data []byte) error {
		w, e := zw.CreateHeader(&zip.FileHeader{Name: name, Method: zip.Deflate, Modified: time.Now()})
		if e != nil {
			return e
		}
		_, e = w.Write(data)
		return e
	}
	names := fileNames{}
	used := map[string]bool{}
	files := gdpdu.Table{File: tableFiles + ".csv", Name: "Dateien", Description: "Alle Dokumente dieser Datenüberlassung mit Prüfsumme", Keys: 1,
		Columns: []gdpdu.Column{text("Datei", ""), text("Art", ""), text("Beleg_oder_Rechnungs_ID", ""), text("SHA256", ""), number("Groesse_Bytes", "", 0)}}
	for _, d := range documents {
		data, readErr := d.Read()
		if readErr != nil {
			err = fmt.Errorf("Das Dokument %s ließ sich nicht lesen: %w", d.Path, readErr)
			return result, err
		}
		if d.AddExtension {
			d.Path += documentExtension(data)
		}
		if used[d.Path] {
			d.Path = strings.Replace(d.Path, ".", "_"+short(d.Owner)+".", 1)
		}
		used[d.Path] = true
		sum := sha256.Sum256(data)
		d.Sum, d.Size = hex.EncodeToString(sum[:]), int64(len(data))
		if err = store(d.Path, data); err != nil {
			return result, err
		}
		names[d.Key] = *d
		files.Rows = append(files.Rows, []string{d.Path, d.Kind, d.Owner, d.Sum, strconv.FormatInt(d.Size, 10)})
		result.Files++
	}
	tables := append(records.tables(names), files)
	anchors, err := s.store.AuditAnchors()
	if err != nil {
		return result, err
	}
	receipts := records.anchorTable(anchors)
	if len(receipts.Rows) > 0 {
		tables = append(tables, receipts)
	}
	for _, t := range tables {
		if err = compress(t.File, t.CSV()); err != nil {
			return result, err
		}
	}
	company, err := s.store.Company()
	if err != nil {
		return result, err
	}
	location := strings.TrimSpace(company.Address.Postcode + " " + company.Address.City)
	index, err := gdpdu.Index(gdpdu.Supplier{Name: company.Name, Location: location, Comment: fmt.Sprintf("Datenüberlassung nach GoBD, E-Rechnung-Studio %s", currentVersion)}, "Datenüberlassung "+gdpdu.Day(from)+" bis "+gdpdu.Day(to), tables)
	if err != nil {
		return result, err
	}
	if err = compress("index.xml", index); err != nil {
		return result, err
	}
	if err = compress(gdpdu.DTDName, gdpdu.DTD); err != nil {
		return result, err
	}
	if len(procedure) > 0 {
		if err = store("verfahrensdokumentation.pdf", procedure); err != nil {
			return result, err
		}
	}
	if len(report) > 0 {
		if err = store("nachweisbericht.pdf", report); err != nil {
			return result, err
		}
	}
	seq, head, err := s.store.AuditHead()
	if err != nil {
		return result, err
	}
	readme := fmt.Sprintf(auditReadme, company.Name, gdpdu.Day(from), gdpdu.Day(to), currentVersion, time.Now().Format("02.01.2006 15:04"), result.Files, seq, head)
	if len(receipts.Rows) > 0 {
		readme += fmt.Sprintf(auditReadmeReceipts, licensePublicKey)
	}
	if err = compress("LIESMICH.txt", []byte(readme)); err != nil {
		return result, err
	}
	if err = zw.Close(); err != nil {
		return result, err
	}
	if err = file.Sync(); err != nil {
		return result, err
	}
	if err = file.Close(); err != nil {
		return result, err
	}
	if err = os.Rename(tmp, target); err != nil {
		return result, err
	}
	result.Sum, result.Bytes = hex.EncodeToString(out.h.Sum(nil)), out.n
	summary, _ := json.Marshal(map[string]any{"from": from, "to": to, "files": result.Files, "bytes": result.Bytes, "sha256": result.Sum})
	err = s.store.LogExport(fmt.Sprintf("Prüfer-Export %s bis %s", gdpdu.Day(from), gdpdu.Day(to)), string(summary), result.Sum)
	return result, err
}

const auditReadme = `Datenüberlassung für die Außenprüfung (GoBD Rz. 167)
====================================================

Unternehmen:      %s
Zeitraum:         %s bis %s
Programm:         E-Rechnung-Studio %s
Erstellt:         %s
Dokumente:        %d

index.xml         Beschreibung aller CSV-Dateien nach dem Beschreibungsstandard.
                  Die Prüfsoftware liest die Daten über diese Datei ein.
*.csv             Die Daten: UTF-8, Semikolon, Dezimalkomma, Datum TT.MM.JJJJ,
                  erste Zeile mit den Feldnamen.
belege/           Alle Dokumente im Originalformat. dateien.csv nennt zu jedem
                  Dokument die SHA-256-Prüfsumme und den zugehörigen Beleg.
aenderungsprotokoll.csv
                  Alle Änderungen zu diesen Daten mit dem Inhalt davor und
                  danach. Jeder Eintrag enthält den Hash seines Vorgängers.

Stand des Änderungsprotokolls beim Export:
Eintrag %d, Hash %s

Das Archiv ist unverschlüsselt, weil die Datenaustauschplattformen der
Finanzverwaltung verschlüsselte Archive abweisen. Bei einer Übergabe auf einem
Datenträger gehört es auf einen verschlüsselten Datenträger.
`

const auditReadmeReceipts = `
nachweise.csv     Bestätigungen des Anbieters, dass das Änderungsprotokoll zum
                  genannten Zeitpunkt bis zu dieser laufenden Nummer reichte
                  und mit diesem Hash endete. Der Hash muss zur Zeile mit
                  derselben Nummer in aenderungsprotokoll.csv passen. Ein
                  später neu berechnetes Protokoll passt nicht mehr dazu.
                  Der Nachweis ist mit Ed25519 signiert: Signiert ist der Teil
                  vor dem Punkt, dahinter steht die Signatur.
                  Öffentlicher Schlüssel (Base64): %s
nachweisbericht.pdf
                  Die Nachweise in lesbarer Form mit dem Ergebnis der Prüfung.
                  Prüfen lassen sie sich unter https://papierleicht.de/nachweis
                  im Browser, ohne dass Daten übertragen werden.
`

// anchorTable lists the server receipts for log entries of this period. An
// auditor can only match a receipt against an entry that is in the package.
func (r auditRecords) anchorTable(anchors []store.AuditAnchor) gdpdu.Table {
	t := gdpdu.Table{File: tableAnchors + ".csv", Name: "Nachweise", Description: "Signierte Bestätigungen des Anbieters zum Stand des Änderungsprotokolls", Keys: 1,
		Columns: []gdpdu.Column{number("Lfd_Nr", "", 0), text("Hash", ""), text("Bestaetigt_am", "UTC"), text("Nachweis", "")}}
	inPeriod := map[int64]bool{}
	for _, e := range r.Log {
		inPeriod[e.Seq] = true
	}
	for _, a := range anchors {
		if inPeriod[a.Seq] {
			t.Rows = append(t.Rows, []string{strconv.FormatInt(a.Seq, 10), a.Hash, a.At, a.Receipt})
		}
	}
	return t
}

// auditView is a table for the data view.
type auditView struct {
	Name    string     `json:"name"`
	Title   string     `json:"title"`
	Columns []string   `json:"columns"`
	Numeric []bool     `json:"numeric"`
	Rows    [][]string `json:"rows"`
}

func viewOf(t gdpdu.Table) auditView {
	v := auditView{Name: strings.TrimSuffix(t.File, ".csv"), Title: strings.ReplaceAll(t.Name, "_", " "), Rows: t.Rows}
	if v.Rows == nil {
		v.Rows = [][]string{}
	}
	for _, c := range t.Columns {
		v.Columns = append(v.Columns, c.Name)
		v.Numeric = append(v.Numeric, c.Kind == gdpdu.Number)
	}
	return v
}
