package server

import (
	"net/http"
	"net/url"
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

	if allowed[origin] {
		return true
	}

	originURL, err := url.Parse(origin)
	if err != nil {
		return false
	}

	requestHost := r.Host
	if requestHost == "" {
		return false
	}

	return strings.EqualFold(originURL.Host, requestHost)
}
