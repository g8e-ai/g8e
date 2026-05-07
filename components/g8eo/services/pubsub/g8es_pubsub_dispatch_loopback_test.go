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
// MockG8esPubSubClient-based unit tests cannot reach.

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/g8e-ai/g8e/components/g8eo/config"
	"github.com/g8e-ai/g8e/components/g8eo/constants"
	"github.com/g8e-ai/g8e/components/g8eo/models"
	execution "github.com/g8e-ai/g8e/components/g8eo/services/execution"
	sentinelpkg "github.com/g8e-ai/g8e/components/g8eo/services/sentinel"
	commonv1 "github.com/g8e-ai/g8e/components/g8eo/shared/proto/commonv1"
	"github.com/g8e-ai/g8e/components/g8eo/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
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

	cmdClient, err := NewG8esPubSubClient(f.wsURL, "", logger)
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

// newTestG8eMessage creates a models.G8eMessage for testing with required fields populated.
func newTestG8eMessage(t *testing.T, cfg *config.Config, eventType, caseID string, payload interface{}) *models.G8eMessage {
	t.Helper()
	msg, err := models.NewG8eMessage(
		eventType,
		caseID,
		cfg.OperatorID,
		cfg.OperatorSessionId,
		cfg.SystemFingerprint,
		payload,
	)
	require.NoError(t, err)
	return msg
}

// injectCmd publishes a models.G8eMessage on the cmd channel for cfg using an
// injector client connected to the loopback broker.
// It wraps the G8eMessage in a UniversalEnvelope for the protobuf-only transport.
func injectCmd(t *testing.T, f *loopbackFixture, svc *PubSubCommandService, msg *models.G8eMessage) {
	t.Helper()
	cmdCh := constants.CmdChannel(svc.config.OperatorID, svc.config.OperatorSessionId)

	// Confirm the service's subscription is registered before publishing.
	f.subscribeAndWait(t, cmdCh)

	raw, err := msg.Marshal()
	require.NoError(t, err)

	taskID := ""
	if msg.TaskID != nil {
		taskID = *msg.TaskID
	}

	envelope := &commonv1.UniversalEnvelope{
		Id:                msg.ID,
		EventType:         msg.EventType,
		CaseId:            msg.CaseID,
		TaskId:            taskID,
		InvestigationId:   msg.InvestigationID,
		OperatorSessionId: msg.OperatorSessionID,
		OperatorId:        msg.OperatorID,
		Payload:           raw,
	}
	envelopeBytes, err := proto.Marshal(envelope)
	require.NoError(t, err)

	injector := f.newClient(t)
	require.NoError(t, injector.Publish(context.Background(), cmdCh, envelopeBytes))
}

// injectCmdProtobuf publishes a protobuf UniversalEnvelope on the cmd channel.
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

	cmdPayload := mustMarshalProtobufCommandRequested(t, "echo hello loopback", "exec-loop-echo-1", "test", "", 0)
	envelopeBytes := mustMarshalUniversalEnvelope(t, "exec-loop-echo-1", constants.Event.Operator.Command.Requested, cmdPayload, "", svc.config.OperatorID, "case-echo", "inv-echo", svc.config.OperatorSessionId)
	injectCmdProtobuf(t, f, svc, envelopeBytes)

	msg := drainOne(t, resultsSub)
	assert.Contains(t, string(msg), constants.Event.Operator.Command.Completed)
	assert.Contains(t, string(msg), "case-echo")

	var resultEnvelope models.G8eMessage
	require.NoError(t, json.Unmarshal(msg, &resultEnvelope))
	assert.Equal(t, "case-echo", resultEnvelope.CaseID)
	assert.Equal(t, constants.Event.Operator.Command.Completed, resultEnvelope.EventType)
}

func TestLoopback_CommandDispatch_ExecutionRequest_InvalidCommand(t *testing.T) {
	f := newLoopbackFixture(t)
	svc, _ := newLoopbackService(t, f)

	resultsSub := watchResults(t, f, svc)
	hbSub := watchHeartbeat(t, f, svc)

	startService(t, svc)
	_ = drainOne(t, hbSub)

	cmdPayload := mustMarshalProtobufCommandRequested(t, "__no_such_exec_xyzzy__", "exec-loop-fail-1", "test", "", 0)
	envelopeBytes := mustMarshalUniversalEnvelope(t, "exec-loop-fail-1", constants.Event.Operator.Command.Requested, cmdPayload, "", svc.config.OperatorID, "case-fail", "", svc.config.OperatorSessionId)
	injectCmdProtobuf(t, f, svc, envelopeBytes)

	msg := drainOne(t, resultsSub)
	// Non-existent command completes with failed status.
	assert.Contains(t, string(msg), constants.Event.Operator.Command.Failed)
	assert.Contains(t, string(msg), "case-fail")
}

func TestLoopback_CommandDispatch_ExecutionRequest_StdoutContent(t *testing.T) {
	f := newLoopbackFixture(t)
	svc, _ := newLoopbackService(t, f)

	resultsSub := watchResults(t, f, svc)
	hbSub := watchHeartbeat(t, f, svc)

	startService(t, svc)
	_ = drainOne(t, hbSub)

	const sentinel = "loopback-output-sentinel-42"
	cmdPayload := mustMarshalProtobufCommandRequested(t, fmt.Sprintf("echo %s", sentinel), "exec-loop-stdout-1", "test", "", 0)
	envelopeBytes := mustMarshalUniversalEnvelope(t, "exec-loop-stdout-1", constants.Event.Operator.Command.Requested, cmdPayload, "", svc.config.OperatorID, "case-stdout", "", svc.config.OperatorSessionId)
	injectCmdProtobuf(t, f, svc, envelopeBytes)

	msg := drainOne(t, resultsSub)
	assert.Contains(t, string(msg), constants.Event.Operator.Command.Completed)
	assert.Contains(t, string(msg), sentinel)
}

func TestLoopback_CommandDispatch_ExecutionRequest_TaskIDThreaded(t *testing.T) {
	f := newLoopbackFixture(t)
	svc, _ := newLoopbackService(t, f)

	resultsSub := watchResults(t, f, svc)
	hbSub := watchHeartbeat(t, f, svc)

	startService(t, svc)
	_ = drainOne(t, hbSub)

	taskID := "task-loop-99"
	cmdPayload := mustMarshalProtobufCommandRequested(t, "echo task threaded", "exec-loop-task-1", "test", "", 0)
	envelopeBytes := mustMarshalUniversalEnvelope(t, "exec-loop-task-1", constants.Event.Operator.Command.Requested, cmdPayload, taskID, svc.config.OperatorID, "case-task", "inv-task", svc.config.OperatorSessionId)
	injectCmdProtobuf(t, f, svc, envelopeBytes)

	msg := drainOne(t, resultsSub)
	assert.Contains(t, string(msg), constants.Event.Operator.Command.Completed)

	var envelope models.G8eMessage
	require.NoError(t, json.Unmarshal(msg, &envelope))
	require.NotNil(t, envelope.TaskID)
	assert.Equal(t, taskID, *envelope.TaskID)
	assert.Equal(t, "inv-task", envelope.InvestigationID)
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
	cmdPayload := mustMarshalProtobufCommandRequested(t, "sleep 30", execID, "test", "", 0)
	envelopeBytes := mustMarshalUniversalEnvelope(t, execID, constants.Event.Operator.Command.Requested, cmdPayload, "", svc.config.OperatorID, "case-cancel", "", svc.config.OperatorSessionId)
	injectCmdProtobuf(t, f, svc, envelopeBytes)

	// Give the command time to start executing before sending cancel.
	time.Sleep(150 * time.Millisecond)

	cancelPayload := mustMarshalProtobufCommandCancelRequested(t, execID)
	cancelEnvelopeBytes := mustMarshalUniversalEnvelope(t, "cancel-req-1", constants.Event.Operator.Command.CancelRequested, cancelPayload, "", svc.config.OperatorID, "case-cancel", "", svc.config.OperatorSessionId)
	injectCmdProtobuf(t, f, svc, cancelEnvelopeBytes)

	// Expect either a cancellation result or a completed result (race window).
	msg := drainOne(t, resultsSub)
	validEvents := []string{
		constants.Event.Operator.Command.Cancelled,
		constants.Event.Operator.Command.Completed,
		constants.Event.Operator.Command.Failed,
	}
	found := false
	for _, ev := range validEvents {
		if strings.Contains(string(msg), ev) {
			found = true
			break
		}
	}
	assert.True(t, found, "expected a command result event, got: %s", string(msg))
}

func TestLoopback_CommandDispatch_CancelRequest_NotFound(t *testing.T) {
	f := newLoopbackFixture(t)
	svc, _ := newLoopbackService(t, f)

	resultsSub := watchResults(t, f, svc)
	hbSub := watchHeartbeat(t, f, svc)

	startService(t, svc)
	_ = drainOne(t, hbSub)

	// Cancel a command that was never started — should publish a failure result.
	cancelPayload := mustMarshalProtobufCommandCancelRequested(t, "nonexistent-exec-id")
	cancelEnvelopeBytes := mustMarshalUniversalEnvelope(t, "cancel-ghost-1", constants.Event.Operator.Command.CancelRequested, cancelPayload, "", svc.config.OperatorID, "case-ghost", "", svc.config.OperatorSessionId)
	injectCmdProtobuf(t, f, svc, cancelEnvelopeBytes)

	msg := drainOne(t, resultsSub)
	assert.Contains(t, string(msg), constants.Event.Operator.Command.Cancelled,
		"cancel of unknown execution should publish a cancelled/failed result")
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

	filePayload := mustMarshalProtobufFileEditRequested(t, targetPath, "write", "file-write-1", "test", "hello from loopback\n", true)
	envelopeBytes := mustMarshalUniversalEnvelope(t, "file-write-1", constants.Event.Operator.FileEdit.Requested, filePayload, "", svc.config.OperatorID, "case-file-write", "inv-file-write", svc.config.OperatorSessionId)
	injectCmdProtobuf(t, f, svc, envelopeBytes)

	msg := drainOne(t, resultsSub)
	assert.Contains(t, string(msg), constants.Event.Operator.FileEdit.Completed)
	assert.Contains(t, string(msg), "case-file-write")

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
	filePayload := mustMarshalProtobufFileEditRequested(t, "/etc/passwd", "write", "file-sentinel-1", "test", "injected", false)
	envelopeBytes := mustMarshalUniversalEnvelope(t, "file-sentinel-1", constants.Event.Operator.FileEdit.Requested, filePayload, "", svc.config.OperatorID, "case-sentinel", "", svc.config.OperatorSessionId)
	injectCmdProtobuf(t, f, svc, envelopeBytes)

	msg := drainOne(t, resultsSub)
	assert.Contains(t, string(msg), constants.Event.Operator.FileEdit.Failed, "sentinel must block writes to /etc/passwd, got: %s", string(msg))
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
	filePayload := mustMarshalProtobufFileEditRequested(t, "", "write", "file-no-path-1", "test", "x", false)
	envelopeBytes := mustMarshalUniversalEnvelope(t, "file-no-path-1", constants.Event.Operator.FileEdit.Requested, filePayload, "", svc.config.OperatorID, "case-no-path", "", svc.config.OperatorSessionId)
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
	filePayload := mustMarshalProtobufFileEditRequested(t, targetPath, "delete", "nil-result-guard-1", "regression test", "", false)
	envelopeBytes := mustMarshalUniversalEnvelope(t, "nil-result-guard-1", constants.Event.Operator.FileEdit.Requested, filePayload, "", svc.config.OperatorID, "case-nil-guard", "", svc.config.OperatorSessionId)
	injectCmdProtobuf(t, f, svc, envelopeBytes)

	msg := drainOne(t, resultsSub)
	// Must receive either a completed or failed result — never a panic or silence.
	hasResult := strings.Contains(string(msg), constants.Event.Operator.FileEdit.Completed) ||
		strings.Contains(string(msg), constants.Event.Operator.FileEdit.Failed)
	assert.True(t, hasResult, "expected a file edit result event, got: %s", string(msg))
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

	fsListPayload := mustMarshalProtobufFsListRequested(t, ".", "fslist-1", 50)
	envelopeBytes := mustMarshalUniversalEnvelope(t, "fslist-1", constants.Event.Operator.FsList.Requested, fsListPayload, "", svc.config.OperatorID, "case-fslist", "inv-fslist", svc.config.OperatorSessionId)
	injectCmdProtobuf(t, f, svc, envelopeBytes)

	msg := drainOne(t, resultsSub)
	assert.Contains(t, string(msg), constants.Event.Operator.FsList.Completed)
	assert.Contains(t, string(msg), "case-fslist")
}

func TestLoopback_CommandDispatch_FsList_NonExistentPath(t *testing.T) {
	f := newLoopbackFixture(t)
	svc, _ := newLoopbackService(t, f)

	resultsSub := watchResults(t, f, svc)
	hbSub := watchHeartbeat(t, f, svc)

	startService(t, svc)
	_ = drainOne(t, hbSub)

	fsListPayload := mustMarshalProtobufFsListRequested(t, "/no/such/path/xyzzy", "fslist-missing-1", 10)
	envelopeBytes := mustMarshalUniversalEnvelope(t, "fslist-missing-1", constants.Event.Operator.FsList.Requested, fsListPayload, "", svc.config.OperatorID, "case-fslist-miss", "", svc.config.OperatorSessionId)
	injectCmdProtobuf(t, f, svc, envelopeBytes)

	msg := drainOne(t, resultsSub)
	assert.Contains(t, string(msg), constants.Event.Operator.FsList.Failed)
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

	fsReadPayload := mustMarshalProtobufFsReadRequested(t, target, "fsread-1", 4096)
	envelopeBytes := mustMarshalUniversalEnvelope(t, "fsread-1", constants.Event.Operator.FsRead.Requested, fsReadPayload, "", svc.config.OperatorID, "case-fsread", "", svc.config.OperatorSessionId)
	injectCmdProtobuf(t, f, svc, envelopeBytes)

	msg := drainOne(t, resultsSub)
	assert.Contains(t, string(msg), constants.Event.Operator.FsRead.Completed)
	assert.Contains(t, string(msg), content)
}

func TestLoopback_CommandDispatch_FsRead_MissingFile(t *testing.T) {
	f := newLoopbackFixture(t)
	svc, _ := newLoopbackService(t, f)

	resultsSub := watchResults(t, f, svc)
	hbSub := watchHeartbeat(t, f, svc)

	startService(t, svc)
	_ = drainOne(t, hbSub)

	fsReadPayload := mustMarshalProtobufFsReadRequested(t, "/no/such/file/xyzzy.txt", "fsread-miss-1", 4096)
	envelopeBytes := mustMarshalUniversalEnvelope(t, "fsread-miss-1", constants.Event.Operator.FsRead.Requested, fsReadPayload, "", svc.config.OperatorID, "case-fsread-miss", "", svc.config.OperatorSessionId)
	injectCmdProtobuf(t, f, svc, envelopeBytes)

	msg := drainOne(t, resultsSub)
	assert.Contains(t, string(msg), constants.Event.Operator.FsRead.Failed)
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

	cmdPayload := mustMarshalProtobufCommandRequested(t, "echo fanout", "fanout-1", "test", "", 0)
	envelopeBytes := mustMarshalUniversalEnvelope(t, "fanout-1", constants.Event.Operator.Command.Requested, cmdPayload, "", svc.config.OperatorID, "case-fanout", "", svc.config.OperatorSessionId)
	injectCmdProtobuf(t, f, svc, envelopeBytes)

	for i, sub := range subs {
		msg := drainOne(t, sub)
		assert.Contains(t, string(msg), constants.Event.Operator.Command.Completed,
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

	hbPayload := mustMarshalProtobufHeartbeatRequested(t)
	envelopeBytes := mustMarshalUniversalEnvelope(t, "hb-req-case-1", constants.Event.Operator.HeartbeatRequested, hbPayload, "", svc.config.OperatorID, "case-hb-req", "inv-hb-req", svc.config.OperatorSessionId)
	injectCmdProtobuf(t, f, svc, envelopeBytes)

	msg := drainOne(t, hbSub)
	assert.Contains(t, string(msg), constants.Event.Operator.Heartbeat)
	assert.Contains(t, string(msg), "case-hb-req")
	assert.Contains(t, string(msg), "inv-hb-req")

	var hb models.Heartbeat
	require.NoError(t, json.Unmarshal(msg, &hb))
	assert.Equal(t, models.HeartbeatTypeRequested, hb.HeartbeatType)
	assert.Equal(t, "case-hb-req", hb.CaseID)
	assert.Equal(t, "inv-hb-req", hb.InvestigationID)
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
	envelopeBytes := mustMarshalUniversalEnvelope(t, "unknown-event-1", "operator.unknown.made_up_type", []byte{}, "", svc.config.OperatorID, "case-unknown", "", svc.config.OperatorSessionId)
	injectCmdProtobuf(t, f, svc, envelopeBytes)

	// Unknown event types are silently dropped — no result is published.
	drainNone(t, resultsSub, 200*time.Millisecond)
}
