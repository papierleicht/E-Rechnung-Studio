package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"papierleicht.de/e-rechnung-studio/internal/updatesign"
)

// keygen writes a key, then the public key printed for it is built in.
func newSigningKey(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "update-signing-key.txt")
	var out bytes.Buffer
	if code := run([]string{"keygen", "-out", path}, &out, &out); code != 0 {
		t.Fatal(out.String())
	}
	info, err := os.Stat(path)
	if err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("private key file not private: %v %v", info.Mode(), err)
	}
	pub := strings.TrimSpace(strings.TrimPrefix(strings.Split(out.String(), "\n")[1], "public key:"))
	old := updatesign.PublicKey
	updatesign.PublicKey = pub
	t.Cleanup(func() { updatesign.PublicKey = old })
	key, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(key)
}

func TestSignedPackagesVerifyAndChangesFail(t *testing.T) {
	t.Setenv("UPDATE_SIGNING_KEY", newSigningKey(t))
	dir := t.TempDir()
	pkg := filepath.Join(dir, "e-rechnung-studio-linux-amd64.AppImage")
	if err := os.WriteFile(pkg, bytes.Repeat([]byte{7}, 1<<20), 0o644); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if code := run([]string{"sign", pkg}, &out, &out); code != 0 {
		t.Fatal(out.String())
	}
	if code := run([]string{"verify", pkg}, &out, &out); code != 0 {
		t.Fatal(out.String())
	}
	if err := os.WriteFile(pkg, []byte("ausgetauscht"), 0o644); err != nil {
		t.Fatal(err)
	}
	if code := run([]string{"verify", pkg}, &out, &out); code == 0 {
		t.Fatal("changed package verified")
	}
}

func TestKeyOfAnotherAppIsRefused(t *testing.T) {
	newSigningKey(t)
	other := filepath.Join(t.TempDir(), "other.txt")
	var out bytes.Buffer
	if code := run([]string{"keygen", "-out", other}, &out, &out); code != 0 {
		t.Fatal(out.String())
	}
	key, err := os.ReadFile(other)
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("UPDATE_SIGNING_KEY", string(key))
	pkg := filepath.Join(t.TempDir(), "e-rechnung-studio-windows-amd64.zip")
	if err := os.WriteFile(pkg, []byte("zip"), 0o644); err != nil {
		t.Fatal(err)
	}
	out.Reset()
	if code := run([]string{"sign", pkg}, &out, &out); code == 0 || !strings.Contains(out.String(), "does not belong") {
		t.Fatalf("foreign key accepted: %s", out.String())
	}
	if _, err := os.Stat(pkg + updatesign.Suffix); err == nil {
		t.Fatal("signature written with a foreign key")
	}
}

func TestKeygenNeverOverwritesAKey(t *testing.T) {
	path := filepath.Join(t.TempDir(), "key.txt")
	if err := os.WriteFile(path, []byte("vorhandener Schlüssel\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if code := run([]string{"keygen", "-out", path}, &out, &out); code == 0 {
		t.Fatal("existing key overwritten")
	}
	if data, _ := os.ReadFile(path); string(data) != "vorhandener Schlüssel\n" {
		t.Fatal("existing key changed")
	}
}
