package main

import (
	"embed"
	"io/fs"
)

//go:embed ui/dist
var distFS embed.FS

// uiFS returns the ui/dist sub-tree so the server can serve it at /.
func uiFS() fs.FS {
	sub, err := fs.Sub(distFS, "ui/dist")
	if err != nil {
		panic("uiFS: " + err.Error())
	}
	return sub
}
