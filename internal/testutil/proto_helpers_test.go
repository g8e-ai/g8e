// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

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
