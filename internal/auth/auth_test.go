package auth

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/LiukScot/dashboard/internal/db"
)

func newTestService(t *testing.T) *Service {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "auth.sqlite")
	database, err := db.Open(dbPath)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })
	if err := db.RunMigrations(database); err != nil {
		t.Fatalf("migrations: %v", err)
	}
	return NewService(database, 3600)
}

func TestLoginSuccessReturnsSession(t *testing.T) {
	t.Parallel()
	svc := newTestService(t)
	if err := svc.CreateUser("alice@example.com", "correct horse"); err != nil {
		t.Fatalf("create user: %v", err)
	}
	sid, err := svc.Login("alice@example.com", "correct horse")
	if err != nil {
		t.Fatalf("login: %v", err)
	}
	if len(sid) < 32 {
		t.Fatalf("session id looks too short: %q", sid)
	}
	user, err := svc.ValidateSession(sid)
	if err != nil {
		t.Fatalf("validate session: %v", err)
	}
	if user.Email != "alice@example.com" {
		t.Fatalf("expected alice@example.com, got %q", user.Email)
	}
}

func TestLoginRejectsWrongPassword(t *testing.T) {
	t.Parallel()
	svc := newTestService(t)
	if err := svc.CreateUser("bob@example.com", "right"); err != nil {
		t.Fatalf("create user: %v", err)
	}
	if _, err := svc.Login("bob@example.com", "wrong"); !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("expected ErrInvalidCredentials, got %v", err)
	}
}

func TestLoginRejectsUnknownEmail(t *testing.T) {
	t.Parallel()
	svc := newTestService(t)
	// Burn-bcrypt timing path runs; only the error contract is asserted here.
	if _, err := svc.Login("ghost@example.com", "anything"); !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("expected ErrInvalidCredentials, got %v", err)
	}
}

func TestCreateUserRejectsLongPassword(t *testing.T) {
	t.Parallel()
	svc := newTestService(t)
	long := strings.Repeat("p", bcryptMaxPasswordBytes+1)
	if err := svc.CreateUser("eve@example.com", long); !errors.Is(err, ErrPasswordTooLong) {
		t.Fatalf("expected ErrPasswordTooLong, got %v", err)
	}
}

func TestValidateSessionUnknownReturnsNotFound(t *testing.T) {
	t.Parallel()
	svc := newTestService(t)
	if _, err := svc.ValidateSession("nonexistent"); !errors.Is(err, ErrSessionNotFound) {
		t.Fatalf("expected ErrSessionNotFound, got %v", err)
	}
}

func TestMaskSessionIDDoesNotLeakFullToken(t *testing.T) {
	t.Parallel()
	full := "abcdef0123456789abcdef0123456789"
	masked := maskSessionID(full)
	if strings.Contains(masked, full) {
		t.Fatalf("masked output %q contains full token", masked)
	}
	if !strings.HasPrefix(masked, "abcdef01") {
		t.Fatalf("expected prefix retained, got %q", masked)
	}
}
