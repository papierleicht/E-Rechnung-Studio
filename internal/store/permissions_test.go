package store

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// The database also holds bank data and must stay private to its owner.
func TestDatabasePrivatePermissions(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("mode bits are not used on Windows")
	}
	path := filepath.Join(t.TempDir(), "app.db")
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	fi, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if fi.Mode().Perm()&0o077 != 0 {
		t.Fatal(fi.Mode())
	}
}
