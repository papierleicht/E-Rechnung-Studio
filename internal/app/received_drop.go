package app

import (
	"encoding/base64"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

type receivedDrop struct {
	Filename string `json:"filename"`
	Encoded  string `json:"encoded"`
	Error    string `json:"error"`
}

// What a drop target accepts and how it complains, in the user's words.
type dropRule struct {
	extensions []string
	maxSize    int64
	// The messages for several files at once, a wrong file type and a wrong size.
	oneOnly, wrongType, wrongSize string
}

var receivedDropRule = dropRule{
	extensions: []string{".pdf", ".xml"},
	maxSize:    maxReceivedSize,
	oneOnly:    "Lege immer nur eine einzelne Rechnung ab.",
	wrongType:  "Lege eine XML-Datei oder ein PDF ab.",
	wrongSize:  "Lege eine Datei bis 20 MB ab, die nicht leer ist.",
}

// The same limit as the file picker in the CSV dialog.
const maxCSVDropSize = 5_000_000

var csvDropRule = dropRule{
	extensions: []string{".csv", ".txt"},
	maxSize:    maxCSVDropSize,
	oneOnly:    "Lege immer nur einen einzelnen Kontoauszug ab.",
	wrongType:  "Lege eine CSV-Datei ab, wie sie dein Online-Banking exportiert.",
	wrongSize:  "Lege eine CSV-Datei bis 5 MB ab, die nicht leer ist.",
}

// Read only the dropped original; persistence still requires ImportReceivedInvoice.
func readReceivedDrop(paths []string) receivedDrop {
	return readDrop(paths, receivedDropRule)
}

// Read the dropped bank statement; the dialog still previews and confirms the import.
func readCSVDrop(paths []string) receivedDrop {
	return readDrop(paths, csvDropRule)
}

func readDrop(paths []string, rule dropRule) receivedDrop {
	if len(paths) != 1 {
		return receivedDrop{Error: rule.oneOnly}
	}
	path := paths[0]
	if !slices.Contains(rule.extensions, strings.ToLower(filepath.Ext(path))) {
		return receivedDrop{Error: rule.wrongType}
	}
	info, err := os.Stat(path)
	if err != nil || !info.Mode().IsRegular() {
		return receivedDrop{Error: "Die Datei lässt sich nicht lesen. Lege eine einzelne Datei statt eines Ordners ab."}
	}
	if info.Size() == 0 || info.Size() > rule.maxSize {
		return receivedDrop{Error: rule.wrongSize}
	}
	file, err := os.Open(path)
	if err != nil {
		return receivedDrop{Error: "Die Datei lässt sich nicht öffnen. Prüfe die Zugriffsrechte."}
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, rule.maxSize+1))
	if err != nil || len(data) == 0 || int64(len(data)) > rule.maxSize {
		return receivedDrop{Error: "Die Datei ist leer, zu groß oder lässt sich nicht lesen."}
	}
	return receivedDrop{Filename: filepath.Base(path), Encoded: base64.StdEncoding.EncodeToString(data)}
}
