package umzug

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"mime"
	"net/http"
	"path"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Moving over the official API of a program: the customer enters an access
// key, the app reads contacts and documents with their files. It only reads;
// the key stays in memory for the one run.

// Errors of a remote program that the customer can act on.
var (
	ErrUnauthorized = errors.New("Der Schlüssel wurde nicht angenommen. Prüfe, ob er vollständig ist und ob dein Tarif die Schnittstelle enthält.")
	ErrNotFound     = errors.New("not found")
	ErrConflict     = errors.New("conflict")
)

// Client paces requests and repeats them when the program asks to wait.
type Client struct {
	Base     string
	Auth     func(*http.Request)
	Interval time.Duration
	// MaxInterval caps how far the client slows down after the program
	// asked it to wait; zero keeps the interval.
	MaxInterval time.Duration
	// Backoff is the first wait before a retry; zero means two seconds.
	Backoff time.Duration
	HTTP    *http.Client

	mu   sync.Mutex
	last time.Time
}

const maxDownload = MaxFileSize

func (c *Client) wait(ctx context.Context) error {
	c.mu.Lock()
	next := c.last.Add(c.Interval)
	now := time.Now()
	if next.Before(now) {
		next = now
	}
	c.last = next
	c.mu.Unlock()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(time.Until(next)):
		return nil
	}
}

// slowDown spaces the following requests further apart once the program
// said there were too many.
func (c *Client) slowDown() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.MaxInterval > c.Interval {
		c.Interval = min(max(c.Interval*2, 100*time.Millisecond), c.MaxInterval)
	}
}

// Get returns the body and the response headers.
func (c *Client) Get(ctx context.Context, path, accept string) ([]byte, http.Header, error) {
	client := c.HTTP
	if client == nil {
		client = &http.Client{Timeout: 2 * time.Minute}
	}
	backoff := c.Backoff
	if backoff <= 0 {
		backoff = 2 * time.Second
	}
	for attempt := 0; ; attempt++ {
		if err := c.wait(ctx); err != nil {
			return nil, nil, err
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.Base+path, nil)
		if err != nil {
			return nil, nil, err
		}
		if accept != "" {
			req.Header.Set("Accept", accept)
		}
		req.Header.Set("User-Agent", "papierleicht-umzug")
		if c.Auth != nil {
			c.Auth(req)
		}
		resp, err := client.Do(req)
		if err != nil {
			if attempt < 3 && ctx.Err() == nil {
				select {
				case <-ctx.Done():
					return nil, nil, ctx.Err()
				case <-time.After(backoff):
				}
				backoff *= 2
				continue
			}
			return nil, nil, fmt.Errorf("Keine Verbindung zum Programm: %w", err)
		}
		body, readErr := io.ReadAll(io.LimitReader(resp.Body, maxDownload+1))
		resp.Body.Close()
		switch {
		case resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500:
			if resp.StatusCode == http.StatusTooManyRequests {
				c.slowDown()
			}
			if attempt >= 5 {
				return nil, nil, fmt.Errorf("Das Programm antwortet gerade nicht (%d). Versuche es später noch einmal; schon Übernommenes bleibt.", resp.StatusCode)
			}
			delay := backoff
			if s, err := strconv.Atoi(resp.Header.Get("Retry-After")); err == nil && s > 0 && s < 600 {
				delay = time.Duration(s) * time.Second
			}
			backoff *= 2
			select {
			case <-ctx.Done():
				return nil, nil, ctx.Err()
			case <-time.After(delay):
			}
			continue
		case resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden:
			return nil, nil, ErrUnauthorized
		case resp.StatusCode == http.StatusNotFound:
			return nil, nil, ErrNotFound
		case resp.StatusCode == http.StatusConflict || resp.StatusCode == http.StatusNotAcceptable:
			return nil, nil, ErrConflict
		case resp.StatusCode >= 400:
			return nil, nil, fmt.Errorf("Das Programm hat die Anfrage abgelehnt (%d).", resp.StatusCode)
		}
		if readErr != nil {
			return nil, nil, readErr
		}
		if len(body) > maxDownload {
			return nil, nil, errors.New("Die Datei ist größer als 50 MB.")
		}
		return body, resp.Header, nil
	}
}

// GetJSON reads a JSON answer into v.
func (c *Client) GetJSON(ctx context.Context, path string, v any) error {
	body, _, err := c.Get(ctx, path, "application/json")
	if err != nil {
		return err
	}
	if err = json.Unmarshal(body, v); err != nil {
		return fmt.Errorf("Das Programm hat unerwartete Daten geschickt: %w", err)
	}
	return nil
}

// fileName takes the name a download announces, or the fallback with an
// extension that fits its content type.
func fileName(h http.Header, fallback string) string {
	if _, params, err := mime.ParseMediaType(h.Get("Content-Disposition")); err == nil {
		if name := path.Base(strings.ReplaceAll(params["filename"], "\\", "/")); name != "" && name != "." && name != "/" {
			return name
		}
	}
	fallback = strings.NewReplacer("/", "-", "\\", "-", ":", "-").Replace(strings.TrimSpace(fallback))
	if fallback == "" {
		fallback = "Beleg"
	}
	if path.Ext(fallback) != "" && len(path.Ext(fallback)) <= 5 {
		return fallback
	}
	media, _, _ := mime.ParseMediaType(h.Get("Content-Type"))
	switch {
	case media == "application/pdf":
		return fallback + ".pdf"
	case strings.HasSuffix(media, "xml"):
		return fallback + ".xml"
	case media == "image/png":
		return fallback + ".png"
	case media == "image/jpeg":
		return fallback + ".jpg"
	}
	return fallback
}

// flexNumber reads a number that programs send as number, string or null.
type flexNumber struct {
	Value float64
	Set   bool
}

func (n *flexNumber) UnmarshalJSON(b []byte) error {
	s := strings.Trim(strings.TrimSpace(string(b)), `"`)
	if s == "" || s == "null" {
		*n = flexNumber{}
		return nil
	}
	f, err := strconv.ParseFloat(strings.Replace(s, ",", ".", 1), 64)
	if err != nil {
		*n = flexNumber{}
		return nil
	}
	*n = flexNumber{Value: f, Set: true}
	return nil
}

// flexString reads an id or a text that programs send as string or number.
type flexString string

func (s *flexString) UnmarshalJSON(b []byte) error {
	var text string
	if err := json.Unmarshal(b, &text); err == nil {
		*s = flexString(text)
		return nil
	}
	raw := strings.TrimSpace(string(b))
	if raw == "null" {
		*s = ""
		return nil
	}
	*s = flexString(raw)
	return nil
}

// amount writes an amount as the archive keeps it: 119.00, without sign.
func amount(f float64) string {
	return strconv.FormatFloat(math.Abs(math.Round(f*100)/100), 'f', 2, 64)
}

// isoDate takes the date part of the formats programs send; a Unix time in
// seconds is read as well.
func isoDate(v string) string {
	v = strings.TrimSpace(v)
	if len(v) >= 10 && v[4] == '-' && v[7] == '-' {
		return v[:10]
	}
	if n, err := strconv.ParseInt(v, 10, 64); err == nil && n > 100000000 {
		return time.Unix(n, 0).In(berlin()).Format("2006-01-02")
	}
	if t, err := time.Parse("02.01.2006", v); err == nil {
		return t.Format("2006-01-02")
	}
	return ""
}

func berlin() *time.Location {
	if loc, err := time.LoadLocation("Europe/Berlin"); err == nil {
		return loc
	}
	return time.UTC
}

// RemoteRef is one document in the list of a program, before its files are loaded.
type RemoteRef struct {
	ID string
	// Kind is the archive kind: "invoice", "credit-note" or "document".
	Kind string
	// Type is the program's own kind of document, used to load its files.
	Type string
	// EInvoice says the program marks the document as an e-invoice.
	EInvoice bool
	Meta     Booking
}

// RemoteFile is a file of a remote document.
type RemoteFile struct {
	Name string
	Data []byte
}

// Provider reads one program over its API.
type Provider interface {
	// Check confirms the key and names the account.
	Check(ctx context.Context) (string, error)
	// Customers lists the customers as a table for the list dialog.
	Customers(ctx context.Context) (Table, error)
	// Documents lists the documents without their files.
	Documents(ctx context.Context) ([]RemoteRef, error)
	// Files loads the files of one document.
	Files(ctx context.Context, ref RemoteRef) ([]RemoteFile, error)
}

// providers are the programs the app can read over their API, by source id.
var providers = map[string]func(key string) Provider{}

// RegisterProvider makes a program readable over its API; adapters register
// themselves, tests may add fakes.
func RegisterProvider(source string, f func(key string) Provider) {
	if f == nil {
		delete(providers, source)
		return
	}
	providers[source] = f
}

// HasAPI tells whether the app can read a program over its API.
func HasAPI(source string) bool { _, ok := providers[source]; return ok }

// NewProvider returns the API reader of a program.
func NewProvider(source, key string) (Provider, bool) {
	f, ok := providers[source]
	if !ok {
		return nil, false
	}
	return f(strings.TrimSpace(key)), true
}

// customerColumns are the columns of a customer table read over an API;
// their names are the ones GuessMapping knows.
var customerColumns = []string{"Firma", "Vorname", "Nachname", "Straße", "Adresszusatz", "PLZ", "Ort", "Land", "E-Mail", "USt-IdNr.", "Kundennummer"}

func customerRow(company, first, last, street, additional, zip, city, country, email, vat, number string) []string {
	return []string{company, first, last, street, additional, zip, city, country, email, vat, number}
}

// Group turns a loaded remote document into a group for Read.
func (r RemoteRef) Group(files []RemoteFile) Group {
	g := Group{Meta: &r.Meta}
	for _, f := range files {
		data := f.Data
		g.Entries = append(g.Entries, Entry{Origin: f.Name, Name: f.Name, Size: int64(len(data)), open: func() ([]byte, error) { return data, nil }})
	}
	return g
}
