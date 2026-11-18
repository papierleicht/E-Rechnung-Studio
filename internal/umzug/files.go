// Package umzug reads what customers bring along from another program: folders
// and ZIP exports with invoices, and tables with customers and articles. It
// only reads; archiving and saving happen in the store after the customer
// confirmed what the app found.
package umzug

import (
	"archive/zip"
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
)

// Limits keep an import from reading more than a desktop can hold at once.
const (
	MaxFileSize   = 50 << 20
	maxNestedZIP  = 512 << 20
	maxEntries    = 100_000
	maxTotalBytes = 20 << 30
)

// Entry is one file found in a folder or an archive. Open reads its bytes.
type Entry struct {
	// Origin is the path as the customer sees it, e.g. "export.zip/2024/RE-1.pdf".
	Origin string
	Name   string
	Size   int64
	open   func() ([]byte, error)
}

// Open returns the bytes of the entry.
func (e Entry) Open() ([]byte, error) { return e.open() }

// Skipped is a file the import leaves out, with the reason shown to the customer.
type Skipped struct {
	Origin string `json:"origin"`
	Reason string `json:"reason"`
}

// Kind of a file by its extension: "pdf", "xml", "image" or "".
func fileKind(name string) string {
	switch strings.ToLower(path.Ext(name)) {
	case ".pdf":
		return "pdf"
	case ".xml":
		return "xml"
	case ".png", ".jpg", ".jpeg", ".tif", ".tiff", ".heic", ".webp":
		return "image"
	}
	return ""
}

func mimeOf(name string) string {
	switch strings.ToLower(path.Ext(name)) {
	case ".pdf":
		return "application/pdf"
	case ".xml":
		return "application/xml"
	case ".png":
		return "image/png"
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".tif", ".tiff":
		return "image/tiff"
	case ".heic":
		return "image/heic"
	case ".webp":
		return "image/webp"
	}
	return "application/octet-stream"
}

func hidden(name string) bool {
	base := path.Base(filepath.ToSlash(name))
	return strings.HasPrefix(base, ".") || strings.HasPrefix(base, "~$") || base == "Thumbs.db" || base == "desktop.ini" ||
		strings.Contains(filepath.ToSlash(name), "__MACOSX/")
}

// Collection is what a set of chosen files and folders contains. Archives
// stay open until Close.
type Collection struct {
	Entries []Entry
	Skipped []Skipped
	closers []io.Closer
}

// Close releases the archives opened while collecting.
func (c *Collection) Close() error {
	var first error
	for _, cl := range c.closers {
		if err := cl.Close(); err != nil && first == nil {
			first = err
		}
	}
	c.closers = nil
	return first
}

// Collect walks the chosen paths: files, folders and ZIP archives, also ZIPs
// inside ZIPs (exports for tax advisers often nest them). Only documents
// (PDF, XML, images) become entries; everything else is listed as skipped.
func Collect(paths []string) (*Collection, error) {
	c := &Collection{}
	var total int64
	add := func(e Entry) error {
		if len(c.Entries) >= maxEntries {
			return fmt.Errorf("Mehr als %d Dateien auf einmal kann die App nicht übernehmen. Wähle bitte einzelne Jahre oder Ordner.", maxEntries)
		}
		total += e.Size
		if total > maxTotalBytes {
			return errors.New("Die Auswahl ist größer als 20 GB. Wähle bitte einzelne Jahre oder Ordner.")
		}
		c.Entries = append(c.Entries, e)
		return nil
	}
	var visit func(origin, name string, size int64, open func() ([]byte, error), depth int) error
	visit = func(origin, name string, size int64, open func() ([]byte, error), depth int) error {
		if hidden(origin) {
			return nil
		}
		if strings.EqualFold(path.Ext(name), ".zip") {
			if depth >= 3 {
				c.Skipped = append(c.Skipped, Skipped{origin, "ZIP-Dateien, die so tief verschachtelt sind, liest die App nicht."})
				return nil
			}
			if size > maxNestedZIP && depth > 0 {
				c.Skipped = append(c.Skipped, Skipped{origin, "Diese ZIP-Datei in der ZIP-Datei ist zu groß. Entpacke sie bitte vorher."})
				return nil
			}
			data, err := open()
			if err != nil {
				c.Skipped = append(c.Skipped, Skipped{origin, "Die ZIP-Datei ließ sich nicht lesen."})
				return nil
			}
			r, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
			if err != nil {
				c.Skipped = append(c.Skipped, Skipped{origin, "Die ZIP-Datei ist beschädigt oder verschlüsselt."})
				return nil
			}
			return walkZIP(r, origin, depth+1, visit)
		}
		if fileKind(name) == "" {
			c.Skipped = append(c.Skipped, Skipped{origin, "Kein Beleg (PDF, XML oder Bild)."})
			return nil
		}
		if size > MaxFileSize {
			c.Skipped = append(c.Skipped, Skipped{origin, "Die Datei ist größer als 50 MB."})
			return nil
		}
		if size == 0 {
			c.Skipped = append(c.Skipped, Skipped{origin, "Die Datei ist leer."})
			return nil
		}
		return add(Entry{Origin: origin, Name: path.Base(name), Size: size, open: open})
	}
	for _, p := range paths {
		info, err := os.Stat(p)
		if err != nil {
			c.Close()
			return nil, fmt.Errorf("%s ließ sich nicht öffnen.", filepath.Base(p))
		}
		base := filepath.Base(p)
		if !info.IsDir() {
			if strings.EqualFold(filepath.Ext(p), ".zip") {
				// Large archives on disk are read from the file, not into memory.
				r, err := zip.OpenReader(p)
				if err != nil {
					c.Skipped = append(c.Skipped, Skipped{base, "Die ZIP-Datei ist beschädigt oder verschlüsselt."})
					continue
				}
				c.closers = append(c.closers, r)
				if err = walkZIP(&r.Reader, base, 1, visit); err != nil {
					c.Close()
					return nil, err
				}
				continue
			}
			file := p
			if err = visit(base, base, info.Size(), func() ([]byte, error) { return os.ReadFile(file) }, 0); err != nil {
				c.Close()
				return nil, err
			}
			continue
		}
		root := p
		err = filepath.WalkDir(root, func(full string, d fs.DirEntry, err error) error {
			if err != nil {
				rel, _ := filepath.Rel(root, full)
				c.Skipped = append(c.Skipped, Skipped{filepath.ToSlash(filepath.Join(base, rel)), "Der Ordner ließ sich nicht lesen."})
				return nil
			}
			rel, _ := filepath.Rel(root, full)
			origin := filepath.ToSlash(filepath.Join(base, rel))
			if d.IsDir() {
				if full != root && hidden(d.Name()) {
					return filepath.SkipDir
				}
				return nil
			}
			info, err := d.Info()
			if err != nil || !info.Mode().IsRegular() {
				return nil
			}
			if strings.EqualFold(filepath.Ext(full), ".zip") {
				r, err := zip.OpenReader(full)
				if err != nil {
					c.Skipped = append(c.Skipped, Skipped{origin, "Die ZIP-Datei ist beschädigt oder verschlüsselt."})
					return nil
				}
				c.closers = append(c.closers, r)
				return walkZIP(&r.Reader, origin, 1, visit)
			}
			return visit(origin, d.Name(), info.Size(), func() ([]byte, error) { return os.ReadFile(full) }, 0)
		})
		if err != nil {
			c.Close()
			return nil, err
		}
	}
	sort.SliceStable(c.Entries, func(i, j int) bool { return c.Entries[i].Origin < c.Entries[j].Origin })
	return c, nil
}

func walkZIP(r *zip.Reader, origin string, depth int, visit func(origin, name string, size int64, open func() ([]byte, error), depth int) error) error {
	for _, f := range r.File {
		if f.FileInfo().IsDir() {
			continue
		}
		name := filepath.ToSlash(f.Name)
		// A ZIP may claim paths outside itself; only the name inside counts.
		name = strings.TrimLeft(path.Clean("/"+name), "/")
		file := f
		open := func() ([]byte, error) {
			if file.UncompressedSize64 > maxNestedZIP {
				return nil, errors.New("too large")
			}
			rc, err := file.Open()
			if err != nil {
				return nil, err
			}
			defer rc.Close()
			return io.ReadAll(io.LimitReader(rc, maxNestedZIP+1))
		}
		if err := visit(origin+"/"+name, name, int64(f.UncompressedSize64), open, depth); err != nil {
			return err
		}
	}
	return nil
}
