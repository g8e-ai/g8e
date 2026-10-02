// Copyright (c) 2026 Lateralus Labs, LLC.
// Licensed under the Business Source License 1.1 — see LICENSE for details.

package publicdisclosure

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/models"
)

func TestPublicFeedReleaseIdentity_PreservesKnownAndUnknownProvenance(t *testing.T) {
	for _, tc := range []struct {
		name, view, wire string
		valid            bool
	}{
		{name: "recorded", view: `,"release":"v2.2.8","release_basis":"recorded","source_revision":"abc123"`, wire: `,"release":"v2.2.8","release_basis":"PUBLIC_RELEASE_BASIS_RECORDED","source_revision":"abc123"`, valid: true},
		{name: "asserted", view: `,"release":"v2.2.7","release_basis":"asserted"`, wire: `,"release":"v2.2.7","release_basis":"PUBLIC_RELEASE_BASIS_ASSERTED"`, valid: true},
		{name: "unknown", view: `,"release":"","release_basis":"unknown"`, wire: `,"release_basis":"PUBLIC_RELEASE_BASIS_UNKNOWN"`, valid: true},
		{name: "historical identity absent", valid: true},
		{name: "release without basis", view: `,"release":"v2.2.8"`, wire: `,"release":"v2.2.8"`},
		{name: "known basis without release", view: `,"release_basis":"recorded"`, wire: `,"release_basis":"PUBLIC_RELEASE_BASIS_RECORDED"`},
		{name: "unknown cannot name a release", view: `,"release":"v2.2.8","release_basis":"unknown"`, wire: `,"release":"v2.2.8","release_basis":"PUBLIC_RELEASE_BASIS_UNKNOWN"`},
		{name: "unknown basis value", view: `,"release_basis":"inferred"`, wire: `,"release_basis":"PUBLIC_RELEASE_BASIS_INFERRED"`},
		{name: "revision is a string", view: `,"release_basis":"unknown","source_revision":42`, wire: `,"release_basis":"PUBLIC_RELEASE_BASIS_UNKNOWN","source_revision":42`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			bodies := []struct {
				name string
				kind models.PublicFeedRecordType
				body string
			}{
				{"live event", models.PublicFeedRecordTypeEvent, `{"schema_version":"1.6.0","kind":"stage_updated","dataset_id":"ds","quality_state":"live_in_progress","observed_at":"2026-10-02T12:00:00Z","event_id":"event-1","run_id":"run-1","lifecycle_status":"running","completed":0,"total":1` + tc.wire + `}`},
				{"view snapshot", models.PublicFeedRecordTypeProjection, `{"schema_version":"1.6.0","kind":"evaluation_summary","dataset_id":"ds","quality_state":"live_in_progress","observed_at":"2026-10-02T12:00:00Z"` + tc.view + `}`},
				{"lifecycle", models.PublicFeedRecordTypeProjection, `{"schema_version":"1.0.0","message_type":"PublicAssignmentLifecycleRecord","idempotency_key":"run-1:assignment-1","record":{"assignment_id":"assignment-1","run_id":"run-1","scenario_id":"scenario-1"` + tc.wire + `}}`},
				{"result", models.PublicFeedRecordTypeProjection, `{"schema_version":"1.2.0","message_type":"PublicAssignmentResultProjection","idempotency_key":"run-1:assignment-1","record":{"assignment_id":"assignment-1","run_id":"run-1","scenario_id":"scenario-1"` + tc.wire + `}}`},
			}
			for _, body := range bodies {
				t.Run(body.name, func(t *testing.T) {
					err := ValidatePublicFeedRecord(body.kind, []byte(body.body))
					if tc.valid {
						require.NoError(t, err)
					} else if body.name == "live event" || body.name == "view snapshot" {
						require.ErrorIs(t, err, constants.ErrPublicFeedRecordSchemaInvalid)
					} else {
						require.ErrorIs(t, err, constants.ErrEvidenceSchemaMismatch)
					}
				})
			}
		})
	}
}
