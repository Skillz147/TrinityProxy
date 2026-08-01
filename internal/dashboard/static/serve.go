package static

import (
	"io/fs"
	"log/slog"
	"net/http"
	"path"
	"strings"

	"github.com/Skillz147/TrinityProxy/internal/logutil"
)

// Register serves the embedded SPA on mux (non-/api routes).
func Register(mux *http.ServeMux, ui fs.FS, log *slog.Logger) {
	indexData, err := fs.ReadFile(ui, "index.html")
	if err != nil {
		logutil.Fatal(log, "embedded UI missing index.html", "err", err)
	}

	fileServer := http.FileServer(http.FS(ui))
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api/") {
			http.NotFound(w, r)
			return
		}
		if r.URL.Path != "/" {
			clean := path.Clean(r.URL.Path)
			if clean == "." {
				clean = "/"
			}
			if clean != "/" {
				if _, err := fs.Stat(ui, strings.TrimPrefix(clean, "/")); err == nil {
					fileServer.ServeHTTP(w, r)
					return
				}
			}
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(indexData)
	})
	log.Info("serving embedded dashboard UI")
}
