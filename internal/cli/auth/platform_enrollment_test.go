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

	"github.com/g8e-ai/g8e/v2/internal/models"
)

func TestPlatformEnrollmentFingerprints(t *testing.T) {
	t.Run("keeps only present keys in a fixed order", func(t *testing.T) {
		got := PlatformEnrollmentFingerprints(models.PlatformEnrollmentCSRFingerprints{CLI: "cli-fp", App: "app-fp"})
		assert.Equal(t, []EnrollmentFingerprint{{Label: "App key", Value: "app-fp"}, {Label: "CLI key", Value: "cli-fp"}}, got)
	})
	t.Run("none present", func(t *testing.T) {
		assert.Empty(t, PlatformEnrollmentFingerprints(models.PlatformEnrollmentCSRFingerprints{}))
	})
}

func TestPlatformEnrollmentIssuedIdentity(t *testing.T) {
	tests := []struct {
		name string
		in   models.PlatformEnrollmentEnrolledRequest
		want string
	}{
		{name: "operator id", in: models.PlatformEnrollmentEnrolledRequest{ComponentKind: models.PlatformComponentOperator, OperatorID: "op-1", OperatorSessionID: "sess-1"}, want: "op-1"},
		{name: "operator session fallback", in: models.PlatformEnrollmentEnrolledRequest{ComponentKind: models.PlatformComponentOperator, OperatorSessionID: "sess-1"}, want: "sess-1"},
		{name: "app policy", in: models.PlatformEnrollmentEnrolledRequest{ComponentKind: models.PlatformComponentEnsemble, PolicyID: "policy-1", OperatorID: "ignored"}, want: "policy-1"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, PlatformEnrollmentIssuedIdentity(tt.in))
		})
	}
}

func TestDecodePlatformEnrollmentResponses(t *testing.T) {
	decision, err := DecodePlatformEnrollmentDecision([]byte(`{"request_id":"req-1","state":"approved"}`))
	require.NoError(t, err)
	assert.Equal(t, "req-1", decision.RequestID)
	assert.Equal(t, models.PlatformEnrollmentState("approved"), decision.State)

	revoke, err := DecodePlatformEnrollmentRevoke([]byte(`{"request_id":"req-2","component_kind":"operator","state":"revoked"}`))
	require.NoError(t, err)
	assert.Equal(t, "req-2", revoke.RequestID)
	assert.Equal(t, models.PlatformComponentOperator, revoke.ComponentKind)

	_, err = DecodePlatformEnrollmentDecision([]byte(`not json`))
	require.ErrorContains(t, err, "parse decision response")
	_, err = DecodePlatformEnrollmentRevoke([]byte(`not json`))
	require.ErrorContains(t, err, "parse response")
}
