package app

import (
	"bytes"
	"encoding/base64"
	"os"
	"path/filepath"
	"testing"
)

func TestReceivedDropPreservesOriginal(t *testing.T) {
	path := filepath.Join(t.TempDir(), "Rechnung.PDF")
	original := []byte{0, 1, 128, 255, '\n'}
	if err := os.WriteFile(path, original, 0600); err != nil {
		t.Fatal(err)
	}
	result := readReceivedDrop([]string{path})
	decoded, err := base64.StdEncoding.DecodeString(result.Encoded)
	if err != nil || result.Error != "" || result.Filename != "Rechnung.PDF" || !bytes.Equal(original, decoded) {
		t.Fatalf("unexpected payload: %+v %v", result, err)
	}
	after, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(original, after) {
		t.Fatal("original changed")
	}
}

func TestReceivedDropRejectsInvalidInputs(t *testing.T) {
	dir := t.TempDir()
	empty := filepath.Join(dir, "empty.xml")
	large := filepath.Join(dir, "large.pdf")
	if err := os.WriteFile(empty, nil, 0600); err != nil {
		t.Fatal(err)
	}
	file, err := os.Create(large)
	if err != nil {
		t.Fatal(err)
	}
	if err := file.Truncate(maxReceivedSize + 1); err != nil {
		t.Fatal(err)
	}
	file.Close()
	folder := filepath.Join(dir, "folder.pdf")
	if err := os.Mkdir(folder, 0700); err != nil {
		t.Fatal(err)
	}
	for _, paths := range [][]string{nil, {empty, large}, {empty}, {large}, {folder}, {filepath.Join(dir, "missing.xml")}, {filepath.Join(dir, "text.txt")}} {
		result := readReceivedDrop(paths)
		if result.Error == "" || result.Encoded != "" {
			t.Fatalf("accepted invalid input %v", paths)
		}
	}
}

func TestCSVDropAcceptsStatementsOnly(t *testing.T) {
	dir := t.TempDir()
	statement := filepath.Join(dir, "Umsätze.CSV")
	original := []byte("Datum;Betrag\n2026-10-15;-20,00\n")
	if err := os.WriteFile(statement, original, 0600); err != nil {
		t.Fatal(err)
	}
	result := readCSVDrop([]string{statement})
	decoded, err := base64.StdEncoding.DecodeString(result.Encoded)
	if err != nil || result.Error != "" || result.Filename != "Umsätze.CSV" || !bytes.Equal(original, decoded) {
		t.Fatalf("unexpected payload: %+v %v", result, err)
	}
	pdf := filepath.Join(dir, "Rechnung.pdf")
	if err := os.WriteFile(pdf, []byte("%PDF"), 0600); err != nil {
		t.Fatal(err)
	}
	large := filepath.Join(dir, "large.csv")
	file, err := os.Create(large)
	if err != nil {
		t.Fatal(err)
	}
	if err := file.Truncate(maxCSVDropSize + 1); err != nil {
		t.Fatal(err)
	}
	file.Close()
	for _, paths := range [][]string{nil, {statement, statement}, {pdf}, {large}} {
		result := readCSVDrop(paths)
		if result.Error == "" || result.Encoded != "" {
			t.Fatalf("accepted invalid input %v", paths)
		}
	}
}
