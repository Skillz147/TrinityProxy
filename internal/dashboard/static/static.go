package static

import (
	"embed"
	"io/fs"
)

//go:embed all:dist
var content embed.FS

// FS is the built dashboard UI (synced from web/dashboard/dist at build time).
func FS() (fs.FS, error) {
	return fs.Sub(content, "dist")
}

// Available reports whether embedded UI assets were included in the binary.
func Available() bool {
	entries, err := content.ReadDir("dist")
	return err == nil && len(entries) > 0
}
