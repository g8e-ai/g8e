// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package auth

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/v2/internal/constants"
)

func TestDecodeCLIBindResponse(t *testing.T) {
	got, err := DecodeCLIBindResponse([]byte(`{"success":true,"cli_session_id":"cli-2","user_id":"user-1","operator_id":"op-1","operator_session_id":"sess-1","bound":[{"operator_id":"op-1","operator_session_id":"sess-1"}]}`), []string{"sess-1"})
	require.NoError(t, err)
	assert.Equal(t, "cli-2", got.CLISessionID)
	assert.Equal(t, "sess-1", got.OperatorSessionID)

	_, err = DecodeCLIBindResponse([]byte(`{"success":true,"cli_session_id":"cli-2","user_id":"user-1","operator_id":"op-1","operator_session_id":"sess-1","bound":[]}`), []string{"sess-1"})
	assert.ErrorIs(t, err, constants.ErrCLIRefreshFailed)
}

func TestDecodeCLIUnbindResponse(t *testing.T) {
	got, err := DecodeCLIUnbindResponse([]byte(`{"success":true,"cli_session_id":"cli-3","user_id":"user-1"}`))
	require.NoError(t, err)
	assert.Equal(t, "cli-3", got.CLISessionID)

	_, err = DecodeCLIUnbindResponse([]byte(`{"success":false}`))
	assert.ErrorIs(t, err, constants.ErrCLIRefreshFailed)
}
