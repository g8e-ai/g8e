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

package pubsub

import (
	"context"
	"crypto/ed25519"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/g8e-ai/g8e/internal/models"
	pubsubtest "github.com/g8e-ai/g8e/internal/services/pubsub/pubsubtest"
	"github.com/g8e-ai/g8e/internal/services/vault"
	"github.com/g8e-ai/g8e/internal/testutil"
	operatorv1 "github.com/g8e-ai/g8e/protocol/proto/g8e/operator/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
)

func TestNewHistoryService(t *testing.T) {
	t.Run("creates service successfully", func(t *testing.T) {
		t.Parallel()
		cfg := testutil.NewTestConfig(t)
		logger := testutil.NewTestLogger()
		client := pubsubtest.NewMockOperatorPubSubClient()
		svc := NewHistoryService(cfg, logger, client)
		require.NotNil(t, svc)
		assert.Equal(t, cfg, svc.config)
		assert.Equal(t, logger, svc.logger)
		assert.Equal(t, client, svc.client)
	})
}

func TestHistoryService_SetAuditStore(t *testing.T) {
	t.Run("sets audit store for observed-state content evidence", func(t *testing.T) {
		t.Parallel()
		cfg := testutil.NewTestConfig(t)
		logger := testutil.NewTestLogger()
		client := pubsubtest.NewMockOperatorPubSubClient()
		svc := NewHistoryService(cfg, logger, client)

		// Create a mock audit store
		mockAuditStore := &mockAuditEventRecorder{}
		svc.SetAuditStore(mockAuditStore)

		assert.Equal(t, mockAuditStore, svc.auditStore)
	})

	t.Run("sets nil audit store", func(t *testing.T) {
		t.Parallel()
		cfg := testutil.NewTestConfig(t)
		logger := testutil.NewTestLogger()
		client := pubsubtest.NewMockOperatorPubSubClient()
		svc := NewHistoryService(cfg, logger, client)

		svc.SetAuditStore(nil)
		assert.Nil(t, svc.auditStore)
	})
}

func TestHistoryService_HandleFetchLogsRequest(t *testing.T) {
	t.Run("rejects invalid protobuf payload", func(t *testing.T) {
		t.Parallel()
		cfg := testutil.NewTestConfig(t)
		logger := testutil.NewTestLogger()
		client := pubsubtest.NewMockOperatorPubSubClient()
		svc := NewHistoryService(cfg, logger, client)

		msg := &PubSubCommandMessage{
			Payload: []byte("invalid protobuf"),
		}
		svc.HandleFetchLogsRequest(context.Background(), msg)

		published := client.LastPublished()
		require.NotNil(t, published)
		assert.Contains(t, string(published.Data), "invalid request payload")
	})

	t.Run("rejects missing execution_id", func(t *testing.T) {
		t.Parallel()
		cfg := testutil.NewTestConfig(t)
		logger := testutil.NewTestLogger()
		client := pubsubtest.NewMockOperatorPubSubClient()
		svc := NewHistoryService(cfg, logger, client)

		req := &operatorv1.FetchLogsRequested{ExecutionId: ""}
		payload, err := proto.Marshal(req)
		require.NoError(t, err)

		msg := &PubSubCommandMessage{
			Payload: payload,
		}
		svc.HandleFetchLogsRequest(context.Background(), msg)

		published := client.LastPublished()
		require.NotNil(t, published)
		assert.Contains(t, string(published.Data), "missing execution_id")
	})

	t.Run("rejects when local store not available", func(t *testing.T) {
		t.Parallel()
		cfg := testutil.NewTestConfig(t)
		logger := testutil.NewTestLogger()
		client := pubsubtest.NewMockOperatorPubSubClient()
		svc := NewHistoryService(cfg, logger, client)

		req := &operatorv1.FetchLogsRequested{ExecutionId: "exec-1"}
		payload, err := proto.Marshal(req)
		require.NoError(t, err)

		msg := &PubSubCommandMessage{
			Payload: payload,
		}
		svc.HandleFetchLogsRequest(context.Background(), msg)

		published := client.LastPublished()
		require.NotNil(t, published)
		assert.Contains(t, string(published.Data), "consolidated execution vault is not available")
	})
}

func TestHistoryService_HandleFetchHistoryRequest(t *testing.T) {
	t.Run("rejects when history handler not available", func(t *testing.T) {
		t.Parallel()
		cfg := testutil.NewTestConfig(t)
		logger := testutil.NewTestLogger()
		client := pubsubtest.NewMockOperatorPubSubClient()
		svc := NewHistoryService(cfg, logger, client)

		msg := &PubSubCommandMessage{
			Payload: []byte("{}"),
		}
		svc.HandleFetchHistoryRequest(context.Background(), msg)

		published := client.LastPublished()
		require.NotNil(t, published)
		assert.Contains(t, string(published.Data), "history handler not available")
	})
}

func TestHistoryService_HandleFetchFileHistoryRequest(t *testing.T) {
	t.Run("rejects invalid protobuf payload", func(t *testing.T) {
		t.Parallel()
		cfg := testutil.NewTestConfig(t)
		logger := testutil.NewTestLogger()
		client := pubsubtest.NewMockOperatorPubSubClient()
		svc := NewHistoryService(cfg, logger, client)

		msg := &PubSubCommandMessage{
			Payload: []byte("invalid protobuf"),
		}
		svc.HandleFetchFileHistoryRequest(context.Background(), msg)

		published := client.LastPublished()
		require.NotNil(t, published)
		assert.Contains(t, string(published.Data), "invalid request payload")
	})

	t.Run("rejects when history handler not available", func(t *testing.T) {
		t.Parallel()
		cfg := testutil.NewTestConfig(t)
		logger := testutil.NewTestLogger()
		client := pubsubtest.NewMockOperatorPubSubClient()
		svc := NewHistoryService(cfg, logger, client)

		req := &operatorv1.FetchFileHistoryRequested{FilePath: "/test/file.txt"}
		payload, err := proto.Marshal(req)
		require.NoError(t, err)

		msg := &PubSubCommandMessage{
			Payload: payload,
		}
		svc.HandleFetchFileHistoryRequest(context.Background(), msg)

		published := client.LastPublished()
		require.NotNil(t, published)
		assert.Contains(t, string(published.Data), "history handler not available")
	})
}

func TestHistoryService_HandleRestoreFileRequest(t *testing.T) {
	t.Run("rejects invalid protobuf payload", func(t *testing.T) {
		t.Parallel()
		cfg := testutil.NewTestConfig(t)
		logger := testutil.NewTestLogger()
		client := pubsubtest.NewMockOperatorPubSubClient()
		svc := NewHistoryService(cfg, logger, client)

		msg := &PubSubCommandMessage{
			Payload: []byte("invalid protobuf"),
		}
		svc.HandleRestoreFileRequest(context.Background(), msg)

		published := client.LastPublished()
		require.NotNil(t, published)
		assert.Contains(t, string(published.Data), "invalid request payload")
	})

	t.Run("rejects when history handler not available", func(t *testing.T) {
		t.Parallel()
		cfg := testutil.NewTestConfig(t)
		logger := testutil.NewTestLogger()
		client := pubsubtest.NewMockOperatorPubSubClient()
		svc := NewHistoryService(cfg, logger, client)

		req := &operatorv1.RestoreFileRequested{
			FilePath:   "/test/file.txt",
			CommitHash: "abc123",
		}
		payload, err := proto.Marshal(req)
		require.NoError(t, err)

		msg := &PubSubCommandMessage{
			Payload: payload,
		}
		svc.HandleRestoreFileRequest(context.Background(), msg)

		published := client.LastPublished()
		require.NotNil(t, published)
		assert.Contains(t, string(published.Data), "history handler not available")
	})
}

func TestHistoryService_HandleFetchFileDiffRequest(t *testing.T) {
	t.Run("rejects when local store not available", func(t *testing.T) {
		t.Parallel()
		cfg := testutil.NewTestConfig(t)
		logger := testutil.NewTestLogger()
		client := pubsubtest.NewMockOperatorPubSubClient()
		svc := NewHistoryService(cfg, logger, client)

		req := &operatorv1.FetchFileDiffRequested{DiffId: "diff-1"}
		payload, _ := proto.Marshal(req)
		msg := &PubSubCommandMessage{
			Payload: payload,
		}
		svc.HandleFetchFileDiffRequest(context.Background(), msg)

		published := client.LastPublished()
		require.NotNil(t, published)
		assert.Contains(t, string(published.Data), "execution vault not available on this operator")
	})

	t.Run("rejects invalid protobuf payload when local store is available", func(t *testing.T) {
		t.Parallel()
		cfg := testutil.NewTestConfig(t)
		logger := testutil.NewTestLogger()
		client := pubsubtest.NewMockOperatorPubSubClient()
		svc := NewHistoryService(cfg, logger, client)

		// Create vault
		_, privKey, err := ed25519.GenerateKey(nil)
		require.NoError(t, err)
		tmpDir := t.TempDir()
		vaultDir := filepath.Join(tmpDir, "vault")
		require.NoError(t, os.MkdirAll(vaultDir, 0700))
		vHeader, _, err := vault.NewVaultHeader(privKey)
		require.NoError(t, err)
		require.NoError(t, vHeader.Save(vaultDir))
		testVault, err := vault.NewVault(&vault.VaultConfig{DataDir: vaultDir, Logger: logger})
		require.NoError(t, err)
		require.NoError(t, testVault.Unlock(privKey))
		defer testVault.Close()

		// Set executionVault directly since there's no setter method
		mockVault := &mockExecutionVault{}
		svc.executionVault = mockVault

		msg := &PubSubCommandMessage{
			Payload: []byte("invalid protobuf"),
		}
		svc.HandleFetchFileDiffRequest(context.Background(), msg)

		published := client.LastPublished()
		require.NotNil(t, published)
		assert.Contains(t, string(published.Data), "invalid request payload")
	})

	t.Run("rejects when neither diff_id nor operator_session_id provided", func(t *testing.T) {
		t.Parallel()
		cfg := testutil.NewTestConfig(t)
		logger := testutil.NewTestLogger()
		client := pubsubtest.NewMockOperatorPubSubClient()
		svc := NewHistoryService(cfg, logger, client)

		// Create vault
		_, privKey, err := ed25519.GenerateKey(nil)
		require.NoError(t, err)
		tmpDir := t.TempDir()
		vaultDir := filepath.Join(tmpDir, "vault")
		require.NoError(t, os.MkdirAll(vaultDir, 0700))
		vHeader, _, err := vault.NewVaultHeader(privKey)
		require.NoError(t, err)
		require.NoError(t, vHeader.Save(vaultDir))
		testVault, err := vault.NewVault(&vault.VaultConfig{DataDir: vaultDir, Logger: logger})
		require.NoError(t, err)
		require.NoError(t, testVault.Unlock(privKey))
		defer testVault.Close()

		// Set executionVault directly since there's no setter method
		mockVault := &mockExecutionVault{}
		svc.executionVault = mockVault

		req := &operatorv1.FetchFileDiffRequested{}
		payload, _ := proto.Marshal(req)
		msg := &PubSubCommandMessage{
			Payload: payload,
		}
		svc.HandleFetchFileDiffRequest(context.Background(), msg)

		published := client.LastPublished()
		require.NotNil(t, published)
		assert.Contains(t, string(published.Data), "either diff_id or operator_session_id is required")
	})
}

func TestHistoryService_publishFetchLogsResult(t *testing.T) {
	t.Run("publishes result successfully", func(t *testing.T) {
		t.Parallel()
		cfg := testutil.NewTestConfig(t)
		logger := testutil.NewTestLogger()
		client := pubsubtest.NewMockOperatorPubSubClient()
		svc := NewHistoryService(cfg, logger, client)

		record := &models.ExecutionRecord{
			ID:               "exec-1",
			Command:          "ls -la",
			ExitCode:         0,
			DurationMs:       1000,
			StdoutCompressed: []byte("stdout data"),
			StderrCompressed: []byte("stderr data"),
			StdoutSize:       10,
			StderrSize:       10,
			TimestampUTC:     time.Now().UTC(),
		}

		msg := &PubSubCommandMessage{ID: "msg-1"}
		svc.publishFetchLogsResult(context.Background(), msg, record)

		published := client.LastPublished()
		require.NotNil(t, published)
		assert.Contains(t, string(published.Data), "ls -la")
	})
}
