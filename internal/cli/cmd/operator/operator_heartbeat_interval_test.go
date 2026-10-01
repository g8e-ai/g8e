// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package operatorcmd

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/v2/internal/constants"
)

func TestValidateHeartbeatInterval(t *testing.T) {
	maxSeconds := int(constants.OperatorHeartbeatStaleAfter.Seconds()) / 2

	for _, seconds := range []int{0, 1, 15, maxSeconds} {
		assert.NoError(t, validateHeartbeatInterval(seconds), "%d seconds must be accepted", seconds)
	}

	for _, seconds := range []int{-1, maxSeconds + 1, 60, 300} {
		err := validateHeartbeatInterval(seconds)
		require.Error(t, err, "%d seconds must be rejected", seconds)
		assert.ErrorIs(t, err, constants.ErrOperatorHeartbeatIntervalInvalid)
	}
}

func TestOperatorStartCmd_RejectsHeartbeatIntervalPastStaleWindow(t *testing.T) {
	cmd := operatorStartCmd()
	require.NoError(t, cmd.Flags().Set("heartbeat-interval", "60"))

	err := cmd.RunE(cmd, nil)
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrOperatorHeartbeatIntervalInvalid)
}
