// Package webui serves the embedded Next.js static export. The export is
// copied into dist at build time; go:embed all:dist includes Next's _next
// directory (the all: prefix is required for underscore-prefixed paths).
package webui

import (
	"bytes"
	"embed"
	"io/fs"
	"mime"
	"net/http"
	"path"
	"strings"
	"time"
)

//go:embed all:dist
var distFS embed.FS

func Handler() http.Handler {
	return &spaHandler{fs: distFS}
}

type spaHandler struct {
	fs fs.FS
}

func (h *spaHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		w.Header().Set("Allow", "GET, HEAD")
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	clean := path.Clean("/" + r.URL.Path)
	rel := strings.TrimPrefix(clean, "/")
	if rel == "" || rel == "." {
		rel = "index.html"
	}
	if strings.Contains(rel, "..") {
		http.NotFound(w, r)
		return
	}
	candidates := []string{rel, rel + ".html", path.Join(rel, "index.html")}
	for _, candidate := range candidates {
		if data, err := fs.ReadFile(h.fs, "dist/"+candidate); err == nil {
			h.serve(w, r, candidate, data)
			return
		}
	}
	// Unknown extensionless routes are client-side routes; anything with a
	// file extension is a missing asset and must 404 rather than return HTML.
	if path.Ext(rel) != "" {
		http.NotFound(w, r)
		return
	}
	index, err := fs.ReadFile(h.fs, "dist/index.html")
	if err != nil {
		http.Error(w, "UI is not embedded in this build", http.StatusServiceUnavailable)
		return
	}
	h.serve(w, r, "index.html", index)
}

func (h *spaHandler) serve(w http.ResponseWriter, r *http.Request, name string, data []byte) {
	contentType := mime.TypeByExtension(path.Ext(name))
	if contentType == "" {
		contentType = http.DetectContentType(data)
	}
	if contentType != "" {
		w.Header().Set("Content-Type", contentType)
	}
	if strings.HasPrefix(name, "_next/static/") {
		w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	} else {
		w.Header().Set("Cache-Control", "no-cache")
	}
	http.ServeContent(w, r, name, time.Time{}, bytes.NewReader(data))
}
