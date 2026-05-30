package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/LiukScot/dashboard/internal/auth"
	"github.com/LiukScot/dashboard/internal/collectors"
	"github.com/LiukScot/dashboard/internal/config"
	"github.com/LiukScot/dashboard/internal/db"
)

// newTestServer builds a Server with a fresh ephemeral SQLite DB, optional
// collectors (caller supplies what the test exercises), and just enough of
// Server.routes() so endpoints can be hit through s.mux.
func newTestServer(t *testing.T, cfg *config.Config, withCollectors func(*Server)) *Server {
	t.Helper()
	if cfg == nil {
		cfg = &config.Config{SessionTTL: 3600}
	}
	dbPath := filepath.Join(t.TempDir(), "dashboard.sqlite")
	database, err := db.Open(dbPath)
	require.NoError(t, err)
	t.Cleanup(func() { _ = database.Close() })
	require.NoError(t, db.RunMigrations(database))

	srv := &Server{
		cfg:       cfg,
		authSvc:   auth.NewService(database, cfg.SessionTTL),
		mux:       http.NewServeMux(),
		startedAt: time.Now(),
	}
	if withCollectors != nil {
		withCollectors(srv)
	}
	return srv
}

// loginUser creates a user and returns the live session ID.
func loginUser(t *testing.T, srv *Server, email, password string) string {
	t.Helper()
	require.NoError(t, srv.authSvc.CreateUser(email, password))
	sid, err := srv.authSvc.Login(email, password)
	require.NoError(t, err)
	return sid
}

// --- /api/v1/auth/login -----------------------------------------------------

func TestHandleLoginRejectsOversizedBody(t *testing.T) {
	t.Parallel()
	srv := newTestServer(t, nil, nil)

	password := strings.Repeat("x", 5000)
	body := fmt.Sprintf(`{"email":"person@example.com","password":"%s"}`, password)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", strings.NewReader(body))
	res := httptest.NewRecorder()
	srv.handleLogin(res, req)

	assert.Equal(t, http.StatusBadRequest, res.Code)
}

func TestHandleLoginRejectsOversizedEmail(t *testing.T) {
	t.Parallel()
	srv := newTestServer(t, nil, nil)

	// 251 'a' chars + "@b.c" = 255 bytes, over the 254-byte RFC 5321 limit.
	email := strings.Repeat("a", 251) + "@b.c"
	body := fmt.Sprintf(`{"email":%q,"password":"pw"}`, email)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", strings.NewReader(body))
	res := httptest.NewRecorder()
	srv.handleLogin(res, req)

	assert.Equal(t, http.StatusBadRequest, res.Code)
}

func TestHandleLoginAllowsEmailAtMaxLengthBoundary(t *testing.T) {
	t.Parallel()
	srv := newTestServer(t, nil, nil)

	// 250 'a' chars + "@b.c" = 254 bytes (RFC 5321 max — must pass validation).
	email := strings.Repeat("a", 250) + "@b.c"
	body := fmt.Sprintf(`{"email":%q,"password":"pw"}`, email)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", strings.NewReader(body))
	res := httptest.NewRecorder()
	srv.handleLogin(res, req)

	// Validation passes; auth fails with 401 (unknown user), not 400.
	assert.Equal(t, http.StatusUnauthorized, res.Code)
}

func TestHandleLoginMalformedJSON(t *testing.T) {
	t.Parallel()
	srv := newTestServer(t, nil, nil)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", strings.NewReader(`{"email":`))
	res := httptest.NewRecorder()
	srv.handleLogin(res, req)

	assert.Equal(t, http.StatusBadRequest, res.Code)
}

func TestHandleLoginRejectsUnknownUser(t *testing.T) {
	t.Parallel()
	srv := newTestServer(t, nil, nil)

	body := `{"email":"ghost@example.com","password":"whatever"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", strings.NewReader(body))
	res := httptest.NewRecorder()
	srv.handleLogin(res, req)

	assert.Equal(t, http.StatusUnauthorized, res.Code)
	assert.NotContains(t, res.Header().Get("Set-Cookie"), "DASHBOARD_SESSID=",
		"failed login must not set a session cookie with a value")
}

func TestHandleLoginSetsCookieOnSuccess(t *testing.T) {
	t.Parallel()
	srv := newTestServer(t, nil, nil)
	require.NoError(t, srv.authSvc.CreateUser("ok@example.com", "pw"))

	body := `{"email":"ok@example.com","password":"pw"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", strings.NewReader(body))
	res := httptest.NewRecorder()
	srv.handleLogin(res, req)

	require.Equal(t, http.StatusOK, res.Code)
	setCookie := res.Header().Get("Set-Cookie")
	assert.Contains(t, setCookie, "DASHBOARD_SESSID=")
	assert.Contains(t, setCookie, "HttpOnly")
	assert.Contains(t, setCookie, "SameSite=Strict")
}

// --- /api/v1/auth/logout ----------------------------------------------------

func TestHandleLogoutClearsCookieAndSession(t *testing.T) {
	t.Parallel()
	srv := newTestServer(t, nil, nil)
	sid := loginUser(t, srv, "alice@example.com", "pw")

	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/logout", nil)
	req.AddCookie(&http.Cookie{Name: "DASHBOARD_SESSID", Value: sid})
	res := httptest.NewRecorder()
	srv.handleLogout(res, req)

	require.Equal(t, http.StatusOK, res.Code)
	// Cookie must be cleared (Max-Age=0 in net/http output).
	setCookie := res.Header().Get("Set-Cookie")
	assert.Contains(t, setCookie, "DASHBOARD_SESSID=")
	assert.Contains(t, setCookie, "Max-Age=0")

	// Session row must be gone.
	_, err := srv.authSvc.ValidateSession(sid)
	assert.ErrorIs(t, err, auth.ErrSessionNotFound)
}

func TestHandleLogoutWithoutCookieStillReturnsOK(t *testing.T) {
	t.Parallel()
	srv := newTestServer(t, nil, nil)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/logout", nil)
	res := httptest.NewRecorder()
	srv.handleLogout(res, req)

	assert.Equal(t, http.StatusOK, res.Code)
}

// --- /api/v1/auth/session ---------------------------------------------------

func TestHandleSessionReturnsAuthenticatedFalseWithoutCookie(t *testing.T) {
	t.Parallel()
	srv := newTestServer(t, nil, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/session", nil)
	res := httptest.NewRecorder()
	srv.handleSession(res, req)

	require.Equal(t, http.StatusOK, res.Code)

	var body struct {
		Authenticated bool `json:"authenticated"`
	}
	require.NoError(t, json.NewDecoder(res.Body).Decode(&body))
	assert.False(t, body.Authenticated)
}

func TestHandleSessionReturnsUserWhenAuthenticated(t *testing.T) {
	t.Parallel()
	srv := newTestServer(t, nil, nil)
	sid := loginUser(t, srv, "alice@example.com", "pw")

	req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/session", nil)
	req.AddCookie(&http.Cookie{Name: "DASHBOARD_SESSID", Value: sid})
	res := httptest.NewRecorder()
	srv.handleSession(res, req)

	require.Equal(t, http.StatusOK, res.Code)
	var body struct {
		Authenticated bool `json:"authenticated"`
		User          *struct {
			Email string `json:"email"`
		} `json:"user"`
	}
	require.NoError(t, json.NewDecoder(res.Body).Decode(&body))
	assert.True(t, body.Authenticated)
	require.NotNil(t, body.User)
	assert.Equal(t, "alice@example.com", body.User.Email)
}

// --- withAuth ---------------------------------------------------------------

func TestWithAuthRejectsMissingCookie(t *testing.T) {
	t.Parallel()
	srv := newTestServer(t, nil, nil)

	called := false
	handler := srv.withAuth(func(http.ResponseWriter, *http.Request) { called = true })

	req := httptest.NewRequest(http.MethodGet, "/api/v1/system/overview", nil)
	res := httptest.NewRecorder()
	handler(res, req)

	assert.Equal(t, http.StatusUnauthorized, res.Code)
	assert.False(t, called, "inner handler must not run when cookie is missing")
}

func TestWithAuthRejectsInvalidSession(t *testing.T) {
	t.Parallel()
	srv := newTestServer(t, nil, nil)

	called := false
	handler := srv.withAuth(func(http.ResponseWriter, *http.Request) { called = true })

	req := httptest.NewRequest(http.MethodGet, "/api/v1/system/overview", nil)
	req.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: "does-not-exist"})
	res := httptest.NewRecorder()
	handler(res, req)

	assert.Equal(t, http.StatusUnauthorized, res.Code)
	assert.False(t, called)
}

func TestWithAuthInjectsUserIntoContext(t *testing.T) {
	t.Parallel()
	srv := newTestServer(t, nil, nil)
	sid := loginUser(t, srv, "alice@example.com", "pw")

	var seenEmail string
	handler := srv.withAuth(func(w http.ResponseWriter, r *http.Request) {
		if u := auth.UserFromContext(r.Context()); u != nil {
			seenEmail = u.Email
		}
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/me", nil)
	req.AddCookie(&http.Cookie{Name: "DASHBOARD_SESSID", Value: sid})
	res := httptest.NewRecorder()
	handler(res, req)

	assert.Equal(t, http.StatusOK, res.Code)
	assert.Equal(t, "alice@example.com", seenEmail)
}

// --- /api/v1/auth/me --------------------------------------------------------

func TestHandleMeReturnsAuthenticatedUser(t *testing.T) {
	t.Parallel()
	srv := newTestServer(t, nil, nil)
	user := &auth.User{ID: 1, Email: "alice@example.com"}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/me", nil)
	ctx := auth.WithUser(req.Context(), user)
	req = req.WithContext(ctx)
	res := httptest.NewRecorder()
	srv.handleMe(res, req)

	require.Equal(t, http.StatusOK, res.Code)
	var got auth.User
	require.NoError(t, json.NewDecoder(res.Body).Decode(&got))
	assert.Equal(t, user.Email, got.Email)
}

// --- /healthz ---------------------------------------------------------------

func TestHandleHealthReturnsStatusAndUptime(t *testing.T) {
	t.Parallel()
	srv := &Server{startedAt: time.Now().Add(-2 * time.Second)}

	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	res := httptest.NewRecorder()
	srv.handleHealth(res, req)

	require.Equal(t, http.StatusOK, res.Code)
	var body struct {
		Status string `json:"status"`
		Uptime int64  `json:"uptime"`
	}
	require.NoError(t, json.NewDecoder(res.Body).Decode(&body))
	assert.Equal(t, "ok", body.Status)
	assert.GreaterOrEqual(t, body.Uptime, int64(1))
}

// --- /api/v1/system/history -------------------------------------------------

func TestHandleSystemHistoryUnavailableWhenNotConfigured(t *testing.T) {
	t.Parallel()
	srv := newTestServer(t, nil, nil) // sysHist == nil

	req := httptest.NewRequest(http.MethodGet, "/api/v1/system/history", nil)
	res := httptest.NewRecorder()
	srv.handleSystemHistory(res, req)

	assert.Equal(t, http.StatusServiceUnavailable, res.Code)
}

func TestHandleSystemHistoryRejectsInvalidRange(t *testing.T) {
	t.Parallel()
	dbPath := filepath.Join(t.TempDir(), "dashboard.sqlite")
	database, err := db.Open(dbPath)
	require.NoError(t, err)
	t.Cleanup(func() { _ = database.Close() })
	require.NoError(t, db.RunMigrations(database))

	srv := newTestServer(t, nil, func(s *Server) {
		s.sysHist = collectors.NewSystemHistory(database, nil, time.Minute)
	})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/system/history?range=bogus", nil)
	res := httptest.NewRecorder()
	srv.handleSystemHistory(res, req)

	assert.Equal(t, http.StatusBadRequest, res.Code)
}

// --- /api/v1/cron/* ---------------------------------------------------------

func TestHandleCronWeekUnavailableWhenCollectorNil(t *testing.T) {
	t.Parallel()
	srv := newTestServer(t, nil, nil) // cronColl == nil

	req := httptest.NewRequest(http.MethodGet, "/api/v1/cron/week", nil)
	res := httptest.NewRecorder()
	srv.handleCronWeek(res, req)

	assert.Equal(t, http.StatusServiceUnavailable, res.Code)
}

func TestHandleCronWeekRejectsInvalidStart(t *testing.T) {
	t.Parallel()
	dbPath := filepath.Join(t.TempDir(), "dashboard.sqlite")
	database, err := db.Open(dbPath)
	require.NoError(t, err)
	t.Cleanup(func() { _ = database.Close() })
	require.NoError(t, db.RunMigrations(database))

	srv := newTestServer(t, nil, func(s *Server) {
		s.cronColl = collectors.NewCronCollector(database, nil, "")
	})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/cron/week?start=nope", nil)
	res := httptest.NewRecorder()
	srv.handleCronWeek(res, req)

	assert.Equal(t, http.StatusBadRequest, res.Code)
}

func TestHandleHideCronJobMissingFingerprint(t *testing.T) {
	t.Parallel()
	dbPath := filepath.Join(t.TempDir(), "dashboard.sqlite")
	database, err := db.Open(dbPath)
	require.NoError(t, err)
	t.Cleanup(func() { _ = database.Close() })
	require.NoError(t, db.RunMigrations(database))

	srv := newTestServer(t, nil, func(s *Server) {
		s.cronColl = collectors.NewCronCollector(database, nil, "")
	})

	// No PathValue set: handler must 400.
	req := httptest.NewRequest(http.MethodPost, "/api/v1/cron/jobs//hide", nil)
	res := httptest.NewRecorder()
	srv.handleHideCronJob(res, req)

	assert.Equal(t, http.StatusBadRequest, res.Code)
}

func TestHandleResetHiddenCronJobsUnavailableWhenCollectorNil(t *testing.T) {
	t.Parallel()
	srv := newTestServer(t, nil, nil)

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/cron/hidden", nil)
	res := httptest.NewRecorder()
	srv.handleResetHiddenCronJobs(res, req)

	assert.Equal(t, http.StatusServiceUnavailable, res.Code)
}

func TestHandleHiddenCronJobCountReturnsZero(t *testing.T) {
	t.Parallel()
	dbPath := filepath.Join(t.TempDir(), "dashboard.sqlite")
	database, err := db.Open(dbPath)
	require.NoError(t, err)
	t.Cleanup(func() { _ = database.Close() })
	require.NoError(t, db.RunMigrations(database))

	srv := newTestServer(t, nil, func(s *Server) {
		s.cronColl = collectors.NewCronCollector(database, nil, "")
	})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/cron/hidden/count", nil)
	res := httptest.NewRecorder()
	srv.handleHiddenCronJobCount(res, req)

	require.Equal(t, http.StatusOK, res.Code)
	var body struct {
		Count int `json:"count"`
	}
	require.NoError(t, json.NewDecoder(res.Body).Decode(&body))
	assert.Equal(t, 0, body.Count)
}

// --- handleStatic path traversal --------------------------------------------

func TestHandleStaticServesFileInsidePublicDir(t *testing.T) {
	t.Parallel()
	publicDir := t.TempDir()
	// Pick a non-index filename: net/http.ServeFile auto-redirects
	// `/index.html` to `/` (301), which would mask the assertion.
	require.NoError(t, os.WriteFile(filepath.Join(publicDir, "robots.txt"), []byte("User-agent: *"), 0o644))

	srv := newTestServer(t, &config.Config{PublicDir: publicDir, SessionTTL: 3600}, nil)

	req := httptest.NewRequest(http.MethodGet, "/robots.txt", nil)
	res := httptest.NewRecorder()
	srv.handleStatic(res, req)

	assert.Equal(t, http.StatusOK, res.Code)
	assert.Contains(t, res.Body.String(), "User-agent")
}

func TestHandleStaticBlocksPathTraversal(t *testing.T) {
	t.Parallel()
	publicDir := t.TempDir()
	// Write a sentinel outside publicDir; the traversal attempt must
	// never reach it.
	parent := filepath.Dir(publicDir)
	secret := filepath.Join(parent, "secret.txt")
	require.NoError(t, os.WriteFile(secret, []byte("classified"), 0o644))
	t.Cleanup(func() { _ = os.Remove(secret) })

	srv := newTestServer(t, &config.Config{PublicDir: publicDir, SessionTTL: 3600}, nil)

	// `filepath.Clean("/" + r.URL.Path)` strips `..`, so a literal
	// `..%2f` is the only way the cleaned path stays outside publicDir.
	// The HTTP layer will decode that before the handler sees it.
	req := httptest.NewRequest(http.MethodGet, "/../secret.txt", nil)
	res := httptest.NewRecorder()
	srv.handleStatic(res, req)

	// Either 404 (cleaned to /secret.txt then not found inside publicDir)
	// or 200 serving fallback HTML; what must NOT happen is the body
	// containing the secret payload.
	assert.NotContains(t, res.Body.String(), "classified")
}

func TestHandleStaticReturns404ForAPIRoute(t *testing.T) {
	t.Parallel()
	srv := newTestServer(t, &config.Config{PublicDir: t.TempDir(), SessionTTL: 3600}, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/unknown", nil)
	res := httptest.NewRecorder()
	srv.handleStatic(res, req)

	assert.Equal(t, http.StatusNotFound, res.Code)
	assert.Contains(t, res.Body.String(), "api route not found")
}

func TestHandleStaticSPAFallback(t *testing.T) {
	t.Parallel()
	publicDir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(publicDir, "200.html"), []byte("<!doctype html><h1>spa</h1>"), 0o644))

	srv := newTestServer(t, &config.Config{PublicDir: publicDir, SessionTTL: 3600}, nil)

	req := httptest.NewRequest(http.MethodGet, "/some/spa/route", nil)
	res := httptest.NewRecorder()
	srv.handleStatic(res, req)

	assert.Equal(t, http.StatusOK, res.Code)
	assert.Contains(t, res.Body.String(), "spa")
}

// --- middleware -------------------------------------------------------------

func TestSecurityHeadersSet(t *testing.T) {
	t.Parallel()
	srv := &Server{cfg: &config.Config{}}
	wrapped := srv.securityHeaders(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	res := httptest.NewRecorder()
	wrapped.ServeHTTP(res, req)

	checks := map[string]string{
		"X-Content-Type-Options": "nosniff",
		"X-Frame-Options":        "DENY",
		"Referrer-Policy":        "no-referrer",
	}
	for header, want := range checks {
		assert.Equal(t, want, res.Header().Get(header), "header %s", header)
	}
}

func TestSecurityHeadersNoHSTSWhenCookieSecureDisabled(t *testing.T) {
	t.Parallel()
	srv := &Server{cfg: &config.Config{CookieSecure: false}}
	wrapped := srv.securityHeaders(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	res := httptest.NewRecorder()
	wrapped.ServeHTTP(res, req)

	assert.Empty(t, res.Header().Get("Strict-Transport-Security"),
		"HSTS must not be emitted over plain HTTP (CookieSecure=false)")
}

func TestSecurityHeadersSetsHSTSWhenCookieSecureEnabled(t *testing.T) {
	t.Parallel()
	srv := &Server{cfg: &config.Config{CookieSecure: true}}
	wrapped := srv.securityHeaders(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	res := httptest.NewRecorder()
	wrapped.ServeHTTP(res, req)

	hsts := res.Header().Get("Strict-Transport-Security")
	assert.Contains(t, hsts, "max-age=", "HSTS header must be set when running over HTTPS")
	assert.Contains(t, hsts, "includeSubDomains")
}

func TestCorsMiddlewareAllowsConfiguredOrigin(t *testing.T) {
	t.Parallel()
	srv := &Server{cfg: &config.Config{AllowedOrigins: "http://localhost:5173"}}
	wrapped := srv.corsMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/session", nil)
	req.Header.Set("Origin", "http://localhost:5173")
	res := httptest.NewRecorder()
	wrapped.ServeHTTP(res, req)

	assert.Equal(t, "http://localhost:5173", res.Header().Get("Access-Control-Allow-Origin"))
	assert.Equal(t, "true", res.Header().Get("Access-Control-Allow-Credentials"))
}

func TestCorsMiddlewarePreflightShortCircuits(t *testing.T) {
	t.Parallel()
	srv := &Server{cfg: &config.Config{AllowedOrigins: "http://localhost:5173"}}
	called := false
	wrapped := srv.corsMiddleware(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		called = true
	}))

	req := httptest.NewRequest(http.MethodOptions, "/api/v1/auth/login", nil)
	req.Header.Set("Origin", "http://localhost:5173")
	res := httptest.NewRecorder()
	wrapped.ServeHTTP(res, req)

	assert.Equal(t, http.StatusNoContent, res.Code)
	assert.False(t, called, "preflight must short-circuit before inner handler")
}

// --- origin allowlist (parse / check) ---------------------------------------

func TestRequestOriginAllowedAllowsConfiguredOrigin(t *testing.T) {
	t.Parallel()
	req := httptest.NewRequest(http.MethodGet, "http://127.0.0.1:4200/ws", nil)
	req.Host = "127.0.0.1:4200"
	req.Header.Set("Origin", "http://localhost:5173")

	allowed := parseAllowedOrigins("http://localhost:4200,http://localhost:5173")
	assert.True(t, requestOriginAllowed(req, allowed))
}

func TestRequestOriginAllowedAllowsSameHostOrigin(t *testing.T) {
	t.Parallel()
	req := httptest.NewRequest(http.MethodGet, "http://100.75.217.127:4200/ws", nil)
	req.Host = "100.75.217.127:4200"
	req.Header.Set("Origin", "http://100.75.217.127:4200")

	allowed := parseAllowedOrigins("http://localhost:4200,http://127.0.0.1:4200")
	assert.True(t, requestOriginAllowed(req, allowed))
}

func TestRequestOriginAllowedRejectsOtherOrigin(t *testing.T) {
	t.Parallel()
	req := httptest.NewRequest(http.MethodGet, "http://127.0.0.1:4200/ws", nil)
	req.Host = "127.0.0.1:4200"
	req.Header.Set("Origin", "http://evil.example")

	allowed := parseAllowedOrigins("http://localhost:4200,http://localhost:5173")
	assert.False(t, requestOriginAllowed(req, allowed))
}

func TestRequestOriginAllowedRejectsEmptyOrigin(t *testing.T) {
	t.Parallel()
	req := httptest.NewRequest(http.MethodGet, "/ws", nil)
	allowed := parseAllowedOrigins("http://localhost:4200")
	assert.False(t, requestOriginAllowed(req, allowed))
}

func TestRequestOriginAllowedRejectsMalformedOrigin(t *testing.T) {
	t.Parallel()
	req := httptest.NewRequest(http.MethodGet, "http://127.0.0.1/ws", nil)
	req.Host = "127.0.0.1:4200"
	// Control character makes url.Parse fail.
	req.Header.Set("Origin", "http://\x7f.example")

	allowed := parseAllowedOrigins("http://localhost:4200")
	assert.False(t, requestOriginAllowed(req, allowed))
}

func TestParseAllowedOriginsIgnoresBlanks(t *testing.T) {
	t.Parallel()
	got := parseAllowedOrigins(" http://a.example , , http://b.example,,")
	assert.True(t, got["http://a.example"])
	assert.True(t, got["http://b.example"])
	assert.Len(t, got, 2)
}

// --- end-to-end smoke against an httptest.Server ---------------------------

func TestServerEndToEndAuthFlow(t *testing.T) {
	t.Parallel()
	// Spin up a real Server with routes attached, hit it over HTTP to
	// exercise the routing layer + cookie round-trip.
	dbPath := filepath.Join(t.TempDir(), "dashboard.sqlite")
	database, err := db.Open(dbPath)
	require.NoError(t, err)
	t.Cleanup(func() { _ = database.Close() })
	require.NoError(t, db.RunMigrations(database))

	cfg := &config.Config{SessionTTL: 3600, AllowedOrigins: "http://localhost", PublicDir: t.TempDir()}
	authSvc := auth.NewService(database, cfg.SessionTTL)
	require.NoError(t, authSvc.CreateUser("alice@example.com", "pw"))

	srv := &Server{
		cfg:       cfg,
		authSvc:   authSvc,
		mux:       http.NewServeMux(),
		startedAt: time.Now(),
	}
	srv.mux.HandleFunc("POST /api/v1/auth/login", srv.handleLogin)
	srv.mux.HandleFunc("GET /api/v1/auth/session", srv.handleSession)
	srv.mux.HandleFunc("GET /healthz", srv.handleHealth)

	httpSrv := httptest.NewServer(srv.mux)
	t.Cleanup(httpSrv.Close)

	// Use a client with a cookie jar so the login cookie survives.
	jar, err := cookiejar.New(nil)
	require.NoError(t, err)
	client := &http.Client{
		Jar:     jar,
		Timeout: 5 * time.Second,
		Transport: &http.Transport{
			DialContext: (&net.Dialer{Timeout: 2 * time.Second}).DialContext,
		},
	}

	// Pre-login: unauthenticated.
	resp, err := client.Get(httpSrv.URL + "/api/v1/auth/session")
	require.NoError(t, err)
	t.Cleanup(func() { _ = resp.Body.Close() })
	var pre struct {
		Authenticated bool `json:"authenticated"`
	}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&pre))
	assert.False(t, pre.Authenticated)

	// Login.
	body := strings.NewReader(`{"email":"alice@example.com","password":"pw"}`)
	resp2, err := client.Post(httpSrv.URL+"/api/v1/auth/login", "application/json", body)
	require.NoError(t, err)
	t.Cleanup(func() { _ = resp2.Body.Close() })
	assert.Equal(t, http.StatusOK, resp2.StatusCode)

	// Post-login: authenticated, jar carried the cookie.
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	getReq, err := http.NewRequestWithContext(ctx, http.MethodGet, httpSrv.URL+"/api/v1/auth/session", nil)
	require.NoError(t, err)
	resp3, err := client.Do(getReq)
	require.NoError(t, err)
	t.Cleanup(func() { _ = resp3.Body.Close() })
	var post struct {
		Authenticated bool `json:"authenticated"`
	}
	require.NoError(t, json.NewDecoder(resp3.Body).Decode(&post))
	assert.True(t, post.Authenticated)
}
