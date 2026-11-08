// Package banking provides local-only, read-only FinTS synchronization.
package banking

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"sort"
	"strconv"
	"strings"
	"time"

	fints "github.com/BergerAPI/go-fints"
	"github.com/google/uuid"
	_ "modernc.org/sqlite"
)

type Connection struct {
	ID               string `json:"id"`
	Label            string `json:"label"`
	Endpoint         string `json:"endpoint"`
	BankCode         string `json:"bankCode"`
	UserID           string `json:"userId"`
	CustomerID       string `json:"customerId"`
	SecurityFunction string `json:"securityFunction"`
	TANMedium        string `json:"tanMedium"`
	SystemID         string `json:"-"`
	RememberPIN      bool   `json:"rememberPin"`
}
type Account struct {
	ID           string        `json:"id"`
	ConnectionID string        `json:"connectionId"`
	Data         fints.Account `json:"data"`
	LastSync     string        `json:"lastSync"`
	// Label is the name the user gave the account; empty until set.
	Label string `json:"label"`
	// Balance is the last balance the bank reported; nil if never fetched.
	Balance *Balance `json:"balance,omitempty"`
}

// Balance keeps the bank's own date for the booked balance and when the app
// fetched it, so stale values stay recognisable.
type Balance struct {
	Booked    fints.DatedBalance `json:"booked"`
	Available *fints.Money       `json:"available,omitempty"`
	FetchedAt string             `json:"fetchedAt"`
	// Source is BalanceSourceStatement or BalanceSourceQuery.
	Source string `json:"source"`
}

const (
	// BalanceSourceStatement is the closing balance sent with the transactions.
	BalanceSourceStatement = "statement"
	// BalanceSourceQuery is the bank's answer to a separate balance query.
	BalanceSourceQuery = "query"
)

type Booking struct {
	ID        string            `json:"id"`
	AccountID string            `json:"accountId"`
	Data      fints.Transaction `json:"data"`
}
type Overview struct {
	Connections []Connection `json:"connections"`
	Accounts    []Account    `json:"accounts"`
	// ProductSource names where the FinTS product ID comes from ("" if none).
	ProductSource string `json:"productSource"`
	// CSVLabels names accounts known only from CSV imports, by their IBAN.
	CSVLabels map[string]string `json:"csvLabels"`
}
type localStore struct{ db *sql.DB }

// newStore creates the banking tables in the shared application database.
func newStore(db *sql.DB) (*localStore, error) {
	_, err := db.Exec(`
 CREATE TABLE IF NOT EXISTS bank_settings (id INTEGER PRIMARY KEY CHECK(id=1), product_id TEXT NOT NULL);
 CREATE TABLE IF NOT EXISTS bank_connections (id TEXT PRIMARY KEY, data BLOB NOT NULL, system_id TEXT NOT NULL DEFAULT '');
 CREATE TABLE IF NOT EXISTS bank_accounts (id TEXT PRIMARY KEY, connection_id TEXT NOT NULL REFERENCES bank_connections(id), data BLOB NOT NULL, last_sync TEXT NOT NULL DEFAULT '');
 CREATE TABLE IF NOT EXISTS bank_bookings (id TEXT PRIMARY KEY, account_id TEXT NOT NULL REFERENCES bank_accounts(id), booking_date TEXT NOT NULL, data BLOB NOT NULL);
 CREATE INDEX IF NOT EXISTS bank_bookings_account ON bank_bookings(account_id,booking_date);
 CREATE TABLE IF NOT EXISTS bank_balances (account_id TEXT PRIMARY KEY REFERENCES bank_accounts(id), data BLOB NOT NULL);`)
	if err != nil {
		return nil, err
	}
	// Databases from before accounts could be named lack the column.
	var named int
	if err = db.QueryRow(`SELECT COUNT(*) FROM pragma_table_info('bank_accounts') WHERE name='label'`).Scan(&named); err != nil {
		return nil, err
	}
	if named == 0 {
		if _, err = db.Exec(`ALTER TABLE bank_accounts ADD COLUMN label TEXT NOT NULL DEFAULT ''`); err != nil {
			return nil, err
		}
	}
	return &localStore{db}, nil
}
func (s *localStore) overview() (Overview, error) {
	o := Overview{Connections: []Connection{}, Accounts: []Account{}}
	rows, err := s.db.Query("SELECT data FROM bank_connections ORDER BY id")
	if err != nil {
		return o, err
	}
	for rows.Next() {
		var b []byte
		var c Connection
		if err = rows.Scan(&b); err != nil {
			break
		}
		if err = json.Unmarshal(b, &c); err != nil {
			break
		}
		o.Connections = append(o.Connections, c)
	}
	if err == nil {
		err = rows.Err()
	}
	rows.Close()
	if err != nil {
		return o, err
	}
	rows, err = s.db.Query("SELECT a.id,a.connection_id,a.data,a.last_sync,a.label,b.data FROM bank_accounts a LEFT JOIN bank_balances b ON b.account_id=a.id ORDER BY a.id")
	if err != nil {
		return o, err
	}
	defer rows.Close()
	for rows.Next() {
		var a Account
		var b, balance []byte
		if err = rows.Scan(&a.ID, &a.ConnectionID, &b, &a.LastSync, &a.Label, &balance); err != nil {
			return o, err
		}
		if err = json.Unmarshal(b, &a.Data); err != nil {
			return o, err
		}
		if len(balance) > 0 {
			a.Balance = &Balance{}
			if err = json.Unmarshal(balance, a.Balance); err != nil {
				return o, err
			}
		}
		o.Accounts = append(o.Accounts, a)
	}
	return o, rows.Err()
}
func (s *localStore) connection(id string) (Connection, error) {
	var c Connection
	var b []byte
	var system string
	err := s.db.QueryRow("SELECT data,system_id FROM bank_connections WHERE id=?", id).Scan(&b, &system)
	if err != nil {
		return c, err
	}
	err = json.Unmarshal(b, &c)
	c.SystemID = system
	return c, err
}
func (s *localStore) saveConnection(c Connection) error {
	b, e := json.Marshal(c)
	if e != nil {
		return e
	}
	_, e = s.db.Exec("INSERT INTO bank_connections(id,data,system_id) VALUES(?,?,?) ON CONFLICT(id) DO UPDATE SET data=excluded.data,system_id=excluded.system_id", c.ID, b, c.SystemID)
	return e
}

// accountKey identifies an account at the bank, whichever connection found it.
func accountKey(a fints.Account) string {
	if iban := strings.ToUpper(strings.Join(strings.Fields(a.IBAN), "")); iban != "" {
		return iban
	}
	return a.CountryCode + ":" + a.BankCode + ":" + a.AccountNumber + ":" + a.Subaccount
}

// saveAccounts stores the accounts a connection found. An account another
// connection already holds is skipped, so one account never exists twice with
// its bookings; the number of skipped accounts is returned.
func (s *localStore) saveAccounts(c Connection, accounts []fints.Account) (int, error) {
	tx, e := s.db.Begin()
	if e != nil {
		return 0, e
	}
	defer tx.Rollback()
	held := map[string]bool{}
	rows, e := tx.Query("SELECT data FROM bank_accounts WHERE connection_id<>?", c.ID)
	if e != nil {
		return 0, e
	}
	for rows.Next() {
		var b []byte
		var a fints.Account
		if e = rows.Scan(&b); e == nil {
			e = json.Unmarshal(b, &a)
		}
		if e != nil {
			rows.Close()
			return 0, e
		}
		held[accountKey(a)] = true
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		return 0, e
	}
	skipped := 0
	for _, a := range accounts {
		key := accountKey(a)
		if held[key] {
			skipped++
			continue
		}
		id := hash(c.ID + ":" + key)
		b, e := json.Marshal(a)
		if e != nil {
			return 0, e
		}
		if _, e = tx.Exec("INSERT INTO bank_accounts(id,connection_id,data) VALUES(?,?,?) ON CONFLICT(id) DO UPDATE SET data=excluded.data", id, c.ID, b); e != nil {
			return 0, e
		}
	}
	return skipped, tx.Commit()
}

// accountCount is the number of accounts a connection holds.
func (s *localStore) accountCount(connectionID string) (int, error) {
	var n int
	err := s.db.QueryRow("SELECT COUNT(*) FROM bank_accounts WHERE connection_id=?", connectionID).Scan(&n)
	return n, err
}

// duplicateConnection finds an already working connection with the same
// login at the same bank. One that found no accounts is a failed attempt and
// does not count.
func (s *localStore) duplicateConnection(c Connection) (Connection, bool, error) {
	o, err := s.overview()
	if err != nil {
		return Connection{}, false, err
	}
	login := func(c Connection) [3]string {
		user := strings.ToLower(strings.TrimSpace(c.UserID))
		customer := strings.ToLower(strings.TrimSpace(c.CustomerID))
		if customer == "" {
			customer = user
		}
		return [3]string{strings.TrimSpace(c.BankCode), user, customer}
	}
	for _, known := range o.Connections {
		if known.ID == c.ID || login(known) != login(c) {
			continue
		}
		for _, a := range o.Accounts {
			if a.ConnectionID == known.ID {
				return known, true, nil
			}
		}
	}
	return Connection{}, false, nil
}

// deleteConnection removes a connection that holds no accounts.
func (s *localStore) deleteConnection(id string) error {
	_, err := s.db.Exec("DELETE FROM bank_connections WHERE id=? AND NOT EXISTS (SELECT 1 FROM bank_accounts WHERE connection_id=?)", id, id)
	return err
}
func (s *localStore) setAccountLabel(id, label string) error {
	res, err := s.db.Exec("UPDATE bank_accounts SET label=? WHERE id=?", label, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return sql.ErrNoRows
	}
	return nil
}
func (s *localStore) account(id string) (Account, error) {
	var a Account
	var b []byte
	e := s.db.QueryRow("SELECT id,connection_id,data,last_sync,label FROM bank_accounts WHERE id=?", id).Scan(&a.ID, &a.ConnectionID, &b, &a.LastSync, &a.Label)
	if e == nil {
		e = json.Unmarshal(b, &a.Data)
	}
	return a, e
}
func (s *localStore) saveBalance(accountID string, b fints.AccountBalance, source string, fetchedAt time.Time) error {
	raw, err := json.Marshal(Balance{Booked: b.Booked, Available: b.Available, Source: source, FetchedAt: fetchedAt.UTC().Format(time.RFC3339)})
	if err != nil {
		return err
	}
	_, err = s.db.Exec("INSERT INTO bank_balances(account_id,data) VALUES(?,?) ON CONFLICT(account_id) DO UPDATE SET data=excluded.data", accountID, raw)
	return err
}
func hash(s string) string { h := sha256.Sum256([]byte(s)); return hex.EncodeToString(h[:]) }

// ImportConflictError means a formerly imported booking is absent from a
// complete overlapping bank response. Refuse the whole batch rather than guess
// whether it is a removed payment, amended metadata, or truncated bank history.
type ImportConflictError struct{ Date string }

func (e *ImportConflictError) Error() string {
	return "bank history differs from previously imported bookings on " + e.Date
}

func usableBankReference(reference string) bool {
	switch strings.ToUpper(strings.TrimSpace(reference)) {
	case "", "NONREF", "NREF", "NOTPROVIDED", "NOT PROVIDED", "NOTAVAILABLE", "NOT AVAILABLE", "0":
		return false
	}
	return true
}
func bookingFingerprint(a Account, b fints.Transaction, raw []byte) string {
	if usableBankReference(b.BankReference) {
		// A bank reference can be shared by a batch. Retain date, amount, currency,
		// and an occurrence counter; never collapse that batch to one payment.
		identity, _ := json.Marshal([]string{b.BankReference, b.BookingDate, b.Currency, strconv.FormatInt(b.Amount, 10)})
		return hash(a.ID + ":reference:" + string(identity))
	}
	return hash(a.ID + ":content:" + string(raw))
}

// importBookings requires a complete booked-history response for the requested
// inclusive dates. A stable bank reference permits metadata refresh; without
// it changed records require review. All writes, including last_sync, are atomic.
func (s *localStore) importBookings(ctx context.Context, a Account, bookings []fints.Transaction, window ...time.Time) (int, error) {
	type prepared struct {
		id, fp, date string
		raw          []byte
	}
	preparedRows := make([]prepared, 0, len(bookings))
	from, to := "", ""
	if len(window) == 2 {
		from, to = window[0].Format(time.DateOnly), window[1].Format(time.DateOnly)
	}
	for _, b := range bookings {
		if _, err := time.Parse(time.DateOnly, b.BookingDate); err != nil {
			return 0, errors.New("invalid booking date")
		}
		if len(window) == 2 && (b.BookingDate < from || b.BookingDate > to) {
			return 0, errors.New("bank returned bookings outside requested date window")
		}
		if len(window) != 2 {
			if from == "" || b.BookingDate < from {
				from = b.BookingDate
			}
			if to == "" || b.BookingDate > to {
				to = b.BookingDate
			}
		}
		raw, err := json.Marshal(b)
		if err != nil {
			return 0, err
		}
		preparedRows = append(preparedRows, prepared{fp: bookingFingerprint(a, b, raw), date: b.BookingDate, raw: raw})
	}
	// Stable ordering also makes refreshed metadata deterministic when references
	// are shared by multiple equal payments.
	sort.Slice(preparedRows, func(i, j int) bool {
		if preparedRows[i].fp != preparedRows[j].fp {
			return preparedRows[i].fp < preparedRows[j].fp
		}
		return string(preparedRows[i].raw) < string(preparedRows[j].raw)
	})
	counts := map[string]int{}
	incoming := map[string]bool{}
	for i := range preparedRows {
		r := &preparedRows[i]
		counts[r.fp]++
		r.id = hash(r.fp + ":" + strconv.Itoa(counts[r.fp]))
		incoming[r.id] = true
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	existing := map[string]bool{}
	rows, err := tx.QueryContext(ctx, "SELECT id,booking_date FROM bank_bookings WHERE account_id=? AND booking_date>=? AND booking_date<=?", a.ID, from, to)
	if err != nil {
		return 0, err
	}
	for rows.Next() {
		var id, date string
		if err = rows.Scan(&id, &date); err != nil {
			rows.Close()
			return 0, err
		}
		if !incoming[id] {
			rows.Close()
			return 0, &ImportConflictError{Date: date}
		}
		existing[id] = true
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return 0, err
	}
	added := 0
	for _, r := range preparedRows {
		_, err = tx.ExecContext(ctx, "INSERT INTO bank_bookings(id,account_id,booking_date,data) VALUES(?,?,?,?) ON CONFLICT(id) DO UPDATE SET data=excluded.data", r.id, a.ID, r.date, r.raw)
		if err != nil {
			return 0, err
		}
		if !existing[r.id] {
			added++
		}
	}
	if _, err = tx.ExecContext(ctx, "UPDATE bank_accounts SET last_sync=? WHERE id=?", time.Now().UTC().Format(time.RFC3339), a.ID); err != nil {
		return 0, err
	}
	if err = ctx.Err(); err != nil {
		return 0, err
	}
	if err = tx.Commit(); err != nil {
		return 0, err
	}
	return added, nil
}
func (s *localStore) bookings(id string) ([]Booking, error) {
	out := []Booking{}
	rows, e := s.db.Query("SELECT id,data FROM bank_bookings WHERE account_id=? ORDER BY booking_date DESC,id", id)
	if e != nil {
		return out, e
	}
	defer rows.Close()
	for rows.Next() {
		b := Booking{AccountID: id}
		var raw []byte
		if e = rows.Scan(&b.ID, &raw); e != nil {
			return nil, e
		}
		if e = json.Unmarshal(raw, &b.Data); e != nil {
			return nil, e
		}
		out = append(out, b)
	}
	return out, rows.Err()
}
func newID() string { return uuid.NewString() }

// ErrAccountNotFound reports an account the store does not hold.
var ErrAccountNotFound = errors.New("Das Konto wurde nicht gefunden.")

// RemovedAccount reports what removing an account took with it.
type RemovedAccount struct {
	Name     string `json:"name"`
	IBAN     string `json:"iban"`
	Bookings int    `json:"bookings"`
	From     string `json:"from"`
	To       string `json:"to"`
	// ConnectionRemoved is set when it was the connection's last account.
	ConnectionRemoved bool `json:"connectionRemoved"`
}

// removeAccount deletes an account with its bookings and balance. The
// connection goes with its last account; dropPIN runs before that and a
// failure keeps everything as it was.
func (s *localStore) removeAccount(id string, dropPIN func(Connection) error) (RemovedAccount, error) {
	var r RemovedAccount
	tx, err := s.db.Begin()
	if err != nil {
		return r, err
	}
	defer tx.Rollback()
	var connectionID, label string
	var raw []byte
	err = tx.QueryRow("SELECT connection_id,data,label FROM bank_accounts WHERE id=?", id).Scan(&connectionID, &raw, &label)
	if errors.Is(err, sql.ErrNoRows) {
		return r, ErrAccountNotFound
	}
	if err != nil {
		return r, err
	}
	var data fints.Account
	if err = json.Unmarshal(raw, &data); err != nil {
		return r, err
	}
	var first, last sql.NullString
	if err = tx.QueryRow("SELECT COUNT(*),MIN(booking_date),MAX(booking_date) FROM bank_bookings WHERE account_id=?", id).Scan(&r.Bookings, &first, &last); err != nil {
		return r, err
	}
	r.From, r.To = first.String, last.String
	r.IBAN = data.IBAN
	var c Connection
	if err = tx.QueryRow("SELECT data FROM bank_connections WHERE id=?", connectionID).Scan(&raw); err != nil {
		return r, err
	}
	if err = json.Unmarshal(raw, &c); err != nil {
		return r, err
	}
	r.Name = strings.TrimSpace(label)
	if r.Name == "" {
		r.Name = c.Label
	}
	for _, statement := range []string{
		"DELETE FROM bank_bookings WHERE account_id=?",
		"DELETE FROM bank_balances WHERE account_id=?",
		"DELETE FROM bank_accounts WHERE id=?",
	} {
		if _, err = tx.Exec(statement, id); err != nil {
			return r, err
		}
	}
	var left int
	if err = tx.QueryRow("SELECT COUNT(*) FROM bank_accounts WHERE connection_id=?", connectionID).Scan(&left); err != nil {
		return r, err
	}
	if left == 0 {
		if c.RememberPIN {
			if err = dropPIN(c); err != nil {
				return r, err
			}
		}
		if _, err = tx.Exec("DELETE FROM bank_connections WHERE id=?", connectionID); err != nil {
			return r, err
		}
		r.ConnectionRemoved = true
	}
	return r, tx.Commit()
}
