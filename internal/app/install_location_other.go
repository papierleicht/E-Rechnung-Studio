//go:build !darwin && !windows

package app

import (
	"os"
	"path/filepath"
	"strings"
)

// runningApp returns the AppImage the process runs from. Other Linux installs
// come from a package manager, which also updates them.
func runningApp() (string, bool) {
	img, mount := os.Getenv("APPIMAGE"), os.Getenv("APPDIR")
	exe, err := os.Executable()
	if err != nil || img == "" || mount == "" || !strings.HasPrefix(exe, filepath.Clean(mount)+string(filepath.Separator)) {
		return "", false
	}
	if info, err := os.Stat(img); err != nil || !info.Mode().IsRegular() {
		return "", false
	}
	return img, true
}

// recommendedTarget is ~/Applications, where AppImage tools look first.
func recommendedTarget(current string) (string, bool) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", false
	}
	return filepath.Join(home, "Applications", filepath.Base(current)), true
}

func copyApp(src, dst string) error { return copyExecutable(src, dst) }

const relaunchCommand = `exec "$2"`

// relaunchEnv drops the variables of this AppImage's runtime so the new one
// sets its own.
func relaunchEnv() []string {
	var env []string
	for _, kv := range os.Environ() {
		switch strings.SplitN(kv, "=", 2)[0] {
		case "APPIMAGE", "APPDIR", "ARGV0", "OWD":
			continue
		}
		env = append(env, kv)
	}
	return env
}
