package dashboardui

import (
	"embed"
	"io/fs"
)

//go:embed dist/*
var files embed.FS

func FileSystem() (fs.FS, error) {
	return fs.Sub(files, "dist")
}
