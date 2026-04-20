package site

import (
	"bytes"
	"embed"
	"html/template"
	"io/fs"
	"os"
	"path/filepath"
)

//go:embed templates/*.tmpl templates/partials/*.tmpl static/* install.sh
var siteFS embed.FS

//go:embed data/benchmarks.json
var benchmarksJSON []byte
var templateFS = siteFS

type Site struct {
	page      LandingPage
	templates *template.Template
	static    fs.FS
	install   []byte
}

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

func loadSite() (*Site, error) {
	templates, err := parseTemplates()
	if err != nil {
		return nil, err
	}

	files, err := staticFS()
	if err != nil {
		return nil, err
	}

	install, err := siteFS.ReadFile("install.sh")
	if err != nil {
		return nil, err
	}

	return &Site{
		page:      DefaultPage(),
		templates: templates,
		static:    files,
		install:   install,
	}, nil
}

// InstallScript returns the embedded install.sh bytes served at /install.sh.
func (s *Site) InstallScript() []byte {
	return s.install
}

func (s *Site) RenderIndex() ([]byte, error) {
	var buf bytes.Buffer
	if err := s.templates.ExecuteTemplate(&buf, "layout", s.page); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func RenderIndex() ([]byte, error) {
	s, err := loadSite()
	if err != nil {
		return nil, err
	}
	return s.RenderIndex()
}

func (s *Site) ExportDir(root string) error {
	if err := os.RemoveAll(root); err != nil {
		return err
	}
	if err := os.MkdirAll(root, 0o755); err != nil {
		return err
	}

	index, err := s.RenderIndex()
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(root, "index.html"), index, 0o644); err != nil {
		return err
	}

	if err := os.WriteFile(filepath.Join(root, "install.sh"), s.install, 0o755); err != nil {
		return err
	}

	return fs.WalkDir(s.static, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		target := filepath.Join(root, "static", path)
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}

		body, err := fs.ReadFile(s.static, path)
		if err != nil {
			return err
		}
		return os.WriteFile(target, body, 0o644)
	})
}

func ExportDir(root string) error {
	s, err := loadSite()
	if err != nil {
		return err
	}
	return s.ExportDir(root)
}
