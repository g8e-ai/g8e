// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package model_provenance

import (
	"bytes"
	"context"
	"log/slog"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"

	evalv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/eval/v1"
)

type stubAttestationPublisher struct {
	completions []*evalv1.ModelProvenanceObservationCompleted
}

func (s *stubAttestationPublisher) PublishModelProvenanceObservationCompleted(_ context.Context, _ string, completion *evalv1.ModelProvenanceObservationCompleted) error {
	s.completions = append(s.completions, completion)
	return nil
}

func TestHandler_BeginAndFinalize(t *testing.T) {
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
	tracker, err := NewTracker(TrackerConfig{
		OperatorID: "test-provenance-operator",
		Attestor:   attestor,
		Now:        func() time.Time { return now },
	})
	require.NoError(t, err)
	publisher := &stubAttestationPublisher{}
	logger := slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil))
	handler, err := NewHandler(tracker, publisher, logger)
	require.NoError(t, err)

	begin := &evalv1.ModelProvenanceObservationCommand{
		ProviderAttemptId:      "attempt-1",
		Phase:                  evalv1.ModelProvenanceObservationPhase_MODEL_PROVENANCE_OBSERVATION_PHASE_BEGIN,
		AttemptStartedAtUnixMs: now.UnixMilli(),
		ServedModelTag:         "probe-model:7b",
		ExpectedModelDigest:    "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
	}
	beginPayload, err := proto.Marshal(begin)
	require.NoError(t, err)
	providerAttemptID, err := handler.HandleCommand(ctx, "msg-begin", beginPayload)
	require.NoError(t, err)
	assert.Equal(t, "attempt-1", providerAttemptID)

	finalize := &evalv1.ModelProvenanceObservationCommand{
		ProviderAttemptId:        "attempt-1",
		Phase:                    evalv1.ModelProvenanceObservationPhase_MODEL_PROVENANCE_OBSERVATION_PHASE_FINALIZE,
		AttemptStartedAtUnixMs:   now.UnixMilli(),
		AttemptCompletedAtUnixMs: now.Add(2 * time.Second).UnixMilli(),
		ServedModelTag:           "probe-model:7b",
		ExpectedModelDigest:      "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
	}
	finalizePayload, err := proto.Marshal(finalize)
	require.NoError(t, err)
	attestationDigest, err := handler.HandleCommand(ctx, "msg-finalize", finalizePayload)
	require.NoError(t, err)
	assert.NotEmpty(t, attestationDigest)
	require.Len(t, publisher.completions, 1)
	assert.Equal(t, "attempt-1", publisher.completions[0].GetWindow().GetProviderAttemptId())
}

func TestHandler_UnsupportedPhase(t *testing.T) {
	ctx := context.Background()
	now := time.Unix(1_700_000_000, 0).UTC()
	tracker, err := NewTracker(TrackerConfig{
		Attestor: &stubAttestor{window: &evalv1.ModelProvenanceAttestationWindow{DigestMatch: true}},
		Now:      func() time.Time { return now },
	})
	require.NoError(t, err)
	logger := slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil))
	handler, err := NewHandler(tracker, &stubAttestationPublisher{}, logger)
	require.NoError(t, err)

	command := &evalv1.ModelProvenanceObservationCommand{ProviderAttemptId: "attempt-1"}
	payload, err := proto.Marshal(command)
	require.NoError(t, err)
	_, err = handler.HandleCommand(ctx, "msg-1", payload)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported phase")
}

func TestNewHandler_RequiresDependencies(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil))
	_, err := NewHandler(nil, &stubAttestationPublisher{}, logger)
	require.Error(t, err)
}
