package umzug

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	erechnung "github.com/papierleicht/go-e-rechnung-validator"
)

// sevdesk over its public API, https://api.sevdesk.de/openapi.yaml. The key
// is the API token from the user settings. The app reads written invoices and
// credit notes; receipts (Belege) come from sevdesk's own export, because the
// API documents no download for their files.

func init() {
	RegisterProvider("sevdesk", func(key string) Provider { return newSevdesk("https://my.sevdesk.de/api/v1", key) })
}

type sevdesk struct{ c *Client }

func newSevdesk(base, key string) *sevdesk {
	return &sevdesk{c: &Client{
		Base:        base,
		Auth:        func(r *http.Request) { r.Header.Set("Authorization", key) },
		Interval:    300 * time.Millisecond,
		MaxInterval: 3 * time.Second,
	}}
}

func (s *sevdesk) Check(ctx context.Context) (string, error) {
	var v struct {
		Objects struct {
			Version string `json:"version"`
		} `json:"objects"`
	}
	if err := s.c.GetJSON(ctx, "/Tools/bookkeepingSystemVersion", &v); err != nil {
		return "", err
	}
	return "", nil
}

const sevPage = 1000

// sevAll reads all pages of a list.
func sevAll[T any](ctx context.Context, c *Client, path string, q url.Values) ([]T, error) {
	var out []T
	for offset := 0; ; offset += sevPage {
		q.Set("limit", strconv.Itoa(sevPage))
		q.Set("offset", strconv.Itoa(offset))
		var p struct {
			Objects []T `json:"objects"`
		}
		if err := c.GetJSON(ctx, path+"?"+q.Encode(), &p); err != nil {
			return out, err
		}
		out = append(out, p.Objects...)
		if len(p.Objects) < sevPage {
			return out, nil
		}
	}
}

type sevRef struct {
	ID flexString `json:"id"`
}

type sevContact struct {
	ID             flexString `json:"id"`
	Name           string     `json:"name"`
	Surename       string     `json:"surename"`
	Familyname     string     `json:"familyname"`
	CustomerNumber flexString `json:"customerNumber"`
	VatNumber      string     `json:"vatNumber"`
	Category       *sevRef    `json:"category"`
	Parent         *sevRef    `json:"parent"`
}

type sevAddress struct {
	Contact sevRef `json:"contact"`
	Street  string `json:"street"`
	Zip     string `json:"zip"`
	City    string `json:"city"`
	Name2   string `json:"name2"`
}

type sevWay struct {
	Contact sevRef     `json:"contact"`
	Value   string     `json:"value"`
	Main    flexString `json:"main"`
}

// sevSupplier is the category id sevdesk gives suppliers.
const sevSupplier = "2"

func (s *sevdesk) Customers(ctx context.Context) (Table, error) {
	t := Table{Columns: customerColumns, Encoding: "API", Delimiter: "API"}
	contacts, err := sevAll[sevContact](ctx, s.c, "/Contact", url.Values{"depth": {"1"}})
	if err != nil {
		return t, err
	}
	addresses, err := sevAll[sevAddress](ctx, s.c, "/ContactAddress", url.Values{})
	if err != nil {
		return t, err
	}
	ways, err := sevAll[sevWay](ctx, s.c, "/CommunicationWay", url.Values{"type": {"EMAIL"}})
	if err != nil {
		return t, err
	}
	address := map[string]sevAddress{}
	for _, a := range addresses {
		if _, ok := address[string(a.Contact.ID)]; !ok {
			address[string(a.Contact.ID)] = a
		}
	}
	email := map[string]string{}
	for _, w := range ways {
		id := string(w.Contact.ID)
		if _, ok := email[id]; !ok || w.Main == "1" {
			email[id] = strings.TrimSpace(w.Value)
		}
	}
	for _, c := range contacts {
		// Persons under an organisation are its contact persons, not customers.
		if c.Parent != nil && c.Parent.ID != "" {
			continue
		}
		if c.Category != nil && string(c.Category.ID) == sevSupplier {
			continue
		}
		a := address[string(c.ID)]
		first, last := c.Surename, c.Familyname
		t.Rows = append(t.Rows, customerRow(c.Name, first, last, a.Street, a.Name2, a.Zip, a.City, "", email[string(c.ID)], c.VatNumber, string(c.CustomerNumber)))
	}
	return t, nil
}

type sevInvoice struct {
	ID                 flexString `json:"id"`
	InvoiceNumber      string     `json:"invoiceNumber"`
	InvoiceDate        flexString `json:"invoiceDate"`
	InvoiceType        string     `json:"invoiceType"`
	CreditNoteNumber   string     `json:"creditNoteNumber"`
	CreditNoteDate     flexString `json:"creditNoteDate"`
	Status             flexString `json:"status"`
	Address            string     `json:"address"`
	SumGross           flexNumber `json:"sumGross"`
	PropertyIsEInvoice flexString `json:"propertyIsEInvoice"`
}

func (s *sevdesk) Documents(ctx context.Context) ([]RemoteRef, error) {
	invoices, err := sevAll[sevInvoice](ctx, s.c, "/Invoice", url.Values{})
	if err != nil {
		return nil, err
	}
	notes, err := sevAll[sevInvoice](ctx, s.c, "/CreditNote", url.Values{})
	if err != nil {
		return nil, err
	}
	var refs []RemoteRef
	add := func(v sevInvoice, typ, kind, number, date string) {
		// 100 is a draft, 50 a switched-off template of a recurring invoice.
		if v.Status == "100" || v.Status == "50" || v.ID == "" {
			return
		}
		ref := RemoteRef{ID: typ + "-" + string(v.ID), Kind: kind, Type: typ, EInvoice: v.PropertyIsEInvoice == "1" || v.PropertyIsEInvoice == "true",
			Meta: Booking{Number: number, Date: isoDate(date), Text: firstLine(v.Address), Direction: "outgoing", Source: "api"}}
		if v.SumGross.Set {
			ref.Meta.Gross = amount(v.SumGross.Value)
		}
		refs = append(refs, ref)
	}
	for _, v := range invoices {
		kind := "invoice"
		if v.InvoiceType == "MA" {
			kind = "document"
		}
		add(v, "Invoice", kind, v.InvoiceNumber, string(v.InvoiceDate))
	}
	for _, v := range notes {
		add(v, "CreditNote", "credit-note", v.CreditNoteNumber, string(v.CreditNoteDate))
	}
	return refs, nil
}

func firstLine(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.IndexAny(s, "\r\n"); i >= 0 {
		s = s[:i]
	}
	return strings.TrimSpace(s)
}

type sevFile struct {
	Filename       string     `json:"filename"`
	MimeType       string     `json:"mimeType"`
	Base64Encoded  flexString `json:"base64encoded"`
	Content        string     `json:"content"`
	ObjectsWrapped *struct {
		Filename      string     `json:"filename"`
		Base64Encoded flexString `json:"base64encoded"`
		Content       string     `json:"content"`
	} `json:"objects"`
}

func (s *sevdesk) Files(ctx context.Context, ref RemoteRef) ([]RemoteFile, error) {
	id := strings.TrimPrefix(ref.ID, ref.Type+"-")
	base := "/" + ref.Type + "/" + url.PathEscape(id)
	// preventSendBy keeps sevdesk from marking the document as sent.
	body, _, err := s.c.Get(ctx, base+"/getPdf?preventSendBy=true", "application/json")
	if err != nil {
		return nil, err
	}
	var f sevFile
	if err = json.Unmarshal(body, &f); err != nil {
		return nil, errors.New("sevdesk hat die PDF-Datei in unerwarteter Form geschickt.")
	}
	name, encoded, content := f.Filename, f.Base64Encoded, f.Content
	if w := f.ObjectsWrapped; content == "" && w != nil {
		name, encoded, content = w.Filename, w.Base64Encoded, w.Content
	}
	if content == "" {
		return nil, errors.New("sevdesk hat zu diesem Beleg keine Datei geschickt.")
	}
	data := []byte(content)
	if encoded != "false" && encoded != "0" {
		if data, err = base64.StdEncoding.DecodeString(content); err != nil {
			return nil, errors.New("sevdesk hat die PDF-Datei in unerwarteter Form geschickt.")
		}
	}
	if name == "" {
		name = ref.Meta.Number
	}
	if !strings.HasSuffix(strings.ToLower(name), ".pdf") {
		name += ".pdf"
	}
	files := []RemoteFile{{Name: fileName(http.Header{}, name), Data: data}}
	// An e-invoice without the XML in its PDF (XRechnung) gets its XML.
	if ref.Type == "Invoice" && ref.EInvoice {
		if _, err := erechnung.ExtractXML(data); err != nil {
			var x struct {
				Objects string `json:"objects"`
			}
			if s.c.GetJSON(ctx, base+"/getXml", &x) == nil && strings.TrimSpace(x.Objects) != "" {
				files = append([]RemoteFile{{Name: strings.TrimSuffix(name, ".pdf") + ".xml", Data: []byte(x.Objects)}}, files...)
			}
		}
	}
	return files, nil
}
