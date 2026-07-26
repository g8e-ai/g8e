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
)

// BuildFetchHistoryRequestedPayload builds a FetchHistoryRequested payload bytes.
func BuildFetchHistoryRequestedPayload(t *testing.T, execID string, sessionID string, limit int32, offset int32) []byte {
	t.Helper()
	p := &operatorv1.FetchHistoryRequested{
		ExecutionId:       execID,
		OperatorSessionId: sessionID,
		Limit:             limit,
		Offset:            offset,
	}
	b, err := proto.Marshal(p)
	if err != nil {
		t.Fatalf("failed to marshal protobuf FetchHistoryRequested: %v", err)
	}
	return b
}

// BuildFetchFileHistoryRequestedPayload builds a FetchFileHistoryRequested payload bytes.
func BuildFetchFileHistoryRequestedPayload(t *testing.T, execID string, filePath string, limit int32, operatorSessionID string) []byte {
	t.Helper()
	p := &operatorv1.FetchFileHistoryRequested{
		ExecutionId:       execID,
		FilePath:          filePath,
		Limit:             limit,
		OperatorSessionId: operatorSessionID,
	}
	b, err := proto.Marshal(p)
	if err != nil {
		t.Fatalf("failed to marshal protobuf FetchFileHistoryRequested: %v", err)
	}
	return b
}

// BuildRestoreFileRequestedPayload builds a RestoreFileRequested payload bytes.
func BuildRestoreFileRequestedPayload(t *testing.T, execID string, filePath string, commitHash string, sessionID string) []byte {
	t.Helper()
	p := &operatorv1.RestoreFileRequested{
		ExecutionId:       execID,
		FilePath:          filePath,
		CommitHash:        commitHash,
		OperatorSessionId: sessionID,
	}
	b, err := proto.Marshal(p)
	if err != nil {
		t.Fatalf("failed to marshal protobuf RestoreFileRequested: %v", err)
	}
	return b
}
