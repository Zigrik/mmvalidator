package web

import (
	"embed"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io/fs"
	"net/http"
	"path"
	"strings"
	"time"

	"mmvalidator/internal/checker"
	"mmvalidator/internal/jobs"
	"mmvalidator/internal/parser"
)

//go:embed static/*
var static embed.FS

type Server struct {
	jobs                 *jobs.Manager
	configuredCredential bool
	validatorFor         func(string) checker.Validator
}

func New(m *jobs.Manager, configuredCredential bool, validatorFor func(string) checker.Validator) *Server {
	return &Server{jobs: m, configuredCredential: configuredCredential, validatorFor: validatorFor}
}
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/config", s.config)
	mux.HandleFunc("/api/jobs", s.create)
	mux.HandleFunc("/api/jobs/", s.job)
	assets, err := fs.Sub(static, "static")
	if err != nil {
		panic(err)
	}
	mux.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.FS(assets))))
	mux.HandleFunc("/", s.index)
	return withSecurity(mux)
}

func (s *Server) index(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		method(w)
		return
	}
	body, err := static.ReadFile("static/index.html")
	if err != nil {
		http.Error(w, "interface unavailable", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	_, _ = w.Write(body)
}
func (s *Server) config(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		method(w)
		return
	}
	reply(w, http.StatusOK, map[string]bool{"credentialConfigured": s.configuredCredential})
}
func (s *Server) create(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		method(w)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 100<<20)
	if err := r.ParseMultipartForm(100 << 20); err != nil {
		bad(w, "file is too large or malformed")
		return
	}
	var records []parser.InputRecord
	var err error
	if f, header, e := r.FormFile("file"); e == nil {
		defer f.Close()
		ext := strings.ToLower(path.Ext(header.Filename))
		if ext != ".csv" && ext != ".txt" {
			bad(w, "only .csv and .txt files are supported")
			return
		}
		if ext == ".csv" {
			records, err = parser.ParseCSV(f)
		} else {
			records, err = parser.ParseText(f)
		}
	} else {
		records, err = parser.ParseText(strings.NewReader(r.FormValue("text")))
	}
	if err != nil {
		bad(w, err.Error())
		return
	}
	if len(records) == 0 {
		bad(w, "provide at least one non-empty code")
		return
	}
	token := r.FormValue("credential")
	if !s.configuredCredential && token == "" {
		bad(w, "enter an API token")
		return
	}
	j := s.jobs.CreateWithValidator(records, s.validatorFor(token))
	reply(w, http.StatusAccepted, j.Snapshot(false))
}
func (s *Server) job(w http.ResponseWriter, r *http.Request) {
	tail := strings.TrimPrefix(r.URL.Path, "/api/jobs/")
	parts := strings.Split(tail, "/")
	if parts[0] == "" {
		http.NotFound(w, r)
		return
	}
	j, ok := s.jobs.Get(parts[0])
	if !ok {
		http.NotFound(w, r)
		return
	}
	if len(parts) == 1 {
		if r.Method == http.MethodDelete {
			if err := s.jobs.Cancel(parts[0]); err != nil {
				bad(w, err.Error())
				return
			}
			w.WriteHeader(http.StatusNoContent)
			return
		}
		if r.Method != http.MethodGet {
			method(w)
			return
		}
		reply(w, http.StatusOK, j.Snapshot(false))
		return
	}
	switch parts[1] {
	case "invalid":
		if r.Method != http.MethodGet {
			method(w)
			return
		}
		reply(w, http.StatusOK, j.Snapshot(true))
	case "invalid.csv":
		s.csv(w, r, j, false)
	case "errors.csv":
		s.csv(w, r, j, true)
	case "events":
		s.events(w, r, j)
	default:
		http.NotFound(w, r)
	}
}
func (s *Server) csv(w http.ResponseWriter, r *http.Request, j *jobs.Job, errs bool) {
	if r.Method != http.MethodGet {
		method(w)
		return
	}
	name := "invalid.csv"
	if errs {
		name = "check_errors.csv"
	}
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", name))
	w.Write([]byte{0xEF, 0xBB, 0xBF})
	jobs.WriteCSV(j.Snapshot(true), errs, csv.NewWriter(w))
}
func (s *Server) events(w http.ResponseWriter, r *http.Request, j *jobs.Job) {
	if r.Method != http.MethodGet {
		method(w)
		return
	}
	f, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unavailable", 500)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	tick := time.NewTicker(500 * time.Millisecond)
	defer tick.Stop()
	for {
		data, _ := json.Marshal(j.Snapshot(false))
		fmt.Fprintf(w, "data: %s\n\n", data)
		f.Flush()
		if state := j.Snapshot(false).State; state == jobs.Completed || state == jobs.CompletedWithErrors || state == jobs.Cancelled || state == jobs.Failed {
			return
		}
		select {
		case <-r.Context().Done():
			return
		case <-tick.C:
		}
	}
}
func reply(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
func bad(w http.ResponseWriter, msg string) {
	reply(w, http.StatusBadRequest, map[string]string{"error": msg})
}
func method(w http.ResponseWriter) {
	w.Header().Set("Allow", "GET, POST, DELETE")
	bad(w, "method not allowed")
}
func withSecurity(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "same-origin")
		next.ServeHTTP(w, r)
	})
}
