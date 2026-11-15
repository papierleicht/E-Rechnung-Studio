package app

import (
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// The updater replaces the app where it runs: the .app bundle on macOS, the
// .exe on Windows and the AppImage on Linux. That only works when the app may
// write to that folder and the folder still exists after the app quits.
// InstallLocation tells the frontend whether this is the case and where the
// app can move itself so that updates work.
type InstallLocation struct {
	// CanUpdate is false when the updater could not replace the app here.
	CanUpdate bool `json:"canUpdate"`
	// Problem names why: "translocated" (macOS runs a download from a hidden,
	// read-only copy), "diskImage" (started from a mounted disk image),
	// "temporary" (started from a temporary folder, e.g. inside a ZIP) or
	// "readOnly" (the folder is not writable).
	Problem string `json:"problem,omitempty"`
	// Current is the app the updater would replace.
	Current string `json:"current,omitempty"`
	// Target is where MoveApp puts the app; empty when it cannot move itself.
	Target string `json:"target,omitempty"`
	// TargetExists is true when MoveApp would replace an app already there.
	TargetExists bool `json:"targetExists"`
}

// Location reports whether updates can replace the app where it runs.
func (s *UpdateService) Location() InstallLocation {
	current, ok := runningApp()
	if !ok || currentVersion == "dev" {
		// Development builds and unpackaged binaries are not updated.
		return InstallLocation{CanUpdate: true}
	}
	loc := inspectLocation(current, os.TempDir(), dirWritable)
	if loc.CanUpdate {
		return loc
	}
	if target, ok := recommendedTarget(current); ok && !samePath(target, current) {
		loc.Target = target
		if _, err := os.Lstat(target); err == nil {
			loc.TargetExists = true
		}
	}
	return loc
}

// MoveApp copies the app to the recommended place, starts it from there once
// this process has ended and quits. Data stay where they are: they live in the
// user's configuration folder, not next to the app.
func (s *UpdateService) MoveApp() error {
	loc := s.Location()
	if loc.CanUpdate {
		return errors.New("Die App kann sich an ihrem jetzigen Ort schon aktualisieren.")
	}
	if loc.Target == "" {
		return errors.New("Die App kann sich hier nicht selbst verschieben. Verschiebe sie bitte von Hand.")
	}
	if err := installApp(loc.Current, loc.Target); err != nil {
		log.Printf("move app: %v", err)
		return fmt.Errorf("Die App ließ sich nicht nach %s kopieren. Verschiebe sie bitte von Hand.", filepath.Dir(loc.Target))
	}
	if err := relaunchAfterExit(loc.Target, os.Getpid()); err != nil {
		log.Printf("move app: relaunch: %v", err)
		return fmt.Errorf("Die App liegt jetzt in %s, ließ sich aber nicht neu starten. Beende sie und öffne sie dort.", filepath.Dir(loc.Target))
	}
	if s.app != nil {
		go func() {
			// Let the frontend receive the answer before the window closes.
			time.Sleep(300 * time.Millisecond)
			s.app.Quit()
		}()
	}
	return nil
}

// inspectLocation decides whether the updater can replace current. tempDir is
// the system's temporary folder; writable tests a folder.
func inspectLocation(current, tempDir string, writable func(string) bool) InstallLocation {
	loc := InstallLocation{Current: current}
	parent := filepath.Dir(current)
	slashed := filepath.ToSlash(current)
	switch {
	case strings.Contains(slashed, "/AppTranslocation/"):
		loc.Problem = "translocated"
	case strings.HasPrefix(slashed, "/Volumes/") && !writable(parent):
		loc.Problem = "diskImage"
	case tempDir != "" && within(current, tempDir):
		loc.Problem = "temporary"
	case !writable(parent):
		loc.Problem = "readOnly"
	default:
		loc.CanUpdate = true
	}
	return loc
}

// installApp copies current to target. An app already at target is replaced
// only after the copy is complete and comes back if the swap fails.
func installApp(current, target string) error {
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return err
	}
	staged := target + ".neu"
	old := target + ".alt"
	_ = os.RemoveAll(staged)
	if err := copyApp(current, staged); err != nil {
		_ = os.RemoveAll(staged)
		return err
	}
	_ = os.RemoveAll(old)
	hadOld := false
	if _, err := os.Lstat(target); err == nil {
		if err := os.Rename(target, old); err != nil {
			_ = os.RemoveAll(staged)
			return err
		}
		hadOld = true
	}
	if err := os.Rename(staged, target); err != nil {
		if hadOld {
			_ = os.Rename(old, target)
		}
		_ = os.RemoveAll(staged)
		return err
	}
	if hadOld {
		if err := os.RemoveAll(old); err != nil {
			log.Printf("move app: remove previous copy: %v", err)
		}
	}
	return nil
}

// dirWritable creates and removes a file in dir.
func dirWritable(dir string) bool {
	f, err := os.CreateTemp(dir, ".e-rechnung-studio-*")
	if err != nil {
		return false
	}
	name := f.Name()
	_ = f.Close()
	_ = os.Remove(name)
	return true
}

// within reports whether path lies inside dir, after resolving symbolic links
// (macOS reaches its temporary folder through /var → /private/var).
func within(path, dir string) bool {
	path, dir = resolved(path), resolved(dir)
	rel, err := filepath.Rel(dir, path)
	if err != nil {
		return false
	}
	return rel != "." && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) && !filepath.IsAbs(rel)
}

func samePath(a, b string) bool {
	a, b = resolved(a), resolved(b)
	if runtime.GOOS == "windows" {
		return strings.EqualFold(a, b)
	}
	return a == b
}

func resolved(p string) string {
	p = filepath.Clean(p)
	// Resolve the longest part that exists; a target may not exist yet.
	rest := ""
	for dir := p; ; dir = filepath.Dir(dir) {
		if r, err := filepath.EvalSymlinks(dir); err == nil {
			p = filepath.Join(r, rest)
			break
		}
		if filepath.Dir(dir) == dir {
			break
		}
		rest = filepath.Join(filepath.Base(dir), rest)
	}
	if runtime.GOOS == "windows" {
		p = strings.ToLower(p)
	}
	return p
}

// copyExecutable copies a single executable file and flushes it to disk.
func copyExecutable(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o755)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		_ = out.Close()
		return err
	}
	if err := out.Sync(); err != nil {
		_ = out.Close()
		return err
	}
	return out.Close()
}
