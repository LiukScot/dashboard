package auth

import (
	"context"
	"net/http"
)

type contextKey string

const (
	userContextKey  contextKey = "user"
	SessionCookieName          = "DASHBOARD_SESSID"
)

// WithUser stores a User in the request context.
func WithUser(ctx context.Context, user *User) context.Context {
	return context.WithValue(ctx, userContextKey, user)
}

// UserFromContext retrieves the User from a request context.
func UserFromContext(ctx context.Context) *User {
	user, _ := ctx.Value(userContextKey).(*User)
	return user
}

// ValidateSessionFromCookie checks a session from raw cookie header (for WebSocket upgrades)
func ValidateSessionFromCookie(authSvc *Service, r *http.Request) (*User, error) {
	cookie, err := r.Cookie(SessionCookieName)
	if err != nil {
		return nil, ErrSessionNotFound
	}
	return authSvc.ValidateSession(cookie.Value)
}
