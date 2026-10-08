// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.

package operatorcmd

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/v2/internal/constants"
)

func TestDeploymentStateRejectsIncompleteOrUnknownProgress(t *testing.T) {
	for _, data := range []string{
		`{`, `{}`, `{"phase":"unknown","updated_at":"2026-10-08T00:00:00Z"}`,
		`{"phase":"pending_approval","updated_at":"2026-10-08T00:00:00Z"}`,
		`{"phase":"ready","updated_at":"2026-10-08T00:00:00Z"}`,
		`{"phase":"failed","updated_at":"2026-10-08T00:00:00Z"}`,
		`{"phase":"ready","operator_session_id":"session"}`,
	} {
		t.Run(data, func(t *testing.T) {
			_, err := decodeOperatorDeploymentState([]byte(data))
			require.ErrorIs(t, err, constants.ErrOperatorDeployFailed)
		})
	}
	state, err := decodeOperatorDeploymentState([]byte("null"))
	require.NoError(t, err)
	require.Nil(t, state)
}
