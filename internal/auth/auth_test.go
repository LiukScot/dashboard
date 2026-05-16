package auth

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/LiukScot/dashboard/internal/db"
)

func newTestService(t *testing.T) *Service {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "auth.sqlite")
	database, err := db.Open(dbPath)
	require.NoError(t, err, "open db")
	t.Cleanup(func() { _ = database.Close() })
	require.NoError(t, db.RunMigrations(database), "migrations")
	return NewService(database, 3600)
}

func TestLoginSuccessReturnsSession(t *testing.T) {
	t.Parallel()
	svc := newTestService(t)

	require.NoError(t, svc.CreateUser("alice@example.com", "correct horse"))

	sid, err := svc.Login("alice@example.com", "correct horse")
	require.NoError(t, err)
	assert.GreaterOrEqual(t, len(sid), 32, "session id length")

	user, err := svc.ValidateSession(sid)
	require.NoError(t, err)
	assert.Equal(t, "alice@example.com", user.Email)
}

func TestLoginRejectsWrongPassword(t *testing.T) {
	t.Parallel()
	svc := newTestService(t)

	require.NoError(t, svc.CreateUser("bob@example.com", "right"))

	_, err := svc.Login("bob@example.com", "wrong")
	assert.ErrorIs(t, err, ErrInvalidCredentials)
}

func TestLoginRejectsUnknownEmail(t *testing.T) {
	t.Parallel()
	svc := newTestService(t)
	// Burn-bcrypt timing path runs; only the error contract is asserted here.
	_, err := svc.Login("ghost@example.com", "anything")
	assert.ErrorIs(t, err, ErrInvalidCredentials)
}

func TestCreateUserRejectsLongPassword(t *testing.T) {
	t.Parallel()
	svc := newTestService(t)

	long := strings.Repeat("p", bcryptMaxPasswordBytes+1)
	err := svc.CreateUser("eve@example.com", long)
	assert.ErrorIs(t, err, ErrPasswordTooLong)
}

func TestValidateSessionUnknownReturnsNotFound(t *testing.T) {
	t.Parallel()
	svc := newTestService(t)

	_, err := svc.ValidateSession("nonexistent")
	assert.ErrorIs(t, err, ErrSessionNotFound)
}

func TestMaskSessionIDDoesNotLeakFullToken(t *testing.T) {
	t.Parallel()

	full := "abcdef0123456789abcdef0123456789"
	masked := maskSessionID(full)
	assert.NotContains(t, masked, full)
	assert.True(t, strings.HasPrefix(masked, "abcdef01"))
}

func TestMaskSessionIDShortInputReturnsRedacted(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "***", maskSessionID("short"))
	assert.Equal(t, "***", maskSessionID(""))
}

func TestLogoutRemovesSession(t *testing.T) {
	t.Parallel()
	svc := newTestService(t)

	require.NoError(t, svc.CreateUser("carol@example.com", "pw"))
	sid, err := svc.Login("carol@example.com", "pw")
	require.NoError(t, err)

	// Session exists before logout.
	_, err = svc.ValidateSession(sid)
	require.NoError(t, err)

	require.NoError(t, svc.Logout(sid))

	_, err = svc.ValidateSession(sid)
	assert.ErrorIs(t, err, ErrSessionNotFound)
}

func TestValidateSessionExpiredReturnsExpiredAndPrunes(t *testing.T) {
	t.Parallel()
	// TTL = -1s ensures the row is born expired; first validate must
	// return ErrSessionExpired AND delete the row, so the second call
	// returns ErrSessionNotFound.
	dbPath := filepath.Join(t.TempDir(), "auth.sqlite")
	database, err := db.Open(dbPath)
	require.NoError(t, err)
	t.Cleanup(func() { _ = database.Close() })
	require.NoError(t, db.RunMigrations(database))
	svc := NewService(database, -1)

	require.NoError(t, svc.CreateUser("dan@example.com", "pw"))
	sid, err := svc.Login("dan@example.com", "pw")
	require.NoError(t, err)

	_, err = svc.ValidateSession(sid)
	assert.ErrorIs(t, err, ErrSessionExpired)

	_, err = svc.ValidateSession(sid)
	assert.ErrorIs(t, err, ErrSessionNotFound, "expired session should be pruned")
}

func TestCreateUserRejectsDuplicateEmail(t *testing.T) {
	t.Parallel()
	svc := newTestService(t)

	require.NoError(t, svc.CreateUser("dup@example.com", "pw"))
	err := svc.CreateUser("dup@example.com", "other")
	assert.Error(t, err, "duplicate email must be rejected by UNIQUE constraint")
}
