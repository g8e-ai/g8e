// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package provider_observer

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	evalv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/eval/v1"
)

type stubObservationPublisher struct {
	completions []*evalv1.ProviderBoundaryObservationCompleted
	err         error
}

func (s *stubObservationPublisher) PublishProviderBoundaryObservationCompleted(_ context.Context, _ string, completion *evalv1.ProviderBoundaryObservationCompleted) error {
	if s.err != nil {
		return s.err
	}
	s.completions = append(s.completions, completion)
	return nil
}

func testProviderObserverHandler(t *testing.T) (*Handler, *stubObservationPublisher) {
	t.Helper()
	now := time.Unix(1_700_000_000, 0).UTC()
	tracker, err := NewTracker(TrackerConfig{
		ObserverID:     "observer-test",
		Collector:      &stubCollector{},
		SampleInterval: time.Hour,
		Now:            func() time.Time { return now },
	})
	require.NoError(t, err)
	publisher := &stubObservationPublisher{}
	logger := slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil))
	handler, err := NewHandler(tracker, publisher, logger)
	require.NoError(t, err)
	return handler, publisher
}

func TestNewHandler_RequiresDependencies(t *testing.T) {
	t.Parallel()
	tracker, err := NewTracker(TrackerConfig{
		ObserverID: "observer-test",
		Collector:  &stubCollector{},
	})
	require.NoError(t, err)
	publisher := &stubObservationPublisher{}
	logger := slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil))

	tests := []struct {
		name      string
		tracker   *Tracker
		publisher ObservationResultPublisher
		logger    *slog.Logger
	}{
		{name: "nil tracker", tracker: nil, publisher: publisher, logger: logger},
		{name: "nil publisher", tracker: tracker, publisher: nil, logger: logger},
		{name: "nil logger", tracker: tracker, publisher: publisher, logger: nil},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			_, err := NewHandler(test.tracker, test.publisher, test.logger)
			require.ErrorIs(t, err, constants.ErrMissingRequiredField)
		})
	}
}

func TestHandler_HandleCommand(t *testing.T) {
	ctx := context.Background()
	now := time.Unix(1_700_000_000, 0).UTC()

	tests := []struct {
		name       string
		setup      func(t *testing.T, handler *Handler) []byte
		wantID     string
		wantErr    bool
		wantSubstr string
		wantPubs   int
	}{
		{
			name: "begin returns provider attempt id",
			setup: func(t *testing.T, _ *Handler) []byte {
				command := &evalv1.ProviderBoundaryObservationCommand{
					ProviderAttemptId:      "attempt-1",
					Phase:                  evalv1.ProviderBoundaryObservationPhase_PROVIDER_BOUNDARY_OBSERVATION_PHASE_BEGIN,
					AttemptStartedAtUnixMs: now.UnixMilli(),
				}
				payload, err := proto.Marshal(command)
				require.NoError(t, err)
				return payload
			},
			wantID: "attempt-1",
		},
		{
			name: "finalize publishes completion window",
			setup: func(t *testing.T, handler *Handler) []byte {
				begin := &evalv1.ProviderBoundaryObservationCommand{
					ProviderAttemptId:      "attempt-1",
					Phase:                  evalv1.ProviderBoundaryObservationPhase_PROVIDER_BOUNDARY_OBSERVATION_PHASE_BEGIN,
					AttemptStartedAtUnixMs: now.UnixMilli(),
				}
				beginPayload, err := proto.Marshal(begin)
				require.NoError(t, err)
				_, err = handler.HandleCommand(ctx, "msg-begin", beginPayload)
				require.NoError(t, err)

				finalize := &evalv1.ProviderBoundaryObservationCommand{
					ProviderAttemptId:        "attempt-1",
					InferenceTransactionId:   "txn-1",
					Phase:                    evalv1.ProviderBoundaryObservationPhase_PROVIDER_BOUNDARY_OBSERVATION_PHASE_FINALIZE,
					AttemptStartedAtUnixMs:   now.UnixMilli(),
					AttemptCompletedAtUnixMs: now.Add(2 * time.Second).UnixMilli(),
					AttemptStatus:            evalv1.ProviderBoundaryObservationAttemptStatus_PROVIDER_BOUNDARY_OBSERVATION_ATTEMPT_STATUS_COMPLETED,
				}
				payload, err := proto.Marshal(finalize)
				require.NoError(t, err)
				return payload
			},
			wantPubs: 1,
		},
		{
			name: "invalid payload",
			setup: func(t *testing.T, _ *Handler) []byte {
				return []byte("not-a-proto")
			},
			wantErr:    true,
			wantSubstr: "unmarshal",
		},
		{
			name: "unsupported phase",
			setup: func(t *testing.T, _ *Handler) []byte {
				command := &evalv1.ProviderBoundaryObservationCommand{
					ProviderAttemptId: "attempt-1",
					Phase:             evalv1.ProviderBoundaryObservationPhase_PROVIDER_BOUNDARY_OBSERVATION_PHASE_UNSPECIFIED,
				}
				payload, err := proto.Marshal(command)
				require.NoError(t, err)
				return payload
			},
			wantErr:    true,
			wantSubstr: "unsupported phase",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			handler, publisher := testProviderObserverHandler(t)
			payload := test.setup(t, handler)
			id, err := handler.HandleCommand(ctx, "msg-1", payload)
			if test.wantErr {
				require.Error(t, err)
				if test.wantSubstr != "" {
					assert.Contains(t, err.Error(), test.wantSubstr)
				}
				return
			}
			require.NoError(t, err)
			if test.wantID != "" {
				assert.Equal(t, test.wantID, id)
			}
			assert.Len(t, publisher.completions, test.wantPubs)
		})
	}
}

func TestHandler_PublishFailure(t *testing.T) {
	ctx := context.Background()
	handler, publisher := testProviderObserverHandler(t)
	publisher.err = errors.New("publish failed")
	now := time.Unix(1_700_000_000, 0).UTC()

	begin := &evalv1.ProviderBoundaryObservationCommand{
		ProviderAttemptId:      "attempt-1",
		Phase:                  evalv1.ProviderBoundaryObservationPhase_PROVIDER_BOUNDARY_OBSERVATION_PHASE_BEGIN,
		AttemptStartedAtUnixMs: now.UnixMilli(),
	}
	beginPayload, err := proto.Marshal(begin)
	require.NoError(t, err)
	_, err = handler.HandleCommand(ctx, "msg-begin", beginPayload)
	require.NoError(t, err)

	finalize := &evalv1.ProviderBoundaryObservationCommand{
		ProviderAttemptId:        "attempt-1",
		Phase:                    evalv1.ProviderBoundaryObservationPhase_PROVIDER_BOUNDARY_OBSERVATION_PHASE_FINALIZE,
		AttemptStartedAtUnixMs:   now.UnixMilli(),
		AttemptCompletedAtUnixMs: now.Add(time.Second).UnixMilli(),
	}
	finalizePayload, err := proto.Marshal(finalize)
	require.NoError(t, err)
	_, err = handler.HandleCommand(ctx, "msg-finalize", finalizePayload)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "publish completion")
}
