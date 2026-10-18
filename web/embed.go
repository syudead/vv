// Package web embeds the SPA; serving lives in internal/httpapi/spa.go because embed cannot refer to parent directories.
// Tracked dist/.gitkeep keeps the embed target valid before the SPA is built.
package web

import (
	"embed"
	"io/fs"
)

//go:embed all:dist
var dist embed.FS

// Dist returns the filesystem rooted at web/dist.
func Dist() fs.FS {
	sub, err := fs.Sub(dist, "dist")
	if err != nil {
		// Unreachable because dist is embedded.
		panic(err)
	}
	return sub
}
