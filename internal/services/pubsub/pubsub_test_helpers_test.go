// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package pubsub

import (
	"context"
	"encoding/json"
	"log/slog"
	"testing"

	"github.com/g8e-ai/g8e/v2/internal/services/scrubbing"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
)

// mustMarshalJSON marshals v to json.RawMessage, fatally failing the test on error.
func mustMarshalJSON(t *testing.T, v interface{}) json.RawMessage {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("mustMarshalJSON: %v", err)
	}
	return json.RawMessage(b)
}

// mustMarshalProto marshals a protobuf message to bytes, fatally failing the test on error.
func mustMarshalProto(t *testing.T, msg proto.Message) []byte {
	t.Helper()
	data, err := proto.Marshal(msg)
	if err != nil {
		t.Fatalf("failed to marshal proto: %v", err)
	}
	return data
}

// mustNewScrubbingSvc creates a scrubbing service for tests, fatally failing on error.
func mustNewScrubbingSvc(t *testing.T, logger *slog.Logger) *scrubbing.ScrubbingService {
	t.Helper()
	svc, err := scrubbing.NewScrubbingService(context.Background(), scrubbing.DefaultConfig(), logger, nil)
	require.NoError(t, err)
	return svc
}
