package app

import (
	"context"
	"errors"
	"io/fs"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/updater"
	"papierleicht.de/e-rechnung-studio/internal/store"
)

// currentVersion is the product version from version.txt (managed by Release
// Please). Run sets it; tests see "dev".
var currentVersion = "dev"

// updateAPIURL and updateFilesURL point a test build at a local copy of the
// release API and its downloads, set with -ldflags "-X …"; empty is GitHub.
var updateAPIURL, updateFilesURL string

type UpdateStatus struct {
	CurrentVersion   string `json:"currentVersion"`
	AvailableVersion string `json:"availableVersion,omitempty"`
	UpdateAvailable  bool   `json:"updateAvailable"`
	// Notes are the German release notes of the available version (Markdown),
	// the text of its GitHub release.
	Notes string `json:"notes,omitempty"`
	// Skipped says the customer chose to skip the available version: the app
	// does not point to it again, the settings still offer it.
	Skipped bool `json:"skipped,omitempty"`
}

// UpdateProgress is the state of a running update, asked for by the update
// window while it downloads, verifies and installs.
type UpdateProgress struct {
	// Phase is "idle", "downloading", "verifying", "installing", "ready" or "failed".
	Phase   string `json:"phase"`
	Written int64  `json:"written"`
	Total   int64  `json:"total"`
	Message string `json:"message,omitempty"`
}

// VersionNotes are the German release notes of one version, as Markdown.
type VersionNotes struct {
	Version string `json:"version"`
	Notes   string `json:"notes"`
}

// updaterAPI is the part of the Wails updater the service uses.
type updaterAPI interface {
	Check(ctx context.Context) (*updater.Release, error)
	DownloadAndInstall(ctx context.Context) error
	Restart(ctx context.Context) error
}

type UpdateService struct {
	app     *application.App
	updater updaterAPI
	store   *store.Store
	// notes holds the release notes the app ships with, <version>.md.
	notes fs.FS

	mu       sync.Mutex
	progress UpdateProgress
	release  string
}

func (s *UpdateService) Status() UpdateStatus {
	return UpdateStatus{CurrentVersion: currentVersion}
}

// Check looks for a newer release.
func (s *UpdateService) Check() (UpdateStatus, error) {
	status := s.Status()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	release, err := s.updater.Check(ctx)
	if err != nil {
		return status, err
	}
	if release != nil {
		status.AvailableVersion = release.Version
		status.UpdateAvailable = true
		status.Notes = strings.TrimSpace(release.Notes)
		if st, err := s.state(); err == nil && st.Skipped != "" && st.Skipped == release.Version {
			status.Skipped = true
		}
		s.mu.Lock()
		s.release = release.Version
		s.mu.Unlock()
	}
	return status, nil
}

func (s *UpdateService) state() (store.UpdateState, error) {
	if s.store == nil {
		return store.UpdateState{}, nil
	}
	return s.store.UpdateState()
}

func (s *UpdateService) saveState(f func(*store.UpdateState)) error {
	if s.store == nil {
		return nil
	}
	st, err := s.store.UpdateState()
	if err != nil {
		return err
	}
	f(&st)
	return s.store.SaveUpdateState(st)
}

func (s *UpdateService) setProgress(f func(*UpdateProgress)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	f(&s.progress)
}

// Progress returns the state of the running update.
func (s *UpdateService) Progress() UpdateProgress {
	s.mu.Lock()
	defer s.mu.Unlock()
	p := s.progress
	if p.Phase == "" {
		p.Phase = "idle"
	}
	return p
}

// onUpdaterEvent follows the updater's events for Progress.
func (s *UpdateService) onUpdaterEvent(name string, data any) {
	if list, ok := data.([]any); ok && len(list) > 0 {
		data = list[0]
	}
	s.setProgress(func(p *UpdateProgress) {
		switch name {
		case updater.EventDownloadProgress:
			if pr, ok := data.(updater.Progress); ok {
				p.Written, p.Total = pr.Written, pr.Total
			}
		case updater.EventVerifying:
			p.Phase = "verifying"
		case updater.EventInstalling:
			p.Phase = "installing"
		}
	})
}

// Install downloads the available version, checks its signature and prepares
// it; Restart then starts it. The call returns when the update is ready.
func (s *UpdateService) Install() error {
	s.setProgress(func(p *UpdateProgress) { *p = UpdateProgress{Phase: "downloading"} })
	ctx := context.Background()
	// Ask again first: the release, its signature and its download address
	// are then those of now, also when the dialog stood open for a while or
	// a first attempt failed.
	release, err := s.updater.Check(ctx)
	if err == nil && release == nil {
		err = updater.ErrNoPendingRelease
	}
	if err == nil {
		err = s.updater.DownloadAndInstall(ctx)
	}
	if err != nil {
		if s.app != nil {
			s.app.Logger.Error("update install", "error", err)
		}
		message := "Das Update ließ sich nicht installieren. Prüfe die Internetverbindung und versuche es noch einmal."
		if errors.Is(err, updater.ErrNoPendingRelease) {
			message = "Es gibt gerade kein Update zum Installieren."
		}
		s.setProgress(func(p *UpdateProgress) { p.Phase, p.Message = "failed", message })
		return errors.New(message)
	}
	s.setProgress(func(p *UpdateProgress) { p.Phase = "ready" })
	return nil
}

// Restart ends the app and starts the installed update.
func (s *UpdateService) Restart() error {
	if err := s.updater.Restart(context.Background()); err != nil {
		return errors.New("Die App ließ sich nicht neu starten. Beende sie und öffne sie wieder; das Update ist dann installiert.")
	}
	return nil
}

// Skip stops pointing to a version until a newer one comes.
func (s *UpdateService) Skip(version string) error {
	return s.saveState(func(st *store.UpdateState) { st.Skipped = version })
}

var versionFile = regexp.MustCompile(`^(\d+)\.(\d+)\.(\d+)\.md$`)

// versionKey makes versions comparable: 1.10.0 sorts after 1.9.2.
func versionKey(v string) [3]int {
	var k [3]int
	for i, part := range strings.SplitN(strings.TrimPrefix(v, "v"), ".", 3) {
		n, _ := strconv.Atoi(strings.TrimRightFunc(part, func(r rune) bool { return r < '0' || r > '9' }))
		k[i] = n
	}
	return k
}

func newer(a, b string) bool {
	ka, kb := versionKey(a), versionKey(b)
	for i := range ka {
		if ka[i] != kb[i] {
			return ka[i] > kb[i]
		}
	}
	return false
}

// ReleaseNotes lists the release notes the app ships with, newest first.
func (s *UpdateService) ReleaseNotes() ([]VersionNotes, error) {
	out := []VersionNotes{}
	if s.notes == nil {
		return out, nil
	}
	entries, err := fs.ReadDir(s.notes, ".")
	if err != nil {
		return out, err
	}
	for _, e := range entries {
		if !versionFile.MatchString(e.Name()) {
			continue
		}
		raw, err := fs.ReadFile(s.notes, e.Name())
		if err != nil {
			return out, err
		}
		out = append(out, VersionNotes{Version: strings.TrimSuffix(e.Name(), ".md"), Notes: strings.TrimSpace(string(raw))})
	}
	sort.Slice(out, func(i, j int) bool { return newer(out[i].Version, out[j].Version) })
	return out, nil
}

// WhatsNew returns the notes of the versions installed since the customer
// last saw them, newest first, or nothing. A new installation starts quietly;
// after an update from a version that did not remember this yet, the app
// shows the notes of the current version.
func (s *UpdateService) WhatsNew() ([]VersionNotes, error) {
	st, err := s.state()
	if err != nil || st.Seen == currentVersion || s.store == nil {
		return []VersionNotes{}, err
	}
	if st.Seen == "" {
		company, err := s.store.Company()
		if err != nil {
			return []VersionNotes{}, err
		}
		if strings.TrimSpace(company.Name) == "" {
			return []VersionNotes{}, s.MarkSeen()
		}
	}
	all, err := s.ReleaseNotes()
	if err != nil {
		return []VersionNotes{}, err
	}
	out := []VersionNotes{}
	for _, n := range all {
		switch {
		case newer(n.Version, currentVersion):
		case st.Seen == "" && n.Version != currentVersion:
		case st.Seen != "" && !newer(n.Version, st.Seen):
		default:
			out = append(out, n)
		}
	}
	if len(out) == 0 {
		return out, s.MarkSeen()
	}
	return out, nil
}

// MarkSeen remembers that the customer read the notes of this version.
func (s *UpdateService) MarkSeen() error {
	return s.saveState(func(st *store.UpdateState) { st.Seen = currentVersion })
}
