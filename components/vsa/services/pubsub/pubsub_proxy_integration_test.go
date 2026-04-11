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
	"sync"
	"testing"
	"time"

	"github.com/g8e-ai/g8e/components/vsa/constants"
	"github.com/g8e-ai/g8e/components/vsa/models"
	execution "github.com/g8e-ai/g8e/components/vsa/services/execution"
	"github.com/g8e-ai/g8e/components/vsa/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type testPubSubPayload struct {
	Key       string `json:"key"`
	Timestamp int64  `json:"timestamp"`
}

// TestVSODBPubSubConnection tests connection to the VSODB pub/sub endpoint
func TestVSODBPubSubConnection(t *testing.T) {
	testutil.TestPubSubAvailable(t)

	t.Run("connects to VSODB pub/sub endpoint", func(t *testing.T) {
		client := NewTestPubSubClient(t)
		assert.NotNil(t, client, "Should connect to VSODB pub/sub endpoint")
	})

	t.Run("performs pub/sub round-trip through VSODB", func(t *testing.T) {
		channel := fmt.Sprintf("test:proxy:%d", time.Now().UnixNano())
		msgChan := testutil.SubscribeToChannel(t, testutil.GetTestVSODBDirectURL(), channel)

		time.Sleep(50 * time.Millisecond)

		testutil.PublishTestMessage(t, testutil.GetTestVSODBDirectURL(), channel, "test-value")

		received := testutil.WaitForMessage(t, msgChan, 2*time.Second)
		assert.NotNil(t, received)
		assert.Contains(t, string(received), "test-value")
	})

	t.Run("handles pub/sub with JSON payload", func(t *testing.T) {
		channel := fmt.Sprintf("test:channel:%d", time.Now().UnixNano())
		msgChan := testutil.SubscribeToChannel(t, testutil.GetTestVSODBDirectURL(), channel)

		time.Sleep(50 * time.Millisecond)

		payload := testPubSubPayload{
			Key:       "value",
			Timestamp: time.Now().Unix(),
		}
		payloadJSON, err := json.Marshal(payload)
		require.NoError(t, err)

		testutil.PublishTestMessage(t, testutil.GetTestVSODBDirectURL(), channel, string(payloadJSON))

		received := testutil.WaitForMessage(t, msgChan, 2*time.Second)
		assert.NotNil(t, received)
		assert.Contains(t, string(received), "value")
	})
}

// TestVSODBPubSubCommandFlow tests the full command flow through VSODB pub/sub
func TestVSODBPubSubCommandFlow(t *testing.T) {

	t.Run("command execution flow through VSODB pub/sub", func(t *testing.T) {
		db := NewTestPubSubClient(t)

		cfg := testutil.NewTestConfig(t)

		logger := testutil.NewTestLogger()

		execSvc := execution.NewExecutionService(cfg, logger)
		fileEditSvc := execution.NewFileEditService(cfg, logger)

		svc, err := NewPubSubCommandService(CommandServiceConfig{Config: cfg, Logger: logger, Execution: execSvc, FileEdit: fileEditSvc, PubSubClient: nil})
		require.NoError(t, err)

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
		msgChan := testutil.SubscribeToChannel(t, testutil.GetTestVSODBDirectURL(), resultsChannel)

		time.Sleep(100 * time.Millisecond)

		caseID := fmt.Sprintf("case-%s-%d", t.Name(), time.Now().UnixNano())
		commandChannel := constants.CmdChannel(cfg.OperatorID, cfg.OperatorSessionId)
		msg := PubSubCommandMessage{
			ID:        fmt.Sprintf("proxy-exec-%d", time.Now().UnixNano()),
			EventType: constants.Event.Operator.Command.Requested,
			CaseID:    caseID,
			Payload:   json.RawMessage(`{"command":"echo hello from vsodb","justification":"Integration test via VSODB pub/sub"}`),
			Timestamp: time.Now().UTC(),
		}

		msgJSON, err := json.Marshal(msg)
		require.NoError(t, err)

		testutil.PublishTestMessage(t, testutil.GetTestVSODBDirectURL(), commandChannel, string(msgJSON))

		received := testutil.WaitForMessage(t, msgChan, 5*time.Second)
		require.NotNil(t, received)
		assert.Contains(t, string(received), constants.Event.Operator.Command.Completed)
		assert.Contains(t, string(received), "hello from vsodb")
	})

	t.Run("heartbeat flow through VSODB pub/sub", func(t *testing.T) {
		db := NewTestPubSubClient(t)

		cfg := testutil.NewTestConfig(t)

		logger := testutil.NewTestLogger()

		execSvc := execution.NewExecutionService(cfg, logger)
		fileEditSvc := execution.NewFileEditService(cfg, logger)

		svc, err := NewPubSubCommandService(CommandServiceConfig{Config: cfg, Logger: logger, Execution: execSvc, FileEdit: fileEditSvc, PubSubClient: nil})
		require.NoError(t, err)

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

		heartbeatChannel := constants.HeartbeatChannel(cfg.OperatorID, cfg.OperatorSessionId)
		msgChan := testutil.SubscribeToChannel(t, testutil.GetTestVSODBDirectURL(), heartbeatChannel)

		time.Sleep(100 * time.Millisecond)

		caseID := fmt.Sprintf("case-%s-%d", t.Name(), time.Now().UnixNano())
		commandChannel := constants.CmdChannel(cfg.OperatorID, cfg.OperatorSessionId)
		msg := PubSubCommandMessage{
			ID:        fmt.Sprintf("proxy-hb-%d", time.Now().UnixNano()),
			EventType: constants.Event.Operator.HeartbeatRequested,
			CaseID:    caseID,
			Payload:   json.RawMessage(`{}`),
			Timestamp: time.Now().UTC(),
		}

		msgJSON, err := json.Marshal(msg)
		require.NoError(t, err)

		testutil.PublishTestMessage(t, testutil.GetTestVSODBDirectURL(), commandChannel, string(msgJSON))

		received := testutil.WaitForMessage(t, msgChan, 3*time.Second)
		require.NotNil(t, received)
		assert.Contains(t, string(received), constants.Event.Operator.Heartbeat)
	})

	t.Run("file edit flow through VSODB pub/sub", func(t *testing.T) {
		db := NewTestPubSubClient(t)

		cfg := testutil.NewTestConfig(t)

		logger := testutil.NewTestLogger()

		execSvc := execution.NewExecutionService(cfg, logger)
		fileEditSvc := execution.NewFileEditService(cfg, logger)

		svc, err := NewPubSubCommandService(CommandServiceConfig{Config: cfg, Logger: logger, Execution: execSvc, FileEdit: fileEditSvc, PubSubClient: nil})
		require.NoError(t, err)

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
		msgChan := testutil.SubscribeToChannel(t, testutil.GetTestVSODBDirectURL(), resultsChannel)

		time.Sleep(100 * time.Millisecond)

		caseID := fmt.Sprintf("case-%s-%d", t.Name(), time.Now().UnixNano())
		tmpFile := filepath.Join(t.TempDir(), "vsodb-proxy-test.txt")
		commandChannel := constants.CmdChannel(cfg.OperatorID, cfg.OperatorSessionId)
		msg := PubSubCommandMessage{
			ID:        fmt.Sprintf("proxy-file-%d", time.Now().UnixNano()),
			EventType: constants.Event.Operator.FileEdit.Requested,
			CaseID:    caseID,
			Payload: mustMarshalJSON(t, models.FileEditPayload{
				Operation:       "write",
				FilePath:        tmpFile,
				Content:         "hello from vsodb file edit",
				CreateIfMissing: true,
				Justification:   "Integration test file write via VSODB pub/sub",
			}),
			Timestamp: time.Now().UTC(),
		}

		msgJSON, err := json.Marshal(msg)
		require.NoError(t, err)

		testutil.PublishTestMessage(t, testutil.GetTestVSODBDirectURL(), commandChannel, string(msgJSON))

		received := testutil.WaitForMessage(t, msgChan, 5*time.Second)
		require.NotNil(t, received)
		assert.Contains(t, string(received), "file.edit.completed")
	})
}

// TestVSODBPubSubConnectionResilience tests connection resilience
func TestVSODBPubSubConnectionResilience(t *testing.T) {
	testutil.TestPubSubAvailable(t)

	t.Run("multiple subscribers on same channel", func(t *testing.T) {
		channel := fmt.Sprintf("test:multi-sub:%d", time.Now().UnixNano())

		sub1 := testutil.SubscribeToChannel(t, testutil.GetTestVSODBDirectURL(), channel)
		sub2 := testutil.SubscribeToChannel(t, testutil.GetTestVSODBDirectURL(), channel)

		time.Sleep(50 * time.Millisecond)

		testutil.PublishTestMessage(t, testutil.GetTestVSODBDirectURL(), channel, "broadcast-message")

		msg1 := testutil.WaitForMessage(t, sub1, 2*time.Second)
		msg2 := testutil.WaitForMessage(t, sub2, 2*time.Second)

		assert.NotNil(t, msg1)
		assert.NotNil(t, msg2)
		assert.Contains(t, string(msg1), "broadcast-message")
		assert.Contains(t, string(msg2), "broadcast-message")
	})

	t.Run("concurrent pub/sub operations", func(t *testing.T) {
		const numOps = 5
		var wg sync.WaitGroup
		results := make([][]byte, numOps)

		for i := 0; i < numOps; i++ {
			wg.Add(1)
			go func(idx int) {
				defer wg.Done()
				channel := fmt.Sprintf("test:concurrent:%d:%d", time.Now().UnixNano(), idx)
				msgChan := testutil.SubscribeToChannel(t, testutil.GetTestVSODBDirectURL(), channel)

				time.Sleep(50 * time.Millisecond)

				testutil.PublishTestMessage(t, testutil.GetTestVSODBDirectURL(), channel,
					fmt.Sprintf("message-%d", idx))

				results[idx] = testutil.WaitForMessage(t, msgChan, 2*time.Second)
			}(i)
		}

		wg.Wait()

		for i, result := range results {
			assert.NotNil(t, result, "Result %d should not be nil", i)
			assert.Contains(t, string(result), fmt.Sprintf("message-%d", i))
		}
	})

	t.Run("TLS configuration via VSODBPubSubClient", func(t *testing.T) {
		client := NewTestPubSubClient(t)
		assert.NotNil(t, client)

		channel := fmt.Sprintf("test:tls:%d", time.Now().UnixNano())
		ctx := context.Background()

		payload := models.Heartbeat{
			EventType:       constants.Event.Operator.Heartbeat,
			SourceComponent: "vsa",
			Timestamp:       models.NowTimestamp(),
		}
		data, err := json.Marshal(payload)
		require.NoError(t, err)

		err = client.Publish(ctx, channel, data)
		assert.NoError(t, err, "Should publish via TLS-secured VSODB connection")
	})
}

// TestVSODBPubSubFullWorkflow tests the complete VSA workflow through VSODB pub/sub
func TestVSODBPubSubFullWorkflow(t *testing.T) {

	t.Run("complete command lifecycle", func(t *testing.T) {
		db := NewTestPubSubClient(t)

		cfg := testutil.NewTestConfig(t)

		cfg.HeartbeatInterval = 500 * time.Millisecond
		logger := testutil.NewTestLogger()

		execSvc := execution.NewExecutionService(cfg, logger)
		fileEditSvc := execution.NewFileEditService(cfg, logger)

		svc, err := NewPubSubCommandService(CommandServiceConfig{Config: cfg, Logger: logger, Execution: execSvc, FileEdit: fileEditSvc, PubSubClient: nil})
		require.NoError(t, err)

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
		msgChan := testutil.SubscribeToChannel(t, testutil.GetTestVSODBDirectURL(), resultsChannel)

		time.Sleep(100 * time.Millisecond)

		commandChannel := constants.CmdChannel(cfg.OperatorID, cfg.OperatorSessionId)

		commands := []struct {
			id      string
			command string
			args    []string
		}{
			{"wf-1", "echo", []string{"step-1"}},
			{"wf-2", "echo", []string{"step-2"}},
			{"wf-3", "echo", []string{"step-3"}},
		}

		caseID := fmt.Sprintf("case-%s-%d", t.Name(), time.Now().UnixNano())
		for _, cmd := range commands {
			msg := PubSubCommandMessage{
				ID:        cmd.id,
				EventType: constants.Event.Operator.Command.Requested,
				CaseID:    caseID,
				Payload: mustMarshalJSON(t, models.CommandPayload{
					Command:       cmd.command,
					Justification: "Workflow test",
				}),
				Timestamp: time.Now().UTC(),
			}

			msgJSON, err := json.Marshal(msg)
			require.NoError(t, err)

			testutil.PublishTestMessage(t, testutil.GetTestVSODBDirectURL(), commandChannel, string(msgJSON))

			received := testutil.WaitForMessage(t, msgChan, 5*time.Second)
			require.NotNil(t, received, "Expected result for command %s", cmd.id)
			assert.Contains(t, string(received), constants.Event.Operator.Command.Completed)
		}
	})
}
