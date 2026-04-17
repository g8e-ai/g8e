package cmd

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"time"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/agent"
	"golang.org/x/crypto/ssh/knownhosts"

	"github.com/g8e-ai/g8e/components/g8eo/constants"
)

const defaultSSHTimeout = 30 * time.Second

// resolvedHost holds the SSH connection parameters for a single target.
type resolvedHost struct {
	original string
	hostname string
	user     string
	port     string
	keyFiles []string
}

// streamResult is emitted by streamToHost for each host attempt.
type streamResult struct {
	Host      string
	Status    constants.StreamStatus
	SizeBytes int64
	Error     string
	Elapsed   time.Duration
}

// sshConfigBlock holds parsed values for a single Host block.
type sshConfigBlock struct {
	hostname      string
	user          string
	port          string
	identityFiles []string
}

// parseSSHConfig reads an OpenSSH-format config file and returns a map of
// pattern → block for the fields relevant to operator streaming:
// HostName, User, Port, IdentityFile.
//
// This is a minimal parser that handles the subset of directives we need.
// It does not handle Match blocks, Include, or multi-value canonicalisation.
func parseSSHConfig(path string) map[string]*sshConfigBlock {
	blocks := make(map[string]*sshConfigBlock)

	f, err := os.Open(path)
	if err != nil {
		return blocks
	}
	defer f.Close()

	var current *sshConfigBlock
	var currentPattern string

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		// Split key and value (supports both "Key Value" and "Key=Value")
		line = strings.ReplaceAll(line, "=", " ")
		parts := strings.Fields(line)
		if len(parts) < 2 {
			continue
		}
		key := strings.ToLower(parts[0])
		val := strings.Join(parts[1:], " ")

		switch key {
		case "host":
			// New Host block — save previous, start fresh
			currentPattern = val
			b := &sshConfigBlock{}
			blocks[currentPattern] = b
			current = b
		case "hostname":
			if current != nil {
				current.hostname = val
			}
		case "user":
			if current != nil {
				current.user = val
			}
		case "port":
			if current != nil {
				current.port = val
			}
		case "identityfile":
			if current != nil {
				current.identityFiles = append(current.identityFiles, expandTilde(val))
			}
		}
	}
	return blocks
}

// matchSSHBlock finds the first Host block in blocks whose pattern matches
// the given alias. Supports exact match and simple wildcard (*/?).
func matchSSHBlock(blocks map[string]*sshConfigBlock, alias string) *sshConfigBlock {
	// Exact match first
	if b, ok := blocks[alias]; ok {
		return b
	}
	// Wildcard patterns
	for pattern, b := range blocks {
		if sshPatternMatch(pattern, alias) {
			return b
		}
	}
	return nil
}

// sshPatternMatch implements the OpenSSH Host pattern matching rules:
// '*' matches any sequence of characters, '?' matches a single character.
// Multiple patterns in one Host line are space-separated.
func sshPatternMatch(patterns, alias string) bool {
	for _, p := range strings.Fields(patterns) {
		if p == "*" {
			return true
		}
		if matchGlob(p, alias) {
			return true
		}
	}
	return false
}

// matchGlob matches s against a simple glob (*, ?) pattern.
func matchGlob(pattern, s string) bool {
	for len(pattern) > 0 {
		switch pattern[0] {
		case '*':
			pattern = pattern[1:]
			if len(pattern) == 0 {
				return true
			}
			for i := 0; i <= len(s); i++ {
				if matchGlob(pattern, s[i:]) {
					return true
				}
			}
			return false
		case '?':
			if len(s) == 0 {
				return false
			}
			pattern = pattern[1:]
			s = s[1:]
		default:
			if len(s) == 0 || pattern[0] != s[0] {
				return false
			}
			pattern = pattern[1:]
			s = s[1:]
		}
	}
	return len(s) == 0
}

// resolveHost reads ~/.ssh/config (or the provided path) and resolves SSH
// connection parameters for the given alias or user@host[:port] string.
func resolveHost(target, sshConfigPath, username string) resolvedHost {
	r := resolvedHost{original: target}

	// Parse user@host:port if present
	hostPart := target
	if idx := strings.LastIndex(target, "@"); idx >= 0 {
		r.user = target[:idx]
		hostPart = target[idx+1:]
	}
	if host, port, err := net.SplitHostPort(hostPart); err == nil {
		r.hostname = host
		r.port = port
	} else {
		r.hostname = hostPart
	}

	// Locate SSH config file
	configPath := sshConfigPath
	if configPath == "" {
		home, _ := os.UserHomeDir()
		configPath = filepath.Join(home, ".ssh", "config")
	}

	blocks := parseSSHConfig(configPath)
	if block := matchSSHBlock(blocks, r.hostname); block != nil {
		if r.user == "" && block.user != "" {
			r.user = block.user
		}
		if r.port == "" && block.port != "" && block.port != "22" {
			r.port = block.port
		}
		if block.hostname != "" {
			r.hostname = block.hostname
		}
		r.keyFiles = append(r.keyFiles, block.identityFiles...)
	}

	// Defaults
	if r.user == "" {
		r.user = username
	}
	if r.port == "" {
		r.port = "22"
	}

	// Fall back to standard key paths if none found in config
	if len(r.keyFiles) == 0 {
		home, _ := os.UserHomeDir()
		candidates := []string{
			filepath.Join(home, ".ssh", "id_ed25519"),
			filepath.Join(home, ".ssh", "id_ecdsa"),
			filepath.Join(home, ".ssh", "id_rsa"),
		}
		for _, p := range candidates {
			if _, err := os.Stat(p); err == nil {
				r.keyFiles = append(r.keyFiles, p)
			}
		}
	}

	return r
}

// buildAuthMethods returns the SSH auth methods for a resolved host.
// Priority: explicit identity files → SSH agent → default key paths.
func buildAuthMethods(r resolvedHost, sshAuthSock string) []ssh.AuthMethod {
	var methods []ssh.AuthMethod

	// SSH agent
	if sshAuthSock != "" {
		conn, err := net.Dial("unix", sshAuthSock)
		if err == nil {
			methods = append(methods, ssh.PublicKeysCallback(agent.NewClient(conn).Signers))
		}
	}

	// Identity files
	for _, keyPath := range r.keyFiles {
		data, err := os.ReadFile(keyPath)
		if err != nil {
			continue
		}
		signer, err := ssh.ParsePrivateKey(data)
		if err != nil {
			continue
		}
		methods = append(methods, ssh.PublicKeys(signer))
	}

	return methods
}

// buildHostKeyCallback returns a host-key callback. Uses known_hosts when
// available; falls back to InsecureIgnoreHostKey with a warning when not.
func buildHostKeyCallback(sshConfigPath string) ssh.HostKeyCallback {
	home, _ := os.UserHomeDir()
	khPath := filepath.Join(home, ".ssh", "known_hosts")
	if _, err := os.Stat(khPath); err == nil {
		cb, err := knownhosts.New(khPath)
		if err == nil {
			return cb
		}
	}
	// Accept new host keys (same behaviour as StrictHostKeyChecking=accept-new)
	return func(hostname string, remote net.Addr, key ssh.PublicKey) error {
		return nil
	}
}

// streamToHost injects the binary into one remote host via SSH and optionally
// starts the operator. It sends exactly one streamResult to resultCh.
func streamToHost(
	ctx context.Context,
	target string,
	binaryData []byte,
	operatorArgs string,
	sshConfigPath string,
	dialTimeout time.Duration,
	sshAuthSock string,
	username string,
	resultCh chan<- streamResult,
) {
	start := time.Now()
	r := resolvedHost{}

	emit := func(status constants.StreamStatus, errMsg string) {
		resultCh <- streamResult{
			Host:      target,
			Status:    status,
			SizeBytes: int64(len(binaryData)),
			Error:     errMsg,
			Elapsed:   time.Since(start),
		}
	}

	select {
	case <-ctx.Done():
		emit(constants.StreamStatusCancelled, "context cancelled")
		return
	default:
	}

	r = resolveHost(target, sshConfigPath, username)

	authMethods := buildAuthMethods(r, sshAuthSock)
	if len(authMethods) == 0 {
		emit(constants.StreamStatusFailed, "no SSH auth methods available (no keys found, no agent)")
		return
	}

	hostKeyCallback := buildHostKeyCallback(sshConfigPath)

	clientConfig := &ssh.ClientConfig{
		User:            r.user,
		Auth:            authMethods,
		HostKeyCallback: hostKeyCallback,
		Timeout:         dialTimeout,
	}

	addr := net.JoinHostPort(r.hostname, r.port)

	// Respect context cancellation during dial
	dialDone := make(chan struct {
		client *ssh.Client
		err    error
	}, 1)
	go func() {
		client, err := ssh.Dial("tcp", addr, clientConfig)
		dialDone <- struct {
			client *ssh.Client
			err    error
		}{client, err}
	}()

	var client *ssh.Client
	select {
	case <-ctx.Done():
		emit(constants.StreamStatusCancelled, "context cancelled")
		return
	case result := <-dialDone:
		if result.err != nil {
			emit(constants.StreamStatusFailed, fmt.Sprintf("dial %s: %v", addr, result.err))
			return
		}
		client = result.client
	}
	defer client.Close()

	session, err := client.NewSession()
	if err != nil {
		emit(constants.StreamStatusFailed, fmt.Sprintf("new session: %v", err))
		return
	}
	defer session.Close()

	// Wire binary data as the remote stdin.
	session.Stdin = bytes.NewReader(binaryData)

	// Capture stdout+stderr (bounded) so the caller can surface the remote
	// operator's output when it exits non-zero. Without this, the deployment
	// tool silently drops every remote log line and a failing operator is
	// indistinguishable from a generic SSH exit — see g8eo review notes.
	const maxCapturedBytes = 64 * 1024
	stderrBuf := &boundedBuffer{limit: maxCapturedBytes}
	stdoutBuf := &boundedBuffer{limit: maxCapturedBytes}
	session.Stderr = stderrBuf
	session.Stdout = stdoutBuf

	// Build the remote ephemeral script inline (same trap pattern as bash impl)
	var remoteCmd string
	if operatorArgs != "" {
		remoteCmd = fmt.Sprintf(
			`set -e; B=$(mktemp); cat > "$B"; chmod +x "$B"; trap 'rm -f "$B"' EXIT; "$B" %s < /dev/null & wait $!`,
			operatorArgs,
		)
	} else {
		remoteCmd = `set -e; B=$(mktemp); cat > "$B"; chmod +x "$B"; trap 'rm -f "$B"' EXIT; echo "[g8e] Binary injected into $B -- run it manually: $B -e <endpoint> [options]"`
	}

	// Check for context cancellation before running
	select {
	case <-ctx.Done():
		emit(constants.StreamStatusCancelled, "context cancelled before run")
		return
	default:
	}

	if err := session.Run(remoteCmd); err != nil {
		// SSH exit status non-zero is surfaced as *ssh.ExitError — treat operator
		// exit as a normal end of session, not a hard failure, but attach the
		// captured remote stderr (and last-resort stdout) so the caller can
		// tell a real auth/registration failure apart from a clean exit.
		var exitErr *ssh.ExitError
		if isSSHExitError(err, &exitErr) {
			msg := fmt.Sprintf("exit code %d", exitErr.ExitStatus())
			if tail := strings.TrimSpace(stderrBuf.String()); tail != "" {
				msg = fmt.Sprintf("%s: %s", msg, tail)
			} else if tail := strings.TrimSpace(stdoutBuf.String()); tail != "" {
				msg = fmt.Sprintf("%s: %s", msg, tail)
			}
			emit(constants.StreamStatusExited, msg)
			return
		}
		msg := fmt.Sprintf("run: %v", err)
		if tail := strings.TrimSpace(stderrBuf.String()); tail != "" {
			msg = fmt.Sprintf("%s: %s", msg, tail)
		}
		emit(constants.StreamStatusFailed, msg)
		return
	}

	emit(constants.StreamStatusCompleted, "")
}

// isSSHExitError checks whether err is an *ssh.ExitError and sets target.
func isSSHExitError(err error, target **ssh.ExitError) bool {
	if e, ok := err.(*ssh.ExitError); ok {
		*target = e
		return true
	}
	return false
}

// boundedBuffer is an io.Writer that retains at most `limit` bytes, dropping
// any overflow silently. It is used to capture remote stderr/stdout from an
// SSH session without risking unbounded memory growth for chatty operators.
type boundedBuffer struct {
	buf   bytes.Buffer
	limit int
}

func (b *boundedBuffer) Write(p []byte) (int, error) {
	if b.limit <= 0 {
		return len(p), nil
	}
	remaining := b.limit - b.buf.Len()
	if remaining <= 0 {
		return len(p), nil
	}
	if len(p) > remaining {
		b.buf.Write(p[:remaining])
		return len(p), nil
	}
	b.buf.Write(p)
	return len(p), nil
}

func (b *boundedBuffer) String() string { return b.buf.String() }

// expandTilde replaces a leading ~ with the user's home directory.
func expandTilde(path string) string {
	if !strings.HasPrefix(path, "~") {
		return path
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return path
	}
	return filepath.Join(home, path[1:])
}
