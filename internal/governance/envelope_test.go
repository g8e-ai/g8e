// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package governance

import (
	"errors"
	"testing"

	"time"

	"google.golang.org/protobuf/types/known/structpb"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	commonv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/common/v1"
)

func TestGovernanceEnvelope_GenerateMessageID(t *testing.T) {
	expiresAt := timestamppb.New(time.Now().Add(5 * time.Minute))

	env := &GovernanceEnvelope{
		ActionType:      "EXECUTE_BASH",
		TargetResource:  "localhost",
		Payload:         []byte("echo 'hello world'"),
		ExpiresAt:       expiresAt,
		Nonce:           "nonce-123",
		StateMerkleRoot: "root-abc",
	}

	id1, err := GenerateMessageID(env)
	if err != nil {
		t.Fatalf("Failed to generate MessageID: %v", err)
	}

	if id1 == "" {
		t.Fatal("MessageID should not be empty")
	}

	// Verify determinism
	id2, _ := GenerateMessageID(env)
	if id1 != id2 {
		t.Errorf("MessageID generation not deterministic: %s != %s", id1, id2)
	}

	// Verify sensitivity to ActionType
	env.ActionType = "FILE_EDIT"
	id3, _ := GenerateMessageID(env)
	if id1 == id3 {
		t.Error("MessageID should change when ActionType changes")
	}

	// Reset and verify sensitivity to Payload
	env.ActionType = "EXECUTE_BASH"
	env.Payload = []byte("echo 'hello world!'")
	id4, _ := GenerateMessageID(env)
	if id1 == id4 {
		t.Error("MessageID should change when Payload changes")
	}

	// Verify sensitivity to StateMerkleRoot
	env.Payload = []byte("echo 'hello world'")
	env.StateMerkleRoot = "root-def"
	id5, _ := GenerateMessageID(env)
	if id1 == id5 {
		t.Error("MessageID should change when StateMerkleRoot changes")
	}

	// Verify sensitivity to Nonce
	env.StateMerkleRoot = "root-abc"
	env.Nonce = "nonce-456"
	id6, _ := GenerateMessageID(env)
	if id1 == id6 {
		t.Error("MessageID should change when Nonce changes")
	}

	// Verify sensitivity to ExpiresAt
	env.Nonce = "nonce-123"
	env.ExpiresAt = timestamppb.New(time.Now().Add(10 * time.Minute))
	id7, _ := GenerateMessageID(env)
	if id1 == id7 {
		t.Error("MessageID should change when ExpiresAt changes")
	}

	// Verify sensitivity to RequestorUserId
	env.ExpiresAt = expiresAt
	env.RequestorUserId = "user-123"
	id8, _ := GenerateMessageID(env)
	if id1 == id8 {
		t.Error("MessageID should change when RequestorUserId changes")
	}

	// Verify sensitivity to ActingAppId
	env.RequestorUserId = ""
	env.ActingAppId = "app-456"
	id9, _ := GenerateMessageID(env)
	if id1 == id9 {
		t.Error("MessageID should change when ActingAppId changes")
	}
}

func TestGovernanceEnvelope_GenerateMessageID_WithIntentData(t *testing.T) {
	intentData, _ := structpb.NewStruct(map[string]interface{}{
		"command": "echo test",
		"cwd":     "/home",
	})

	env := &GovernanceEnvelope{
		ActionType:      "EXECUTE_BASH",
		TargetResource:  "localhost",
		Payload:         []byte("echo test"),
		ExpiresAt:       timestamppb.New(time.Now().Add(5 * time.Minute)),
		Nonce:           "nonce-123",
		StateMerkleRoot: "root-abc",
		IntentData:      intentData,
	}

	id1, err := GenerateMessageID(env)
	if err != nil {
		t.Fatalf("Failed to generate MessageID with intent_data: %v", err)
	}

	// Verify sensitivity to IntentData
	intentData2, _ := structpb.NewStruct(map[string]interface{}{
		"command": "echo test2",
		"cwd":     "/home",
	})
	env.IntentData = intentData2
	id2, _ := GenerateMessageID(env)
	if id1 == id2 {
		t.Error("MessageID should change when IntentData changes")
	}
}

func TestGovernanceEnvelope_GenerateMessageID_DeterministicCanonicalization(t *testing.T) {
	// Create two envelopes that are logically identical but constructed differently
	intent1, _ := structpb.NewStruct(map[string]interface{}{
		"a": "1",
		"b": "2",
		"c": map[string]interface{}{
			"x": true,
			"y": false,
		},
	})

	intent2, _ := structpb.NewStruct(map[string]interface{}{
		"c": map[string]interface{}{
			"y": false,
			"x": true,
		},
		"b": "2",
		"a": "1",
	})

	expiresAt := time.Date(2026, 5, 12, 12, 0, 0, 0, time.UTC)
	expiresAtPB := timestamppb.New(expiresAt)

	env1 := &GovernanceEnvelope{
		ActionType:     "TEST",
		TargetResource: "res",
		Payload:        []byte("payload"),
		ExpiresAt:      expiresAtPB,
		Nonce:          "nonce",
		IntentData:     intent1,
	}

	env2 := &GovernanceEnvelope{
		ActionType:     "TEST",
		TargetResource: "res",
		Payload:        []byte("payload"),
		ExpiresAt:      expiresAtPB,
		Nonce:          "nonce",
		IntentData:     intent2,
	}

	id1, err := GenerateMessageID(env1)
	if err != nil {
		t.Fatalf("id1 failed: %v", err)
	}

	id2, err := GenerateMessageID(env2)
	if err != nil {
		t.Fatalf("id2 failed: %v", err)
	}

	if id1 != id2 {
		t.Errorf("Determinism failed:\nid1: %s\nid2: %s", id1, id2)
	}

	// Verify that setting irrelevant fields doesn't change the hash
	env1.Id = "some-id"
	env1.Governance = &commonv1.GovernanceMetadata{L1: &commonv1.L1Metadata{Validated: true}}

	id3, err := GenerateMessageID(env1)
	if err != nil {
		t.Fatalf("id3 failed: %v", err)
	}

	if id1 != id3 {
		t.Errorf("Irrelevant fields changed the hash:\nid1: %s\nid3: %s", id1, id3)
	}
}

func TestGovernanceEnvelope_GenerateMessageID_NilEnvelope(t *testing.T) {
	_, err := GenerateMessageID(nil)
	if err == nil {
		t.Fatal("GenerateMessageID should return error for nil envelope")
	}
	if !errors.Is(err, constants.ErrTxInvalidEnvelope) {
		t.Errorf("GenerateMessageID should return ErrTxInvalidEnvelope for nil envelope, got: %v", err)
	}
}

func TestGovernanceEnvelope_GenerateMessageID_IDHashMismatch(t *testing.T) {
	expiresAt := timestamppb.New(time.Now().Add(5 * time.Minute))

	env := &GovernanceEnvelope{
		Id:              "wrong-id",
		ActionType:      "EXECUTE_BASH",
		TargetResource:  "localhost",
		Payload:         []byte("echo 'hello world'"),
		ExpiresAt:       expiresAt,
		Nonce:           "nonce-123",
		StateMerkleRoot: "root-abc",
	}

	computedHash, err := GenerateMessageID(env)
	if err != nil {
		t.Fatalf("Failed to generate MessageID: %v", err)
	}

	if env.Id == computedHash {
		t.Error("Pre-set Id should not match computed hash when wrong")
	}

	env.Id = computedHash
	computedHash2, _ := GenerateMessageID(env)
	if computedHash != computedHash2 {
		t.Error("Id should match computed hash when set correctly")
	}
}

func TestGenerateMessageID_L3ProofNotInHash(t *testing.T) {
	expiresAt := timestamppb.New(time.Now().Add(5 * time.Minute))

	base := &GovernanceEnvelope{
		ActionType:      "EXECUTE_BASH",
		TargetResource:  "localhost",
		Payload:         []byte("echo test"),
		ExpiresAt:       expiresAt,
		Nonce:           "nonce-123",
		StateMerkleRoot: "root-abc",
	}

	idNoL3, err := GenerateMessageID(base)
	if err != nil {
		t.Fatalf("Failed to generate base hash: %v", err)
	}

	// Adding an L3 proof must NOT change the hash — L2 signs before L3 exists.
	// Tamper-evidence for L3 is provided by verifyL3Posture at verification time,
	// not by the transaction hash.
	withProof := &GovernanceEnvelope{
		ActionType:      "EXECUTE_BASH",
		TargetResource:  "localhost",
		Payload:         []byte("echo test"),
		ExpiresAt:       expiresAt,
		Nonce:           "nonce-123",
		StateMerkleRoot: "root-abc",
		Governance: &commonv1.GovernanceMetadata{
			L3: &commonv1.L3Metadata{
				Proof: &commonv1.L3Proof{
					CliSignature:        "sig-abc",
					MtlsCertFingerprint: "fp-xyz",
				},
			},
		},
	}
	idWithProof, err := GenerateMessageID(withProof)
	if err != nil {
		t.Fatalf("Failed to generate hash with proof: %v", err)
	}
	if idNoL3 != idWithProof {
		t.Error("L3 proof must NOT affect the transaction hash (L2 signs before L3)")
	}

	// Changing the proof identity must NOT change the hash
	withDiffProof := &GovernanceEnvelope{
		ActionType:      "EXECUTE_BASH",
		TargetResource:  "localhost",
		Payload:         []byte("echo test"),
		ExpiresAt:       expiresAt,
		Nonce:           "nonce-123",
		StateMerkleRoot: "root-abc",
		Governance: &commonv1.GovernanceMetadata{
			L3: &commonv1.L3Metadata{
				Proof: &commonv1.L3Proof{
					CliSignature:        "sig-different",
					MtlsCertFingerprint: "fp-xyz",
				},
			},
		},
	}
	idDiffProof, err := GenerateMessageID(withDiffProof)
	if err != nil {
		t.Fatalf("Failed to generate hash with different proof: %v", err)
	}
	if idWithProof != idDiffProof {
		t.Error("Changing L3 proof identity must NOT change the hash")
	}

	// L1-only Governance (no L3) must not change the hash vs no Governance at all
	withL1Only := &GovernanceEnvelope{
		ActionType:      "EXECUTE_BASH",
		TargetResource:  "localhost",
		Payload:         []byte("echo test"),
		ExpiresAt:       expiresAt,
		Nonce:           "nonce-123",
		StateMerkleRoot: "root-abc",
		Governance: &commonv1.GovernanceMetadata{
			L1: &commonv1.L1Metadata{Validated: true},
		},
	}
	idL1Only, err := GenerateMessageID(withL1Only)
	if err != nil {
		t.Fatalf("Failed to generate hash with L1 only: %v", err)
	}
	if idNoL3 != idL1Only {
		t.Error("L1-only Governance (no L3) must not change the hash vs no Governance")
	}
}
