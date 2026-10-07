// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package operator

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/models"
)

func TestCheckStoppable(t *testing.T) {
	assert.NoError(t, CheckStoppable(models.OperatorDocumentGo{OperatorType: constants.OperatorTypeRemote}))
	assert.ErrorIs(t, CheckStoppable(models.OperatorDocumentGo{OperatorType: constants.OperatorTypeEmbedded}), constants.ErrOperatorStopEmbedded)
	assert.ErrorIs(t, CheckStoppable(models.OperatorDocumentGo{}), constants.ErrOperatorStopNotRemote)
}

func TestNewStopRequest_TrimsReason(t *testing.T) {
	assert.Equal(t, models.StopOperatorRequest{OperatorSessionID: "s1", Reason: "why"}, NewStopRequest("s1", "  why \n"))
}

func TestDecodeStopResponse(t *testing.T) {
	got, err := DecodeStopResponse([]byte(`{"success":true,"operator_id":"op","operator_session_id":"s1","transaction_id":"tx"}`))
	require.NoError(t, err)
	assert.Equal(t, "op", got.OperatorID)

	_, err = DecodeStopResponse([]byte(`{"success":false}`))
	assert.ErrorContains(t, err, "unsuccessful")

	_, err = DecodeStopResponse([]byte(`not json`))
	assert.ErrorContains(t, err, "parse response")
}
