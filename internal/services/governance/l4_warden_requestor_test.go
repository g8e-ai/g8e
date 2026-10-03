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
	govtypes "github.com/g8e-ai/g8e/v2/internal/governance"
	"github.com/g8e-ai/g8e/v2/internal/testutil"
	commonv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/common/v1"
)

type requestorNotary struct {
	userID string
}

func (n *requestorNotary) VerifyL3Proof(_ context.Context, userID, _, _ string, _ *commonv1.L3Proof) (bool, error) {
	n.userID = userID
	return userID == "requesting-user", nil
}

func TestL4Warden_L3VerifiesRequestorInsteadOfExecutionTarget(t *testing.T) {
	for _, target := range []string{string(constants.DocIDEmbeddedOperator), "outbound-operator"} {
		t.Run(target, func(t *testing.T) {
			notary := &requestorNotary{}
			warden := &L4Warden{l3Notary: notary, logger: testutil.NewTestLogger()}
			env := &govtypes.GovernanceEnvelope{
				OperatorId: target, RequestorUserId: "requesting-user",
				ActionType: string(constants.ActionTypeExecuteBash),
				Governance: &commonv1.GovernanceMetadata{L3: &commonv1.L3Metadata{Proof: &commonv1.L3Proof{CredentialId: "credential-1"}}},
			}
			valid, err := warden.verifyL3Posture(context.Background(), env, &RatifyPosture{})
			require.NoError(t, err)
			require.True(t, valid)
			require.Equal(t, env.RequestorUserId, notary.userID)
		})
	}
}
