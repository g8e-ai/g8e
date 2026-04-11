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
	"encoding/json"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/g8e-ai/g8e/components/g8eo/constants"
	"github.com/g8e-ai/g8e/components/g8eo/models"
	execution "github.com/g8e-ai/g8e/components/g8eo/services/execution"
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
		msg := PubSubCommandMessage{
			ID:        fmt.Sprintf("real-echo-%d", time.Now().UnixNano()),
			EventType: constants.Event.Operator.Command.Requested,
			CaseID:    caseID,
			Payload:   json.RawMessage(`{"command":"echo hello real data flow","justification":"Real data flow test"}`),
			Timestamp: time.Now().UTC(),
		}

		msgJSON, err := json.Marshal(msg)
		require.NoError(t, err)

		testutil.PublishTestMessage(t, testutil.GetTestG8esDirectURL(), commandChannel, string(msgJSON))

		received := testutil.WaitForMessage(t, msgChan, 5*time.Second)
		require.NotNil(t, received)

		var result models.G8eMessage
		err = json.Unmarshal(received, &result)
		require.NoError(t, err)

		assert.Equal(t, constants.Event.Operator.Command.Completed, result.EventType)
		assert.Equal(t, caseID, result.CaseID)

		var payload models.ExecutionResultsPayload
		require.NoError(t, json.Unmarshal(result.Payload, &payload))
		assert.Contains(t, payload.Stdout, "hello real data flow")
		require.NotNil(t, payload.ReturnCode, "return_code must be present")
		assert.Equal(t, 0, *payload.ReturnCode)
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
		msg := PubSubCommandMessage{
			ID:        fmt.Sprintf("real-exit-%d", time.Now().UnixNano()),
			EventType: constants.Event.Operator.Command.Requested,
			CaseID:    caseID,
			Payload:   json.RawMessage(`{"command":"sh -c 'exit 42'","justification":"Non-zero exit code test"}`),
			Timestamp: time.Now().UTC(),
		}

		msgJSON, err := json.Marshal(msg)
		require.NoError(t, err)

		testutil.PublishTestMessage(t, testutil.GetTestG8esDirectURL(), commandChannel, string(msgJSON))

		received := testutil.WaitForMessage(t, msgChan, 5*time.Second)
		require.NotNil(t, received)

		var result models.G8eMessage
		err = json.Unmarshal(received, &result)
		require.NoError(t, err)

		var payload models.ExecutionResultsPayload
		require.NoError(t, json.Unmarshal(result.Payload, &payload))
		require.NotNil(t, payload.ReturnCode, "return_code must be present")
		assert.Equal(t, 42, *payload.ReturnCode)
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
		msg := PubSubCommandMessage{
			ID:        fmt.Sprintf("real-stderr-%d", time.Now().UnixNano()),
			EventType: constants.Event.Operator.Command.Requested,
			CaseID:    caseID,
			Payload:   json.RawMessage(`{"command":"sh -c 'echo stdout-msg \u0026\u0026 echo stderr-msg \u003e\u00262'","justification":"Stderr output test"}`),
			Timestamp: time.Now().UTC(),
		}

		msgJSON, err := json.Marshal(msg)
		require.NoError(t, err)

		testutil.PublishTestMessage(t, testutil.GetTestG8esDirectURL(), commandChannel, string(msgJSON))

		received := testutil.WaitForMessage(t, msgChan, 5*time.Second)
		require.NotNil(t, received)

		var result models.G8eMessage
		err = json.Unmarshal(received, &result)
		require.NoError(t, err)

		var payload models.ExecutionResultsPayload
		require.NoError(t, json.Unmarshal(result.Payload, &payload))
		assert.Contains(t, payload.Stdout, "stdout-msg")
		assert.Contains(t, payload.Stderr, "stderr-msg")
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

		caseID := fmt.Sprintf("case-%s-%d", t.Name(), time.Now().UnixNano())
		commandChannel := constants.CmdChannel(cfg.OperatorID, cfg.OperatorSessionId)
		tmpFile := filepath.Join(t.TempDir(), "g8es-flow-test.txt")

		// Step 1: Write file
		writeMsg := PubSubCommandMessage{
			ID:        fmt.Sprintf("flow-write-%d", time.Now().UnixNano()),
			EventType: constants.Event.Operator.FileEdit.Requested,
			CaseID:    caseID,
			Payload: mustMarshalJSON(t, models.FileEditPayload{
				Operation:       "write",
				FilePath:        tmpFile,
				Content:         "initial content",
				CreateIfMissing: true,
				Justification:   "File flow test - write",
			}),
			Timestamp: time.Now().UTC(),
		}

		writeMsgJSON, err := json.Marshal(writeMsg)
		require.NoError(t, err)

		testutil.PublishTestMessage(t, testutil.GetTestG8esDirectURL(), commandChannel, string(writeMsgJSON))

		writeResult := testutil.WaitForMessage(t, msgChan, 5*time.Second)
		require.NotNil(t, writeResult)
		assert.Contains(t, string(writeResult), "file.edit.completed")

		// Step 2: Read file
		readMsg := PubSubCommandMessage{
			ID:        fmt.Sprintf("flow-read-%d", time.Now().UnixNano()),
			EventType: constants.Event.Operator.FileEdit.Requested,
			CaseID:    caseID,
			Payload: mustMarshalJSON(t, models.FileEditPayload{
				Operation:     "read",
				FilePath:      tmpFile,
				Justification: "File flow test - read",
			}),
			Timestamp: time.Now().UTC(),
		}

		readMsgJSON, err := json.Marshal(readMsg)
		require.NoError(t, err)

		testutil.PublishTestMessage(t, testutil.GetTestG8esDirectURL(), commandChannel, string(readMsgJSON))

		readResult := testutil.WaitForMessage(t, msgChan, 5*time.Second)
		require.NotNil(t, readResult)
		assert.Contains(t, string(readResult), "file.edit.completed")
		assert.Contains(t, string(readResult), "initial content")

		// Step 3: Replace content
		replaceMsg := PubSubCommandMessage{
			ID:        fmt.Sprintf("flow-replace-%d", time.Now().UnixNano()),
			EventType: constants.Event.Operator.FileEdit.Requested,
			CaseID:    caseID,
			Payload: mustMarshalJSON(t, models.FileEditPayload{
				Operation:     "replace",
				FilePath:      tmpFile,
				OldContent:    "initial content",
				NewContent:    "modified content",
				Justification: "File flow test - replace",
			}),
			Timestamp: time.Now().UTC(),
		}

		replaceMsgJSON, err := json.Marshal(replaceMsg)
		require.NoError(t, err)

		testutil.PublishTestMessage(t, testutil.GetTestG8esDirectURL(), commandChannel, string(replaceMsgJSON))

		replaceResult := testutil.WaitForMessage(t, msgChan, 5*time.Second)
		require.NotNil(t, replaceResult)
		assert.Contains(t, string(replaceResult), "file.edit.completed")

		// Step 4: Read modified file
		readMsg2 := PubSubCommandMessage{
			ID:        fmt.Sprintf("flow-read2-%d", time.Now().UnixNano()),
			EventType: constants.Event.Operator.FileEdit.Requested,
			CaseID:    caseID,
			Payload: mustMarshalJSON(t, models.FileEditPayload{
				Operation:     "read",
				FilePath:      tmpFile,
				Justification: "File flow test - read modified",
			}),
			Timestamp: time.Now().UTC(),
		}

		readMsg2JSON, err := json.Marshal(readMsg2)
		require.NoError(t, err)

		testutil.PublishTestMessage(t, testutil.GetTestG8esDirectURL(), commandChannel, string(readMsg2JSON))

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
		msg := PubSubCommandMessage{
			ID:        fmt.Sprintf("real-large-%d", time.Now().UnixNano()),
			EventType: constants.Event.Operator.Command.Requested,
			CaseID:    caseID,
			Payload:   json.RawMessage(`{"command":"sh -c 'for i in $(seq 1 50); do echo Line $i: data; done'","justification":"Large output test"}`),
			Timestamp: time.Now().UTC(),
		}

		msgJSON, err := json.Marshal(msg)
		require.NoError(t, err)

		testutil.PublishTestMessage(t, testutil.GetTestG8esDirectURL(), commandChannel, string(msgJSON))

		received := testutil.WaitForMessage(t, msgChan, 10*time.Second)
		require.NotNil(t, received)

		var result models.G8eMessage
		err = json.Unmarshal(received, &result)
		require.NoError(t, err)

		assert.Equal(t, constants.Event.Operator.Command.Completed, result.EventType)
		var payload models.ExecutionResultsPayload
		require.NoError(t, json.Unmarshal(result.Payload, &payload))
		assert.Contains(t, payload.Stdout, "Line 1: data")
		assert.Contains(t, payload.Stdout, "Line 50: data")
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
		msg := PubSubCommandMessage{
			ID:        fmt.Sprintf("real-error-%d", time.Now().UnixNano()),
			EventType: constants.Event.Operator.FileEdit.Requested,
			CaseID:    caseID,
			Payload:   json.RawMessage(`{"operation":"read","file_path":"/nonexistent/path/file.txt","justification":"Error scenario test"}`),
			Timestamp: time.Now().UTC(),
		}

		msgJSON, err := json.Marshal(msg)
		require.NoError(t, err)

		testutil.PublishTestMessage(t, testutil.GetTestG8esDirectURL(), commandChannel, string(msgJSON))

		received := testutil.WaitForMessage(t, msgChan, 5*time.Second)
		require.NotNil(t, received)
		assert.Contains(t, string(received), "file.edit.failed")
	})
}
