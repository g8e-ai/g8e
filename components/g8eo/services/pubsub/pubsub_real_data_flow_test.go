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

package pubsub

import (
	"context"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/g8e-ai/g8e/components/g8eo/constants"
	execution "github.com/g8e-ai/g8e/components/g8eo/services/execution"
	"github.com/g8e-ai/g8e/components/g8eo/shared/proto/operatorv1"
	"github.com/g8e-ai/g8e/components/g8eo/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestRealDataFlow_CommandExecution tests real command execution through the full g8eo stack
func TestRealDataFlow_CommandExecution(t *testing.T) {

	t.Run("executes echo command and receives result via g8es pub/sub", func(t *testing.T) {
		db := NewTestPubSubClient(t)

		cfg := testutil.NewTestConfig(t)

		logger := testutil.NewTestLogger()

		execSvc := execution.NewExecutionService(cfg, logger)
		fileEditSvc := execution.NewFileEditService(cfg, logger)

		resultsSvc, err := NewPubSubResultsService(cfg, logger, db, nil)
		require.NoError(t, err)

		svc, err := NewPubSubCommandService(CommandServiceConfig{
			Config:         cfg,
			Logger:         logger,
			Execution:      execSvc,
			FileEdit:       fileEditSvc,
			PubSubClient:   db,
			ResultsService: resultsSvc,
		})
		require.NoError(t, err)

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		err = svc.Start(ctx)
		require.NoError(t, err)
		defer svc.Stop()

		resultsChannel := constants.ResultsChannel(cfg.OperatorID, cfg.OperatorSessionId)
		msgChan := testutil.SubscribeToChannel(t, testutil.GetTestG8esDirectURL(), resultsChannel)

		time.Sleep(100 * time.Millisecond)

		caseID := fmt.Sprintf("case-%s-%d", t.Name(), time.Now().UnixNano())
		commandChannel := constants.CmdChannel(cfg.OperatorID, cfg.OperatorSessionId)
		execID := fmt.Sprintf("real-echo-%d", time.Now().UnixNano())
		cmdPayload := testutil.MustMarshalProtobufCommandRequested(t, "echo hello real data flow", execID, "Real data flow test", "", 0)
		envelopeBytes := testutil.MustMarshalUniversalEnvelope(t, execID, constants.Event.Operator.Command.Requested, cmdPayload, "", cfg.OperatorID, caseID, "inv-real-flow", cfg.OperatorSessionId)

		testutil.PublishTestMessage(t, testutil.GetTestG8esDirectURL(), commandChannel, string(envelopeBytes))

		received := testutil.WaitForMessage(t, msgChan, 5*time.Second)
		require.NotNil(t, received)

		env := testutil.MustUnmarshalUniversalEnvelope(t, received)
		assert.Equal(t, constants.Event.Operator.Command.Completed, env.EventType)
		assert.Equal(t, caseID, env.CaseId)

		var payload operatorv1.CommandResult
		testutil.MustUnmarshalPayload(t, env.Payload, &payload)
		assert.Contains(t, payload.Output, "hello real data flow")
		assert.Equal(t, int32(0), payload.ExitCode)
	})

	t.Run("executes command with non-zero exit code", func(t *testing.T) {
		db := NewTestPubSubClient(t)

		cfg := testutil.NewTestConfig(t)

		logger := testutil.NewTestLogger()

		execSvc := execution.NewExecutionService(cfg, logger)
		fileEditSvc := execution.NewFileEditService(cfg, logger)

		resultsSvc, err := NewPubSubResultsService(cfg, logger, db, nil)
		require.NoError(t, err)

		svc, err := NewPubSubCommandService(CommandServiceConfig{
			Config:         cfg,
			Logger:         logger,
			Execution:      execSvc,
			FileEdit:       fileEditSvc,
			PubSubClient:   db,
			ResultsService: resultsSvc,
		})
		require.NoError(t, err)

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		err = svc.Start(ctx)
		require.NoError(t, err)
		defer svc.Stop()

		resultsChannel := constants.ResultsChannel(cfg.OperatorID, cfg.OperatorSessionId)
		msgChan := testutil.SubscribeToChannel(t, testutil.GetTestG8esDirectURL(), resultsChannel)

		time.Sleep(100 * time.Millisecond)

		caseID := fmt.Sprintf("case-%s-%d", t.Name(), time.Now().UnixNano())
		commandChannel := constants.CmdChannel(cfg.OperatorID, cfg.OperatorSessionId)
		execID := fmt.Sprintf("real-exit-%d", time.Now().UnixNano())
		cmdPayload := testutil.MustMarshalProtobufCommandRequested(t, "sh -c 'exit 42'", execID, "Non-zero exit code test", "", 0)
		envelopeBytes := testutil.MustMarshalUniversalEnvelope(t, execID, constants.Event.Operator.Command.Requested, cmdPayload, "", cfg.OperatorID, caseID, "inv-real-exit", cfg.OperatorSessionId)

		testutil.PublishTestMessage(t, testutil.GetTestG8esDirectURL(), commandChannel, string(envelopeBytes))

		received := testutil.WaitForMessage(t, msgChan, 5*time.Second)
		require.NotNil(t, received)

		env := testutil.MustUnmarshalUniversalEnvelope(t, received)
		var payload operatorv1.CommandResult
		testutil.MustUnmarshalPayload(t, env.Payload, &payload)
		assert.Equal(t, int32(42), payload.ExitCode)
	})

	t.Run("executes command with stderr output", func(t *testing.T) {
		db := NewTestPubSubClient(t)

		cfg := testutil.NewTestConfig(t)

		logger := testutil.NewTestLogger()

		execSvc := execution.NewExecutionService(cfg, logger)
		fileEditSvc := execution.NewFileEditService(cfg, logger)

		resultsSvc, err := NewPubSubResultsService(cfg, logger, db, nil)
		require.NoError(t, err)

		svc, err := NewPubSubCommandService(CommandServiceConfig{
			Config:         cfg,
			Logger:         logger,
			Execution:      execSvc,
			FileEdit:       fileEditSvc,
			PubSubClient:   db,
			ResultsService: resultsSvc,
		})
		require.NoError(t, err)

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		err = svc.Start(ctx)
		require.NoError(t, err)
		defer svc.Stop()

		resultsChannel := constants.ResultsChannel(cfg.OperatorID, cfg.OperatorSessionId)
		msgChan := testutil.SubscribeToChannel(t, testutil.GetTestG8esDirectURL(), resultsChannel)

		time.Sleep(100 * time.Millisecond)

		caseID := fmt.Sprintf("case-%s-%d", t.Name(), time.Now().UnixNano())
		commandChannel := constants.CmdChannel(cfg.OperatorID, cfg.OperatorSessionId)
		execID := fmt.Sprintf("real-stderr-%d", time.Now().UnixNano())
		cmdPayload := testutil.MustMarshalProtobufCommandRequested(t, "sh -c 'echo error message >&2'", execID, "Stderr output test", "", 0)
		envelopeBytes := testutil.MustMarshalUniversalEnvelope(t, execID, constants.Event.Operator.Command.Requested, cmdPayload, "", cfg.OperatorID, caseID, "inv-real-stderr", cfg.OperatorSessionId)

		testutil.PublishTestMessage(t, testutil.GetTestG8esDirectURL(), commandChannel, string(envelopeBytes))

		received := testutil.WaitForMessage(t, msgChan, 5*time.Second)
		require.NotNil(t, received)

		env := testutil.MustUnmarshalUniversalEnvelope(t, received)
		var payload operatorv1.CommandResult
		testutil.MustUnmarshalPayload(t, env.Payload, &payload)
		assert.Contains(t, payload.Output, "error message")
	})
}

// TestRealDataFlow_FileOperations tests real file operations through the full g8eo stack
func TestRealDataFlow_FileOperations(t *testing.T) {

	t.Run("create, read, modify, read file workflow", func(t *testing.T) {
		db := NewTestPubSubClient(t)

		cfg := testutil.NewTestConfig(t)

		logger := testutil.NewTestLogger()

		execSvc := execution.NewExecutionService(cfg, logger)
		fileEditSvc := execution.NewFileEditService(cfg, logger)

		resultsSvc, err := NewPubSubResultsService(cfg, logger, db, nil)
		require.NoError(t, err)

		svc, err := NewPubSubCommandService(CommandServiceConfig{
			Config:         cfg,
			Logger:         logger,
			Execution:      execSvc,
			FileEdit:       fileEditSvc,
			PubSubClient:   db,
			ResultsService: resultsSvc,
		})
		require.NoError(t, err)

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		err = svc.Start(ctx)
		require.NoError(t, err)
		defer svc.Stop()

		resultsChannel := constants.ResultsChannel(cfg.OperatorID, cfg.OperatorSessionId)
		msgChan := testutil.SubscribeToChannel(t, testutil.GetTestG8esDirectURL(), resultsChannel)

		time.Sleep(100 * time.Millisecond)

		caseID := fmt.Sprintf("case-flow-multi-%d", time.Now().UnixNano())
		commandChannel := constants.CmdChannel(cfg.OperatorID, cfg.OperatorSessionId)
		tmpFile := filepath.Join(t.TempDir(), "g8es-flow-test.txt")

		// Step 1: Write file
		execID1 := fmt.Sprintf("flow-write-%d", time.Now().UnixNano())
		writePayload := testutil.MustMarshalProtobufFileEditRequested(t, testutil.FileEditRequestFields{
			FilePath:        tmpFile,
			Operation:       "write",
			ExecutionId:     execID1,
			Justification:   "File flow test - write",
			Content:         "initial content",
			CreateIfMissing: true,
		})
		envelopeBytes1 := testutil.MustMarshalUniversalEnvelope(t, execID1, constants.Event.Operator.FileEdit.Requested, writePayload, "", cfg.OperatorID, caseID, "inv-write", cfg.OperatorSessionId)

		testutil.PublishTestMessage(t, testutil.GetTestG8esDirectURL(), commandChannel, string(envelopeBytes1))

		writeResult := testutil.WaitForMessage(t, msgChan, 5*time.Second)
		require.NotNil(t, writeResult)
		assert.Contains(t, string(writeResult), "file.edit.completed")

		// Step 2: Read file
		execID2 := fmt.Sprintf("flow-read-%d", time.Now().UnixNano())
		readPayload := testutil.MustMarshalProtobufFileEditRequested(t, testutil.FileEditRequestFields{
			FilePath:      tmpFile,
			Operation:     "read",
			ExecutionId:   execID2,
			Justification: "File flow test - read",
		})
		envelopeBytes2 := testutil.MustMarshalUniversalEnvelope(t, execID2, constants.Event.Operator.FileEdit.Requested, readPayload, "", cfg.OperatorID, caseID, "inv-read", cfg.OperatorSessionId)

		testutil.PublishTestMessage(t, testutil.GetTestG8esDirectURL(), commandChannel, string(envelopeBytes2))

		readResult := testutil.WaitForMessage(t, msgChan, 5*time.Second)
		require.NotNil(t, readResult)
		assert.Contains(t, string(readResult), "file.edit.completed")
		assert.Contains(t, string(readResult), "initial content")

		// Step 3: Replace content
		execID3 := fmt.Sprintf("flow-replace-%d", time.Now().UnixNano())
		replacePayload := testutil.MustMarshalProtobufFileEditRequested(t, testutil.FileEditRequestFields{
			FilePath:      tmpFile,
			Operation:     "replace",
			ExecutionId:   execID3,
			Justification: "File flow test - replace",
			OldContent:    "initial content",
			NewContent:    "modified content",
		})
		envelopeBytes3 := testutil.MustMarshalUniversalEnvelope(t, execID3, constants.Event.Operator.FileEdit.Requested, replacePayload, "", cfg.OperatorID, caseID, "inv-replace", cfg.OperatorSessionId)

		testutil.PublishTestMessage(t, testutil.GetTestG8esDirectURL(), commandChannel, string(envelopeBytes3))

		replaceResult := testutil.WaitForMessage(t, msgChan, 5*time.Second)
		require.NotNil(t, replaceResult)
		assert.Contains(t, string(replaceResult), "file.edit.completed")

		// Step 4: Read modified file
		execID4 := fmt.Sprintf("flow-read2-%d", time.Now().UnixNano())
		readPayload2 := testutil.MustMarshalProtobufFileEditRequested(t, testutil.FileEditRequestFields{
			FilePath:      tmpFile,
			Operation:     "read",
			ExecutionId:   execID4,
			Justification: "File flow test - read modified",
		})
		envelopeBytes4 := testutil.MustMarshalUniversalEnvelope(t, execID4, constants.Event.Operator.FileEdit.Requested, readPayload2, "", cfg.OperatorID, caseID, "inv-read-mod", cfg.OperatorSessionId)

		testutil.PublishTestMessage(t, testutil.GetTestG8esDirectURL(), commandChannel, string(envelopeBytes4))

		readResult2 := testutil.WaitForMessage(t, msgChan, 5*time.Second)
		require.NotNil(t, readResult2)
		assert.Contains(t, string(readResult2), "file.edit.completed")
		assert.Contains(t, string(readResult2), "modified content")
	})

	t.Run("handles large payload through g8es pub/sub", func(t *testing.T) {
		db := NewTestPubSubClient(t)

		cfg := testutil.NewTestConfig(t)

		logger := testutil.NewTestLogger()

		execSvc := execution.NewExecutionService(cfg, logger)
		fileEditSvc := execution.NewFileEditService(cfg, logger)

		resultsSvc, err := NewPubSubResultsService(cfg, logger, db, nil)
		require.NoError(t, err)

		svc, err := NewPubSubCommandService(CommandServiceConfig{
			Config:         cfg,
			Logger:         logger,
			Execution:      execSvc,
			FileEdit:       fileEditSvc,
			PubSubClient:   db,
			ResultsService: resultsSvc,
		})
		require.NoError(t, err)

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		err = svc.Start(ctx)
		require.NoError(t, err)
		defer svc.Stop()

		resultsChannel := constants.ResultsChannel(cfg.OperatorID, cfg.OperatorSessionId)
		msgChan := testutil.SubscribeToChannel(t, testutil.GetTestG8esDirectURL(), resultsChannel)

		time.Sleep(100 * time.Millisecond)

		caseID := fmt.Sprintf("case-%s-%d", t.Name(), time.Now().UnixNano())
		commandChannel := constants.CmdChannel(cfg.OperatorID, cfg.OperatorSessionId)
		execID := fmt.Sprintf("real-large-%d", time.Now().UnixNano())
		cmdPayload := testutil.MustMarshalProtobufCommandRequested(t, "sh -c 'for i in $(seq 1 50); do echo Line $i: data; done'", execID, "Large output test", "", 0)
		envelopeBytes := testutil.MustMarshalUniversalEnvelope(t, execID, constants.Event.Operator.Command.Requested, cmdPayload, "", cfg.OperatorID, caseID, "inv-real-large", cfg.OperatorSessionId)

		testutil.PublishTestMessage(t, testutil.GetTestG8esDirectURL(), commandChannel, string(envelopeBytes))

		received := testutil.WaitForMessage(t, msgChan, 10*time.Second)
		require.NotNil(t, received)

		env := testutil.MustUnmarshalUniversalEnvelope(t, received)
		assert.Equal(t, constants.Event.Operator.Command.Completed, env.EventType)
		var payload operatorv1.CommandResult
		testutil.MustUnmarshalPayload(t, env.Payload, &payload)
		assert.Contains(t, payload.Output, "Line 1: data")
		assert.Contains(t, payload.Output, "Line 50: data")
	})

	t.Run("handles error scenario gracefully", func(t *testing.T) {
		db := NewTestPubSubClient(t)

		cfg := testutil.NewTestConfig(t)

		logger := testutil.NewTestLogger()

		execSvc := execution.NewExecutionService(cfg, logger)
		fileEditSvc := execution.NewFileEditService(cfg, logger)

		resultsSvc, err := NewPubSubResultsService(cfg, logger, db, nil)
		require.NoError(t, err)

		svc, err := NewPubSubCommandService(CommandServiceConfig{
			Config:         cfg,
			Logger:         logger,
			Execution:      execSvc,
			FileEdit:       fileEditSvc,
			PubSubClient:   db,
			ResultsService: resultsSvc,
		})
		require.NoError(t, err)

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		err = svc.Start(ctx)
		require.NoError(t, err)
		defer svc.Stop()

		resultsChannel := constants.ResultsChannel(cfg.OperatorID, cfg.OperatorSessionId)
		msgChan := testutil.SubscribeToChannel(t, testutil.GetTestG8esDirectURL(), resultsChannel)

		time.Sleep(100 * time.Millisecond)

		caseID := fmt.Sprintf("case-%s-%d", t.Name(), time.Now().UnixNano())
		commandChannel := constants.CmdChannel(cfg.OperatorID, cfg.OperatorSessionId)
		execID := fmt.Sprintf("real-error-%d", time.Now().UnixNano())
		readPayload := testutil.MustMarshalProtobufFileEditRequested(t, testutil.FileEditRequestFields{
			Operation:     "read",
			FilePath:      "/nonexistent/path/file.txt",
			ExecutionId:   execID,
			Justification: "Error scenario test",
		})
		envelopeBytes := testutil.MustMarshalUniversalEnvelope(t, execID, constants.Event.Operator.FileEdit.Requested, readPayload, "", cfg.OperatorID, caseID, "inv-real-error", cfg.OperatorSessionId)

		testutil.PublishTestMessage(t, testutil.GetTestG8esDirectURL(), commandChannel, string(envelopeBytes))

		received := testutil.WaitForMessage(t, msgChan, 5*time.Second)
		require.NotNil(t, received)
		assert.Contains(t, string(received), "file.edit.failed")
	})
}
