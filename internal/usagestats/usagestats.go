// Package usagestats adds the usage statistics to the update check.
//
// The request for the latest release always names the app version and the
// operating system, so the update server can count checks by version and
// system. Nothing is stored for that on the device.
//
// With "Bei der Verbesserung helfen" on, the request also carries the month
// this installation was first counted and marks for "first check today / this
// week / this month". The server adds them to daily counters, which gives the
// exact number of helping installations per day, week and month without any
// identifier: nothing in the request tells two checks of one installation
// apart from checks of two. For the marks the installation keeps the dates it
// last sent each one in a small file in the data folder; with the setting off
// the file is neither read nor written, and Forget deletes it.
package usagestats

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"
)

// Header names, read by the update server.
const (
	HeaderUsage   = "X-Papierleicht-Statistik"
	HeaderVersion = "X-Papierleicht-Version"
	HeaderSystem  = "X-Papierleicht-System"
)

// FileName is the state file in the data folder.
const FileName = "nutzungsstatistik.json"

// state is what the installation remembers: the period it last sent each
// mark for and the month it was first counted.
type state struct {
	Since string `json:"since"`
	Day   string `json:"day,omitempty"`
	Week  string `json:"week,omitempty"`
	Month string `json:"month,omitempty"`
}

// Transport adds the statistics to requests for the latest release on the
// update server and passes every other request on unchanged.
type Transport struct {
	// Base sends the requests; nil uses http.DefaultTransport.
	Base http.RoundTripper
	// Host is the update server's host, e.g. "lizenz.papierleicht.de".
	Host string
	// Enabled reports "Bei der Verbesserung helfen" at the time of the check.
	Enabled func() bool
	// Version is the app version.
	Version string
	// File is the state file; see FileName.
	File string
	// Now is the clock in local time; nil uses time.Now.
	Now func() time.Time

	mu sync.Mutex
}

func (t *Transport) base() http.RoundTripper {
	if t.Base != nil {
		return t.Base
	}
	return http.DefaultTransport
}

// RoundTrip implements http.RoundTripper. The marks count as sent only when
// the server answered; after an error the next check sends them again.
func (t *Transport) RoundTrip(req *http.Request) (*http.Response, error) {
	if req.URL.Host != t.Host || !strings.HasSuffix(req.URL.Path, "/releases/latest") {
		return t.base().RoundTrip(req)
	}
	req = req.Clone(req.Context())
	req.Header.Set(HeaderVersion, t.Version)
	req.Header.Set(HeaderSystem, runtime.GOOS+"/"+runtime.GOARCH)
	if t.Enabled == nil || !t.Enabled() {
		return t.base().RoundTrip(req)
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	now := time.Now()
	if t.Now != nil {
		now = t.Now()
	}
	old := t.load()
	next, marks := advance(old, now)
	req.Header.Set(HeaderUsage, "v=1; neu="+strings.Join(marks, ",")+"; seit="+next.Since)
	resp, err := t.base().RoundTrip(req)
	if err == nil && (resp.StatusCode == http.StatusOK || resp.StatusCode == http.StatusNotFound) && next != old {
		// A lost write only means a mark is sent once more.
		_ = t.save(next)
	}
	return resp, err
}

// advance returns the new state and the marks for the periods that changed.
func advance(s state, now time.Time) (state, []string) {
	year, week := now.ISOWeek()
	day, month := now.Format(time.DateOnly), now.Format("2006-01")
	weekKey := fmt.Sprintf("%04d-W%02d", year, week)
	marks := []string{}
	if s.Since == "" {
		s.Since = month
	}
	if s.Day != day {
		s.Day = day
		marks = append(marks, "tag")
	}
	if s.Week != weekKey {
		s.Week = weekKey
		marks = append(marks, "woche")
	}
	if s.Month != month {
		s.Month = month
		marks = append(marks, "monat")
	}
	return s, marks
}

func (t *Transport) load() state {
	var s state
	raw, err := os.ReadFile(t.File)
	if err != nil || json.Unmarshal(raw, &s) != nil || len(s.Since) != 7 {
		return state{}
	}
	return s
}

func (t *Transport) save(s state) error {
	if t.File == "" {
		return errors.New("usage statistics: no state file")
	}
	raw, err := json.Marshal(s)
	if err != nil {
		return err
	}
	tmp := t.File + ".tmp"
	if err := os.WriteFile(tmp, raw, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, filepath.Clean(t.File))
}

// Forget deletes the dates of the last marks, so turning the help off leaves
// nothing of it on the device. A nil Transport does nothing.
func (t *Transport) Forget() error {
	if t == nil || t.File == "" {
		return nil
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if err := os.Remove(t.File); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}
