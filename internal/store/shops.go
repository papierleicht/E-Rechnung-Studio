package store

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"reflect"
	"strings"
	"time"

	"github.com/shopspring/decimal"
	"papierleicht.de/e-rechnung-studio/internal/model"
	"papierleicht.de/e-rechnung-studio/internal/shops"
)

const shopInvoiceMissingReason = "Die zugehörige Rechnung wurde gelöscht. Prüf die Bestellung; eine weitere Rechnung erstellt die App dafür nicht automatisch."
const shopOrderChangedReason = "Die Bestellung wurde nach der Rechnungserstellung im Shop geändert. Prüf die zugehörige Rechnung."

// markShopInvoiceMissing keeps the source link of a deleted invoice as a
// tombstone, so synchronizing cannot recreate an invoice the user deleted.
func markShopInvoiceMissing(tx *sql.Tx, invoiceID string) error {
	_, err := tx.Exec(`UPDATE shop_orders SET data=json_set(data,'$.reviewReason',?) WHERE invoice_id=?`, shopInvoiceMissingReason, invoiceID)
	return err
}

func (s *Store) ShopConnections() ([]shops.Connection, error) { return shopConnections(s.db) }

func shopConnections(q rowQuerier) ([]shops.Connection, error) {
	rows, err := q.Query(`SELECT data FROM shop_connections ORDER BY provider,url,id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []shops.Connection{}
	for rows.Next() {
		var raw []byte
		var connection shops.Connection
		if err = rows.Scan(&raw); err != nil {
			return nil, err
		}
		if err = json.Unmarshal(raw, &connection); err != nil {
			return nil, err
		}
		connection.HasCredentials = false
		out = append(out, connection)
	}
	return out, rows.Err()
}

func (s *Store) ShopConnection(id string) (shops.Connection, error) {
	var raw []byte
	var connection shops.Connection
	if err := s.db.QueryRow(`SELECT data FROM shop_connections WHERE id=?`, id).Scan(&raw); err != nil {
		return connection, err
	}
	err := json.Unmarshal(raw, &connection)
	connection.HasCredentials = false
	return connection, err
}

func validateShopConnection(connection shops.Connection) error {
	if strings.TrimSpace(connection.ID) == "" || strings.TrimSpace(connection.Provider) == "" || strings.TrimSpace(connection.URL) == "" {
		return errors.New("Die Shop-Verbindung ist unvollständig.")
	}
	parsed, err := url.Parse(connection.URL)
	if err != nil || parsed.Host == "" || parsed.Scheme != "https" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return errors.New("Die Shop-Adresse muss eine HTTPS-Adresse ohne Zugangsdaten, Abfrageparameter oder Fragment sein.")
	}
	return nil
}

func writeShopConnection(tx *sql.Tx, connection shops.Connection, update bool) error {
	if err := validateShopConnection(connection); err != nil {
		return err
	}
	// Availability is derived from the keychain at runtime, never restored from
	// a backup or persisted alongside connection metadata.
	connection.HasCredentials = false
	raw, err := encode(connection)
	if err != nil {
		return err
	}
	query := `INSERT INTO shop_connections(id,provider,url,data) VALUES(?,?,?,?)`
	if update {
		query += ` ON CONFLICT(id) DO UPDATE SET provider=excluded.provider,url=excluded.url,data=excluded.data`
	}
	_, err = tx.Exec(query, connection.ID, connection.Provider, connection.URL, raw)
	return err
}

func (s *Store) SaveShopConnection(connection shops.Connection) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	// Once orders exist, changing provider or URL would make their external IDs
	// point at a different shop. A separate connection retains that provenance.
	var count int
	if err = tx.QueryRow(`SELECT COUNT(*) FROM shop_orders WHERE connection_id=? AND EXISTS(SELECT 1 FROM shop_connections WHERE id=? AND (provider<>? OR url<>? OR COALESCE(json_extract(data,'$.siteId'),'')<>?))`, connection.ID, connection.ID, connection.Provider, connection.URL, connection.SiteID).Scan(&count); err != nil {
		return err
	}
	if count > 0 {
		return errors.New("Leg für einen anderen Shop eine neue Verbindung an; diese Verbindung enthält bereits Bestellungen.")
	}
	if err = writeShopConnection(tx, connection, true); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) ShopOrders(connectionID string) ([]shops.Order, error) {
	return shopOrders(s.db, connectionID)
}

func shopOrders(q rowQuerier, connectionID string) ([]shops.Order, error) {
	rows, err := q.Query(`SELECT o.data,CASE WHEN o.invoice_id<>'' AND i.id IS NULL THEN 1 ELSE 0 END FROM shop_orders o LEFT JOIN invoices i ON i.id=o.invoice_id WHERE ?='' OR o.connection_id=? ORDER BY json_extract(o.data,'$.createdAt') DESC,o.id`, connectionID, connectionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []shops.Order{}
	for rows.Next() {
		var raw []byte
		var missing bool
		var order shops.Order
		if err = rows.Scan(&raw, &missing); err != nil {
			return nil, err
		}
		if err = json.Unmarshal(raw, &order); err != nil {
			return nil, err
		}
		if missing {
			order.ReviewReason = shopInvoiceMissingReason
		}
		out = append(out, order)
	}
	return out, rows.Err()
}

func readShopOrder(tx *sql.Tx, id string) (shops.Order, error) {
	var raw []byte
	var order shops.Order
	if err := tx.QueryRow(`SELECT data FROM shop_orders WHERE id=?`, id).Scan(&raw); err != nil {
		return order, err
	}
	err := json.Unmarshal(raw, &order)
	return order, err
}

func (s *Store) ShopOrder(id string) (shops.Order, error) {
	var raw []byte
	var missing bool
	var order shops.Order
	if err := s.db.QueryRow(`SELECT o.data,CASE WHEN o.invoice_id<>'' AND i.id IS NULL THEN 1 ELSE 0 END FROM shop_orders o LEFT JOIN invoices i ON i.id=o.invoice_id WHERE o.id=?`, id).Scan(&raw, &missing); err != nil {
		return order, err
	}
	err := json.Unmarshal(raw, &order)
	if missing {
		order.ReviewReason = shopInvoiceMissingReason
	}
	return order, err
}

func shopStableID(prefix, connectionID, externalID string) string {
	hash := sha256.Sum256([]byte(connectionID + "\x00" + externalID))
	return prefix + "-" + hex.EncodeToString(hash[:16])
}

func writeShopOrder(tx *sql.Tx, order shops.Order, update bool) error {
	if strings.TrimSpace(order.ID) == "" || strings.TrimSpace(order.ConnectionID) == "" || strings.TrimSpace(order.ExternalID) == "" {
		return errors.New("Die Shop-Bestellung ist unvollständig.")
	}
	raw, err := encode(order)
	if err != nil {
		return err
	}
	query := `INSERT INTO shop_orders(id,connection_id,external_id,invoice_id,data) VALUES(?,?,?,?,?)`
	if update {
		query += ` ON CONFLICT(id) DO UPDATE SET invoice_id=excluded.invoice_id,data=excluded.data WHERE shop_orders.connection_id=excluded.connection_id AND shop_orders.external_id=excluded.external_id`
	}
	result, err := tx.Exec(query, order.ID, order.ConnectionID, order.ExternalID, order.InvoiceID, raw)
	if err != nil {
		return err
	}
	if count, err := result.RowsAffected(); err != nil || count != 1 {
		return errors.New("Die Bestellungskennung gehört bereits zu einer anderen Bestellung.")
	}
	return nil
}

func comparableShopCustomer(customer model.Customer) model.Customer {
	customer = model.NormalizeCustomer(customer)
	customer.ID, customer.CreatedAt, customer.UpdatedAt = "", "", ""
	customer.PaymentTermDays, customer.InvoiceSettings = nil, nil
	return customer
}

// Invoice-relevant changes require review even if the provider fails to update
// its modification timestamp. Paid/status changes are included so cancellations
// and refunds cannot leave an invoice silently disconnected from its source.
func sameShopOrderSnapshot(left, right shops.Order) bool {
	left.ID, right.ID = "", ""
	left.InvoiceID, right.InvoiceID = "", ""
	left.ImportedAt, right.ImportedAt = "", ""
	left.UpdatedAt, right.UpdatedAt = "", ""
	left.ReviewReason, right.ReviewReason = "", ""
	left.ConversionError, right.ConversionError = "", ""
	left.Customer = comparableShopCustomer(left.Customer)
	right.Customer = comparableShopCustomer(right.Customer)
	return reflect.DeepEqual(left, right)
}

func (s *Store) UpsertShopOrder(order shops.Order) (shops.Order, bool, error) {
	// Conversion errors describe local company settings or a failed conversion,
	// never facts supplied by a shop. A poll always permits another attempt.
	order.ConversionError = ""
	tx, err := s.db.Begin()
	if err != nil {
		return order, false, err
	}
	defer tx.Rollback()
	var raw []byte
	err = tx.QueryRow(`SELECT data FROM shop_orders WHERE connection_id=? AND external_id=?`, order.ConnectionID, order.ExternalID).Scan(&raw)
	created := errors.Is(err, sql.ErrNoRows)
	if err != nil && !created {
		return order, false, err
	}
	if created {
		if order.ID == "" {
			order.ID = shopStableID("shop-order", order.ConnectionID, order.ExternalID)
		}
		order.InvoiceID = ""
		order.ImportedAt = now()
	} else {
		var existing shops.Order
		if err = json.Unmarshal(raw, &existing); err != nil {
			return order, false, err
		}
		// An out-of-order poll must not replace the newer persisted snapshot.
		previousTime, previousErr := time.Parse(time.RFC3339Nano, existing.UpdatedAt)
		incomingTime, incomingErr := time.Parse(time.RFC3339Nano, order.UpdatedAt)
		if previousErr == nil && incomingErr == nil && incomingTime.Before(previousTime) {
			if existing.ConversionError != "" {
				existing.ConversionError = ""
				if err = writeShopOrder(tx, existing, true); err != nil {
					return existing, false, err
				}
				return existing, false, tx.Commit()
			}
			return existing, false, nil
		}
		order.ID, order.InvoiceID, order.ImportedAt = existing.ID, existing.InvoiceID, existing.ImportedAt
		if order.InvoiceID != "" {
			if !sameShopOrderSnapshot(existing, order) {
				order.ReviewReason = shopOrderChangedReason
			} else if existing.ReviewReason != "" {
				order.ReviewReason = existing.ReviewReason
			}
			var present int
			if err = tx.QueryRow(`SELECT COUNT(*) FROM invoices WHERE id=?`, order.InvoiceID).Scan(&present); err != nil {
				return order, false, err
			}
			if present == 0 {
				order.ReviewReason = shopInvoiceMissingReason
			}
		}
	}
	if err = writeShopOrder(tx, order, true); err != nil {
		return order, false, err
	}
	return order, created, tx.Commit()
}

// CreateShopInvoice atomically saves the customer, draft, and source link. The
// optional timestamp rejects a draft built from an outdated order snapshot.
func (s *Store) CreateShopInvoice(orderID string, draft model.InvoiceDraft, gross string, expectedUpdatedAt ...string) (model.InvoiceDraft, error) {
	tx, err := s.db.Begin()
	if err != nil {
		return draft, err
	}
	defer tx.Rollback()
	order, err := readShopOrder(tx, orderID)
	if err != nil {
		return draft, err
	}
	if order.InvoiceID != "" {
		var raw []byte
		err = tx.QueryRow(`SELECT data FROM invoices WHERE id=?`, order.InvoiceID).Scan(&raw)
		if errors.Is(err, sql.ErrNoRows) {
			order.ReviewReason = shopInvoiceMissingReason
			if err = writeShopOrder(tx, order, true); err != nil {
				return draft, err
			}
			if err = tx.Commit(); err != nil {
				return draft, err
			}
			return draft, errors.New(shopInvoiceMissingReason)
		}
		if err != nil {
			return draft, err
		}
		var existing model.InvoiceDraft
		err = json.Unmarshal(raw, &existing)
		if err == nil && order.ConversionError != "" {
			order.ConversionError = ""
			if err = writeShopOrder(tx, order, true); err != nil {
				return existing, err
			}
			return existing, tx.Commit()
		}
		return existing, err
	}
	if order.ReviewReason != "" {
		return draft, fmt.Errorf("Die Bestellung muss zuerst geprüft werden: %s", order.ReviewReason)
	}
	if len(expectedUpdatedAt) > 0 && order.UpdatedAt != expectedUpdatedAt[0] {
		return draft, errors.New("Die Bestellung wurde inzwischen aktualisiert. Lade sie erneut.")
	}
	orderGross, orderGrossErr := decimal.NewFromString(order.Gross)
	draftGross, draftGrossErr := decimal.NewFromString(gross)
	prepaid := decimal.Zero
	var prepaidErr error
	if draft.PrepaidAmount != "" {
		prepaid, prepaidErr = decimal.NewFromString(draft.PrepaidAmount)
	}
	expectedPrepaid := decimal.Zero
	if order.Paid {
		expectedPrepaid = orderGross
	}
	if orderGrossErr != nil || draftGrossErr != nil || prepaidErr != nil || !orderGross.Equal(draftGross) || !prepaid.Equal(expectedPrepaid) ||
		order.Currency != draft.Currency || !reflect.DeepEqual(comparableShopCustomer(order.Customer), comparableShopCustomer(draft.Customer)) ||
		!reflect.DeepEqual(order.Lines, draft.Lines) || !reflect.DeepEqual(order.Adjustments, draft.Adjustments) || !reflect.DeepEqual(order.SourceTaxes, draft.SourceTaxes) {
		return draft, errors.New("Der Rechnungsentwurf stimmt nicht mehr mit der Bestellung überein. Lade die Bestellung erneut.")
	}
	if draft.ID != "" || (draft.Status != "" && draft.Status != "draft") {
		return draft, errors.New("Für diese Bestellung kann nur ein neuer Rechnungsentwurf angelegt werden.")
	}
	timestamp := now()
	customer := draft.Customer
	if customer.ID == "" {
		customerKey := order.CustomerExternalID
		if customerKey == "" {
			customerKey = order.ExternalID
		}
		customer.ID = shopStableID("shop-customer", order.ConnectionID, customerKey)
	}
	// Preserve existing user preferences while refreshing the shop's contact
	// snapshot. Existing issued invoices embed their own immutable customer.
	var existingRaw []byte
	err = tx.QueryRow(`SELECT data FROM customers WHERE id=?`, customer.ID).Scan(&existingRaw)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return draft, err
	}
	if err == nil {
		var existing model.Customer
		if err = json.Unmarshal(existingRaw, &existing); err != nil {
			return draft, err
		}
		customer.CreatedAt = existing.CreatedAt
		if customer.InvoiceSettings == nil {
			customer.InvoiceSettings = existing.InvoiceSettings
		}
		if customer.PaymentTermDays == nil {
			customer.PaymentTermDays = existing.PaymentTermDays
		}
	} else {
		customer.CreatedAt = timestamp
	}
	customer.UpdatedAt = timestamp
	customerRaw, err := encode(customer)
	if err != nil {
		return draft, err
	}
	if _, err = tx.Exec(`INSERT INTO customers(id,name,data,created_at,updated_at) VALUES(?,?,?,?,?) ON CONFLICT(id) DO UPDATE SET name=excluded.name,data=excluded.data,updated_at=excluded.updated_at`, customer.ID, customer.Name, customerRaw, customer.CreatedAt, customer.UpdatedAt); err != nil {
		return draft, err
	}
	draft.ID, draft.Status, draft.CreatedAt, draft.UpdatedAt = newID("inv"), "draft", timestamp, timestamp
	draft.CustomerID, draft.Customer = customer.ID, customer
	raw, err := encode(draft)
	if err != nil {
		return draft, err
	}
	if _, err = tx.Exec(`INSERT INTO invoices(id,number,customer_name,status,issue_date,gross,data,created_at,updated_at) VALUES(?,?,?,?,?,?,?,?,?)`, draft.ID, draft.Number, customer.Name, draft.Status, draft.IssueDate, gross, raw, timestamp, timestamp); err != nil {
		return draft, err
	}
	if err = rememberPaymentTerm(tx, draft); err != nil {
		return draft, err
	}
	order.InvoiceID = draft.ID
	order.ConversionError = ""
	if err = writeShopOrder(tx, order, true); err != nil {
		return draft, err
	}
	return draft, tx.Commit()
}

func (s *Store) RecordShopOrderReview(id, reason string) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	order, err := readShopOrder(tx, id)
	if err != nil {
		return err
	}
	order.ConversionError = reason
	if err = writeShopOrder(tx, order, true); err != nil {
		return err
	}
	return tx.Commit()
}

func restoreShops(tx *sql.Tx, connections []shops.Connection, orders []shops.Order) error {
	for _, connection := range connections {
		if err := writeShopConnection(tx, connection, false); err != nil {
			return fmt.Errorf("Die gesicherte Shop-Verbindung ist ungültig: %w", err)
		}
	}
	for _, order := range orders {
		if order.InvoiceID != "" {
			var present int
			if err := tx.QueryRow(`SELECT COUNT(*) FROM invoices WHERE id=?`, order.InvoiceID).Scan(&present); err != nil {
				return err
			}
			if present == 0 {
				order.ReviewReason = shopInvoiceMissingReason
			}
		}
		if err := writeShopOrder(tx, order, false); err != nil {
			return fmt.Errorf("Die gesicherte Shop-Bestellung ist ungültig: %w", err)
		}
	}
	return nil
}
