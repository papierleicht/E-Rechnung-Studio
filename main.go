// Command e-rechnung-studio is the desktop app. The application itself lives
// in internal/app; this package embeds what only the module root can reach:
// the built frontend and the product version.
package main

import (
	"embed"

	"papierleicht.de/e-rechnung-studio/internal/app"
)

//go:embed all:frontend/dist
var assets embed.FS

// version.txt is the product version managed by Release Please.
//
//go:embed version.txt
var version string

func main() {
	app.Run(assets, version)
}
