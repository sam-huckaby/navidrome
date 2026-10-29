package ui

import (
	"embed"
	"io/fs"
)

//go:embed all:build all:pwa
var filesystem embed.FS

func BuildAssets() fs.FS {
	build, _ := fs.Sub(filesystem, "build")
	return build
}

// PWABuildAssets contains the deployable build of the separate Music client.
func PWABuildAssets() fs.FS {
	pwa, _ := fs.Sub(filesystem, "pwa")
	return pwa
}
