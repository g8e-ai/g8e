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

package testutil

import (
	"testing"

	"google.golang.org/protobuf/proto"

	operatorv1 "github.com/g8e-ai/g8e/protocol/proto/g8e/operator/v1"
	"github.com/stretchr/testify/require"
)

func TestBuildFetchHistoryRequestedPayload(t *testing.T) {
	payload := BuildFetchHistoryRequestedPayload(t, "exec-006", "session-123", 50, 0)
	require.NotNil(t, payload)

	hist := &operatorv1.FetchHistoryRequested{}
	err := proto.Unmarshal(payload, hist)
	require.NoError(t, err)
	require.Equal(t, "exec-006", hist.ExecutionId)
	require.Equal(t, "session-123", hist.OperatorSessionId)
	require.Equal(t, int32(50), hist.Limit)
	require.Equal(t, int32(0), hist.Offset)
}

func TestBuildFetchFileHistoryRequestedPayload(t *testing.T) {
	payload := BuildFetchFileHistoryRequestedPayload(t, "exec-007", "/tmp/file.txt", 20, "session-456")
	require.NotNil(t, payload)

	fhist := &operatorv1.FetchFileHistoryRequested{}
	err := proto.Unmarshal(payload, fhist)
	require.NoError(t, err)
	require.Equal(t, "exec-007", fhist.ExecutionId)
	require.Equal(t, "/tmp/file.txt", fhist.FilePath)
	require.Equal(t, int32(20), fhist.Limit)
	require.Equal(t, "session-456", fhist.OperatorSessionId)
}

func TestBuildRestoreFileRequestedPayload(t *testing.T) {
	payload := BuildRestoreFileRequestedPayload(t, "exec-009", "/tmp/file.txt", "abc123", "session-789")
	require.NotNil(t, payload)

	restore := &operatorv1.RestoreFileRequested{}
	err := proto.Unmarshal(payload, restore)
	require.NoError(t, err)
	require.Equal(t, "exec-009", restore.ExecutionId)
	require.Equal(t, "/tmp/file.txt", restore.FilePath)
	require.Equal(t, "abc123", restore.CommitHash)
	require.Equal(t, "session-789", restore.OperatorSessionId)
}
