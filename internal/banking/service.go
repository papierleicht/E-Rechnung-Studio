package banking

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/url"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unicode/utf8"

	fints "github.com/BergerAPI/go-fints"
	"github.com/zalando/go-keyring"
)

const keyringService = "de.papierleicht.fints"

// Origins of the FinTS product ID, in the order they are consulted. Only the
// origin is reported to the UI; the ID itself never leaves the backend.
const (
	ProductSourceBuild        = "build"
	ProductSourceOwn          = "own"
	ProductSourceSubscription = "subscription"
)

// hbci4JavaTestProductID is HBCI4Java's public test registration. It belongs to
// another product and must not identify papierleicht towards a bank.
const hbci4JavaTestProductID = "36792786FA12F235F04647689"

// ProductIDSource supplies a registered product ID, e.g. from an active
// subscription. It returns an empty ID when it has none.
type ProductIDSource func() (string, error)

// errAccountsKnown ends a new connection whose accounts are all set up already.
var errAccountsKnown = errors.New("accounts already set up")

const maxAccountLabel = 60

var bankCodePattern = regexp.MustCompile(`^[0-9]{8}$`)

type Vault interface {
	Get(string) (string, error)
	Set(string, string) error
	Delete(string) error
}
type systemVault struct{}

func (systemVault) Get(id string) (string, error) { return keyring.Get(keyringService, id) }
func (systemVault) Set(id, pin string) error      { return keyring.Set(keyringService, id, pin) }
func (systemVault) Delete(id string) error {
	e := keyring.Delete(keyringService, id)
	if errors.Is(e, keyring.ErrNotFound) {
		return nil
	}
	return e
}

type readClient interface {
	Accounts(context.Context) ([]fints.Account, error)
	Transactions(context.Context, fints.Account, time.Time, time.Time) ([]fints.Transaction, error)
	TANMethods(context.Context) ([]fints.TANMethod, error)
}
type balanceClient interface {
	Balance(context.Context, fints.Account) (*fints.AccountBalance, error)
}

// statementBalanceClient exposes the closing balance that came with the
// transactions, which needs no further request to the bank.
type statementBalanceClient interface {
	StatementBalance() *fints.DatedBalance
}
type factory func(fints.Config) (readClient, error)

// errBalanceNeedsApproval stops an optional balance fetch that would ask the
// customer for a second TAN or app approval within one bank fetch.
var errBalanceNeedsApproval = errors.New("balance requires approval")

type Challenge struct {
	ID            string `json:"id"`
	Text          string `json:"text"`
	Decoupled     bool   `json:"decoupled"`
	ImageMIMEType string `json:"imageMimeType"`
	Image         []byte `json:"image"`
}
type Operation struct {
	ID           string            `json:"id"`
	ConnectionID string            `json:"connectionId"`
	Status       string            `json:"status"`
	Message      string            `json:"message"`
	Challenge    *Challenge        `json:"challenge,omitempty"`
	Methods      []fints.TANMethod `json:"methods"`
	Media        []fints.TANMedium `json:"media"`
	Added        int               `json:"added"`
}
type job struct {
	result Operation
	cancel context.CancelFunc
	tan    chan string
	done   chan struct{}
}
type Service struct {
	mu             sync.Mutex
	store          *localStore
	vault          Vault
	factory        factory
	productID      string
	subscription   ProductIDSource
	diagnosticPath string
	active         *job
	closed         bool
}

// New stores banking data in the shared application database db; dir is the
// data directory, where the last error is kept.
func New(db *sql.DB, dir, productID string) (*Service, error) {
	st, e := newStore(db)
	if e != nil {
		return nil, e
	}
	return &Service{store: st, vault: systemVault{}, factory: func(c fints.Config) (readClient, error) { return fints.NewClient(c) }, productID: strings.TrimSpace(productID), diagnosticPath: filepath.Join(dir, "banking-last-error.json")}, nil
}
func (s *Service) Close() error {
	s.mu.Lock()
	s.closed = true
	j := s.active
	if j != nil {
		j.cancel()
	}
	s.mu.Unlock()
	if j != nil {
		<-j.done
	}
	// The database belongs to the application store, which closes it.
	return nil
}
func (s *Service) Overview() (Overview, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	o, e := s.store.overview()
	if e != nil {
		return o, e
	}
	// A failing subscription source only disables the bank fetch; the
	// overview of already imported data must stay available.
	_, o.ProductSource, _ = s.resolveProductID()
	return o, nil
}

// SetSubscriptionSource registers the product ID delivered with an active
// subscription. It is consulted after build and own IDs.
func (s *Service) SetSubscriptionSource(src ProductIDSource) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.subscription = src
}
func (s *Service) SetProductID(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.productID != "" {
		return errors.New("Die Produktkennung ist in dieser Installation bereits vorgegeben.")
	}
	id = strings.TrimSpace(id)
	if id == "" || len(id) > 25 {
		return errors.New("Gib eine gültige, registrierte FinTS-Produktkennung ein (maximal 25 Zeichen).")
	}
	if id == hbci4JavaTestProductID {
		return errors.New("Die öffentliche HBCI4Java-Testkennung gehört zu einem anderen Produkt. Gib eine eigene, registrierte Kennung ein.")
	}
	_, e := s.store.db.Exec("INSERT INTO bank_settings(id,product_id) VALUES(1,?) ON CONFLICT(id) DO UPDATE SET product_id=excluded.product_id", id)
	return e
}
func validateConnection(c Connection) error {
	u, e := url.Parse(c.Endpoint)
	if e != nil || u.Scheme != "https" || u.Hostname() == "" || u.User != nil || u.Fragment != "" || u.RawQuery != "" {
		return errors.New("Gib die HTTPS-FinTS-Adresse deiner Bank ein, ohne Zugangsdaten und ohne URL-Parameter.")
	}
	if !bankCodePattern.MatchString(c.BankCode) {
		return errors.New("Gib die achtstellige Bankleitzahl ein.")
	}
	if strings.TrimSpace(c.UserID) == "" || strings.TrimSpace(c.Label) == "" {
		return errors.New("Die Bankbezeichnung und die Benutzerkennung fehlen.")
	}
	if len(c.SecurityFunction) > 3 {
		return errors.New("Das TAN-Verfahren ist ungültig.")
	}
	return nil
}

// Connect stores only non-secret configuration in SQLite and discovers accounts.
// Password persistence is opt-in and fails closed if the OS vault is unavailable.
func (s *Service) Connect(c Connection, pin string, remember bool) (Operation, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if e := s.available(); e != nil {
		return Operation{}, e
	}
	c.Endpoint = strings.TrimSpace(c.Endpoint)
	c.BankCode = strings.TrimSpace(c.BankCode)
	if e := validateConnection(c); e != nil {
		return Operation{}, e
	}
	if strings.TrimSpace(pin) == "" {
		return Operation{}, errors.New("Gib deine Online-Banking-PIN ein.")
	}
	hadSavedPIN := false
	if c.ID == "" {
		known, found, e := s.store.duplicateConnection(c)
		if e != nil {
			return Operation{}, e
		}
		if found {
			return Operation{}, fmt.Errorf("Diese Bankverbindung ist bereits eingerichtet („%s“). Öffne sie unter Konten, um Umsätze abzurufen.", known.Label)
		}
		c.ID = newID()
	} else {
		old, e := s.store.connection(c.ID)
		if e != nil {
			return Operation{}, errors.New("Bankverbindung nicht gefunden.")
		}
		hadSavedPIN = old.RememberPIN
		if old.Endpoint == c.Endpoint && old.UserID == c.UserID && old.CustomerID == c.CustomerID && old.BankCode == c.BankCode {
			c.SystemID = old.SystemID
		} else {
			return Operation{}, errors.New("Für einen anderen Bankzugang lege eine neue Verbindung an.")
		}
	}
	product, e := s.getProductID()
	if e != nil {
		return Operation{}, e
	}
	c.RememberPIN = remember
	// Keep the old credential only in memory so a failed metadata write can
	// compensate for a preceding keyring change. Never leave an unreachable
	// credential behind when the database rejects a new connection.
	previousPIN := ""
	if hadSavedPIN {
		previousPIN, e = s.vault.Get(c.ID)
		if e != nil {
			return Operation{}, errors.New("Die bisher gespeicherte PIN ist nicht verfügbar. Entferne sie zuerst und verbinde dich dann erneut.")
		}
	}
	if remember {
		if e = s.vault.Set(c.ID, pin); e != nil {
			return Operation{}, errors.New("Die PIN konnte nicht im Schlüsselspeicher deines Betriebssystems gespeichert werden. Versuche es noch einmal, ohne die PIN zu speichern.")
		}
	} else if hadSavedPIN {
		if e = s.vault.Delete(c.ID); e != nil {
			return Operation{}, errors.New("Die bisher gespeicherte PIN konnte nicht entfernt werden.")
		}
	}
	if e = s.store.saveConnection(c); e != nil {
		var restoreErr error
		if hadSavedPIN {
			restoreErr = s.vault.Set(c.ID, previousPIN)
		} else if remember {
			restoreErr = s.vault.Delete(c.ID)
		}
		if restoreErr != nil {
			return Operation{}, errors.New("Die Bankverbindung konnte nicht gespeichert werden, und die Änderung im Schlüsselspeicher konnte nicht zurückgesetzt werden. Entferne den Eintrag selbst im Schlüsselspeicher deines Betriebssystems.")
		}
		return Operation{}, errors.New("Die Bankverbindung konnte nicht gespeichert werden. Die PIN-Änderung wurde zurückgesetzt.")
	}
	return s.start(c, pin, product, nil, time.Time{}, time.Time{}), nil
}
func (s *Service) Sync(accountID, pin, from, to string) (Operation, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if e := s.available(); e != nil {
		return Operation{}, e
	}
	a, e := s.store.account(accountID)
	if e != nil {
		return Operation{}, errors.New("Konto nicht gefunden.")
	}
	c, e := s.store.connection(a.ConnectionID)
	if e != nil {
		return Operation{}, e
	}
	start, e := time.Parse("2006-01-02", from)
	if e != nil {
		return Operation{}, errors.New("Das Startdatum ist ungültig.")
	}
	end, e := time.Parse("2006-01-02", to)
	if e != nil || end.Before(start) {
		return Operation{}, errors.New("Der Zeitraum ist ungültig.")
	}
	if end.Sub(start) > 366*24*time.Hour {
		return Operation{}, errors.New("Rufe höchstens ein Jahr auf einmal ab.")
	}
	if pin == "" && c.RememberPIN {
		pin, e = s.vault.Get(c.ID)
		if e != nil {
			return Operation{}, errors.New("Die gespeicherte PIN ist nicht verfügbar. Gib deine PIN noch einmal ein.")
		}
	}
	if pin == "" {
		return Operation{}, errors.New("Gib deine Online-Banking-PIN ein.")
	}
	product, e := s.getProductID()
	if e != nil {
		return Operation{}, e
	}
	return s.start(c, pin, product, &a, start, end), nil
}
func (s *Service) getProductID() (string, error) {
	id, _, e := s.resolveProductID()
	if e != nil {
		return "", errors.New("Die FinTS-Produktkennung deines Abos ist gerade nicht verfügbar. Versuche es später noch einmal.")
	}
	if id == "" {
		return "", errors.New("Für diese Installation ist noch keine registrierte FinTS-Produktkennung hinterlegt.")
	}
	return id, nil
}

// resolveProductID consults the build, the user's own ID and the subscription
// in this order. Callers must hold s.mu.
func (s *Service) resolveProductID() (id, source string, err error) {
	if s.productID != "" {
		return s.productID, ProductSourceBuild, nil
	}
	_ = s.store.db.QueryRow("SELECT product_id FROM bank_settings WHERE id=1").Scan(&id)
	if id = strings.TrimSpace(id); id != "" && id != hbci4JavaTestProductID {
		return id, ProductSourceOwn, nil
	}
	if s.subscription == nil {
		return "", "", nil
	}
	if id, err = s.subscription(); err != nil {
		return "", "", err
	}
	if id = strings.TrimSpace(id); id == "" {
		return "", "", nil
	}
	return id, ProductSourceSubscription, nil
}
func (s *Service) available() error {
	if s.closed {
		return errors.New("Banking wurde beendet.")
	}
	if s.active != nil {
		select {
		case <-s.active.done:
		default:
			return errors.New("Es läuft bereits ein Bankabruf.")
		}
	}
	return nil
}
func (s *Service) start(c Connection, pin, product string, a *Account, from, to time.Time) Operation {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	j := &job{result: Operation{ID: newID(), ConnectionID: c.ID, Status: "running", Message: "Verbindung zur Bank wird hergestellt.", Methods: []fints.TANMethod{}, Media: []fints.TANMedium{}}, cancel: cancel, tan: make(chan string, 1), done: make(chan struct{})}
	s.active = j
	go s.run(ctx, j, c, pin, product, a, from, to)
	return j.result
}
func (s *Service) run(ctx context.Context, j *job, c Connection, pin, product string, a *Account, from, to time.Time) {
	defer close(j.done)
	defer j.cancel()
	// HKVVB's product version allows at most five characters; the product name
	// is represented by its registered ProductID, not this version field.
	config := fints.Config{Endpoint: c.Endpoint, BankCode: c.BankCode, UserID: c.UserID, CustomerID: c.CustomerID, PIN: pin, ProductID: product, ProductVersion: "1.0.0", SystemID: c.SystemID, SecurityFunction: c.SecurityFunction, TANMedium: c.TANMedium, AutoPoll: true, Timeout: 45 * time.Second}
	var balancePhase atomic.Bool
	config.TANHandler = func(ctx context.Context, ch fints.TANChallenge) (string, error) {
		if balancePhase.Load() {
			return "", errBalanceNeedsApproval
		}
		s.mu.Lock()
		challenge := &Challenge{ID: newID(), Text: ch.Challenge, Decoupled: ch.Decoupled, ImageMIMEType: ch.ImageMIMEType, Image: append([]byte(nil), ch.Image...)}
		j.result.Challenge = challenge
		if ch.Decoupled {
			j.result.Status = "approval"
			j.result.Message = "Gib den Abruf in deiner Banking-App frei."
		} else {
			j.result.Status = "tan"
			j.result.Message = "Gib die TAN für den Bankabruf ein."
		}
		s.mu.Unlock()
		if ch.Decoupled {
			return "", nil
		}
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case tan := <-j.tan:
			return tan, nil
		}
	}
	config.OnTANPoll = func(ctx context.Context, ch fints.TANChallenge) error {
		s.mu.Lock()
		j.result.Message = fmt.Sprintf("Die Freigabe in deiner Banking-App wird geprüft (%d).", ch.PollAttempt)
		s.mu.Unlock()
		return ctx.Err()
	}
	importCommitted := false
	balanceNote := ""
	skipped := 0
	stage := "client_setup"
	client, e := s.factory(config)
	if e == nil {
		if a == nil {
			var accounts []fints.Account
			stage = "accounts_and_login"
			accounts, e = client.Accounts(ctx)
			if e == nil {
				stage = "save_accounts"
				skipped, e = s.store.saveAccounts(c, accounts)
				if e == nil && skipped > 0 {
					if n, countErr := s.store.accountCount(c.ID); countErr != nil {
						e = countErr
					} else if n == 0 {
						e = errAccountsKnown
					}
				}
			}
			if e == nil {
				var methods []fints.TANMethod
				stage = "tan_methods"
				methods, e = client.TANMethods(ctx)
				if e == nil {
					s.mu.Lock()
					j.result.Methods = methods
					s.mu.Unlock()
				}
			}
		} else {
			var bookings []fints.Transaction
			stage = "transactions_and_login"
			bookings, e = client.Transactions(ctx, a.Data, from, to)
			if e == nil && ctx.Err() == nil {
				var added int
				stage = "import_bookings"
				added, e = s.store.importBookings(ctx, *a, bookings, from, to)
				importCommitted = e == nil
				s.mu.Lock()
				j.result.Added = added
				s.mu.Unlock()
			}
			if importCommitted {
				balancePhase.Store(true)
				balanceNote = s.fetchBalance(ctx, client, *a)
				balancePhase.Store(false)
			}
		}
	}
	if stateClient, ok := client.(interface{ State() fints.ClientState }); ok && (ctx.Err() == nil || importCommitted) && !errors.Is(e, errAccountsKnown) {
		state := stateClient.State()
		c.SystemID = state.SystemID
		c.SecurityFunction = state.SecurityFunction
		c.TANMedium = state.TANMedium
		if persistErr := s.store.saveConnection(c); e == nil && !importCommitted {
			e = persistErr
			if e != nil {
				stage = "save_connection"
			}
		}
	}
	if ctx.Err() != nil && !importCommitted {
		e = ctx.Err()
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	j.result.Challenge = nil
	if e != nil {
		j.result.Status = "error"
		if errors.Is(e, context.Canceled) {
			j.result.Status = "cancelled"
		}
		j.result.Message = safeError(e)
		if errors.Is(e, errAccountsKnown) {
			// Nothing new came of this login: drop it and its PIN again.
			_ = s.store.deleteConnection(c.ID)
			_ = s.vault.Delete(c.ID)
		} else if !errors.Is(e, context.Canceled) {
			diagnostic := makeBankDiagnostic(stage, c.SecurityFunction, e)
			if source, ok := client.(interface{ DiagnosticTrace() fints.ExchangeTrace }); ok {
				trace := source.DiagnosticTrace()
				diagnostic.Trace = &trace
			}
			if err := writeBankDiagnostic(s.diagnosticPath, diagnostic); err != nil {
				j.result.Message += " Die lokale Fehlerdiagnose konnte nicht gespeichert werden."
			}
		}
		var medium *fints.TANMediumRequiredError
		if errors.As(e, &medium) {
			j.result.Media = medium.Media
		}
	} else {
		j.result.Status = "done"
		if a == nil {
			j.result.Message = "Konten gefunden. Wähle ein Konto zum Abruf aus."
			if skipped > 0 {
				j.result.Message += " Bereits eingerichtete Konten wurden nicht doppelt angelegt."
			}
		} else {
			j.result.Message = fmt.Sprintf("%d neue Buchungen übernommen.", j.result.Added) + balanceNote
		}
	}
}

// fetchBalance is optional: transactions are already committed, and a missing
// balance only means the manually entered one stays in use. The closing
// balance of the statement comes first; a separate balance query (HKSAL) is
// only the fallback for banks that send transactions without it.
func (s *Service) fetchBalance(ctx context.Context, client readClient, a Account) string {
	if ctx.Err() != nil {
		return ""
	}
	var b *fints.AccountBalance
	source := BalanceSourceStatement
	if sc, ok := client.(statementBalanceClient); ok {
		if closing := sc.StatementBalance(); closing != nil {
			b = &fints.AccountBalance{Booked: *closing}
		}
	}
	if b == nil {
		bc, ok := client.(balanceClient)
		if !ok {
			return " Der Kontostand wurde von der Bank nicht übermittelt."
		}
		balanceCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
		defer cancel()
		var err error
		if b, err = bc.Balance(balanceCtx, a.Data); err != nil || b == nil {
			return " Der Kontostand wurde von der Bank nicht übermittelt."
		}
		source = BalanceSourceQuery
	}
	if err := s.store.saveBalance(a.ID, *b, source, time.Now()); err != nil {
		return " Der Kontostand konnte nicht gespeichert werden."
	}
	return " Kontostand aktualisiert."
}
func safeError(e error) string {
	if errors.Is(e, context.Canceled) {
		return "Bankabruf abgebrochen."
	}
	if errors.Is(e, context.DeadlineExceeded) {
		return "Zeitlimit erreicht. Starte den Abruf noch einmal."
	}
	if errors.Is(e, errAccountsKnown) {
		return "Alle Konten dieses Zugangs sind bereits eingerichtet. Öffne sie unter Konten, um Umsätze abzurufen."
	}
	var conflict *ImportConflictError
	if errors.As(e, &conflict) {
		return "Die Bank liefert für den " + conflict.Date + " abweichende oder unvollständige Buchungen. Es wurde nichts neu übernommen. Prüfe diesen Tag anhand deines Kontoauszugs. Vorhandene Buchungen bleiben erhalten."
	}
	var medium *fints.TANMediumRequiredError
	if errors.As(e, &medium) {
		return "Wähle ein TAN-Gerät aus und verbinde dich noch einmal."
	}
	var be *fints.BankError
	if errors.As(e, &be) {
		codes := diagnosticBankCodes(be)
		if len(codes) == 0 {
			return "Die Bank hat den Abruf abgelehnt (Code unbekannt)."
		}
		label := "Code "
		if len(codes) > 1 {
			label = "Codes "
		}
		return "Die Bank hat den Abruf abgelehnt (" + label + strings.Join(codes, ", ") + ")."
	}
	for _, failure := range fints.FailureDiagnostics(e) {
		switch failure.Reason {
		case "ambiguous_tan_response":
			return "Die TAN-Anforderung der Bank konnte dem gesendeten Auftrag nicht eindeutig zugeordnet werden. Die Freigabe wurde angehalten."
		case "missing_tan_reference":
			return "Die Bankantwort enthält keine verwendbare TAN-Auftragsreferenz. Die Freigabe konnte nicht fortgesetzt werden."
		case "missing_dialog_id", "missing_system_id":
			return "Die Bankantwort enthält keine verwendbare Dialog- oder Systemkennung. Die Anmeldung konnte nicht abgeschlossen werden."
		case "tan_method_not_allowed":
			return "Das ausgewählte TAN-Verfahren ist laut Bankantwort für diesen Zugang nicht freigegeben."
		case "no_accounts":
			return "Die Bank hat keine abrufbaren Konten zurückgegeben."
		case "http_status":
			return fmt.Sprintf("Der Bankserver hat mit HTTP-Status %d geantwortet. Die FinTS-Anfrage konnte nicht verarbeitet werden.", failure.HTTPStatus)
		case "invalid_base64", "invalid_protocol_data":
			return "Die Antwort des Bankservers konnte nicht als FinTS-Nachricht gelesen werden."
		case "automatic_polling_disabled":
			return "Das gewählte TAN-Verfahren erlaubt laut Bank keine automatische Freigabeabfrage."
		case "poll_limit_reached":
			return "Die Bank hat die Freigabe innerhalb der erlaubten Anzahl von Abfragen nicht bestätigt."
		}
	}
	return "Der Bankabruf ist fehlgeschlagen. Prüfe Bankadresse, TAN-Verfahren und Verbindung. Es wurden keine unvollständigen Umsätze übernommen."
}
func (s *Service) Status(id string) (Operation, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.active == nil || s.active.result.ID != id {
		return Operation{}, errors.New("Bankabruf nicht gefunden.")
	}
	r := s.active.result
	if r.Challenge != nil {
		c := *r.Challenge
		c.Image = append([]byte(nil), c.Image...)
		r.Challenge = &c
	}
	r.Methods = append([]fints.TANMethod{}, r.Methods...)
	r.Media = append([]fints.TANMedium{}, r.Media...)
	return r, nil
}
func (s *Service) SubmitTAN(id, challengeID, tan string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	j := s.active
	if j == nil || j.result.ID != id || j.result.Status != "tan" || j.result.Challenge == nil || j.result.Challenge.ID != challengeID {
		return errors.New("Diese TAN-Anfrage ist nicht mehr aktuell.")
	}
	if strings.TrimSpace(tan) == "" {
		return errors.New("Gib eine TAN ein.")
	}
	select {
	case j.tan <- tan:
		j.result.Status = "running"
		j.result.Message = "Freigabe wird geprüft."
		j.result.Challenge = nil
		return nil
	default:
		return errors.New("Die TAN wurde bereits übermittelt.")
	}
}
func (s *Service) Cancel(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.active == nil || s.active.result.ID != id {
		return errors.New("Bankabruf nicht gefunden.")
	}
	s.active.cancel()
	return nil
}

// RemoveAccount deletes an account with its bookings from this app. The bank
// keeps its data, so the account can be set up and retrieved again. The caller
// makes sure no booking is still assigned.
func (s *Service) RemoveAccount(id string) (RemovedAccount, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if e := s.available(); e != nil {
		return RemovedAccount{}, e
	}
	return s.store.removeAccount(id, func(c Connection) error {
		if e := s.vault.Delete(c.ID); e != nil {
			return errors.New("Die gespeicherte PIN konnte nicht aus dem Schlüsselspeicher deines Betriebssystems entfernt werden.")
		}
		return nil
	})
}

// SetAccountLabel names an account; an empty name restores the default.
func (s *Service) SetAccountLabel(id, label string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	label, err := NormalizeAccountLabel(label)
	if err != nil {
		return err
	}
	if e := s.store.setAccountLabel(id, label); e != nil {
		if errors.Is(e, sql.ErrNoRows) {
			return ErrAccountNotFound
		}
		return e
	}
	return nil
}

// NormalizeAccountLabel collapses whitespace and checks the length of an
// account name; an empty name means the account has none.
func NormalizeAccountLabel(label string) (string, error) {
	label = strings.Join(strings.Fields(label), " ")
	if utf8.RuneCountInString(label) > maxAccountLabel {
		return "", fmt.Errorf("Der Kontoname darf höchstens %d Zeichen lang sein.", maxAccountLabel)
	}
	return label, nil
}
func (s *Service) Bookings(id string) ([]Booking, error) { return s.store.bookings(id) }

// ForgetPIN removes only the credential; imported bookings remain available offline.
func (s *Service) ForgetPIN(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if e := s.available(); e != nil {
		return e
	}
	c, e := s.store.connection(id)
	if e != nil {
		return errors.New("Bankverbindung nicht gefunden.")
	}
	// Delete even if metadata says no PIN is stored: it may be stale after a
	// previously interrupted save or external database restore.
	if e = s.vault.Delete(id); e != nil {
		return errors.New("Die PIN konnte nicht aus dem Schlüsselspeicher deines Betriebssystems entfernt werden.")
	}
	c.RememberPIN = false
	return s.store.saveConnection(c)
}
