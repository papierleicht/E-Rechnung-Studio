package app

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"papierleicht.de/e-rechnung-studio/internal/account"
	"papierleicht.de/e-rechnung-studio/internal/cloudbackup"
	"papierleicht.de/e-rechnung-studio/internal/model"
	"papierleicht.de/e-rechnung-studio/internal/store"
)

type memoryVault struct{ m map[string]string }

func (v *memoryVault) Get(k string) (string, error) { return v.m[k], nil }
func (v *memoryVault) Set(k, s string) error        { v.m[k] = s; return nil }
func (v *memoryVault) Delete(k string) error        { delete(v.m, k); return nil }

// fakeCloud stores ciphertext like the license server does: objects are
// written once and a version needs all of its objects.
type fakeCloud struct {
	mu        sync.Mutex
	backup    bool
	versions  []account.BackupVersion
	manifests map[string][]byte
	objects   map[string][]byte
	// objectUploads counts every stored object; failAfter stops the transfer.
	objectUploads int
	failAfter     int
	deletionAt    time.Time
}

func newFakeCloud() *fakeCloud {
	return &fakeCloud{backup: true, manifests: map[string][]byte{}, objects: map[string][]byte{}, failAfter: -1}
}

func (f *fakeCloud) HasFeature(string) bool { f.mu.Lock(); defer f.mu.Unlock(); return f.backup }
func (f *fakeCloud) DeviceName() string     { return "Testgerät" }
func (f *fakeCloud) BackupState(context.Context) (account.BackupState, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	st := account.BackupState{Access: "active", Versions: len(f.versions), QuotaBytes: 1 << 30}
	if !f.deletionAt.IsZero() {
		st.DeletionAt = &f.deletionAt
	}
	return st, nil
}
func (f *fakeCloud) ListBackups(context.Context) ([]account.BackupVersion, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if !f.backup {
		return nil, errors.New("kein Abo")
	}
	return slices.Clone(f.versions), nil
}
func (f *fakeCloud) MissingObjects(_ context.Context, ids []string) ([]string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	missing := []string{}
	for _, id := range ids {
		if _, ok := f.objects[id]; !ok {
			missing = append(missing, id)
		}
	}
	return missing, nil
}
func (f *fakeCloud) UploadObject(_ context.Context, id string, data []byte) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.failAfter == 0 {
		return account.ErrOffline
	}
	if f.failAfter > 0 {
		f.failAfter--
	}
	if _, ok := f.objects[id]; !ok {
		f.objects[id] = bytes.Clone(data)
		f.objectUploads++
	}
	return nil
}
func (f *fakeCloud) DownloadObject(_ context.Context, id string) ([]byte, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	data, ok := f.objects[id]
	if !ok {
		return nil, &account.APIError{Status: 404, Message: "Das Dokument wurde nicht gefunden."}
	}
	return bytes.Clone(data), nil
}
func (f *fakeCloud) UploadBackup(_ context.Context, format int, manifest []byte, objects []string, logical int64) (account.BackupVersion, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, id := range objects {
		if _, ok := f.objects[id]; !ok {
			return account.BackupVersion{}, &account.APIError{Status: 409, Message: "Es fehlen Dokumente."}
		}
	}
	id := "v" + strconv.Itoa(len(f.versions)+1)
	v := account.BackupVersion{ID: id, DeviceName: "Testgerät", Size: int64(len(manifest)), FormatVersion: format, Objects: len(objects), LogicalSize: logical}
	f.versions = append([]account.BackupVersion{v}, f.versions...)
	f.manifests[id] = bytes.Clone(manifest)
	return v, nil
}
func (f *fakeCloud) DownloadBackup(_ context.Context, id string) ([]byte, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return bytes.Clone(f.manifests[id]), nil
}
func (f *fakeCloud) RequestBackupDeletion(context.Context) (time.Time, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.deletionAt = time.Date(2026, 11, 9, 12, 0, 0, 0, time.UTC)
	return f.deletionAt, nil
}
func (f *fakeCloud) CancelBackupDeletion(context.Context) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.deletionAt = time.Time{}
	return nil
}

func (f *fakeCloud) uploads() int { f.mu.Lock(); defer f.mu.Unlock(); return len(f.versions) }

func newBackupTest(t *testing.T, cloud *fakeCloud, vault *memoryVault) (*BackupService, *InvoiceService, string) {
	t.Helper()
	dir := t.TempDir()
	inv, err := NewInvoiceService(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { inv.Close() })
	return NewBackupService(dir, inv, cloud, vault), inv, dir
}

func setupKey(t *testing.T, s *BackupService) string {
	t.Helper()
	key, err := s.CreateBackupKey()
	if err != nil {
		t.Fatal(err)
	}
	if st := s.BackupStatus(); st.KeyConfigured || !st.KeyPending {
		t.Fatalf("key stored before confirmation: %+v", st)
	}
	if _, err := s.ConfirmBackupKey(strings.Replace(key, key[:4], "AAAA", 1)); err == nil && key[:4] != "AAAA" {
		t.Fatal("confirmation with a different key accepted")
	}
	if st, err := s.ConfirmBackupKey(strings.ToLower(key)); err != nil || !st.KeyConfigured || st.KeyPending {
		t.Fatalf("confirm: %+v %v", st, err)
	}
	return key
}

// addDocuments stores a photo receipt, an incoming document and an issued
// invoice, so a state holds every kind of file.
func addDocuments(t *testing.T, inv *InvoiceService, suffix string) (receipt, photo []byte, expense store.Expense) {
	t.Helper()
	receipt = []byte("%PDF-1.4 Eingangsrechnung " + suffix + strings.Repeat(" Inhalt", 2000))
	stored, err := inv.store.SaveReceived(model.ReceivedInvoice{ID: storeSum(receipt), Filename: "beleg" + suffix + ".pdf"}, receipt)
	if err != nil || stored.Duplicate {
		t.Fatalf("receipt: %+v %v", stored, err)
	}
	photo = bytes.Repeat([]byte("\xff\xd8\xff Foto "+suffix), 20000)
	expense, err = inv.store.SaveExpense(store.Expense{Date: "2026-10-01", Supplier: "Bäckerei " + suffix, Category: "sonstige", Gross: "11.90", TaxRate: "19", PaidWith: "cash",
		AttachmentName: "foto.jpg", AttachmentType: "image/jpeg", Attachment: photo}, func(string) bool { return true }, "asset")
	if err != nil {
		t.Fatal(err)
	}
	return receipt, photo, expense
}

func storeSum(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func issue(t *testing.T, inv *InvoiceService, number string) string {
	t.Helper()
	if _, err := inv.store.Company(); err != nil {
		t.Fatal(err)
	}
	d := testDraft()
	d.Number = number
	issued, err := inv.IssueInvoice(d)
	if err != nil {
		t.Fatal(err)
	}
	return issued.Draft.ID
}

func TestCloudBackupRequiresPlan(t *testing.T) {
	cloud := newFakeCloud()
	cloud.backup = false
	s, _, _ := newBackupTest(t, cloud, &memoryVault{m: map[string]string{}})
	if _, err := s.CreateBackupKey(); err == nil {
		t.Fatal("key without backup plan")
	}
	if _, err := s.BackupNow(); err == nil {
		t.Fatal("backup without plan")
	}
	if _, err := s.BackupVersions(); err == nil {
		t.Fatal("versions without plan")
	}
}

func TestCloudBackupRestoreOnNewDevice(t *testing.T) {
	cloud := newFakeCloud()
	vault := &memoryVault{m: map[string]string{}}
	s, inv, _ := newBackupTest(t, cloud, vault)
	if _, err := s.BackupNow(); err == nil {
		t.Fatal("backup without key")
	}
	key := setupKey(t, s)
	if _, err := s.CreateBackupKey(); err == nil {
		t.Fatal("second key replaced the stored one")
	}
	if shown, err := s.ShowBackupKey(); err != nil || !strings.EqualFold(shown, key) {
		t.Fatalf("key cannot be shown again: %v", err)
	}
	company := testCompany()
	company.Name = "Geheimfirma Beispiel GmbH"
	if _, err := inv.SaveCompany(company); err != nil {
		t.Fatal(err)
	}
	if _, err := inv.SaveCustomer(model.Customer{Name: "Kundin Eins"}); err != nil {
		t.Fatal(err)
	}
	receipt, photo, expense := addDocuments(t, inv, "A")
	invoice := issue(t, inv, "RE-2026-0001")
	pdfBefore, err := inv.store.Artifact(invoice, "facturx-pdf")
	if err != nil || len(pdfBefore) == 0 {
		t.Fatalf("artifact: %v", err)
	}
	st, err := s.BackupNow()
	if err != nil || st.LastBackupAt == "" || cloud.uploads() != 1 {
		t.Fatalf("backup: %+v %v", st, err)
	}
	for id, data := range cloud.objects {
		if bytes.Contains(data, []byte("Geheimfirma")) || bytes.Contains(data, []byte("Eingangsrechnung")) {
			t.Fatalf("server received plaintext in %s", id)
		}
	}

	// New device: empty data, same account, key typed in by the customer.
	other, otherInv, otherDir := newBackupTest(t, cloud, &memoryVault{m: map[string]string{}})
	if _, err := otherInv.SaveCustomer(model.Customer{Name: "Lokal vorhanden"}); err != nil {
		t.Fatal(err)
	}
	if _, err := other.PreviewRestore("v1"); err == nil {
		t.Fatal("preview without key")
	}
	if _, err := other.ImportBackupKey("FALSCH-1234"); err == nil {
		t.Fatal("invalid key imported")
	}
	if _, err := other.RestoreVersion("v1"); err == nil {
		t.Fatal("restore without preview")
	}
	if _, err := other.ImportBackupKey(key); err != nil {
		t.Fatal(err)
	}
	p, err := other.PreviewRestore("v1")
	if err != nil || p.Backup.Customers != 1 || p.Backup.Received != 1 || p.Backup.Expenses != 1 || p.Backup.Invoices != 1 || p.Backup.DeviceName != "Testgerät" || p.Backup.Documents < 4 {
		t.Fatalf("preview %+v %v", p, err)
	}
	res, err := other.RestoreVersion("v1")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(res.SafetyCopy, otherDir) {
		t.Fatalf("safety copy outside the data directory: %s", res.SafetyCopy)
	}
	safety, err := openArchive(res.SafetyCopy)
	if err != nil || !bytes.Contains(safety.document, []byte("Lokal vorhanden")) {
		t.Fatalf("replaced data was not saved locally first: %v", err)
	}
	safety.Close()
	restored, err := otherInv.store.Company()
	if err != nil || restored.Name != "Geheimfirma Beispiel GmbH" {
		t.Fatalf("restored company %q, %v", restored.Name, err)
	}
	lists, _ := otherInv.ApplicationLists()
	if len(lists.Customers) != 1 || lists.Customers[0].Name != "Kundin Eins" {
		t.Fatalf("restored customers %+v", lists.Customers)
	}
	// Every document comes back byte for byte.
	if _, original, err := otherInv.store.GetReceived(storeSum(receipt)); err != nil || !bytes.Equal(original, receipt) {
		t.Fatalf("receipt changed: %v", err)
	}
	if e, err := otherInv.store.ExpenseAttachment(expense.ID); err != nil || !bytes.Equal(e.Attachment, photo) {
		t.Fatalf("photo changed: %v", err)
	}
	if pdf, err := otherInv.store.Artifact(invoice, "facturx-pdf"); err != nil || !bytes.Equal(pdf, pdfBefore) {
		t.Fatalf("invoice PDF changed: %v", err)
	}
	if entries, _ := os.ReadDir(filepath.Join(otherDir, "Sicherungen")); len(entries) != 1 {
		t.Fatalf("temporary restore files left: %v", entries)
	}
	// The restore is on record on the device it happened on.
	log, err := other.BackupLog()
	if err != nil || len(log) != 1 || log[0].Kind != store.BackupEventRestore || !strings.Contains(log[0].Summary, "wiederhergestellt") {
		t.Fatalf("protocol %+v %v", log, err)
	}

	// A device with a different key cannot read the backup.
	third, _, _ := newBackupTest(t, cloud, &memoryVault{m: map[string]string{}})
	k, _ := cloudbackup.NewKey()
	third.ImportBackupKey(k.String())
	if _, err := third.PreviewRestore("v1"); err == nil || !strings.Contains(err.Error(), "anderen Wiederherstellungsschlüssel") {
		t.Fatalf("foreign key: %v", err)
	}
}

func TestBackupStoresEveryDocumentOnce(t *testing.T) {
	cloud := newFakeCloud()
	s, inv, _ := newBackupTest(t, cloud, &memoryVault{m: map[string]string{}})
	setupKey(t, s)
	if _, err := inv.SaveCompany(testCompany()); err != nil {
		t.Fatal(err)
	}
	addDocuments(t, inv, "A")
	if _, err := s.BackupNow(); err != nil {
		t.Fatal(err)
	}
	first := cloud.objectUploads
	// Nothing changed: a new version needs no document again.
	if _, err := s.BackupNow(); err != nil {
		t.Fatal(err)
	}
	if cloud.objectUploads != first || cloud.uploads() != 2 {
		t.Fatalf("%d objects, %d versions", cloud.objectUploads, cloud.uploads())
	}
	// One new photo adds that photo and the small changed data, not everything.
	addDocuments(t, inv, "B")
	if _, err := s.BackupNow(); err != nil {
		t.Fatal(err)
	}
	if added := cloud.objectUploads - first; added == 0 || added > 4 {
		t.Fatalf("%d objects added", added)
	}
}

func TestBackupContinuesAfterInterruption(t *testing.T) {
	cloud := newFakeCloud()
	s, inv, _ := newBackupTest(t, cloud, &memoryVault{m: map[string]string{}})
	setupKey(t, s)
	addDocuments(t, inv, "A")
	cloud.failAfter = 1
	st, err := s.BackupNow()
	if err == nil || st.LastError == "" || cloud.uploads() != 0 || cloud.objectUploads != 1 {
		t.Fatalf("interrupted: %+v %v, %d versions, %d objects", st, err, cloud.uploads(), cloud.objectUploads)
	}
	// No version exists until it is complete; the next run sends the rest.
	cloud.failAfter = -1
	st, err = s.BackupNow()
	if err != nil || st.LastError != "" || cloud.uploads() != 1 {
		t.Fatalf("resume: %+v %v", st, err)
	}
	if cloud.objectUploads < 2 {
		t.Fatalf("%d objects", cloud.objectUploads)
	}
}

func TestVerifyFindsDamagedDocuments(t *testing.T) {
	cloud := newFakeCloud()
	s, inv, _ := newBackupTest(t, cloud, &memoryVault{m: map[string]string{}})
	setupKey(t, s)
	addDocuments(t, inv, "A")
	if _, err := s.BackupNow(); err != nil {
		t.Fatal(err)
	}
	res, err := s.VerifyVersion("v1")
	if err != nil || !res.OK || res.Documents != 2 || !strings.Contains(res.Summary, "2 Dokumente") {
		t.Fatalf("verify: %+v %v", res, err)
	}
	if st := s.BackupStatus(); st.LastVerifiedAt == "" || !st.Pending {
		t.Fatalf("status %+v", st)
	}
	// A damaged object is reported and recorded, not hidden.
	k, _ := s.key()
	env, _ := cloudbackup.Unpack(k, cloud.manifests["v1"])
	damaged := k.ObjectID(env.Blobs[0].Sum)
	cloud.objects[damaged][len(cloud.objects[damaged])-1] ^= 1
	res, err = s.VerifyVersion("v1")
	if err != nil || res.OK || !strings.Contains(res.Summary, "fehlgeschlagen") {
		t.Fatalf("damaged: %+v %v", res, err)
	}
	// A missing one as well.
	delete(cloud.objects, damaged)
	if res, err = s.VerifyVersion("v1"); err != nil || res.OK {
		t.Fatalf("missing: %+v %v", res, err)
	}
	log, _ := s.BackupLog()
	if len(log) != 3 || !log[2].OK || log[0].OK || log[1].OK {
		t.Fatalf("protocol %+v", log)
	}
	// A restore refuses such a state before it changes anything.
	if _, err := s.PreviewRestore("v1"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.RestoreVersion("v1"); err == nil {
		t.Fatal("restored a state with a missing document")
	}
	if list, _ := inv.store.Expenses(); len(list) != 1 {
		t.Fatal("local data changed by a failed restore")
	}
}

func TestRestoreOfOlderStateKeepsProtocolAndNumbers(t *testing.T) {
	cloud := newFakeCloud()
	s, inv, _ := newBackupTest(t, cloud, &memoryVault{m: map[string]string{}})
	setupKey(t, s)
	if _, err := inv.SaveCompany(testCompany()); err != nil {
		t.Fatal(err)
	}
	issue(t, inv, "RE-2026-0001")
	if _, err := s.BackupNow(); err != nil {
		t.Fatal(err)
	}
	counters := func() map[string]int {
		rows, err := inv.store.DB().Query("SELECT name,value FROM counters")
		if err != nil {
			t.Fatal(err)
		}
		defer rows.Close()
		out := map[string]int{}
		for rows.Next() {
			var name string
			var value int
			rows.Scan(&name, &value)
			out[name] = value
		}
		return out
	}
	// Issued after the backup, so a restore of v1 removes it.
	d := testDraft()
	d.Number, d.AutomaticNumber = "", true
	later, err := inv.IssueInvoice(d)
	if err != nil {
		t.Fatal(err)
	}
	before := counters()
	if err := inv.store.AddBackupEvent(store.BackupEvent{Kind: store.BackupEventVerify, OK: true, Summary: "Geprüft"}); err != nil {
		t.Fatal(err)
	}
	p, err := s.PreviewRestore("v1")
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(p.Impact.Invoices, []string{later.Draft.Number}) {
		t.Fatalf("impact %+v", p.Impact)
	}
	res, err := s.RestoreVersion("v1")
	if err != nil {
		t.Fatal(err)
	}
	after := counters()
	for name, value := range before {
		if after[name] < value {
			t.Fatalf("counter %s went back from %d to %d", name, value, after[name])
		}
	}
	log, _ := s.BackupLog()
	if len(log) != 2 || log[0].Kind != store.BackupEventRestore || log[1].Kind != store.BackupEventVerify {
		t.Fatalf("protocol lost entries: %+v", log)
	}
	if !strings.Contains(log[0].Summary, filepath.Base(res.SafetyCopy)) {
		t.Fatalf("restore entry misses the safety copy: %s", log[0].Summary)
	}
}

func TestBackupFileRoundTrip(t *testing.T) {
	s, inv, dir := newBackupTest(t, newFakeCloud(), &memoryVault{m: map[string]string{}})
	receipt, photo, expense := addDocuments(t, inv, "A")
	path := filepath.Join(dir, "sicherung.zip")
	res, err := s.ExportArchive(path)
	if err != nil || res.Documents != 2 {
		t.Fatalf("export: %+v %v", res, err)
	}
	if log, _ := s.BackupLog(); len(log) != 1 || log[0].Kind != store.BackupEventExport {
		t.Fatalf("protocol %+v", log)
	}
	// The archive works without the app: originals under their real format.
	z, err := zip.OpenReader(path)
	if err != nil {
		t.Fatal(err)
	}
	names := map[string]bool{}
	for _, f := range z.File {
		names[f.Name] = true
	}
	z.Close()
	if !names[archiveReadme] || !names[archiveDocument] || !names[archiveFolder+storeSum(receipt)+".pdf"] || !names[archiveFolder+storeSum(photo)+".jpg"] {
		t.Fatalf("archive %v", names)
	}

	other, otherInv, _ := newBackupTest(t, newFakeCloud(), &memoryVault{m: map[string]string{}})
	p, err := other.InspectArchive(path)
	if err != nil || p.Backup.Received != 1 || p.Backup.Expenses != 1 || p.Backup.Documents != 2 {
		t.Fatalf("inspect %+v %v", p, err)
	}
	if _, err := other.ImportArchive(path); err != nil {
		t.Fatal(err)
	}
	if _, original, err := otherInv.store.GetReceived(storeSum(receipt)); err != nil || !bytes.Equal(original, receipt) {
		t.Fatalf("receipt changed: %v", err)
	}
	if e, err := otherInv.store.ExpenseAttachment(expense.ID); err != nil || !bytes.Equal(e.Attachment, photo) {
		t.Fatalf("photo changed: %v", err)
	}
	if log, _ := other.BackupLog(); len(log) != 1 || log[0].Kind != store.BackupEventRestore {
		t.Fatalf("protocol %+v", log)
	}
}

func TestBackupFileWithChangedDocumentIsRefused(t *testing.T) {
	s, inv, dir := newBackupTest(t, newFakeCloud(), &memoryVault{m: map[string]string{}})
	receipt, _, _ := addDocuments(t, inv, "A")
	path := filepath.Join(dir, "sicherung.zip")
	if _, err := s.ExportArchive(path); err != nil {
		t.Fatal(err)
	}
	// Rewrite the archive with one altered receipt, as an editor would.
	in, err := zip.OpenReader(path)
	if err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	zw := zip.NewWriter(&out)
	for _, f := range in.File {
		data, _ := readArchiveFile(f, maxArchiveDocument)
		if strings.Contains(f.Name, storeSum(receipt)) {
			data = bytes.Replace(data, []byte("Eingangsrechnung"), []byte("Eingangsrechnunh"), 1)
		}
		w, _ := zw.Create(f.Name)
		w.Write(data)
	}
	zw.Close()
	in.Close()
	changed := filepath.Join(dir, "verändert.zip")
	if err := os.WriteFile(changed, out.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}
	other, otherInv, _ := newBackupTest(t, newFakeCloud(), &memoryVault{m: map[string]string{}})
	if _, err := otherInv.SaveCustomer(model.Customer{Name: "Bleibt"}); err != nil {
		t.Fatal(err)
	}
	if _, err := other.ImportArchive(changed); err == nil || !strings.Contains(err.Error(), "verändert") {
		t.Fatalf("changed document accepted: %v", err)
	}
	if lists, _ := otherInv.ApplicationLists(); len(lists.Customers) != 1 {
		t.Fatal("local data changed by a refused import")
	}
}

func TestOldJSONBackupStillImports(t *testing.T) {
	_, inv, dir := newBackupTest(t, newFakeCloud(), &memoryVault{m: map[string]string{}})
	if _, err := inv.SaveCustomer(model.Customer{Name: "Aus alter Sicherung"}); err != nil {
		t.Fatal(err)
	}
	backup, err := inv.ExportBackup()
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "alt.json")
	if err := os.WriteFile(path, []byte(backup), 0o600); err != nil {
		t.Fatal(err)
	}
	other, otherInv, _ := newBackupTest(t, newFakeCloud(), &memoryVault{m: map[string]string{}})
	if p, err := other.InspectArchive(path); err != nil || p.Backup.Customers != 1 {
		t.Fatalf("inspect %+v %v", p, err)
	}
	if _, err := other.ImportArchive(path); err != nil {
		t.Fatal(err)
	}
	if lists, _ := otherInv.ApplicationLists(); len(lists.Customers) != 1 {
		t.Fatal("customers missing")
	}
}

func TestBackupDeletionCanBeRequestedAndCanceled(t *testing.T) {
	cloud := newFakeCloud()
	s, _, _ := newBackupTest(t, cloud, &memoryVault{m: map[string]string{}})
	at, err := s.DeleteAllBackups()
	if err != nil || at != "2026-11-09T12:00:00Z" {
		t.Fatalf("deletion: %q %v", at, err)
	}
	if st, err := s.BackupAccount(); err != nil || st.DeletionAt != at {
		t.Fatalf("account %+v %v", st, err)
	}
	if err := s.CancelBackupDeletion(); err != nil {
		t.Fatal(err)
	}
	if st, _ := s.BackupAccount(); st.DeletionAt != "" {
		t.Fatalf("account %+v", st)
	}
}

func TestAutoBackupThrottlesAndFlushesOnShutdown(t *testing.T) {
	cloud := newFakeCloud()
	s, inv, _ := newBackupTest(t, cloud, &memoryVault{m: map[string]string{}})
	now := time.Date(2026, 10, 26, 12, 0, 0, 0, time.UTC)
	s.now = func() time.Time { return now }
	setupKey(t, s)

	// A device without data has nothing worth a version, e.g. a new one.
	s.autoBackup(false)
	if cloud.uploads() != 0 {
		t.Fatal("empty device created a version")
	}
	inv.SaveCustomer(model.Customer{Name: "Erste"})
	s.autoBackup(false)
	if cloud.uploads() != 1 {
		t.Fatalf("first automatic backup: %d uploads", cloud.uploads())
	}
	now = now.Add(2 * time.Hour)
	s.autoBackup(false)
	if cloud.uploads() != 1 {
		t.Fatal("unchanged data uploaded again")
	}
	inv.SaveCustomer(model.Customer{Name: "Neu"})
	now = now.Add(2 * time.Hour)
	s.autoBackup(false)
	if cloud.uploads() != 2 {
		t.Fatal("change not uploaded")
	}
	inv.SaveCustomer(model.Customer{Name: "Noch einer"})
	now = now.Add(10 * time.Minute)
	s.autoBackup(false)
	if st := s.BackupStatus(); cloud.uploads() != 2 || !st.Pending {
		t.Fatalf("not throttled: %d uploads, %+v", cloud.uploads(), st)
	}
	s.startLoop()
	if err := s.ServiceShutdown(); err != nil {
		t.Fatal(err)
	}
	if st := s.BackupStatus(); cloud.uploads() != 3 || st.Pending {
		t.Fatalf("shutdown did not flush: %d uploads, %+v", cloud.uploads(), st)
	}

	s.SetAutoBackup(false)
	inv.SaveCustomer(model.Customer{Name: "Ohne Automatik"})
	now = now.Add(2 * time.Hour)
	s.autoBackup(false)
	if cloud.uploads() != 3 {
		t.Fatal("automatic backup ran while disabled")
	}
}

func TestBackupFingerprintIgnoresExportTime(t *testing.T) {
	a := "{\n  \"format\": \"x\",\n  \"exportedAt\": \"2026-10-26T10:00:00Z\",\n  \"settings\": []\n}"
	b := strings.Replace(a, "10:00:00", "11:00:00", 1)
	if fingerprint(a) != fingerprint(b) {
		t.Fatal("export time changes the fingerprint")
	}
	if fingerprint(a) == fingerprint(strings.Replace(a, "[]", "[1]", 1)) {
		t.Fatal("content change not detected")
	}
}

func TestOptionalKeyCheckComesBackOncePerYear(t *testing.T) {
	cloud := newFakeCloud()
	s, _, _ := newBackupTest(t, cloud, &memoryVault{m: map[string]string{}})
	if s.BackupStatus().KeyCheckDue {
		t.Fatal("no key, no check")
	}
	setupKey(t, s)
	if s.BackupStatus().KeyCheckDue {
		t.Fatal("a new key needs no check yet")
	}
	later := time.Now().Add(keyCheckInterval + time.Hour)
	s.now = func() time.Time { return later }
	if !s.BackupStatus().KeyCheckDue {
		t.Fatal("a year later the check is offered")
	}
	// Postponing keeps the backup running and asks again later.
	if st := s.PostponeKeyCheck(); st.KeyCheckDue {
		t.Fatal("postponed")
	}
	later = later.Add(keyCheckPostpone + time.Hour)
	if !s.BackupStatus().KeyCheckDue {
		t.Fatal("the postponed check comes back")
	}
	// A wrong key changes nothing.
	if _, err := s.CheckBackupKey("AAAA-AAAA"); err == nil {
		t.Fatal("wrong key accepted")
	}
	other, err := cloudbackup.NewKey()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.CheckBackupKey(other.String()); err == nil {
		t.Fatal("foreign key accepted")
	}
	if !s.BackupStatus().KeyCheckDue {
		t.Fatal("a failed check must not count")
	}
	right, err := s.ShowBackupKey()
	if err != nil {
		t.Fatal(err)
	}
	st, err := s.CheckBackupKey(right)
	if err != nil || st.KeyCheckDue {
		t.Fatalf("right key: %+v %v", st, err)
	}
}
