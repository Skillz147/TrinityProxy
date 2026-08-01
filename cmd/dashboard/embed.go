package main

import (
	"embed"
	"io/fs"
)

// Built UI is synced from web/dashboard/dist before go build (see scripts/build-dashboard-ui.sh).
//
//go:embed all:dist
var embeddedUI embed.FS

func embeddedUIFS() (fs.FS, error) {
	return fs.Sub(embeddedUI, "dist")
}

func embeddedUIAvailable() bool {
	_, err := fs.Stat(embeddedUI, "dist/index.html")
	return err == nil
}
