// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package model_provenance

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	evalv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/eval/v1"
)

type stubAttestor struct {
	window *evalv1.ModelProvenanceAttestationWindow
	err    error
}

func (s *stubAttestor) Attest(_ context.Context, _, _ string, _ time.Time) (*evalv1.ModelProvenanceAttestationWindow, error) {
	if s.err != nil {
		return nil, s.err
	}
	return s.window, nil
}

func TestTracker_BeginAndFinalize(t *testing.T) {
	ctx := context.Background()
	now := time.Unix(1_700_000_000, 0).UTC()
	attestor := &stubAttestor{
		window: &evalv1.ModelProvenanceAttestationWindow{
			SchemaVersion:        SchemaVersion,
			ServedModelTag:       "probe-model:7b",
			ExpectedModelDigest:  "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
			ObservedModelDigest:  "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
			ManifestDigest:       "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
			AttestedAtUnixMs:     now.UnixMilli(),
			DigestMatch:          true,
		},
	}
	tracker, err := NewTracker(TrackerConfig{
		OperatorID: "test-provenance-operator",
		Attestor:   attestor,
		Now:        func() time.Time { return now },
	})
	require.NoError(t, err)

	begin := &evalv1.ModelProvenanceObservationCommand{
		ProviderAttemptId:      "attempt-1",
		Phase:                  evalv1.ModelProvenanceObservationPhase_MODEL_PROVENANCE_OBSERVATION_PHASE_BEGIN,
		AttemptStartedAtUnixMs: now.UnixMilli(),
		ServedModelTag:         "probe-model:7b",
		ExpectedModelDigest:    "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
	}
	require.NoError(t, tracker.Begin(ctx, begin))

	finalize := &evalv1.ModelProvenanceObservationCommand{
		ProviderAttemptId:        "attempt-1",
		Phase:                    evalv1.ModelProvenanceObservationPhase_MODEL_PROVENANCE_OBSERVATION_PHASE_FINALIZE,
		AttemptStartedAtUnixMs:   now.UnixMilli(),
		AttemptCompletedAtUnixMs: now.Add(2 * time.Second).UnixMilli(),
		ServedModelTag:           "probe-model:7b",
		ExpectedModelDigest:      "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
	}
	window, err := tracker.Finalize(ctx, finalize)
	require.NoError(t, err)
	assert.Equal(t, "attempt-1", window.GetProviderAttemptId())
	assert.Equal(t, "test-provenance-operator", window.GetProvenanceOperatorId())
	assert.NotEmpty(t, window.GetAttestationDigest())
}

func TestTracker_FinalizeWithoutBegin_UsesFinalizeCommandBinding(t *testing.T) {
	ctx := context.Background()
	now := time.Unix(1_700_000_000, 0).UTC()
	attestor := &stubAttestor{
		window: &evalv1.ModelProvenanceAttestationWindow{
			SchemaVersion:       SchemaVersion,
			ServedModelTag:      "probe-model:7b",
			ExpectedModelDigest: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
			ObservedModelDigest: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
			ManifestDigest:      "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
			AttestedAtUnixMs:    now.UnixMilli(),
			DigestMatch:         true,
		},
	}
	tracker, err := NewTracker(TrackerConfig{Attestor: attestor, Now: func() time.Time { return now }})
	require.NoError(t, err)

	finalize := &evalv1.ModelProvenanceObservationCommand{
		ProviderAttemptId:        "attempt-orphan",
		Phase:                    evalv1.ModelProvenanceObservationPhase_MODEL_PROVENANCE_OBSERVATION_PHASE_FINALIZE,
		AttemptStartedAtUnixMs:   now.UnixMilli(),
		AttemptCompletedAtUnixMs: now.Add(time.Second).UnixMilli(),
		ServedModelTag:           "probe-model:7b",
		ExpectedModelDigest:      "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
	}
	window, err := tracker.Finalize(ctx, finalize)
	require.NoError(t, err)
	assert.Equal(t, "attempt-orphan", window.GetProviderAttemptId())
}

func TestTracker_Finalize_DigestMismatchFails(t *testing.T) {
	ctx := context.Background()
	now := time.Unix(1_700_000_000, 0).UTC()
	attestor := &stubAttestor{
		window: &evalv1.ModelProvenanceAttestationWindow{
			SchemaVersion:       SchemaVersion,
			ServedModelTag:      "probe-model:7b",
			ExpectedModelDigest: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
			ObservedModelDigest: "cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc",
			ManifestDigest:      "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
			AttestedAtUnixMs:    now.UnixMilli(),
			DigestMatch:         false,
		},
	}
	tracker, err := NewTracker(TrackerConfig{Attestor: attestor, Now: func() time.Time { return now }})
	require.NoError(t, err)

	finalize := &evalv1.ModelProvenanceObservationCommand{
		ProviderAttemptId:      "attempt-1",
		Phase:                  evalv1.ModelProvenanceObservationPhase_MODEL_PROVENANCE_OBSERVATION_PHASE_FINALIZE,
		ServedModelTag:         "probe-model:7b",
		ExpectedModelDigest:    "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		AttemptStartedAtUnixMs: now.UnixMilli(),
	}
	_, err = tracker.Finalize(ctx, finalize)
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrModelProvenanceDigestMismatch)
}
