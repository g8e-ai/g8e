//go:build e2e

// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package e2e

import (
	"github.com/g8e-ai/g8e/v2/internal/constants"
	operatorv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/operator/v1"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestFleetSelection_RejectsMissingDuplicateAndMalformedSessions(t *testing.T) {
	const session = "11111111-1111-4111-8111-111111111111"
	for _, raw := range []string{"bad", session + ",", session + "," + session} {
		_, err := parseFleetSessions(raw)
		require.Error(t, err)
	}
	sessions, err := parseFleetSessions(" " + session + " ")
	require.NoError(t, err)
	require.Equal(t, []string{session}, sessions)
	selected := &operatorv1.OperatorDocument{OperatorSessionId: session, Status: string(constants.OperatorStatusStale)}
	unrelated := &operatorv1.OperatorDocument{OperatorSessionId: "other", Status: string(constants.OperatorStatusActive)}
	got, err := selectFleetOperators([]*operatorv1.OperatorDocument{unrelated, selected}, sessions)
	require.NoError(t, err)
	require.Equal(t, []*operatorv1.OperatorDocument{selected}, got, "selected stale workers must remain visible to assertions")
	_, err = selectFleetOperators([]*operatorv1.OperatorDocument{unrelated}, sessions)
	require.Error(t, err)
	_, err = selectFleetOperators([]*operatorv1.OperatorDocument{selected, selected}, sessions)
	require.Error(t, err)
	got, err = selectFleetOperators([]*operatorv1.OperatorDocument{unrelated, selected}, nil)
	require.NoError(t, err)
	require.Len(t, got, 2)
}
