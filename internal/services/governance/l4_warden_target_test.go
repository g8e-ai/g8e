// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package governance

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/services/gateway/embedded"
	"github.com/g8e-ai/g8e/v2/internal/testutil"
)

func TestL4Warden_RejectsForeignExecutionTarget(t *testing.T) {
	warden, key := createStrictVerifier(t, testutil.NewStatefulMockReplayStore(), testutil.NewMockStateRootProvider("root-1"), testutil.NewConfigurableMockL3Notary(true))
	env := signedEnvelope(t, constants.ActionTypeFsRead, typedPayload(t, constants.ActionTypeFsRead), key, constants.PostureDoctrine)
	env.OperatorId = "foreign-outbound-operator"
	rehash(t, env)
	_, err := warden.VerifyEnvelope(context.Background(), env)
	require.ErrorIs(t, err, constants.ErrTxTargetOperatorMismatch)
}

func TestL4Warden_ExecutionTarget(t *testing.T) {
	outbound := &testutil.MockExecutionTarget{OperatorIDs: []string{"operator-1"}}
	gateway := embedded.New(nil, nil)
	cases := []struct {
		name       string
		target     ExecutionTarget
		operatorID string
		action     constants.ActionType
		wantError  bool
	}{
		{"outbound own Operator", outbound, "operator-1", constants.ActionTypeFsRead, false},
		{"outbound foreign Operator", outbound, "operator-2", constants.ActionTypeFsRead, true},
		{"gateway outbound Operator", gateway, "operator-1", constants.ActionTypeFsRead, true},
		{"gateway embedded Operator", gateway, string(constants.DocIDEmbeddedOperator), constants.ActionTypeFsRead, false},
		{"gateway enrollment bootstrap", gateway, string(constants.DocIDEmbeddedOperator), constants.ActionTypePlatformEnrollmentCreate, false},
		{"unbound enrollment rejected", gateway, "", constants.ActionTypePlatformEnrollmentCreate, true},
		{"missing authority", nil, "operator-1", constants.ActionTypeFsRead, true},
		{"unbound host read", gateway, "", constants.ActionTypeFsRead, true},
		{"unbound app update", gateway, "", constants.ActionTypeDocumentUpdate, false},
		{"unbound app delete", gateway, "", constants.ActionTypeDocumentDelete, false},
		{"unbound app update without target dependency", nil, "", constants.ActionTypeDocumentUpdate, true},
		{"bound document foreign Operator", gateway, "operator-1", constants.ActionTypeDocumentUpdate, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			replay := testutil.NewStatefulMockReplayStore()
			warden, key := createStrictVerifier(t, replay, testutil.NewMockStateRootProvider("root-1"), testutil.NewConfigurableMockL3Notary(true))
			warden.executionTarget = tc.target
			env := signedEnvelope(t, tc.action, typedPayload(t, tc.action), key, constants.PostureDoctrine)
			env.OperatorId = tc.operatorID
			rehash(t, env)
			_, err := warden.VerifyEnvelope(context.Background(), env)
			if tc.wantError {
				require.ErrorIs(t, err, constants.ErrTxTargetOperatorMismatch)
				require.NotContains(t, replay.Nonces, env.Nonce, "rejection must release the reserved nonce")
			} else {
				require.NoError(t, err)
			}
		})
	}
}
