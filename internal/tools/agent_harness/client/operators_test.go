// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package client

import (
	"context"
	"testing"

	operatorv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/operator/v1"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/tools/agent_harness/config"
)

func stackDataOperator(id, sessionID string) *operatorv1.OperatorDocument {
	return operatorv1.OperatorDocument{
		ID: id, OperatorSessionID: sessionID, CurrentHostname: constants.DataOperatorHostname,
		Status: constants.OperatorStatusActive, OperatorType: constants.OperatorTypeRemote,
	}
}

func TestDiscoverRemoteOperator_RequiresAuthenticatedUser(t *testing.T) {
	client, err := New(config.Config{MTLSBaseURL: "https://gateway.invalid"})
	require.NoError(t, err)

	_, _, err = client.DiscoverRemoteOperator(context.Background())

	assert.ErrorIs(t, err, constants.ErrMissingRequiredField)
}
