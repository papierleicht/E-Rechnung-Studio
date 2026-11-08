package store

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"papierleicht.de/e-rechnung-studio/internal/model"
	"strings"
	"time"
)

// Original bytes never change, including when the same file is imported again.
func (s *Store) SaveReceived(invoice model.ReceivedInvoice, original []byte) (model.ReceivedImport, error) {
	tx, err := s.db.Begin()
	if err != nil {
		return model.ReceivedImport{}, err
	}
	defer tx.Rollback()
	var exists int
	if err = tx.QueryRow(`SELECT COUNT(*) FROM received_invoices WHERE id=?`, invoice.ID).Scan(&exists); err != nil {
		return model.ReceivedImport{}, err
	}
	if exists == 0 {
		if invoice.ReceiptNumber, err = nextReceiptNumber(tx); err != nil {
			return model.ReceivedImport{}, err
		}
		raw, err := json.Marshal(invoice)
		if err != nil {
			return model.ReceivedImport{}, err
		}
		if _, err = tx.Exec(`INSERT INTO received_invoices(id,data,original) VALUES(?,?,?)`, invoice.ID, raw, original); err != nil {
			return model.ReceivedImport{}, err
		}
		summary := map[string]string{"number": invoice.ReceiptNumber, "filename": invoice.Filename, "invoiceNumber": invoice.Inspection.InvoiceNumber, "seller": invoice.Inspection.SellerName, "issueDate": invoice.Inspection.IssueDate}
		if err = s.logChange(tx, AuditEntry{Action: AuditCreated, Object: AuditReceived, ObjectID: invoice.ID, Label: receivedLabel(invoice), After: stateJSON(summary), Sum: receivedSum(invoice, original)}); err != nil {
			return model.ReceivedImport{}, err
		}
	}
	if err = tx.Commit(); err != nil {
		return model.ReceivedImport{}, err
	}
	stored, _, err := s.GetReceived(invoice.ID)
	return model.ReceivedImport{Invoice: stored, Duplicate: exists > 0}, err
}

func receivedLabel(v model.ReceivedInvoice) string {
	return strings.TrimSpace("Eingangsbeleg " + v.ReceiptNumber + " " + v.Filename)
}

// receivedLock uses the day of import for documents without a readable date.
func receivedLock(rules []lockRule, v model.ReceivedInvoice) string {
	date := v.Inspection.IssueDate
	if date == "" {
		date = v.ImportedAt[:min(10, len(v.ImportedAt))]
	}
	return lockedBy(rules, v.ImportedAt, date)
}

// linkedExpenses lists the receipts recorded for a document.
func linkedExpenses(q sqlRunner, id string) ([]Expense, error) {
	expenses, err := readExpenses(q)
	if err != nil {
		return nil, err
	}
	linked := []Expense{}
	for _, e := range expenses {
		if e.ReceivedID == id && e.CancelledAt == "" {
			linked = append(linked, e)
		}
	}
	return linked, nil
}

// refuseUsedReceived keeps a document that a receipt is based on.
func refuseUsedReceived(q sqlRunner, id string) error {
	linked, err := linkedExpenses(q, id)
	if err != nil {
		return err
	}
	if len(linked) > 0 {
		return fmt.Errorf("Zu diesem Dokument ist der Beleg %s erfasst. Storniere ihn zuerst.", linked[0].Number)
	}
	return nil
}

// refuseLockedReceived protects a received document in a closed period, also
// when only the receipt recorded for it lies in that period.
func refuseLockedReceived(q sqlRunner, id string) error {
	var raw []byte
	err := q.QueryRow(`SELECT data FROM received_invoices WHERE id=?`, id).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	var item model.ReceivedInvoice
	if err = json.Unmarshal(raw, &item); err != nil {
		return err
	}
	rules, err := lockRules(q)
	if err != nil {
		return err
	}
	if why := receivedLock(rules, item); why != "" {
		return fmt.Errorf("Dieser Beleg ist festgeschrieben. %s", why)
	}
	linked, err := linkedExpenses(q, id)
	if err != nil {
		return err
	}
	for _, e := range linked {
		if why := lockedBy(rules, e.CreatedAt, e.Date, e.PaidDate); why != "" {
			return fmt.Errorf("Dieser Beleg ist festgeschrieben. %s", why)
		}
	}
	return nil
}

func (s *Store) ListReceived() ([]model.ReceivedInvoice, error) {
	rows, err := s.db.Query(`SELECT data FROM received_invoices ORDER BY json_extract(data,'$.importedAt') DESC, id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []model.ReceivedInvoice{}
	for rows.Next() {
		var raw []byte
		if err := rows.Scan(&raw); err != nil {
			return nil, err
		}
		var item model.ReceivedInvoice
		if err := json.Unmarshal(raw, &item); err != nil {
			return nil, err
		}
		if item.Status == "" {
			item.Status = "active"
		}
		if s.scope.Load().covers(item.Inspection.IssueDate, item.ImportedAt[:min(10, len(item.ImportedAt))]) {
			items = append(items, item)
		}
	}
	if err = rows.Err(); err != nil {
		return nil, err
	}
	if err = rows.Close(); err != nil {
		return nil, err
	}
	rules, err := lockRules(s.db)
	if err != nil {
		return nil, err
	}
	for i := range items {
		items[i].Locked = receivedLock(rules, items[i])
	}
	return items, nil
}

func (s *Store) GetReceived(id string) (model.ReceivedInvoice, []byte, error) {
	var raw, original []byte
	var item model.ReceivedInvoice
	err := s.db.QueryRow(`SELECT data,original FROM received_invoices WHERE id=?`, id).Scan(&raw, &original)
	if err != nil {
		return item, nil, err
	}
	err = json.Unmarshal(raw, &item)
	if item.Status == "" {
		item.Status = "active"
	}
	return item, original, err
}

// ChangeReceivedStatus changes metadata only, preserving the original bytes.
func (s *Store) ChangeReceivedStatus(id, status, reason string) (model.ReceivedInvoice, error) {
	var item model.ReceivedInvoice
	reason = strings.TrimSpace(reason)
	if status != "active" && status != "archived" && status != "misimport" {
		return item, errors.New("Unbekannter Belegstatus.")
	}
	if status == "misimport" && reason == "" {
		return item, errors.New("Gib einen Grund für den Fehlimport an.")
	}
	if len(reason) > 2000 {
		return item, errors.New("Die Begründung darf höchstens 2000 Zeichen enthalten.")
	}
	tx, err := s.db.Begin()
	if err != nil {
		return item, err
	}
	defer tx.Rollback()
	var raw []byte
	if err = tx.QueryRow(`SELECT data FROM received_invoices WHERE id=?`, id).Scan(&raw); err != nil {
		return item, err
	}
	if err = json.Unmarshal(raw, &item); err != nil {
		return item, err
	}
	if item.Status == "" {
		item.Status = "active"
	}
	if item.Status == status {
		return item, errors.New("Der Beleg hat diesen Status bereits.")
	}
	if err = refuseLockedReceived(tx, id); err != nil {
		return item, err
	}
	previous := item.Status
	if status == "archived" && item.Status != "active" {
		return item, errors.New("Stelle den Fehlimport zuerst wieder her.")
	}
	if status == "misimport" {
		if err = refuseUsedReceived(tx, id); err != nil {
			return item, err
		}
		payments, e := readPayments(tx)
		if e != nil {
			return item, e
		}
		for _, p := range payments {
			if p.Direction == "incoming" && p.InvoiceID == id && p.RevokedAt == "" {
				return item, errors.New("Hebe zuerst die Zahlungszuordnungen dieses Belegs auf.")
			}
		}
	}
	item.Status = status
	item.History = append(item.History, model.ReceivedEvent{Action: status, Reason: reason, At: time.Now().UTC().Format(time.RFC3339Nano)})
	raw, err = json.Marshal(item)
	if err != nil {
		return item, err
	}
	if _, err = tx.Exec(`UPDATE received_invoices SET data=? WHERE id=?`, raw, id); err != nil {
		return item, err
	}
	original, err := readBlob(tx, sourceReceived+id)
	if err != nil {
		return item, err
	}
	if err = s.logChange(tx, AuditEntry{Action: AuditChanged, Object: AuditReceived, ObjectID: id, Label: receivedLabel(item), Reason: reason, Before: stateJSON(map[string]string{"status": previous}), After: stateJSON(map[string]string{"status": status}), Sum: receivedSum(item, original)}); err != nil {
		return item, err
	}
	return item, tx.Commit()
}

// Permanent deletion is limited to explicitly confirmed, classified misimports.
func (s *Store) DeleteReceivedMisimport(id, filename string, noRetentionRequired bool) error {
	if !noRetentionRequired {
		return errors.New("Bestätige, dass dieser Fehlimport nicht aufbewahrungspflichtig ist.")
	}
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	payments, err := readPayments(tx)
	if err != nil {
		return err
	}
	for _, p := range payments {
		if p.Direction == "incoming" && p.InvoiceID == id {
			return errors.New("Dieser Beleg gehört zu einem Zahlungsverlauf und muss erhalten bleiben.")
		}
	}
	if err = refuseLockedReceived(tx, id); err != nil {
		return err
	}
	if err = refuseUsedReceived(tx, id); err != nil {
		return err
	}
	var raw []byte
	err = tx.QueryRow(`SELECT data FROM received_invoices WHERE id=?`, id).Scan(&raw)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	result, err := tx.Exec(`DELETE FROM received_invoices WHERE id=? AND json_extract(data,'$.status')='misimport' AND json_extract(data,'$.filename')=?`, id, filename)
	if err != nil {
		return err
	}
	count, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if count != 1 {
		return errors.New("Löschen ist nicht möglich. Du kannst nur gekennzeichnete Fehlimporte löschen, und zwar mit exakt bestätigtem Dateinamen.")
	}
	// The document goes, the trace stays: what it was and that it was deleted.
	var item model.ReceivedInvoice
	if err = json.Unmarshal(raw, &item); err != nil {
		return err
	}
	summary := map[string]string{"number": item.ReceiptNumber, "filename": item.Filename}
	if err = s.logChange(tx, AuditEntry{Action: AuditDeleted, Object: AuditReceived, ObjectID: id, Label: receivedLabel(item), Reason: "Fehlimport, nach Bestätigung nicht aufbewahrungspflichtig", Before: stateJSON(summary), Sum: auditGone}); err != nil {
		return err
	}
	var choice []byte
	err = tx.QueryRow("SELECT value FROM settings WHERE key=?", inputTaxPrefix+id).Scan(&choice)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	if choice != nil {
		if _, err = tx.Exec("DELETE FROM settings WHERE key=?", inputTaxPrefix+id); err != nil {
			return err
		}
		if err = s.logChange(tx, AuditEntry{Action: AuditDeleted, Object: AuditInputTax, ObjectID: id, Label: inputTaxLabel, Before: string(choice), Sum: auditGone}); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// inspectionState is what a check found, without the data of the document.
func inspectionState(v model.ImportInspection) map[string]any {
	rules := []string{}
	for _, issue := range v.Validation.Issues {
		rules = append(rules, issue.RuleID)
	}
	return map[string]any{"state": v.Validation.State, "rules": rules}
}

// UpdateReceivedInspection stores a new check of a document. The original stays
// as it is. A result that differs keeps the earlier one in the change log and
// the history, so the check behind an earlier decision stays traceable.
func (s *Store) UpdateReceivedInspection(id string, inspection model.ImportInspection) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var raw, original []byte
	if err = tx.QueryRow(`SELECT data,original FROM received_invoices WHERE id=?`, id).Scan(&raw, &original); err != nil {
		return err
	}
	var item model.ReceivedInvoice
	if err = json.Unmarshal(raw, &item); err != nil {
		return err
	}
	before, after := stateJSON(inspectionState(item.Inspection)), stateJSON(inspectionState(inspection))
	updated := item
	updated.Inspection = inspection
	sum := receivedSum(updated, original)
	if before != after || sum != receivedSum(item, original) {
		earlier := "Vorheriges Ergebnis: " + item.Inspection.Validation.State
		if rules := inspectionState(item.Inspection)["rules"].([]string); len(rules) > 0 {
			earlier += " (" + strings.Join(rules, ", ") + ")"
		}
		item.History = append(item.History, model.ReceivedEvent{Action: "rechecked", Reason: earlier, At: time.Now().UTC().Format(time.RFC3339Nano)})
		if err = s.logChange(tx, AuditEntry{Action: AuditChanged, Object: AuditReceived, ObjectID: id, Label: receivedLabel(item), Reason: "Neu geprüft", Before: before, After: after, Sum: sum}); err != nil {
			return err
		}
	}
	item.Inspection = inspection
	if raw, err = json.Marshal(item); err != nil {
		return err
	}
	if _, err = tx.Exec(`UPDATE received_invoices SET data=? WHERE id=?`, raw, id); err != nil {
		return err
	}
	return tx.Commit()
}

type backupReceived struct {
	ID       string          `json:"id"`
	Data     json.RawMessage `json:"data"`
	Original []byte          `json:"original,omitempty"`
	// Blob is the SHA-256 of the original when a snapshot keeps it separately.
	Blob string `json:"blob,omitempty"`
}

func backupReceivedRows(q sqlRunner, c *blobCollector) ([]backupReceived, error) {
	rows, err := q.Query(`SELECT id,data,length(original) FROM received_invoices ORDER BY id`)
	if err != nil {
		return nil, err
	}
	items := []backupReceived{}
	var sizes []int64
	for rows.Next() {
		var item backupReceived
		var raw []byte
		var size int64
		if err := rows.Scan(&item.ID, &raw, &size); err != nil {
			rows.Close()
			return nil, err
		}
		item.Data = raw
		items, sizes = append(items, item), append(sizes, size)
	}
	if err = rows.Close(); err != nil {
		return nil, err
	}
	for i := range items {
		if c == nil {
			items[i].Original, err = readBlob(q, sourceReceived+items[i].ID)
		} else {
			items[i].Blob, err = c.reference(sourceReceived+items[i].ID, sizes[i])
		}
		if err != nil {
			return nil, err
		}
	}
	return items, nil
}
