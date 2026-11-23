// Package web embeds the built GUI frontend (web/dist) into the bv binary.
//
// go:embed cannot reach parent directories, so the embed lives here, next to
// the frontend sources. web/dist/index.html is a committed placeholder page so
// `go build ./...` works on a machine without Node; the real frontend build
// (`npm --prefix web run build`) overwrites web/dist with hashed assets, which
// are gitignored.
package web

import (
	"embed"
	"io/fs"
)

//go:embed all:dist
var dist embed.FS

// FS is the frontend asset tree rooted at web/dist: index.html at the top,
// hashed bundles under assets/.
var FS = mustSub(dist, "dist")

func mustSub(fsys fs.FS, dir string) fs.FS {
	sub, err := fs.Sub(fsys, dir)
	if err != nil {
		panic("web: embedded " + dir + " missing: " + err.Error())
	}
	return sub
}
