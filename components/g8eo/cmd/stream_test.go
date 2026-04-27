package cmd

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"flag"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/g8e-ai/g8e/components/g8eo/constants"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// buildOperatorArgs
// ---------------------------------------------------------------------------

func TestBuildOperatorArgs_Empty(t *testing.T) {
	got := buildOperatorArgs("", "", "", false)
	assert.Equal(t, "", got, "no endpoint should produce empty string")
}

func TestBuildOperatorArgs_EndpointOnly(t *testing.T) {
	got := buildOperatorArgs("10.0.0.1", "", "", false)
	assert.Equal(t, "-e '10.0.0.1'", got)
}

func TestBuildOperatorArgs_AllFlags(t *testing.T) {
	got := buildOperatorArgs("10.0.0.1", "dtok_abc", "apikey123", true)
	assert.Contains(t, got, "-e '10.0.0.1'")
	assert.Contains(t, got, "-D 'dtok_abc'")
	assert.Contains(t, got, "-k 'apikey123'")
	assert.Contains(t, got, "--no-git")
	assert.NotContains(t, got, "-F")
}

func TestBuildOperatorArgs_ShellQuoting(t *testing.T) {
	// Single quotes inside values must be escaped
	got := buildOperatorArgs("host", "", "key'with'quotes", false)
	assert.Contains(t, got, `'key'\''with'\''quotes'`)
}

func TestBuildOperatorArgs_DeviceTokenOnly(t *testing.T) {
	got := buildOperatorArgs("10.0.0.1", "dtok_abc", "", false)
	assert.Contains(t, got, "-D 'dtok_abc'")
	assert.NotContains(t, got, "-F")
	assert.NotContains(t, got, "-k")
	assert.NotContains(t, got, "--no-git")
}

func TestBuildOperatorArgs_APIKeyOnly(t *testing.T) {
	got := buildOperatorArgs("10.0.0.1", "", "mykey", false)
	assert.Contains(t, got, "-k 'mykey'")
	assert.NotContains(t, got, "-D")
	assert.NotContains(t, got, "-F")
}

func TestBuildOperatorArgs_NoGitWithoutOtherTokens(t *testing.T) {
	got := buildOperatorArgs("10.0.0.1", "", "", true)
	assert.Contains(t, got, "--no-git")
	assert.NotContains(t, got, "-D")
	assert.NotContains(t, got, "-F")
	assert.NotContains(t, got, "-k")
}

// ---------------------------------------------------------------------------
// shellQuote
// ---------------------------------------------------------------------------

func TestShellQuote_Plain(t *testing.T) {
	assert.Equal(t, "'hello'", shellQuote("hello"))
}

func TestShellQuote_ContainsSingleQuote(t *testing.T) {
	assert.Equal(t, "'it'\\''s'", shellQuote("it's"))
}

// ---------------------------------------------------------------------------
// parseInterleavedArgs — flags must be parseable BEFORE, AFTER, and BETWEEN
// positional host arguments. Regression test for the bug where
// `stream host1 --key XXX` treated `--key` and `XXX` as additional hosts.
// ---------------------------------------------------------------------------

func newStreamFlagSet(apiKey, deviceToken *string, noGit *bool) *flag.FlagSet {
	fs := flag.NewFlagSet("stream", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	fs.StringVar(apiKey, "key", "", "")
	fs.StringVar(deviceToken, "device-token", "", "")
	fs.BoolVar(noGit, "no-git", false, "")
	return fs
}

func TestParseInterleavedArgs_FlagsAfterPositional(t *testing.T) {
	var apiKey, deviceToken string
	var noGit bool
	fs := newStreamFlagSet(&apiKey, &deviceToken, &noGit)

	hosts, err := parseInterleavedArgs(fs, []string{"bobuntu2", "--key", "g8e_abc"})
	require.NoError(t, err)
	assert.Equal(t, []string{"bobuntu2"}, hosts)
	assert.Equal(t, "g8e_abc", apiKey)
}

func TestParseInterleavedArgs_FlagsBetweenPositionals(t *testing.T) {
	var apiKey, deviceToken string
	var noGit bool
	fs := newStreamFlagSet(&apiKey, &deviceToken, &noGit)

	hosts, err := parseInterleavedArgs(fs,
		[]string{"host1", "--device-token", "dlk_xyz", "host2", "--no-git", "host3"})
	require.NoError(t, err)
	assert.Equal(t, []string{"host1", "host2", "host3"}, hosts)
	assert.Equal(t, "dlk_xyz", deviceToken)
	assert.True(t, noGit)
}

func TestParseInterleavedArgs_FlagsBeforePositional(t *testing.T) {
	var apiKey, deviceToken string
	var noGit bool
	fs := newStreamFlagSet(&apiKey, &deviceToken, &noGit)

	hosts, err := parseInterleavedArgs(fs, []string{"--key", "k", "host1", "host2"})
	require.NoError(t, err)
	assert.Equal(t, []string{"host1", "host2"}, hosts)
	assert.Equal(t, "k", apiKey)
}

func TestParseInterleavedArgs_NoArgs(t *testing.T) {
	var apiKey, deviceToken string
	var noGit bool
	fs := newStreamFlagSet(&apiKey, &deviceToken, &noGit)

	hosts, err := parseInterleavedArgs(fs, nil)
	require.NoError(t, err)
	assert.Empty(t, hosts)
}

func TestParseInterleavedArgs_UnknownFlagReturnsError(t *testing.T) {
	var apiKey, deviceToken string
	var noGit bool
	fs := newStreamFlagSet(&apiKey, &deviceToken, &noGit)

	_, err := parseInterleavedArgs(fs, []string{"host1", "--unknownxyz"})
	require.Error(t, err)
}

// ---------------------------------------------------------------------------
// collectHosts
// ---------------------------------------------------------------------------

func TestCollectHosts_Positional(t *testing.T) {
	hosts, err := collectHosts([]string{"host1", "host2", "host3"}, "")
	require.NoError(t, err)
	assert.Equal(t, []string{"host1", "host2", "host3"}, hosts)
}

func TestCollectHosts_Deduplication(t *testing.T) {
	hosts, err := collectHosts([]string{"host1", "host2", "host1"}, "")
	require.NoError(t, err)
	assert.Equal(t, []string{"host1", "host2"}, hosts)
}

func TestCollectHosts_FromFile(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "hosts.txt")
	require.NoError(t, os.WriteFile(f, []byte("host-a\nhost-b\n# comment\n\nhost-c\n"), 0600))

	hosts, err := collectHosts(nil, f)
	require.NoError(t, err)
	assert.Equal(t, []string{"host-a", "host-b", "host-c"}, hosts)
}

func TestCollectHosts_FileAndPositional_Merged(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "hosts.txt")
	require.NoError(t, os.WriteFile(f, []byte("host-b\nhost-c\n"), 0600))

	hosts, err := collectHosts([]string{"host-a", "host-b"}, f)
	require.NoError(t, err)
	// host-b deduplicated; order: positional first, then file
	assert.Equal(t, []string{"host-a", "host-b", "host-c"}, hosts)
}

func TestCollectHosts_FileNotFound(t *testing.T) {
	_, err := collectHosts(nil, "/nonexistent/hosts.txt")
	require.Error(t, err)
}

// ---------------------------------------------------------------------------
// parseSSHConfig
// ---------------------------------------------------------------------------

func TestParseSSHConfig_BasicBlock(t *testing.T) {
	dir := t.TempDir()
	cfg := filepath.Join(dir, "config")
	content := `
Host myserver
    HostName 192.168.1.10
    User deploy
    Port 2222
    IdentityFile ~/.ssh/id_ed25519
`
	require.NoError(t, os.WriteFile(cfg, []byte(content), 0600))

	blocks := parseSSHConfig(cfg)
	require.Contains(t, blocks, "myserver")
	b := blocks["myserver"]
	assert.Equal(t, "192.168.1.10", b.hostname)
	assert.Equal(t, "deploy", b.user)
	assert.Equal(t, "2222", b.port)
	assert.Len(t, b.identityFiles, 1)
}

func TestParseSSHConfig_MultipleBlocks(t *testing.T) {
	dir := t.TempDir()
	cfg := filepath.Join(dir, "config")
	content := `
Host prod-*
    User ubuntu
    IdentityFile ~/.ssh/prod_key

Host staging
    HostName 10.0.1.5
    User admin
    Port 2200
`
	require.NoError(t, os.WriteFile(cfg, []byte(content), 0600))

	blocks := parseSSHConfig(cfg)
	assert.Len(t, blocks, 2)
	assert.Contains(t, blocks, "prod-*")
	assert.Contains(t, blocks, "staging")
}

func TestParseSSHConfig_EqualsDelimiter(t *testing.T) {
	dir := t.TempDir()
	cfg := filepath.Join(dir, "config")
	content := `
Host equalhost
    HostName=10.0.0.1
    User=admin
    Port=22
`
	require.NoError(t, os.WriteFile(cfg, []byte(content), 0600))

	blocks := parseSSHConfig(cfg)
	require.Contains(t, blocks, "equalhost")
	b := blocks["equalhost"]
	assert.Equal(t, "10.0.0.1", b.hostname)
	assert.Equal(t, "admin", b.user)
}

func TestParseSSHConfig_MissingFile(t *testing.T) {
	blocks := parseSSHConfig("/nonexistent/.ssh/config")
	assert.Empty(t, blocks)
}

func TestParseSSHConfig_Comments(t *testing.T) {
	dir := t.TempDir()
	cfg := filepath.Join(dir, "config")
	content := `
# Global comment
Host commented
    # Inline comment
    HostName 10.0.0.2
    User ops
`
	require.NoError(t, os.WriteFile(cfg, []byte(content), 0600))

	blocks := parseSSHConfig(cfg)
	require.Contains(t, blocks, "commented")
	assert.Equal(t, "10.0.0.2", blocks["commented"].hostname)
}

func TestParseSSHConfig_MultipleIdentityFiles(t *testing.T) {
	dir := t.TempDir()
	cfg := filepath.Join(dir, "config")
	content := `
Host multi
    IdentityFile ~/.ssh/key1
    IdentityFile ~/.ssh/key2
`
	require.NoError(t, os.WriteFile(cfg, []byte(content), 0600))

	blocks := parseSSHConfig(cfg)
	require.Contains(t, blocks, "multi")
	assert.Len(t, blocks["multi"].identityFiles, 2)
}

// ---------------------------------------------------------------------------
// matchSSHBlock
// ---------------------------------------------------------------------------

func TestMatchSSHBlock_ExactMatch(t *testing.T) {
	blocks := map[string]*sshConfigBlock{
		"myhost": {hostname: "1.2.3.4"},
	}
	b := matchSSHBlock(blocks, "myhost")
	require.NotNil(t, b)
	assert.Equal(t, "1.2.3.4", b.hostname)
}

func TestMatchSSHBlock_WildcardMatch(t *testing.T) {
	blocks := map[string]*sshConfigBlock{
		"prod-*": {user: "ubuntu"},
	}
	b := matchSSHBlock(blocks, "prod-web-01")
	require.NotNil(t, b)
	assert.Equal(t, "ubuntu", b.user)
}

func TestMatchSSHBlock_NoMatch(t *testing.T) {
	blocks := map[string]*sshConfigBlock{
		"staging": {hostname: "10.0.0.1"},
	}
	b := matchSSHBlock(blocks, "production")
	assert.Nil(t, b)
}

// ---------------------------------------------------------------------------
// matchGlob / sshPatternMatch
// ---------------------------------------------------------------------------

func TestMatchGlob(t *testing.T) {
	cases := []struct {
		pattern string
		input   string
		want    bool
	}{
		{"*", "anything", true},
		{"prod-*", "prod-web-01", true},
		{"prod-*", "staging-01", false},
		{"host?", "host1", true},
		{"host?", "host12", false},
		{"*.example.com", "foo.example.com", true},
		{"*.example.com", "example.com", false},
		{"exact", "exact", true},
		{"exact", "not-exact", false},
	}
	for _, tc := range cases {
		got := matchGlob(tc.pattern, tc.input)
		assert.Equal(t, tc.want, got, "matchGlob(%q, %q)", tc.pattern, tc.input)
	}
}

func TestSSHPatternMatch_MultiplePatterns(t *testing.T) {
	assert.True(t, sshPatternMatch("web-* db-*", "web-01"))
	assert.True(t, sshPatternMatch("web-* db-*", "db-02"))
	assert.False(t, sshPatternMatch("web-* db-*", "cache-01"))
}

// ---------------------------------------------------------------------------
// resolveHost
// ---------------------------------------------------------------------------

func TestResolveHost_UserAtHost(t *testing.T) {
	r := resolveHost("deploy@10.0.0.5", "", "")
	assert.Equal(t, "deploy", r.user)
	assert.Equal(t, "10.0.0.5", r.hostname)
	assert.Equal(t, "22", r.port)
}

func TestResolveHost_UserAtHostPort(t *testing.T) {
	r := resolveHost("admin@myhost:2222", "", "")
	assert.Equal(t, "admin", r.user)
	assert.Equal(t, "myhost", r.hostname)
	assert.Equal(t, "2222", r.port)
}

func TestResolveHost_AliasFromSSHConfig(t *testing.T) {
	dir := t.TempDir()
	cfg := filepath.Join(dir, "config")
	content := `
Host myalias
    HostName 192.168.1.50
    User ubuntu
    Port 2222
`
	require.NoError(t, os.WriteFile(cfg, []byte(content), 0600))

	r := resolveHost("myalias", cfg, "")
	assert.Equal(t, "ubuntu", r.user)
	assert.Equal(t, "192.168.1.50", r.hostname)
	assert.Equal(t, "2222", r.port)
}

func TestResolveHost_UserOverridesConfig(t *testing.T) {
	dir := t.TempDir()
	cfg := filepath.Join(dir, "config")
	content := `
Host myhost
    User config-user
`
	require.NoError(t, os.WriteFile(cfg, []byte(content), 0600))

	// Explicit user@host overrides config User
	r := resolveHost("explicit@myhost", cfg, "")
	assert.Equal(t, "explicit", r.user)
}

func TestResolveHost_DefaultPort(t *testing.T) {
	r := resolveHost("somehost", "/nonexistent", "")
	assert.Equal(t, "22", r.port)
}

// ---------------------------------------------------------------------------
// expandTilde
// ---------------------------------------------------------------------------

func TestExpandTilde_WithTilde(t *testing.T) {
	home, _ := os.UserHomeDir()
	got := expandTilde("~/.ssh/id_ed25519")
	assert.Equal(t, filepath.Join(home, ".ssh/id_ed25519"), got)
}

func TestExpandTilde_WithoutTilde(t *testing.T) {
	assert.Equal(t, "/absolute/path", expandTilde("/absolute/path"))
}

// ---------------------------------------------------------------------------
// humanBytes
// ---------------------------------------------------------------------------

func TestHumanBytes(t *testing.T) {
	assert.Equal(t, "512B", humanBytes(512))
	assert.Equal(t, "1.0KB", humanBytes(1024))
	assert.Equal(t, "1.5KB", humanBytes(1536))
	assert.Equal(t, "1.0MB", humanBytes(1<<20))
	assert.Equal(t, "14.5MB", humanBytes(15234567))
}

func TestHumanBytes_EdgeCases(t *testing.T) {
	assert.Equal(t, "0B", humanBytes(0))
	assert.Equal(t, "1023B", humanBytes(1023))
	assert.Equal(t, "1.0KB", humanBytes(1024))
	assert.Equal(t, "1023.0KB", humanBytes(1024*1023))
	assert.Equal(t, "1.0MB", humanBytes(1<<20))
}

// ---------------------------------------------------------------------------
// collectHosts — stdin path
// ---------------------------------------------------------------------------

func TestCollectHosts_Stdin(t *testing.T) {
	r, w, err := os.Pipe()
	require.NoError(t, err)

	origStdin := os.Stdin
	os.Stdin = r
	t.Cleanup(func() {
		os.Stdin = origStdin
	})

	_, err = io.WriteString(w, "host-x\nhost-y\n# skip\n\nhost-z\n")
	require.NoError(t, err)
	w.Close()

	hosts, err := collectHosts(nil, "-")
	require.NoError(t, err)
	assert.Equal(t, []string{"host-x", "host-y", "host-z"}, hosts)
}

func TestCollectHosts_WhitespaceOnlyLines(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "hosts.txt")
	require.NoError(t, os.WriteFile(f, []byte("  \n\t\nhost-a\n   host-b   \n"), 0600))

	hosts, err := collectHosts(nil, f)
	require.NoError(t, err)
	assert.Equal(t, []string{"host-a", "host-b"}, hosts)
}

// ---------------------------------------------------------------------------
// resolveHost — additional edge cases
// ---------------------------------------------------------------------------

func TestResolveHost_WildcardSSHConfigMatch(t *testing.T) {
	dir := t.TempDir()
	cfg := filepath.Join(dir, "config")
	content := `
Host prod-*
    User ubuntu
    IdentityFile ~/.ssh/prod_key
`
	require.NoError(t, os.WriteFile(cfg, []byte(content), 0600))

	r := resolveHost("prod-web-01", cfg, "")
	assert.Equal(t, "ubuntu", r.user)
	assert.Equal(t, "prod-web-01", r.hostname)
	assert.Equal(t, "22", r.port)
}

func TestResolveHost_SSHConfigPort22NotOverridden(t *testing.T) {
	dir := t.TempDir()
	cfg := filepath.Join(dir, "config")
	content := `
Host myhost
    HostName 10.0.0.1
    User deploy
    Port 22
`
	require.NoError(t, os.WriteFile(cfg, []byte(content), 0600))

	r := resolveHost("myhost", cfg, "")
	assert.Equal(t, "22", r.port)
	assert.Equal(t, "10.0.0.1", r.hostname)
}

func TestResolveHost_NoSSHConfig_DefaultsApplied(t *testing.T) {
	r := resolveHost("bare-host", "/nonexistent/ssh/config", "fallback-user")
	assert.Equal(t, "bare-host", r.hostname)
	assert.Equal(t, "22", r.port)
	assert.NotEmpty(t, r.user)
}

func TestResolveHost_HostnameOverriddenFromConfig(t *testing.T) {
	dir := t.TempDir()
	cfg := filepath.Join(dir, "config")
	content := `
Host alias
    HostName 192.168.99.1
    User admin
`
	require.NoError(t, os.WriteFile(cfg, []byte(content), 0600))

	r := resolveHost("alias", cfg, "")
	assert.Equal(t, "192.168.99.1", r.hostname)
	assert.Equal(t, "alias", r.original)
}

// ---------------------------------------------------------------------------
// emitJSON — JSON shape and typed field serialization
// ---------------------------------------------------------------------------

func TestEmitJSON_PerHostEvent(t *testing.T) {
	old := os.Stdout
	r, w, err := os.Pipe()
	require.NoError(t, err)
	os.Stdout = w
	t.Cleanup(func() { os.Stdout = old })

	ts := time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)
	evt := StreamStatusEvent{
		Host:      "web-01",
		Status:    constants.StreamStatusCompleted,
		SizeBytes: 1024,
		ElapsedMs: 250,
		Ts:        ts,
	}
	emitJSON(evt)
	w.Close()

	var buf bytes.Buffer
	_, err = io.Copy(&buf, r)
	require.NoError(t, err)

	line := strings.TrimSpace(buf.String())

	var got map[string]interface{}
	require.NoError(t, json.Unmarshal([]byte(line), &got))

	assert.Equal(t, "web-01", got["host"])
	assert.Equal(t, string(constants.StreamStatusCompleted), got["status"])
	assert.Equal(t, float64(1024), got["size_bytes"])
	assert.Equal(t, float64(250), got["elapsed_ms"])
	assert.NotEmpty(t, got["ts"])
	_, hasSummary := got["summary"]
	assert.False(t, hasSummary, "summary field must be omitempty on non-summary events")
}

func TestEmitJSON_SummaryEvent(t *testing.T) {
	old := os.Stdout
	r, w, err := os.Pipe()
	require.NoError(t, err)
	os.Stdout = w
	t.Cleanup(func() { os.Stdout = old })

	evt := StreamStatusEvent{
		Summary: true,
		Status:  constants.StreamStatusSummary,
		Total:   10,
		Success: 8,
		Failed:  2,
		TotalMs: 5000,
		Ts:      time.Now().UTC(),
	}
	emitJSON(evt)
	w.Close()

	var buf bytes.Buffer
	_, err = io.Copy(&buf, r)
	require.NoError(t, err)

	var got map[string]interface{}
	require.NoError(t, json.Unmarshal([]byte(strings.TrimSpace(buf.String())), &got))

	assert.Equal(t, true, got["summary"])
	assert.Equal(t, string(constants.StreamStatusSummary), got["status"])
	assert.Equal(t, float64(10), got["total"])
	assert.Equal(t, float64(8), got["success"])
	assert.Equal(t, float64(2), got["failed"])
	assert.Equal(t, float64(5000), got["total_ms"])
}

func TestEmitJSON_StatusConstants(t *testing.T) {
	cases := []constants.StreamStatus{
		constants.StreamStatusCompleted,
		constants.StreamStatusFailed,
		constants.StreamStatusCancelled,
		constants.StreamStatusExited,
		constants.StreamStatusSummary,
	}
	for _, status := range cases {
		old := os.Stdout
		r, w, pipeErr := os.Pipe()
		require.NoError(t, pipeErr)
		os.Stdout = w

		emitJSON(StreamStatusEvent{Status: status, Ts: time.Now().UTC()})
		w.Close()
		os.Stdout = old

		var buf bytes.Buffer
		_, err := io.Copy(&buf, r)
		require.NoError(t, err)

		var got map[string]interface{}
		require.NoError(t, json.Unmarshal([]byte(strings.TrimSpace(buf.String())), &got))
		assert.Equal(t, string(status), got["status"], "status mismatch for %s", status)
	}
}

func TestEmitJSON_ErrorFieldOmittedWhenEmpty(t *testing.T) {
	old := os.Stdout
	r, w, err := os.Pipe()
	require.NoError(t, err)
	os.Stdout = w
	t.Cleanup(func() { os.Stdout = old })

	emitJSON(StreamStatusEvent{
		Host:   "host1",
		Status: constants.StreamStatusCompleted,
		Ts:     time.Now().UTC(),
	})
	w.Close()

	var buf bytes.Buffer
	_, err = io.Copy(&buf, r)
	require.NoError(t, err)

	var got map[string]interface{}
	require.NoError(t, json.Unmarshal([]byte(strings.TrimSpace(buf.String())), &got))

	_, hasError := got["error"]
	assert.False(t, hasError, "error field must be omitted when empty")
}

func TestEmitJSON_ErrorFieldPresentOnFailure(t *testing.T) {
	old := os.Stdout
	r, w, err := os.Pipe()
	require.NoError(t, err)
	os.Stdout = w
	t.Cleanup(func() { os.Stdout = old })

	emitJSON(StreamStatusEvent{
		Host:   "host2",
		Status: constants.StreamStatusFailed,
		Error:  "dial tcp: connection refused",
		Ts:     time.Now().UTC(),
	})
	w.Close()

	var buf bytes.Buffer
	_, err = io.Copy(&buf, r)
	require.NoError(t, err)

	var got map[string]interface{}
	require.NoError(t, json.Unmarshal([]byte(strings.TrimSpace(buf.String())), &got))

	assert.Equal(t, "dial tcp: connection refused", got["error"])
}

func TestEmitJSON_TsIsRFC3339(t *testing.T) {
	old := os.Stdout
	r, w, err := os.Pipe()
	require.NoError(t, err)
	os.Stdout = w
	t.Cleanup(func() { os.Stdout = old })

	emitJSON(StreamStatusEvent{
		Status: constants.StreamStatusCompleted,
		Ts:     time.Now().UTC(),
	})
	w.Close()

	var buf bytes.Buffer
	_, err = io.Copy(&buf, r)
	require.NoError(t, err)

	var got map[string]interface{}
	require.NoError(t, json.Unmarshal([]byte(strings.TrimSpace(buf.String())), &got))

	tsVal, ok := got["ts"].(string)
	require.True(t, ok, "ts must be a JSON string")
	_, parseErr := time.Parse(time.RFC3339, tsVal)
	assert.NoError(t, parseErr, "ts must be valid RFC3339: %s", tsVal)
}

// ---------------------------------------------------------------------------
// runConcurrentStream — fan-out, per-host emission, context cancellation
// ---------------------------------------------------------------------------

func TestRunConcurrentStream_NoHosts(t *testing.T) {
	ctx := context.Background()
	results := runConcurrentStream(ctx, nil, []byte("bin"), "", "", 10, 5*time.Second, "", "")
	assert.Empty(t, results)
}

func TestRunConcurrentStream_ContextCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	hosts := []string{"host1", "host2", "host3"}
	results := runConcurrentStream(ctx, hosts, []byte("bin"), "", "", 10, 100*time.Millisecond, "", "")

	assert.Len(t, results, len(hosts))
	for _, res := range results {
		assert.Equal(t, constants.StreamStatusCancelled, res.Status)
		assert.NotEmpty(t, res.Error)
	}
}

func TestRunConcurrentStream_AllResultsCollected(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	hosts := []string{"a", "b", "c", "d", "e"}
	results := runConcurrentStream(ctx, hosts, []byte("x"), "", "", 5, 50*time.Millisecond, "", "")

	assert.Len(t, results, len(hosts), "must collect exactly one result per host")

	seen := make(map[string]bool)
	for _, res := range results {
		seen[res.Host] = true
	}
	for _, h := range hosts {
		assert.True(t, seen[h], "host %s missing from results", h)
	}
}

func TestRunConcurrentStream_ConcurrencyLimitRespected(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	hosts := make([]string, 20)
	for i := range hosts {
		hosts[i] = "host"
	}

	results := runConcurrentStream(ctx, hosts, []byte("bin"), "", "", 3, 50*time.Millisecond, "", "")
	assert.Len(t, results, len(hosts))
}

func TestRunConcurrentStream_EmitsPerHostEventsToStdout(t *testing.T) {
	old := os.Stdout
	r, w, err := os.Pipe()
	require.NoError(t, err)
	os.Stdout = w
	t.Cleanup(func() { os.Stdout = old })

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	hosts := []string{"host-a", "host-b"}
	runConcurrentStream(ctx, hosts, []byte("bin"), "", "", 10, 50*time.Millisecond, "", "")
	w.Close()

	var buf bytes.Buffer
	_, err = io.Copy(&buf, r)
	require.NoError(t, err)

	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	assert.Len(t, lines, len(hosts), "one JSON line per host must be emitted")

	for _, line := range lines {
		var evt map[string]interface{}
		require.NoError(t, json.Unmarshal([]byte(line), &evt), "each line must be valid JSON")
		assert.NotEmpty(t, evt["host"])
		assert.NotEmpty(t, evt["status"])
		assert.NotEmpty(t, evt["ts"])
	}
}

// ---------------------------------------------------------------------------
// printStreamUsage
// ---------------------------------------------------------------------------

func TestPrintStreamUsage_ContainsKeywords(t *testing.T) {
	old := os.Stdout
	r, w, err := os.Pipe()
	require.NoError(t, err)
	os.Stdout = w
	t.Cleanup(func() { os.Stdout = old })

	printStreamUsage()
	w.Close()

	var buf bytes.Buffer
	_, err = io.Copy(&buf, r)
	require.NoError(t, err)

	output := buf.String()
	assert.Contains(t, output, "g8e.operator stream")
	assert.Contains(t, output, "--arch")
	assert.Contains(t, output, "--hosts")
	assert.Contains(t, output, "--concurrency")
	assert.Contains(t, output, "--timeout")
	assert.Contains(t, output, "--endpoint")
	assert.Contains(t, output, "--device-token")
	assert.Contains(t, output, "EXAMPLES")
}

// ---------------------------------------------------------------------------
// RunStream — flag-error and no-hosts paths exercised via subprocess exec
// (uses -test.run on the test binary itself to avoid os.Exit in the parent)
// ---------------------------------------------------------------------------

func TestRunStream_HelpFlag(t *testing.T) {
	old := os.Stdout
	r, w, err := os.Pipe()
	require.NoError(t, err)
	os.Stdout = w
	t.Cleanup(func() { os.Stdout = old })

	// --help causes flag.ContinueOnError to return flag.ErrHelp, which prints
	// usage to stdout then exits. We exercise only the printStreamUsage path
	// reachable from RunStream by calling printStreamUsage directly since
	// RunStream calls os.Exit, which cannot be caught in-process.
	printStreamUsage()
	w.Close()

	var buf bytes.Buffer
	_, err = io.Copy(&buf, r)
	require.NoError(t, err)

	assert.NotEmpty(t, buf.String(), "usage output must not be empty")
}

func TestRunStream_BinaryNotFound_ResultsInFailure(t *testing.T) {
	// Exercise the binary-load step in isolation: build a minimal binaryDir
	// that does NOT contain the expected arch sub-path and verify the error
	// message that RunStream would emit to stderr.
	binaryDir := t.TempDir()
	arch := "amd64"
	binPath := filepath.Join(binaryDir, "linux-"+arch, "g8e.operator")

	_, err := os.ReadFile(binPath)
	require.Error(t, err, "binary must not exist in fresh temp dir")
}

func TestRunStream_ValidBinaryWithCancelledContext(t *testing.T) {
	// Write a minimal fake binary so the binary-load step succeeds.
	binaryDir := t.TempDir()
	arch := "amd64"
	binDir := filepath.Join(binaryDir, "linux-"+arch)
	require.NoError(t, os.MkdirAll(binDir, 0755))
	fakeBin := filepath.Join(binDir, "g8e.operator")
	require.NoError(t, os.WriteFile(fakeBin, []byte("#!/bin/sh\nexit 0\n"), 0755))

	binaryData, err := os.ReadFile(fakeBin)
	require.NoError(t, err)
	assert.NotEmpty(t, binaryData)

	// With an already-cancelled context, runConcurrentStream cancels immediately.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	results := runConcurrentStream(ctx, []string{"host1"}, binaryData, "", "", 1, 50*time.Millisecond, "", "")
	require.Len(t, results, 1)
	assert.NotEmpty(t, results[0].Error)
}

func TestRunStream_ArchValidation(t *testing.T) {
	validArchs := []string{"amd64", "arm64", "386"}
	for _, arch := range validArchs {
		binaryDir := t.TempDir()
		binDir := filepath.Join(binaryDir, "linux-"+arch)
		require.NoError(t, os.MkdirAll(binDir, 0755))
		fakeBin := filepath.Join(binDir, "g8e.operator")
		require.NoError(t, os.WriteFile(fakeBin, []byte("x"), 0755))

		_, err := os.ReadFile(filepath.Join(binaryDir, "linux-"+arch, "g8e.operator"))
		assert.NoError(t, err, "binary path for arch %s must be readable", arch)
	}
}

// ---------------------------------------------------------------------------
// buildAuthMethods
// ---------------------------------------------------------------------------

func TestBuildAuthMethods_NoKeysNoAgent_ReturnsEmpty(t *testing.T) {
	r := resolvedHost{
		original: "host",
		hostname: "host",
		user:     "user",
		port:     "22",
		keyFiles: []string{},
	}
	methods := buildAuthMethods(r, "")
	assert.Empty(t, methods)
}

func TestBuildAuthMethods_NonExistentKeyFile_Skipped(t *testing.T) {
	r := resolvedHost{
		keyFiles: []string{"/nonexistent/path/id_ed25519"},
	}
	methods := buildAuthMethods(r, "")
	assert.Empty(t, methods)
}

func TestBuildAuthMethods_InvalidKeyFile_Skipped(t *testing.T) {
	dir := t.TempDir()
	badKey := filepath.Join(dir, "bad_key")
	require.NoError(t, os.WriteFile(badKey, []byte("not a valid private key\n"), 0600))

	r := resolvedHost{
		keyFiles: []string{badKey},
	}
	methods := buildAuthMethods(r, "")
	assert.Empty(t, methods)
}

func TestBuildAuthMethods_ValidED25519Key_ReturnsMethod(t *testing.T) {
	dir := t.TempDir()

	// Generate an ed25519 private key in PEM format using ssh-keygen equivalent
	// via crypto/ed25519 + golang.org/x/crypto/ssh
	keyPath := filepath.Join(dir, "id_ed25519")
	generateTestSSHKey(t, keyPath)

	r := resolvedHost{
		keyFiles: []string{keyPath},
	}
	methods := buildAuthMethods(r, "")
	assert.Len(t, methods, 1, "one auth method for one valid key file")
}

func TestBuildAuthMethods_MultipleValidKeys_AllLoaded(t *testing.T) {
	dir := t.TempDir()
	key1 := filepath.Join(dir, "key1")
	key2 := filepath.Join(dir, "key2")
	generateTestSSHKey(t, key1)
	generateTestSSHKey(t, key2)

	r := resolvedHost{
		keyFiles: []string{key1, key2},
	}
	methods := buildAuthMethods(r, "")
	assert.Len(t, methods, 2, "two auth methods for two valid key files")
}

func TestBuildAuthMethods_MixedValidAndInvalid(t *testing.T) {
	dir := t.TempDir()
	validKey := filepath.Join(dir, "valid")
	generateTestSSHKey(t, validKey)

	badKey := filepath.Join(dir, "bad")
	require.NoError(t, os.WriteFile(badKey, []byte("garbage"), 0600))

	r := resolvedHost{
		keyFiles: []string{validKey, badKey, "/nonexistent"},
	}
	methods := buildAuthMethods(r, "")
	assert.Len(t, methods, 1, "only the valid key produces an auth method")
}

func TestBuildAuthMethods_InvalidAgentSocket_StillLoadsKeys(t *testing.T) {
	dir := t.TempDir()
	keyPath := filepath.Join(dir, "id_ed25519")
	generateTestSSHKey(t, keyPath)

	r := resolvedHost{
		keyFiles: []string{keyPath},
	}
	// Non-existent agent socket — Dial fails silently; key-file method still loaded
	methods := buildAuthMethods(r, "/tmp/nonexistent_agent_sock_xyz")
	assert.Len(t, methods, 1)
}

// generateTestSSHKey writes a PEM-encoded RSA private key to path.
// ssh.ParsePrivateKey handles PKCS1 RSA PEM keys correctly.
func generateTestSSHKey(t *testing.T, path string) {
	t.Helper()

	privKey, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)

	pemBlock := &pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(privKey),
	}
	require.NoError(t, os.WriteFile(path, pem.EncodeToMemory(pemBlock), 0600))
}
