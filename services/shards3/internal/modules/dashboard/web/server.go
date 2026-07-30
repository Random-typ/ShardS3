package web

import (
	"bytes"
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"html/template"
	"io/fs"
	"net/http"
	"strings"
	"time"

	"shards3/services/shards3/internal/modules/dashboard"
)

//go:embed templates/*.gohtml static/**
var assetsFS embed.FS

type Server struct {
	service   *dashboard.Service
	templates *template.Template
	static    http.Handler
}

type pageView struct {
	Title   string
	Content template.HTML
	Buckets []dashboard.Bucket
}

type objectBrowserView struct {
	Bucket       string
	Prefix       string
	DisplayPath  string
	HasParent    bool
	ParentPrefix string
	Entries      []dashboard.ObjectEntry
}

type healthView struct {
	Ok        bool
	Message   string
	Error     string
	CheckedAt string
}

func NewServer(service *dashboard.Service) (*Server, error) {
	templates, err := template.New("dashboard").Funcs(template.FuncMap{
		"formatBytes": formatBytes,
	}).ParseFS(assetsFS, "templates/*.gohtml")
	if err != nil {
		return nil, fmt.Errorf("parse templates: %w", err)
	}

	staticFS, err := fs.Sub(assetsFS, "static")
	if err != nil {
		return nil, fmt.Errorf("prepare static files: %w", err)
	}

	return &Server{service: service, templates: templates, static: http.StripPrefix("/static/", http.FileServer(http.FS(staticFS)))}, nil
}

func (s *Server) Routes() http.Handler {
	mux := http.NewServeMux()
	mux.Handle("GET /static/", s.static)
	mux.HandleFunc("GET /", s.dashboardPage)
	mux.HandleFunc("GET /dashboard", s.dashboardPage)
	mux.HandleFunc("GET /buckets", s.bucketsPage)
	mux.HandleFunc("GET /backends", s.backendsPage)
	mux.HandleFunc("GET /settings", s.settingsPage)
	mux.HandleFunc("GET /health/longlived", s.healthLongLived)
	mux.HandleFunc("GET /fragments/health", s.healthFragment)
	mux.HandleFunc("GET /fragments/buckets", s.bucketsFragment)
	mux.HandleFunc("GET /fragments/buckets/{bucket}/objects", s.bucketObjectsFragment)
	mux.HandleFunc("POST /fragments/buckets", s.createBucket)
	mux.HandleFunc("POST /fragments/buckets/delete", s.deleteBucket)

	return mux
}

func (s *Server) dashboardPage(w http.ResponseWriter, r *http.Request) {
	s.renderPage(w, "dashboard_content", pageView{Title: "ShardS3 Dashboard", Buckets: s.service.ListBuckets()})
}

func (s *Server) bucketsPage(w http.ResponseWriter, r *http.Request) {
	s.renderPage(w, "buckets_content", pageView{Title: "Buckets - ShardS3", Buckets: s.service.ListBuckets()})
}

func (s *Server) backendsPage(w http.ResponseWriter, r *http.Request) {
	s.renderPage(w, "backends_content", pageView{Title: "Backends - ShardS3"})
}

func (s *Server) settingsPage(w http.ResponseWriter, r *http.Request) {
	s.renderPage(w, "settings_content", pageView{Title: "Settings - ShardS3"})
}

func (s *Server) renderPage(w http.ResponseWriter, contentTemplate string, view pageView) {
	var content bytes.Buffer
	if err := s.templates.ExecuteTemplate(&content, contentTemplate, view); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	view.Content = template.HTML(content.String())
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := s.templates.ExecuteTemplate(w, "base", view); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func (s *Server) healthFragment(w http.ResponseWriter, r *http.Request) {
	view := s.currentHealth(r.Context())

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := s.templates.ExecuteTemplate(w, "health_fragment", view); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func (s *Server) healthLongLived(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming not supported", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	ctx := r.Context()
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	s.writeHealthEvent(w, s.currentHealth(ctx))
	flusher.Flush()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.writeHealthEvent(w, s.currentHealth(ctx))
			flusher.Flush()
		}
	}
}

func (s *Server) writeHealthEvent(w http.ResponseWriter, view healthView) {
	payload, err := json.Marshal(view)
	if err != nil {
		return
	}

	fmt.Fprintf(w, "event: health\ndata: %s\n\n", payload)
}

func (s *Server) currentHealth(ctx context.Context) healthView {
	view := healthView{CheckedAt: time.Now().Format(time.RFC1123)}
	result, err := s.service.Health(ctx)
	if err != nil {
		view.Error = err.Error()
		return view
	}

	view.Ok = true
	view.Message = result.Message
	return view
}

func (s *Server) bucketsFragment(w http.ResponseWriter, r *http.Request) {
	view := pageView{Title: "ShardS3 Dashboard", Buckets: s.service.ListBuckets()}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := s.templates.ExecuteTemplate(w, "buckets_fragment", view); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func (s *Server) bucketObjectsFragment(w http.ResponseWriter, r *http.Request) {
	bucketName := strings.TrimSpace(r.PathValue("bucket"))
	prefix := strings.TrimSpace(r.URL.Query().Get("prefix"))

	entries, err := s.service.BrowseObjects(bucketName, prefix)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	view := objectBrowserView{
		Bucket:       bucketName,
		Prefix:       prefix,
		DisplayPath:  "/" + strings.TrimPrefix(prefix, "/"),
		HasParent:    prefix != "",
		ParentPrefix: parentPrefix(prefix),
		Entries:      entries,
	}

	if view.DisplayPath == "/" {
		view.DisplayPath = "/"
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := s.templates.ExecuteTemplate(w, "objects_fragment", view); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func (s *Server) createBucket(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if err := s.service.CreateBucket(strings.TrimSpace(r.FormValue("bucket_name"))); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	s.bucketsFragment(w, r)
}

func (s *Server) deleteBucket(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if err := s.service.DeleteBucket(strings.TrimSpace(r.FormValue("bucket_name"))); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	s.bucketsFragment(w, r)
}

func formatBytes(bytes int64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}

	div, exp := int64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}

	value := float64(bytes) / float64(div)
	units := []string{"KB", "MB", "GB", "TB", "PB"}
	return fmt.Sprintf("%.1f %s", value, units[exp])
}

func parentPrefix(prefix string) string {
	prefix = strings.TrimSpace(prefix)
	prefix = strings.TrimSuffix(prefix, "/")
	if prefix == "" {
		return ""
	}

	idx := strings.LastIndex(prefix, "/")
	if idx < 0 {
		return ""
	}

	return prefix[:idx+1]
}
