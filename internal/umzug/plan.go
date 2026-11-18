package umzug

import (
	"errors"
	"path"
	"sort"
	"strings"
)

// Plan is what an import of a collection would do: documents to archive
// (receipts and whole exports) and customer lists found inside exports.
type Plan struct {
	Groups  []Group
	Tables  []ExportTable
	Skipped []Skipped
}

// ExportTable points to a customer list inside an export, so it can be read
// again when the customer opens it.
type ExportTable struct {
	// Format is "datev" or "gdpdu".
	Format string `json:"format"`
	// Origin is the DATEV file or the index.xml.
	Origin string `json:"origin"`
	// Table is the name of a GDPdU table.
	Table string `json:"table"`
	Label string `json:"label"`
	Rows  int    `json:"rows"`
}

type bundle struct {
	label   string
	date    string
	entries []Entry
}

func lowerBase(origin string) string { return strings.ToLower(path.Base(origin)) }

// Analyze recognizes DATEV exports, DATEV document packages and GDPdU exports
// in a collection and groups everything else into documents.
func Analyze(c *Collection) (*Plan, error) {
	p := &Plan{Skipped: append([]Skipped(nil), c.Skipped...)}
	byOrigin := map[string]Entry{}
	byLower := map[string]Entry{}
	for _, e := range c.Entries {
		byOrigin[e.Origin] = e
		byLower[strings.ToLower(e.Origin)] = e
	}
	find := func(dir, name string) (Entry, bool) {
		name = strings.TrimPrefix(path.Clean("/"+strings.ReplaceAll(name, `\`, "/")), "/")
		if e, ok := byOrigin[dir+"/"+name]; ok {
			return e, true
		}
		e, ok := byLower[strings.ToLower(dir+"/"+name)]
		return e, ok
	}
	claimed := map[string]bool{}
	bundles := map[string]*bundle{}
	addToBundle := func(dir, label string, e Entry) {
		b := bundles[dir+"\x00"+label]
		if b == nil {
			b = &bundle{label: label}
			bundles[dir+"\x00"+label] = b
		}
		if !claimed[e.Origin] {
			b.entries = append(b.entries, e)
			claimed[e.Origin] = true
		}
	}

	// DATEV document packages: receipts listed in document.xml.
	type pending struct {
		group int
		guid  string
	}
	var waiting []pending
	for _, e := range c.Entries {
		if lowerBase(e.Origin) != "document.xml" {
			continue
		}
		data, err := e.Open()
		if err != nil {
			continue
		}
		docs := readDocumentXML(data)
		if len(docs) == 0 {
			continue
		}
		dir := path.Dir(e.Origin)
		addToBundle(dir, "DATEV-Belegpaket", e)
		for _, d := range docs {
			var g Group
			for _, name := range d.Files {
				if f, ok := find(dir, name); ok && !claimed[f.Origin] {
					g.Entries = append(g.Entries, f)
					claimed[f.Origin] = true
				}
			}
			if d.DataFile != "" {
				if f, ok := find(dir, d.DataFile); ok {
					if raw, err := f.Open(); err == nil {
						if b, ok := readLedger(raw); ok {
							if d.Direction != "" {
								b.Direction = d.Direction
							}
							g.Meta = &b
						}
					}
					addToBundle(dir, "DATEV-Belegpaket", f)
				}
			}
			if len(g.Entries) == 0 {
				continue
			}
			if g.Meta == nil && d.Direction != "" {
				g.Meta = &Booking{Direction: d.Direction}
			}
			p.Groups = append(p.Groups, g)
			if d.GUID != "" {
				waiting = append(waiting, pending{len(p.Groups) - 1, d.GUID})
			}
		}
	}

	// GDPdU exports: index.xml with the tables it describes.
	for _, e := range c.Entries {
		if lowerBase(e.Origin) != "index.xml" || claimed[e.Origin] {
			continue
		}
		data, err := e.Open()
		if err != nil {
			continue
		}
		idx, err := readIndex(data)
		if err != nil {
			continue
		}
		dir := path.Dir(e.Origin)
		label := "Prüferexport (GDPdU)"
		if idx.Media != "" {
			label += " " + idx.Media
		}
		addToBundle(dir, label, e)
		for _, other := range c.Entries {
			if path.Dir(other.Origin) == dir && strings.EqualFold(path.Ext(other.Origin), ".dtd") {
				addToBundle(dir, label, other)
			}
		}
		for _, t := range idx.Tables {
			f, ok := find(dir, t.URL)
			if !ok {
				p.Skipped = append(p.Skipped, Skipped{dir + "/" + t.URL, "Diese Tabelle nennt die index.xml, sie fehlt aber im Export."})
				continue
			}
			addToBundle(dir, label, f)
			if !customerTable(t.Columns) {
				continue
			}
			raw, err := f.Open()
			if err != nil {
				continue
			}
			if table, err := readGdpduTable(t, raw); err == nil && len(table.Rows) > 0 {
				name := t.Description
				if name == "" {
					name = t.Name
				}
				p.Tables = append(p.Tables, ExportTable{Format: "gdpdu", Origin: e.Origin, Table: t.Name, Label: name + " (Prüferexport)", Rows: len(table.Rows)})
			}
		}
	}

	// DATEV-Format files: Buchungsstapel, Debitoren/Kreditoren and others.
	bookings := map[string]Booking{}
	names := map[string]string{}
	for _, e := range c.Entries {
		if claimed[e.Origin] || fileKind(e.Name) != "data" || strings.EqualFold(path.Ext(e.Name), ".dtd") {
			continue
		}
		data, err := e.Open()
		if err != nil || !isDatev(data) {
			continue
		}
		f, err := ReadDatev(data)
		if err != nil {
			continue
		}
		dir := path.Dir(e.Origin)
		addToBundle(dir, "DATEV-Export", e)
		b := bundles[dir+"\x00DATEV-Export"]
		if to := isoDay(f.To); to > b.date {
			b.date = to
		}
		for guid, booking := range f.Bookings() {
			bookings[guid] = booking
		}
		for account, name := range f.partnerNames() {
			names[account] = name
		}
		if f.Category == datevPartners {
			customers := 0
			m := GuessMapping(ListCustomers, f.Table.Columns)
			for _, row := range f.Table.Rows {
				if isDebitor, ok := debitor(cell(row, m, "account"), f.AccountLength); ok && isDebitor {
					customers++
				}
			}
			if customers > 0 {
				p.Tables = append(p.Tables, ExportTable{Format: "datev", Origin: e.Origin, Label: "Debitoren (DATEV)", Rows: customers})
			}
		}
	}
	for _, w := range waiting {
		g := &p.Groups[w.group]
		b, ok := bookings[w.guid]
		if !ok {
			continue
		}
		if g.Meta != nil && g.Meta.Number != "" {
			continue
		}
		if name := names[b.Account]; name != "" {
			b.Text = name
		}
		g.Meta = &b
	}

	// Everything else: receipts as before, lists and unknown data are skipped.
	var rest []Entry
	for _, e := range c.Entries {
		if claimed[e.Origin] {
			continue
		}
		if fileKind(e.Name) == "data" {
			p.Skipped = append(p.Skipped, Skipped{e.Origin, "Keine Rechnung und kein bekannter Export. Kunden- und Artikellisten übernimmst du mit „Kunden aus Liste“ oder „Artikel aus Liste“."})
			continue
		}
		rest = append(rest, e)
	}
	p.Groups = append(p.Groups, groupEntries(rest)...)

	keys := make([]string, 0, len(bundles))
	for k := range bundles {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		b := bundles[k]
		dir, _, _ := strings.Cut(k, "\x00")
		sort.Slice(b.entries, func(i, j int) bool { return b.entries[i].Origin < b.entries[j].Origin })
		g := Group{Entries: b.entries, Export: dir, Label: b.label}
		if b.date != "" {
			g.Meta = &Booking{Date: b.date}
		}
		p.Groups = append(p.Groups, g)
	}
	return p, nil
}

// LoadExportTable reads a customer list found inside an export.
func LoadExportTable(c *Collection, t ExportTable) (Table, error) {
	var entry *Entry
	for i := range c.Entries {
		if c.Entries[i].Origin == t.Origin {
			entry = &c.Entries[i]
			break
		}
	}
	if entry == nil {
		return Table{}, errors.New("Die Liste ist im Export nicht mehr zu finden.")
	}
	data, err := entry.Open()
	if err != nil {
		return Table{}, err
	}
	switch t.Format {
	case "datev":
		f, err := ReadDatev(data)
		return f.Table, err
	case "gdpdu":
		idx, err := readIndex(data)
		if err != nil {
			return Table{}, err
		}
		dir := path.Dir(t.Origin)
		for _, table := range idx.Tables {
			if table.Name != t.Table {
				continue
			}
			url := strings.TrimPrefix(path.Clean("/"+strings.ReplaceAll(table.URL, `\`, "/")), "/")
			for _, e := range c.Entries {
				if strings.EqualFold(e.Origin, dir+"/"+url) {
					raw, err := e.Open()
					if err != nil {
						return Table{}, err
					}
					return readGdpduTable(table, raw)
				}
			}
		}
	}
	return Table{}, errors.New("Die Liste ist im Export nicht mehr zu finden.")
}
