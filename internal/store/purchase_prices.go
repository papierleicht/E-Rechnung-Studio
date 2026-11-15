package store

import (
	"bytes"
	"encoding/json"
	"errors"
)

// SetPurchasePrices records the purchase prices of items an issued invoice
// sold under the Differenzbesteuerung, by line ID; an empty price removes
// one. They are internal records (§ 25a Abs. 6 UStG): the invoice document
// stays unchanged. A period that is closed or whose return is filed keeps
// its figures, so the prices of its invoices no longer change.
func (s *Store) SetPurchasePrices(id string, prices map[string]string) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var raw []byte
	var status string
	if err = tx.QueryRow(`SELECT data,status FROM invoices WHERE id=?`, id).Scan(&raw, &status); err != nil {
		return errors.New("Die Rechnung wurde nicht gefunden.")
	}
	if status == "draft" {
		return errors.New("Einkaufspreise eines Entwurfs trägst du in den Positionen ein.")
	}
	// Numbers stay as written: only the purchase prices change.
	var d map[string]any
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	if err = dec.Decode(&d); err != nil {
		return err
	}
	lines, _ := d["lines"].([]any)
	before, after := map[string]string{}, map[string]string{}
	changed := false
	for _, l := range lines {
		line, ok := l.(map[string]any)
		if !ok {
			continue
		}
		lineID, _ := line["id"].(string)
		old, _ := line["purchasePrice"].(string)
		before[lineID] = old
		after[lineID] = old
		price, set := prices[lineID]
		if !set || price == old {
			continue
		}
		changed = true
		after[lineID] = price
		if price == "" {
			delete(line, "purchasePrice")
		} else {
			line["purchasePrice"] = price
		}
	}
	if !changed {
		return nil
	}
	var draft struct {
		Number       string `json:"number"`
		IssueDate    string `json:"issueDate"`
		DeliveryDate string `json:"deliveryDate"`
		PeriodEnd    string `json:"periodEnd"`
	}
	_ = json.Unmarshal(raw, &draft)
	rules, err := lockRules(tx)
	if err != nil {
		return err
	}
	taxDate := draft.DeliveryDate
	if taxDate == "" {
		taxDate = draft.PeriodEnd
	}
	if taxDate == "" {
		taxDate = draft.IssueDate
	}
	if why := lockedBy(rules, "", taxDate, draft.IssueDate); why != "" {
		return errors.New(why + " Einkaufspreise von Rechnungen dieses Zeitraums lassen sich nicht mehr ändern.")
	}
	updated, err := json.Marshal(d)
	if err != nil {
		return err
	}
	if _, err = tx.Exec(`UPDATE invoices SET data=? WHERE id=?`, updated, id); err != nil {
		return err
	}
	if err = s.logChange(tx, AuditEntry{Action: AuditChanged, Object: AuditInvoice, ObjectID: id, Label: "Rechnung " + draft.Number + ": Einkaufspreise",
		Before: stateJSON(before), After: stateJSON(after)}); err != nil {
		return err
	}
	return tx.Commit()
}
