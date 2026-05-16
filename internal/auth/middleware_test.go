package auth

import (
	"context"
	"net/http"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/LiukScot/dashboard/internal/db"
)

func TestWithUserAndUserFromContextRoundTrip(t *testing.T) {
	t.Parallel()

	want := &User{ID: 42, Email: "alice@example.com"}
	ctx := WithUser(context.Background(), want)

	got := UserFromContext(ctx)
	require.NotNil(t, got)
	assert.Equal(t, want.ID, got.ID)
	assert.Equal(t, want.Email, got.Email)
}

func TestUserFromContextReturnsNilWhenAbsent(t *testing.T) {
	t.Parallel()

	got := UserFromContext(context.Background())
	assert.Nil(t, got)
}

func TestUserFromContextReturnsNilOnWrongType(t *testing.T) {
	t.Parallel()
	// The context key is package-private, so we can't poison it from
	// outside. We can still confirm that an unrelated value doesn't
	// surface as a User: passing a foreign context to UserFromContext
	// should yield nil.
	ctx := context.WithValue(context.Background(), struct{ k string }{"other"}, "not-a-user")
	got := UserFromContext(ctx)
	assert.Nil(t, got)
}

func newMiddlewareTestService(t *testing.T, ttlSeconds int) *Service {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "auth.sqlite")
	database, err := db.Open(dbPath)
	require.NoError(t, err)
	t.Cleanup(func() { _ = database.Close() })
	require.NoError(t, db.RunMigrations(database))
	return NewService(database, ttlSeconds)
}

func TestValidateSessionFromCookieMissingCookie(t *testing.T) {
	t.Parallel()
	svc := newMiddlewareTestService(t, 3600)

	req, err := http.NewRequest(http.MethodGet, "/ws", nil)
	require.NoError(t, err)

	user, err := ValidateSessionFromCookie(svc, req)
	assert.Nil(t, user)
	assert.ErrorIs(t, err, ErrSessionNotFound)
}

func TestValidateSessionFromCookieInvalidSession(t *testing.T) {
	t.Parallel()
	svc := newMiddlewareTestService(t, 3600)

	req, err := http.NewRequest(http.MethodGet, "/ws", nil)
	require.NoError(t, err)
	req.AddCookie(&http.Cookie{Name: "DASHBOARD_SESSID", Value: "does-not-exist"})

	user, err := ValidateSessionFromCookie(svc, req)
	assert.Nil(t, user)
	assert.ErrorIs(t, err, ErrSessionNotFound)
}

func TestValidateSessionFromCookieValid(t *testing.T) {
	t.Parallel()
	svc := newMiddlewareTestService(t, 3600)

	require.NoError(t, svc.CreateUser("alice@example.com", "pw"))
	sid, err := svc.Login("alice@example.com", "pw")
	require.NoError(t, err)

	req, err := http.NewRequest(http.MethodGet, "/ws", nil)
	require.NoError(t, err)
	req.AddCookie(&http.Cookie{Name: "DASHBOARD_SESSID", Value: sid})

	user, err := ValidateSessionFromCookie(svc, req)
	require.NoError(t, err)
	require.NotNil(t, user)
	assert.Equal(t, "alice@example.com", user.Email)
}

func TestValidateSessionFromCookieExpired(t *testing.T) {
	t.Parallel()
	// Born-expired session (TTL = -1s).
	svc := newMiddlewareTestService(t, -1)

	require.NoError(t, svc.CreateUser("alice@example.com", "pw"))
	sid, err := svc.Login("alice@example.com", "pw")
	require.NoError(t, err)

	req, err := http.NewRequest(http.MethodGet, "/ws", nil)
	require.NoError(t, err)
	req.AddCookie(&http.Cookie{Name: "DASHBOARD_SESSID", Value: sid})

	user, err := ValidateSessionFromCookie(svc, req)
	assert.Nil(t, user)
	assert.ErrorIs(t, err, ErrSessionExpired)
}
