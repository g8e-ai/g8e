package uap

import (
	"testing"

	"time"

	"google.golang.org/protobuf/types/known/timestamppb"
)

func TestUAPEnvelope_GenerateMessageID(t *testing.T) {
	env := &UAPEnvelope{
		ActionType:     "EXECUTE_BASH",
		TargetResource: "localhost",
		Payload:        []byte("echo 'hello world'"),
		ExpiresAt:      timestamppb.New(time.Now().Add(5 * time.Minute)),
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

	// Verify sensitivity to Intent
	env.TargetResource = "other-host"
	id3, _ := GenerateMessageID(env)
	if id1 == id3 {
		t.Error("MessageID should change when Intent changes")
	}

	// Reset and verify sensitivity to Context
	env.TargetResource = "localhost"
	env.Payload = []byte("echo 'hello world!'")
	id4, _ := GenerateMessageID(env)
	if id1 == id4 {
		t.Error("MessageID should change when Context changes")
	}
}
