package server

import (
	"net/http"
	"strings"
)

func parseAllowedOrigins(origins string) map[string]bool {
	allowed := make(map[string]bool)
	for _, origin := range strings.Split(origins, ",") {
		origin = strings.TrimSpace(origin)
		if origin == "" {
			continue
		}
		allowed[origin] = true
	}
	return allowed
}

func requestOriginAllowed(r *http.Request, allowed map[string]bool) bool {
	origin := strings.TrimSpace(r.Header.Get("Origin"))
	if origin == "" {
		return false
	}

	return allowed[origin]
}
