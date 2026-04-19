package site

import (
	"embed"
	"html/template"
	"io/fs"
)

//go:embed templates/*.tmpl templates/partials/*.tmpl static/*
var siteFS embed.FS
var templateFS = siteFS

func parseTemplates() (*template.Template, error) {
	return template.ParseFS(
		siteFS,
		"templates/*.tmpl",
		"templates/partials/*.tmpl",
	)
}

func staticFS() (fs.FS, error) {
	return fs.Sub(siteFS, "static")
}
