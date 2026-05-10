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

	"github.com/g8e-ai/g8e/components/g8eo/constants"
	storage "github.com/g8e-ai/g8e/components/g8eo/services/storage"
	"github.com/g8e-ai/g8e/components/g8eo/shared/proto/operatorv1"
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
	db := NewMockOperatorPubSubClient()
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
		EventType:       constants.Event.Operator.FetchLogs.Requested,
		Payload:         testutil.MustMarshalProtobufFetchLogsRequested(t, "raw-exec-001"),
	}

	hs.HandleFetchLogsRequest(context.Background(), msg)

	published := db.LastPublished()
	require.NotNil(t, published)

	envelope := testutil.MustUnmarshalUniversalEnvelope(t, published.Data)
	assert.Equal(t, constants.Event.Operator.FetchLogs.Completed, envelope.EventType)

	var result operatorv1.FetchLogsResult
	testutil.MustUnmarshalPayload(t, envelope.Payload, &result)
	assert.Equal(t, "raw-exec-001", result.ExecutionId)
	assert.Equal(t, constants.Status.VaultMode.Raw, result.SentinelMode)
}

// ---------------------------------------------------------------------------
// publishFetchLogsResult — covered via HandleFetchLogsRequest with a seeded
// LocalStoreService so the scrubbed path returns a record.
// ---------------------------------------------------------------------------

func TestHistoryService_PublishFetchLogsResult_PublishesOnResultsChannel(t *testing.T) {
	cfg := testutil.NewTestConfig(t)
	logger := testutil.NewTestLogger()
	db := NewMockOperatorPubSubClient()
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
		EventType:       constants.Event.Operator.FetchLogs.Requested,
		Payload:         testutil.MustMarshalProtobufFetchLogsRequested(t, "scrubbed-exec-001"),
	}

	hs.HandleFetchLogsRequest(context.Background(), msg)

	published := db.LastPublished()
	require.NotNil(t, published)

	envelope := testutil.MustUnmarshalUniversalEnvelope(t, published.Data)
	assert.Equal(t, constants.Event.Operator.FetchLogs.Completed, envelope.EventType)

	var result operatorv1.FetchLogsResult
	testutil.MustUnmarshalPayload(t, envelope.Payload, &result)
	assert.Equal(t, "scrubbed-exec-001", result.ExecutionId)
	assert.Equal(t, constants.Status.VaultMode.Scrubbed, result.SentinelMode)
}

// ---------------------------------------------------------------------------
// publishFetchLogsPayload — verify wire message shape (TaskID, InvestigationID,
// OperatorSessionID threading) via the raw vault path.
// ---------------------------------------------------------------------------

func TestHistoryService_PublishFetchLogsPayload_ThreadsMessageIDs(t *testing.T) {
	cfg := testutil.NewTestConfig(t)
	logger := testutil.NewTestLogger()
	db := NewMockOperatorPubSubClient()
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
		EventType:         constants.Event.Operator.FetchLogs.Requested,
		Payload:           testutil.MustMarshalProtobufFetchLogsRequested(t, "thread-exec-001"),
	}

	hs.HandleFetchLogsRequest(context.Background(), msg)

	published := db.LastPublished()
	require.NotNil(t, published)

	envelope := testutil.MustUnmarshalUniversalEnvelope(t, published.Data)
	assert.Equal(t, constants.Event.Operator.FetchLogs.Completed, envelope.EventType)
	assert.Equal(t, "case-3", envelope.CaseId)
	assert.Equal(t, "inv-3", envelope.InvestigationId)
	assert.Equal(t, "sess-xyz", envelope.OperatorSessionId)
	assert.Equal(t, taskID, envelope.TaskId)
}

// ---------------------------------------------------------------------------
// publishFetchLogsPayload — published to the correct results channel
// ---------------------------------------------------------------------------

func TestHistoryService_PublishFetchLogsPayload_PublishesToResultsChannel(t *testing.T) {
	cfg := testutil.NewTestConfig(t)
	logger := testutil.NewTestLogger()
	db := NewMockOperatorPubSubClient()
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
		ID:        "channel-msg-1",
		CaseID:    "case-4",
		EventType: constants.Event.Operator.FetchLogs.Requested,
		Payload:   testutil.MustMarshalProtobufFetchLogsRequested(t, "channel-exec-001"),
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
	db := NewMockOperatorPubSubClient()
	t.Cleanup(func() { db.Close() })

	hs := NewHistoryService(cfg, logger, db)
	rv := newTestRawVault(t)
	hs.rawVault = rv

	msg := PubSubCommandMessage{
		ID:        "fallback-msg-1",
		CaseID:    "case-5",
		EventType: constants.Event.Operator.FetchLogs.Requested,
		Payload:   testutil.MustMarshalProtobufFetchLogsRequested(t, "nonexistent-raw-exec"),
	}

	hs.HandleFetchLogsRequest(context.Background(), msg)

	published := db.LastPublished()
	require.NotNil(t, published)

	envelope := testutil.MustUnmarshalUniversalEnvelope(t, published.Data)
	assert.Equal(t, constants.Event.Operator.FetchLogs.Failed, envelope.EventType)
}

// ---------------------------------------------------------------------------
// HandleFetchLogsRequest — default vault mode is raw when empty
// ---------------------------------------------------------------------------

func TestHistoryService_HandleFetchLogsRequest_DefaultVaultModeIsRaw(t *testing.T) {
	hs, db := newTestHistoryService(t)

	msg := PubSubCommandMessage{
		ID:        "default-mode-1",
		CaseID:    "case-6",
		EventType: constants.Event.Operator.FetchLogs.Requested,
		Payload:   testutil.MustMarshalProtobufFetchLogsRequested(t, "exec-default"),
	}

	hs.HandleFetchLogsRequest(context.Background(), msg)

	published := db.LastPublished()
	require.NotNil(t, published)

	envelope := testutil.MustUnmarshalUniversalEnvelope(t, published.Data)
	assert.Equal(t, constants.Event.Operator.FetchLogs.Failed, envelope.EventType,
		"should fail because raw vault is nil, not scrubbed vault")
}
