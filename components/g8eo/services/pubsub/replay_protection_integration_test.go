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
	"testing"
	"time"

	"github.com/g8e-ai/g8e/components/g8eo/constants"
	execution "github.com/g8e-ai/g8e/components/g8eo/services/execution"
	storage "github.com/g8e-ai/g8e/components/g8eo/services/storage"
	"github.com/g8e-ai/g8e/components/g8eo/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestReplayProtection_Integration verifies that g8eo correctly identifies and
// rejects replayed transactions using the persistent KV store.
func TestReplayProtection_Integration(t *testing.T) {
	testutil.TestPubSubAvailable(t)

	db := NewTestPubSubClient(t)
	cfg := testutil.NewTestConfig(t)
	logger := testutil.NewTestLogger()

	execSvc := execution.NewExecutionService(cfg, logger)
	fileEditSvc := execution.NewFileEditService(cfg, logger)
	localStore := storage.NewLocalStoreService(cfg, logger)
	
	resultsSvc, err := NewPubSubResultsService(cfg, logger, db, nil)
	require.NoError(t, err)

	svc, err := NewPubSubCommandService(CommandServiceConfig{
		Config:         cfg,
		Logger:         logger,
		Execution:      execSvc,
		FileEdit:       fileEditSvc,
		PubSubClient:   db,
		ResultsService: resultsSvc,
		LocalStore:     localStore,
	})
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	err = svc.Start(ctx)
	require.NoError(t, err)
	defer svc.Stop()

	// Subscribe to results channel to verify first execution
	resultsChannel := constants.ResultsChannel(cfg.OperatorID, cfg.OperatorSessionId)
	msgChan := testutil.SubscribeToChannel(t, testutil.GetTestOperatorDirectURL(), resultsChannel)

	// 1. Send first transaction with a unique nonce
	nonce := "unique-nonce-123"
	commandChannel := constants.CmdChannel(cfg.OperatorID, cfg.OperatorSessionId)
	cmdReq := testutil.MustMarshalProtobufCommandRequested(t, "echo first", "exec-1", "First execution", "", 0)
	envBytes := testutil.MustMarshalUniversalEnvelopeWithNonce(t, "exec-1", constants.Event.Operator.Command.Requested, cmdReq, "", cfg.OperatorID, "case-1", "", cfg.OperatorSessionId, nonce)

	testutil.PublishTestMessage(t, testutil.GetTestOperatorDirectURL(), commandChannel, string(envBytes))

	// Wait for execution result of the first command
	firstResult := testutil.WaitForMessage(t, msgChan, 3*time.Second)
	assert.NotNil(t, firstResult, "First execution should succeed")
	assert.Contains(t, string(firstResult), constants.Event.Operator.Command.Completed)

	// 2. Replay the EXACT SAME transaction
	testutil.PublishTestMessage(t, testutil.GetTestOperatorDirectURL(), commandChannel, string(envBytes))

	// 3. Verify that NO result is published for the replayed command (it should be rejected at the dispatcher)
	select {
	case unexpected := <-msgChan:
		t.Fatalf("Replayed transaction was NOT rejected, received: %s", unexpected)
	case <-time.After(2 * time.Second):
		// Success: no message received within timeout means it was likely rejected
	}
	
	// 4. Verify that the nonce is actually in the KV store
	_, found := localStore.KVGet("g8e:nonce:" + nonce)
	assert.True(t, found, "Nonce should be recorded in KV store")
}
