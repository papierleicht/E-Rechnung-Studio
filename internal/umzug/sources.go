package umzug

import (
	"embed"
	"encoding/json"
	"fmt"
	"path"
	"sort"
)

// Every source has a folder in quellen/ with quelle.json and anleitung.md.
// The guide is plain Markdown, so the website can show the same text.
//
//go:embed quellen/*/quelle.json quellen/*/anleitung.md
var guides embed.FS

// Source is a program customers move from, with the guide that tells them
// what to do there.
type Source struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Vendor string `json:"vendor"`
	// Kind is "cloud", "desktop" or "other".
	Kind    string `json:"kind"`
	Summary string `json:"summary"`
	// Deadline is the day the program stops, as YYYY-MM-DD.
	Deadline string `json:"deadline,omitempty"`
	// Ways are the imports this source supports: "files", "customers", "products".
	Ways []string `json:"ways"`
	// Checked says against what the guide was written, shown under it.
	Checked string   `json:"checked"`
	Links   []string `json:"sources"`
	// Guide is the Markdown text of the guide.
	Guide string `json:"guide"`
}

// Sources lists the guides, programs with a deadline first, "andere" last.
func Sources() ([]Source, error) {
	dirs, err := guides.ReadDir("quellen")
	if err != nil {
		return nil, err
	}
	out := make([]Source, 0, len(dirs))
	for _, d := range dirs {
		raw, err := guides.ReadFile(path.Join("quellen", d.Name(), "quelle.json"))
		if err != nil {
			return nil, err
		}
		var s Source
		if err = json.Unmarshal(raw, &s); err != nil {
			return nil, fmt.Errorf("quellen/%s/quelle.json: %w", d.Name(), err)
		}
		guide, err := guides.ReadFile(path.Join("quellen", d.Name(), "anleitung.md"))
		if err != nil {
			return nil, err
		}
		s.Guide = string(guide)
		if s.Links == nil {
			s.Links = []string{}
		}
		out = append(out, s)
	}
	sort.SliceStable(out, func(i, j int) bool {
		a, b := out[i], out[j]
		if (a.ID == "andere") != (b.ID == "andere") {
			return b.ID == "andere"
		}
		if (a.Deadline != "") != (b.Deadline != "") {
			return a.Deadline != ""
		}
		return a.Name < b.Name
	})
	return out, nil
}

// SourceByID returns one source.
func SourceByID(id string) (Source, bool) {
	all, err := Sources()
	if err != nil {
		return Source{}, false
	}
	for _, s := range all {
		if s.ID == id {
			return s, true
		}
	}
	return Source{}, false
}
