package store

import (
	"bytes"
	"encoding/json"
	"errors"

	"papierleicht.de/e-rechnung-studio/internal/model"
)

// SetPurchasePrices records the purchase prices of items an issued invoice
// sold under the Differenzbesteuerung and the documents they come from, by
// line ID; empty values remove them. They are internal records (§ 25a Abs. 6 UStG): the invoice document
// stays unchanged. A period that is closed or whose return is filed keeps
// its figures, so the prices of its invoices no longer change.
func (s *Store) SetPurchasePrices(id string, prices map[string]model.PurchaseAssignment) error {
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
	before, after := map[string]model.PurchaseAssignment{}, map[string]model.PurchaseAssignment{}
	changed := false
	for _, l := range lines {
		line, ok := l.(map[string]any)
		if !ok {
			continue
		}
		lineID, _ := line["id"].(string)
		var old model.PurchaseAssignment
		old.Price, _ = line["purchasePrice"].(string)
		old.Source, _ = line["purchaseSource"].(string)
		old.SourceLabel, _ = line["purchaseSourceLabel"].(string)
		before[lineID], after[lineID] = old, old
		next, set := prices[lineID]
		if next.Source == "" {
			next.SourceLabel = ""
		}
		if !set || next == old {
			continue
		}
		changed = true
		after[lineID] = next
		for key, value := range map[string]string{"purchasePrice": next.Price, "purchaseSource": next.Source, "purchaseSourceLabel": next.SourceLabel} {
			if value == "" {
				delete(line, key)
			} else {
				line[key] = value
			}
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

// PurchaseSourceUses names, per linked document, the invoices whose lines
// take their purchase price from it: the number, or "Entwurf".
func (s *Store) PurchaseSourceUses() (map[string][]string, error) {
	rows, err := s.db.Query(`SELECT number,status,data FROM invoices`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	uses := map[string][]string{}
	for rows.Next() {
		var number, status string
		var raw []byte
		if err = rows.Scan(&number, &status, &raw); err != nil {
			return nil, err
		}
		var d struct {
			Lines []struct {
				PurchaseSource string `json:"purchaseSource"`
			} `json:"lines"`
		}
		if json.Unmarshal(raw, &d) != nil {
			continue
		}
		label := number
		if status == "draft" || label == "" {
			label = "Entwurf"
		}
		for _, l := range d.Lines {
			if l.PurchaseSource != "" {
				uses[l.PurchaseSource] = append(uses[l.PurchaseSource], label)
			}
		}
	}
	return uses, rows.Err()
}
