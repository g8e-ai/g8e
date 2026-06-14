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
	"crypto/ed25519"
	"encoding/hex"
	"fmt"
	"io"
	"log/slog"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/g8e-ai/g8e/internal/certs"
	"github.com/g8e-ai/g8e/internal/config"
	"github.com/g8e-ai/g8e/internal/constants"
)

// testConfigCounter ensures unique OperatorIDs across parallel test calls.
var testConfigCounter atomic.Uint64

// NewTestConfig returns a test configuration with isolated workDir.
// Does NOT modify global constants.Paths to avoid data races in parallel tests.
func NewTestConfig(t *testing.T) *config.Config {
	t.Helper()

	// Ensure certs package has a valid CA configured for httpclient calls in tests
	if len(certs.GetRawCA()) == 0 {
		certs.SetCA([]byte(GenerateTestCA(t, "test-ca")))
	}
	n := testConfigCounter.Add(1)

	safeName := strings.NewReplacer("/", "-", " ", "_", ":", "-").Replace(t.Name())
	if len(safeName) > 40 {
		safeName = safeName[:40]
	}

	operatorID := fmt.Sprintf("test-op-%s-%d", safeName, n)
	operatorSessionID := fmt.Sprintf("test-sess-%s-%d", safeName, n)
	workDir := t.TempDir()

	pkiDir := filepath.Join(workDir, constants.RuntimeDirname, constants.PkiDirname)
	secretsDir := filepath.Join(workDir, constants.RuntimeDirname, constants.SecretsDirname)
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
		WorkDir:                     workDir,
		PKIDir:                      pkiDir,
		SecretsDir:                  secretsDir,
		VaultDir:                    filepath.Join(workDir, constants.RuntimeDirname, constants.VaultDirname),
		VaultKeyPath:                filepath.Join(workDir, constants.RuntimeDirname, constants.VaultDirname, constants.VaultKeyFilename),
		VaultRequireUnlock:          false,
		ExecutionVaultEnabled:       true,
		ExecutionVaultMaxSizeMB:     1024,
		ExecutionVaultRetentionDays: 30,
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
// proxies /ws/pubsub to Operator internally. Must not include a path - callers append /ws/pubsub as needed.
func GetTestOperatorDirectURL() string {
	// g8e uses ZERO environment variables - use default URL
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
		t.Fatalf("failed to listen on tcp addr: %v", err)
	}
	defer l.Close()

	return l.Addr().(*net.TCPAddr).Port
}
