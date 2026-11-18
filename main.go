// Command e-rechnung-studio is the desktop app. The application itself lives
// in internal/app; this package embeds what only the module root can reach:
// the built frontend and the product version.
package main

import (
	"embed"
	"io/fs"

	"papierleicht.de/e-rechnung-studio/internal/app"
)

//go:embed all:frontend/dist
var assets embed.FS

// version.txt is the product version managed by Release Please.
//
//go:embed version.txt
var version string

// The German release notes, one file per version.
//
//go:embed docs/versionshinweise/*.md
var notes embed.FS

func main() {
	releaseNotes, err := fs.Sub(notes, "docs/versionshinweise")
	if err != nil {
		panic(err)
	}
	app.Run(assets, version, releaseNotes)
}
