package umzug

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"
)

// Lexware Office (formerly lexoffice) over its public API,
// https://developers.lexware.io/docs/. The key is a personal API key; the
// program allows two requests a second for all endpoints together.

func init() {
	RegisterProvider("lexware-office", func(key string) Provider { return newLexware("https://api.lexware.io", key) })
}

type lexware struct{ c *Client }

func newLexware(base, key string) *lexware {
	return &lexware{c: &Client{
		Base:        base,
		Auth:        func(r *http.Request) { r.Header.Set("Authorization", "Bearer "+key) },
		Interval:    600 * time.Millisecond,
		MaxInterval: 2 * time.Second,
	}}
}

func (l *lexware) Check(ctx context.Context) (string, error) {
	var p struct {
		CompanyName string `json:"companyName"`
	}
	if err := l.c.GetJSON(ctx, "/v1/profile", &p); err != nil {
		return "", err
	}
	return p.CompanyName, nil
}

type lexPage[T any] struct {
	Content       []T  `json:"content"`
	Last          bool `json:"last"`
	TotalPages    int  `json:"totalPages"`
	TotalElements int  `json:"totalElements"`
}

// lexAll reads all pages of a list; Lexware stops after 10,000 entries of one
// query, which the caller learns from the total.
func lexAll[T any](ctx context.Context, c *Client, path string, q url.Values) ([]T, int, error) {
	var out []T
	total := 0
	for page := 0; ; page++ {
		q.Set("page", strconv.Itoa(page))
		q.Set("size", "250")
		var p lexPage[T]
		if err := c.GetJSON(ctx, path+"?"+q.Encode(), &p); err != nil {
			return out, total, err
		}
		total = p.TotalElements
		out = append(out, p.Content...)
		if p.Last || len(p.Content) == 0 || page+1 >= p.TotalPages || len(out) >= lexWindow {
			return out, total, nil
		}
	}
}

const lexWindow = 10000

type lexContact struct {
	ID    string `json:"id"`
	Roles struct {
		Customer *struct {
			Number flexString `json:"number"`
		} `json:"customer"`
	} `json:"roles"`
	Company *struct {
		Name              string `json:"name"`
		VatRegistrationID string `json:"vatRegistrationId"`
		ContactPersons    []struct {
			FirstName    string `json:"firstName"`
			LastName     string `json:"lastName"`
			EmailAddress string `json:"emailAddress"`
			Primary      bool   `json:"primary"`
		} `json:"contactPersons"`
	} `json:"company"`
	Person *struct {
		FirstName string `json:"firstName"`
		LastName  string `json:"lastName"`
	} `json:"person"`
	Addresses struct {
		Billing []lexAddress `json:"billing"`
	} `json:"addresses"`
	EmailAddresses struct {
		Business []string `json:"business"`
		Office   []string `json:"office"`
		Private  []string `json:"private"`
		Other    []string `json:"other"`
	} `json:"emailAddresses"`
}

type lexAddress struct {
	Supplement  string `json:"supplement"`
	Street      string `json:"street"`
	Zip         string `json:"zip"`
	City        string `json:"city"`
	CountryCode string `json:"countryCode"`
}

func (l *lexware) Customers(ctx context.Context) (Table, error) {
	contacts, _, err := lexAll[lexContact](ctx, l.c, "/v1/contacts", url.Values{"customer": {"true"}})
	t := Table{Columns: customerColumns, Encoding: "API", Delimiter: "API"}
	if err != nil {
		return t, err
	}
	for _, c := range contacts {
		var company, first, last, vat, number string
		if c.Company != nil {
			company, vat = c.Company.Name, c.Company.VatRegistrationID
		}
		if c.Person != nil {
			first, last = c.Person.FirstName, c.Person.LastName
		}
		if c.Roles.Customer != nil {
			number = string(c.Roles.Customer.Number)
		}
		var a lexAddress
		if len(c.Addresses.Billing) > 0 {
			a = c.Addresses.Billing[0]
		}
		email := first1(c.EmailAddresses.Business, c.EmailAddresses.Office, c.EmailAddresses.Private, c.EmailAddresses.Other)
		if email == "" && c.Company != nil {
			for _, p := range c.Company.ContactPersons {
				if p.EmailAddress != "" && (p.Primary || email == "") {
					email = p.EmailAddress
				}
			}
		}
		t.Rows = append(t.Rows, customerRow(company, first, last, a.Street, a.Supplement, a.Zip, a.City, a.CountryCode, email, vat, number))
	}
	return t, nil
}

func first1(lists ...[]string) string {
	for _, l := range lists {
		for _, v := range l {
			if v = strings.TrimSpace(v); v != "" {
				return v
			}
		}
	}
	return ""
}

type lexVoucher struct {
	ID            string     `json:"id"`
	VoucherType   string     `json:"voucherType"`
	VoucherStatus string     `json:"voucherStatus"`
	VoucherNumber string     `json:"voucherNumber"`
	VoucherDate   string     `json:"voucherDate"`
	ContactName   string     `json:"contactName"`
	TotalAmount   flexNumber `json:"totalAmount"`
}

// lexTypes are the voucher types that are invoices or receipts; quotations,
// order confirmations and delivery notes stay in Lexware.
var lexTypes = map[string]struct{ kind, direction string }{
	"invoice":            {"invoice", "outgoing"},
	"downpaymentinvoice": {"invoice", "outgoing"},
	"creditnote":         {"credit-note", "outgoing"},
	"salesinvoice":       {"invoice", "outgoing"},
	"salescreditnote":    {"credit-note", "outgoing"},
	"purchaseinvoice":    {"invoice", "incoming"},
	"purchasecreditnote": {"credit-note", "incoming"},
}

func (l *lexware) Documents(ctx context.Context) ([]RemoteRef, error) {
	types := strings.Join(slices.Sorted(maps.Keys(lexTypes)), ",")
	seen := map[string]bool{}
	var refs []RemoteRef
	add := func(vs []lexVoucher) {
		for _, v := range vs {
			t, ok := lexTypes[v.VoucherType]
			if !ok || v.VoucherStatus == "draft" || seen[v.ID] {
				continue
			}
			seen[v.ID] = true
			ref := RemoteRef{ID: v.ID, Kind: t.kind, Type: v.VoucherType, Meta: Booking{
				Number: v.VoucherNumber, Date: isoDate(v.VoucherDate), Text: v.ContactName, Direction: t.direction, Source: "api",
			}}
			if v.TotalAmount.Set {
				ref.Meta.Gross = amount(v.TotalAmount.Value)
			}
			refs = append(refs, ref)
		}
	}
	query := func(archived string, from, to string) url.Values {
		q := url.Values{"voucherType": {types}, "voucherStatus": {"any"}, "archived": {archived}, "sort": {"voucherDate,ASC"}}
		if from != "" {
			q.Set("voucherDateFrom", from)
			q.Set("voucherDateTo", to)
		}
		return q
	}
	for _, archived := range []string{"false", "true"} {
		vs, total, err := lexAll[lexVoucher](ctx, l.c, "/v1/voucherlist", query(archived, "", ""))
		if err != nil {
			return refs, err
		}
		if total <= lexWindow {
			add(vs)
			continue
		}
		// More than one query may return: read year by year, and month by
		// month for a year that is still too large.
		for year := 2000; year <= time.Now().Year(); year++ {
			vs, total, err = lexAll[lexVoucher](ctx, l.c, "/v1/voucherlist", query(archived, fmt.Sprintf("%d-01-01", year), fmt.Sprintf("%d-12-31", year)))
			if err != nil {
				return refs, err
			}
			if total <= lexWindow {
				add(vs)
				continue
			}
			for month := time.January; month <= time.December; month++ {
				start := time.Date(year, month, 1, 0, 0, 0, 0, time.UTC)
				vs, _, err = lexAll[lexVoucher](ctx, l.c, "/v1/voucherlist", query(archived, start.Format("2006-01-02"), start.AddDate(0, 1, -1).Format("2006-01-02")))
				if err != nil {
					return refs, err
				}
				add(vs)
			}
		}
	}
	return refs, nil
}

// lexDocumentPaths are the documents written in Lexware's invoicing; their
// file comes from the document endpoint, the others from bookkeeping.
var lexDocumentPaths = map[string]string{
	"invoice":            "/v1/invoices/",
	"creditnote":         "/v1/credit-notes/",
	"downpaymentinvoice": "/v1/down-payment-invoices/",
}

func (l *lexware) Files(ctx context.Context, ref RemoteRef) ([]RemoteFile, error) {
	if base, ok := lexDocumentPaths[ref.Type]; ok {
		return l.documentFiles(ctx, base+url.PathEscape(ref.ID)+"/file", ref.Meta.Number)
	}
	var v struct {
		Files []string `json:"files"`
	}
	if err := l.c.GetJSON(ctx, "/v1/vouchers/"+url.PathEscape(ref.ID), &v); err != nil {
		return nil, err
	}
	var files []RemoteFile
	for i, id := range v.Files {
		data, h, err := l.c.Get(ctx, "/v1/files/"+url.PathEscape(id), "*/*")
		if err != nil {
			return nil, err
		}
		name := ref.Meta.Number
		if i > 0 {
			name = fmt.Sprintf("%s-%d", name, i+1)
		}
		files = append(files, RemoteFile{Name: fileName(h, name), Data: data})
	}
	return files, nil
}

// documentFiles loads the file of a written document: an XRechnung comes as
// XML, together with Lexware's view of it as PDF; everything else as PDF.
func (l *lexware) documentFiles(ctx context.Context, path, number string) ([]RemoteFile, error) {
	data, h, err := l.c.Get(ctx, path, "*/*")
	if errors.Is(err, ErrConflict) {
		return nil, errors.New("Lexware hat für diesen Beleg keine Datei erzeugt (Entwurf).")
	}
	if err != nil {
		return nil, err
	}
	files := []RemoteFile{{Name: fileName(h, number), Data: data}}
	if strings.Contains(h.Get("Content-Type"), "xml") {
		if pdf, ph, err := l.c.Get(ctx, path, "application/pdf"); err == nil {
			files = append(files, RemoteFile{Name: viewName(fileName(ph, number)), Data: pdf})
		}
	}
	return files, nil
}

// viewName marks a PDF that only shows an e-invoice.
func viewName(name string) string {
	ext := ""
	if i := strings.LastIndex(name, "."); i > 0 {
		name, ext = name[:i], name[i:]
	}
	return name + " (Ansicht)" + ext
}
