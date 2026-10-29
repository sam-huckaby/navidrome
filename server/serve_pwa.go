package server

import (
	"io/fs"
	"net/http"
	"path"
	"strings"

	"github.com/navidrome/navidrome/conf"
)

// pwaAssetsHandler serves only the PWA build. Navigation routes fall back to
// index.html; missing assets must remain 404 instead of receiving HTML.
func pwaAssetsHandler(assets fs.FS) http.Handler {
	root := path.Join(conf.Server.BasePath, "/pwa")
	files := http.FileServer(http.FS(assets))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			w.Header().Set("Allow", "GET, HEAD")
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		name := strings.TrimPrefix(r.URL.Path, root)
		name = strings.TrimPrefix(name, "/")
		if name == "" || name == "index.html" {
			servePWAIndex(w, r, assets)
			return
		}

		file, err := fs.Stat(assets, name)
		if err != nil || file.IsDir() {
			if err == nil || !isPWANavigation(r, name) {
				http.NotFound(w, r)
				return
			}
			servePWAIndex(w, r, assets)
			return
		}
		if name == "sw.js" {
			w.Header().Set("Cache-Control", "no-cache")
		} else if name == "manifest.webmanifest" {
			w.Header().Set("Content-Type", "application/manifest+json")
		}
		r.URL.Path = "/" + name
		files.ServeHTTP(w, r)
	})
}

func isPWANavigation(r *http.Request, name string) bool {
	if strings.HasPrefix(name, "assets/") || strings.HasPrefix(name, "fonts/") || strings.HasPrefix(name, "icons/") || path.Ext(name) != "" {
		return false
	}
	return r.Header.Get("Sec-Fetch-Mode") == "navigate" || strings.Contains(r.Header.Get("Accept"), "text/html")
}

func servePWAIndex(w http.ResponseWriter, r *http.Request, assets fs.FS) {
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	r.URL.Path = "/"
	http.FileServer(http.FS(assets)).ServeHTTP(w, r)
}
