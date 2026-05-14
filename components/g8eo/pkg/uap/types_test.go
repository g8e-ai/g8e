package uap

import (
	"testing"

	"time"

	"google.golang.org/protobuf/types/known/structpb"
	"google.golang.org/protobuf/types/known/timestamppb"
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
