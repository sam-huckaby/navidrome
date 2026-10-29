package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/navidrome/navidrome/conf"
	"github.com/navidrome/navidrome/ui"
)

func TestPWABuildRouting(t *testing.T) {
	for _, tc := range []struct {
		name, url, accept, mode string
		status                  int
		contentType, cache      string
	}{
		{"root", "/pwa/", "", "", 200, "text/html", "no-cache"},
		{"deep link", "/pwa/albums/123", "text/html", "", 200, "text/html", "no-cache"},
		{"navigation header", "/pwa/albums/123", "", "navigate", 200, "text/html", "no-cache"},
		{"missing asset", "/pwa/assets/nope.js", "text/html", "navigate", 404, "", ""},
		{"missing media", "/pwa/offline-media/nope.mp3", "text/html", "navigate", 404, "", ""},
		{"missing non-navigation", "/pwa/albums/123", "", "", 404, "", ""},
		{"service worker", "/pwa/sw.js", "", "", 200, "javascript", "no-cache"},
		{"manifest", "/pwa/manifest.webmanifest", "", "", 200, "application/manifest+json", ""},
		{"hashed asset", "/pwa/assets/index-C-RNJvFT.js", "", "", 200, "javascript", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodGet, tc.url, nil)
			r.Header.Set("Accept", tc.accept)
			r.Header.Set("Sec-Fetch-Mode", tc.mode)
			w := httptest.NewRecorder()
			pwaAssetsHandler(ui.PWABuildAssets()).ServeHTTP(w, r)
			if w.Code != tc.status || !strings.Contains(w.Header().Get("Content-Type"), tc.contentType) || w.Header().Get("Cache-Control") != tc.cache {
				t.Fatalf("status=%d content-type=%q cache=%q body=%q", w.Code, w.Header().Get("Content-Type"), w.Header().Get("Cache-Control"), w.Body.String())
			}
		})
	}
}

func TestPWABasePath(t *testing.T) {
	old := conf.Server.BasePath
	conf.Server.BasePath = "/music"
	t.Cleanup(func() { conf.Server.BasePath = old })
	w := httptest.NewRecorder()
	pwaAssetsHandler(ui.PWABuildAssets()).ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/music/pwa/", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d", w.Code)
	}
}
