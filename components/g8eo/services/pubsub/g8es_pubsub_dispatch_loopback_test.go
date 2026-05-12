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

// Dispatch loopback tests cover the full g8eo command dispatch pipeline using an
// in-process PubSubBroker (via loopbackFixture).  No external infrastructure is
// required.  All tests run in the default suite (no //go:build integration tag).
//
// Pattern for every test:
//  1. Build loopbackFixture (in-process broker + httptest server).
//  2. Build PubSubCommandService connected to the broker.
//  3. Subscribe a watcher client to the relevant results/heartbeat channel and
//     call subscribeAndWait so the subscription is guaranteed registered before
//     the service starts or any publish fires.
//  4. Start the service (or call the handler directly for targeted tests).
//  5. Inject a command message on the cmd channel via a second client.
//  6. drainOne — assert the expected result arrived on the watcher channel.
//
// These tests exercise the full dispatch → service → publish path that
// MockOperatorPubSubClient-based unit tests cannot reach.

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/g8e-ai/g8e/components/g8eo/constants"
	"github.com/g8e-ai/g8e/components/g8eo/models"
	execution "github.com/g8e-ai/g8e/components/g8eo/services/execution"
	sentinelpkg "github.com/g8e-ai/g8e/components/g8eo/services/sentinel"
	"github.com/g8e-ai/g8e/components/g8eo/shared/proto/commonv1"
	"github.com/g8e-ai/g8e/components/g8eo/shared/proto/operatorv1"
	"github.com/g8e-ai/g8e/components/g8eo/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// =============================================================================
// Helpers shared by this file
// =============================================================================

// newLoopbackService builds a fully-wired PubSubCommandService connected to the
// loopback broker.  HeartbeatInterval is set to 0 to disable the scheduler
// unless the caller needs it.
func newLoopbackService(t *testing.T, f *loopbackFixture) (*PubSubCommandService, *PubSubResultsService) {
	t.Helper()
	cfg := testutil.NewTestConfig(t)
	cfg.HeartbeatInterval = 0
	logger := testutil.NewTestLogger()

	cmdClient, err := NewOperatorPubSubClient(f.wsURL, "", logger)
	require.NoError(t, err)
	t.Cleanup(cmdClient.Close)

	resultsSvc, err := NewPubSubResultsService(cfg, logger, cmdClient, nil)
	require.NoError(t, err)

	sentinel := sentinelpkg.NewSentinel(&sentinelpkg.SentinelConfig{
		Enabled:                true,
		ThreatDetectionEnabled: true,
	}, logger)

	svc, err := NewPubSubCommandService(CommandServiceConfig{
		Config:         cfg,
		Logger:         logger,
		Execution:      execution.NewExecutionService(cfg, logger),
		FileEdit:       execution.NewFileEditService(cfg, logger),
		PubSubClient:   cmdClient,
		ResultsService: resultsSvc,
		Sentinel:       sentinel,
	})
	require.NoError(t, err)

	return svc, resultsSvc
}

// injectCmdProtobuf publishes a UAPEnvelope JSON containing a protobuf payload on the cmd channel.
func injectCmdProtobuf(t *testing.T, f *loopbackFixture, svc *PubSubCommandService, envelopeBytes []byte) {
	t.Helper()
	cmdCh := constants.CmdChannel(svc.config.OperatorID, svc.config.OperatorSessionId)

	// Confirm the service's subscription is registered before publishing.
	f.subscribeAndWait(t, cmdCh)

	injector := f.newClient(t)
	require.NoError(t, injector.Publish(context.Background(), cmdCh, envelopeBytes))
}

// watchResults subscribes a client to the results channel for svc and waits for
// that subscription to be confirmed before returning.
func watchResults(t *testing.T, f *loopbackFixture, svc *PubSubCommandService) <-chan []byte {
	t.Helper()
	resultsCh := constants.ResultsChannel(svc.config.OperatorID, svc.config.OperatorSessionId)
	watcher := f.newClient(t)
	sub, err := watcher.Subscribe(context.Background(), resultsCh)
	require.NoError(t, err)
	f.subscribeAndWait(t, resultsCh)
	return sub
}

// watchHeartbeat subscribes a client to the heartbeat channel for svc and waits
// for that subscription to be confirmed before returning.
func watchHeartbeat(t *testing.T, f *loopbackFixture, svc *PubSubCommandService) <-chan []byte {
	t.Helper()
	hbCh := constants.HeartbeatChannel(svc.config.OperatorID, svc.config.OperatorSessionId)
	watcher := f.newClient(t)
	sub, err := watcher.Subscribe(context.Background(), hbCh)
	require.NoError(t, err)
	f.subscribeAndWait(t, hbCh)
	return sub
}

// startService starts svc and registers a cleanup to stop it.
func startService(t *testing.T, svc *PubSubCommandService) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	require.NoError(t, svc.Start(ctx))
	t.Cleanup(func() {
		cancel()
		svc.Stop() //nolint:errcheck
	})
}

func assertLoopbackEnvelope(t *testing.T, data []byte, eventType string) *commonv1.GovernanceEnvelope {
	t.Helper()
	env := testutil.MustUnmarshalUniversalEnvelope(t, data)
	assert.Equal(t, eventType, env.EventType)
	return env
}

// =============================================================================
// CommandService — HandleExecutionRequest end-to-end
// =============================================================================

func TestLoopback_CommandDispatch_ExecutionRequest_EchoCommand(t *testing.T) {
	f := newLoopbackFixture(t)
	svc, _ := newLoopbackService(t, f)

	resultsSub := watchResults(t, f, svc)
	hbSub := watchHeartbeat(t, f, svc)

	startService(t, svc)

	// Consume the automatic heartbeat published on connect.
	_ = drainOne(t, hbSub)

	cmdPayload := testutil.MustMarshalProtobufCommandRequested(t, "echo hello loopback", "exec-loop-echo-1", "test", "", 0)
	envelopeBytes := testutil.MustMarshalUniversalEnvelope(t, "exec-loop-echo-1", constants.Event.Operator.Command.Requested, cmdPayload, "", svc.config.OperatorID, "case-echo", "inv-echo", svc.config.OperatorSessionId)
	injectCmdProtobuf(t, f, svc, envelopeBytes)

	msg := drainOne(t, resultsSub)
	resultEnvelope := assertLoopbackEnvelope(t, msg, constants.Event.Operator.Command.Completed)
	assert.Equal(t, "case-echo", resultEnvelope.CaseId)

	var result operatorv1.CommandResult
	testutil.MustUnmarshalPayload(t, resultEnvelope.Payload, &result)
	assert.Equal(t, protoExecutionStatus(constants.ExecutionStatusCompleted), result.Status)
	assert.Contains(t, result.Output, "hello loopback")
}

func TestLoopback_CommandDispatch_ExecutionRequest_InvalidCommand(t *testing.T) {
	f := newLoopbackFixture(t)
	svc, _ := newLoopbackService(t, f)

	resultsSub := watchResults(t, f, svc)
	hbSub := watchHeartbeat(t, f, svc)

	startService(t, svc)
	_ = drainOne(t, hbSub)

	cmdPayload := testutil.MustMarshalProtobufCommandRequested(t, "missingcmdxyzzy", "exec-loop-fail-1", "test", "", 0)
	envelopeBytes := testutil.MustMarshalUniversalEnvelope(t, "exec-loop-fail-1", constants.Event.Operator.Command.Requested, cmdPayload, "", svc.config.OperatorID, "case-fail", "", svc.config.OperatorSessionId)
	injectCmdProtobuf(t, f, svc, envelopeBytes)

	msg := drainOne(t, resultsSub)
	// Non-existent command completes with failed status.
	resultEnvelope := assertLoopbackEnvelope(t, msg, constants.Event.Operator.Command.Failed)
	assert.Equal(t, "case-fail", resultEnvelope.CaseId)

	var result operatorv1.CommandResult
	testutil.MustUnmarshalPayload(t, resultEnvelope.Payload, &result)
	assert.Equal(t, protoExecutionStatus(constants.ExecutionStatusFailed), result.Status)
}

func TestLoopback_CommandDispatch_ExecutionRequest_StdoutContent(t *testing.T) {
	f := newLoopbackFixture(t)
	svc, _ := newLoopbackService(t, f)

	resultsSub := watchResults(t, f, svc)
	hbSub := watchHeartbeat(t, f, svc)

	startService(t, svc)
	_ = drainOne(t, hbSub)

	const sentinel = "loopback-output-sentinel-42"
	cmdPayload := testutil.MustMarshalProtobufCommandRequested(t, fmt.Sprintf("echo %s", sentinel), "exec-loop-stdout-1", "test", "", 0)
	envelopeBytes := testutil.MustMarshalUniversalEnvelope(t, "exec-loop-stdout-1", constants.Event.Operator.Command.Requested, cmdPayload, "", svc.config.OperatorID, "case-stdout", "", svc.config.OperatorSessionId)
	injectCmdProtobuf(t, f, svc, envelopeBytes)

	msg := drainOne(t, resultsSub)
	resultEnvelope := assertLoopbackEnvelope(t, msg, constants.Event.Operator.Command.Completed)

	var result operatorv1.CommandResult
	testutil.MustUnmarshalPayload(t, resultEnvelope.Payload, &result)
	assert.Contains(t, result.Output, sentinel)
}

func TestLoopback_CommandDispatch_ExecutionRequest_TaskIDThreaded(t *testing.T) {
	f := newLoopbackFixture(t)
	svc, _ := newLoopbackService(t, f)

	resultsSub := watchResults(t, f, svc)
	hbSub := watchHeartbeat(t, f, svc)

	startService(t, svc)
	_ = drainOne(t, hbSub)

	taskID := "task-loop-99"
	cmdPayload := testutil.MustMarshalProtobufCommandRequested(t, "echo task threaded", "exec-loop-task-1", "test", "", 0)
	envelopeBytes := testutil.MustMarshalUniversalEnvelope(t, "exec-loop-task-1", constants.Event.Operator.Command.Requested, cmdPayload, taskID, svc.config.OperatorID, "case-task", "inv-task", svc.config.OperatorSessionId)
	injectCmdProtobuf(t, f, svc, envelopeBytes)

	msg := drainOne(t, resultsSub)
	envelope := assertLoopbackEnvelope(t, msg, constants.Event.Operator.Command.Completed)
	assert.Equal(t, taskID, envelope.TaskId)
	assert.Equal(t, "inv-task", envelope.InvestigationId)
}

// =============================================================================
// CommandService — HandleCancelRequest end-to-end
// =============================================================================

func TestLoopback_CommandDispatch_CancelRequest_RunningCommand(t *testing.T) {
	f := newLoopbackFixture(t)
	svc, _ := newLoopbackService(t, f)

	resultsSub := watchResults(t, f, svc)
	hbSub := watchHeartbeat(t, f, svc)

	startService(t, svc)
	_ = drainOne(t, hbSub)

	// Start a long-running command.
	execID := "cancel-target-exec-1"
	cmdPayload := testutil.MustMarshalProtobufCommandRequested(t, "sleep 30", execID, "test", "", 0)
	envelopeBytes := testutil.MustMarshalUniversalEnvelope(t, execID, constants.Event.Operator.Command.Requested, cmdPayload, "", svc.config.OperatorID, "case-cancel", "", svc.config.OperatorSessionId)
	injectCmdProtobuf(t, f, svc, envelopeBytes)

	// Give the command time to start executing before sending cancel.
	time.Sleep(150 * time.Millisecond)

	cancelPayload := testutil.MustMarshalProtobufCommandCancelRequested(t, execID)
	cancelEnvelopeBytes := testutil.MustMarshalUniversalEnvelope(t, "cancel-req-1", constants.Event.Operator.Command.CancelRequested, cancelPayload, "", svc.config.OperatorID, "case-cancel", "", svc.config.OperatorSessionId)
	injectCmdProtobuf(t, f, svc, cancelEnvelopeBytes)

	// Expect either a cancellation result or a completed result (race window).
	msg := drainOne(t, resultsSub)
	env := testutil.MustUnmarshalUniversalEnvelope(t, msg)
	validEvents := []string{
		constants.Event.Operator.Command.Cancelled,
		constants.Event.Operator.Command.Completed,
		constants.Event.Operator.Command.Failed,
	}
	found := false
	for _, ev := range validEvents {
		if env.EventType == ev {
			found = true
			break
		}
	}
	assert.True(t, found, "expected a command result event, got: %s", env.EventType)
}

func TestLoopback_CommandDispatch_CancelRequest_NotFound(t *testing.T) {
	f := newLoopbackFixture(t)
	svc, _ := newLoopbackService(t, f)

	resultsSub := watchResults(t, f, svc)
	hbSub := watchHeartbeat(t, f, svc)

	startService(t, svc)
	_ = drainOne(t, hbSub)

	// Cancel a command that was never started — should publish a failure result.
	cancelPayload := testutil.MustMarshalProtobufCommandCancelRequested(t, "nonexistent-exec-id")
	cancelEnvelopeBytes := testutil.MustMarshalUniversalEnvelope(t, "cancel-ghost-1", constants.Event.Operator.Command.CancelRequested, cancelPayload, "", svc.config.OperatorID, "case-ghost", "", svc.config.OperatorSessionId)
	injectCmdProtobuf(t, f, svc, cancelEnvelopeBytes)

	msg := drainOne(t, resultsSub)
	assertLoopbackEnvelope(t, msg, constants.Event.Operator.Command.Cancelled)
}

// =============================================================================
// FileOpsService — HandleFileEditRequest end-to-end
// =============================================================================

func TestLoopback_CommandDispatch_FileEdit_WriteAndRead(t *testing.T) {
	f := newLoopbackFixture(t)
	svc, _ := newLoopbackService(t, f)

	resultsSub := watchResults(t, f, svc)
	hbSub := watchHeartbeat(t, f, svc)

	startService(t, svc)
	_ = drainOne(t, hbSub)

	targetPath := filepath.Join(t.TempDir(), "loopback-write.txt")

	filePayload := testutil.MustMarshalProtobufFileEditRequested(t, testutil.FileEditRequestFields{
		FilePath:        targetPath,
		Operation:       "write",
		ExecutionId:     "file-write-1",
		Justification:   "test",
		Content:         "hello from loopback\n",
		CreateIfMissing: true,
	})
	envelopeBytes := testutil.MustMarshalUniversalEnvelope(t, "file-write-1", constants.Event.Operator.FileEdit.Requested, filePayload, "", svc.config.OperatorID, "case-file-write", "inv-file-write", svc.config.OperatorSessionId)
	injectCmdProtobuf(t, f, svc, envelopeBytes)

	msg := drainOne(t, resultsSub)
	resultEnvelope := assertLoopbackEnvelope(t, msg, constants.Event.Operator.FileEdit.Completed)
	assert.Equal(t, "case-file-write", resultEnvelope.CaseId)

	var result operatorv1.FileEditResult
	testutil.MustUnmarshalPayload(t, resultEnvelope.Payload, &result)
	assert.Equal(t, protoExecutionStatus(constants.ExecutionStatusCompleted), result.Status)

	data, err := os.ReadFile(targetPath)
	require.NoError(t, err)
	assert.Equal(t, "hello from loopback\n", string(data))
}

func TestLoopback_CommandDispatch_FileEdit_SentinelBlocked(t *testing.T) {
	f := newLoopbackFixture(t)
	svc, _ := newLoopbackService(t, f)

	resultsSub := watchResults(t, f, svc)
	hbSub := watchHeartbeat(t, f, svc)

	startService(t, svc)
	_ = drainOne(t, hbSub)

	// Attempt to write to a critical system path — sentinel must block this.
	// We use a path under a temp dir with the /etc/passwd name so the sentinel
	// pattern matches without touching any real system file.
	tmpDir := t.TempDir()
	targetPath := filepath.Join(tmpDir, "etc", "passwd")
	filePayload := testutil.MustMarshalProtobufFileEditRequested(t, testutil.FileEditRequestFields{
		FilePath:        targetPath,
		Operation:       "insert",
		ExecutionId:     "file-insert-1",
		Justification:   "test",
		InsertContent:   "inserted\n",
		InsertPosition:  5,
		CreateIfMissing: true,
	})
	envelopeBytes := testutil.MustMarshalUniversalEnvelope(t, "file-sentinel-1", constants.Event.Operator.FileEdit.Requested, filePayload, "", svc.config.OperatorID, "case-sentinel", "", svc.config.OperatorSessionId)
	injectCmdProtobuf(t, f, svc, envelopeBytes)

	msg := drainOne(t, resultsSub)
	assertLoopbackEnvelope(t, msg, constants.Event.Operator.FileEdit.Failed)
	_, statErr := os.Stat(targetPath)
	assert.True(t, os.IsNotExist(statErr), "sentinel-blocked write must not create the file")
}

func TestLoopback_CommandDispatch_FileEdit_MissingPath(t *testing.T) {
	f := newLoopbackFixture(t)
	svc, _ := newLoopbackService(t, f)

	resultsSub := watchResults(t, f, svc)
	hbSub := watchHeartbeat(t, f, svc)

	startService(t, svc)
	_ = drainOne(t, hbSub)

	// Missing file_path — handler should log and not publish (no result expected).
	// We assert no message arrives within a short window.
	filePayload := testutil.MustMarshalProtobufFileEditRequested(t, testutil.FileEditRequestFields{
		FilePath:        "",
		Operation:       "replace",
		ExecutionId:     "file-replace-1",
		Justification:   "test",
		OldContent:      "original\n",
		NewContent:      "replaced\n",
		CreateIfMissing: true,
	})
	envelopeBytes := testutil.MustMarshalUniversalEnvelope(t, "file-no-path-1", constants.Event.Operator.FileEdit.Requested, filePayload, "", svc.config.OperatorID, "case-no-path", "", svc.config.OperatorSessionId)
	injectCmdProtobuf(t, f, svc, envelopeBytes)

	drainNone(t, resultsSub, 200*time.Millisecond)
}

func TestLoopback_CommandDispatch_FileEdit_NilResultNoPanic(t *testing.T) {
	// Regression: HandleFileEditRequest must not panic when ExecuteFileEdit
	// returns (nil, nil). A result must always be published.
	f := newLoopbackFixture(t)
	svc, _ := newLoopbackService(t, f)

	resultsSub := watchResults(t, f, svc)
	hbSub := watchHeartbeat(t, f, svc)

	startService(t, svc)
	_ = drainOne(t, hbSub)

	// A delete on a path that does not exist forces ExecuteFileEdit down the
	// error path; even if it somehow returned (nil, nil), the nil guard must
	// synthesise a failed result rather than dereferencing nil.
	targetPath := filepath.Join(t.TempDir(), "does-not-exist.txt")
	filePayload := testutil.MustMarshalProtobufFileEditRequested(t, testutil.FileEditRequestFields{
		FilePath:        targetPath,
		Operation:       "replace",
		ExecutionId:     "file-replace-1",
		Justification:   "test",
		OldContent:      "original\n",
		NewContent:      "replaced\n",
		CreateIfMissing: true,
	})
	envelopeBytes := testutil.MustMarshalUniversalEnvelope(t, "nil-result-guard-1", constants.Event.Operator.FileEdit.Requested, filePayload, "", svc.config.OperatorID, "case-nil-guard", "", svc.config.OperatorSessionId)
	injectCmdProtobuf(t, f, svc, envelopeBytes)

	msg := drainOne(t, resultsSub)
	// Must receive either a completed or failed result — never a panic or silence.
	env := testutil.MustUnmarshalUniversalEnvelope(t, msg)
	hasResult := env.EventType == constants.Event.Operator.FileEdit.Completed ||
		env.EventType == constants.Event.Operator.FileEdit.Failed
	assert.True(t, hasResult, "expected a file edit result event, got: %s", env.EventType)
}

// =============================================================================
// FileOpsService — HandleFsListRequest end-to-end
// =============================================================================

func TestLoopback_CommandDispatch_FsList_WorkDir(t *testing.T) {
	f := newLoopbackFixture(t)
	svc, _ := newLoopbackService(t, f)

	resultsSub := watchResults(t, f, svc)
	hbSub := watchHeartbeat(t, f, svc)

	startService(t, svc)
	_ = drainOne(t, hbSub)

	fsListPayload := testutil.MustMarshalProtobufFsListRequested(t, ".", "fslist-1", 50, 0)
	envelopeBytes := testutil.MustMarshalUniversalEnvelope(t, "fslist-1", constants.Event.Operator.FsList.Requested, fsListPayload, "", svc.config.OperatorID, "case-fslist", "inv-fslist", svc.config.OperatorSessionId)
	injectCmdProtobuf(t, f, svc, envelopeBytes)

	msg := drainOne(t, resultsSub)
	resultEnvelope := assertLoopbackEnvelope(t, msg, constants.Event.Operator.FsList.Completed)
	assert.Equal(t, "case-fslist", resultEnvelope.CaseId)

	var result operatorv1.FsListResult
	testutil.MustUnmarshalPayload(t, resultEnvelope.Payload, &result)
	assert.Equal(t, protoExecutionStatus(constants.ExecutionStatusCompleted), result.Status)
}

func TestLoopback_CommandDispatch_FsList_NonExistentPath(t *testing.T) {
	f := newLoopbackFixture(t)
	svc, _ := newLoopbackService(t, f)

	resultsSub := watchResults(t, f, svc)
	hbSub := watchHeartbeat(t, f, svc)

	startService(t, svc)
	_ = drainOne(t, hbSub)

	fsListPayload := testutil.MustMarshalProtobufFsListRequested(t, "/no/such/path/xyzzy", "fslist-missing-1", 10, 0)
	envelopeBytes := testutil.MustMarshalUniversalEnvelope(t, "fslist-missing-1", constants.Event.Operator.FsList.Requested, fsListPayload, "", svc.config.OperatorID, "case-fslist-miss", "", svc.config.OperatorSessionId)
	injectCmdProtobuf(t, f, svc, envelopeBytes)

	msg := drainOne(t, resultsSub)
	assertLoopbackEnvelope(t, msg, constants.Event.Operator.FsList.Failed)
}

// =============================================================================
// FileOpsService — HandleFsReadRequest end-to-end
// =============================================================================

func TestLoopback_CommandDispatch_FsRead_ExistingFile(t *testing.T) {
	f := newLoopbackFixture(t)
	svc, _ := newLoopbackService(t, f)

	resultsSub := watchResults(t, f, svc)
	hbSub := watchHeartbeat(t, f, svc)

	startService(t, svc)
	_ = drainOne(t, hbSub)

	// Write a known file then read it back via pub/sub.
	dir := t.TempDir()
	target := filepath.Join(dir, "readme.txt")
	const content = "loopback fs read test content"
	require.NoError(t, os.WriteFile(target, []byte(content), 0o644))

	fsReadPayload := testutil.MustMarshalProtobufFsReadRequested(t, target, "fsread-1", 4096)
	envelopeBytes := testutil.MustMarshalUniversalEnvelope(t, "fsread-1", constants.Event.Operator.FsRead.Requested, fsReadPayload, "", svc.config.OperatorID, "case-fsread", "", svc.config.OperatorSessionId)
	injectCmdProtobuf(t, f, svc, envelopeBytes)

	msg := drainOne(t, resultsSub)
	resultEnvelope := assertLoopbackEnvelope(t, msg, constants.Event.Operator.FsRead.Completed)

	var result operatorv1.FsReadResult
	testutil.MustUnmarshalPayload(t, resultEnvelope.Payload, &result)
	assert.Contains(t, result.Content, content)
}

func TestLoopback_CommandDispatch_FsRead_MissingFile(t *testing.T) {
	f := newLoopbackFixture(t)
	svc, _ := newLoopbackService(t, f)

	resultsSub := watchResults(t, f, svc)
	hbSub := watchHeartbeat(t, f, svc)

	startService(t, svc)
	_ = drainOne(t, hbSub)

	fsReadPayload := testutil.MustMarshalProtobufFsReadRequested(t, "/no/such/file/xyzzy.txt", "fsread-miss-1", 4096)
	envelopeBytes := testutil.MustMarshalUniversalEnvelope(t, "fsread-miss-1", constants.Event.Operator.FsRead.Requested, fsReadPayload, "", svc.config.OperatorID, "case-fsread-miss", "", svc.config.OperatorSessionId)
	injectCmdProtobuf(t, f, svc, envelopeBytes)

	msg := drainOne(t, resultsSub)
	assertLoopbackEnvelope(t, msg, constants.Event.Operator.FsRead.Failed)
}

// =============================================================================
// Multi-subscriber fan-out — same result delivered to multiple watchers
// =============================================================================

func TestLoopback_CommandDispatch_FanOut_MultipleResultSubscribers(t *testing.T) {
	f := newLoopbackFixture(t)
	svc, _ := newLoopbackService(t, f)

	// Build three independent result watchers before starting the service.
	resultsCh := constants.ResultsChannel(svc.config.OperatorID, svc.config.OperatorSessionId)

	const n = 3
	subs := make([]<-chan []byte, n)
	for i := range subs {
		c := f.newClient(t)
		var err error
		subs[i], err = c.Subscribe(context.Background(), resultsCh)
		require.NoError(t, err)
	}
	for i := 0; i < n; i++ {
		f.subscribeAndWait(t, resultsCh)
	}

	hbSub := watchHeartbeat(t, f, svc)
	startService(t, svc)
	_ = drainOne(t, hbSub)

	cmdPayload := testutil.MustMarshalProtobufCommandRequested(t, "echo fanout", "fanout-1", "test", "", 0)
	envelopeBytes := testutil.MustMarshalUniversalEnvelope(t, "fanout-1", constants.Event.Operator.Command.Requested, cmdPayload, "", svc.config.OperatorID, "case-fanout", "", svc.config.OperatorSessionId)
	injectCmdProtobuf(t, f, svc, envelopeBytes)

	for i, sub := range subs {
		msg := drainOne(t, sub)
		env := testutil.MustUnmarshalUniversalEnvelope(t, msg)
		assert.Equal(t, constants.Event.Operator.Command.Completed, env.EventType,
			"subscriber %d did not receive the result", i)
	}
}

// =============================================================================
// Heartbeat — inbound heartbeat request dispatched end-to-end
// =============================================================================

func TestLoopback_CommandDispatch_HeartbeatRequested_CaseIDThreaded(t *testing.T) {
	f := newLoopbackFixture(t)
	svc, _ := newLoopbackService(t, f)

	hbSub := watchHeartbeat(t, f, svc)

	startService(t, svc)

	// Consume the automatic heartbeat.
	_ = drainOne(t, hbSub)

	hbPayload := testutil.MustMarshalProtobufHeartbeatRequested(t)
	envelopeBytes := testutil.MustMarshalUniversalEnvelope(t, "hb-req-case-1", constants.Event.Operator.HeartbeatRequested, hbPayload, "", svc.config.OperatorID, "case-hb-req", "inv-hb-req", svc.config.OperatorSessionId)
	injectCmdProtobuf(t, f, svc, envelopeBytes)

	msg := drainOne(t, hbSub)
	env := assertLoopbackEnvelope(t, msg, constants.Event.Operator.Heartbeat)
	assert.Equal(t, "case-hb-req", env.CaseId)
	assert.Equal(t, "inv-hb-req", env.InvestigationId)

	var hb operatorv1.HeartbeatResult
	testutil.MustUnmarshalPayload(t, env.Payload, &hb)
	assert.Equal(t, string(models.HeartbeatTypeRequested), hb.Status)
	assert.Equal(t, "case-hb-req", hb.CaseId)
	assert.Equal(t, "inv-hb-req", hb.InvestigationId)
}

// =============================================================================
// Unknown event type — dispatcher must not crash or deadlock
// =============================================================================

func TestLoopback_CommandDispatch_UnknownEventType_NoResult(t *testing.T) {
	f := newLoopbackFixture(t)
	svc, _ := newLoopbackService(t, f)

	resultsSub := watchResults(t, f, svc)
	hbSub := watchHeartbeat(t, f, svc)

	startService(t, svc)
	_ = drainOne(t, hbSub)

	// Unknown event type - use empty payload since we don't have a specific protobuf for it
	envelopeBytes := testutil.MustMarshalUniversalEnvelope(t, "unknown-event-1", "operator.unknown.made_up_type", []byte{}, "", svc.config.OperatorID, "case-unknown", "", svc.config.OperatorSessionId)
	injectCmdProtobuf(t, f, svc, envelopeBytes)

	// Unknown event types are silently dropped — no result is published.
	drainNone(t, resultsSub, 200*time.Millisecond)
}
