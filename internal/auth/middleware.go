package auth

import (
	"context"
	"net/http"
)

type contextKey string

const userContextKey contextKey = "user"

func Middleware(authSvc *Service) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			cookie, err := r.Cookie("DASHBOARD_SESSID")
			if err != nil {
				http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
				return
			}

			user, err := authSvc.ValidateSession(cookie.Value)
			if err != nil {
				http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
				return
			}

			ctx := context.WithValue(r.Context(), userContextKey, user)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func UserFromContext(ctx context.Context) *User {
	user, _ := ctx.Value(userContextKey).(*User)
	return user
}

// ValidateSessionFromCookie checks a session from raw cookie header (for WebSocket upgrades)
func ValidateSessionFromCookie(authSvc *Service, r *http.Request) (*User, error) {
	cookie, err := r.Cookie("DASHBOARD_SESSID")
	if err != nil {
		return nil, ErrSessionNotFound
	}
	return authSvc.ValidateSession(cookie.Value)
}
