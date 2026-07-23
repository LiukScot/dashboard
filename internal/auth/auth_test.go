package auth

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/bcrypt"

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

func TestCreateUserUsesWorkFactorCost(t *testing.T) {
	t.Parallel()
	svc := newTestService(t)

	require.NoError(t, svc.CreateUser("cost@example.com", "pw"))

	var hash string
	require.NoError(t, svc.db.QueryRow(
		"SELECT password_hash FROM users WHERE email = ?", "cost@example.com",
	).Scan(&hash))

	cost, err := bcrypt.Cost([]byte(hash))
	require.NoError(t, err)
	assert.Equal(t, bcryptWorkFactor, cost, "new users hashed at current work factor")
}

func TestLoginUpgradesWeakHashLazily(t *testing.T) {
	t.Parallel()
	svc := newTestService(t)

	// Seed a user whose hash uses an outdated, weaker cost (pre-upgrade).
	weakCost := bcryptWorkFactor - 2
	weakHash, err := bcrypt.GenerateFromPassword([]byte("correct horse"), weakCost)
	require.NoError(t, err)
	_, err = svc.db.Exec(
		"INSERT INTO users (email, password_hash) VALUES (?, ?)",
		"legacy@example.com", string(weakHash),
	)
	require.NoError(t, err)

	// A successful login must transparently re-hash at the new work factor.
	_, err = svc.Login("legacy@example.com", "correct horse")
	require.NoError(t, err)

	var stored string
	require.NoError(t, svc.db.QueryRow(
		"SELECT password_hash FROM users WHERE email = ?", "legacy@example.com",
	).Scan(&stored))

	cost, err := bcrypt.Cost([]byte(stored))
	require.NoError(t, err)
	assert.Equal(t, bcryptWorkFactor, cost, "weak hash upgraded on login")
	assert.NotEqual(t, string(weakHash), stored, "stored hash actually changed")

	// The upgraded hash must still verify the same password.
	_, err = svc.Login("legacy@example.com", "correct horse")
	assert.NoError(t, err, "login works after re-hash")
}

func TestLoginDoesNotRehashCurrentCost(t *testing.T) {
	t.Parallel()
	svc := newTestService(t)

	require.NoError(t, svc.CreateUser("stable@example.com", "pw"))

	var before string
	require.NoError(t, svc.db.QueryRow(
		"SELECT password_hash FROM users WHERE email = ?", "stable@example.com",
	).Scan(&before))

	_, err := svc.Login("stable@example.com", "pw")
	require.NoError(t, err)

	var after string
	require.NoError(t, svc.db.QueryRow(
		"SELECT password_hash FROM users WHERE email = ?", "stable@example.com",
	).Scan(&after))

	assert.Equal(t, before, after, "hash already at work factor is not rewritten")
}

func TestCreateUserRejectsDuplicateEmail(t *testing.T) {
	t.Parallel()
	svc := newTestService(t)

	require.NoError(t, svc.CreateUser("dup@example.com", "pw"))
	err := svc.CreateUser("dup@example.com", "other")
	assert.Error(t, err, "duplicate email must be rejected by UNIQUE constraint")
}
