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
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/g8e-ai/g8e/internal/certs"
	"github.com/g8e-ai/g8e/internal/config"
	"github.com/g8e-ai/g8e/internal/constants"
)

// testConfigCounter ensures unique OperatorIDs across parallel test calls.
var testConfigCounter atomic.Uint64

// testTrustStore is a shared TrustStore for tests, replacing the deprecated
// global certs.SetCA/GetRawCA pattern.
var (
	testTrustStoreOnce sync.Once
	testTrustStore     *certs.TrustStore
)

// GetTestTrustStore returns a shared TrustStore initialized with a test CA.
// It replaces the deprecated certs.GetRawCA() + certs.NewTrustStore() pattern.
func GetTestTrustStore() *certs.TrustStore {
	testTrustStoreOnce.Do(func() {
		testTrustStore = certs.NewTrustStore(nil)
	})
	return testTrustStore
}

// EnsureTestCA ensures the shared test TrustStore has a CA configured.
// Call this from test helpers that need TLS to work.
func EnsureTestCA(t *testing.T) {
	t.Helper()
	ts := GetTestTrustStore()
	if len(ts.GetRawCA()) == 0 {
		ts.SetCA([]byte(GenerateTestCA(t, "test-ca")))
	}
}

// NewTestConfig returns a test configuration with isolated workDir.
// Does NOT modify global constants.Paths to avoid data races in parallel tests.
func NewTestConfig(t *testing.T) *config.Config {
	t.Helper()

	// Ensure shared test TrustStore has a valid CA configured
	EnsureTestCA(t)
	n := testConfigCounter.Add(1)

	safeName := strings.NewReplacer("/", "-", " ", "_", ":", "-").Replace(t.Name())
	if len(safeName) > 40 {
		safeName = safeName[:40]
	}

	operatorID := fmt.Sprintf("test-op-%s-%d", safeName, n)
	operatorSessionID := fmt.Sprintf("test-sess-%s-%d", safeName, n)
	workDir := TempDir(t)

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
		// Tests use a mock L3Notary, so NotaryPosture mirrors the prior default
		// behavior. The operator has no posture of its own; this represents the
		// gateway-provided posture that the operator would receive at enrollment.
		Posture: config.PostureNotary,
		Gateway: config.GatewayConfig{
			MaxPayloadBytes: 10 * 1024 * 1024,
			CertMode:        "localhost",
			PasskeyRpID:     "localhost",
			PasskeyRpName:   "g8e",
			Posture:         config.PostureNotary,
		},
		WorkDir:                     workDir,
		PKIDir:                      pkiDir,
		SecretsDir:                  secretsDir,
		VaultDir:                    filepath.Join(workDir, constants.RuntimeDirname, constants.VaultDirname),
		VaultKeyPath:                filepath.Join(workDir, constants.RuntimeDirname, constants.VaultDirname, constants.VaultKeyFilename),
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
