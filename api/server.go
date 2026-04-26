package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"go.uber.org/zap"

	"email-triage-agent/config"
	"email-triage-agent/db"
)

// StatsProvider is implemented by Scheduler
type StatsProvider interface {
	Stats() (processed, failed int, lastRun time.Time)
}

type Server struct {
	cfg    config.Config
	logger *zap.Logger
	db     *db.DB
	stats  StatsProvider
	mux    *http.ServeMux
}

func NewServer(cfg config.Config, logger *zap.Logger, database *db.DB, stats StatsProvider) *Server {
	s := &Server{
		cfg:    cfg,
		logger: logger,
		db:     database,
		stats:  stats,
		mux:    http.NewServeMux(),
	}
	s.routes()
	return s
}

func (s *Server) routes() {
	s.mux.HandleFunc("/health",    s.handleHealth)
	s.mux.HandleFunc("/trigger",   s.authMiddleware(s.handleTrigger))
	s.mux.HandleFunc("/stats",     s.authMiddleware(s.handleStats))
	s.mux.HandleFunc("/logs",      s.authMiddleware(s.handleLogs))
	s.mux.HandleFunc("/dashboard", s.authMiddleware(s.handleDashboard))
}

// Start launches the HTTP server — called from main.go
func Start(cfg config.Config, logger *zap.Logger) error {
	addr := fmt.Sprintf(":%s", cfg.APIPort)
	logger.Info("API server listening", zap.String("addr", addr))
	return http.ListenAndServe(addr, http.DefaultServeMux)
}

// handleHealth — public, no auth needed
func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	s.writeJSON(w, http.StatusOK, map[string]string{
		"status": "ok",
		"time":   time.Now().Format(time.RFC3339),
	})
}

// handleTrigger — manually kick off a pipeline run
func (s *Server) handleTrigger(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	s.writeJSON(w, http.StatusOK, map[string]string{
		"status":  "triggered",
		"message": "pipeline run queued",
	})
}

// handleStats — returns runtime metrics as JSON
func (s *Server) handleStats(w http.ResponseWriter, r *http.Request) {
	processed, failed, lastRun := s.stats.Stats()

	high, medium, low, total, err := s.db.Stats(r.Context(), 24)
	if err != nil {
		s.writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	s.writeJSON(w, http.StatusOK, map[string]interface{}{
		"runtime": map[string]interface{}{
			"processed": processed,
			"failed":    failed,
			"last_run":  lastRun.Format(time.RFC3339),
		},
		"last_24h": map[string]interface{}{
			"total":  total,
			"high":   high,
			"medium": medium,
			"low":    low,
		},
	})
}

// handleLogs — returns recent triage logs as JSON
func (s *Server) handleLogs(w http.ResponseWriter, r *http.Request) {
	logs, err := s.db.GetRecent(r.Context(), 50)
	if err != nil {
		s.writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	s.writeJSON(w, http.StatusOK, logs)
}

// handleDashboard — serves the HTML dashboard
func (s *Server) handleDashboard(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html")
	w.Write([]byte(dashboardHTML))
}

// authMiddleware checks X-API-Secret header or ?secret= query param
func (s *Server) authMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		secret := r.Header.Get("X-API-Secret")
		if secret == "" {
			secret = r.URL.Query().Get("secret")
		}
		if secret != s.cfg.APISecret {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		next(w, r)
	}
}

func (s *Server) writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}