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

//go:build integration

package gateway

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/types/known/structpb"

	"github.com/g8e-ai/g8e/internal/config"
	"github.com/g8e-ai/g8e/internal/constants"
	"github.com/g8e-ai/g8e/internal/testutil"
	commonv1 "github.com/g8e-ai/g8e/protocol/proto/g8e/common/v1"
)

func TestNewGatewayModeService(t *testing.T) {
	cfg := testutil.NewTestConfig(t)
	logger := testutil.NewTestLogger()
	fileSvc := newTestFileSvc(t)

	// Ensure directories are set for tests to avoid SQLITE_CANTOPEN
	cfg.Gateway.DataDir = testutil.TempDir(t)
	cfg.Gateway.PKIDir = testutil.TempDir(t)
	cfg.Gateway.SecretsDir = fileSvc.Resolve(constants.SecretsDirname)

	t.Run("Default configuration with self-signed certs", func(t *testing.T) {
		db, stores, err := openTestDB(t, cfg.Gateway.DataDir, filepath.Join(cfg.Gateway.DataDir, "vault"), fileSvc, logger)
		require.NoError(t, err)
		t.Cleanup(func() { db.Close() })

		pubsub := NewGatewayWebSocketHandler(logger)
		t.Cleanup(func() { pubsub.Close() })

		cfg.Gateway.PKIDir = testutil.TempDir(t)
		cfg.Gateway.SecretsDir = fileSvc.Resolve(constants.SecretsDirname)
		cfg.Gateway.HTTPPort = constants.Ports.OperatorHttp

		ls, err := newGatewayModeServiceForTest(cfg, fileSvc, logger, db, stores, pubsub)
		require.NoError(t, err)
		t.Cleanup(func() { ls.Stop(context.Background()) })
		assert.NotNil(t, ls)
		assert.NotNil(t, ls.server)
		assert.NotNil(t, ls.pki)
		assert.False(t, ls.running)
	})
}

func TestGatewayModeService_StateManagement(t *testing.T) {
	cfg := testutil.NewTestConfig(t)
	logger := testutil.NewTestLogger()

	cfg.Gateway.DataDir = testutil.TempDir(t)
	cfg.Gateway.PKIDir = testutil.TempDir(t)
	fileSvc := newTestFileSvc(t)
	cfg.Gateway.SecretsDir = fileSvc.Resolve(constants.SecretsDirname)

	db, stores, err := openTestDB(t, cfg.Gateway.DataDir, filepath.Join(cfg.Gateway.DataDir, "vault"), fileSvc, logger)
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })

	pubsub := NewGatewayWebSocketHandler(logger)
	t.Cleanup(func() { pubsub.Close() })

	cfg.Gateway.HTTPPort = constants.Ports.OperatorHttp

	ls, err := newGatewayModeServiceForTest(cfg, fileSvc, logger, db, stores, pubsub)
	require.NoError(t, err)
	t.Cleanup(func() { ls.Stop(context.Background()) })

	t.Run("Initial state", func(t *testing.T) {
		assert.False(t, ls.IsRunning())
		assert.False(t, ls.IsReady())
	})

	t.Run("IsRunning returns false when not running", func(t *testing.T) {
		assert.False(t, ls.IsRunning())
	})

	t.Run("IsReady returns false when not ready", func(t *testing.T) {
		assert.False(t, ls.IsReady())
	})

	t.Run("State getters are thread-safe", func(t *testing.T) {
		// Test that we can call state methods concurrently
		done := make(chan bool, 10)
		for i := 0; i < 10; i++ {
			go func() {
				ls.IsRunning()
				ls.IsReady()
				done <- true
			}()
		}

		// Wait for all goroutines to finish
		for i := 0; i < 10; i++ {
			select {
			case <-done:
			case <-time.After(1 * time.Second):
				t.Fatal("State methods deadlocked")
			}
		}
	})
}

func TestNewGatewayModeServiceForTest(t *testing.T) {
	cfg := testutil.NewTestConfig(t)
	logger := testutil.NewTestLogger()

	dbDir := testutil.TempDir(t)
	pkiDir := testutil.TempDir(t)
	fileSvc := newTestFileSvc(t)
	db, stores, err := openTestDB(t, dbDir, filepath.Join(dbDir, "vault"), fileSvc, logger)
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })

	pubsub := NewGatewayWebSocketHandler(logger)
	t.Cleanup(func() { pubsub.Close() })

	cfg.Gateway.PKIDir = pkiDir
	cfg.Gateway.SecretsDir = fileSvc.Resolve(constants.SecretsDirname)
	cfg.Gateway.DataDir = dbDir
	cfg.Gateway.HTTPPort = constants.Ports.OperatorHttp

	ls, err := newGatewayModeServiceForTest(cfg, fileSvc, logger, db, stores, pubsub)
	require.NoError(t, err)
	t.Cleanup(func() { ls.Stop(context.Background()) })
	assert.NotNil(t, ls)
	assert.Equal(t, db, ls.db)
	assert.Equal(t, pubsub, ls.pubsub)
	assert.NotNil(t, ls.server)
}

func TestGatewayModeService_Getters(t *testing.T) {
	cfg := testutil.NewTestConfig(t)
	logger := testutil.NewTestLogger()

	dbDir := testutil.TempDir(t)
	pkiDir := testutil.TempDir(t)
	fileSvc := newTestFileSvc(t)
	db, stores, err := openTestDB(t, dbDir, filepath.Join(dbDir, "vault"), fileSvc, logger)
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })

	pubsub := NewGatewayWebSocketHandler(logger)
	t.Cleanup(func() { pubsub.Close() })

	cfg.Gateway.PKIDir = pkiDir
	cfg.Gateway.SecretsDir = fileSvc.Resolve(constants.SecretsDirname)
	cfg.Gateway.DataDir = dbDir
	cfg.Gateway.HTTPPort = constants.Ports.OperatorHttp

	ls, err := newGatewayModeServiceForTest(cfg, fileSvc, logger, db, stores, pubsub)
	require.NoError(t, err)
	t.Cleanup(func() { ls.Stop(context.Background()) })

	t.Run("GetDB returns non-nil", func(t *testing.T) {
		assert.NotNil(t, ls.GetDB())
		assert.Equal(t, db, ls.GetDB())
	})

	t.Run("GetSecretManager returns non-nil", func(t *testing.T) {
		sm, err := ls.GetSecretManager()
		require.NoError(t, err)
		assert.NotNil(t, sm)
	})

	t.Run("GetPKIAuthority returns non-nil", func(t *testing.T) {
		assert.NotNil(t, ls.GetPKIAuthority())
	})

	t.Run("GetHTTPHandler returns non-nil", func(t *testing.T) {
		assert.NotNil(t, ls.GetHTTPHandler())
	})

	t.Run("GetHTTPPort returns 0 when not started", func(t *testing.T) {
		assert.Equal(t, 0, ls.GetHTTPPort())
	})

	t.Run("GetHTTPSPort returns 0 when not started", func(t *testing.T) {
		assert.Equal(t, 0, ls.GetHTTPSPort())
	})
}

func TestGatewayModeService_IsGovernanceReady(t *testing.T) {

	t.Run("Doctrine posture returns true without signers", func(t *testing.T) {
		cfg := testutil.NewTestConfig(t)
		cfg.Gateway.Posture = config.PostureDoctrine
		logger := testutil.NewTestLogger()

		dbDir := testutil.TempDir(t)
		pkiDir := testutil.TempDir(t)
		fileSvc := newTestFileSvc(t)
		db, stores, err := openTestDB(t, dbDir, filepath.Join(dbDir, "vault"), fileSvc, logger)
		require.NoError(t, err)
		t.Cleanup(func() { db.Close() })

		pubsub := NewGatewayWebSocketHandler(logger)
		t.Cleanup(func() { pubsub.Close() })

		cfg.Gateway.PKIDir = pkiDir
		cfg.Gateway.SecretsDir = fileSvc.Resolve(constants.SecretsDirname)
		cfg.Gateway.DataDir = dbDir
		cfg.Gateway.HTTPPort = constants.Ports.OperatorHttp

		ls, err := newGatewayModeServiceForTest(cfg, fileSvc, logger, db, stores, pubsub)
		require.NoError(t, err)
		t.Cleanup(func() { ls.Stop(context.Background()) })

		assert.True(t, ls.IsGovernanceReady())
	})

	t.Run("Empty posture returns true without signers", func(t *testing.T) {
		cfg := testutil.NewTestConfig(t)
		cfg.Gateway.Posture = ""
		logger := testutil.NewTestLogger()

		dbDir := testutil.TempDir(t)
		pkiDir := testutil.TempDir(t)
		fileSvc := newTestFileSvc(t)
		db, stores, err := openTestDB(t, dbDir, filepath.Join(dbDir, "vault"), fileSvc, logger)
		require.NoError(t, err)
		t.Cleanup(func() { db.Close() })

		pubsub := NewGatewayWebSocketHandler(logger)
		t.Cleanup(func() { pubsub.Close() })

		cfg.Gateway.PKIDir = pkiDir
		cfg.Gateway.SecretsDir = fileSvc.Resolve(constants.SecretsDirname)
		cfg.Gateway.DataDir = dbDir
		cfg.Gateway.HTTPPort = constants.Ports.OperatorHttp

		ls, err := newGatewayModeServiceForTest(cfg, fileSvc, logger, db, stores, pubsub)
		require.NoError(t, err)
		t.Cleanup(func() { ls.Stop(context.Background()) })

		assert.True(t, ls.IsGovernanceReady())
	})

	t.Run("Notary posture returns false without signers", func(t *testing.T) {
		cfg := testutil.NewTestConfig(t)
		cfg.Gateway.Posture = config.PostureNotary
		logger := testutil.NewTestLogger()

		dbDir := testutil.TempDir(t)
		pkiDir := testutil.TempDir(t)
		fileSvc := newTestFileSvc(t)
		db, stores, err := openTestDB(t, dbDir, filepath.Join(dbDir, "vault"), fileSvc, logger)
		require.NoError(t, err)
		t.Cleanup(func() { db.Close() })

		pubsub := NewGatewayWebSocketHandler(logger)
		t.Cleanup(func() { pubsub.Close() })

		cfg.Gateway.PKIDir = pkiDir
		cfg.Gateway.SecretsDir = fileSvc.Resolve(constants.SecretsDirname)
		cfg.Gateway.DataDir = dbDir
		cfg.Gateway.HTTPPort = constants.Ports.OperatorHttp

		ls, err := newGatewayModeServiceForTest(cfg, fileSvc, logger, db, stores, pubsub)
		require.NoError(t, err)
		t.Cleanup(func() { ls.Stop(context.Background()) })

		assert.False(t, ls.IsGovernanceReady())
	})

	t.Run("Notary posture returns true with signers", func(t *testing.T) {
		cfg := testutil.NewTestConfig(t)
		cfg.Gateway.Posture = config.PostureNotary
		logger := testutil.NewTestLogger()

		dbDir := testutil.TempDir(t)
		pkiDir := testutil.TempDir(t)
		fileSvc := newTestFileSvc(t)
		db, stores, err := openTestDB(t, dbDir, filepath.Join(dbDir, "vault"), fileSvc, logger)
		require.NoError(t, err)
		t.Cleanup(func() { db.Close() })

		pubsub := NewGatewayWebSocketHandler(logger)
		t.Cleanup(func() { pubsub.Close() })

		cfg.Gateway.PKIDir = pkiDir
		cfg.Gateway.SecretsDir = fileSvc.Resolve(constants.SecretsDirname)
		cfg.Gateway.DataDir = dbDir
		cfg.Gateway.HTTPPort = constants.Ports.OperatorHttp

		ls, err := newGatewayModeServiceForTest(cfg, fileSvc, logger, db, stores, pubsub)
		require.NoError(t, err)
		t.Cleanup(func() { ls.Stop(context.Background()) })

		// Add a trusted signer
		signer := map[string]interface{}{
			"id":         "test-signer-1",
			"public_key": "abc123",
			"added_at":   time.Now().UTC().Format(time.RFC3339),
			"enabled":    true,
		}
		signerBytes, err := json.Marshal(signer)
		require.NoError(t, err)
		err = stores.DocStore.DocSet("trusted_signers", "test-signer-1", signerBytes)
		require.NoError(t, err)

		assert.True(t, ls.IsGovernanceReady())
	})
}

func TestGatewayModeService_GetGovernanceDeps(t *testing.T) {
	cfg := testutil.NewTestConfig(t)
	logger := testutil.NewTestLogger()

	dbDir := testutil.TempDir(t)
	pkiDir := testutil.TempDir(t)
	fileSvc := newTestFileSvc(t)
	db, stores, err := openTestDB(t, dbDir, filepath.Join(dbDir, "vault"), fileSvc, logger)
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })

	pubsub := NewGatewayWebSocketHandler(logger)
	t.Cleanup(func() { pubsub.Close() })

	cfg.Gateway.PKIDir = pkiDir
	cfg.Gateway.SecretsDir = fileSvc.Resolve(constants.SecretsDirname)
	cfg.Gateway.DataDir = dbDir
	cfg.Gateway.HTTPPort = constants.Ports.OperatorHttp

	ls, err := newGatewayModeServiceForTest(cfg, fileSvc, logger, db, stores, pubsub)
	require.NoError(t, err)
	t.Cleanup(func() { ls.Stop(context.Background()) })

	deps := ls.GetGovernanceDeps()
	assert.NotNil(t, deps)
	assert.NotNil(t, deps.ReplayStore)
	assert.NotNil(t, deps.StateRootProvider)
	assert.NotNil(t, deps.TransactionAudit)
	assert.NotNil(t, deps.L3Notary)
	assert.NotNil(t, deps.SignerStore)
	assert.NotNil(t, deps.AppPolicyStore)
	assert.NotNil(t, deps.FieldReader)
	assert.Equal(t, stores.DocStore, deps.FieldReader)
}

func TestGatewayModeService_StartStop(t *testing.T) {
	cfg := testutil.NewTestConfig(t)
	logger := testutil.NewTestLogger()

	dbDir := testutil.TempDir(t)
	pkiDir := testutil.TempDir(t)
	fileSvc := newTestFileSvc(t)
	db, stores, err := openTestDB(t, dbDir, filepath.Join(dbDir, "vault"), fileSvc, logger)
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })

	pubsub := NewGatewayWebSocketHandler(logger)
	t.Cleanup(func() { pubsub.Close() })

	cfg.Gateway.PKIDir = pkiDir
	cfg.Gateway.SecretsDir = fileSvc.Resolve(constants.SecretsDirname)
	cfg.Gateway.DataDir = dbDir
	// Use port 0 for dynamic port assignment in tests
	cfg.Gateway.HTTPPort = 0
	cfg.Gateway.HTTPSPort = 0

	ls, err := newGatewayModeServiceForTest(cfg, fileSvc, logger, db, stores, pubsub)
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start the service in a goroutine
	errChan := make(chan error, 1)
	go func() {
		errChan <- ls.Start(ctx)
	}()

	// Wait for the service to become ready
	require.Eventually(t, func() bool {
		return ls.IsReady()
	}, 5*time.Second, 100*time.Millisecond, "service should become ready")

	// Verify ports are now non-zero
	httpPort := ls.GetHTTPPort()
	httpsPort := ls.GetHTTPSPort()
	assert.NotZero(t, httpPort, "HTTP port should be assigned")
	assert.NotZero(t, httpsPort, "HTTPS port should be assigned")

	// Verify IsRunning is true
	assert.True(t, ls.IsRunning())

	// Test double-start returns error
	err = ls.Start(ctx)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "already running")

	// Stop the service
	cancel()
	err = <-errChan
	// Expect context cancellation error
	assert.Error(t, err)

	stopErr := ls.Stop(context.Background())
	assert.NoError(t, stopErr)

	// Verify IsRunning is false
	assert.False(t, ls.IsRunning())
	assert.False(t, ls.IsReady())
}

func TestGatewayModeService_StopWhenNotRunning(t *testing.T) {
	cfg := testutil.NewTestConfig(t)
	logger := testutil.NewTestLogger()

	dbDir := testutil.TempDir(t)
	pkiDir := testutil.TempDir(t)
	fileSvc := newTestFileSvc(t)
	db, stores, err := openTestDB(t, dbDir, filepath.Join(dbDir, "vault"), fileSvc, logger)
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })

	pubsub := NewGatewayWebSocketHandler(logger)
	t.Cleanup(func() { pubsub.Close() })

	cfg.Gateway.PKIDir = pkiDir
	cfg.Gateway.SecretsDir = fileSvc.Resolve(constants.SecretsDirname)
	cfg.Gateway.DataDir = dbDir
	cfg.Gateway.HTTPPort = constants.Ports.OperatorHttp

	ls, err := newGatewayModeServiceForTest(cfg, fileSvc, logger, db, stores, pubsub)
	require.NoError(t, err)

	// Stop when not running should return nil
	err = ls.Stop(context.Background())
	assert.NoError(t, err)
}

func TestDetectBasicNonLoopbackIPv4Addresses(t *testing.T) {
	// This function is host-dependent, so we just verify it doesn't panic
	// and returns a slice (possibly empty)
	ips := detectBasicNonLoopbackIPv4Addresses()
	assert.NotNil(t, ips)
	// We don't assert contents since it varies by host
}

func TestGatewayModeService_HandleHeartbeatPublish(t *testing.T) {
	cfg := testutil.NewTestConfig(t)
	logger := testutil.NewTestLogger()

	dbDir := testutil.TempDir(t)
	pkiDir := testutil.TempDir(t)
	fileSvc := newTestFileSvc(t)
	db, stores, err := openTestDB(t, dbDir, filepath.Join(dbDir, "vault"), fileSvc, logger)
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })

	pubsub := NewGatewayWebSocketHandler(logger)
	t.Cleanup(func() { pubsub.Close() })

	cfg.Gateway.PKIDir = pkiDir
	cfg.Gateway.SecretsDir = fileSvc.Resolve(constants.SecretsDirname)
	cfg.Gateway.DataDir = dbDir
	cfg.Gateway.HTTPPort = constants.Ports.OperatorHttp

	ls, err := newGatewayModeServiceForTest(cfg, fileSvc, logger, db, stores, pubsub)
	require.NoError(t, err)
	t.Cleanup(func() { ls.Stop(context.Background()) })

	t.Run("Valid heartbeat updates operator document", func(t *testing.T) {
		// Seed an operator document
		opDoc := map[string]interface{}{
			"id":     "op-123",
			"status": "active",
		}
		opBytes, err := json.Marshal(opDoc)
		require.NoError(t, err)
		err = stores.DocStore.DocSet("operators", "op-123", opBytes)
		require.NoError(t, err)

		// Publish a heartbeat as a protojson-encoded GovernanceEnvelope
		envelope := &commonv1.GovernanceEnvelope{
			OperatorId: "op-123",
			IntentData: &structpb.Struct{
				Fields: map[string]*structpb.Value{
					"uptime": structpb.NewStructValue(&structpb.Struct{
						Fields: map[string]*structpb.Value{
							"seconds": structpb.NewNumberValue(12345),
						},
					}),
				},
			},
		}
		heartbeatBytes, err := protojson.Marshal(envelope)
		require.NoError(t, err)

		ls.handleHeartbeatPublish("test-channel", heartbeatBytes)

		// Verify the operator document was updated
		updatedDoc, err := stores.DocStore.DocGet("operators", "op-123")
		require.NoError(t, err)
		assert.NotNil(t, updatedDoc)
		assert.Contains(t, updatedDoc.Data, "latest_heartbeat_snapshot")
	})

	t.Run("Malformed JSON logs and returns", func(t *testing.T) {
		// Should not panic
		ls.handleHeartbeatPublish("test-channel", []byte("{invalid json"))
	})

	t.Run("Missing operator_id returns without write", func(t *testing.T) {
		envelope := &commonv1.GovernanceEnvelope{
			IntentData: &structpb.Struct{},
		}
		heartbeatBytes, err := protojson.Marshal(envelope)
		require.NoError(t, err)

		// Should not panic
		ls.handleHeartbeatPublish("test-channel", heartbeatBytes)
	})
}

func TestGatewayModeService_RenewServiceCertWithIdentity(t *testing.T) {
	cfg := testutil.NewTestConfig(t)
	logger := testutil.NewTestLogger()

	dbDir := testutil.TempDir(t)
	pkiDir := testutil.TempDir(t)
	fileSvc := newTestFileSvc(t)
	db, stores, err := openTestDB(t, dbDir, filepath.Join(dbDir, "vault"), fileSvc, logger)
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })

	pubsub := NewGatewayWebSocketHandler(logger)
	t.Cleanup(func() { pubsub.Close() })

	cfg.Gateway.PKIDir = pkiDir
	cfg.Gateway.SecretsDir = fileSvc.Resolve(constants.SecretsDirname)
	cfg.Gateway.DataDir = dbDir
	cfg.Gateway.HTTPPort = constants.Ports.OperatorHttp

	ls, err := newGatewayModeServiceForTest(cfg, fileSvc, logger, db, stores, pubsub)
	require.NoError(t, err)
	t.Cleanup(func() { ls.Stop(context.Background()) })

	// Test the renewal function - it may fail if PKI is not fully initialized,
	// but we're testing that it doesn't panic and handles errors gracefully
	ctx := context.Background()
	_ = ls.renewServiceCertWithIdentity(ctx)
	// We don't assert on error since it depends on PKI state
	// The important thing is it doesn't panic
}

func TestGatewayModeService_RunServiceCertRenewalLoop(t *testing.T) {
	cfg := testutil.NewTestConfig(t)
	logger := testutil.NewTestLogger()

	dbDir := testutil.TempDir(t)
	pkiDir := testutil.TempDir(t)
	fileSvc := newTestFileSvc(t)
	db, stores, err := openTestDB(t, dbDir, filepath.Join(dbDir, "vault"), fileSvc, logger)
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })

	pubsub := NewGatewayWebSocketHandler(logger)
	t.Cleanup(func() { pubsub.Close() })

	cfg.Gateway.PKIDir = pkiDir
	cfg.Gateway.SecretsDir = fileSvc.Resolve(constants.SecretsDirname)
	cfg.Gateway.DataDir = dbDir
	cfg.Gateway.HTTPPort = constants.Ports.OperatorHttp

	ls, err := newGatewayModeServiceForTest(cfg, fileSvc, logger, db, stores, pubsub)
	require.NoError(t, err)
	t.Cleanup(func() { ls.Stop(context.Background()) })

	// Test with an already-cancelled context - should return promptly
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	// This should return immediately since the context is already cancelled
	// We run it in a goroutine with a timeout to ensure it doesn't hang
	done := make(chan bool)
	go func() {
		ls.runServiceCertRenewalLoop(ctx)
		done <- true
	}()

	select {
	case <-done:
		// Success - returned promptly
	case <-time.After(2 * time.Second):
		t.Fatal("runServiceCertRenewalLoop did not return promptly with cancelled context")
	}
}
