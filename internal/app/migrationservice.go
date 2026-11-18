package app

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/wailsapp/wails/v3/pkg/application"
	"papierleicht.de/e-rechnung-studio/internal/model"
	"papierleicht.de/e-rechnung-studio/internal/store"
	"papierleicht.de/e-rechnung-studio/internal/umzug"
)

// MigrationService moves documents, customers and articles over from another
// program ("Umzug"). Documents go unchanged into the legacy archive; lists
// become customers and articles only after the customer saw every row.
type MigrationService struct {
	invoices *InvoiceService
	// mu keeps two imports from running at once.
	mu sync.Mutex
}

func NewMigrationService(invoices *InvoiceService) *MigrationService {
	return &MigrationService{invoices: invoices}
}

// MigrationOverview is what the move page shows when it opens.
type MigrationOverview struct {
	Sources []umzug.Source     `json:"sources"`
	Years   []model.LegacyYear `json:"years"`
	Total   int                `json:"total"`
}

func (s *MigrationService) Overview() (MigrationOverview, error) {
	sources, err := umzug.Sources()
	if err != nil {
		return MigrationOverview{}, err
	}
	years, err := s.invoices.store.LegacyYears()
	if err != nil {
		return MigrationOverview{}, err
	}
	total := 0
	for _, y := range years {
		total += y.Count
	}
	return MigrationOverview{Sources: sources, Years: years, Total: total}, nil
}

// FilePreviewItem is one document the app found, before anything is saved.
type FilePreviewItem struct {
	Origin    string `json:"origin"`
	Name      string `json:"name"`
	Files     int    `json:"files"`
	Kind      string `json:"kind"`
	Direction string `json:"direction"`
	Number    string `json:"number"`
	Date      string `json:"date"`
	Partner   string `json:"partner"`
	Gross     string `json:"gross"`
	Currency  string `json:"currency"`
	EInvoice  bool   `json:"eInvoice"`
	// Basis is "einvoice", "datev" or "" (see model.LegacyDocument).
	Basis string `json:"basis"`
	// Duplicate says that the archive already holds this file.
	Duplicate bool `json:"duplicate"`
}

// FilePreview lists what an import of the chosen paths would archive.
type FilePreview struct {
	Items   []FilePreviewItem `json:"items"`
	Skipped []umzug.Skipped   `json:"skipped"`
	// Tables are customer lists found inside exports.
	Tables     []umzug.ExportTable `json:"tables"`
	New        int                 `json:"new"`
	Duplicates int                 `json:"duplicates"`
	EInvoices  int                 `json:"eInvoices"`
}

func (s *MigrationService) own() (umzug.Own, error) {
	c, err := s.invoices.store.Company()
	return umzug.Own{Name: c.Name, VATID: c.VATID, TaxNumber: c.TaxNumber}, err
}

func sumOfBytes(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func cleanPaths(paths []string) ([]string, error) {
	out := make([]string, 0, len(paths))
	for _, p := range paths {
		if p = strings.TrimSpace(p); p == "" {
			continue
		}
		if !filepath.IsAbs(p) {
			return nil, errors.New("Wähle die Dateien bitte über den Dialog aus.")
		}
		out = append(out, filepath.Clean(p))
	}
	if len(out) == 0 {
		return nil, errors.New("Wähle mindestens eine Datei oder einen Ordner aus.")
	}
	return out, nil
}

// PreviewFiles reads the chosen files, folders and ZIP archives without saving.
func (s *MigrationService) PreviewFiles(paths []string) (FilePreview, error) {
	preview := FilePreview{Items: []FilePreviewItem{}, Skipped: []umzug.Skipped{}, Tables: []umzug.ExportTable{}}
	paths, err := cleanPaths(paths)
	if err != nil {
		return preview, err
	}
	own, err := s.own()
	if err != nil {
		return preview, err
	}
	c, err := umzug.Collect(paths)
	if err != nil {
		return preview, err
	}
	defer c.Close()
	plan, err := umzug.Analyze(c)
	if err != nil {
		return preview, err
	}
	preview.Skipped = append(preview.Skipped, plan.Skipped...)
	preview.Tables = append(preview.Tables, plan.Tables...)
	for _, g := range plan.Groups {
		d, err := umzug.Read(g, "", own)
		if err != nil {
			preview.Skipped = append(preview.Skipped, umzug.Skipped{Origin: g.Entries[0].Origin, Reason: "Die Datei ließ sich nicht lesen."})
			continue
		}
		item := FilePreviewItem{Origin: d.Origin, Name: d.Files[0].Name, Files: len(d.Files), Kind: d.Kind, Direction: d.Direction, Number: d.Number, Date: d.Date, Partner: d.Partner, Gross: d.Gross, Currency: d.Currency, EInvoice: d.EInvoice, Basis: d.Basis}
		if d.Kind == "export" {
			if item.Duplicate, err = s.invoices.store.HasLegacyExport(d.SourceID); err != nil {
				return preview, err
			}
		}
		for _, f := range d.Files {
			if d.Kind == "export" {
				break
			}
			known, err := s.invoices.store.HasLegacyFile(sumOfBytes(f.Data))
			if err != nil {
				return preview, err
			}
			item.Duplicate = item.Duplicate || known
		}
		if item.Duplicate {
			preview.Duplicates++
		} else {
			preview.New++
			if item.EInvoice {
				preview.EInvoices++
			}
		}
		preview.Items = append(preview.Items, item)
	}
	return preview, nil
}

// FileImportResult says what an import archived.
type FileImportResult struct {
	Imported   int             `json:"imported"`
	Duplicates int             `json:"duplicates"`
	Failed     []umzug.Skipped `json:"failed"`
}

// ImportFiles archives the chosen files. Each document is saved on its own,
// so an interruption keeps what was done and a second run only adds the rest.
func (s *MigrationService) ImportFiles(source string, paths []string) (FileImportResult, error) {
	result := FileImportResult{Failed: []umzug.Skipped{}}
	if _, ok := umzug.SourceByID(source); !ok {
		return result, errors.New("Wähle zuerst, aus welchem Programm du umziehst.")
	}
	paths, err := cleanPaths(paths)
	if err != nil {
		return result, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	own, err := s.own()
	if err != nil {
		return result, err
	}
	c, err := umzug.Collect(paths)
	if err != nil {
		return result, err
	}
	defer c.Close()
	plan, err := umzug.Analyze(c)
	if err != nil {
		return result, err
	}
	for _, g := range plan.Groups {
		d, err := umzug.Read(g, source, own)
		if err != nil {
			result.Failed = append(result.Failed, umzug.Skipped{Origin: g.Entries[0].Origin, Reason: "Die Datei ließ sich nicht lesen."})
			continue
		}
		_, added, err := s.invoices.store.AddLegacyDocument(d)
		switch {
		case err != nil:
			result.Failed = append(result.Failed, umzug.Skipped{Origin: d.Origin, Reason: err.Error()})
		case added:
			result.Imported++
		default:
			result.Duplicates++
		}
	}
	return result, nil
}

// Archive lists the legacy archive.
func (s *MigrationService) Archive(filter store.LegacyFilter) ([]model.LegacyDocument, error) {
	return s.invoices.store.LegacyDocuments(filter)
}

// OpenArchiveFile shows an archived file in the program the system uses for
// it, from a temporary copy, so the archive itself never changes.
func (s *MigrationService) OpenArchiveFile(documentID, sum string) error {
	f, err := s.invoices.store.LegacyFile(documentID, sum)
	if err != nil {
		return err
	}
	app := application.Get()
	if app == nil {
		return errors.New("Die Datei kannst du nur in der App öffnen.")
	}
	ext := strings.ToLower(filepath.Ext(f.Name))
	if ext == "" || len(ext) > 6 {
		ext = ".bin"
	}
	return openTemporaryCopy(app, "papierleicht-altbestand-*"+ext, f.Data)
}

// SaveArchiveFile writes an archived file to the chosen path.
func (s *MigrationService) SaveArchiveFile(documentID, sum, path string) error {
	f, err := s.invoices.store.LegacyFile(documentID, sum)
	if err != nil {
		return err
	}
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		if errors.Is(err, os.ErrExist) {
			return errors.New("Am gewählten Ort gibt es schon eine Datei mit diesem Namen.")
		}
		return err
	}
	if _, err = file.Write(f.Data); err != nil {
		file.Close()
		os.Remove(path)
		return err
	}
	return file.Close()
}

// SaveArchiveFiles writes all files of a legacy document, e.g. a whole
// export, into a new folder inside the chosen one and returns its path.
func (s *MigrationService) SaveArchiveFiles(documentID, dir string) (string, error) {
	docs, err := s.invoices.store.LegacyDocuments(store.LegacyFilter{})
	if err != nil {
		return "", err
	}
	var doc *model.LegacyDocument
	for i := range docs {
		if docs[i].ID == documentID {
			doc = &docs[i]
			break
		}
	}
	if doc == nil {
		return "", errors.New("Dieses Dokument gibt es im Altbestand nicht.")
	}
	if !filepath.IsAbs(dir) {
		return "", errors.New("Wähle den Ordner bitte über den Dialog aus.")
	}
	name := strings.Map(func(r rune) rune {
		if strings.ContainsRune(`/\:*?"<>|`, r) || r < 32 {
			return '-'
		}
		return r
	}, strings.TrimSpace(doc.Number))
	if name == "" {
		name = doc.ID
	}
	target := filepath.Join(dir, name)
	if err = os.Mkdir(target, 0o700); err != nil {
		if errors.Is(err, os.ErrExist) {
			return "", fmt.Errorf("Im gewählten Ordner gibt es „%s“ schon.", name)
		}
		return "", err
	}
	for _, f := range doc.Files {
		file, err := s.invoices.store.LegacyFile(doc.ID, f.SHA256)
		if err != nil {
			return target, err
		}
		// Names inside an export may contain folders, never a way out of the target.
		rel := filepath.FromSlash(strings.TrimLeft(filepath.ToSlash(filepath.Clean("/"+f.Name)), "/"))
		path := filepath.Join(target, rel)
		if err = os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			return target, err
		}
		if err = os.WriteFile(path, file.Data, 0o600); err != nil {
			return target, err
		}
	}
	return target, nil
}

// ListPreview shows how the rows of a customer or article list would be
// taken over. Without a mapping the app assigns the columns by their names.
type ListPreview struct {
	List      string                   `json:"list"`
	Fields    []umzug.Field            `json:"fields"`
	Columns   []string                 `json:"columns"`
	Mapping   map[string]int           `json:"mapping"`
	Encoding  string                   `json:"encoding"`
	Delimiter string                   `json:"delimiter"`
	Customers []umzug.CustomerProposal `json:"customers"`
	Products  []umzug.ProductProposal  `json:"products"`
	// New counts the rows that would be saved.
	New int `json:"new"`
}

func readListFile(path string) (umzug.Table, error) {
	if !filepath.IsAbs(path) {
		return umzug.Table{}, errors.New("Wähle die Liste bitte über den Dialog aus.")
	}
	info, err := os.Stat(path)
	if err != nil {
		return umzug.Table{}, errors.New("Die Liste ließ sich nicht öffnen.")
	}
	if info.Size() > umzug.MaxTableSize {
		return umzug.Table{}, errors.New("Die Liste ist größer als 20 MB.")
	}
	if ext := strings.ToLower(filepath.Ext(path)); ext == ".xlsx" || ext == ".xls" || ext == ".ods" {
		return umzug.Table{}, errors.New("Excel- und Calc-Dateien liest die App noch nicht. Öffne die Liste in Excel oder LibreOffice und speichere sie als CSV.")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return umzug.Table{}, errors.New("Die Liste ließ sich nicht lesen.")
	}
	return umzug.ReadList(data)
}

func (s *MigrationService) readList(list string, load func() (umzug.Table, error), mapping map[string]int) (ListPreview, error) {
	preview := ListPreview{List: list, Customers: []umzug.CustomerProposal{}, Products: []umzug.ProductProposal{}}
	if list != umzug.ListCustomers && list != umzug.ListProducts {
		return preview, errors.New("Unbekannte Liste.")
	}
	table, err := load()
	if err != nil {
		return preview, err
	}
	m := umzug.Mapping(mapping)
	if mapping == nil {
		m = umzug.GuessMapping(list, table.Columns)
	}
	for field, at := range m {
		if at < 0 || at >= len(table.Columns) {
			delete(m, field)
		}
	}
	preview.Fields, preview.Columns, preview.Mapping = umzug.Fields(list), table.Columns, m
	preview.Encoding, preview.Delimiter = table.Encoding, table.Delimiter
	if list == umzug.ListCustomers {
		existing, err := s.invoices.store.Customers()
		if err != nil {
			return preview, err
		}
		preview.Customers = umzug.Customers(table, m, existing)
		for _, p := range preview.Customers {
			if p.Error == "" && p.Existing == "" {
				preview.New++
			}
		}
		return preview, nil
	}
	existing, err := s.invoices.store.Products()
	if err != nil {
		return preview, err
	}
	preview.Products = umzug.Products(table, m, existing)
	for _, p := range preview.Products {
		if p.Error == "" && p.Existing == "" {
			preview.New++
		}
	}
	return preview, nil
}

// PreviewList reads a customer or article list without saving.
func (s *MigrationService) PreviewList(list, path string, mapping map[string]int) (ListPreview, error) {
	return s.readList(list, func() (umzug.Table, error) { return readListFile(path) }, mapping)
}

// exportTable reads a customer list found inside an export.
func exportTable(paths []string, table umzug.ExportTable) func() (umzug.Table, error) {
	return func() (umzug.Table, error) {
		paths, err := cleanPaths(paths)
		if err != nil {
			return umzug.Table{}, err
		}
		c, err := umzug.Collect(paths)
		if err != nil {
			return umzug.Table{}, err
		}
		defer c.Close()
		return umzug.LoadExportTable(c, table)
	}
}

// PreviewExportList reads a customer list from inside an export without saving.
func (s *MigrationService) PreviewExportList(paths []string, table umzug.ExportTable, mapping map[string]int) (ListPreview, error) {
	return s.readList(umzug.ListCustomers, exportTable(paths, table), mapping)
}

// ImportExportList saves the new customers of a list inside an export.
func (s *MigrationService) ImportExportList(paths []string, table umzug.ExportTable, mapping map[string]int) (ListImportResult, error) {
	return s.importList(umzug.ListCustomers, exportTable(paths, table), mapping)
}

// ListImportResult says what an import of a list saved.
type ListImportResult struct {
	Created  int             `json:"created"`
	Existing int             `json:"existing"`
	Failed   []umzug.Skipped `json:"failed"`
}

// ImportList saves the new rows of a list the customer has reviewed. Rows
// with an error and entries that already exist are left out.
func (s *MigrationService) ImportList(list, path string, mapping map[string]int) (ListImportResult, error) {
	return s.importList(list, func() (umzug.Table, error) { return readListFile(path) }, mapping)
}

func (s *MigrationService) importList(list string, load func() (umzug.Table, error), mapping map[string]int) (ListImportResult, error) {
	result := ListImportResult{Failed: []umzug.Skipped{}}
	s.mu.Lock()
	defer s.mu.Unlock()
	preview, err := s.readList(list, load, mapping)
	if err != nil {
		return result, err
	}
	fail := func(row int, err error) {
		result.Failed = append(result.Failed, umzug.Skipped{Origin: fmt.Sprintf("Zeile %d", row), Reason: err.Error()})
	}
	for _, p := range preview.Customers {
		switch {
		case p.Error != "":
			fail(p.Row, errors.New(p.Error))
		case p.Existing != "":
			result.Existing++
		default:
			if _, err := s.invoices.SaveCustomer(p.Item); err != nil {
				fail(p.Row, err)
				continue
			}
			result.Created++
		}
	}
	for _, p := range preview.Products {
		switch {
		case p.Error != "":
			fail(p.Row, errors.New(p.Error))
		case p.Existing != "":
			result.Existing++
		default:
			if _, err := s.invoices.SaveProduct(p.Item); err != nil {
				fail(p.Row, err)
				continue
			}
			result.Created++
		}
	}
	return result, nil
}
