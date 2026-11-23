// Package web embeds the built GUI frontend (web/dist) into the bv binary.
//
// go:embed cannot reach parent directories, so the embed lives here, next to
// the frontend sources. web/dist/index.html is a committed placeholder page so
// `go build ./...` works on a machine without Node. The real frontend build
// (`npm --prefix web run build`) writes to web/dist/app, which is gitignored
// and never touches the placeholder; when it exists, FS serves it instead.
package web

import (
	"embed"
	"io/fs"
)

//go:embed all:dist
var dist embed.FS

// appDir is where `npm run build` writes the frontend (vite.config.ts
// build.outDir).
const appDir = "dist/app"

// Built reports whether this binary embeds the Vite build rather than the
// committed placeholder page (dist/index.html).
var Built = func() bool {
	_, err := fs.Stat(dist, appDir+"/index.html")
	return err == nil
}()

// FS is the tree bv serve serves: the Vite build when present, else the
// placeholder. Either way index.html is at the top and hashed bundles are
// under assets/.
var FS = mustSub(dist, root())

func root() string {
	if Built {
		return appDir
	}
	return "dist"
}

func mustSub(fsys fs.FS, dir string) fs.FS {
	sub, err := fs.Sub(fsys, dir)
	if err != nil {
		panic("web: embedded " + dir + " missing: " + err.Error())
	}
	return sub
}
