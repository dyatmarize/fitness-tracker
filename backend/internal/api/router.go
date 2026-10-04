package api

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/dyatmarize/fitness-tracker/backend/internal/config"
	syncpkg "github.com/dyatmarize/fitness-tracker/backend/internal/sync"
	"github.com/dyatmarize/fitness-tracker/backend/internal/coros"
)

// Server holds shared dependencies for HTTP handlers.
type Server struct {
	cfg    *config.Config
	pool   *pgxpool.Pool
	client *coros.Client
	worker *syncpkg.Worker
}

func New(cfg *config.Config, pool *pgxpool.Pool, client *coros.Client, worker *syncpkg.Worker) *Server {
	return &Server{cfg: cfg, pool: pool, client: client, worker: worker}
}

// Router builds the HTTP routes.
func (s *Server) Router() http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.Recoverer)
	r.Use(middleware.Logger)

	// Public routes: health check, login flow and status.
	r.Group(func(r chi.Router) {
		r.Get("/api/healthz", s.handleHealthz)
		r.Get("/api/auth/start", s.handleAuthStart)
		r.Get("/api/meta/status", s.handleMetaStatus)
		r.Post("/api/sync", s.handleSync)
	})

	return r
}

func (s *Server) handleHealthz(w http.ResponseWriter, r *http.Request) {
	if err := s.pool.Ping(r.Context()); err != nil {
		http.Error(w, "database unreachable", http.StatusServiceUnavailable)
		return
	}
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ok"))
}

// handleAuthStart begins the COROS OAuth flow and returns the login URL.
func (s *Server) handleAuthStart(w http.ResponseWriter, r *http.Request) {
	url, err := s.client.BeginLogin(r.Context())
	if err != nil {
		writeJSON(w, http.StatusConflict, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"auth_url": url})
}

// handleMetaStatus reports login and sync state.
func (s *Server) handleMetaStatus(w http.ResponseWriter, r *http.Request) {
	loggedIn, err := s.client.LoggedIn(r.Context())
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"logged_in": loggedIn,
		"sync":      s.worker.CurrentStatus(),
	})
}

// handleSync triggers a sync pass in the background.
func (s *Server) handleSync(w http.ResponseWriter, r *http.Request) {
	if !s.worker.Sync(r.Context()) {
		writeJSON(w, http.StatusAccepted, map[string]string{"status": "already running"})
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]string{"status": "started"})
}

// requireToken enforces the APP_TOKEN bearer token on protected routes.
func (s *Server) requireToken(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if s.cfg.AppToken != "" {
			h := r.Header.Get("Authorization")
			if !strings.HasPrefix(h, "Bearer ") || strings.TrimPrefix(h, "Bearer ") != s.cfg.AppToken {
				w.Header().Set("WWW-Authenticate", `Bearer realm="fitness-tracker"`)
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}
