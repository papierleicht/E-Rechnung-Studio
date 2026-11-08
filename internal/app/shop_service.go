package app

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/zalando/go-keyring"
	"papierleicht.de/e-rechnung-studio/internal/engine"
	"papierleicht.de/e-rechnung-studio/internal/model"
	"papierleicht.de/e-rechnung-studio/internal/shops"
)

const shopKeyringService = "de.papierleicht.shops"
const shopSyncInterval = 5 * time.Minute

type shopReviewError struct{ message string }

func (e *shopReviewError) Error() string { return e.message }

type shopVault interface {
	Get(string) (string, error)
	Set(string, string) error
	Delete(string) error
}

type shopSystemVault struct{}

// Bind the secret to its origin as well as its local ID. Restoring a backup
// cannot make an existing keychain entry authenticate against a different URL.
type shopSecret struct {
	Provider    string            `json:"provider"`
	URL         string            `json:"url"`
	SiteID      string            `json:"siteId,omitempty"`
	Credentials shops.Credentials `json:"credentials"`
}

func (shopSystemVault) Get(id string) (string, error) { return keyring.Get(shopKeyringService, id) }
func (shopSystemVault) Set(id, value string) error    { return keyring.Set(shopKeyringService, id, value) }
func (shopSystemVault) Delete(id string) error {
	err := keyring.Delete(shopKeyringService, id)
	if errors.Is(err, keyring.ErrNotFound) {
		return nil
	}
	return err
}

// ShopService uses the invoice database and engine so imported orders follow
// exactly the same numbering, validation and issuance workflow as other drafts.
type ShopService struct {
	mu       sync.Mutex
	wg       sync.WaitGroup
	invoices *InvoiceService
	fetcher  shops.Fetcher
	vault    shopVault
	ctx      context.Context
	cancel   context.CancelFunc
	busy     bool
	closed   bool
	started  bool
}

func NewShopService(invoices *InvoiceService) *ShopService {
	ctx, cancel := context.WithCancel(context.Background())
	return &ShopService{invoices: invoices, fetcher: shops.NewClient(), vault: shopSystemVault{}, ctx: ctx, cancel: cancel}
}

//wails:ignore
func (s *ShopService) Start() {
	s.mu.Lock()
	if s.started || s.closed {
		s.mu.Unlock()
		return
	}
	s.started = true
	s.wg.Add(1)
	s.mu.Unlock()
	go func() {
		defer s.wg.Done()
		ticker := time.NewTicker(shopSyncInterval)
		defer ticker.Stop()
		for {
			connections, err := s.invoices.store.ShopConnections()
			if err == nil {
				for _, c := range connections {
					if s.ctx.Err() != nil {
						return
					}
					if c.Enabled && !s.invoices.ReadOnly() {
						_, _ = s.Sync(c.ID)
					}
				}
			}
			select {
			case <-s.ctx.Done():
				return
			case <-ticker.C:
			}
		}
	}()
}

//wails:ignore
func (s *ShopService) Close() error {
	s.mu.Lock()
	s.closed = true
	s.cancel()
	s.mu.Unlock()
	s.wg.Wait()
	return nil
}

func (s *ShopService) begin() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return errors.New("Die Shop-Anbindung wurde beendet.")
	}
	if s.busy {
		return errors.New("Ein Shop wird gerade verarbeitet. Versuch es gleich noch einmal.")
	}
	// The auditor view keeps the database read-only; shops wait until it ends.
	if s.invoices.ReadOnly() {
		return errors.New("Solange die Prüferansicht offen ist, ruft die App keine Shops ab.")
	}
	s.busy = true
	s.wg.Add(1)
	s.invoices.shopMu.Lock()
	return nil
}

func (s *ShopService) end() {
	s.invoices.shopMu.Unlock()
	s.mu.Lock()
	s.busy = false
	s.mu.Unlock()
	s.wg.Done()
}

func (s *ShopService) Overview() (shops.Overview, error) {
	s.mu.Lock()
	busy := s.busy
	s.mu.Unlock()
	out := shops.Overview{Syncing: busy}
	var err error
	out.Connections, err = s.invoices.store.ShopConnections()
	if err != nil {
		return out, err
	}
	for i := range out.Connections {
		_, err := s.credentials(out.Connections[i])
		out.Connections[i].HasCredentials = err == nil
	}
	out.Orders, err = s.invoices.store.ShopOrders("")
	return out, err
}

func (s *ShopService) credentials(c shops.Connection) (shops.Credentials, error) {
	var secret shopSecret
	raw, err := s.vault.Get(c.ID)
	if err != nil {
		return shops.Credentials{}, errors.New("Verbinde den Shop erneut, um seine Zugangsdaten zu hinterlegen.")
	}
	if json.Unmarshal([]byte(raw), &secret) != nil || secret.Provider != c.Provider || secret.URL != c.URL || secret.SiteID != c.SiteID || shops.ValidateCredentials(c.Provider, secret.Credentials) != nil {
		return shops.Credentials{}, errors.New("Verbinde den Shop erneut, um seine Zugangsdaten zu hinterlegen.")
	}
	return secret.Credentials, nil
}

func shopSince(c shops.Connection) time.Time {
	since, _ := time.Parse("2006-01-02", c.ImportFrom)
	if checkpoint, err := time.Parse(time.RFC3339, c.LastSync); err == nil {
		// Replaying a small overlap covers timestamp boundaries and updates that
		// became visible during pagination. Order identity makes replay harmless.
		checkpoint = checkpoint.Add(-5 * time.Minute)
		if checkpoint.After(since) {
			since = checkpoint
		}
	}
	return since
}

func (s *ShopService) Connect(c shops.Connection, credentials shops.Credentials) (shops.Connection, error) {
	if err := s.begin(); err != nil {
		return shops.Connection{}, err
	}
	defer s.end()
	var err error
	c, err = shops.NormalizeConnection(c)
	if err != nil {
		return shops.Connection{}, err
	}
	if c.ImportFrom == "" {
		c.ImportFrom = time.Now().AddDate(0, 0, -30).Format("2006-01-02")
	}
	from, err := time.Parse("2006-01-02", c.ImportFrom)
	if err != nil || from.After(time.Now().UTC().Add(24*time.Hour)) {
		return shops.Connection{}, errors.New("Wähle ein gültiges Startdatum bis heute.")
	}
	credentials.AccessToken = strings.TrimSpace(credentials.AccessToken)
	credentials.ClientID = strings.TrimSpace(credentials.ClientID)
	credentials.ClientSecret = strings.TrimSpace(credentials.ClientSecret)
	credentials.ConsumerKey = strings.TrimSpace(credentials.ConsumerKey)
	credentials.ConsumerSecret = strings.TrimSpace(credentials.ConsumerSecret)
	credentials.APIKey = strings.TrimSpace(credentials.APIKey)
	credentials.Username = strings.TrimSpace(credentials.Username)
	if err = shops.ValidateCredentials(c.Provider, credentials); err != nil {
		return shops.Connection{}, err
	}
	connections, err := s.invoices.store.ShopConnections()
	if err != nil {
		return shops.Connection{}, err
	}
	var existing *shops.Connection
	for _, candidate := range connections {
		if candidate.ID == c.ID || (candidate.Provider == c.Provider &&
			(candidate.URL == c.URL || (c.Provider == "wix" && candidate.SiteID == c.SiteID))) {
			copy := candidate
			existing = &copy
			break
		}
	}
	if c.ID != "" && (existing == nil || existing.ID != c.ID) {
		return shops.Connection{}, errors.New("Die Shop-Anbindung wurde nicht gefunden.")
	}
	if existing != nil {
		if c.Provider == "wix" && existing.SiteID == c.SiteID && existing.URL != c.URL {
			return shops.Connection{}, errors.New("Diese Wix-Website ist bereits verbunden. Verwende die vorhandene Shop-Anbindung.")
		}
		if existing.Provider != c.Provider || existing.URL != c.URL || existing.SiteID != c.SiteID {
			return shops.Connection{}, errors.New("Leg für einen anderen Shop eine neue Anbindung an.")
		}
		c.ID, c.LastSync = existing.ID, existing.LastSync
		if c.ImportFrom != existing.ImportFrom {
			c.LastSync = ""
		}
	} else {
		c.ID, c.LastSync = uuid.NewString(), ""
	}
	c.Enabled, c.HasCredentials, c.LastError = true, true, ""
	started := time.Now().UTC()
	ctx, cancel := context.WithTimeout(s.ctx, 2*time.Minute)
	defer cancel()
	orders, err := s.fetcher.FetchOrders(ctx, c, credentials, shopSince(c))
	if err == nil {
		err = ctx.Err()
	}
	if err != nil {
		return shops.Connection{}, errors.New(shopFetchError(err))
	}
	raw, _ := json.Marshal(shopSecret{Provider: c.Provider, URL: c.URL, SiteID: c.SiteID, Credentials: credentials})
	old, oldErr := s.vault.Get(c.ID)
	if err = s.vault.Set(c.ID, string(raw)); err != nil {
		return shops.Connection{}, errors.New("Die Zugangsdaten ließen sich nicht im Schlüsselbund speichern. Entsperr den Schlüsselbund und versuch es erneut.")
	}
	if err = s.invoices.store.SaveShopConnection(c); err != nil {
		if oldErr == nil {
			_ = s.vault.Set(c.ID, old)
		} else {
			_ = s.vault.Delete(c.ID)
		}
		return shops.Connection{}, err
	}
	if _, err = s.importOrders(c, orders); err != nil {
		c.LastError = "Der Import ließ sich nicht vollständig speichern. Ruf den Shop erneut ab."
		_ = s.invoices.store.SaveShopConnection(c)
		return c, errors.New(c.LastError)
	}
	c.LastSync = started.Format(time.RFC3339)
	err = s.invoices.store.SaveShopConnection(c)
	return c, err
}

func shopFetchError(err error) string {
	if errors.Is(err, context.Canceled) {
		return "Der Shop-Abruf wurde abgebrochen."
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return "Der Shop hat nicht rechtzeitig geantwortet. Versuch den Abruf erneut."
	}
	return shops.SafeError(err)
}

func (s *ShopService) Sync(connectionID string) (shops.SyncResult, error) {
	if err := s.begin(); err != nil {
		return shops.SyncResult{}, err
	}
	defer s.end()
	c, err := s.invoices.store.ShopConnection(connectionID)
	if err != nil {
		return shops.SyncResult{}, errors.New("Die Shop-Anbindung wurde nicht gefunden.")
	}
	if !c.Enabled {
		return shops.SyncResult{}, errors.New("Der Shop ist pausiert. Aktiviere die Anbindung zuerst.")
	}
	credentials, err := s.credentials(c)
	if err != nil {
		c.LastError = err.Error()
		_ = s.invoices.store.SaveShopConnection(c)
		return shops.SyncResult{}, err
	}
	started := time.Now().UTC()
	ctx, cancel := context.WithTimeout(s.ctx, 2*time.Minute)
	defer cancel()
	orders, err := s.fetcher.FetchOrders(ctx, c, credentials, shopSince(c))
	if err == nil {
		err = ctx.Err()
	}
	if err != nil {
		c.LastError = shopFetchError(err)
		_ = s.invoices.store.SaveShopConnection(c)
		return shops.SyncResult{}, errors.New(c.LastError)
	}
	result, err := s.importOrders(c, orders)
	if err != nil {
		c.LastError = "Der Import ließ sich nicht vollständig speichern. Ruf den Shop erneut ab."
		_ = s.invoices.store.SaveShopConnection(c)
		return result, errors.New(c.LastError)
	}
	c.LastSync, c.LastError = started.Format(time.RFC3339), ""
	return result, s.invoices.store.SaveShopConnection(c)
}

func (s *ShopService) importOrders(c shops.Connection, orders []shops.Order) (shops.SyncResult, error) {
	result := shops.SyncResult{}
	for _, o := range orders {
		if err := s.ctx.Err(); err != nil {
			return result, err
		}
		if o.ExternalID == "" {
			return result, errors.New("Die Bestellkennung fehlt.")
		}
		o.ID, o.InvoiceID, o.ImportedAt, o.ConnectionID = "", "", "", c.ID
		o.ConversionError = ""
		saved, added, err := s.invoices.store.UpsertShopOrder(o)
		if err != nil {
			return result, err
		}
		if added {
			result.Imported++
		}
		if saved.ReviewReason == "" && saved.InvoiceID == "" && c.AutoCreateDrafts {
			if _, err := s.createInvoice(saved); err != nil {
				// Business mapping problems become reviewable order state. Storage
				// errors abort without advancing the checkpoint so retry is safe.
				var review *shopReviewError
				if !errors.As(err, &review) {
					return result, err
				}
				if err = s.invoices.store.RecordShopOrderReview(saved.ID, err.Error()); err != nil {
					return result, err
				}
				saved.ConversionError = "Die Bestellung benötigt eine Prüfung."
			} else {
				result.Drafts++
			}
		}
		if saved.ReviewReason != "" || saved.ConversionError != "" {
			result.Review++
		}
	}
	return result, nil
}

func (s *ShopService) SetEnabled(id string, enabled bool) error {
	if err := s.begin(); err != nil {
		return err
	}
	defer s.end()
	c, err := s.invoices.store.ShopConnection(id)
	if err != nil {
		return errors.New("Die Shop-Anbindung wurde nicht gefunden.")
	}
	if enabled {
		if _, err := s.credentials(c); err != nil {
			return err
		}
	}
	c.Enabled = enabled
	return s.invoices.store.SaveShopConnection(c)
}

func (s *ShopService) Disconnect(id string) error {
	if err := s.begin(); err != nil {
		return err
	}
	defer s.end()
	c, err := s.invoices.store.ShopConnection(id)
	if err != nil {
		return errors.New("Die Shop-Anbindung wurde nicht gefunden.")
	}
	// Pause before removing the secret so a keychain failure is safely retryable.
	c.Enabled, c.HasCredentials = false, false
	if err = s.invoices.store.SaveShopConnection(c); err != nil {
		return err
	}
	if err = s.vault.Delete(id); err != nil {
		return errors.New("Der Shop wurde pausiert. Seine Zugangsdaten ließen sich nicht aus dem Schlüsselbund entfernen; versuch das Trennen erneut.")
	}
	return nil
}

func (s *ShopService) CreateInvoice(orderID string) (model.InvoiceDraft, error) {
	if err := s.begin(); err != nil {
		return model.InvoiceDraft{}, err
	}
	defer s.end()
	o, err := s.invoices.store.ShopOrder(orderID)
	if err != nil {
		return model.InvoiceDraft{}, errors.New("Die Bestellung wurde nicht gefunden.")
	}
	d, err := s.createInvoice(o)
	var review *shopReviewError
	if errors.As(err, &review) && o.InvoiceID == "" && o.ReviewReason == "" {
		_ = s.invoices.store.RecordShopOrderReview(o.ID, err.Error())
	}
	return d, err
}

func (s *ShopService) createInvoice(o shops.Order) (model.InvoiceDraft, error) {
	if o.InvoiceID != "" {
		d, err := s.invoices.store.Invoice(o.InvoiceID)
		if err != nil {
			return d, errors.New("Die zugehörige Rechnung wurde gelöscht. Prüf die Bestellung; eine zweite Rechnung erstellt die App dafür nicht automatisch.")
		}
		return d, nil
	}
	if o.ReviewReason != "" {
		return model.InvoiceDraft{}, &shopReviewError{o.ReviewReason}
	}
	c, err := s.invoices.store.Company()
	if err != nil {
		return model.InvoiceDraft{}, errors.New("Die Unternehmenseinstellungen konnten nicht gelesen werden.")
	}
	// The company profile decides about § 19 UStG; the order has to agree with it.
	taxed, untaxed := shopOrderTaxes(o)
	if !c.SmallBusiness {
		if treatment := c.InvoiceTaxTreatment(); treatment != "standard" && treatment != "mixed" {
			return model.InvoiceDraft{}, &shopReviewError{"Die Shop-Steuern passen nicht zu deinen Unternehmenseinstellungen. Prüf den Steuerfall der Bestellung im Shop."}
		}
		if untaxed {
			return model.InvoiceDraft{}, &shopReviewError{"Die Bestellung enthält Positionen ohne Umsatzsteuer. Prüf die steuerliche Behandlung im Shop."}
		}
	} else if taxed {
		return model.InvoiceDraft{}, &shopReviewError{"Die Bestellung weist Umsatzsteuer aus, laut deinem Unternehmensprofil rechnest du aber als Kleinunternehmer ohne Umsatzsteuer ab. Prüf die Steuereinstellungen im Shop."}
	}
	if strings.TrimSpace(o.Customer.Name) == "" || strings.TrimSpace(o.Customer.Address.Street) == "" || strings.TrimSpace(o.Customer.Address.City) == "" || len(o.Customer.Address.Country) != 2 {
		return model.InvoiceDraft{}, &shopReviewError{"Die Rechnungsadresse ist unvollständig. Ergänze sie im Shop und ruf erneut ab."}
	}
	if len(o.Currency) != 3 {
		return model.InvoiceDraft{}, &shopReviewError{"Die Bestellwährung ist ungültig. Prüf die Bestellung im Shop."}
	}
	if o.Currency != "EUR" {
		return model.InvoiceDraft{}, &shopReviewError{"Die Bestellung verwendet eine Fremdwährung. Der Rechnungsablauf unterstützt derzeit nur EUR; bearbeite diesen Beleg separat. Die Bestellwährung wird nicht umgerechnet."}
	}
	d := model.NewDraft()
	d.AutomaticNumber = true
	d.DeliveryDate = "" // An order date is not proof of delivery or service.
	d.TaxSource, d.TaxTreatment = "manual", "mixed"
	if c.SmallBusiness {
		d.TaxTreatment, d.TaxExemptionReason = "small-business", c.ExemptionReason
	}
	d.Customer = model.NormalizeCustomer(o.Customer)
	connection, err := s.invoices.store.ShopConnection(o.ConnectionID)
	if err != nil {
		return model.InvoiceDraft{}, errors.New("Die Shop-Anbindung wurde nicht gefunden.")
	}
	identity := o.CustomerExternalID
	if identity == "" {
		address, _ := json.Marshal(d.Customer.Address)
		identity = strings.ToLower(strings.TrimSpace(d.Customer.Email)) + "|" + d.Customer.Name + "|" + string(address)
	}
	origin := connection.Provider + "|" + connection.URL
	if connection.SiteID != "" {
		origin += "|site:" + connection.SiteID
	}
	hash := sha256.Sum256([]byte(origin + "|" + identity))
	d.Customer.ID = "shop-customer-" + hex.EncodeToString(hash[:16])
	d.Customer.InvoiceSettings = nil
	d.CustomerID = d.Customer.ID
	d.Currency, d.Lines, d.Adjustments, d.SourceTaxes = o.Currency, o.Lines, o.Adjustments, o.SourceTaxes
	d.Notes = "Bestellung " + o.Number + " · " + connection.Label
	if o.Paid {
		d.PrepaidAmount = o.Gross
		d.PaymentTerms, d.DueDate = "Bereits im Shop bezahlt.", ""
		d.PaymentMeansCode, d.DisablePaymentQR = "97", true
	}
	inv, err := s.invoices.engine.Build(d, c)
	if err != nil {
		return model.InvoiceDraft{}, &shopReviewError{"Die Bestellpositionen lassen sich nicht vollständig übernehmen. Prüf Mengen, Preise und Steuern im Shop."}
	}
	gross, err := decimal.NewFromString(o.Gross)
	if err != nil || gross.IsNegative() || !gross.Equal(gross.Round(2)) || !gross.Equal(inv.GrandTotal) {
		return model.InvoiceDraft{}, &shopReviewError{"Der berechnete Rechnungsbetrag weicht vom Shop ab. Prüf Rabatte, Versand und Steuern im Shop."}
	}
	d, err = s.invoices.store.CreateShopInvoice(o.ID, d, engine.Totals(inv).Gross, o.UpdatedAt)
	if err != nil {
		return d, errors.New("Der Rechnungsentwurf ließ sich nicht speichern. Prüf die Bestellung und versuch es erneut.")
	}
	return d, nil
}

// shopOrderTaxes tells whether an order charges VAT and whether it has
// positive amounts without any.
func shopOrderTaxes(o shops.Order) (taxed, untaxed bool) {
	for _, line := range o.Lines {
		price, _ := decimal.NewFromString(line.UnitPrice)
		switch {
		case line.TaxCategory == "S":
			taxed = true
		case price.IsPositive():
			untaxed = true
		}
	}
	for _, adjustment := range o.Adjustments {
		if adjustment.TaxCategory == "S" {
			taxed = true
		} else if adjustment.Charge {
			untaxed = true
		}
	}
	return taxed, untaxed
}
