package usagestats

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"regexp"
	"runtime"
	"sort"
	"sync"
	"time"
)

// Recorder counts which areas and functions of the app are used, when the
// user agreed to help improve the software ("Bei der Verbesserung helfen").
// It keeps one counter per event and day in a file in the data folder and
// sends the finished days to the update server, without any identifier.
// Turning the setting off deletes what was not sent yet.
type Recorder struct {
	// File is the counter file; see DetailsFileName.
	File string
	// Enabled reports the user's setting; nil means off.
	Enabled func() bool
	// URL receives the report, e.g. "https://lizenz.papierleicht.de/api/updates/usage".
	URL string
	// Version is the app version.
	Version string
	// Client sends the report; nil uses a client with a timeout.
	Client *http.Client
	// Now is the clock in local time; nil uses time.Now.
	Now func() time.Time

	mu sync.Mutex
}

// DetailsFileName is the counter file in the data folder.
const DetailsFileName = "nutzungsdetails.json"

// keepDays bounds the counters kept while the server cannot be reached; the
// server accepts no older days either.
const keepDays = 30

// eventPattern is "bereich:<area>" or "funktion:<function>".
var eventPattern = regexp.MustCompile(`^(bereich|funktion):[a-z0-9_]{1,40}$`)

type details struct {
	Days map[string]map[string]int `json:"days"`
}

// Report is the request body the server reads.
type Report struct {
	V       int                       `json:"v"`
	Version string                    `json:"version"`
	System  string                    `json:"system"`
	Days    map[string]map[string]int `json:"days"`
}

func (r *Recorder) now() time.Time {
	if r != nil && r.Now != nil {
		return r.Now()
	}
	return time.Now()
}

func (r *Recorder) enabled() bool {
	return r != nil && r.File != "" && r.Enabled != nil && r.Enabled()
}

// Record counts one use of event for today. It does nothing without the
// user's consent or for a name outside the pattern.
func (r *Recorder) Record(event string) {
	if !eventPattern.MatchString(event) || !r.enabled() {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	d := r.load()
	day := r.now().Format(time.DateOnly)
	if d.Days[day] == nil {
		d.Days[day] = map[string]int{}
	}
	d.Days[day][event]++
	r.prune(d)
	// A lost write only loses a count.
	_ = r.save(d)
}

// Send reports the finished days (before today) and forgets them once the
// server accepted them. Without consent it deletes the file instead.
func (r *Recorder) Send(ctx context.Context) error {
	if r == nil || r.File == "" {
		return nil
	}
	if !r.enabled() {
		return r.Forget()
	}
	r.mu.Lock()
	d := r.load()
	r.prune(d)
	today := r.now().Format(time.DateOnly)
	report := Report{V: 1, Version: r.Version, System: runtime.GOOS + "/" + runtime.GOARCH, Days: map[string]map[string]int{}}
	for day, events := range d.Days {
		if day < today && len(events) > 0 {
			report.Days[day] = events
		}
	}
	r.mu.Unlock()
	if len(report.Days) == 0 || r.URL == "" {
		return nil
	}
	body, err := json.Marshal(report)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, r.URL, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	client := r.Client
	if client == nil {
		client = &http.Client{Timeout: 30 * time.Second}
	}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		return fmt.Errorf("usage report: HTTP %d", resp.StatusCode)
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	d = r.load()
	for day := range report.Days {
		delete(d.Days, day)
	}
	return r.save(d)
}

// Forget deletes the counters not sent yet.
func (r *Recorder) Forget() error {
	if r == nil || r.File == "" {
		return nil
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if err := os.Remove(r.File); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

// prune keeps the newest keepDays days.
func (r *Recorder) prune(d *details) {
	if len(d.Days) <= keepDays {
		return
	}
	days := make([]string, 0, len(d.Days))
	for day := range d.Days {
		days = append(days, day)
	}
	sort.Strings(days)
	for _, day := range days[:len(days)-keepDays] {
		delete(d.Days, day)
	}
}

func (r *Recorder) load() *details {
	d := &details{}
	if raw, err := os.ReadFile(r.File); err == nil {
		_ = json.Unmarshal(raw, d)
	}
	if d.Days == nil {
		d.Days = map[string]map[string]int{}
	}
	return d
}

func (r *Recorder) save(d *details) error {
	if len(d.Days) == 0 {
		if err := os.Remove(r.File); err != nil && !os.IsNotExist(err) {
			return err
		}
		return nil
	}
	raw, err := json.Marshal(d)
	if err != nil {
		return err
	}
	tmp := r.File + ".tmp"
	if err := os.WriteFile(tmp, raw, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, r.File)
}
