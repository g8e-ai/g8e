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

	"github.com/g8e-ai/g8e/components/g8eo/constants"
	"github.com/g8e-ai/g8e/components/g8eo/models"
	storage "github.com/g8e-ai/g8e/components/g8eo/services/storage"
	"github.com/g8e-ai/g8e/components/g8eo/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestHistoryService(t *testing.T) (*HistoryService, *MockG8esPubSubClient) {
	t.Helper()
	cfg := testutil.NewTestConfig(t)
	logger := testutil.NewTestLogger()
	db := NewMockG8esPubSubClient()
	t.Cleanup(func() { db.Close() })
	return NewHistoryService(cfg, logger, db), db
}

// ---------------------------------------------------------------------------
// NewHistoryService
// ---------------------------------------------------------------------------

func TestNewHistoryService_CreatesService(t *testing.T) {
	hs, _ := newTestHistoryService(t)
	require.NotNil(t, hs)
	assert.NotNil(t, hs.config)
	assert.Nil(t, hs.localStore)
	assert.Nil(t, hs.rawVault)
	assert.Nil(t, hs.historyHandler)
}

// ---------------------------------------------------------------------------
// SetLocalStoreService
// ---------------------------------------------------------------------------

func TestHistoryService_SetLocalStoreService(t *testing.T) {
	hs, _ := newTestHistoryService(t)
	ls := newTestLocalStore(t)

	hs.localStore = ls
	assert.Equal(t, ls, hs.localStore)
}

// ---------------------------------------------------------------------------
// SetRawVaultService
// ---------------------------------------------------------------------------

func TestHistoryService_SetRawVaultService(t *testing.T) {
	hs, _ := newTestHistoryService(t)
	rv := newTestRawVault(t)

	hs.rawVault = rv
	assert.Equal(t, rv, hs.rawVault)
}

// ---------------------------------------------------------------------------
// SetHistoryHandler
// ---------------------------------------------------------------------------

func TestHistoryService_SetHistoryHandler(t *testing.T) {
	hs, _ := newTestHistoryService(t)

	avConfig := storage.AuditVaultConfig{
		Enabled:                   true,
		DataDir:                   t.TempDir(),
		DBPath:                    "audit.db",
		LedgerDir:                 "ledger",
		MaxDBSizeMB:               100,
		RetentionDays:             30,
		PruneIntervalMinutes:      60,
		OutputTruncationThreshold: 102400,
		HeadTailSize:              51200,
	}
	avs, err := storage.NewAuditVaultService(&avConfig, testutil.NewTestLogger())
	require.NoError(t, err)
	t.Cleanup(func() { avs.Close() })

	hh := storage.NewHistoryHandler(avs, nil, testutil.NewTestLogger())
	hs.historyHandler = hh
	assert.Equal(t, hh, hs.historyHandler)
}

// ---------------------------------------------------------------------------
// HandleFetchLogsRequest — invalid payload
// ---------------------------------------------------------------------------

func TestHistoryService_HandleFetchLogsRequest_InvalidPayload(t *testing.T) {
	hs, db := newTestHistoryService(t)

	msg := PubSubCommandMessage{
		ID:      "msg-logs-1",
		CaseID:  "case-1",
		Payload: []byte(`{invalid}`),
	}
	hs.HandleFetchLogsRequest(context.Background(), msg)

	published := db.LastPublished()
	require.NotNil(t, published)
	assert.Contains(t, string(published.Data), constants.Event.Operator.FetchLogs.Failed)
}

// ---------------------------------------------------------------------------
// HandleFetchLogsRequest — missing execution_id
// ---------------------------------------------------------------------------

func TestHistoryService_HandleFetchLogsRequest_MissingExecutionID(t *testing.T) {
	hs, db := newTestHistoryService(t)

	msg := PubSubCommandMessage{
		ID:      "msg-logs-2",
		CaseID:  "case-1",
		Payload: mustMarshalJSON(t, models.FetchLogsRequestPayload{ExecutionID: ""}),
	}
	hs.HandleFetchLogsRequest(context.Background(), msg)

	published := db.LastPublished()
	require.NotNil(t, published)
	assert.Contains(t, string(published.Data), constants.Event.Operator.FetchLogs.Failed)
}

// ---------------------------------------------------------------------------
// HandleFetchLogsRequest — scrubbed vault not available
// ---------------------------------------------------------------------------

func TestHistoryService_HandleFetchLogsRequest_ScrubbedVaultNotAvailable(t *testing.T) {
	hs, db := newTestHistoryService(t)

	msg := PubSubCommandMessage{
		ID:     "msg-logs-3",
		CaseID: "case-1",
		Payload: mustMarshalJSON(t, models.FetchLogsRequestPayload{
			ExecutionID:  "exec-123",
			SentinelMode: constants.Status.VaultMode.Scrubbed,
		}),
	}
	hs.HandleFetchLogsRequest(context.Background(), msg)

	published := db.LastPublished()
	require.NotNil(t, published)
	assert.Contains(t, string(published.Data), constants.Event.Operator.FetchLogs.Failed)
}

// ---------------------------------------------------------------------------
// HandleFetchLogsRequest — raw vault not available falls back to scrubbed
// ---------------------------------------------------------------------------

func TestHistoryService_HandleFetchLogsRequest_RawVaultNotAvailable_FallsBack(t *testing.T) {
	hs, db := newTestHistoryService(t)

	msg := PubSubCommandMessage{
		ID:     "msg-logs-4",
		CaseID: "case-1",
		Payload: mustMarshalJSON(t, models.FetchLogsRequestPayload{
			ExecutionID:  "exec-456",
			SentinelMode: constants.Status.VaultMode.Raw,
		}),
	}
	hs.HandleFetchLogsRequest(context.Background(), msg)

	published := db.LastPublished()
	require.NotNil(t, published)
	assert.Contains(t, string(published.Data), constants.Event.Operator.FetchLogs.Failed)
}

// ---------------------------------------------------------------------------
// HandleFetchLogsRequest — raw vault present, execution not found
// ---------------------------------------------------------------------------

func TestHistoryService_HandleFetchLogsRequest_RawVaultPresent_ExecutionNotFound(t *testing.T) {
	hs, db := newTestHistoryService(t)
	rv := newTestRawVault(t)
	hs.rawVault = rv

	msg := PubSubCommandMessage{
		ID:     "msg-logs-5",
		CaseID: "case-1",
		Payload: mustMarshalJSON(t, models.FetchLogsRequestPayload{
			ExecutionID:  "nonexistent-exec",
			SentinelMode: constants.Status.VaultMode.Raw,
		}),
	}
	hs.HandleFetchLogsRequest(context.Background(), msg)

	published := db.LastPublished()
	require.NotNil(t, published)
	assert.Contains(t, string(published.Data), constants.Event.Operator.FetchLogs.Failed)
}

// ---------------------------------------------------------------------------
// HandleFetchHistoryRequest — history handler not available
// ---------------------------------------------------------------------------

func TestHistoryService_HandleFetchHistoryRequest_HandlerNotAvailable(t *testing.T) {
	hs, db := newTestHistoryService(t)

	msg := PubSubCommandMessage{
		ID:      "msg-hist-1",
		CaseID:  "case-1",
		Payload: mustMarshalJSON(t, map[string]string{}),
	}
	hs.HandleFetchHistoryRequest(context.Background(), msg)

	published := db.LastPublished()
	require.NotNil(t, published)
	assert.Contains(t, string(published.Data), constants.Event.Operator.FetchHistory.Failed)
}

// ---------------------------------------------------------------------------
// HandleFetchFileHistoryRequest — invalid payload
// ---------------------------------------------------------------------------

func TestHistoryService_HandleFetchFileHistoryRequest_InvalidPayload(t *testing.T) {
	hs, db := newTestHistoryService(t)

	msg := PubSubCommandMessage{
		ID:      "msg-filehist-1",
		CaseID:  "case-1",
		Payload: []byte(`{invalid}`),
	}
	hs.HandleFetchFileHistoryRequest(context.Background(), msg)

	published := db.LastPublished()
	require.NotNil(t, published)
	assert.Contains(t, string(published.Data), constants.Event.Operator.FetchFileHistory.Failed)
}

// ---------------------------------------------------------------------------
// HandleRestoreFileRequest — invalid payload
// ---------------------------------------------------------------------------

func TestHistoryService_HandleRestoreFileRequest_InvalidPayload(t *testing.T) {
	hs, db := newTestHistoryService(t)

	msg := PubSubCommandMessage{
		ID:      "msg-restore-1",
		CaseID:  "case-1",
		Payload: []byte(`{invalid}`),
	}
	hs.HandleRestoreFileRequest(context.Background(), msg)

	published := db.LastPublished()
	require.NotNil(t, published)
	assert.Contains(t, string(published.Data), constants.Event.Operator.RestoreFile.Failed)
}

// ---------------------------------------------------------------------------
// HandleFetchFileDiffRequest — local store not available
// ---------------------------------------------------------------------------

func TestHistoryService_HandleFetchFileDiffRequest_LocalStoreNotAvailable(t *testing.T) {
	hs, db := newTestHistoryService(t)

	msg := PubSubCommandMessage{
		ID:      "msg-diff-1",
		CaseID:  "case-1",
		Payload: mustMarshalJSON(t, models.FetchFileDiffRequestPayload{DiffID: "diff-123"}),
	}
	hs.HandleFetchFileDiffRequest(context.Background(), msg)

	published := db.LastPublished()
	require.NotNil(t, published)
	assert.Contains(t, string(published.Data), constants.Event.Operator.FetchFileDiff.Failed)
}

// ---------------------------------------------------------------------------
// HandleFetchFileDiffRequest — missing both diff_id and operator_session_id
// ---------------------------------------------------------------------------

func TestHistoryService_HandleFetchFileDiffRequest_MissingBothIDs(t *testing.T) {
	hs, db := newTestHistoryService(t)
	ls := newTestLocalStore(t)
	hs.localStore = ls

	msg := PubSubCommandMessage{
		ID:      "msg-diff-2",
		CaseID:  "case-1",
		Payload: mustMarshalJSON(t, models.FetchFileDiffRequestPayload{}),
	}
	hs.HandleFetchFileDiffRequest(context.Background(), msg)

	published := db.LastPublished()
	require.NotNil(t, published)
	assert.Contains(t, string(published.Data), constants.Event.Operator.FetchFileDiff.Failed)
}
