package server

import (
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// scriptTagRe captures <script ...>BODY</script>. The leading group keeps the
// opening tag attributes so we can decide whether the tag is inline (no `src`).
// (?is): case-insensitive + dot matches newline (script bodies span lines).
var scriptTagRe = regexp.MustCompile(`(?is)<script(\s[^>]*)?>(.*?)</script>`)

// scanInlineScriptHashes walks publicDir, parses every *.html file, and returns
// the base64-encoded SHA-256 of each inline <script> body it finds.
//
// The browser hashes the raw bytes between the opening tag's `>` and the
// closing `</script>`, including surrounding whitespace — we must hash the
// exact same substring or the CSP entry will not match.
func scanInlineScriptHashes(publicDir string) ([]string, error) {
	if publicDir == "" {
		return nil, nil
	}
	info, err := os.Stat(publicDir)
	if err != nil || !info.IsDir() {
		return nil, nil
	}

	seen := make(map[string]struct{})
	walkErr := filepath.WalkDir(publicDir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		ext := strings.ToLower(filepath.Ext(path))
		if ext != ".html" && ext != ".htm" {
			return nil
		}
		data, readErr := os.ReadFile(path)
		if readErr != nil {
			return fmt.Errorf("read %s: %w", path, readErr)
		}
		for _, h := range extractInlineScriptHashes(data) {
			seen[h] = struct{}{}
		}
		return nil
	})
	if walkErr != nil {
		return nil, walkErr
	}

	out := make([]string, 0, len(seen))
	for h := range seen {
		out = append(out, h)
	}
	sort.Strings(out)
	return out, nil
}

// extractInlineScriptHashes returns the base64 SHA-256 of every inline
// <script> body (i.e. <script> tags without a `src` attribute) in html.
func extractInlineScriptHashes(html []byte) []string {
	matches := scriptTagRe.FindAllSubmatch(html, -1)
	out := make([]string, 0, len(matches))
	for _, m := range matches {
		attrs := m[1]
		if hasSrcAttr(attrs) {
			continue
		}
		body := m[2]
		sum := sha256.Sum256(body)
		out = append(out, base64.StdEncoding.EncodeToString(sum[:]))
	}
	return out
}

// hasSrcAttr reports whether the opening-tag attribute slice declares a `src`.
// We look for `src=` rather than the bare word so we don't match e.g. `srcset`.
var srcAttrRe = regexp.MustCompile(`(?i)\bsrc\s*=`)

func hasSrcAttr(attrs []byte) bool {
	return srcAttrRe.Match(attrs)
}

// buildCSP renders the Content-Security-Policy string with one
// `'sha256-<hash>'` source per inline script found at startup.
//
// We keep style-src 'unsafe-inline' for Tailwind's runtime style injection.
// Scripts get hashes — never 'unsafe-inline' — so a stray injected <script>
// is still blocked.
func buildCSP(scriptHashes []string) string {
	var b strings.Builder
	b.WriteString("default-src 'self'; script-src 'self'")
	for _, h := range scriptHashes {
		b.WriteString(" 'sha256-")
		b.WriteString(h)
		b.WriteString("'")
	}
	// ws:/wss: are required alongside 'self' for WebSocket upgrades: the CSP3
	// spec does not guarantee that 'self' covers wss: when the page is https:.
	b.WriteString("; style-src 'self' 'unsafe-inline'; img-src 'self' data:; connect-src 'self' ws: wss:; frame-ancestors 'none'")
	return b.String()
}
