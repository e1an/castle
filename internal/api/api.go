package api

import (
	"context"
	"crypto/subtle"
	"embed"
	"encoding/json"
	"io/fs"
	"net/http"
	"os/exec"
	"path/filepath"
	"strconv"
	"time"

	"github.com/e1an/castle/config"
	"github.com/e1an/castle/internal/events"
)

//go:embed ui
var uiFiles embed.FS

type Server struct {
	mux        *http.ServeMux
	store      *events.Store
	recDir     string
	configPath string
	cfg        *config.Config
	reloadFn   func(*config.Config) error
	authUser   string
	authPass   string
}

func New(store *events.Store, recDir, configPath string, cfg *config.Config, reloadFn func(*config.Config) error) *Server {
	s := &Server{
		mux:        http.NewServeMux(),
		store:      store,
		recDir:     recDir,
		configPath: configPath,
		cfg:        cfg,
		reloadFn:   reloadFn,
	}
	s.routes()
	return s
}

// WithAuth enables HTTP Basic Auth.  Pass empty strings to disable.
func (s *Server) WithAuth(username, password string) *Server {
	s.authUser = username
	s.authPass = password
	return s
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if s.authUser != "" {
		u, p, ok := r.BasicAuth()
		if !ok || subtle.ConstantTimeCompare([]byte(u), []byte(s.authUser)) == 0 ||
			subtle.ConstantTimeCompare([]byte(p), []byte(s.authPass)) == 0 {
			w.Header().Set("WWW-Authenticate", `Basic realm="Castle"`)
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}
	}
	s.mux.ServeHTTP(w, r)
}

func (s *Server) routes() {
	s.mux.HandleFunc("GET /health", s.handleHealth)
	s.mux.HandleFunc("GET /api/events", s.handleListEvents)
	s.mux.HandleFunc("GET /api/config", s.handleGetConfig)
	s.mux.HandleFunc("PUT /api/config", s.handlePutConfig)
	s.mux.HandleFunc("POST /api/reload", s.handleReload)
	s.mux.HandleFunc("POST /api/test-stream", s.handleTestStream)
	s.mux.HandleFunc("GET /recordings/", s.handleServeRecording)

	sub, err := fs.Sub(uiFiles, "ui")
	if err != nil {
		panic(err)
	}
	fileServer := http.FileServer(http.FS(sub))
	s.mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		f, err := sub.Open(r.URL.Path)
		if err != nil {
			r.URL.Path = "/"
		} else {
			f.Close()
		}
		fileServer.ServeHTTP(w, r)
	})
}

func (s *Server) handleHealth(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

func (s *Server) handleListEvents(w http.ResponseWriter, r *http.Request) {
	camID := r.URL.Query().Get("camera_id")
	limit := 50
	if l := r.URL.Query().Get("limit"); l != "" {
		if n, err := strconv.Atoi(l); err == nil && n > 0 {
			limit = n
		}
	}

	evts, err := s.store.List(camID, limit)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(evts)
}

func (s *Server) handleGetConfig(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(s.cfg)
}

func (s *Server) handlePutConfig(w http.ResponseWriter, r *http.Request) {
	var incoming config.Config
	if err := json.NewDecoder(r.Body).Decode(&incoming); err != nil {
		http.Error(w, "invalid JSON: "+err.Error(), http.StatusBadRequest)
		return
	}

	if err := config.Save(s.configPath, &incoming); err != nil {
		http.Error(w, "save failed: "+err.Error(), http.StatusInternalServerError)
		return
	}

	if err := s.reloadFn(&incoming); err != nil {
		http.Error(w, "reload failed: "+err.Error(), http.StatusInternalServerError)
		return
	}

	s.cfg = &incoming
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

func (s *Server) handleReload(w http.ResponseWriter, _ *http.Request) {
	cfg, err := config.Load(s.configPath)
	if err != nil {
		http.Error(w, "load failed: "+err.Error(), http.StatusInternalServerError)
		return
	}

	if err := s.reloadFn(cfg); err != nil {
		http.Error(w, "reload failed: "+err.Error(), http.StatusInternalServerError)
		return
	}

	s.cfg = cfg
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

func (s *Server) handleTestStream(w http.ResponseWriter, r *http.Request) {
	var body struct {
		URL string `json:"url"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.URL == "" {
		http.Error(w, "missing url", http.StatusBadRequest)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 8*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "ffprobe",
		"-v", "error",
		"-rtsp_transport", "tcp",
		"-show_entries", "stream=codec_type,codec_name,width,height",
		"-of", "json",
		body.URL,
	)
	out, err := cmd.Output()
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadGateway)
		json.NewEncoder(w).Encode(map[string]string{"error": "stream unreachable"})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.Write(out)
}

func (s *Server) handleServeRecording(w http.ResponseWriter, r *http.Request) {
	rel := r.URL.Path[len("/recordings/"):]
	http.ServeFile(w, r, filepath.Join(s.recDir, rel))
}
