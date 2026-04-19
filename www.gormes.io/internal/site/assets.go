package site

import (
	"html/template"
	"os"
	"path/filepath"
	"runtime"
)

func parseTemplates() (*template.Template, error) {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		return nil, os.ErrNotExist
	}

	root := filepath.Join(filepath.Dir(file), "..", "..")
	return template.ParseFS(
		os.DirFS(root),
		"templates/*.tmpl",
		"templates/partials/*.tmpl",
	)
}
