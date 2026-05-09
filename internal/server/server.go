package server

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/LiukScot/dashboard/internal/auth"
	"github.com/LiukScot/dashboard/internal/collectors"
	"github.com/LiukScot/dashboard/internal/config"
)

type Server struct {
	cfg        *config.Config
	authSvc    *auth.Service
	sysColl    *collectors.SystemCollector
	sysHist    *collectors.SystemHistory
	dockerColl *collectors.DockerCollector
	f2bColl    *collectors.Fail2BanCollector
	logColl    *collectors.LogCollector
	cronColl   *collectors.CronCollector
	wsHandler  *WSHandler
	mux        *http.ServeMux
}

func New(cfg *config.Config, authSvc *auth.Service,
	sysColl *collectors.SystemCollector,
	sysHist *collectors.SystemHistory,
	dockerColl *collectors.DockerCollector,
	f2bColl *collectors.Fail2BanCollector,
	logColl *collectors.LogCollector,
	cronColl *collectors.CronCollector,
) *Server {
	hub := NewHub()
	wsHandler := NewWSHandler(hub, authSvc, sysColl, dockerColl, cfg)

	s := &Server{
		cfg:        cfg,
		authSvc:    authSvc,
		sysColl:    sysColl,
		sysHist:    sysHist,
		dockerColl: dockerColl,
		f2bColl:    f2bColl,
		logColl:    logColl,
		cronColl:   cronColl,
		wsHandler:  wsHandler,
		mux:        http.NewServeMux(),
	}

	s.routes()
	return s
}

func (s *Server) routes() {
	// Auth routes (no middleware)
	s.mux.HandleFunc("POST /api/v1/auth/login", s.handleLogin)
	s.mux.HandleFunc("POST /api/v1/auth/logout", s.handleLogout)
	s.mux.HandleFunc("GET /api/v1/auth/session", s.handleSession)
	s.mux.HandleFunc("GET /api/v1/auth/me", s.withAuth(s.handleMe))

	// System routes
	s.mux.HandleFunc("GET /api/v1/system/overview", s.withAuth(s.handleSystemOverview))
	s.mux.HandleFunc("GET /api/v1/system/cpu-history", s.withAuth(s.handleCPUHistory))
	s.mux.HandleFunc("GET /api/v1/system/history", s.withAuth(s.handleSystemHistory))
	s.mux.HandleFunc("GET /api/v1/system/network", s.withAuth(s.handleNetwork))

	// Docker routes
	s.mux.HandleFunc("GET /api/v1/docker/containers", s.withAuth(s.handleDockerContainers))

	// Security routes
	s.mux.HandleFunc("GET /api/v1/security/fail2ban", s.withAuth(s.handleFail2Ban))
	s.mux.HandleFunc("GET /api/v1/security/fail2ban/bans", s.withAuth(s.handleFail2BanBans))
	s.mux.HandleFunc("GET /api/v1/security/logs", s.withAuth(s.handleLogs))

	// Cron routes
	s.mux.HandleFunc("GET /api/v1/cron/week", s.withAuth(s.handleCronWeek))
	s.mux.HandleFunc("POST /api/v1/cron/jobs/{fingerprint}/hide", s.withAuth(s.handleHideCronJob))
	s.mux.HandleFunc("DELETE /api/v1/cron/hidden", s.withAuth(s.handleResetHiddenCronJobs))
	s.mux.HandleFunc("GET /api/v1/cron/hidden/count", s.withAuth(s.handleHiddenCronJobCount))

	// WebSocket
	s.mux.HandleFunc("GET /ws", s.wsHandler.HandleUpgrade)

	// Static files (SPA fallback)
	s.mux.HandleFunc("/", s.handleStatic)
}

func (s *Server) Start() error {
	// Start WebSocket broadcast
	s.wsHandler.StartBroadcastLoop(3 * time.Second)

	// Initial collection to populate data
	s.sysColl.Collect()
	s.sysColl.CollectNetwork()

	addr := fmt.Sprintf("%s:%d", s.cfg.Host, s.cfg.Port)
	log.Printf("dashboard listening on %s", addr)

	srv := &http.Server{
		Addr:              addr,
		Handler:           s.securityHeaders(s.corsMiddleware(s.mux)),
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       120 * time.Second,
	}
	return srv.ListenAndServe()
}

// securityHeaders sets baseline response headers that harden the SPA + cookie
// session against XSS-driven clickjacking and MIME sniffing. CSP is left out
// because the SPA bundle still inlines styles via Tailwind; add when fixed.
func (s *Server) securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("X-Frame-Options", "DENY")
		h.Set("Referrer-Policy", "no-referrer")
		next.ServeHTTP(w, r)
	})
}

// CORS middleware
func (s *Server) corsMiddleware(next http.Handler) http.Handler {
	allowed := parseAllowedOrigins(s.cfg.AllowedOrigins)

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		if requestOriginAllowed(r, allowed) {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Access-Control-Allow-Credentials", "true")
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		}

		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}

		next.ServeHTTP(w, r)
	})
}

// Auth wrapper — validates session and injects user into request context
func (s *Server) withAuth(handler http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cookie, err := r.Cookie("DASHBOARD_SESSID")
		if err != nil {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
			return
		}

		user, err := s.authSvc.ValidateSession(cookie.Value)
		if err != nil {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
			return
		}

		ctx := auth.WithUser(r.Context(), user)
		handler(w, r.WithContext(ctx))
	}
}

// --- Auth handlers ---

func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 4096)
	var body struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid body"})
		return
	}

	sid, err := s.authSvc.Login(body.Email, body.Password)
	if err != nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid credentials"})
		return
	}

	http.SetCookie(w, &http.Cookie{
		Name:     "DASHBOARD_SESSID",
		Value:    sid,
		Path:     "/",
		HttpOnly: true,
		Secure:   s.cfg.CookieSecure,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   s.cfg.SessionTTL,
	})

	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
	cookie, err := r.Cookie("DASHBOARD_SESSID")
	if err == nil {
		s.authSvc.Logout(cookie.Value)
	}

	http.SetCookie(w, &http.Cookie{
		Name:     "DASHBOARD_SESSID",
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		Secure:   s.cfg.CookieSecure,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   -1,
	})

	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) handleSession(w http.ResponseWriter, r *http.Request) {
	response := map[string]any{"authenticated": false}

	cookie, err := r.Cookie("DASHBOARD_SESSID")
	if err != nil {
		writeJSON(w, http.StatusOK, response)
		return
	}

	user, err := s.authSvc.ValidateSession(cookie.Value)
	if err != nil {
		writeJSON(w, http.StatusOK, response)
		return
	}

	response["authenticated"] = true
	response["user"] = user
	writeJSON(w, http.StatusOK, response)
}

func (s *Server) handleMe(w http.ResponseWriter, r *http.Request) {
	user := auth.UserFromContext(r.Context())
	writeJSON(w, http.StatusOK, user)
}

// --- System handlers ---

func (s *Server) handleSystemOverview(w http.ResponseWriter, r *http.Request) {
	metrics, err := s.sysColl.Collect()
	if err != nil {
		log.Printf("system overview error: %v", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to collect system metrics"})
		return
	}
	writeJSON(w, http.StatusOK, metrics)
}

func (s *Server) handleCPUHistory(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, s.sysColl.History())
}

var historyRanges = map[string]time.Duration{
	"1h":  time.Hour,
	"6h":  6 * time.Hour,
	"24h": 24 * time.Hour,
	"7d":  7 * 24 * time.Hour,
	"30d": 30 * 24 * time.Hour,
}

func (s *Server) handleSystemHistory(w http.ResponseWriter, r *http.Request) {
	if s.sysHist == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "history unavailable"})
		return
	}
	rangeKey := r.URL.Query().Get("range")
	if rangeKey == "" {
		rangeKey = "24h"
	}
	dur, ok := historyRanges[rangeKey]
	if !ok {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid range"})
		return
	}
	samples, err := s.sysHist.Query(dur)
	if err != nil {
		log.Printf("system history error: %v", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to load history"})
		return
	}
	writeJSON(w, http.StatusOK, samples)
}

func (s *Server) handleNetwork(w http.ResponseWriter, r *http.Request) {
	metrics, err := s.sysColl.CollectNetwork()
	if err != nil {
		log.Printf("network metrics error: %v", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to collect network metrics"})
		return
	}
	writeJSON(w, http.StatusOK, metrics)
}

// --- Docker handlers ---

func (s *Server) handleDockerContainers(w http.ResponseWriter, r *http.Request) {
	containers, err := s.dockerColl.ListContainers()
	if err != nil {
		log.Printf("docker containers error: %v", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to list containers"})
		return
	}
	writeJSON(w, http.StatusOK, containers)
}

// --- Security handlers ---

func (s *Server) handleFail2Ban(w http.ResponseWriter, r *http.Request) {
	status, err := s.f2bColl.GetStatus()
	if err != nil {
		log.Printf("fail2ban status error: %v", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to get fail2ban status"})
		return
	}
	writeJSON(w, http.StatusOK, status)
}

func (s *Server) handleFail2BanBans(w http.ResponseWriter, r *http.Request) {
	limit := 50
	if l := r.URL.Query().Get("limit"); l != "" {
		if n, err := strconv.Atoi(l); err == nil && n > 0 {
			limit = n
		}
	}
	if limit > 1000 {
		limit = 1000
	}

	events, err := s.f2bColl.GetRecentBans(limit)
	if err != nil {
		log.Printf("fail2ban bans error: %v", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to get recent bans"})
		return
	}
	writeJSON(w, http.StatusOK, events)
}

func (s *Server) handleLogs(w http.ResponseWriter, r *http.Request) {
	unit := r.URL.Query().Get("unit")
	priority := -1
	if p := r.URL.Query().Get("priority"); p != "" {
		if n, err := strconv.Atoi(p); err == nil {
			priority = n
		}
	}
	limit := 100
	if l := r.URL.Query().Get("limit"); l != "" {
		if n, err := strconv.Atoi(l); err == nil && n > 0 {
			limit = n
		}
	}
	if limit > 1000 {
		limit = 1000
	}

	entries, err := s.logColl.GetLogs(unit, priority, limit)
	if err != nil {
		log.Printf("logs error: %v", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to get logs"})
		return
	}
	writeJSON(w, http.StatusOK, entries)
}

// --- Cron handlers ---

func (s *Server) handleCronWeek(w http.ResponseWriter, r *http.Request) {
	if s.cronColl == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "cron collector unavailable"})
		return
	}

	start := time.Now()
	if raw := r.URL.Query().Get("start"); raw != "" {
		parsed, err := time.ParseInLocation("2006-01-02", raw, time.Local)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid start date"})
			return
		}
		start = parsed
	}

	week, err := s.cronColl.Week(start)
	if err != nil {
		log.Printf("cron week error: %v", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to load cron week"})
		return
	}
	writeJSON(w, http.StatusOK, week)
}

func (s *Server) handleHideCronJob(w http.ResponseWriter, r *http.Request) {
	if s.cronColl == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "cron collector unavailable"})
		return
	}
	fingerprint := strings.TrimSpace(r.PathValue("fingerprint"))
	if fingerprint == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "missing cron job id"})
		return
	}
	if err := s.cronColl.HideJob(fingerprint); err != nil {
		log.Printf("hide cron job error: %v", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to hide cron job"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) handleResetHiddenCronJobs(w http.ResponseWriter, r *http.Request) {
	if s.cronColl == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "cron collector unavailable"})
		return
	}
	if err := s.cronColl.ResetHiddenJobs(); err != nil {
		log.Printf("reset hidden cron jobs error: %v", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to reset hidden cron jobs"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) handleHiddenCronJobCount(w http.ResponseWriter, r *http.Request) {
	if s.cronColl == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "cron collector unavailable"})
		return
	}
	count, err := s.cronColl.HiddenJobCount()
	if err != nil {
		log.Printf("hidden cron job count error: %v", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to count hidden cron jobs"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]int{"count": count})
}

// --- Static file serving (SPA) ---

func (s *Server) handleStatic(w http.ResponseWriter, r *http.Request) {
	if strings.HasPrefix(r.URL.Path, "/api/") {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "api route not found"})
		return
	}

	publicDir := s.cfg.PublicDir

	// Sanitize path to prevent directory traversal
	cleaned := filepath.Clean("/" + r.URL.Path)
	filePath := filepath.Join(publicDir, cleaned)
	absPublic, errPub := filepath.Abs(publicDir)
	absFile, errFile := filepath.Abs(filePath)
	if errPub != nil || errFile != nil {
		log.Printf("static abs path resolution failed: pub=%v file=%v", errPub, errFile)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	if !strings.HasPrefix(absFile, absPublic+string(os.PathSeparator)) && absFile != absPublic {
		http.NotFound(w, r)
		return
	}

	// Try to serve the requested file
	if info, err := os.Stat(filePath); err == nil && !info.IsDir() {
		http.ServeFile(w, r, filePath)
		return
	}

	// SPA fallback: serve 200.html or index.html
	for _, fallback := range []string{"200.html", "index.html"} {
		fp := filepath.Join(publicDir, fallback)
		if _, err := os.Stat(fp); err == nil {
			http.ServeFile(w, r, fp)
			return
		}
	}

	// If no frontend build exists, serve a placeholder
	if _, err := fs.Stat(os.DirFS(publicDir), "."); err != nil {
		w.Header().Set("Content-Type", "text/html")
		w.Write([]byte("<h1>Dashboard</h1><p>Frontend not built yet. Run <code>cd frontend && bun run build</code></p>"))
		return
	}

	http.NotFound(w, r)
}

func writeJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(data); err != nil {
		log.Printf("writeJSON encode error (status=%d): %v", status, err)
	}
}
