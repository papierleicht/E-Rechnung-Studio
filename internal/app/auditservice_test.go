package app

import (
	"archive/zip"
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"papierleicht.de/e-rechnung-studio/internal/account"
	"papierleicht.de/e-rechnung-studio/internal/entitlement"
	"papierleicht.de/e-rechnung-studio/internal/gdpdu"
	"papierleicht.de/e-rechnung-studio/internal/mail"
	"papierleicht.de/e-rechnung-studio/internal/store"
)

type fakeAuditRemote struct {
	plan    bool
	key     ed25519.PrivateKey
	calls   int
	failing bool
	list    []account.StoredReceipt
}

const auditTestDevice = "dieses-geraet-aaaaaaaa"

func (f *fakeAuditRemote) DeviceID() string { return auditTestDevice }

func (f *fakeAuditRemote) AuditReceipts(context.Context) ([]account.StoredReceipt, error) {
	if f.failing {
		return nil, errors.New("offline")
	}
	return f.list, nil
}

func (f *fakeAuditRemote) sign(t *testing.T, device string, seq int64, hash string) account.StoredReceipt {
	t.Helper()
	token, err := entitlement.SignAnchor(f.key, entitlement.Anchor{Subject: "u", Seq: seq, Hash: hash, At: time.Now().Unix()})
	if err != nil {
		t.Fatal(err)
	}
	return account.StoredReceipt{DeviceID: device, Seq: seq, Hash: hash, At: time.Now().UTC(), Receipt: token}
}

func (f *fakeAuditRemote) HasFeature(feature string) bool {
	return f.plan && feature == entitlement.FeatureAudit
}

func (f *fakeAuditRemote) AnchorAudit(_ context.Context, seq int64, hash string) (account.AuditReceipt, error) {
	f.calls++
	if f.failing {
		return account.AuditReceipt{}, errors.New("offline")
	}
	at := time.Now().UTC()
	token, err := entitlement.SignAnchor(f.key, entitlement.Anchor{Subject: "u", Seq: seq, Hash: hash, At: at.Unix()})
	return account.AuditReceipt{Receipt: token, At: at}, err
}

func (f *fakeAuditRemote) CheckAuditReceipt(receipt string) (entitlement.Anchor, error) {
	return entitlement.VerifyAnchor(f.key.Public().(ed25519.PublicKey), receipt)
}

func auditTestService(t *testing.T, plan bool) (*AuditService, *InvoiceService, *fakeAuditRemote) {
	t.Helper()
	s, err := NewInvoiceService(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	if _, err = s.SaveCompany(testCompany()); err != nil {
		t.Fatal(err)
	}
	_, key, _ := ed25519.GenerateKey(nil)
	remote := &fakeAuditRemote{plan: plan, key: key}
	return NewAuditService(s, remote), s, remote
}

func seedAuditData(t *testing.T, s *InvoiceService) {
	t.Helper()
	d := testDraft()
	d.Number, d.IssueDate, d.DeliveryDate, d.DueDate = "RE-2026-0001", "2026-03-10", "2026-03-10", "2026-04-10"
	if _, err := s.IssueInvoice(d); err != nil {
		t.Fatal(err)
	}
	third := testDraft()
	third.Number, third.IssueDate, third.DeliveryDate, third.DueDate = "RE-2026-0003", "2026-03-12", "2026-03-12", "2026-04-12"
	if _, err := s.IssueInvoice(third); err != nil {
		t.Fatal(err)
	}
	outside := testDraft()
	outside.Number, outside.IssueDate, outside.DeliveryDate, outside.DueDate = "RE-2026-0004", "2026-08-01", "2026-08-01", "2026-09-01"
	if _, err := s.IssueInvoice(outside); err != nil {
		t.Fatal(err)
	}
	receipt := store.Expense{Date: "2026-03-01", PaidDate: "2026-03-01", Supplier: "Papierhandel \"Nord\"", Category: "office", Gross: "119.00", TaxRate: "19", PaidWith: "bank",
		Attachment: []byte("%PDF-1.7 erstes Bild"), AttachmentName: "beleg.pdf", AttachmentType: "application/pdf"}
	saved, err := s.store.SaveExpense(receipt, func(string) bool { return true }, "asset")
	if err != nil {
		t.Fatal(err)
	}
	saved.Attachment, saved.AttachmentName = []byte("%PDF-1.7 zweites Bild"), "besser.pdf"
	if _, err = s.store.SaveExpense(saved, func(string) bool { return true }, "asset"); err != nil {
		t.Fatal(err)
	}
	noFile := store.Expense{Date: "2026-03-02", PaidDate: "2026-03-02", Supplier: "Tankstelle", Category: "office", Gross: "50.00", TaxRate: "19", PaidWith: "cash"}
	if _, err = s.store.SaveExpense(noFile, func(string) bool { return true }, "asset"); err != nil {
		t.Fatal(err)
	}
}

func readZip(t *testing.T, path string) map[string][]byte {
	t.Helper()
	r, err := zip.OpenReader(path)
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	out := map[string][]byte{}
	for _, f := range r.File {
		if f.Flags&0x1 != 0 {
			t.Fatalf("%s is encrypted, the upload platforms reject that", f.Name)
		}
		rc, err := f.Open()
		if err != nil {
			t.Fatal(err)
		}
		data, _ := io.ReadAll(rc)
		rc.Close()
		out[f.Name] = data
	}
	return out
}

func TestPackageForTheAuditorIsCompleteAndValid(t *testing.T) {
	a, s, _ := auditTestService(t, true)
	seedAuditData(t, s)
	if err := a.SaveProcedureProfile(store.ProcedureProfile{Responsible: "Erika Beispiel", PaperReceipts: "scan-destroy", Cash: true}); err != nil {
		t.Fatal(err)
	}
	if _, err := a.CreateProcedure(); err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	target := filepath.Join(dir, "pruefer.zip")
	result, err := a.Export(target, "2026-01-01", "2026-06-30")
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := os.ReadFile(target)
	sum := sha256.Sum256(raw)
	if hex.EncodeToString(sum[:]) != result.Sum || int64(len(raw)) != result.Bytes {
		t.Fatalf("the reported hash must be the hash of the archive: %+v", result)
	}
	if _, err = os.Stat(target + ".tmp"); !os.IsNotExist(err) {
		t.Fatal("temporary file left behind")
	}
	files := readZip(t, target)
	for _, want := range []string{"index.xml", gdpdu.DTDName, "LIESMICH.txt", "verfahrensdokumentation.pdf", "ausgangsrechnungen.csv", "ausgangspositionen.csv", "eingangsbelege.csv", "aenderungsprotokoll.csv", "dateien.csv", "belege/ausgang/RE-2026-0001.pdf", "belege/ausgang/RE-2026-0001.xml"} {
		if _, ok := files[want]; !ok {
			t.Errorf("missing %s", want)
		}
	}
	invoices := string(files["ausgangsrechnungen.csv"])
	if !strings.Contains(invoices, "RE-2026-0001") || strings.Contains(invoices, "RE-2026-0004") {
		t.Fatalf("period not respected:\n%s", invoices)
	}
	receipts := string(files["eingangsbelege.csv"])
	if !strings.Contains(receipts, "Papierhandel 'Nord'") || strings.Count(receipts, `"`)%2 != 0 {
		t.Fatalf("text delimiter inside a field:\n%s", receipts)
	}
	replaced := 0
	for name := range files {
		if strings.HasPrefix(name, "belege/ausgaben/") {
			replaced++
		}
	}
	if replaced != 2 {
		t.Fatalf("both versions of the file belong in the package, got %d", replaced)
	}
	for _, line := range strings.Split(strings.TrimSpace(string(files["dateien.csv"])), "\r\n")[1:] {
		fields := strings.Split(line, ";")
		name, want := strings.Trim(fields[0], `"`), strings.Trim(fields[3], `"`)
		got := sha256.Sum256(files[name])
		if hex.EncodeToString(got[:]) != want {
			t.Fatalf("hash of %s does not match", name)
		}
	}
	for name, data := range files {
		if len(name) > 248 {
			t.Fatalf("path too long for the upload platforms: %s", name)
		}
		if !strings.HasSuffix(name, ".csv") {
			continue
		}
		if !strings.Contains(string(files["index.xml"]), "<URL>"+name+"</URL>") {
			t.Errorf("%s is not described in index.xml", name)
		}
		if !strings.HasSuffix(string(data), "\r\n") {
			t.Errorf("%s must end with a record delimiter", name)
		}
	}
	if xmllint, err := exec.LookPath("xmllint"); err == nil {
		for _, name := range []string{"index.xml", gdpdu.DTDName} {
			if err = os.WriteFile(filepath.Join(dir, name), files[name], 0o600); err != nil {
				t.Fatal(err)
			}
		}
		cmd := exec.Command(xmllint, "--noout", "--valid", "index.xml")
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("index.xml: %v\n%s", err, out)
		}
	}
	log, err := s.store.AuditEntries(store.AuditFilter{Object: store.AuditExport})
	if err != nil || len(log) != 1 || log[0].ObjectID != result.Sum {
		t.Fatalf("the handover must be logged: %+v %v", log, err)
	}
	report, err := a.Verify()
	if err != nil || !report.OK {
		t.Fatalf("%+v %v", report, err)
	}
}

func checksumHex(text string) string {
	sum := sha256.Sum256([]byte(text))
	return hex.EncodeToString(sum[:])
}

func TestAllLocalAuditToolsAreFree(t *testing.T) {
	a, s, _ := auditTestService(t, false)
	seedAuditData(t, s)
	if _, err := a.Export(filepath.Join(t.TempDir(), "x.zip"), "2026-01-01", "2026-12-31"); err != nil {
		t.Fatalf("export (Z3): %v", err)
	}
	if _, err := a.Table(tableInvoices, "2026-01-01", "2026-12-31"); err != nil {
		t.Fatalf("table (Z2): %v", err)
	}
	if _, err := a.SaveTable(filepath.Join(t.TempDir(), "x.csv"), tableInvoices, "2026-01-01", "2026-12-31", ""); err != nil {
		t.Fatalf("saved table (Z2): %v", err)
	}
	if _, err := a.OpenView("2026-01-01", "2026-12-31"); err != nil {
		t.Fatalf("view (Z1): %v", err)
	}
	if err := a.CloseView(); err != nil {
		t.Fatal(err)
	}
	if _, err := a.Findings("2026-01-01", "2026-12-31"); err != nil {
		t.Fatalf("findings: %v", err)
	}
	if err := a.SaveProcedureProfile(store.ProcedureProfile{Responsible: "Erika Beispiel", PaperReceipts: "scan-destroy"}); err != nil {
		t.Fatal(err)
	}
	if _, err := a.CreateProcedure(); err != nil {
		t.Fatalf("procedure: %v", err)
	}
	if r, err := a.Verify(); err != nil || !r.OK {
		t.Fatalf("check: %+v %v", r, err)
	}
	if _, err := a.LockPeriod("", "2026-03-31"); err != nil {
		t.Fatalf("closing a period: %v", err)
	}
	a.anchor()
	if anchors, _ := s.store.AuditAnchors(); len(anchors) != 0 {
		t.Fatalf("receipts need a server, so they belong to Plus: %+v", anchors)
	}
}

func TestPackageCarriesTheReceiptsOfItsPeriod(t *testing.T) {
	a, s, remote := auditTestService(t, true)
	seedAuditData(t, s)
	read := func() map[string]string {
		t.Helper()
		path := filepath.Join(t.TempDir(), "x.zip")
		if _, err := a.Export(path, "2020-01-01", "2099-12-31"); err != nil {
			t.Fatal(err)
		}
		z, err := zip.OpenReader(path)
		if err != nil {
			t.Fatal(err)
		}
		defer z.Close()
		out := map[string]string{}
		for _, f := range z.File {
			if f.Name == "nachweisbericht.pdf" {
				out[f.Name] = "ja"
			}
			if strings.HasSuffix(f.Name, ".csv") || strings.HasSuffix(f.Name, ".txt") || strings.HasSuffix(f.Name, ".xml") {
				r, _ := f.Open()
				data, _ := io.ReadAll(r)
				r.Close()
				out[f.Name] = string(data)
			}
		}
		return out
	}
	if files := read(); files[tableAnchors+".csv"] != "" || files["nachweisbericht.pdf"] != "" || strings.Contains(files["LIESMICH.txt"], "nachweise.csv") {
		t.Fatal("no receipts, no table")
	}
	a.anchor()
	anchors, err := s.store.AuditAnchors()
	if err != nil || len(anchors) != 1 {
		t.Fatalf("%+v %v", anchors, err)
	}
	remote.plan = false // Receipts stay part of the package after a plan ended.
	files := read()
	table := files[tableAnchors+".csv"]
	if !strings.Contains(table, anchors[0].Hash) || !strings.Contains(table, anchors[0].Receipt) {
		t.Fatalf("receipt missing: %s", table)
	}
	if !strings.Contains(files[tableLog+".csv"], anchors[0].Hash) {
		t.Fatal("the receipt must match an entry of the log")
	}
	if !strings.Contains(files["index.xml"], tableAnchors+".csv") || !strings.Contains(files["LIESMICH.txt"], "nachweise.csv") {
		t.Fatal("receipts are not described")
	}
	if files["nachweisbericht.pdf"] == "" {
		t.Fatal("the plain report is missing")
	}

	// A receipt of a later log state stays out of an earlier period.
	if got := (auditRecords{}).anchorTable(anchors); len(got.Rows) != 0 {
		t.Fatalf("receipt without its log entry: %v", got.Rows)
	}
}

func TestFindingsNameGapsAndMissingFiles(t *testing.T) {
	a, s, _ := auditTestService(t, true)
	seedAuditData(t, s)
	findings, err := a.Findings("2026-01-01", "2026-12-31")
	if err != nil {
		t.Fatal(err)
	}
	joined := ""
	for _, f := range findings {
		joined += f.Title + ": " + strings.Join(f.Items, ", ") + "\n"
	}
	for _, want := range []string{"Lücken bei den Rechnungsnummern: RE-2026-0002", "Ausgaben ohne Belegdatei", "Tankstelle", "Barzahlungen"} {
		if !strings.Contains(joined, want) {
			t.Errorf("missing %q in\n%s", want, joined)
		}
	}
	if strings.Contains(joined, "Lücken bei den Belegnummern") {
		t.Errorf("no receipt number is missing:\n%s", joined)
	}
}

func TestGapsExplainedByTheDeletionLogAreNoFinding(t *testing.T) {
	used := map[string]bool{"RE-1": true, "RE-3": true, "RE-5": true, "A-0007": true}
	got := numberGaps(used, map[string]bool{"RE-2": true})
	if strings.Join(got, ",") != "RE-4" {
		t.Fatalf("%v", got)
	}
}

func TestReadOnlyViewBlocksWritesAndLimitsThePeriod(t *testing.T) {
	a, s, _ := auditTestService(t, true)
	seedAuditData(t, s)
	before, err := s.ListInvoices()
	if err != nil || len(before) != 3 {
		t.Fatalf("%v %v", before, err)
	}
	state, err := a.OpenView("2026-01-01", "2026-06-30")
	if err != nil || !state.Open {
		t.Fatalf("%+v %v", state, err)
	}
	list, err := s.ListInvoices()
	if err != nil || len(list) != 2 {
		t.Fatalf("an auditor sees only the audited period: %+v %v", list, err)
	}
	if !s.ReadOnly() {
		t.Fatal("background work must see the view")
	}
	if _, err = s.store.SaveExpense(store.Expense{Date: "2026-03-05", Supplier: "X", Category: "office", Gross: "1.00", TaxRate: "19", PaidWith: "bank"}, func(string) bool { return true }, "asset"); err == nil {
		t.Fatal("the view must not allow writes")
	}
	if err = a.CloseView(); err != nil {
		t.Fatal(err)
	}
	if list, _ = s.ListInvoices(); len(list) != 3 {
		t.Fatalf("after closing: %d", len(list))
	}
	if _, err = s.store.SaveExpense(store.Expense{Date: "2026-03-05", Supplier: "X", Category: "office", Gross: "1.00", TaxRate: "19", PaidWith: "bank"}, func(string) bool { return true }, "asset"); err != nil {
		t.Fatalf("writes must work again: %v", err)
	}
}

func TestTheViewStaysInItsPeriodWhateverIsAskedFor(t *testing.T) {
	a, s, _ := auditTestService(t, true)
	seedAuditData(t, s)
	if _, err := a.OpenView("2026-01-01", "2026-06-30"); err != nil {
		t.Fatal(err)
	}
	defer a.CloseView()
	view, err := a.Table(tableInvoices, "2020-01-01", "2030-12-31")
	if err != nil {
		t.Fatal(err)
	}
	for _, row := range view.Rows {
		if row[1] == "RE-2026-0004" {
			t.Fatal("an invoice outside the audited period is visible")
		}
	}
	if len(view.Rows) != 2 {
		t.Fatalf("%d rows", len(view.Rows))
	}
	issued, err := s.store.IssuedDocuments()
	if err != nil || len(issued) != 2 {
		t.Fatalf("reports see other periods: %d %v", len(issued), err)
	}
	if _, err = a.Table(tableInvoices, "2027-01-01", "2027-12-31"); err == nil {
		t.Fatal("a period outside the view must be refused")
	}
	entries, err := a.Entries(store.AuditFilter{})
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if day := dayOf(e.At); day < "2026-01-01" || day > "2026-06-30" {
			t.Fatalf("log entry outside the period: %+v", e)
		}
	}
}

func TestThePagesStillLoadInTheView(t *testing.T) {
	a, s, _ := auditTestService(t, true)
	seedAuditData(t, s)
	if _, err := a.OpenView("2026-01-01", "2026-06-30"); err != nil {
		t.Fatal(err)
	}
	defer a.CloseView()
	loads := map[string]func() error{
		"Bootstrap":       func() error { _, err := s.Bootstrap(); return err },
		"ListInvoices":    func() error { _, err := s.ListInvoices(); return err },
		"Lists":           func() error { _, err := s.ApplicationLists(); return err },
		"Dashboard":       func() error { _, err := s.DashboardSummary(); return err },
		"Finance":         func() error { _, err := s.FinanceReport("2026-01-01", "2026-06-30"); return err },
		"Payments":        func() error { _, err := s.Payments(); return err },
		"PaymentBookings": func() error { _, err := s.PaymentBookings(); return err },
		"Expenses":        func() error { _, err := s.Expenses(); return err },
		"Received":        func() error { _, err := s.ListReceivedInvoices(); return err },
		"DeletedInvoices": func() error { _, err := s.DeletedInvoices(); return err },
		"Findings":        func() error { _, err := a.Findings("2026-01-01", "2026-06-30"); return err },
		"Entries":         func() error { _, err := a.Entries(store.AuditFilter{}); return err },
		"Status":          func() error { _, err := a.Status(); return err },
	}
	for name, load := range loads {
		if err := load(); err != nil {
			t.Errorf("%s: %v", name, err)
		}
	}
}

func TestMailStopsInTheViewBecauseItCouldNotBeProtocolled(t *testing.T) {
	a, s, _ := auditTestService(t, true)
	m := &MailService{invoices: s}
	if _, err := a.OpenView("2026-01-01", "2026-06-30"); err != nil {
		t.Fatal(err)
	}
	defer a.CloseView()
	if _, err := m.SendInvoiceMail(context.Background(), InvoiceMailRequest{Confirmed: true}); !errors.Is(err, errMailReadOnly) {
		t.Fatalf("invoice mail: %v", err)
	}
	if _, err := m.SendDocumentMail(context.Background(), DocumentMailRequest{Confirmed: true}); !errors.Is(err, errMailReadOnly) {
		t.Fatalf("document mail: %v", err)
	}
	if _, err := m.SaveSettings(mail.Settings{}, ""); !errors.Is(err, errMailReadOnly) {
		t.Fatalf("settings: %v", err)
	}
	if _, err := m.ForgetPassword(); !errors.Is(err, errMailReadOnly) {
		t.Fatalf("password: %v", err)
	}
}

func TestReceiptsAreRequestedOnceADayAndStayValid(t *testing.T) {
	a, s, remote := auditTestService(t, true)
	seedAuditData(t, s)
	a.anchor()
	if remote.calls != 1 {
		t.Fatalf("calls %d", remote.calls)
	}
	a.anchor()
	if remote.calls != 1 {
		t.Fatalf("an unchanged log needs no new receipt: %d", remote.calls)
	}
	if _, err := s.store.LockPeriod("", "2026-03-31"); err != nil {
		t.Fatal(err)
	}
	a.anchor()
	if remote.calls != 1 {
		t.Fatalf("one receipt a day is enough: %d", remote.calls)
	}
	a.now = func() time.Time { return time.Now().Add(25 * time.Hour) }
	a.anchor()
	if remote.calls != 2 {
		t.Fatalf("calls %d", remote.calls)
	}
	report, err := a.Verify()
	if err != nil || !report.OK || report.Anchors != 2 {
		t.Fatalf("%+v %v", report, err)
	}
	if err = s.store.AddAuditAnchor(store.AuditAnchor{Seq: 1, Hash: "falsch", At: "2026-01-01T00:00:00Z", Receipt: "x"}); err != nil {
		t.Fatal(err)
	}
	if report, _ = a.Verify(); report.OK {
		t.Fatal("a receipt that does not match must be reported")
	}
}

func TestServerListBringsBackLostReceiptsAndExposesAChangedLog(t *testing.T) {
	a, s, remote := auditTestService(t, true)
	seedAuditData(t, s)
	seq, head, err := s.store.AuditHead()
	if err != nil {
		t.Fatal(err)
	}
	// The receipt is only on the server, for example after a reinstall.
	remote.list = []account.StoredReceipt{remote.sign(t, auditTestDevice, seq, head)}
	h := a.check()
	if !h.OK || h.ComparedAt == "" || h.Matched != 1 || h.Restored != 1 || h.Conflicts != 0 {
		t.Fatalf("restore: %+v", h)
	}
	if anchors, _ := s.store.AuditAnchors(); len(anchors) != 1 || anchors[0].Hash != head {
		t.Fatalf("receipt not restored: %+v", anchors)
	}
	if h = a.check(); h.Restored != 0 || h.Matched != 1 {
		t.Fatalf("second check: %+v", h)
	}

	// Another device has a log of its own and is no conflict.
	remote.list = append(remote.list, remote.sign(t, "anderes-geraet-bbbbbbbb", seq, checksumHex("anderes Protokoll")))
	if h = a.check(); h.Conflicts != 0 {
		t.Fatalf("other device: %+v", h)
	}

	// This device's confirmed state no longer matches its log.
	remote.list = append(remote.list, remote.sign(t, auditTestDevice, seq+40, checksumHex("war einmal")))
	if h = a.check(); h.Conflicts != 1 {
		t.Fatalf("changed log: %+v", h)
	}

	// A forged list entry counts as a conflict and never lands here.
	forged := remote.sign(t, auditTestDevice, seq, head)
	forged.Receipt = "gefaelscht"
	remote.list = []account.StoredReceipt{forged}
	if h = a.check(); h.Conflicts != 1 || h.Matched != 0 {
		t.Fatalf("forged: %+v", h)
	}

	// Offline the last comparison stays.
	before := h.ComparedAt
	remote.failing = true
	if h = a.check(); h.ComparedAt != before {
		t.Fatalf("offline: %+v", h)
	}
	if status, err := a.Status(); err != nil || status.Health.ComparedAt != before {
		t.Fatalf("status: %+v %v", status.Health, err)
	}
}

func TestPlusWatchesTheLogOnceADay(t *testing.T) {
	a, s, remote := auditTestService(t, false)
	seedAuditData(t, s)
	a.watch()
	if a.health.CheckedAt != "" {
		t.Fatal("the daily check belongs to Plus")
	}
	remote.plan = true
	a.watch()
	first := a.health.CheckedAt
	if first == "" || !a.health.OK {
		t.Fatalf("no check: %+v", a.health)
	}
	later := time.Now().Add(2 * time.Hour)
	a.now = func() time.Time { return later }
	a.watch()
	if a.health.CheckedAt != first {
		t.Fatal("once a day is enough")
	}
	later = time.Now().Add(25 * time.Hour)
	a.watch()
	if a.health.CheckedAt == first {
		t.Fatal("the next day needs a new check")
	}
}

func TestMissingReceiptsAreNoticedOnlyWhenTheLogMovedOn(t *testing.T) {
	a, s, _ := auditTestService(t, true)
	seedAuditData(t, s)
	a.anchor()
	if h := a.check(); h.Stale {
		t.Fatalf("fresh receipt: %+v", h)
	}
	a.now = func() time.Time { return time.Now().Add(8 * 24 * time.Hour) }
	if h := a.check(); h.Stale {
		t.Fatalf("nothing changed, nothing to confirm: %+v", h)
	}
	if _, err := s.store.LockPeriod("", "2026-03-31"); err != nil {
		t.Fatal(err)
	}
	if h := a.check(); !h.Stale {
		t.Fatalf("a week of changes without a receipt: %+v", h)
	}
}

func TestNoReceiptsWithoutPlanOrWhileOffline(t *testing.T) {
	a, s, remote := auditTestService(t, false)
	seedAuditData(t, s)
	a.anchor()
	if remote.calls != 0 {
		t.Fatal("no receipts without a plan")
	}
	remote.plan, remote.failing = true, true
	a.anchor()
	if anchors, _ := s.store.AuditAnchors(); len(anchors) != 0 {
		t.Fatal("a failed request stores nothing")
	}
}

func TestProcedureVersionsStayAndNeedTheAnswers(t *testing.T) {
	a, _, _ := auditTestService(t, true)
	if _, err := a.CreateProcedure(); err == nil {
		t.Fatal("the answers of the business are required")
	}
	if err := a.SaveProcedureProfile(store.ProcedureProfile{Responsible: "Erika", PaperReceipts: "unsinn"}); err == nil {
		t.Fatal("unknown paper handling accepted")
	}
	if err := a.SaveProcedureProfile(store.ProcedureProfile{Responsible: "Erika Beispiel", PaperReceipts: "keep", Advisor: "Kanzlei Weber"}); err != nil {
		t.Fatal(err)
	}
	first, err := a.CreateProcedure()
	if err != nil {
		t.Fatal(err)
	}
	second, err := a.CreateProcedure()
	if err != nil {
		t.Fatal(err)
	}
	if first.Version != 1 || second.Version != 2 {
		t.Fatalf("%+v %+v", first, second)
	}
	path := filepath.Join(t.TempDir(), "vd.pdf")
	if err = a.SaveProcedure(path, 1); err != nil {
		t.Fatal(err)
	}
	if raw, _ := os.ReadFile(path); !strings.HasPrefix(string(raw), "%PDF") {
		t.Fatal("not a PDF")
	}
	status, err := a.Status()
	if err != nil || len(status.Procedure) != 2 {
		t.Fatalf("%+v %v", status, err)
	}
	if report, _ := a.Verify(); !report.OK {
		t.Fatalf("%+v", report.Problems)
	}
}
