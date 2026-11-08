package app

import (
	"archive/zip"
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"strings"

	"papierleicht.de/e-rechnung-studio/internal/cloudbackup"
)

// A backup file is a ZIP archive that works without the app: the documents lie
// in their original format under dokumente/, named by their SHA-256, next to
// the backup document that refers to them. Every local backup uses it, so its
// size only depends on the disk, not on what fits into memory or the webview.

const (
	archiveDocument = "sicherung.json"
	archiveFolder   = "dokumente/"
	archiveReadme   = "LIESMICH.txt"
	// maxArchiveDocument bounds a backup document in memory.
	maxArchiveDocument = 1 << 30
)

const archiveReadmeText = `Sicherung aus E-Rechnung-Studio
================================

sicherung.json   Alle Daten: Rechnungen, Kunden, Artikel, Zahlungen, Einstellungen.
dokumente/       Alle Dateien in ihrem Originalformat. Der Dateiname ist die
                 SHA-256-Prüfsumme des Inhalts, damit jede Datei nachprüfbar bleibt.

Wiederherstellen: E-Rechnung-Studio öffnen, Einstellungen, Datensicherung,
"Sicherung importieren". Die App prüft dabei jede Datei gegen ihre Prüfsumme.

Diese Datei ist nicht verschlüsselt. Bewahre sie so sicher auf wie deine Belege.
Passwörter, Bank-PIN und Schlüssel sind nicht enthalten.
`

func documentExtension(data []byte) string {
	switch trimmed := bytes.TrimLeft(bytes.TrimPrefix(data, []byte("\xef\xbb\xbf")), " \t\r\n"); {
	case bytes.HasPrefix(data, []byte("%PDF")):
		return ".pdf"
	case bytes.HasPrefix(data, []byte("\xff\xd8\xff")):
		return ".jpg"
	case bytes.HasPrefix(data, []byte("\x89PNG")):
		return ".png"
	case len(data) > 12 && string(data[4:8]) == "ftyp":
		return ".heic"
	case bytes.HasPrefix(trimmed, []byte("<")):
		return ".xml"
	}
	return ""
}

// writeArchive replaces path only after the archive is complete.
func writeArchive(target string, document []byte, refs []cloudbackup.Ref, read func(cloudbackup.Ref) ([]byte, error), progress func(done int, bytes int64)) (err error) {
	tmp := target + ".tmp"
	file, err := os.OpenFile(tmp, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}
	defer func() {
		file.Close()
		if err != nil {
			os.Remove(tmp)
		}
	}()
	zw := zip.NewWriter(file)
	for name, content := range map[string][]byte{archiveReadme: []byte(archiveReadmeText), archiveDocument: document} {
		w, err := zw.Create(name)
		if err != nil {
			return err
		}
		if _, err = w.Write(content); err != nil {
			return err
		}
	}
	var written int64
	for i, ref := range refs {
		data, err := read(ref)
		if err != nil {
			return err
		}
		// Originals are compressed already, or must stay byte-exact to the eye.
		w, err := zw.CreateHeader(&zip.FileHeader{Name: archiveFolder + ref.Sum + documentExtension(data), Method: zip.Store})
		if err != nil {
			return err
		}
		if _, err = w.Write(data); err != nil {
			return err
		}
		written += ref.Size
		if progress != nil {
			progress(i+1, written)
		}
	}
	if err = zw.Close(); err != nil {
		return err
	}
	if err = file.Sync(); err != nil {
		return err
	}
	if err = file.Close(); err != nil {
		return err
	}
	return os.Rename(tmp, target)
}

// backupArchive is an opened backup file.
type backupArchive struct {
	zip      *zip.ReadCloser
	document []byte
	files    map[string]*zip.File
}

func openArchive(name string) (*backupArchive, error) {
	r, err := zip.OpenReader(name)
	if err != nil {
		return nil, errors.New("Die Datei ist keine Sicherung von E-Rechnung-Studio.")
	}
	a := &backupArchive{zip: r, files: map[string]*zip.File{}}
	for _, f := range r.File {
		switch {
		case f.Name == archiveDocument:
			if a.document, err = readArchiveFile(f, maxArchiveDocument); err != nil {
				r.Close()
				return nil, err
			}
		case strings.HasPrefix(f.Name, archiveFolder):
			base := path.Base(f.Name)
			a.files[strings.TrimSuffix(base, path.Ext(base))] = f
		}
	}
	if a.document == nil {
		r.Close()
		return nil, errors.New("Die Datei ist keine Sicherung von E-Rechnung-Studio.")
	}
	return a, nil
}

func (a *backupArchive) Close() error { return a.zip.Close() }

func readArchiveFile(f *zip.File, limit int64) ([]byte, error) {
	if f.UncompressedSize64 > uint64(limit) {
		return nil, errors.New("Die Sicherung enthält eine zu große Datei.")
	}
	r, err := f.Open()
	if err != nil {
		return nil, errors.New("Die Sicherung ist beschädigt.")
	}
	defer r.Close()
	data, err := io.ReadAll(io.LimitReader(r, limit+1))
	if err != nil || int64(len(data)) > limit {
		return nil, errors.New("Die Sicherung ist beschädigt.")
	}
	return data, nil
}

// open returns a document by its hash; the store checks the bytes against it.
func (a *backupArchive) open(sum string) ([]byte, error) {
	f, ok := a.files[sum]
	if !ok {
		return nil, fmt.Errorf("In der Sicherung fehlt das Dokument %s.", sum)
	}
	return readArchiveFile(f, maxArchiveDocument)
}

// verify checks every document against its name before anything is replaced.
func (a *backupArchive) verify(progress func(done int, bytes int64)) (int64, error) {
	var total int64
	done := 0
	for sum := range a.files {
		data, err := a.open(sum)
		if err != nil {
			return total, err
		}
		if err = (cloudbackup.Ref{Sum: sum}).Check(data); err != nil {
			return total, fmt.Errorf("Das Dokument %s in der Sicherung wurde verändert.", sum)
		}
		total += int64(len(data))
		done++
		if progress != nil {
			progress(done, total)
		}
	}
	return total, nil
}
