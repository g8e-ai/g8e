// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package constants

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidateGovernedRequest(t *testing.T) {
	t.Parallel()

	action, err := ValidateGovernedRequest(Event.Operator.FsRead.Requested)
	require.NoError(t, err)
	assert.Equal(t, ActionTypeFsRead, action)

	action, err = ValidateGovernedRequest(EventAppCaseCreateRequested)
	require.NoError(t, err)
	assert.Equal(t, ActionTypeDocumentUpdate, action)

	_, err = ValidateGovernedRequest(Event.Operator.Command.Completed)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrTxEventNotRequest)

	_, err = ValidateGovernedRequest(EventType("g8e.v1.not.registered"))
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrTxUnknownEventType)
}

func TestValidateGovernedEnvelopeFields(t *testing.T) {
	t.Parallel()

	err := ValidateGovernedEnvelopeFields(Event.Operator.FsRead.Requested, ActionTypeFsRead)
	require.NoError(t, err)

	err = ValidateGovernedEnvelopeFields(Event.Operator.FsRead.Requested, ActionTypeExecuteBash)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrTxEventActionMismatch)
}

func TestValidateGovernedResultEnvelope(t *testing.T) {
	t.Parallel()

	err := ValidateGovernedResultEnvelope(
		Event.Operator.Command.Requested,
		Event.Operator.Command.Completed,
		ActionTypeExecuteBash,
	)
	require.NoError(t, err)

	err = ValidateGovernedResultEnvelope(
		Event.Operator.Command.Requested,
		Event.Operator.Command.Completed,
		ActionTypeFsRead,
	)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrTxEventActionMismatch)

	err = ValidateGovernedResultEnvelope(
		Event.Operator.Command.Requested,
		Event.Operator.FsRead.Completed,
		ActionTypeExecuteBash,
	)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrTxOutcomeNotAllowed)

	err = ValidateGovernedResultEnvelope(
		Event.Operator.Command.Requested,
		EventType("g8e.v1.not.registered"),
		ActionTypeExecuteBash,
	)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrTxOutcomeNotRegistered)
}
