package site

import (
	"embed"
	"html/template"
)

//go:embed templates/*.tmpl templates/partials/*.tmpl
var templateFS embed.FS

func parseTemplates() (*template.Template, error) {
	return template.ParseFS(
		templateFS,
		"templates/*.tmpl",
		"templates/partials/*.tmpl",
	)
}
