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

package pubsub

import (
	"context"
	"encoding/json"
	"log/slog"
	"testing"

	"github.com/g8e-ai/g8e/internal/services/scrubbing"
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
