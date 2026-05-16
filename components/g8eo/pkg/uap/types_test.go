package uap

import (
	"testing"

	"time"

	"google.golang.org/protobuf/types/known/structpb"
	"google.golang.org/protobuf/types/known/timestamppb"

	commonv1 "github.com/g8e-ai/g8e/components/g8eo/internal/shared/proto/commonv1"
)

func TestUAPEnvelope_GenerateMessageID(t *testing.T) {
	expiresAt := timestamppb.New(time.Now().Add(5 * time.Minute))

	env := &UAPEnvelope{
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
}

func TestUAPEnvelope_GenerateMessageID_WithIntentData(t *testing.T) {
	intentData, _ := structpb.NewStruct(map[string]interface{}{
		"command": "echo test",
		"cwd":     "/home",
	})

	env := &UAPEnvelope{
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

func TestUAPEnvelope_GenerateMessageID_DeterministicCanonicalization(t *testing.T) {
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

	env1 := &UAPEnvelope{
		ActionType:     "TEST",
		TargetResource: "res",
		Payload:        []byte("payload"),
		ExpiresAt:      expiresAtPB,
		Nonce:          "nonce",
		IntentData:     intent1,
	}

	env2 := &UAPEnvelope{
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

func TestUAPEnvelope_GenerateMessageID_NilEnvelope(t *testing.T) {
	_, err := GenerateMessageID(nil)
	if err == nil {
		t.Error("GenerateMessageID should return error for nil envelope")
	}
}

func TestUAPEnvelope_GenerateMessageID_IDHashMismatch(t *testing.T) {
	expiresAt := timestamppb.New(time.Now().Add(5 * time.Minute))

	env := &UAPEnvelope{
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
