// Copyright (c) 2026 Lateralus Labs, LLC.
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package testutil

import (
	"bytes"
	"crypto/ed25519"
	"encoding/hex"
	"fmt"
	"io"
	"log/slog"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/g8e-ai/g8e/internal/config"
	"github.com/g8e-ai/g8e/internal/constants"
	"github.com/g8e-ai/g8e/internal/marshaler"
)

// configCounter generates monotonically increasing IDs within a single test binary
// run so that parallel tests never share the same operator/session identity.
var configCounter atomic.Int64

// NewTestConfig returns a minimal Config for unit tests.
//
// Each call produces a unique OperatorID and OperatorSessionId derived from the
// test name and a process-local counter.  This guarantees that every test gets
// its own operator pub/sub channels and no cross-test message bleed can occur.
func NewTestConfig(t *testing.T) *config.Config {
	t.Helper()

	n := configCounter.Add(1)
	// Sanitize t.Name() so the ID is safe as a channel component.
	safeName := strings.NewReplacer("/", "-", " ", "_", ":", "-").Replace(t.Name())
	if len(safeName) > 40 {
		safeName = safeName[:40]
	}

	operatorID := fmt.Sprintf("test-op-%s-%d", safeName, n)
	operatorSessionID := fmt.Sprintf("test-sess-%s-%d", safeName, n)
	workDir := t.TempDir()
	pkiDir := filepath.Join(workDir, constants.Paths.Infra.PkiDir)
	secretsDir := filepath.Join(workDir, constants.Paths.Infra.SecretsDir)
	trustedSignersDir := filepath.Join(pkiDir, "trusted_signers")
	if err := os.MkdirAll(trustedSignersDir, 0700); err != nil {
		t.Fatalf("failed to create trusted signer directory: %v", err)
	}
	pub, _, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatalf("failed to generate test trusted signer: %v", err)
	}
	if err := os.WriteFile(filepath.Join(trustedSignersDir, "test-key.pub"), []byte(hex.EncodeToString(pub)), 0600); err != nil {
		t.Fatalf("failed to write test trusted signer: %v", err)
	}

	return &config.Config{
		ProjectID:          "test-project",
		ComponentName:      constants.ComponentNameG8EO,
		Version:            "test",
		OperatorID:         operatorID,
		OperatorSessionId:  operatorSessionID,
		PubSubURL:          GetTestOperatorDirectURL(),
		MaxConcurrentTasks: 25,
		MaxMemoryMB:        2048,
		HeartbeatInterval:  30 * time.Second,
		Gateway: config.GatewayConfig{
			MaxPayloadBytes: 10 * 1024 * 1024,
		},
		WorkDir:                 workDir,
		PKIDir:                  pkiDir,
		SecretsDir:              secretsDir,
		LocalStoreEnabled:       true,
		LocalStoreDBPath:        filepath.Join(workDir, constants.Paths.Infra.LocalStateDBPath),
		LocalStoreMaxSizeMB:     1024,
		LocalStoreRetentionDays: 30,
	}
}

// NewTestLogger returns a silent logger suitable for unit tests.
func NewTestLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{Level: slog.LevelError}))
}

// NewVerboseTestLogger returns a logger that writes to t.Log, useful for
// debugging a specific test without polluting the full test run output.
func NewVerboseTestLogger(t *testing.T) *slog.Logger {
	t.Helper()
	return slog.New(slog.NewTextHandler(testLogWriter{t: t}, &slog.HandlerOptions{Level: slog.LevelDebug}))
}

// testLogWriter bridges slog output into t.Log so it is only shown on failure.
type testLogWriter struct{ t *testing.T }

func (w testLogWriter) Write(p []byte) (int, error) {
	w.t.Log(strings.TrimRight(string(p), "\n"))
	return len(p), nil
}

// GetTestOperatorDirectURL returns the client WebSocket gateway base URL for g8eo pub/sub tests.
// g8eo connects to pub/sub via client (the single external entry point) at port 443; client
// proxies /ws/pubsub to operator internally. operator is not directly accessible from outside
// the docker network. Must not include a path - callers append /ws/pubsub as needed.
func GetTestOperatorDirectURL() string {
	if u := os.Getenv(marshaler.EnvVar(constants.EnvVar.TestOperatorPubSubURL)); u != "" {
		return u
	}
	return "wss://" + constants.DefaultEndpoint + ":443"
}

// TempFile registers a t.Cleanup to remove path when the test ends.
// Use this whenever a test needs a file outside of t.TempDir() (e.g. /tmp).
// The file is NOT created by this function - only the cleanup is registered.
func TempFile(t *testing.T, path string) {
	t.Helper()
	t.Cleanup(func() {
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			t.Logf("TempFile cleanup: failed to remove %s: %v", path, err)
		}
	})
}

// GetFreePort returns a free TCP port.
func GetFreePort(t *testing.T) int {
	t.Helper()
	addr, err := net.ResolveTCPAddr("tcp", "localhost:0")
	if err != nil {
		t.Fatalf("failed to resolve tcp addr: %v", err)
	}

	l, err := net.ListenTCP("tcp", addr)
	if err != nil {
		t.Fatalf("failed to listen on tcp: %v", err)
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port
}

// RunCLI executes the g8e binary with the given arguments and environment.
func RunCLI(t *testing.T, binPath string, args []string, env []string) (string, string, error) {
	t.Helper()
	cmd := exec.Command(binPath, args...)
	cmd.Env = append(os.Environ(), env...)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	return stdout.String(), stderr.String(), err
}

// GetTestBinaryPath returns the path to the g8e binary for integration tests.
// It checks the G8E_TEST_BINARY_PATH environment variable first, then falls back
// to a relative path from the repository root (../bin/g8e).
func GetTestBinaryPath(t *testing.T) string {
	t.Helper()

	if path := os.Getenv("G8E_TEST_BINARY_PATH"); path != "" {
		if filepath.IsAbs(path) {
			return path
		}
		absPath, err := filepath.Abs(path)
		if err != nil {
			t.Fatalf("failed to resolve absolute path for G8E_TEST_BINARY_PATH=%s: %v", path, err)
		}
		return absPath
	}

	// Fallback to relative path from test directory
	relPath := filepath.Join("..", "bin", "g8e")
	absPath, err := filepath.Abs(relPath)
	if err != nil {
		t.Fatalf("failed to resolve absolute path for %s: %v", relPath, err)
	}
	return absPath
}
