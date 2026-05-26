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
	"testing"
	"time"

	storage "github.com/g8e-ai/g8e/internal/services/storage"
	"github.com/g8e-ai/g8e/internal/services/system"
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
		client := NewMockOperatorPubSubClient()
		svc := NewHistoryService(cfg, logger, client)
		require.NotNil(t, svc)
		assert.Equal(t, cfg, svc.config)
		assert.Equal(t, logger, svc.logger)
		assert.Equal(t, client, svc.client)
	})
}

func TestHistoryService_HandleFetchLogsRequest(t *testing.T) {
	t.Run("rejects invalid protobuf payload", func(t *testing.T) {
		t.Parallel()
		cfg := testutil.NewTestConfig(t)
		logger := testutil.NewTestLogger()
		client := NewMockOperatorPubSubClient()
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
		client := NewMockOperatorPubSubClient()
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
		client := NewMockOperatorPubSubClient()
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
		assert.Contains(t, string(published.Data), "consolidated execution vault is not enabled")
	})
}

func TestHistoryService_HandleFetchHistoryRequest(t *testing.T) {
	t.Run("rejects when history handler not available", func(t *testing.T) {
		t.Parallel()
		cfg := testutil.NewTestConfig(t)
		logger := testutil.NewTestLogger()
		client := NewMockOperatorPubSubClient()
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
		client := NewMockOperatorPubSubClient()
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
		client := NewMockOperatorPubSubClient()
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
		client := NewMockOperatorPubSubClient()
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
		client := NewMockOperatorPubSubClient()
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
		client := NewMockOperatorPubSubClient()
		svc := NewHistoryService(cfg, logger, client)

		msg := &PubSubCommandMessage{
			Payload: []byte("{}"),
		}
		svc.HandleFetchFileDiffRequest(context.Background(), msg)

		published := client.LastPublished()
		require.NotNil(t, published)
		assert.Contains(t, string(published.Data), "local storage not available")
	})
}

func TestHistoryService_publishFetchLogsResult(t *testing.T) {
	t.Run("publishes result successfully", func(t *testing.T) {
		t.Parallel()
		cfg := testutil.NewTestConfig(t)
		logger := testutil.NewTestLogger()
		client := NewMockOperatorPubSubClient()
		svc := NewHistoryService(cfg, logger, client)

		record := &storage.ExecutionRecord{
			ID:               "exec-1",
			Command:          "ls -la",
			ExitCode:         system.IntPtr(0),
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
