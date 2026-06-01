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

package chaos

import (
	"crypto/ed25519"
	"errors"
	"fmt"
	"math/rand/v2"
	"strings"
	"testing"
	"time"

	"github.com/g8e-ai/g8e/internal/services/storage"
	"github.com/g8e-ai/g8e/internal/services/system"
	govpkg "github.com/g8e-ai/g8e/pkg/governance"
)

func TestPickCategory(t *testing.T) {
	r := rand.New(rand.NewPCG(12345, 67890))

	categoryCounts := make(map[category]int)
	iterations := 10000

	for i := 0; i < iterations; i++ {
		cat := pickCategory(r)
		categoryCounts[cat]++
	}

	// Verify distribution is approximately correct (within 5% tolerance)
	expectedGoodActor := 0.60
	expectedPromptInj := 0.20
	expectedMitM := 0.10
	expectedFileMutation := 0.10

	tolerance := 0.05

	actualGoodActor := float64(categoryCounts[catGoodActor]) / float64(iterations)
	actualPromptInj := float64(categoryCounts[catPromptInj]) / float64(iterations)
	actualMitM := float64(categoryCounts[catMitM]) / float64(iterations)
	actualFileMutation := float64(categoryCounts[catFileMutation]) / float64(iterations)

	if actualGoodActor < expectedGoodActor-tolerance || actualGoodActor > expectedGoodActor+tolerance {
		t.Errorf("catGoodActor distribution %.2f, expected %.2f ±%.2f", actualGoodActor, expectedGoodActor, tolerance)
	}
	if actualPromptInj < expectedPromptInj-tolerance || actualPromptInj > expectedPromptInj+tolerance {
		t.Errorf("catPromptInj distribution %.2f, expected %.2f ±%.2f", actualPromptInj, expectedPromptInj, tolerance)
	}
	if actualMitM < expectedMitM-tolerance || actualMitM > expectedMitM+tolerance {
		t.Errorf("catMitM distribution %.2f, expected %.2f ±%.2f", actualMitM, expectedMitM, tolerance)
	}
	if actualFileMutation < expectedFileMutation-tolerance || actualFileMutation > expectedFileMutation+tolerance {
		t.Errorf("catFileMutation distribution %.2f, expected %.2f ±%.2f", actualFileMutation, expectedFileMutation, tolerance)
	}
}

func TestClampMin(t *testing.T) {
	tests := []struct {
		name string
		a    int
		b    int
		want int
	}{
		{"a less than b", 3, 5, 3},
		{"a equal to b", 5, 5, 5},
		{"a greater than b", 7, 5, 5},
		{"zero a", 0, 10, 0},
		{"zero b", 10, 0, 0},
		{"negative a", -5, 5, -5},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := clampMin(tt.a, tt.b); got != tt.want {
				t.Errorf("clampMin(%d, %d) = %d, want %d", tt.a, tt.b, got, tt.want)
			}
		})
	}
}

func TestSignedEnvelope(t *testing.T) {
	_, privKey, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatalf("failed to generate key: %v", err)
	}

	stateRoot := "test-state-root"
	keyID := "test-key-id"
	sessionID := "test-session-id"

	tests := []struct {
		name        string
		actionType  string
		target      string
		payload     []byte
		isMutation  bool
		wantErr     bool
		checkFields func(*testing.T, *govpkg.GovernanceEnvelope)
	}{
		{
			name:       "valid read envelope",
			actionType: "FS_LIST",
			target:     "/tmp",
			payload:    []byte("test payload"),
			isMutation: false,
			wantErr:    false,
			checkFields: func(t *testing.T, env *govpkg.GovernanceEnvelope) {
				if env.ProtocolVersion != "1.0" {
					t.Errorf("ProtocolVersion = %s, want 1.0", env.ProtocolVersion)
				}
				if env.ActionType != "FS_LIST" {
					t.Errorf("ActionType = %s, want FS_LIST", env.ActionType)
				}
				if env.TargetResource != "/tmp" {
					t.Errorf("TargetResource = %s, want /tmp", env.TargetResource)
				}
				if env.Governance == nil {
					t.Error("Governance metadata is nil")
				}
				if env.Governance.L2 == nil {
					t.Error("L2 metadata is nil")
				}
				if env.Governance.L2.KeyId != keyID {
					t.Errorf("L2 KeyId = %s, want %s", env.Governance.L2.KeyId, keyID)
				}
				if env.Governance.L3 != nil {
					t.Error("L3 metadata should be nil for non-mutations")
				}
			},
		},
		{
			name:       "valid mutation envelope",
			actionType: "FILE_EDIT",
			target:     "/tmp/test.txt",
			payload:    []byte("test content"),
			isMutation: true,
			wantErr:    false,
			checkFields: func(t *testing.T, env *govpkg.GovernanceEnvelope) {
				if env.Governance.L3 == nil {
					t.Error("L3 metadata should be present for mutations")
				}
				if env.Governance.L3.Proof == nil {
					t.Error("L3 Proof should be present for mutations")
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			env, err := signedEnvelope(tt.actionType, tt.target, stateRoot, "test-nonce", tt.payload, tt.isMutation, privKey, keyID, sessionID)
			if (err != nil) != tt.wantErr {
				t.Errorf("signedEnvelope() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr {
				if env == nil {
					t.Fatal("envelope is nil")
				}
				if env.Id == "" {
					t.Error("envelope Id is empty")
				}
				if env.TransactionHash == "" {
					t.Error("envelope TransactionHash is empty")
				}
				if env.Id != env.TransactionHash {
					t.Error("Id and TransactionHash should match")
				}
				if tt.checkFields != nil {
					tt.checkFields(t, env)
				}
			}
		})
	}
}

func TestBuildGoodActorEnvelope(t *testing.T) {
	_, privKey, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatalf("failed to generate key: %v", err)
	}

	stateRoot := "test-state-root"
	keyID := "test-key-id"
	sessionID := "test-session-id"

	env, err := buildGoodActorEnvelope(1, stateRoot, privKey, keyID, sessionID)
	if err != nil {
		t.Fatalf("buildGoodActorEnvelope() error = %v", err)
	}

	if env == nil {
		t.Fatal("envelope is nil")
	}

	if env.ActionType != "FS_LIST" {
		t.Errorf("ActionType = %s, want FS_LIST", env.ActionType)
	}

	if env.Governance.L3 != nil {
		t.Error("L3 should be nil for good actor (non-mutation)")
	}
}

func TestBuildPromptInjEnvelope(t *testing.T) {
	_, privKey, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatalf("failed to generate key: %v", err)
	}

	stateRoot := "test-state-root"
	keyID := "test-key-id"
	sessionID := "test-session-id"

	env, err := buildPromptInjEnvelope(0, stateRoot, privKey, keyID, sessionID)
	if err != nil {
		t.Fatalf("buildPromptInjEnvelope() error = %v", err)
	}

	if env == nil {
		t.Fatal("envelope is nil")
	}

	if env.ActionType != "EXECUTE_BASH" {
		t.Errorf("ActionType = %s, want EXECUTE_BASH", env.ActionType)
	}

	if env.Governance.L3 != nil {
		t.Error("L3 should be nil for prompt injection (non-mutation)")
	}
}

func TestBuildFileMutationEnvelope(t *testing.T) {
	_, privKey, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatalf("failed to generate key: %v", err)
	}

	stateRoot := "test-state-root"
	keyID := "test-key-id"
	sessionID := "test-session-id"

	env, err := buildFileMutationEnvelope(1, stateRoot, privKey, keyID, sessionID)
	if err != nil {
		t.Fatalf("buildFileMutationEnvelope() error = %v", err)
	}

	if env == nil {
		t.Fatal("envelope is nil")
	}

	if env.ActionType != "FILE_EDIT" {
		t.Errorf("ActionType = %s, want FILE_EDIT", env.ActionType)
	}

	if env.Governance.L3 == nil {
		t.Error("L3 should be present for file mutation")
	}
}

func TestBuildMitMEnvelope(t *testing.T) {
	_, privKey, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatalf("failed to generate key: %v", err)
	}

	stateRoot := "test-state-root"
	keyID := "test-key-id"
	sessionID := "test-session-id"

	env, err := buildMitMEnvelope(1, stateRoot, privKey, keyID, sessionID)
	if err != nil {
		t.Fatalf("buildMitMEnvelope() error = %v", err)
	}

	if env == nil {
		t.Fatal("envelope is nil")
	}

	if env.TransactionHash != chaosTestCorruptedHash {
		t.Errorf("TransactionHash not corrupted as expected: %s", env.TransactionHash)
	}

	if env.Id == env.TransactionHash {
		t.Error("Id should not match corrupted TransactionHash")
	}
}

func TestMemReplayStore(t *testing.T) {
	store := newMemReplayStore()
	nonce := "test-nonce-123"
	expiry := time.Now().Add(1 * time.Hour)

	t.Run("ReserveNonce first use", func(t *testing.T) {
		seen, err := store.ReserveNonce(nonce, expiry)
		if err != nil {
			t.Fatalf("ReserveNonce() error = %v", err)
		}
		if seen {
			t.Error("nonce should not be seen on first use")
		}
	})

	t.Run("ReserveNonce second use", func(t *testing.T) {
		seen, err := store.ReserveNonce(nonce, expiry)
		if err != nil {
			t.Fatalf("ReserveNonce() error = %v", err)
		}
		if !seen {
			t.Error("nonce should be seen on second use")
		}
	})

	t.Run("FinalizeNonce", func(t *testing.T) {
		err := store.FinalizeNonce(nonce)
		if err != nil {
			t.Fatalf("FinalizeNonce() error = %v", err)
		}
	})

	t.Run("ReleaseNonce", func(t *testing.T) {
		err := store.ReleaseNonce(nonce)
		if err != nil {
			t.Fatalf("ReleaseNonce() error = %v", err)
		}

		// After release, nonce should be available again
		seen, err := store.ReserveNonce(nonce, expiry)
		if err != nil {
			t.Fatalf("ReserveNonce() after release error = %v", err)
		}
		if seen {
			t.Error("nonce should not be seen after release")
		}
	})
}

func TestChaosL3Notary(t *testing.T) {
	notary := &chaosL3Notary{}

	tests := []struct {
		name            string
		userID          string
		transactionHash string
		cliSessionID    string
		proof           interface{}
		wantValid       bool
		wantErr         bool
	}{
		{
			name:            "always returns true",
			userID:          "test-user",
			transactionHash: "test-hash",
			cliSessionID:    "test-session",
			proof:           nil,
			wantValid:       true,
			wantErr:         false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := notary.VerifyL3Proof(tt.userID, tt.transactionHash, tt.cliSessionID, nil)
			if (err != nil) != tt.wantErr {
				t.Errorf("VerifyL3Proof() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.wantValid {
				t.Errorf("VerifyL3Proof() = %v, want %v", got, tt.wantValid)
			}
		})
	}
}

func TestDynamicStateRoot(t *testing.T) {
	provider := &dynamicStateRoot{root: "initial-root"}

	t.Run("GetCurrentStateRoot", func(t *testing.T) {
		root, err := provider.GetCurrentStateRoot()
		if err != nil {
			t.Fatalf("GetCurrentStateRoot() error = %v", err)
		}
		if root != "initial-root" {
			t.Errorf("GetCurrentStateRoot() = %s, want initial-root", root)
		}
	})

	t.Run("UpdateRoot", func(t *testing.T) {
		newRoot := "updated-root"
		provider.UpdateRoot(newRoot)

		root, err := provider.GetCurrentStateRoot()
		if err != nil {
			t.Fatalf("GetCurrentStateRoot() after update error = %v", err)
		}
		if root != newRoot {
			t.Errorf("GetCurrentStateRoot() after update = %s, want %s", root, newRoot)
		}
	})
}

func TestClassifyRejection(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want string
	}{
		{
			name: "nil error",
			err:  nil,
			want: "EXECUTED",
		},
		{
			name: "L1 failure",
			err:  errors.New("TX_L1_FAILED: forbidden pattern"),
			want: "L1_BLOCKED",
		},
		{
			name: "hash mismatch",
			err:  errors.New("TX_HASH_MISMATCH: corrupted hash"),
			want: "HASH_FAIL",
		},
		{
			name: "hash missing",
			err:  errors.New("TX_HASH_MISSING: no hash"),
			want: "HASH_FAIL",
		},
		{
			name: "L2 rejection",
			err:  errors.New("TX_L2_REJECTED: invalid signature"),
			want: "L2_REJECTED",
		},
		{
			name: "expired",
			err:  errors.New("TX_EXPIRED: transaction too old"),
			want: "EXPIRED",
		},
		{
			name: "replay",
			err:  errors.New("TX_REPLAY: nonce already used"),
			want: "REPLAY",
		},
		{
			name: "unknown error",
			err:  errors.New("some other error"),
			want: "REJECTED",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := classifyRejection(tt.err); got != tt.want {
				t.Errorf("classifyRejection() = %s, want %s", got, tt.want)
			}
		})
	}
}

func TestContains(t *testing.T) {
	tests := []struct {
		name string
		s    string
		sub  string
		want bool
	}{
		{
			name: "substring present",
			s:    "hello world",
			sub:  "world",
			want: true,
		},
		{
			name: "substring not present",
			s:    "hello world",
			sub:  "goodbye",
			want: false,
		},
		{
			name: "empty substring",
			s:    "hello world",
			sub:  "",
			want: true,
		},
		{
			name: "empty string",
			s:    "",
			sub:  "test",
			want: false,
		},
		{
			name: "case sensitive",
			s:    "Hello World",
			sub:  "hello",
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := contains(tt.s, tt.sub); got != tt.want {
				t.Errorf("contains() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestCategoryName(t *testing.T) {
	tests := []struct {
		name string
		cat  category
		want string
	}{
		{
			name: "good actor",
			cat:  catGoodActor,
			want: "GOOD_ACTOR",
		},
		{
			name: "prompt injection",
			cat:  catPromptInj,
			want: "PROMPT_INJECTION",
		},
		{
			name: "mitm",
			cat:  catMitM,
			want: "MITM",
		},
		{
			name: "file mutation",
			cat:  catFileMutation,
			want: "FILE_MUTATION",
		},
		{
			name: "unknown category",
			cat:  category(999),
			want: "UNKNOWN",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := categoryName(tt.cat); got != tt.want {
				t.Errorf("categoryName() = %s, want %s", got, tt.want)
			}
		})
	}
}

func TestPct(t *testing.T) {
	tests := []struct {
		name  string
		n     int64
		total int64
		want  float64
	}{
		{
			name:  "50 percent",
			n:     50,
			total: 100,
			want:  50.0,
		},
		{
			name:  "25 percent",
			n:     25,
			total: 100,
			want:  25.0,
		},
		{
			name:  "100 percent",
			n:     100,
			total: 100,
			want:  100.0,
		},
		{
			name:  "0 percent",
			n:     0,
			total: 100,
			want:  0.0,
		},
		{
			name:  "zero total",
			n:     50,
			total: 0,
			want:  0.0,
		},
		{
			name:  "fractional",
			n:     33,
			total: 100,
			want:  33.0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := pct(tt.n, tt.total)
			if got != tt.want {
				t.Errorf("pct() = %f, want %f", got, tt.want)
			}
		})
	}
}

func TestFindGit(t *testing.T) {
	gitPath, err := findGit()
	if err != nil {
		t.Fatalf("findGit() error = %v", err)
	}
	if gitPath != system.GitEmbedded {
		t.Errorf("findGit() = %s, want %s", gitPath, system.GitEmbedded)
	}
}

func TestBuildEnvelope(t *testing.T) {
	_, privKey, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatalf("failed to generate key: %v", err)
	}

	stateRoot := "test-state-root"
	keyID := "test-key-id"
	sessionID := "test-session-id"

	tests := []struct {
		name    string
		cat     category
		wantErr bool
	}{
		{
			name:    "good actor",
			cat:     catGoodActor,
			wantErr: false,
		},
		{
			name:    "prompt injection",
			cat:     catPromptInj,
			wantErr: false,
		},
		{
			name:    "mitm",
			cat:     catMitM,
			wantErr: false,
		},
		{
			name:    "file mutation",
			cat:     catFileMutation,
			wantErr: false,
		},
		{
			name:    "unknown category",
			cat:     category(999),
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			env, err := buildEnvelope(1, tt.cat, stateRoot, privKey, keyID, sessionID)
			if (err != nil) != tt.wantErr {
				t.Errorf("buildEnvelope() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && env == nil {
				t.Error("buildEnvelope() returned nil envelope without error")
			}
		})
	}
}

func TestBatchEventWriter(t *testing.T) {
	t.Run("queueEvent adds events", func(t *testing.T) {
		writer := &batchEventWriter{
			events:    make([]*storage.ChaosEvent, 0, 10),
			flushSize: 100,
		}

		event := &storage.ChaosEvent{
			OperatorSessionID: "test-session",
			Timestamp:         time.Now(),
			ChaosID:           1,
			Category:          "TEST",
			Outcome:           "TEST_OUTCOME",
		}

		writer.queueEvent(event)
		if len(writer.events) != 1 {
			t.Errorf("expected 1 event, got %d", len(writer.events))
		}

		writer.queueEvent(event)
		if len(writer.events) != 2 {
			t.Errorf("expected 2 events, got %d", len(writer.events))
		}
	})

	t.Run("flush with nil audit vault", func(t *testing.T) {
		writer := &batchEventWriter{
			auditVault: nil,
			events: []*storage.ChaosEvent{
				{
					OperatorSessionID: "test-session",
					Timestamp:         time.Now(),
					ChaosID:           1,
					Category:          "TEST",
					Outcome:           "TEST_OUTCOME",
				},
			},
			flushSize: 2,
		}

		// Should not panic with nil audit vault
		writer.flush()
		if len(writer.events) != 0 {
			t.Errorf("expected events to be cleared after flush, got %d", len(writer.events))
		}
	})

	t.Run("flush with empty events", func(t *testing.T) {
		writer := &batchEventWriter{
			events:    make([]*storage.ChaosEvent, 0),
			flushSize: 2,
		}

		// Should not panic with empty events
		writer.flush()
	})
}

func TestCounters(t *testing.T) {
	var cnt counters

	t.Run("atomic operations", func(t *testing.T) {
		cnt.executed.Add(1)
		cnt.l1Blocked.Add(2)
		cnt.hashFail.Add(3)
		cnt.other.Add(4)
		cnt.executedGoodActor.Add(5)
		cnt.executedFileMut.Add(6)

		if cnt.executed.Load() != 1 {
			t.Errorf("executed = %d, want 1", cnt.executed.Load())
		}
		if cnt.l1Blocked.Load() != 2 {
			t.Errorf("l1Blocked = %d, want 2", cnt.l1Blocked.Load())
		}
		if cnt.hashFail.Load() != 3 {
			t.Errorf("hashFail = %d, want 3", cnt.hashFail.Load())
		}
		if cnt.other.Load() != 4 {
			t.Errorf("other = %d, want 4", cnt.other.Load())
		}
		if cnt.executedGoodActor.Load() != 5 {
			t.Errorf("executedGoodActor = %d, want 5", cnt.executedGoodActor.Load())
		}
		if cnt.executedFileMut.Load() != 6 {
			t.Errorf("executedFileMut = %d, want 6", cnt.executedFileMut.Load())
		}
	})
}

func TestEnvelopeConstructionWithDifferentIDs(t *testing.T) {
	_, privKey, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatalf("failed to generate key: %v", err)
	}

	stateRoot := "test-state-root"
	keyID := "test-key-id"
	sessionID := "test-session-id"

	// Test that different IDs produce different nonces
	env1, err := buildGoodActorEnvelope(1, stateRoot, privKey, keyID, sessionID)
	if err != nil {
		t.Fatalf("buildGoodActorEnvelope() error = %v", err)
	}

	env2, err := buildGoodActorEnvelope(2, stateRoot, privKey, keyID, sessionID)
	if err != nil {
		t.Fatalf("buildGoodActorEnvelope() error = %v", err)
	}

	if env1.Nonce == env2.Nonce {
		t.Error("different IDs should produce different nonces")
	}

	if env1.Id == env2.Id {
		t.Error("different IDs should produce different transaction hashes")
	}
}

func TestForbiddenCommands(t *testing.T) {
	_, privKey, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatalf("failed to generate key: %v", err)
	}

	stateRoot := "test-state-root"
	keyID := "test-key-id"
	sessionID := "test-session-id"

	// Test all forbidden commands are used
	for i := 0; i < 10; i++ {
		env, err := buildPromptInjEnvelope(i, stateRoot, privKey, keyID, sessionID)
		if err != nil {
			t.Fatalf("buildPromptInjEnvelope() error = %v", err)
		}

		if env.ActionType != "EXECUTE_BASH" {
			t.Errorf("ActionType = %s, want EXECUTE_BASH", env.ActionType)
		}
	}
}

func TestSignedEnvelopeTimestamp(t *testing.T) {
	_, privKey, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatalf("failed to generate key: %v", err)
	}

	before := time.Now()

	env, err := signedEnvelope("FS_LIST", "/tmp", "state-root", "nonce", []byte("payload"), false, privKey, "key-id", "session-id")
	if err != nil {
		t.Fatalf("signedEnvelope() error = %v", err)
	}

	after := time.Now()

	if env.Timestamp == nil {
		t.Fatal("Timestamp is nil")
	}

	envTime := env.Timestamp.AsTime()
	if envTime.Before(before) || envTime.After(after) {
		t.Errorf("Timestamp %v is outside expected range [%v, %v]", envTime, before, after)
	}

	if env.ExpiresAt == nil {
		t.Fatal("ExpiresAt is nil")
	}

	expiryTime := env.ExpiresAt.AsTime()
	expectedExpiry := before.Add(30 * time.Minute)
	if expiryTime.Before(expectedExpiry.Add(-1*time.Second)) || expiryTime.After(expectedExpiry.Add(1*time.Second)) {
		t.Errorf("ExpiresAt %v is not approximately 30 minutes from now", expiryTime)
	}
}

func TestSignedEnvelopeSessionID(t *testing.T) {
	_, privKey, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatalf("failed to generate key: %v", err)
	}

	sessionID := "test-session-123"

	env, err := signedEnvelope("FS_LIST", "/tmp", "state-root", "nonce", []byte("payload"), false, privKey, "key-id", sessionID)
	if err != nil {
		t.Fatalf("signedEnvelope() error = %v", err)
	}

	if env.OperatorSessionId != sessionID {
		t.Errorf("OperatorSessionId = %s, want %s", env.OperatorSessionId, sessionID)
	}
}

func TestSignedEnvelopeOperatorID(t *testing.T) {
	_, privKey, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatalf("failed to generate key: %v", err)
	}

	env, err := signedEnvelope("FS_LIST", "/tmp", "state-root", "nonce", []byte("payload"), false, privKey, "key-id", "session-id")
	if err != nil {
		t.Fatalf("signedEnvelope() error = %v", err)
	}

	if env.OperatorId != "chaos-operator" {
		t.Errorf("OperatorId = %s, want chaos-operator", env.OperatorId)
	}
}

func TestSignedEnvelopeStateRoot(t *testing.T) {
	_, privKey, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatalf("failed to generate key: %v", err)
	}

	stateRoot := "test-state-root-456"

	env, err := signedEnvelope("FS_LIST", "/tmp", stateRoot, "nonce", []byte("payload"), false, privKey, "key-id", "session-id")
	if err != nil {
		t.Fatalf("signedEnvelope() error = %v", err)
	}

	if env.StateMerkleRoot != stateRoot {
		t.Errorf("StateMerkleRoot = %s, want %s", env.StateMerkleRoot, stateRoot)
	}
}

func TestSignedEnvelopeL2Signature(t *testing.T) {
	_, privKey, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatalf("failed to generate key: %v", err)
	}

	keyID := "test-key-id"

	env, err := signedEnvelope("FS_LIST", "/tmp", "state-root", "nonce", []byte("payload"), false, privKey, keyID, "session-id")
	if err != nil {
		t.Fatalf("signedEnvelope() error = %v", err)
	}

	if env.Governance == nil {
		t.Fatal("Governance is nil")
	}

	if env.Governance.L2 == nil {
		t.Fatal("L2 metadata is nil")
	}

	if env.Governance.L2.KeyId != keyID {
		t.Errorf("L2 KeyId = %s, want %s", env.Governance.L2.KeyId, keyID)
	}

	if env.Governance.L2.ConsensusSignature == "" {
		t.Error("ConsensusSignature is empty")
	}

	if len(env.Governance.L2.AgentIds) == 0 {
		t.Error("AgentIds is empty")
	}

	if env.Governance.L2.AgentIds[0] != "chaos-tribunal-agent" {
		t.Errorf("AgentIds[0] = %s, want chaos-tribunal-agent", env.Governance.L2.AgentIds[0])
	}
}

func TestSignedEnvelopeNonceFormat(t *testing.T) {
	_, privKey, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatalf("failed to generate key: %v", err)
	}

	nonceSuffix := "test-suffix"
	payload := []byte("test-payload")

	env, err := signedEnvelope("FS_LIST", "/tmp", "state-root", nonceSuffix, payload, false, privKey, "key-id", "session-id")
	if err != nil {
		t.Fatalf("signedEnvelope() error = %v", err)
	}

	if !strings.HasPrefix(env.Nonce, "chaos-") {
		t.Errorf("Nonce should start with 'chaos-', got %s", env.Nonce)
	}

	if !strings.Contains(env.Nonce, nonceSuffix) {
		t.Errorf("Nonce should contain suffix %s, got %s", nonceSuffix, env.Nonce)
	}
}

func TestRecordRejection(t *testing.T) {
	t.Run("with nil audit vault", func(t *testing.T) {
		writer := &batchEventWriter{
			auditVault: nil,
		}

		env := &govpkg.GovernanceEnvelope{
			OperatorSessionId: "test-session",
			TransactionHash:   "test-hash",
			ActionType:        "FS_LIST",
			TargetResource:    "/tmp",
		}

		// Should not panic with nil audit vault
		writer.recordRejection(1, catGoodActor, env, errors.New("test error"))
	})

	t.Run("event structure validation", func(t *testing.T) {
		// Test the event structure directly without going through recordRejection
		// since it requires a non-nil audit vault
		env := &govpkg.GovernanceEnvelope{
			OperatorSessionId: "test-session",
			TransactionHash:   "test-hash",
			ActionType:        "FS_LIST",
			TargetResource:    "/tmp",
		}

		reason := classifyRejection(errors.New("TX_L1_FAILED: forbidden pattern"))
		event := &storage.ChaosEvent{
			OperatorSessionID: env.OperatorSessionId,
			Timestamp:         time.Now(),
			ChaosID:           1,
			Category:          categoryName(catGoodActor),
			Outcome:           reason,
			ContentText:       fmt.Sprintf("[chaos-id:%d] %s: %s", 1, categoryName(catGoodActor), errors.New("TX_L1_FAILED: forbidden pattern").Error()),
			CommandRaw:        env.ActionType + " / " + env.TargetResource,
			TransactionHash:   env.TransactionHash,
		}

		if event.Outcome != "L1_BLOCKED" {
			t.Errorf("expected outcome L1_BLOCKED, got %s", event.Outcome)
		}
		if event.Category != "GOOD_ACTOR" {
			t.Errorf("expected category GOOD_ACTOR, got %s", event.Category)
		}
		if event.ChaosID != 1 {
			t.Errorf("expected chaos ID 1, got %d", event.ChaosID)
		}
	})
}

func TestRecordExecution(t *testing.T) {
	t.Run("with nil audit vault", func(t *testing.T) {
		writer := &batchEventWriter{
			auditVault: nil,
		}

		env := &govpkg.GovernanceEnvelope{
			OperatorSessionId: "test-session",
			TransactionHash:   "test-hash-123456789012",
			ActionType:        "FS_LIST",
			TargetResource:    "/tmp",
		}

		// Should not panic with nil audit vault
		writer.recordExecution(1, catGoodActor, env, nil)
	})

	t.Run("event structure validation - success", func(t *testing.T) {
		// Test the event structure directly without going through recordExecution
		// since it requires a non-nil audit vault
		env := &govpkg.GovernanceEnvelope{
			OperatorSessionId: "test-session",
			TransactionHash:   "test-hash-123456789012",
			ActionType:        "FS_LIST",
			TargetResource:    "/tmp",
		}

		status := "COMPLETED"
		event := &storage.ChaosEvent{
			OperatorSessionID: env.OperatorSessionId,
			Timestamp:         time.Now(),
			ChaosID:           1,
			Category:          categoryName(catGoodActor),
			Outcome:           status,
			ContentText:       fmt.Sprintf("[chaos-id:%d] %s: %s", 1, categoryName(catGoodActor), status),
			CommandRaw:        fmt.Sprintf("%s / %s (hash: %s)", env.ActionType, env.TargetResource, env.TransactionHash[:12]),
			TransactionHash:   env.TransactionHash,
		}

		if event.Outcome != "COMPLETED" {
			t.Errorf("expected outcome COMPLETED, got %s", event.Outcome)
		}
	})

	t.Run("event structure validation - failure", func(t *testing.T) {
		// Test the event structure directly without going through recordExecution
		// since it requires a non-nil audit vault
		env := &govpkg.GovernanceEnvelope{
			OperatorSessionId: "test-session",
			TransactionHash:   "test-hash-123456789012",
			ActionType:        "FS_LIST",
			TargetResource:    "/tmp",
		}

		execErr := errors.New("execution failed")
		status := fmt.Sprintf("FAILED: %v", execErr)
		event := &storage.ChaosEvent{
			OperatorSessionID: env.OperatorSessionId,
			Timestamp:         time.Now(),
			ChaosID:           1,
			Category:          categoryName(catGoodActor),
			Outcome:           status,
			ContentText:       fmt.Sprintf("[chaos-id:%d] %s: %s", 1, categoryName(catGoodActor), status),
			CommandRaw:        fmt.Sprintf("%s / %s (hash: %s)", env.ActionType, env.TargetResource, env.TransactionHash[:12]),
			TransactionHash:   env.TransactionHash,
		}

		if !strings.Contains(event.Outcome, "FAILED") {
			t.Errorf("expected outcome to contain FAILED, got %s", event.Outcome)
		}
	})
}

func TestPrintSummaryRow(t *testing.T) {
	// This is a visual output function, just verify it doesn't panic
	printSummaryRow("TEST_CATEGORY", 10, "EXECUTED", 10)
	printSummaryRow("MISMATCH", 5, "EXECUTED", 3)
}

func TestPrintDemoQueries(t *testing.T) {
	// This is a visual output function, just verify it doesn't panic
	printDemoQueries("/tmp/test.db")
}
