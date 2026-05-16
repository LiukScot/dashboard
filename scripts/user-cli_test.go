package main

import (
	"bytes"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestUserCLINoArgsPrintsUsage builds the user-cli binary and runs it with
// no arguments. Contract: prints a usage line containing "user-cli" and
// "create" and exits non-zero.
func TestUserCLINoArgsPrintsUsage(t *testing.T) {
	t.Parallel()
	bin := buildUserCLI(t)

	cmd := exec.Command(bin)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()

	assert.Error(t, err, "no-args should exit non-zero")
	combined := stdout.String() + stderr.String()
	assert.Contains(t, combined, "user-cli")
	assert.Contains(t, combined, "create")
}

func TestUserCLIUnknownCommandFails(t *testing.T) {
	t.Parallel()
	bin := buildUserCLI(t)

	cmd := exec.Command(bin, "no-such-thing")
	cmd.Env = append(cmd.Env, "DB_PATH="+filepath.Join(t.TempDir(), "x.sqlite"))
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()

	assert.Error(t, err)
	combined := stdout.String() + stderr.String()
	assert.True(t,
		strings.Contains(combined, "Unknown command") || strings.Contains(combined, "unknown command"),
		"expected unknown-command error, got: %q", combined)
}

func TestUserCLICreatesUser(t *testing.T) {
	t.Parallel()
	bin := buildUserCLI(t)
	dbPath := filepath.Join(t.TempDir(), "users.sqlite")

	cmd := exec.Command(bin, "create")
	cmd.Env = append(cmd.Env, "DB_PATH="+dbPath)
	cmd.Stdin = strings.NewReader("smoke@example.com\nhunter2\n")
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	require.NoError(t, cmd.Run(), "create should succeed: stderr=%s", stderr.String())
	assert.Contains(t, stdout.String(), "smoke@example.com")

	// `list` must show the user we just created.
	listCmd := exec.Command(bin, "list")
	listCmd.Env = append(listCmd.Env, "DB_PATH="+dbPath)
	var listOut bytes.Buffer
	listCmd.Stdout = &listOut
	listCmd.Stderr = &listOut
	require.NoError(t, listCmd.Run(), "list should succeed: out=%s", listOut.String())
	assert.Contains(t, listOut.String(), "smoke@example.com")
}

// buildUserCLI compiles scripts/user-cli.go into a temp binary and returns
// its path. Test isolates from `go install` state.
func buildUserCLI(t *testing.T) string {
	t.Helper()
	bin := filepath.Join(t.TempDir(), "user-cli")
	// Build the single source file directly; package main contains
	// only user-cli.go (plus this test) so `go build .` would pick up
	// the test too. Pass the source path instead.
	cmd := exec.Command("go", "build", "-o", bin, "user-cli.go")
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	require.NoError(t, cmd.Run(), "go build failed: %s", stderr.String())
	return bin
}
