package uap

import (
	"testing"
)

func TestUAPEnvelope_GenerateMessageID(t *testing.T) {
	env := &UAPEnvelope{
		Intent: Intent{
			ActionType:     "EXECUTE_BASH",
			TargetResource: "localhost",
		},
		Context: Context{
			DataFormat: "raw",
			DataBlob:   "echo 'hello world'",
		},
	}

	id1, err := env.GenerateMessageID()
	if err != nil {
		t.Fatalf("Failed to generate MessageID: %v", err)
	}

	if id1 == "" {
		t.Fatal("MessageID should not be empty")
	}

	// Verify determinism
	id2, _ := env.GenerateMessageID()
	if id1 != id2 {
		t.Errorf("MessageID generation not deterministic: %s != %s", id1, id2)
	}

	// Verify sensitivity to Intent
	env.Intent.TargetResource = "other-host"
	id3, _ := env.GenerateMessageID()
	if id1 == id3 {
		t.Error("MessageID should change when Intent changes")
	}

	// Reset and verify sensitivity to Context
	env.Intent.TargetResource = "localhost"
	env.Context.DataBlob = "echo 'hello world!'"
	id4, _ := env.GenerateMessageID()
	if id1 == id4 {
		t.Error("MessageID should change when Context changes")
	}
}
