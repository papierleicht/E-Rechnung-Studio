package umzug

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// easybill over its public API, https://api.easybill.de/rest/v1/swagger.json.
// The key is the API key from the settings. easybill allows 10 requests a
// minute in PLUS and 60 in BUSINESS; the client slows down when it is told to.

func init() {
	RegisterProvider("easybill", func(key string) Provider { return newEasybill("https://api.easybill.de/rest/v1", key) })
}

type easybill struct{ c *Client }

func newEasybill(base, key string) *easybill {
	return &easybill{c: &Client{
		Base:        base,
		Auth:        func(r *http.Request) { r.Header.Set("Authorization", "Bearer "+key) },
		Interval:    time.Second,
		MaxInterval: 6500 * time.Millisecond,
	}}
}

func (e *easybill) Check(ctx context.Context) (string, error) {
	var p struct {
		Total int `json:"total"`
	}
	if err := e.c.GetJSON(ctx, "/customers?limit=1", &p); err != nil {
		return "", err
	}
	return "", nil
}

func ebAll[T any](ctx context.Context, c *Client, path string, q url.Values) ([]T, error) {
	var out []T
	for page := 1; ; page++ {
		q.Set("page", strconv.Itoa(page))
		q.Set("limit", "1000")
		var p struct {
			Pages int `json:"pages"`
			Items []T `json:"items"`
		}
		if err := c.GetJSON(ctx, path+"?"+q.Encode(), &p); err != nil {
			return out, err
		}
		out = append(out, p.Items...)
		if page >= p.Pages || len(p.Items) == 0 {
			return out, nil
		}
	}
}

type ebCustomer struct {
	Number        string   `json:"number"`
	CompanyName   string   `json:"company_name"`
	FirstName     string   `json:"first_name"`
	LastName      string   `json:"last_name"`
	Street        string   `json:"street"`
	Suffix1       string   `json:"suffix_1"`
	ZipCode       string   `json:"zip_code"`
	City          string   `json:"city"`
	Country       string   `json:"country"`
	Emails        []string `json:"emails"`
	VatIdentifier string   `json:"vat_identifier"`
}

func (e *easybill) Customers(ctx context.Context) (Table, error) {
	t := Table{Columns: customerColumns, Encoding: "API", Delimiter: "API"}
	customers, err := ebAll[ebCustomer](ctx, e.c, "/customers", url.Values{"contactType": {"CUSTOMER"}})
	if err != nil {
		return t, err
	}
	for _, c := range customers {
		t.Rows = append(t.Rows, customerRow(c.CompanyName, c.FirstName, c.LastName, c.Street, c.Suffix1, c.ZipCode, c.City, c.Country, first1(c.Emails), c.VatIdentifier, c.Number))
	}
	return t, nil
}

type ebDocument struct {
	ID           flexString `json:"id"`
	Number       string     `json:"number"`
	Type         string     `json:"type"`
	DocumentDate string     `json:"document_date"`
	Amount       flexNumber `json:"amount"`
	IsDraft      bool       `json:"is_draft"`
	Address      struct {
		CompanyName string `json:"company_name"`
		FirstName   string `json:"first_name"`
		LastName    string `json:"last_name"`
	} `json:"address"`
	FileFormatConfig []struct {
		Type string `json:"type"`
	} `json:"file_format_config"`
}

// ebTypes are the document types that are invoices; offers, delivery notes,
// letters and the templates of recurring invoices stay in easybill.
var ebTypes = map[string]string{
	"INVOICE":       "invoice",
	"STORNO":        "invoice",
	"CREDIT":        "credit-note",
	"STORNO_CREDIT": "credit-note",
}

func (e *easybill) Documents(ctx context.Context) ([]RemoteRef, error) {
	docs, err := ebAll[ebDocument](ctx, e.c, "/documents", url.Values{"type": {"CREDIT,INVOICE,STORNO,STORNO_CREDIT"}, "is_draft": {"0"}})
	if err != nil {
		return nil, err
	}
	var refs []RemoteRef
	for _, d := range docs {
		kind, ok := ebTypes[d.Type]
		if !ok || d.IsDraft || d.ID == "" {
			continue
		}
		partner := d.Address.CompanyName
		if partner == "" {
			partner = strings.TrimSpace(d.Address.FirstName + " " + d.Address.LastName)
		}
		format := ""
		for _, f := range d.FileFormatConfig {
			if strings.HasPrefix(f.Type, "xrechnung") || strings.HasPrefix(f.Type, "zugferd") {
				format = f.Type
			}
		}
		ref := RemoteRef{ID: string(d.ID), Kind: kind, Type: format, EInvoice: format != "",
			Meta: Booking{Number: d.Number, Date: isoDate(d.DocumentDate), Text: partner, Direction: "outgoing", Source: "api"}}
		if d.Amount.Set {
			// easybill sends amounts in cents.
			ref.Meta.Gross = amount(d.Amount.Value / 100)
		}
		refs = append(refs, ref)
	}
	return refs, nil
}

func (e *easybill) Files(ctx context.Context, ref RemoteRef) ([]RemoteFile, error) {
	base := "/documents/" + url.PathEscape(ref.ID)
	var files []RemoteFile
	switch {
	case strings.HasPrefix(ref.Type, "xrechnung"):
		data, h, err := e.c.Get(ctx, base+"/download", "application/xml;format=xrechnung")
		if err == nil {
			files = append(files, RemoteFile{Name: fileName(h, ref.Meta.Number+".xml"), Data: data})
		} else if !noFormat(err) {
			return nil, err
		}
	case strings.HasPrefix(ref.Type, "zugferd"):
		format := "zugferd"
		if strings.HasSuffix(ref.Type, "_extended") {
			format = "zugferd_extended"
		}
		data, h, err := e.c.Get(ctx, base+"/download", "application/pdf;format="+format)
		if err == nil {
			return []RemoteFile{{Name: fileName(h, ref.Meta.Number+".pdf"), Data: data}}, nil
		}
		if !noFormat(err) {
			return nil, err
		}
	}
	data, h, err := e.c.Get(ctx, base+"/pdf", "application/pdf")
	if err != nil {
		if len(files) > 0 {
			return files, nil
		}
		return nil, fmt.Errorf("easybill: %w", err)
	}
	name := fileName(h, ref.Meta.Number+".pdf")
	if len(files) > 0 {
		name = viewName(name)
	}
	return append(files, RemoteFile{Name: name, Data: data}), nil
}

// noFormat tells that easybill has no file in the asked format; the PDF is
// loaded instead.
func noFormat(err error) bool { return errors.Is(err, ErrConflict) || errors.Is(err, ErrNotFound) }
