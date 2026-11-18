package umzug

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// The tests answer as the documented APIs do; the example data follow the
// official specifications, not a real account.

func fakeAPI(t *testing.T, auth string, routes map[string]func(w http.ResponseWriter, r *http.Request)) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("%s %s: only reading is allowed", r.Method, r.URL)
		}
		if r.Header.Get("Authorization") != auth {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		if h, ok := routes[r.URL.Path]; ok {
			h(w, r)
			return
		}
		t.Logf("unknown %s", r.URL)
		w.WriteHeader(http.StatusNotFound)
	}))
	t.Cleanup(srv.Close)
	return srv
}

func fast(c *Client) {
	c.Interval, c.MaxInterval, c.Backoff = 0, 0, time.Millisecond
}

func ciiInvoice(t *testing.T) []byte {
	data, err := os.ReadFile(filepath.Join("..", "einvoice", "testdata", "cii.xml"))
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func TestClientWaitsWhenThereAreTooManyRequests(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if calls.Add(1) < 3 {
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		w.Write([]byte(`{"ok":true}`))
	}))
	defer srv.Close()
	c := &Client{Base: srv.URL, Interval: time.Millisecond, MaxInterval: 4 * time.Millisecond, Backoff: time.Millisecond}
	var v struct{ OK bool }
	if err := c.GetJSON(context.Background(), "/x", &v); err != nil || !v.OK || calls.Load() != 3 {
		t.Fatalf("v=%+v err=%v calls=%d", v, err, calls.Load())
	}
	if c.Interval != 4*time.Millisecond {
		t.Fatalf("interval %v after two 429", c.Interval)
	}
	if _, _, err := (&Client{Base: srv.URL + "/nope", Auth: func(r *http.Request) {}}).Get(context.Background(), "", ""); err != nil {
		t.Fatalf("plain get: %v", err)
	}
}

func TestClientNamesAWrongKey(t *testing.T) {
	srv := fakeAPI(t, "Bearer richtig", nil)
	c := &Client{Base: srv.URL, Auth: func(r *http.Request) { r.Header.Set("Authorization", "Bearer falsch") }}
	if _, _, err := c.Get(context.Background(), "/", ""); !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("err=%v", err)
	}
}

func TestLexwareOffice(t *testing.T) {
	xml := ciiInvoice(t)
	var listCalls atomic.Int32
	srv := fakeAPI(t, "Bearer schluessel", map[string]func(http.ResponseWriter, *http.Request){
		"/v1/profile": func(w http.ResponseWriter, r *http.Request) {
			fmt.Fprint(w, `{"organizationId":"aa","companyName":"Muster GmbH","taxType":"net","smallBusiness":false}`)
		},
		"/v1/contacts": func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Query().Get("customer") != "true" || r.URL.Query().Get("size") != "250" {
				t.Errorf("contacts query %s", r.URL.RawQuery)
			}
			if r.URL.Query().Get("page") == "0" {
				fmt.Fprint(w, `{"content":[{"id":"c1","version":1,"roles":{"customer":{"number":10001}},
					"company":{"name":"Kunde AG","vatRegistrationId":"DE123456789","contactPersons":[{"firstName":"Eva","lastName":"Muster","primary":true,"emailAddress":"eva@kunde.example"}]},
					"addresses":{"billing":[{"supplement":"Haus B","street":"Hauptstr. 1","zip":"10115","city":"Berlin","countryCode":"DE"}]},
					"emailAddresses":{}}],"first":true,"last":false,"totalPages":2,"totalElements":2}`)
				return
			}
			fmt.Fprint(w, `{"content":[{"id":"c2","roles":{"customer":{"number":10002}},"person":{"salutation":"Frau","firstName":"Ina","lastName":"Beispiel"},
				"addresses":{"billing":[{"street":"Weg 2","zip":"80331","city":"München","countryCode":"DE"}]},"emailAddresses":{"private":["ina@example.org"]}}],
				"first":false,"last":true,"totalPages":2,"totalElements":2}`)
		},
		"/v1/voucherlist": func(w http.ResponseWriter, r *http.Request) {
			listCalls.Add(1)
			q := r.URL.Query()
			if q.Get("voucherStatus") != "any" || !strings.Contains(q.Get("voucherType"), "purchaseinvoice") || strings.Contains(q.Get("voucherType"), "quotation") {
				t.Errorf("voucherlist query %s", r.URL.RawQuery)
			}
			if q.Get("archived") == "true" {
				fmt.Fprint(w, `{"content":[],"last":true,"totalPages":0,"totalElements":0}`)
				return
			}
			fmt.Fprint(w, `{"content":[
				{"id":"v1","voucherType":"invoice","voucherStatus":"paid","voucherNumber":"RE1001","voucherDate":"2025-03-01T00:00:00.000+01:00","contactName":"Kunde AG","totalAmount":119.0,"currency":"EUR"},
				{"id":"v2","voucherType":"invoice","voucherStatus":"draft","voucherNumber":"","voucherDate":"2025-03-02T00:00:00.000+01:00","totalAmount":10},
				{"id":"v3","voucherType":"purchaseinvoice","voucherStatus":"open","voucherNumber":"L-77","voucherDate":"2025-04-02T00:00:00.000+02:00","contactName":"Lieferant","totalAmount":-50.5},
				{"id":"v4","voucherType":"quotation","voucherStatus":"open","voucherNumber":"AN1"},
				{"id":"v5","voucherType":"creditnote","voucherStatus":"open","voucherNumber":"GS1","voucherDate":"2025-05-01T00:00:00.000+02:00","totalAmount":20}
			],"last":true,"totalPages":1,"totalElements":5}`)
		},
		"/v1/invoices/v1/file": func(w http.ResponseWriter, r *http.Request) {
			switch r.Header.Get("Accept") {
			case "*/*":
				w.Header().Set("Content-Type", "application/xml")
				w.Header().Set("Content-Disposition", `attachment; filename="RE1001.xml"`)
				w.Write(xml)
			case "application/pdf":
				w.Header().Set("Content-Type", "application/pdf")
				w.Header().Set("Content-Disposition", `attachment; filename="RE1001.pdf"`)
				w.Write([]byte("%PDF-1.4 Ansicht"))
			default:
				w.WriteHeader(http.StatusNotAcceptable)
			}
		},
		"/v1/credit-notes/v5/file": func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusConflict)
		},
		"/v1/vouchers/v3": func(w http.ResponseWriter, r *http.Request) {
			fmt.Fprint(w, `{"id":"v3","files":["f1"]}`)
		},
		"/v1/files/f1": func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "image/jpeg")
			w.Write([]byte("jpeg"))
		},
	})
	l := newLexware(srv.URL, "schluessel")
	fast(l.c)
	ctx := context.Background()
	if account, err := l.Check(ctx); err != nil || account != "Muster GmbH" {
		t.Fatalf("check %q %v", account, err)
	}
	customers, err := l.Customers(ctx)
	if err != nil || len(customers.Rows) != 2 {
		t.Fatalf("customers %+v err=%v", customers, err)
	}
	if got := strings.Join(customers.Rows[0], "|"); got != "Kunde AG|||Hauptstr. 1|Haus B|10115|Berlin|DE|eva@kunde.example|DE123456789|10001" {
		t.Fatalf("row %s", got)
	}
	if got := strings.Join(customers.Rows[1], "|"); got != "|Ina|Beispiel|Weg 2||80331|München|DE|ina@example.org||10002" {
		t.Fatalf("row %s", got)
	}
	mapping := GuessMapping(ListCustomers, customers.Columns)
	if mapping["name"] != 0 || mapping["email"] != 8 || mapping["vatId"] != 9 || mapping["firstName"] != 1 {
		t.Fatalf("mapping %+v", mapping)
	}

	refs, err := l.Documents(ctx)
	if err != nil || len(refs) != 3 || listCalls.Load() != 2 {
		t.Fatalf("refs %+v err=%v calls=%d", refs, err, listCalls.Load())
	}
	if refs[0].Meta != (Booking{Number: "RE1001", Date: "2025-03-01", Gross: "119.00", Text: "Kunde AG", Direction: "outgoing", Source: "api"}) || refs[0].Kind != "invoice" {
		t.Fatalf("ref %+v", refs[0])
	}
	if refs[1].Meta.Direction != "incoming" || refs[1].Meta.Gross != "50.50" || refs[2].Kind != "credit-note" {
		t.Fatalf("refs %+v", refs)
	}
	files, err := l.Files(ctx, refs[0])
	if err != nil || len(files) != 2 || files[0].Name != "RE1001.xml" || files[1].Name != "RE1001 (Ansicht).pdf" {
		t.Fatalf("files %+v err=%v", files, err)
	}
	d, err := Read(refs[0].Group(files), "lexware-office", Own{})
	if err != nil || !d.EInvoice || len(d.Files) != 2 {
		t.Fatalf("read %+v err=%v", d, err)
	}
	if files, err = l.Files(ctx, refs[1]); err != nil || len(files) != 1 || files[0].Name != "L-77.jpg" {
		t.Fatalf("voucher files %+v err=%v", files, err)
	}
	if d, err = Read(refs[1].Group(files), "lexware-office", Own{}); err != nil || d.Basis != "api" || d.Number != "L-77" || d.Direction != "incoming" || d.Gross != "50.50" {
		t.Fatalf("receipt %+v err=%v", d, err)
	}
	if _, err = l.Files(ctx, refs[2]); err == nil || !strings.Contains(err.Error(), "Entwurf") {
		t.Fatalf("draft file err=%v", err)
	}
}

func TestLexwareReadsLargeListsYearByYear(t *testing.T) {
	var years []string
	srv := fakeAPI(t, "Bearer k", map[string]func(http.ResponseWriter, *http.Request){
		"/v1/voucherlist": func(w http.ResponseWriter, r *http.Request) {
			q := r.URL.Query()
			from := q.Get("voucherDateFrom")
			if q.Get("archived") == "true" {
				fmt.Fprint(w, `{"content":[],"last":true,"totalPages":0,"totalElements":0}`)
				return
			}
			if from == "" {
				fmt.Fprint(w, `{"content":[{"id":"x","voucherType":"invoice","voucherStatus":"open"}],"last":false,"totalPages":400,"totalElements":12000}`)
				return
			}
			years = append(years, from)
			if from == "2024-01-01" {
				fmt.Fprint(w, `{"content":[{"id":"y","voucherType":"invoice","voucherStatus":"open","voucherNumber":"RE-2024"}],"last":true,"totalPages":1,"totalElements":1}`)
				return
			}
			fmt.Fprint(w, `{"content":[],"last":true,"totalPages":0,"totalElements":0}`)
		},
	})
	l := newLexware(srv.URL, "k")
	fast(l.c)
	refs, err := l.Documents(context.Background())
	if err != nil || len(refs) != 1 || refs[0].ID != "y" || years[0] != "2000-01-01" {
		t.Fatalf("refs %+v err=%v years=%v", refs, err, years)
	}
}

func TestSevdesk(t *testing.T) {
	pdf := []byte("%PDF-1.4 sevdesk")
	srv := fakeAPI(t, "0123456789abcdef0123456789abcdef", map[string]func(http.ResponseWriter, *http.Request){
		"/Tools/bookkeepingSystemVersion": func(w http.ResponseWriter, r *http.Request) {
			fmt.Fprint(w, `{"objects":{"version":"2.0"}}`)
		},
		"/Contact": func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Query().Get("depth") != "1" || r.URL.Query().Get("limit") != "1000" {
				t.Errorf("contact query %s", r.URL.RawQuery)
			}
			fmt.Fprint(w, `{"total":"4","objects":[
				{"id":"1","objectName":"Contact","name":"Kunde AG","customerNumber":"1001","vatNumber":"DE111","category":{"id":"3","objectName":"Category"}},
				{"id":"2","objectName":"Contact","surename":"Eva","familyname":"Muster","parent":{"id":"1","objectName":"Contact"},"category":{"id":"3"}},
				{"id":"3","objectName":"Contact","name":"Lieferant GmbH","category":{"id":"2"}},
				{"id":"4","objectName":"Contact","surename":"Ina","familyname":"Beispiel","customerNumber":1002,"category":{"id":3}}]}`)
		},
		"/ContactAddress": func(w http.ResponseWriter, r *http.Request) {
			fmt.Fprint(w, `{"objects":[{"id":7,"contact":{"id":"1","objectName":"Contact"},"street":"Hauptstr. 1","zip":"10115","city":"Berlin","name2":"Haus B"}]}`)
		},
		"/CommunicationWay": func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Query().Get("type") != "EMAIL" {
				t.Errorf("way query %s", r.URL.RawQuery)
			}
			fmt.Fprint(w, `{"objects":[{"id":"9","contact":{"id":"1"},"type":"EMAIL","value":"info@kunde.example","main":"0"},
				{"id":"10","contact":{"id":"1"},"type":"EMAIL","value":"rechnung@kunde.example","main":"1"}]}`)
		},
		"/Invoice": func(w http.ResponseWriter, r *http.Request) {
			fmt.Fprint(w, `{"objects":[
				{"id":"11","invoiceNumber":"RE-1001","invoiceDate":"2025-02-03T00:00:00+01:00","invoiceType":"RE","status":"1000","address":"Kunde AG\nHauptstr. 1","sumGross":"119.00","currency":"EUR"},
				{"id":"12","invoiceNumber":"","invoiceType":"RE","status":"100","sumGross":"5"},
				{"id":"13","invoiceNumber":"RE-1002","invoiceDate":"1738540800","invoiceType":"RE","status":"200","sumGross":59.5,"propertyIsEInvoice":true}]}`)
		},
		"/CreditNote": func(w http.ResponseWriter, r *http.Request) {
			fmt.Fprint(w, `{"objects":[{"id":"11","creditNoteNumber":"GU-1001","creditNoteDate":"2025-03-01","status":"200","sumGross":"-10.00"}]}`)
		},
		"/Invoice/11/getPdf": func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Query().Get("preventSendBy") != "true" {
				t.Errorf("getPdf without preventSendBy: %s", r.URL.RawQuery)
			}
			fmt.Fprintf(w, `{"filename":"RE-1001.pdf","mimeType":"application/pdf","base64encoded":true,"content":%q}`, base64.StdEncoding.EncodeToString(pdf))
		},
		"/Invoice/13/getPdf": func(w http.ResponseWriter, r *http.Request) {
			fmt.Fprintf(w, `{"objects":{"filename":"RE-1002.pdf","base64encoded":true,"content":%q}}`, base64.StdEncoding.EncodeToString(pdf))
		},
		"/Invoice/13/getXml": func(w http.ResponseWriter, r *http.Request) {
			fmt.Fprintf(w, `{"objects":%q}`, string(ciiInvoice(t)))
		},
		"/CreditNote/11/getPdf": func(w http.ResponseWriter, r *http.Request) {
			fmt.Fprintf(w, `{"filename":"GU-1001.pdf","base64encoded":true,"content":%q}`, base64.StdEncoding.EncodeToString(pdf))
		},
	})
	s := newSevdesk(srv.URL, "0123456789abcdef0123456789abcdef")
	fast(s.c)
	ctx := context.Background()
	if _, err := s.Check(ctx); err != nil {
		t.Fatal(err)
	}
	customers, err := s.Customers(ctx)
	if err != nil || len(customers.Rows) != 2 {
		t.Fatalf("customers %+v err=%v", customers, err)
	}
	if got := strings.Join(customers.Rows[0], "|"); got != "Kunde AG|||Hauptstr. 1|Haus B|10115|Berlin||rechnung@kunde.example|DE111|1001" {
		t.Fatalf("row %s", got)
	}
	if customers.Rows[1][1] != "Ina" || customers.Rows[1][10] != "1002" {
		t.Fatalf("row %v", customers.Rows[1])
	}
	refs, err := s.Documents(ctx)
	if err != nil || len(refs) != 3 {
		t.Fatalf("refs %+v err=%v", refs, err)
	}
	if refs[0].Meta != (Booking{Number: "RE-1001", Date: "2025-02-03", Gross: "119.00", Text: "Kunde AG", Direction: "outgoing", Source: "api"}) {
		t.Fatalf("ref %+v", refs[0])
	}
	if refs[1].Meta.Date != "2025-02-03" || !refs[1].EInvoice || refs[2].Kind != "credit-note" || refs[2].ID == refs[0].ID || refs[2].Meta.Gross != "10.00" {
		t.Fatalf("refs %+v", refs)
	}
	files, err := s.Files(ctx, refs[0])
	if err != nil || len(files) != 1 || files[0].Name != "RE-1001.pdf" || string(files[0].Data) != string(pdf) {
		t.Fatalf("files %+v err=%v", files, err)
	}
	if files, err = s.Files(ctx, refs[1]); err != nil || len(files) != 2 || files[0].Name != "RE-1002.xml" {
		t.Fatalf("e-invoice files %+v err=%v", files, err)
	}
	if d, err := Read(refs[1].Group(files), "sevdesk", Own{}); err != nil || !d.EInvoice {
		t.Fatalf("read %+v err=%v", d, err)
	}
	if files, err = s.Files(ctx, refs[2]); err != nil || files[0].Name != "GU-1001.pdf" {
		t.Fatalf("credit note files %+v err=%v", files, err)
	}
}

func TestEasybill(t *testing.T) {
	xml := ciiInvoice(t)
	srv := fakeAPI(t, "Bearer eb-key-123", map[string]func(http.ResponseWriter, *http.Request){
		"/customers": func(w http.ResponseWriter, r *http.Request) {
			q := r.URL.Query()
			if q.Get("limit") == "1" {
				fmt.Fprint(w, `{"page":1,"pages":1,"limit":1,"total":1,"items":[]}`)
				return
			}
			if q.Get("contactType") != "CUSTOMER" {
				t.Errorf("customers query %s", r.URL.RawQuery)
			}
			if q.Get("page") == "1" {
				fmt.Fprint(w, `{"page":1,"pages":2,"limit":1000,"total":2,"items":[{"id":1,"number":"K-1","company_name":"Kunde AG","street":"Hauptstr. 1","suffix_1":"Haus B","zip_code":"10115","city":"Berlin","country":"DE","emails":["a@kunde.example","b@kunde.example"],"vat_identifier":"DE111"}]}`)
				return
			}
			fmt.Fprint(w, `{"page":2,"pages":2,"limit":1000,"total":2,"items":[{"id":2,"number":"K-2","first_name":"Ina","last_name":"Beispiel","country":"AT","emails":[]}]}`)
		},
		"/documents": func(w http.ResponseWriter, r *http.Request) {
			q := r.URL.Query()
			if q.Get("is_draft") != "0" || q.Get("type") != "CREDIT,INVOICE,STORNO,STORNO_CREDIT" {
				t.Errorf("documents query %s", r.URL.RawQuery)
			}
			fmt.Fprint(w, `{"page":1,"pages":1,"limit":1000,"total":3,"items":[
				{"id":101,"number":"RE-1","type":"INVOICE","document_date":"2025-01-10","amount":11900,"amount_net":10000,"is_draft":false,"address":{"company_name":"Kunde AG"},"file_format_config":[{"type":"xrechnung3_0_xml"}]},
				{"id":102,"number":"GS-1","type":"CREDIT","document_date":"2025-02-10","amount":-2500,"is_draft":false,"address":{"first_name":"Ina","last_name":"Beispiel"},"file_format_config":[{"type":"default"}]},
				{"id":103,"number":"RE-2","type":"INVOICE","document_date":"2025-03-10","amount":5950,"is_draft":false,"address":{},"file_format_config":[{"type":"zugferd2_4_extended"}]}]}`)
		},
		"/documents/101/download": func(w http.ResponseWriter, r *http.Request) {
			if !strings.HasPrefix(r.Header.Get("Accept"), "application/xml;format=xrechnung") {
				w.WriteHeader(http.StatusNotAcceptable)
				return
			}
			w.Header().Set("Content-Type", "application/xml;format=xrechnung;version=3.0.1")
			w.Write(xml)
		},
		"/documents/101/pdf": func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/pdf")
			w.Write([]byte("%PDF view"))
		},
		"/documents/102/pdf": func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/pdf")
			w.Write([]byte("%PDF credit"))
		},
		"/documents/103/download": func(w http.ResponseWriter, r *http.Request) {
			if r.Header.Get("Accept") != "application/pdf;format=zugferd_extended" {
				t.Errorf("accept %q", r.Header.Get("Accept"))
			}
			w.WriteHeader(http.StatusNotAcceptable)
		},
		"/documents/103/pdf": func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/pdf")
			w.Write([]byte("%PDF plain"))
		},
	})
	e := newEasybill(srv.URL, "eb-key-123")
	fast(e.c)
	ctx := context.Background()
	if _, err := e.Check(ctx); err != nil {
		t.Fatal(err)
	}
	customers, err := e.Customers(ctx)
	if err != nil || len(customers.Rows) != 2 {
		t.Fatalf("customers %+v err=%v", customers, err)
	}
	if got := strings.Join(customers.Rows[0], "|"); got != "Kunde AG|||Hauptstr. 1|Haus B|10115|Berlin|DE|a@kunde.example|DE111|K-1" {
		t.Fatalf("row %s", got)
	}
	refs, err := e.Documents(ctx)
	if err != nil || len(refs) != 3 {
		t.Fatalf("refs %+v err=%v", refs, err)
	}
	if refs[0].Meta != (Booking{Number: "RE-1", Date: "2025-01-10", Gross: "119.00", Text: "Kunde AG", Direction: "outgoing", Source: "api"}) || !refs[0].EInvoice {
		t.Fatalf("ref %+v", refs[0])
	}
	if refs[1].Kind != "credit-note" || refs[1].Meta.Gross != "25.00" || refs[1].Meta.Text != "Ina Beispiel" || refs[1].EInvoice {
		t.Fatalf("ref %+v", refs[1])
	}
	files, err := e.Files(ctx, refs[0])
	if err != nil || len(files) != 2 || files[0].Name != "RE-1.xml" || files[1].Name != "RE-1 (Ansicht).pdf" {
		t.Fatalf("files %+v err=%v", files, err)
	}
	if files, err = e.Files(ctx, refs[1]); err != nil || len(files) != 1 || files[0].Name != "GS-1.pdf" {
		t.Fatalf("files %+v err=%v", files, err)
	}
	if files, err = e.Files(ctx, refs[2]); err != nil || len(files) != 1 || string(files[0].Data) != "%PDF plain" {
		t.Fatalf("fallback files %+v err=%v", files, err)
	}
}

func TestRegisteredProviders(t *testing.T) {
	for _, id := range []string{"lexware-office", "sevdesk", "easybill"} {
		if !HasAPI(id) {
			t.Errorf("%s not registered", id)
		}
	}
	if HasAPI("invoiz") {
		t.Error("invoiz has no API")
	}
}
