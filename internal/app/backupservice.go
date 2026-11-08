package app

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"
	"papierleicht.de/e-rechnung-studio/internal/account"
	"papierleicht.de/e-rechnung-studio/internal/cloudbackup"
	"papierleicht.de/e-rechnung-studio/internal/entitlement"
	"papierleicht.de/e-rechnung-studio/internal/store"
)

// backupKeyName is the keychain entry (service account.KeyringService) of
// the recovery key. It never leaves the device.
const backupKeyName = "backup-key"

const (
	// autoBackupInterval throttles automatic uploads after changes.
	autoBackupInterval = time.Hour
	// keyCheckInterval and keyCheckPostpone pace the optional key check.
	keyCheckInterval = 365 * 24 * time.Hour
	keyCheckPostpone = 90 * 24 * time.Hour
	// backupCheckInterval is how often local changes are looked for.
	backupCheckInterval = 10 * time.Minute
	// objectTimeout bounds the transfer of one document.
	objectTimeout = 5 * time.Minute
	// backupCallTimeout bounds short calls that return little data.
	backupCallTimeout = 30 * time.Second
	shutdownTimeout   = 30 * time.Second
)

type backupSource interface {
	ExportSnapshot() (*store.Snapshot, error)
	ReadBlob(store.SnapshotBlob) ([]byte, error)
	RestoreSnapshot(document []byte, open store.BlobOpener, event store.BackupEvent) error
	RestoreBackup(string) error
	RestoreImpact(document []byte) (store.RestoreImpact, error)
	AddBackupEvent(store.BackupEvent) error
	BackupEvents() ([]store.BackupEvent, error)
}

type backupRemote interface {
	HasFeature(string) bool
	DeviceName() string
	BackupState(context.Context) (account.BackupState, error)
	ListBackups(context.Context) ([]account.BackupVersion, error)
	MissingObjects(context.Context, []string) ([]string, error)
	UploadObject(context.Context, string, []byte) error
	DownloadObject(context.Context, string) ([]byte, error)
	UploadBackup(context.Context, int, []byte, []string, int64) (account.BackupVersion, error)
	DownloadBackup(context.Context, string) ([]byte, error)
	RequestBackupDeletion(context.Context) (time.Time, error)
	CancelBackupDeletion(context.Context) error
}

// backupState is stored next to the database; it holds no secrets.
type backupState struct {
	Auto            bool      `json:"auto"`
	LastBackupAt    time.Time `json:"lastBackupAt"`
	LastFingerprint string    `json:"lastFingerprint"`
	LastError       string    `json:"lastError"`
	LastErrorAt     time.Time `json:"lastErrorAt"`
	LastVerifiedAt  time.Time `json:"lastVerifiedAt"`
	// KeyReminderAt is when the app next offers the optional key check.
	KeyReminderAt time.Time `json:"keyReminderAt"`
}

// BackupProgress describes a running transfer.
type BackupProgress struct {
	// Phase is "upload", "verify", "restore" or "archive".
	Phase      string `json:"phase"`
	Done       int    `json:"done"`
	Total      int    `json:"total"`
	DoneBytes  int64  `json:"doneBytes"`
	TotalBytes int64  `json:"totalBytes"`
}

// CloudBackupStatus is shown in the backup section of "Konto & Abo".
type CloudBackupStatus struct {
	// Available: the verified plan includes the backup feature.
	Available      bool   `json:"available"`
	KeyConfigured  bool   `json:"keyConfigured"`
	KeyPending     bool   `json:"keyPending"`
	Auto           bool   `json:"auto"`
	Running        bool   `json:"running"`
	Pending        bool   `json:"pending"`
	LastBackupAt   string `json:"lastBackupAt"`
	LastError      string `json:"lastError"`
	LastErrorAt    string `json:"lastErrorAt"`
	LastVerifiedAt string `json:"lastVerifiedAt"`
	// KeyCheckDue offers to enter the recovery key as a test. It is optional.
	KeyCheckDue bool            `json:"keyCheckDue"`
	Progress    *BackupProgress `json:"progress,omitempty"`
}

// BackupAccount is the server's view: storage use and what is allowed.
type BackupAccount struct {
	Access        string `json:"access"`
	ReadableUntil string `json:"readableUntil"`
	DeletionAt    string `json:"deletionAt"`
	UsedBytes     int64  `json:"usedBytes"`
	QuotaBytes    int64  `json:"quotaBytes"`
	Versions      int    `json:"versions"`
}

// RestorePreview shows a backup and what restoring it would remove.
type RestorePreview struct {
	Backup cloudbackup.Preview `json:"backup"`
	Impact store.RestoreImpact `json:"impact"`
}

// VerifyResult is the outcome of checking a stored backup.
type VerifyResult struct {
	OK        bool   `json:"ok"`
	Summary   string `json:"summary"`
	Documents int    `json:"documents"`
	Bytes     int64  `json:"bytes"`
}

// ArchiveResult describes a written backup file.
type ArchiveResult struct {
	Path      string `json:"path"`
	Documents int    `json:"documents"`
	Bytes     int64  `json:"bytes"`
}

// RestoreResult names the local copy of the data that the restore replaced.
type RestoreResult struct {
	SafetyCopy string `json:"safetyCopy"`
}

type BackupService struct {
	source  backupSource
	remote  backupRemote
	vault   account.Vault
	dataDir string
	now     func() time.Time
	check   time.Duration

	// runMu serializes uploads and restores.
	runMu sync.Mutex
	mu    sync.Mutex
	state backupState
	// pendingKey is a new key shown to the customer but not yet confirmed.
	pendingKey *cloudbackup.Key
	pending    bool
	running    bool
	progress   *BackupProgress
	lastEmit   time.Time
	preview    *restorePreview
	stop       chan struct{}
	stopOnce   sync.Once
	loopDone   chan struct{}
	onChange   func(CloudBackupStatus)
}

type restorePreview struct {
	id       string
	env      cloudbackup.Envelope
	document []byte
}

func NewBackupService(dataDir string, source backupSource, remote backupRemote, vault account.Vault) *BackupService {
	s := &BackupService{source: source, remote: remote, vault: vault, dataDir: dataDir, now: time.Now, check: backupCheckInterval, stop: make(chan struct{})}
	s.state.Auto = true
	if raw, err := os.ReadFile(s.statePath()); err == nil {
		_ = json.Unmarshal(raw, &s.state)
	}
	return s
}

func (s *BackupService) statePath() string { return filepath.Join(s.dataDir, "cloud-backup.json") }

func (s *BackupService) saveStateLocked() {
	raw, err := json.MarshalIndent(s.state, "", "  ")
	if err != nil {
		return
	}
	tmp := s.statePath() + ".tmp"
	if os.WriteFile(tmp, raw, 0o600) == nil {
		_ = os.Rename(tmp, s.statePath())
	}
}

//wails:ignore
func (s *BackupService) ServiceStartup(ctx context.Context, _ application.ServiceOptions) error {
	s.onChange = func(st CloudBackupStatus) {
		if app := application.Get(); app != nil {
			app.Event.Emit("backup:status", st)
		}
	}
	s.startLoop()
	return nil
}

func (s *BackupService) startLoop() {
	s.loopDone = make(chan struct{})
	go func() {
		defer close(s.loopDone)
		ticker := time.NewTicker(s.check)
		defer ticker.Stop()
		for {
			select {
			case <-s.stop:
				return
			case <-ticker.C:
				s.autoBackup(false)
			}
		}
	}()
}

// ServiceShutdown uploads pending changes once more, bounded by a timeout.
//
//wails:ignore
func (s *BackupService) ServiceShutdown() error {
	s.stopOnce.Do(func() { close(s.stop) })
	if s.loopDone != nil {
		<-s.loopDone
	}
	s.autoBackup(true)
	return nil
}

func (s *BackupService) key() (cloudbackup.Key, bool) {
	raw, err := s.vault.Get(backupKeyName)
	if err != nil || raw == "" {
		return cloudbackup.Key{}, false
	}
	k, err := cloudbackup.ParseKey(raw)
	return k, err == nil
}

func (s *BackupService) BackupStatus() CloudBackupStatus {
	_, hasKey := s.key()
	s.mu.Lock()
	defer s.mu.Unlock()
	st := CloudBackupStatus{Available: s.remote.HasFeature(entitlement.FeatureBackup), KeyConfigured: hasKey, KeyPending: s.pendingKey != nil,
		KeyCheckDue: hasKey && !s.state.KeyReminderAt.IsZero() && !s.now().Before(s.state.KeyReminderAt),
		Auto:        s.state.Auto, Running: s.running, Pending: s.pending, LastError: s.state.LastError, Progress: s.progress,
		LastBackupAt: formatStamp(s.state.LastBackupAt), LastErrorAt: formatStamp(s.state.LastErrorAt), LastVerifiedAt: formatStamp(s.state.LastVerifiedAt)}
	return st
}

func formatStamp(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format(time.RFC3339)
}

// BackupAccount asks the server; it needs a connection.
func (s *BackupService) BackupAccount() (BackupAccount, error) {
	ctx, cancel := context.WithTimeout(context.Background(), backupCallTimeout)
	defer cancel()
	st, err := s.remote.BackupState(ctx)
	if err != nil {
		return BackupAccount{}, err
	}
	out := BackupAccount{Access: st.Access, UsedBytes: st.UsedBytes, QuotaBytes: st.QuotaBytes, Versions: st.Versions}
	if st.ReadableUntil != nil {
		out.ReadableUntil = formatStamp(*st.ReadableUntil)
	}
	if st.DeletionAt != nil {
		out.DeletionAt = formatStamp(*st.DeletionAt)
	}
	return out, nil
}

func (s *BackupService) changed() CloudBackupStatus {
	st := s.BackupStatus()
	if s.onChange != nil {
		s.onChange(st)
	}
	return st
}

func (s *BackupService) requireAvailable() error {
	if !s.remote.HasFeature(entitlement.FeatureBackup) {
		return errors.New("Die verschlüsselte Sicherung gehört zum Backup-Zusatz von papierleicht Plus. Melde dich dafür unter „Konto & Abo“ an.")
	}
	return nil
}

// CreateBackupKey creates a recovery key and returns it once for display.
// It is stored only after ConfirmBackupKey.
func (s *BackupService) CreateBackupKey() (string, error) {
	if err := s.requireAvailable(); err != nil {
		return "", err
	}
	if _, ok := s.key(); ok {
		return "", errors.New("Auf diesem Gerät ist bereits ein Wiederherstellungsschlüssel gespeichert.")
	}
	k, err := cloudbackup.NewKey()
	if err != nil {
		return "", err
	}
	s.mu.Lock()
	s.pendingKey = &k
	s.mu.Unlock()
	s.changed()
	return k.String(), nil
}

// ConfirmBackupKey stores the new key after the customer typed it back.
func (s *BackupService) ConfirmBackupKey(entered string) (CloudBackupStatus, error) {
	k, err := cloudbackup.ParseKey(entered)
	s.mu.Lock()
	pending := s.pendingKey
	s.mu.Unlock()
	if pending == nil {
		return s.BackupStatus(), errors.New("Erstelle zuerst einen Wiederherstellungsschlüssel.")
	}
	if err != nil || !k.Equal(*pending) {
		return s.BackupStatus(), errors.New("Der eingegebene Schlüssel passt nicht zum angezeigten. Schreib ihn bitte genau ab.")
	}
	if err := s.storeKey(k); err != nil {
		return s.BackupStatus(), err
	}
	return s.changed(), nil
}

// ImportBackupKey stores an existing key, e.g. on a new device.
func (s *BackupService) ImportBackupKey(entered string) (CloudBackupStatus, error) {
	k, err := cloudbackup.ParseKey(entered)
	if err != nil {
		return s.BackupStatus(), err
	}
	if err := s.storeKey(k); err != nil {
		return s.BackupStatus(), err
	}
	return s.changed(), nil
}

func (s *BackupService) storeKey(k cloudbackup.Key) error {
	if err := s.vault.Set(backupKeyName, k.String()); err != nil {
		return errors.New("Der Schlüssel konnte nicht im Schlüsselbund gespeichert werden. Ohne Schlüsselbund ist keine automatische Sicherung möglich.")
	}
	s.mu.Lock()
	s.pendingKey = nil
	// The next upload starts a new series with this key.
	s.state.LastFingerprint = ""
	s.state.KeyReminderAt = s.now().Add(keyCheckInterval)
	s.saveStateLocked()
	s.mu.Unlock()
	return nil
}

// CheckBackupKey compares the customer's noted key with this device's, so a
// lost note shows up while the key can still be read here.
func (s *BackupService) CheckBackupKey(entered string) (CloudBackupStatus, error) {
	stored, ok := s.key()
	if !ok {
		return s.BackupStatus(), errors.New("Auf diesem Gerät ist kein Wiederherstellungsschlüssel gespeichert.")
	}
	k, err := cloudbackup.ParseKey(entered)
	if err != nil || !k.Equal(stored) {
		return s.BackupStatus(), errors.New("Dieser Schlüssel passt nicht. Unter „Wiederherstellungsschlüssel“ kannst du ihn anzeigen und neu notieren, solange dieses Gerät läuft.")
	}
	s.remindKeyCheck(keyCheckInterval)
	return s.changed(), nil
}

// PostponeKeyCheck asks again later; the check stays optional.
func (s *BackupService) PostponeKeyCheck() CloudBackupStatus {
	s.remindKeyCheck(keyCheckPostpone)
	return s.changed()
}

func (s *BackupService) remindKeyCheck(after time.Duration) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.state.KeyReminderAt = s.now().Add(after)
	s.saveStateLocked()
}

// CancelBackupKey discards an unconfirmed key.
func (s *BackupService) CancelBackupKey() CloudBackupStatus {
	s.mu.Lock()
	s.pendingKey = nil
	s.mu.Unlock()
	return s.changed()
}

// ShowBackupKey returns the key of this device, so a lost printout can be
// replaced while the device still works.
func (s *BackupService) ShowBackupKey() (string, error) {
	k, ok := s.key()
	if !ok {
		return "", errors.New("Auf diesem Gerät ist kein Wiederherstellungsschlüssel gespeichert.")
	}
	return k.String(), nil
}

// RemoveBackupKey deletes the key from this device's keychain. Existing
// backups stay on the server but need the key to be restored.
func (s *BackupService) RemoveBackupKey() (CloudBackupStatus, error) {
	if err := s.vault.Delete(backupKeyName); err != nil {
		return s.BackupStatus(), errors.New("Der Schlüssel konnte nicht aus dem Schlüsselbund entfernt werden.")
	}
	return s.changed(), nil
}

func (s *BackupService) SetAutoBackup(on bool) CloudBackupStatus {
	s.mu.Lock()
	s.state.Auto = on
	s.saveStateLocked()
	s.mu.Unlock()
	return s.changed()
}

// exportedAtLine is the volatile top-level timestamp of ExportBackup.
var exportedAtLine = regexp.MustCompile(`(?m)^  "exportedAt": "[^"]*",?\n`)

// fingerprint identifies the backup content apart from its export time.
func fingerprint(export string) string {
	sum := sha256.Sum256([]byte(exportedAtLine.ReplaceAllString(export, "")))
	return hex.EncodeToString(sum[:])
}

// runContext ends when the app quits, so a long transfer never holds it up.
// What is already stored stays and the next run continues from there.
func (s *BackupService) runContext() (context.Context, context.CancelFunc) {
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		select {
		case <-s.stop:
			cancel()
		case <-ctx.Done():
		}
	}()
	return ctx, cancel
}

// BackupNow encrypts and uploads what changed.
func (s *BackupService) BackupNow() (CloudBackupStatus, error) {
	if err := s.requireAvailable(); err != nil {
		return s.BackupStatus(), err
	}
	ctx, cancel := s.runContext()
	defer cancel()
	s.runMu.Lock()
	defer s.runMu.Unlock()
	snapshot, err := s.source.ExportSnapshot()
	if err != nil {
		return s.BackupStatus(), err
	}
	err = s.upload(ctx, snapshot)
	return s.changed(), err
}

// upload requires runMu.
func (s *BackupService) upload(ctx context.Context, snapshot *store.Snapshot) error {
	k, ok := s.key()
	if !ok {
		return errors.New("Richte zuerst einen Wiederherstellungsschlüssel ein.")
	}
	s.setRunning(true)
	defer s.setRunning(false)
	err := s.transfer(ctx, k, snapshot)
	s.mu.Lock()
	defer s.mu.Unlock()
	if err != nil {
		s.state.LastError, s.state.LastErrorAt = err.Error(), s.now()
	} else {
		s.state.LastBackupAt, s.state.LastFingerprint = s.now(), fingerprint(string(snapshot.Document))
		s.state.LastError, s.state.LastErrorAt = "", time.Time{}
		s.pending = false
	}
	s.saveStateLocked()
	return err
}

// transfer stores what the server lacks and then the index of this state. A
// state exists only once it is complete; if the transfer stops, the objects
// that arrived stay and the next run sends the rest.
func (s *BackupService) transfer(ctx context.Context, k cloudbackup.Key, snapshot *store.Snapshot) error {
	c := snapshot.Counts
	env := cloudbackup.Envelope{CreatedAt: s.now(), DeviceName: s.remote.DeviceName(), AppVersion: currentVersion,
		Counts: cloudbackup.Counts{Customers: c.Customers, Products: c.Products, Invoices: c.Invoices, Received: c.Received, Expenses: c.Expenses}}
	read := make(map[string]func() ([]byte, error), len(snapshot.Blobs))
	for _, part := range k.Split(snapshot.Document) {
		ref := cloudbackup.NewRef(part)
		env.Document = append(env.Document, ref)
		read[ref.Sum] = func() ([]byte, error) { return part, nil }
	}
	for _, blob := range snapshot.Blobs {
		env.Blobs = append(env.Blobs, cloudbackup.Ref{Sum: blob.Sum, Size: blob.Size})
		read[blob.Sum] = func() ([]byte, error) { return s.source.ReadBlob(blob) }
	}
	refs := env.Refs()
	ids := make([]string, len(refs))
	byID := make(map[string]cloudbackup.Ref, len(refs))
	var logical int64
	for i, ref := range refs {
		ids[i] = k.ObjectID(ref.Sum)
		byID[ids[i]] = ref
		logical += ref.Size
	}
	missing, err := s.remote.MissingObjects(ctx, ids)
	if err != nil {
		return err
	}
	var total, done int64
	for _, id := range missing {
		total += byID[id].Size
	}
	for i, id := range missing {
		ref := byID[id]
		data, err := read[ref.Sum]()
		if err != nil {
			return err
		}
		sealed, err := cloudbackup.Seal(k, data)
		if err != nil {
			return err
		}
		call, cancel := context.WithTimeout(ctx, objectTimeout)
		err = s.remote.UploadObject(call, id, sealed)
		cancel()
		if err != nil {
			return err
		}
		done += ref.Size
		s.setProgress(&BackupProgress{Phase: "upload", Done: i + 1, Total: len(missing), DoneBytes: done, TotalBytes: total})
	}
	manifest, err := cloudbackup.Pack(k, env)
	if err != nil {
		return err
	}
	_, err = s.remote.UploadBackup(ctx, cloudbackup.EnvelopeVersion, manifest, ids, logical)
	return err
}

func (s *BackupService) setRunning(on bool) {
	s.mu.Lock()
	s.running = on
	if !on {
		s.progress = nil
	}
	s.mu.Unlock()
	s.changed()
}

// setProgress reports at most four times a second.
func (s *BackupService) setProgress(p *BackupProgress) {
	s.mu.Lock()
	s.progress = p
	emit := p.Done == p.Total || s.now().Sub(s.lastEmit) >= 250*time.Millisecond
	if emit {
		s.lastEmit = s.now()
	}
	s.mu.Unlock()
	if emit {
		s.changed()
	}
}

// autoBackup uploads changed data at most once per autoBackupInterval; at
// shutdown it ignores the interval so no change stays local only. Documents
// are not read for this: only what is new leaves the device.
func (s *BackupService) autoBackup(shutdown bool) {
	s.mu.Lock()
	auto := s.state.Auto
	s.mu.Unlock()
	if _, ok := s.key(); !auto || !ok || s.requireAvailable() != nil {
		return
	}
	// The read-only view of a tax audit allows no writes, not even the index.
	if v, ok := s.source.(interface{ ReadOnly() bool }); ok && v.ReadOnly() {
		return
	}
	s.runMu.Lock()
	defer s.runMu.Unlock()
	snapshot, err := s.source.ExportSnapshot()
	if err != nil {
		return
	}
	s.mu.Lock()
	// A new device with the key but no data yet has nothing worth a version.
	empty := s.state.LastBackupAt.IsZero() && snapshot.Counts == (store.SnapshotCounts{})
	changed := !empty && fingerprint(string(snapshot.Document)) != s.state.LastFingerprint
	due := shutdown || s.now().Sub(s.state.LastBackupAt) >= autoBackupInterval
	s.pending = changed
	s.mu.Unlock()
	if !changed || !due {
		s.changed()
		return
	}
	var ctx context.Context
	var cancel context.CancelFunc
	if shutdown {
		ctx, cancel = context.WithTimeout(context.Background(), shutdownTimeout)
	} else {
		ctx, cancel = s.runContext()
	}
	defer cancel()
	_ = s.upload(ctx, snapshot)
}

// BackupVersions lists the stored states. The server decides: after the plan
// ended they stay readable for a while.
func (s *BackupService) BackupVersions() ([]account.BackupVersion, error) {
	ctx, cancel := context.WithTimeout(context.Background(), backupCallTimeout)
	defer cancel()
	return s.remote.ListBackups(ctx)
}

// BackupLog is the protocol of restores, checks and backup files.
func (s *BackupService) BackupLog() ([]store.BackupEvent, error) { return s.source.BackupEvents() }

func (s *BackupService) requireKey() (cloudbackup.Key, error) {
	k, ok := s.key()
	if !ok {
		return k, errors.New("Gib zuerst deinen Wiederherstellungsschlüssel ein.")
	}
	return k, nil
}

// fetchObject downloads, decrypts and checks one object. sealed is what the
// server stored, for keeping it encrypted on disk until it is needed.
func (s *BackupService) fetchObject(ctx context.Context, k cloudbackup.Key, ref cloudbackup.Ref) (sealed, plain []byte, err error) {
	call, cancel := context.WithTimeout(ctx, objectTimeout)
	defer cancel()
	if sealed, err = s.remote.DownloadObject(call, k.ObjectID(ref.Sum)); err != nil {
		return nil, nil, err
	}
	if plain, err = cloudbackup.Open(k, sealed); err != nil {
		return nil, nil, err
	}
	return sealed, plain, ref.Check(plain)
}

// openVersion downloads the index and the backup document of a state.
func (s *BackupService) openVersion(ctx context.Context, k cloudbackup.Key, id string) (cloudbackup.Envelope, []byte, error) {
	blob, err := s.remote.DownloadBackup(ctx, id)
	if err != nil {
		return cloudbackup.Envelope{}, nil, err
	}
	env, err := cloudbackup.Unpack(k, blob)
	if err != nil {
		return env, nil, err
	}
	var document []byte
	for _, ref := range env.Document {
		_, part, err := s.fetchObject(ctx, k, ref)
		if err != nil {
			return env, nil, err
		}
		document = append(document, part...)
	}
	return env, document, nil
}

// PreviewRestore downloads a state's index and data without changing anything.
func (s *BackupService) PreviewRestore(id string) (RestorePreview, error) {
	k, err := s.requireKey()
	if err != nil {
		return RestorePreview{}, err
	}
	ctx, cancel := s.runContext()
	defer cancel()
	env, document, err := s.openVersion(ctx, k, id)
	if err != nil {
		return RestorePreview{}, err
	}
	impact, err := s.source.RestoreImpact(document)
	if err != nil {
		return RestorePreview{}, err
	}
	s.mu.Lock()
	s.preview = &restorePreview{id: id, env: env, document: document}
	s.mu.Unlock()
	return RestorePreview{Backup: env.Preview(), Impact: impact}, nil
}

// spoolDocuments downloads and checks every document of a state before
// anything local changes. They stay encrypted on disk until the restore.
func (s *BackupService) spoolDocuments(ctx context.Context, k cloudbackup.Key, env cloudbackup.Envelope, dir string) error {
	preview := env.Preview()
	var done int64
	for i, ref := range env.Blobs {
		sealed, _, err := s.fetchObject(ctx, k, ref)
		if err != nil {
			return err
		}
		if err = os.WriteFile(filepath.Join(dir, ref.Sum), sealed, 0o600); err != nil {
			return err
		}
		done += ref.Size
		s.setProgress(&BackupProgress{Phase: "restore", Done: i + 1, Total: len(env.Blobs), DoneBytes: done, TotalBytes: preview.Bytes})
	}
	return nil
}

func (s *BackupService) backupDir() (string, error) {
	dir := filepath.Join(s.dataDir, "Sicherungen")
	return dir, os.MkdirAll(dir, 0o700)
}

// safetyArchive keeps the data a restore is about to replace.
func (s *BackupService) safetyArchive() (string, error) {
	failed := errors.New("Die lokale Sicherung vor der Wiederherstellung konnte nicht angelegt werden. Deine Daten bleiben unverändert.")
	dir, err := s.backupDir()
	if err != nil {
		return "", failed
	}
	path := filepath.Join(dir, "vor-wiederherstellung-"+s.now().Format("20060102-150405")+".zip")
	if _, err := s.writeCurrentArchive(path, "restore"); err != nil {
		return "", failed
	}
	return path, nil
}

func (s *BackupService) writeCurrentArchive(path, phase string) (ArchiveResult, error) {
	snapshot, err := s.source.ExportSnapshot()
	if err != nil {
		return ArchiveResult{}, err
	}
	refs := make([]cloudbackup.Ref, len(snapshot.Blobs))
	sources := make(map[string]store.SnapshotBlob, len(snapshot.Blobs))
	var total int64
	for i, b := range snapshot.Blobs {
		refs[i] = cloudbackup.Ref{Sum: b.Sum, Size: b.Size}
		sources[b.Sum] = b
		total += b.Size
	}
	read := func(ref cloudbackup.Ref) ([]byte, error) { return s.source.ReadBlob(sources[ref.Sum]) }
	err = writeArchive(path, snapshot.Document, refs, read, func(done int, bytes int64) {
		s.setProgress(&BackupProgress{Phase: phase, Done: done, Total: len(refs), DoneBytes: bytes, TotalBytes: total})
	})
	return ArchiveResult{Path: path, Documents: len(refs), Bytes: total}, err
}

// RestoreVersion replaces local data with the previewed state. All documents
// are downloaded and checked first, and the data it replaces is kept as a
// local backup file. The restore enters the protocol, which it never touches.
func (s *BackupService) RestoreVersion(id string) (RestoreResult, error) {
	s.mu.Lock()
	p := s.preview
	s.mu.Unlock()
	if p == nil || p.id != id {
		return RestoreResult{}, errors.New("Öffne die Sicherung zuerst in der Vorschau.")
	}
	k, err := s.requireKey()
	if err != nil {
		return RestoreResult{}, err
	}
	ctx, cancel := s.runContext()
	defer cancel()
	s.runMu.Lock()
	defer s.runMu.Unlock()
	s.setRunning(true)
	defer s.setRunning(false)
	dir, err := s.backupDir()
	if err != nil {
		return RestoreResult{}, err
	}
	spool, err := os.MkdirTemp(dir, "wiederherstellung-")
	if err != nil {
		return RestoreResult{}, err
	}
	defer os.RemoveAll(spool)
	if err = s.spoolDocuments(ctx, k, p.env, spool); err != nil {
		return RestoreResult{}, err
	}
	safety, err := s.safetyArchive()
	if err != nil {
		return RestoreResult{}, err
	}
	open := func(sum string) ([]byte, error) {
		sealed, err := os.ReadFile(filepath.Join(spool, sum))
		if err != nil {
			return nil, err
		}
		return cloudbackup.Open(k, sealed)
	}
	event := store.BackupEvent{Kind: store.BackupEventRestore, OK: true, BackupAt: formatStamp(p.env.CreatedAt), AppVersion: currentVersion,
		Summary: fmt.Sprintf("Sicherung vom %s (%s) wiederhergestellt. Der vorherige Stand liegt in %s.", germanTime(p.env.CreatedAt), p.env.DeviceName, filepath.Base(safety))}
	if err = s.source.RestoreSnapshot(p.document, open, event); err != nil {
		return RestoreResult{SafetyCopy: safety}, err
	}
	s.mu.Lock()
	s.preview = nil
	s.pending = true
	s.mu.Unlock()
	return RestoreResult{SafetyCopy: safety}, nil
}

func germanTime(t time.Time) string { return t.Local().Format("02.01.2006, 15:04") + " Uhr" }

func germanSize(n int64) string {
	if n >= 1<<30 {
		return strings.Replace(strconv.FormatFloat(float64(n)/(1<<30), 'f', 1, 64), ".", ",", 1) + " GB"
	}
	return strconv.FormatInt(n>>20, 10) + " MB"
}

// VerifyVersion proves that a stored state can be restored: every document is
// downloaded, decrypted with the key and compared with its hash. Nothing local
// changes; the result goes into the protocol.
func (s *BackupService) VerifyVersion(id string) (VerifyResult, error) {
	k, err := s.requireKey()
	if err != nil {
		return VerifyResult{}, err
	}
	ctx, cancel := s.runContext()
	defer cancel()
	s.runMu.Lock()
	defer s.runMu.Unlock()
	s.setRunning(true)
	defer s.setRunning(false)
	env, document, err := s.openVersion(ctx, k, id)
	if errors.Is(err, cloudbackup.ErrWrongKey) {
		return VerifyResult{}, err
	}
	bad := err
	preview := env.Preview()
	result := VerifyResult{Documents: preview.Documents, Bytes: preview.Bytes}
	if bad == nil && !json.Valid(document) {
		bad = cloudbackup.ErrTampered
	}
	if bad == nil {
		var done int64
		for i, ref := range env.Blobs {
			if _, _, bad = s.fetchObject(ctx, k, ref); bad != nil {
				break
			}
			done += ref.Size
			s.setProgress(&BackupProgress{Phase: "verify", Done: i + 1, Total: len(env.Blobs), DoneBytes: done, TotalBytes: preview.Bytes})
		}
	}
	// A lost connection says nothing about the backup.
	if bad != nil && (errors.Is(bad, account.ErrOffline) || ctx.Err() != nil) {
		return result, bad
	}
	event := store.BackupEvent{Kind: store.BackupEventVerify, OK: bad == nil, BackupAt: formatStamp(env.CreatedAt), AppVersion: currentVersion}
	if bad == nil {
		event.Summary = fmt.Sprintf("Sicherung vom %s geprüft: Alles in Ordnung, %d Dokumente (%s) lassen sich wiederherstellen.", germanTime(env.CreatedAt), preview.Documents, germanSize(preview.Bytes))
	} else {
		event.Summary = fmt.Sprintf("Prüfung der Sicherung vom %s fehlgeschlagen: %s", germanTime(env.CreatedAt), bad.Error())
	}
	if err := s.source.AddBackupEvent(event); err != nil {
		return result, err
	}
	s.mu.Lock()
	if bad == nil {
		s.state.LastVerifiedAt = s.now()
		s.saveStateLocked()
	}
	s.pending = true
	s.mu.Unlock()
	result.OK, result.Summary = bad == nil, event.Summary
	return result, nil
}

// SaveVersionArchive writes a stored state to a backup file, without touching
// local data. It is the way out: after the plan ended, or for the tax office.
func (s *BackupService) SaveVersionArchive(id, path string) (ArchiveResult, error) {
	k, err := s.requireKey()
	if err != nil {
		return ArchiveResult{}, err
	}
	ctx, cancel := s.runContext()
	defer cancel()
	s.runMu.Lock()
	defer s.runMu.Unlock()
	s.setRunning(true)
	defer s.setRunning(false)
	env, document, err := s.openVersion(ctx, k, id)
	if err != nil {
		return ArchiveResult{}, err
	}
	preview := env.Preview()
	read := func(ref cloudbackup.Ref) ([]byte, error) {
		_, plain, err := s.fetchObject(ctx, k, ref)
		return plain, err
	}
	err = writeArchive(path, document, env.Blobs, read, func(done int, bytes int64) {
		s.setProgress(&BackupProgress{Phase: "archive", Done: done, Total: len(env.Blobs), DoneBytes: bytes, TotalBytes: preview.Bytes})
	})
	if err != nil {
		return ArchiveResult{}, err
	}
	s.logExport(fmt.Sprintf("Sicherung vom %s als Datei %s gespeichert (%d Dokumente, %s).", germanTime(env.CreatedAt), filepath.Base(path), preview.Documents, germanSize(preview.Bytes)), env.CreatedAt)
	return ArchiveResult{Path: path, Documents: preview.Documents, Bytes: preview.Bytes}, nil
}

func (s *BackupService) logExport(summary string, at time.Time) {
	_ = s.source.AddBackupEvent(store.BackupEvent{Kind: store.BackupEventExport, OK: true, Summary: summary, BackupAt: formatStamp(at), AppVersion: currentVersion})
}

// ExportArchive writes the current data to a backup file. It needs no plan.
func (s *BackupService) ExportArchive(path string) (ArchiveResult, error) {
	s.runMu.Lock()
	defer s.runMu.Unlock()
	s.setRunning(true)
	defer s.setRunning(false)
	result, err := s.writeCurrentArchive(path, "archive")
	if err != nil {
		return result, err
	}
	s.logExport(fmt.Sprintf("Sicherungsdatei %s erstellt (%d Dokumente, %s).", filepath.Base(path), result.Documents, germanSize(result.Bytes)), s.now())
	return result, nil
}

// InspectArchive describes a backup file without changing anything.
func (s *BackupService) InspectArchive(path string) (RestorePreview, error) {
	var document []byte
	var preview cloudbackup.Preview
	if strings.EqualFold(filepath.Ext(path), ".json") {
		raw, err := os.ReadFile(path)
		if err != nil {
			return RestorePreview{}, errors.New("Die Sicherungsdatei ließ sich nicht lesen.")
		}
		document = raw
	} else {
		a, err := openArchive(path)
		if err != nil {
			return RestorePreview{}, err
		}
		defer a.Close()
		document = a.document
		for _, f := range a.files {
			preview.Documents++
			preview.Bytes += int64(f.UncompressedSize64)
		}
	}
	var doc struct {
		ExportedAt string `json:"exportedAt"`
		Customers  []any  `json:"customers"`
		Products   []any  `json:"products"`
		Invoices   []any  `json:"invoices"`
		Received   []any  `json:"received"`
		Settings   []struct {
			Key string `json:"key"`
		} `json:"settings"`
	}
	if json.Unmarshal(document, &doc) != nil {
		return RestorePreview{}, errors.New("Die Datei ist keine Sicherung von E-Rechnung-Studio.")
	}
	preview.CreatedAt, preview.Customers, preview.Products = doc.ExportedAt, len(doc.Customers), len(doc.Products)
	preview.Invoices, preview.Received = len(doc.Invoices), len(doc.Received)
	for _, v := range doc.Settings {
		if strings.HasPrefix(v.Key, "expense:") {
			preview.Expenses++
		}
	}
	impact, err := s.source.RestoreImpact(document)
	if err != nil {
		return RestorePreview{}, err
	}
	return RestorePreview{Backup: preview, Impact: impact}, nil
}

// ImportArchive replaces local data with a backup file after checking every
// document and keeping the current data as a backup file.
func (s *BackupService) ImportArchive(path string) (RestoreResult, error) {
	s.runMu.Lock()
	defer s.runMu.Unlock()
	s.setRunning(true)
	defer s.setRunning(false)
	if strings.EqualFold(filepath.Ext(path), ".json") {
		raw, err := os.ReadFile(path)
		if err != nil {
			return RestoreResult{}, errors.New("Die Sicherungsdatei ließ sich nicht lesen.")
		}
		safety, err := s.safetyArchive()
		if err != nil {
			return RestoreResult{}, err
		}
		return RestoreResult{SafetyCopy: safety}, s.source.RestoreBackup(string(raw))
	}
	a, err := openArchive(path)
	if err != nil {
		return RestoreResult{}, err
	}
	defer a.Close()
	var total int64
	for _, f := range a.files {
		total += int64(f.UncompressedSize64)
	}
	if _, err = a.verify(func(done int, bytes int64) {
		s.setProgress(&BackupProgress{Phase: "restore", Done: done, Total: len(a.files), DoneBytes: bytes, TotalBytes: total})
	}); err != nil {
		return RestoreResult{}, err
	}
	safety, err := s.safetyArchive()
	if err != nil {
		return RestoreResult{}, err
	}
	sum := sha256.Sum256(a.document)
	event := store.BackupEvent{Kind: store.BackupEventRestore, OK: true, AppVersion: currentVersion,
		Summary: fmt.Sprintf("Sicherungsdatei %s wiederhergestellt (Prüfsumme der Daten %s). Der vorherige Stand liegt in %s.", filepath.Base(path), hex.EncodeToString(sum[:8]), filepath.Base(safety))}
	if err = s.source.RestoreSnapshot(a.document, a.open, event); err != nil {
		return RestoreResult{SafetyCopy: safety}, err
	}
	s.mu.Lock()
	s.pending = true
	s.mu.Unlock()
	return RestoreResult{SafetyCopy: safety}, nil
}

// DeleteAllBackups marks everything on the server for deletion. The waiting
// period and a mail to the account protect against a stolen session.
func (s *BackupService) DeleteAllBackups() (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), backupCallTimeout)
	defer cancel()
	at, err := s.remote.RequestBackupDeletion(ctx)
	if err != nil {
		return "", err
	}
	return formatStamp(at), nil
}

func (s *BackupService) CancelBackupDeletion() error {
	ctx, cancel := context.WithTimeout(context.Background(), backupCallTimeout)
	defer cancel()
	return s.remote.CancelBackupDeletion(ctx)
}
