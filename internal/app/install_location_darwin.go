//go:build darwin

package app

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// runningApp returns the .app bundle the process runs from.
func runningApp() (string, bool) {
	exe, err := os.Executable()
	if err != nil {
		return "", false
	}
	parts := strings.Split(filepath.Clean(exe), string(os.PathSeparator))
	for i, p := range parts {
		if strings.HasSuffix(p, ".app") {
			return string(os.PathSeparator) + filepath.Join(parts[1:i+1]...), true
		}
	}
	return "", false
}

// recommendedTarget is the Programme folder, or the user's own one when the
// account may not write to /Applications.
func recommendedTarget(current string) (string, bool) {
	name := filepath.Base(current)
	if dirWritable("/Applications") {
		return filepath.Join("/Applications", name), true
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", false
	}
	return filepath.Join(home, "Applications", name), true
}

// copyApp copies the bundle with its signature and drops the quarantine flag.
// The user already opened this copy; with the flag macOS would run the moved
// app from a hidden read-only copy again.
func copyApp(src, dst string) error {
	if out, err := exec.Command("/usr/bin/ditto", src, dst).CombinedOutput(); err != nil {
		return fmt.Errorf("ditto: %v: %s", err, strings.TrimSpace(string(out)))
	}
	_ = exec.Command("/usr/bin/xattr", "-dr", "com.apple.quarantine", dst).Run()
	return nil
}

const relaunchCommand = `exec /usr/bin/open "$2"`

func relaunchEnv() []string { return os.Environ() }
