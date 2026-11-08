package store

import (
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// Festschreibung: a period the user closed, or whose return was transmitted,
// no longer changes. Receipts recorded before that moment can then only be
// cancelled (GoBD Rz. 85, 93). A receipt found later can still be recorded; the
// next closing covers it.

const lockTables = `
CREATE TABLE IF NOT EXISTS period_locks (id TEXT PRIMARY KEY, from_date TEXT NOT NULL, to_date TEXT NOT NULL, at TEXT NOT NULL);
CREATE TRIGGER IF NOT EXISTS period_locks_immutable_update BEFORE UPDATE ON period_locks BEGIN SELECT RAISE(ABORT, 'Eine Festschreibung lässt sich nicht ändern.'); END;
CREATE TRIGGER IF NOT EXISTS period_locks_immutable_delete BEFORE DELETE ON period_locks BEGIN SELECT RAISE(ABORT, 'Eine Festschreibung lässt sich nicht aufheben.'); END;`

// PeriodLock closes the days from From (empty: from the beginning) to To.
type PeriodLock struct {
	ID   string `json:"id"`
	From string `json:"from"`
	To   string `json:"to"`
	At   string `json:"at"`
}

func lockSum(l PeriodLock) string { return sumOf([]byte(stateJSON(l))) }

func lockLabel(l PeriodLock) string {
	if l.From == "" {
		return "Festschreibung bis " + l.To
	}
	return fmt.Sprintf("Festschreibung %s bis %s", l.From, l.To)
}

var germanMonths = []string{"Januar", "Februar", "März", "April", "Mai", "Juni", "Juli", "August", "September", "Oktober", "November", "Dezember"}

// FilingPeriod reads a period ID of a filing note: 2026, 2026-03 or 2026-Q1.
func FilingPeriod(id string) (from, to, label string, ok bool) {
	year, err := strconv.Atoi(id[:min(4, len(id))])
	if err != nil || len(id) < 4 {
		return "", "", "", false
	}
	first, last := 1, 12
	label = strconv.Itoa(year)
	switch rest := id[4:]; {
	case rest == "":
	case len(rest) == 3 && rest[:2] == "-Q":
		q, err := strconv.Atoi(rest[2:])
		if err != nil || q < 1 || q > 4 {
			return "", "", "", false
		}
		first, last, label = 3*q-2, 3*q, fmt.Sprintf("%d. Quartal %d", q, year)
	case len(rest) == 3 && rest[0] == '-':
		m, err := strconv.Atoi(rest[1:])
		if err != nil || m < 1 || m > 12 {
			return "", "", "", false
		}
		first, last, label = m, m, fmt.Sprintf("%s %d", germanMonths[m-1], year)
	default:
		return "", "", "", false
	}
	start := time.Date(year, time.Month(first), 1, 0, 0, 0, 0, time.UTC)
	end := time.Date(year, time.Month(last)+1, 0, 0, 0, 0, 0, time.UTC)
	return start.Format(time.DateOnly), end.Format(time.DateOnly), label, true
}

func readLocks(q sqlRunner) ([]PeriodLock, error) {
	rows, err := q.Query("SELECT id,from_date,to_date,at FROM period_locks ORDER BY to_date,at,id")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []PeriodLock{}
	for rows.Next() {
		var l PeriodLock
		if err = rows.Scan(&l.ID, &l.From, &l.To, &l.At); err != nil {
			return nil, err
		}
		out = append(out, l)
	}
	return out, rows.Err()
}

// PeriodLocks lists the periods closed by hand.
func (s *Store) PeriodLocks() ([]PeriodLock, error) { return readLocks(s.db) }

// lockRule is one closed period, by hand or through a transmitted return.
type lockRule struct {
	From, To, At, Why string
}

func lockRules(q sqlRunner) ([]lockRule, error) {
	locks, err := readLocks(q)
	if err != nil {
		return nil, err
	}
	rules := []lockRule{}
	for _, l := range locks {
		rules = append(rules, lockRule{l.From, l.To, l.At, fmt.Sprintf("Du hast den Zeitraum bis %s festgeschrieben.", germanDay(l.To))})
	}
	for _, kind := range []struct{ prefix, name string }{{FilingVAT, "Umsatzsteuer-Voranmeldung"}, {FilingAnnual, "Jahreserklärung"}} {
		rows, err := q.Query("SELECT key,value FROM settings WHERE key LIKE ?", kind.prefix+"%")
		if err != nil {
			return nil, err
		}
		for rows.Next() {
			var key string
			var raw []byte
			if err = rows.Scan(&key, &raw); err != nil {
				rows.Close()
				return nil, err
			}
			var f VATFiling
			if err = json.Unmarshal(raw, &f); err != nil {
				rows.Close()
				return nil, err
			}
			from, to, label, ok := FilingPeriod(strings.TrimPrefix(key, kind.prefix))
			if ok {
				rules = append(rules, lockRule{from, to, f.SavedAt, fmt.Sprintf("Die %s %s ist als übermittelt vermerkt.", kind.name, label)})
			}
		}
		if err = rows.Close(); err != nil {
			return nil, err
		}
	}
	return rules, nil
}

func germanDay(date string) string {
	t, err := time.Parse(time.DateOnly, date)
	if err != nil {
		return date
	}
	return t.Format("02.01.2006")
}

// lockedBy names why something with these dates may no longer change. A record
// made after the closing stays open until the next one.
func lockedBy(rules []lockRule, createdAt string, dates ...string) string {
	for _, r := range rules {
		if r.At != "" && createdAt > r.At {
			continue
		}
		for _, date := range dates {
			if date != "" && date <= r.To && (r.From == "" || date >= r.From) {
				return r.Why
			}
		}
	}
	return ""
}

// ClosedByHand names the period closed by hand that covers these dates, if any.
func (s *Store) ClosedByHand(createdAt string, dates ...string) (string, error) {
	locks, err := readLocks(s.db)
	if err != nil {
		return "", err
	}
	rules := []lockRule{}
	for _, l := range locks {
		rules = append(rules, lockRule{l.From, l.To, l.At, fmt.Sprintf("Du hast den Zeitraum bis %s festgeschrieben.", germanDay(l.To))})
	}
	return lockedBy(rules, createdAt, dates...), nil
}

// LockPeriod closes a period for good. The log notes how many receipts it covers
// and a hash of their content.
func (s *Store) LockPeriod(from, to string) (PeriodLock, error) {
	l := PeriodLock{From: from, To: to}
	if !validDate(to) || (from != "" && (!validDate(from) || from > to)) {
		return l, errors.New("Gib einen gültigen Zeitraum an.")
	}
	if to > time.Now().Format(time.DateOnly) {
		return l, errors.New("Du kannst nur abgelaufene oder heutige Tage festschreiben.")
	}
	tx, err := s.db.Begin()
	if err != nil {
		return l, err
	}
	defer tx.Rollback()
	expenses, err := readExpenses(tx)
	if err != nil {
		return l, err
	}
	covered, digest := 0, ""
	for _, e := range expenses {
		if e.Date <= to && (from == "" || e.Date >= from) {
			covered++
			digest += sumOf([]byte(stateJSON(e)))
		}
	}
	l.ID, l.At = newID("lock"), now()
	if _, err = tx.Exec("INSERT INTO period_locks(id,from_date,to_date,at) VALUES(?,?,?,?)", l.ID, l.From, l.To, l.At); err != nil {
		return l, err
	}
	summary := map[string]any{"from": from, "to": to, "receipts": covered, "contentHash": sumOf([]byte(digest))}
	if err = s.logChange(tx, AuditEntry{Action: AuditLocked, Object: AuditLock, ObjectID: l.ID, Label: lockLabel(l), After: stateJSON(summary), Sum: lockSum(l)}); err != nil {
		return l, err
	}
	return l, tx.Commit()
}
