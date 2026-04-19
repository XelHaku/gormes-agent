package site

import (
	"html/template"
	"net/http"
)

type Server struct {
	page      LandingPage
	templates *template.Template
}

func NewServer() (http.Handler, error) {
	templates, err := parseTemplates()
	if err != nil {
		return nil, err
	}

	srv := &Server{
		page:      DefaultPage(),
		templates: templates,
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/", srv.handleIndex)
	return mux, nil
}

func (s *Server) handleIndex(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := s.templates.ExecuteTemplate(w, "layout", s.page); err != nil {
		http.Error(w, "template render error", http.StatusInternalServerError)
		return
	}
}
