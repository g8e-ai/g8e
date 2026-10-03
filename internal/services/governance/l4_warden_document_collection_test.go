// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software
// is released under the Apache License, Version 2.0.

package governance

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/testutil"
	operatorv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/operator/v1"
)

// TestL4Warden_DocumentActionCollectionScope pins the governed document set.
// DOCUMENT_UPDATE and DOCUMENT_DELETE write the Gateway's document store
// directly, which also holds trusted_signers, users, app_policies, and other
// platform-authority records. L4 admits a document action only when its
// collection is marked _governed in protocol/constants/collections.json, so a
// bound or app-authenticated envelope cannot rewrite platform authority.
func TestL4Warden_DocumentActionCollectionScope(t *testing.T) {
	collections := []struct {
		collection constants.CollectionName
		governed   bool
	}{
		{constants.CollectionCases, true},
		{constants.CollectionTasks, true},
		{constants.CollectionInvestigations, true},
		{constants.CollectionMemories, true},
		{constants.CollectionAgentActivityMetadata, true},
		{constants.CollectionStakeResolutions, true},
		{constants.CollectionReputationState, true},
		{constants.CollectionReputationCommitments, true},
		{constants.CollectionTrustedSigners, false},
		{constants.CollectionUsers, false},
		{constants.CollectionAppPolicies, false},
		{constants.CollectionRevokedCertificates, false},
		{constants.CollectionPlatformEnrollments, false},
		{constants.CollectionOperators, false},
		{constants.CollectionSettings, false},
		{constants.CollectionName("unregistered_collection"), false},
	}
	payloads := map[constants.ActionType]func(collection constants.CollectionName) proto.Message{
		constants.ActionTypeDocumentUpdate: func(c constants.CollectionName) proto.Message {
			return &operatorv1.DocumentUpdateRequested{Collection: string(c), DocumentId: "doc-1"}
		},
		constants.ActionTypeDocumentDelete: func(c constants.CollectionName) proto.Message {
			return &operatorv1.DocumentDeleteRequested{Collection: string(c), DocumentId: "doc-1"}
		},
	}
	for actionType, build := range payloads {
		for _, tc := range collections {
			t.Run(string(actionType)+"/"+string(tc.collection), func(t *testing.T) {
				verifier, privKey := createStrictVerifier(t, testutil.NewStatefulMockReplayStore(), testutil.NewMockStateRootProvider("root-1"), testutil.NewConfigurableMockL3Notary(true))
				payload, err := proto.Marshal(build(tc.collection))
				require.NoError(t, err)
				env := signedEnvelope(t, actionType, payload, privKey, constants.PostureDoctrine)

				_, err = verifier.VerifyEnvelope(context.Background(), env)
				if tc.governed {
					require.NoError(t, err)
					return
				}
				require.ErrorIs(t, err, constants.ErrTxDocumentCollectionNotGoverned)
			})
		}
	}
}
