package server

import (
	"crypto/sha256"
	"encoding/base64"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/LiukScot/dashboard/internal/config"
)

// hash returns the same base64-SHA256 a browser computes for an inline script
// body — useful as the source of truth in test expectations.
func hash(body string) string {
	sum := sha256.Sum256([]byte(body))
	return base64.StdEncoding.EncodeToString(sum[:])
}

// TestExtractInlineScriptHashes_GoldenBody pins the exact byte-for-byte hash
// the function returns for a known script body. If this drifts (e.g. someone
// "normalizes" whitespace in the scanner), browsers will reject the script.
func TestExtractInlineScriptHashes_GoldenBody(t *testing.T) {
	t.Parallel()

	body := "\n\t\t\t\twindow.x = 1;\n\t\t\t"
	html := []byte("<!doctype html><html><body><script>" + body + "</script></body></html>")

	got := extractInlineScriptHashes(html)
	require.Len(t, got, 1)
	assert.Equal(t, hash(body), got[0])
}

// TestExtractInlineScriptHashes_SkipsExternal makes sure scripts loaded via
// `src` (which have no body to hash) are not included — otherwise the CSP
// would gain a hash of `""` and we'd be hashing nothing.
func TestExtractInlineScriptHashes_SkipsExternal(t *testing.T) {
	t.Parallel()

	html := []byte(`<script src="/_app/start.js"></script><script>console.log(1)</script>`)
	got := extractInlineScriptHashes(html)

	require.Len(t, got, 1, "external <script src=...> must not be hashed")
	assert.Equal(t, hash("console.log(1)"), got[0])
}

// TestExtractInlineScriptHashes_SkipsSrcset guards the `\bsrc=` boundary —
// `srcset` on a future <script> attribute or sibling tag must not be confused
// with the `src` attribute.
func TestExtractInlineScriptHashes_SkipsSrcset(t *testing.T) {
	t.Parallel()

	html := []byte(`<script data-srcset="x" type="module">body1</script>`)
	got := extractInlineScriptHashes(html)
	require.Len(t, got, 1)
	assert.Equal(t, hash("body1"), got[0])
}

// TestScanInlineScriptHashes_WalksHTMLFiles checks the scanner reads every
// .html file under a directory and dedupes across files.
func TestScanInlineScriptHashes_WalksHTMLFiles(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "index.html"), []byte(`<script>alert(1)</script>`), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "200.html"), []byte(`<script>alert(1)</script><script>alert(2)</script>`), 0o644))
	// Non-HTML file should be ignored even if it contains a script-like substring.
	require.NoError(t, os.WriteFile(filepath.Join(dir, "notes.txt"), []byte(`<script>ignored</script>`), 0o644))

	got, err := scanInlineScriptHashes(dir)
	require.NoError(t, err)

	assert.ElementsMatch(t, []string{hash("alert(1)"), hash("alert(2)")}, got)
}

// TestScanInlineScriptHashes_MissingDirReturnsNil keeps the server bootable in
// environments where the frontend hasn't been built yet — buildCSP then emits
// a hash-free 'self' policy.
func TestScanInlineScriptHashes_MissingDirReturnsNil(t *testing.T) {
	t.Parallel()

	got, err := scanInlineScriptHashes(filepath.Join(t.TempDir(), "does-not-exist"))
	require.NoError(t, err)
	assert.Nil(t, got)
}

// TestBuildCSP_NeverShipsUnsafeInline is the contract test: a regression that
// re-introduces 'unsafe-inline' for script-src must fail CI.
func TestBuildCSP_NeverShipsUnsafeInline(t *testing.T) {
	t.Parallel()

	policy := buildCSP([]string{hash("alert(1)")})

	scriptDir := directive(t, policy, "script-src")
	assert.Contains(t, scriptDir, "'self'")
	assert.Contains(t, scriptDir, "'sha256-"+hash("alert(1)")+"'")
	assert.NotContains(t, scriptDir, "'unsafe-inline'", "script-src must never permit 'unsafe-inline' — use per-script hashes")
	assert.NotContains(t, scriptDir, "'unsafe-eval'")
}

// TestBuildCSP_EmptyHashesStillSafe verifies the no-frontend-build case: the
// policy is restrictive (just 'self'), which is what we want — a missing build
// should not silently relax security.
func TestBuildCSP_EmptyHashesStillSafe(t *testing.T) {
	t.Parallel()

	policy := buildCSP(nil)

	scriptDir := directive(t, policy, "script-src")
	assert.Equal(t, "'self'", strings.TrimSpace(scriptDir))
}

// TestSecurityHeaders_CSPCoversAllInlineScripts is the end-to-end regression
// test for the white-screen bug: build a fake frontend with an inline script,
// boot the server middleware, hit `/`, then verify every inline <script> in
// the served HTML has a matching `'sha256-...'` source in the CSP header.
//
// If this test fails, the dashboard will white-screen in production.
func TestSecurityHeaders_CSPCoversAllInlineScripts(t *testing.T) {
	t.Parallel()

	publicDir := t.TempDir()
	indexHTML := `<!doctype html><html><body>
<script>
	__sveltekit_test = { base: "" };
	console.log("boot");
</script>
<script src="/_app/start.js"></script>
</body></html>`
	require.NoError(t, os.WriteFile(filepath.Join(publicDir, "index.html"), []byte(indexHTML), 0o644))

	cfg := &config.Config{PublicDir: publicDir, AllowedOrigins: ""}
	hashes, err := scanInlineScriptHashes(cfg.PublicDir)
	require.NoError(t, err)
	srv := &Server{cfg: cfg, mux: http.NewServeMux(), csp: buildCSP(hashes)}
	srv.mux.HandleFunc("/", srv.handleStatic)

	handler := srv.securityHeaders(srv.mux)
	ts := httptest.NewServer(handler)
	t.Cleanup(ts.Close)

	res, err := http.Get(ts.URL + "/")
	require.NoError(t, err)
	t.Cleanup(func() { _ = res.Body.Close() })

	require.Equal(t, http.StatusOK, res.StatusCode)
	cspHeader := res.Header.Get("Content-Security-Policy")
	require.NotEmpty(t, cspHeader, "Content-Security-Policy header must be set")

	scriptDir := directive(t, cspHeader, "script-src")
	require.NotContains(t, scriptDir, "'unsafe-inline'", "regression: 'unsafe-inline' re-introduced")

	bodyBytes, err := io.ReadAll(res.Body)
	require.NoError(t, err)
	bodyHashes := extractInlineScriptHashes(bodyBytes)
	require.NotEmpty(t, bodyHashes, "test fixture must contain at least one inline script")

	for _, h := range bodyHashes {
		assert.Contains(t, scriptDir, "'sha256-"+h+"'",
			"served HTML contains inline script whose hash is missing from CSP — browser will block and the page will white-screen")
	}
}

// directive extracts the value (everything after the directive name) from a
// CSP header. Returns trimmed text; fails the test if the directive is absent.
var directiveSepRe = regexp.MustCompile(`\s*;\s*`)

func directive(t *testing.T, policy, name string) string {
	t.Helper()
	for _, part := range directiveSepRe.Split(policy, -1) {
		part = strings.TrimSpace(part)
		if strings.HasPrefix(part, name+" ") || part == name {
			return strings.TrimSpace(strings.TrimPrefix(part, name))
		}
	}
	t.Fatalf("directive %q not found in CSP %q", name, policy)
	return ""
}

