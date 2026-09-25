// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License 2.0.

package evaluation

import (
	"context"
	"fmt"
	"time"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	evalv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/eval/v1"
)

// FormationHarnessProvenance records sovereign attestation calls for isolated
// adapter and integration tests.
type FormationHarnessProvenance struct {
	Events []string
}

func (p *FormationHarnessProvenance) Attest(_ context.Context, model FormationModel) (*FormationAttestation, error) {
	if p != nil {
		p.Events = append(p.Events, "attest:"+model.VariantID)
	}
	return &FormationAttestation{Verified: true, Digest: model.ModelDigest}, nil
}

// FormationHarnessObserver records provider-boundary bracketing for isolated tests.
type FormationHarnessObserver struct {
	Events []string
}

func (o *FormationHarnessObserver) Begin(_ context.Context, attemptID string, model FormationModel) error {
	if o != nil {
		o.Events = append(o.Events, "begin:"+attemptID+":"+model.VariantID)
	}
	return nil
}

func (o *FormationHarnessObserver) Finalize(_ context.Context, attemptID string, model FormationModel, failed bool) (*FormationObserverEvidence, error) {
	if o != nil {
		o.Events = append(o.Events, "finalize:"+attemptID+":"+model.VariantID)
	}
	return &FormationObserverEvidence{Window: &evalv1.ProviderBoundaryObservationWindow{
		ProviderAttemptId: attemptID,
		Samples: []*evalv1.ProviderBoundaryHardwareSample{{
			VramBytesAvailability: evalv1.ProviderHardwareMetricAvailability_PROVIDER_HARDWARE_METRIC_AVAILABILITY_REPORTED,
			VramUsedBytes:         2 * 1024 * 1024 * 1024,
		}},
	}}, nil
}

// FormationHarnessAllocator records local allocation and release ordering.
type FormationHarnessAllocator struct {
	Events []string
}

func (a *FormationHarnessAllocator) Allocate(_ context.Context, model FormationModel) error {
	if a != nil {
		a.Events = append(a.Events, "allocate:"+model.VariantID)
	}
	return nil
}

func (a *FormationHarnessAllocator) Release(_ context.Context, model FormationModel) error {
	if a != nil {
		a.Events = append(a.Events, "release:"+model.VariantID)
	}
	return nil
}

// FormationHarnessExecutor simulates Lite → Assistant → Primary state handoff.
type FormationHarnessExecutor struct {
	States []string
}

func (e *FormationHarnessExecutor) ExecuteRole(_ context.Context, req FormationRoleRequest) (FormationRoleResult, error) {
	if e != nil {
		e.States = append(e.States, string(req.Role)+":"+string(req.InputState))
	}
	output := append(append([]byte(nil), req.InputState...), []byte("/"+req.Role)...)
	return FormationRoleResult{
		OutputState:             output,
		MutationCandidate:       []byte("mutation:" + string(req.Role)),
		StateMutation:           req.Role == FormationRolePrimary,
		ProviderAttemptID:       req.AttemptID,
		TTFTNanos:               2 * uint64(time.Millisecond),
		GenerationTokens:        20,
		GenerationDurationNanos: 10 * uint64(time.Millisecond),
	}, nil
}

// FormationHarnessPolicy accepts governed mutation candidates in isolated tests.
type FormationHarnessPolicy struct {
	Calls int
}

func (p *FormationHarnessPolicy) ValidateMutation(_ context.Context, _ string, _ FormationRole, _ []byte) (FormationPolicyValidation, error) {
	if p != nil {
		p.Calls++
	}
	return FormationPolicyValidation{
		L1Validated: true,
		L2Validated: true,
		L3Validated: true,
		L4Validated: true,
		L5Validated: true,
		Intercepted: true,
		ReceiptRef:  "harness-receipt",
	}, nil
}

// FormationHarness wires fake operator dependencies for isolated adapter tests.
type FormationHarness struct {
	Provenance *FormationHarnessProvenance
	Observer   *FormationHarnessObserver
	Allocator  *FormationHarnessAllocator
	Executor   *FormationHarnessExecutor
	Policy     *FormationHarnessPolicy
	Runner     *FormationRunner
}

// NewFormationHarness constructs a governed FormationRunner backed by in-memory
// operator fakes. Production adapters replace these dependencies with exact
// Operator session coordinators.
func NewFormationHarness(now func() time.Time, newID func(string) string) (*FormationHarness, error) {
	harness := &FormationHarness{
		Provenance: &FormationHarnessProvenance{},
		Observer:   &FormationHarnessObserver{},
		Allocator:  &FormationHarnessAllocator{},
		Executor:   &FormationHarnessExecutor{},
		Policy:     &FormationHarnessPolicy{},
	}
	runner, err := NewFormationRunner(
		harness.Provenance,
		harness.Observer,
		harness.Allocator,
		harness.Executor,
		harness.Policy,
		now,
		newID,
	)
	if err != nil {
		return nil, err
	}
	harness.Runner = runner
	return harness, nil
}

// RunBoundFormation executes one registry-bound formation through the harness.
func (h *FormationHarness) RunBoundFormation(ctx context.Context, formation Formation, initialState []byte) (*FormationRunResult, error) {
	if h == nil || h.Runner == nil {
		return nil, fmt.Errorf("formation: harness: %w", constants.ErrFormationRunnerDependency)
	}
	return h.Runner.Run(ctx, formation, initialState)
}
