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
	"encoding/json"
	"testing"
	"time"

	"github.com/g8e-ai/g8e/components/g8eo/constants"
	"github.com/g8e-ai/g8e/components/g8eo/models"
	storage "github.com/g8e-ai/g8e/components/g8eo/services/storage"
	"github.com/g8e-ai/g8e/components/g8eo/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// publishFetchLogsResultFromRaw — covered via HandleFetchLogsRequest with a
// seeded RawVaultService so the raw path returns a record.
// ---------------------------------------------------------------------------

func TestHistoryService_PublishFetchLogsResultFromRaw_PublishesOnResultsChannel(t *testing.T) {
	cfg := testutil.NewTestConfig(t)
	logger := testutil.NewTestLogger()
	db := NewMockVSODBPubSubClient()
	t.Cleanup(func() { db.Close() })

	hs := NewHistoryService(cfg, logger, db)

	rv := newTestRawVault(t)
	hs.rawVault = rv

	exitCode := 0
	record := &storage.RawExecutionRecord{
		ID:               "raw-exec-001",
		TimestampUTC:     time.Now().UTC(),
		Command:          "echo hello",
		ExitCode:         &exitCode,
		DurationMs:       42,
		StdoutCompressed: []byte("hello"),
		StderrCompressed: []byte(""),
		StdoutSize:       5,
		StderrSize:       0,
	}
	require.NoError(t, rv.StoreRawExecution(record))

	msg := PubSubCommandMessage{
		ID:              "fetch-raw-1",
		CaseID:          "case-1",
		InvestigationID: "inv-1",
		Payload: mustMarshalJSON(t, models.FetchLogsRequestPayload{
			ExecutionID:  "raw-exec-001",
			SentinelMode: constants.Status.VaultMode.Raw,
		}),
	}

	hs.HandleFetchLogsRequest(context.Background(), msg)

	published := db.LastPublished()
	require.NotNil(t, published)
	assert.Contains(t, string(published.Data), constants.Event.Operator.FetchLogs.Completed)
	assert.Contains(t, string(published.Data), "raw-exec-001")
	assert.Contains(t, string(published.Data), constants.Status.VaultMode.Raw)
}

// ---------------------------------------------------------------------------
// publishFetchLogsResult — covered via HandleFetchLogsRequest with a seeded
// LocalStoreService so the scrubbed path returns a record.
// ---------------------------------------------------------------------------

func TestHistoryService_PublishFetchLogsResult_PublishesOnResultsChannel(t *testing.T) {
	cfg := testutil.NewTestConfig(t)
	logger := testutil.NewTestLogger()
	db := NewMockVSODBPubSubClient()
	t.Cleanup(func() { db.Close() })

	hs := NewHistoryService(cfg, logger, db)

	ls := newTestLocalStore(t)
	hs.localStore = ls

	exitCode := 0
	record := &storage.ExecutionRecord{
		ID:               "scrubbed-exec-001",
		TimestampUTC:     time.Now().UTC(),
		Command:          "ls -la",
		ExitCode:         &exitCode,
		DurationMs:       12,
		StdoutCompressed: []byte("file1\nfile2"),
		StderrCompressed: []byte(""),
		StdoutSize:       11,
		StderrSize:       0,
	}
	require.NoError(t, ls.StoreExecution(record))

	msg := PubSubCommandMessage{
		ID:              "fetch-scrubbed-1",
		CaseID:          "case-2",
		InvestigationID: "inv-2",
		Payload: mustMarshalJSON(t, models.FetchLogsRequestPayload{
			ExecutionID:  "scrubbed-exec-001",
			SentinelMode: constants.Status.VaultMode.Scrubbed,
		}),
	}

	hs.HandleFetchLogsRequest(context.Background(), msg)

	published := db.LastPublished()
	require.NotNil(t, published)
	assert.Contains(t, string(published.Data), constants.Event.Operator.FetchLogs.Completed)
	assert.Contains(t, string(published.Data), "scrubbed-exec-001")
	assert.Contains(t, string(published.Data), constants.Status.VaultMode.Scrubbed)
}

// ---------------------------------------------------------------------------
// publishFetchLogsPayload — verify wire message shape (TaskID, InvestigationID,
// OperatorSessionID threading) via the raw vault path.
// ---------------------------------------------------------------------------

func TestHistoryService_PublishFetchLogsPayload_ThreadsMessageIDs(t *testing.T) {
	cfg := testutil.NewTestConfig(t)
	logger := testutil.NewTestLogger()
	db := NewMockVSODBPubSubClient()
	t.Cleanup(func() { db.Close() })

	hs := NewHistoryService(cfg, logger, db)

	rv := newTestRawVault(t)
	hs.rawVault = rv

	exitCode := 0
	record := &storage.RawExecutionRecord{
		ID:               "thread-exec-001",
		TimestampUTC:     time.Now().UTC(),
		Command:          "whoami",
		ExitCode:         &exitCode,
		DurationMs:       5,
		StdoutCompressed: []byte("root"),
		StdoutSize:       4,
	}
	require.NoError(t, rv.StoreRawExecution(record))

	taskID := "task-abc"
	msg := PubSubCommandMessage{
		ID:                "thread-msg-1",
		CaseID:            "case-3",
		InvestigationID:   "inv-3",
		OperatorSessionID: "sess-xyz",
		TaskID:            &taskID,
		Payload: mustMarshalJSON(t, models.FetchLogsRequestPayload{
			ExecutionID:  "thread-exec-001",
			SentinelMode: constants.Status.VaultMode.Raw,
		}),
	}

	hs.HandleFetchLogsRequest(context.Background(), msg)

	published := db.LastPublished()
	require.NotNil(t, published)

	var envelope map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(published.Data, &envelope))

	var investigationID string
	require.NoError(t, json.Unmarshal(envelope["investigation_id"], &investigationID))
	assert.Equal(t, "inv-3", investigationID)

	var operatorSessionID string
	require.NoError(t, json.Unmarshal(envelope["operator_session_id"], &operatorSessionID))
	assert.Equal(t, "sess-xyz", operatorSessionID)
}

// ---------------------------------------------------------------------------
// publishFetchLogsPayload — published to the correct results channel
// ---------------------------------------------------------------------------

func TestHistoryService_PublishFetchLogsPayload_PublishesToResultsChannel(t *testing.T) {
	cfg := testutil.NewTestConfig(t)
	logger := testutil.NewTestLogger()
	db := NewMockVSODBPubSubClient()
	t.Cleanup(func() { db.Close() })

	hs := NewHistoryService(cfg, logger, db)

	ls := newTestLocalStore(t)
	hs.localStore = ls

	exitCode := 0
	record := &storage.ExecutionRecord{
		ID:               "channel-exec-001",
		TimestampUTC:     time.Now().UTC(),
		Command:          "pwd",
		ExitCode:         &exitCode,
		DurationMs:       3,
		StdoutCompressed: []byte("/home/user"),
		StdoutSize:       10,
	}
	require.NoError(t, ls.StoreExecution(record))

	msg := PubSubCommandMessage{
		ID:     "channel-msg-1",
		CaseID: "case-4",
		Payload: mustMarshalJSON(t, models.FetchLogsRequestPayload{
			ExecutionID:  "channel-exec-001",
			SentinelMode: constants.Status.VaultMode.Scrubbed,
		}),
	}

	hs.HandleFetchLogsRequest(context.Background(), msg)

	published := db.LastPublished()
	require.NotNil(t, published)

	expectedChannel := constants.ResultsChannel(cfg.OperatorID, cfg.OperatorSessionId)
	assert.Equal(t, expectedChannel, published.Channel)
}

// ---------------------------------------------------------------------------
// publishFetchLogsResultFromRaw — raw vault fallback to scrubbed when raw vault
// present but execution not found → falls back to scrubbed → error published
// ---------------------------------------------------------------------------

func TestHistoryService_PublishFetchLogsResultFromRaw_FallsBackToScrubbedWhenNotFound(t *testing.T) {
	cfg := testutil.NewTestConfig(t)
	logger := testutil.NewTestLogger()
	db := NewMockVSODBPubSubClient()
	t.Cleanup(func() { db.Close() })

	hs := NewHistoryService(cfg, logger, db)
	rv := newTestRawVault(t)
	hs.rawVault = rv

	msg := PubSubCommandMessage{
		ID:     "fallback-msg-1",
		CaseID: "case-5",
		Payload: mustMarshalJSON(t, models.FetchLogsRequestPayload{
			ExecutionID:  "nonexistent-raw-exec",
			SentinelMode: constants.Status.VaultMode.Raw,
		}),
	}

	hs.HandleFetchLogsRequest(context.Background(), msg)

	published := db.LastPublished()
	require.NotNil(t, published)
	assert.Contains(t, string(published.Data), constants.Event.Operator.FetchLogs.Failed)
}

// ---------------------------------------------------------------------------
// HandleFetchLogsRequest — default vault mode is raw when empty
// ---------------------------------------------------------------------------

func TestHistoryService_HandleFetchLogsRequest_DefaultVaultModeIsRaw(t *testing.T) {
	hs, db := newTestHistoryService(t)

	msg := PubSubCommandMessage{
		ID:     "default-mode-1",
		CaseID: "case-6",
		Payload: mustMarshalJSON(t, models.FetchLogsRequestPayload{
			ExecutionID:  "exec-default",
			SentinelMode: "",
		}),
	}

	hs.HandleFetchLogsRequest(context.Background(), msg)

	published := db.LastPublished()
	require.NotNil(t, published)
	assert.Contains(t, string(published.Data), constants.Event.Operator.FetchLogs.Failed,
		"should fail because raw vault is nil, not scrubbed vault")
}
