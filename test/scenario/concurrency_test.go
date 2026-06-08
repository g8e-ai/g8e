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

//go:build e2e

package scenario

import (
	"context"
	"strings"
	"sync"
	"testing"
)

// TestConcurrencyReplayDetection tests actual replay detection with concurrent submissions.
// It submits the same valid envelope twice concurrently using goroutines and asserts that
// exactly one succeeds and one rejects with TX_REPLAY.
func TestConcurrencyReplayDetection(t *testing.T) {
	// Setup test infrastructure
	ctx := setupTestContext(t)

	// Fetch current state root to bind envelopes
	stateRoot, err := ctx.Client.StateRoot(context.Background())
	if err != nil {
		t.Fatalf("failed to fetch state root: %v", err)
	}
	if stateRoot == "" {
		t.Fatal("gateway returned empty state root")
	}

	// Create a valid envelope with a fixed nonce for replay testing
	intentBytes, err := New().
		WithCommand("echo hello").
		WithOperatorSessionID(ctx.OperatorSessionID).
		WithStateRoot(stateRoot).
		WithNonce("nonce-concurrency-test-123").
		WithL2(ctx.PrivKey, true).
		Build()
	if err != nil {
		t.Fatalf("failed to build envelope: %v", err)
	}

	// Submit the same envelope twice concurrently
	var wg sync.WaitGroup
	results := make(chan Result, 2)

	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			result := submitViaHTTP(t, ctx.Client, intentBytes, ctx.OperatorSessionID, ctx.CLISessionID)
			results <- result
		}()
	}

	// Wait for both submissions to complete
	wg.Wait()
	close(results)

	// Collect results
	var result1, result2 Result
	for result := range results {
		if result1.Error == nil && result1.Receipt == nil {
			result1 = result
		} else {
			result2 = result
		}
	}

	// Assert that exactly one succeeded and one failed
	successCount := 0
	rejectCount := 0

	if result1.Error == nil && result1.Receipt != nil {
		successCount++
	} else {
		rejectCount++
	}

	if result2.Error == nil && result2.Receipt != nil {
		successCount++
	} else {
		rejectCount++
	}

	if successCount != 1 {
		t.Errorf("expected exactly 1 success, got %d", successCount)
	}
	if rejectCount != 1 {
		t.Errorf("expected exactly 1 rejection, got %d", rejectCount)
	}

	// Assert that the rejection was due to replay
	var rejectedResult Result
	if result1.Error != nil {
		rejectedResult = result1
	} else {
		rejectedResult = result2
	}

	if rejectedResult.Error == nil {
		t.Error("expected one result to have an error")
	} else {
		errMsg := rejectedResult.Error.Error()
		if !strings.Contains(errMsg, "replay") && !strings.Contains(errMsg, "REPLAY") {
			t.Errorf("expected rejection reason to contain 'replay', got %q", errMsg)
		}
	}
}
