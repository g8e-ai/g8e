// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package testcmd

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/v2/internal/services/evaluation"
	"github.com/g8e-ai/g8e/v2/internal/services/publicdisclosure"
	evalv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/eval/v1"
)

func TestPublicLoopRecordDoesNotClaimAMeasuredRelease(t *testing.T) {
	record, err := publicLoopRecord(1, "qualification-campaign")
	require.NoError(t, err)
	require.NoError(t, publicdisclosure.ValidatePublicFeedRecord(record.RecordType, []byte(record.RecordBytes)))

	var envelope evaluation.CampaignProjectionEnvelope
	require.NoError(t, json.Unmarshal([]byte(record.RecordBytes), &envelope))
	var projection evalv1.PublicAssignmentLifecycleRecord
	require.NoError(t, evalv1.UnmarshalCanonical(envelope.Record, &projection))
	// This provider-free transport fixture never freezes or measures a campaign.
	require.Empty(t, projection.GetRelease())
	require.Empty(t, projection.GetSourceRevision())
	require.Equal(t, evalv1.PublicReleaseBasis_PUBLIC_RELEASE_BASIS_UNKNOWN, projection.GetReleaseBasis())
}
