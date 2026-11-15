package app

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestInspectLocation(t *testing.T) {
	tmp := t.TempDir()
	writable := func(string) bool { return true }
	readOnly := func(string) bool { return false }
	cases := []struct {
		name, current string
		writable      func(string) bool
		want          string
	}{
		{"applications", "/Applications/E-Rechnung-Studio.app", writable, ""},
		{"translocated", "/private/var/folders/x/T/AppTranslocation/ABC/d/E-Rechnung-Studio.app", writable, "translocated"},
		{"disk image", "/Volumes/E-Rechnung-Studio/E-Rechnung-Studio.app", readOnly, "diskImage"},
		{"writable volume", "/Volumes/Daten/E-Rechnung-Studio.app", writable, ""},
		{"inside a zip", filepath.Join(tmp, "Temp1_app.zip", "e-rechnung-studio.exe"), writable, "temporary"},
		{"read-only folder", "/opt/apps/E-Rechnung-Studio.AppImage", readOnly, "readOnly"},
	}
	for _, c := range cases {
		got := inspectLocation(c.current, tmp, c.writable)
		if got.Problem != c.want || got.CanUpdate != (c.want == "") {
			t.Errorf("%s: got %+v, want problem %q", c.name, got, c.want)
		}
	}
}

func TestWithin(t *testing.T) {
	dir := t.TempDir()
	if !within(filepath.Join(dir, "a", "b"), dir) {
		t.Error("child not inside")
	}
	if within(dir, dir) || within(filepath.Dir(dir), dir) || within(dir+"x", dir) {
		t.Error("outside path reported inside")
	}
}

func TestInstallAppReplacesExistingCopy(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "src", "app.bin")
	target := filepath.Join(dir, "Programs", "E-Rechnung-Studio", "app.bin")
	if err := os.MkdirAll(filepath.Dir(src), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(src, []byte("new"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := installApp(src, target); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(src, []byte("newer"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := installApp(src, target); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(target)
	if err != nil || string(got) != "newer" {
		t.Fatalf("target = %q, %v", got, err)
	}
	for _, leftover := range []string{target + ".neu", target + ".alt"} {
		if _, err := os.Lstat(leftover); !os.IsNotExist(err) {
			t.Errorf("%s left behind", leftover)
		}
	}
	// Windows has no executable bit; elsewhere the copy must keep it.
	if info, err := os.Stat(target); err != nil || (runtime.GOOS != "windows" && info.Mode().Perm()&0o100 == 0) {
		t.Errorf("target not executable: %v", info)
	}
}

func TestInstallAppKeepsExistingCopyWhenCopyFails(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "app.bin")
	if err := os.WriteFile(target, []byte("old"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := installApp(filepath.Join(dir, "missing"), target); err == nil {
		t.Fatal("copy of a missing app succeeded")
	}
	if got, _ := os.ReadFile(target); string(got) != "old" {
		t.Fatalf("existing copy changed: %q", got)
	}
}

func TestLocationOfDevelopmentBuild(t *testing.T) {
	if loc := (&UpdateService{}).Location(); !loc.CanUpdate {
		t.Fatalf("development build reported %+v", loc)
	}
	if err := (&UpdateService{}).MoveApp(); err == nil {
		t.Fatal("development build moved itself")
	}
}
