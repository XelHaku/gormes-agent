package site

import (
	"net/http"
)

type Server struct {
	site *Site
}

func NewServer() (http.Handler, error) {
	site, err := loadSite()
	if err != nil {
		return nil, err
	}

	srv := &Server{
		site: site,
	}

	mux := http.NewServeMux()
	mux.Handle("/static/", http.StripPrefix("/static/", http.FileServerFS(site.static)))
	mux.HandleFunc("/install.sh", srv.handleInstall)
	mux.HandleFunc("/", srv.handleIndex)
	return mux, nil
}

func (s *Server) handleInstall(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/x-shellscript; charset=utf-8")
	w.Header().Set("Cache-Control", "public, max-age=300")
	_, _ = w.Write(s.site.InstallScript())
}

func (s *Server) handleIndex(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	body, err := s.site.RenderIndex()
	if err != nil {
		http.Error(w, "template render error", http.StatusInternalServerError)
		return
	}
	_, _ = w.Write(body)
}
