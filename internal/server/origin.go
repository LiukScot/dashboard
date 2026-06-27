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

// requestOriginAllowed returns true when the request Origin header is either in
// the explicit allowlist or matches the same host as the server. When
// requireHTTPS is true the same-host fallback additionally rejects http: origins
// to prevent cross-protocol requests when the app is running behind TLS.
func requestOriginAllowed(r *http.Request, allowed map[string]bool, requireHTTPS bool) bool {
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

	if !strings.EqualFold(originURL.Host, requestHost) {
		return false
	}
	if requireHTTPS && originURL.Scheme != "https" {
		return false
	}
	return true
}
