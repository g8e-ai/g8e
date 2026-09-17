// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package evaluation

import (
	"fmt"
	"sort"
	"time"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	compliancev1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/compliance/v1"
	evalv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/eval/v1"
)

var homogeneousScheduleRoles = []evalv1.ModelCampaignRole{
	evalv1.ModelCampaignRole_MODEL_CAMPAIGN_ROLE_PRIMARY,
	evalv1.ModelCampaignRole_MODEL_CAMPAIGN_ROLE_ASSISTANT,
	evalv1.ModelCampaignRole_MODEL_CAMPAIGN_ROLE_LITE,
}

// HomogeneousScheduleRequest carries the frozen inputs for one deterministic
// model-role assignment matrix.
type HomogeneousScheduleRequest struct {
	CampaignID      string
	RunID           string
	Catalog         *evalv1.EvaluationScenarioCatalog
	Variants        []*evalv1.ModelVariant
	RepetitionCount uint32
	QueuedAt        time.Time
}

// BuildHomogeneousAssignmentMatrix materializes the full model-role smoke matrix
// without deduplicating variants, roles, or scenarios.
func BuildHomogeneousAssignmentMatrix(req HomogeneousScheduleRequest) ([]*evalv1.EvaluationAssignment, error) {
	if req.CampaignID == "" || req.RunID == "" || req.Catalog == nil || len(req.Variants) == 0 {
		return nil, fmt.Errorf("evaluation: build homogeneous assignment matrix: %w", constants.ErrMissingRequiredField)
	}
	if req.RepetitionCount == 0 {
		req.RepetitionCount = 1
	}
	scenarios := append([]*evalv1.EvaluationScenarioDefinition(nil), req.Catalog.GetScenarios()...)
	sort.Slice(scenarios, func(i, j int) bool {
		return scenarios[i].GetScenarioId() < scenarios[j].GetScenarioId()
	})
	variants := append([]*evalv1.ModelVariant(nil), req.Variants...)
	sort.Slice(variants, func(i, j int) bool {
		left, right := variants[i].GetServedModelTag(), variants[j].GetServedModelTag()
		if left == right {
			return variants[i].GetVariantId() < variants[j].GetVariantId()
		}
		return left < right
	})
	queuedAt := req.QueuedAt
	if queuedAt.IsZero() {
		queuedAt = time.Now().UTC()
	}
	assignments := make([]*evalv1.EvaluationAssignment, 0, len(scenarios)*len(variants)*len(homogeneousScheduleRoles)*int(req.RepetitionCount))
	for _, scenario := range scenarios {
		for _, variant := range variants {
			for _, role := range homogeneousScheduleRoles {
				for repetition := uint32(1); repetition <= req.RepetitionCount; repetition++ {
					assignment, err := buildHomogeneousAssignment(req.CampaignID, req.RunID, scenario, variant, role, repetition, queuedAt)
					if err != nil {
						return nil, err
					}
					assignments = append(assignments, assignment)
				}
			}
		}
	}
	sort.Slice(assignments, func(i, j int) bool {
		return assignments[i].GetDeterministicIdentity() < assignments[j].GetDeterministicIdentity()
	})
	return assignments, nil
}

func buildHomogeneousAssignment(campaignID, runID string, scenario *evalv1.EvaluationScenarioDefinition, variant *evalv1.ModelVariant, role evalv1.ModelCampaignRole, repetition uint32, queuedAt time.Time) (*evalv1.EvaluationAssignment, error) {
	if scenario == nil || variant == nil {
		return nil, fmt.Errorf("evaluation: build homogeneous assignment: %w", constants.ErrMissingRequiredField)
	}
	clonedVariant, ok := proto.Clone(variant).(*evalv1.ModelVariant)
	if !ok {
		return nil, fmt.Errorf("evaluation: build homogeneous assignment: invalid variant clone")
	}
	assignment := &evalv1.EvaluationAssignment{
		SchemaVersion: CampaignSchemaVersion,
		CampaignId:    campaignID,
		RunId:         runID,
		ScenarioRef: &compliancev1.VersionedReference{
			Id:      scenario.GetScenarioId(),
			Version: scenario.GetScenarioVersion(),
		},
		ScenarioId:      scenario.GetScenarioId(),
		Lane:            evalv1.EvaluationLane_EVALUATION_LANE_MODEL_ROLE,
		LifecycleStatus: evalv1.EvaluationAssignmentLifecycleStatus_EVALUATION_ASSIGNMENT_LIFECYCLE_STATUS_QUEUED,
		Repetition:      repetition,
		QueuedAt:        timestamppb.New(queuedAt),
		Target: &evalv1.EvaluationAssignment_Homogeneous{
			Homogeneous: &evalv1.HomogeneousAssignmentTarget{
				CandidateVariant: clonedVariant,
				DesignatedRole:   role,
			},
		},
	}
	identity, err := ComputeAssignmentDeterministicIdentity(assignment)
	if err != nil {
		return nil, fmt.Errorf("evaluation: build homogeneous assignment: %w", err)
	}
	assignment.DeterministicIdentity = identity
	assignment.AssignmentId = identity
	return assignment, nil
}

// ValidateHomogeneousAssignmentMatrix verifies the Phase 4 scheduler gate for one
// homogeneous North Star smoke run.
func ValidateHomogeneousAssignmentMatrix(catalog *evalv1.EvaluationScenarioCatalog, inventory *ModelInventoryFreeze, repetitionCount uint32, assignments []*evalv1.EvaluationAssignment) error {
	if catalog == nil || inventory == nil {
		return fmt.Errorf("evaluation: validate homogeneous assignment matrix: %w", constants.ErrMissingRequiredField)
	}
	if repetitionCount == 0 {
		repetitionCount = 1
	}
	expected := uint64(len(inventory.Variants)) * NorthStarHomogeneousRoleCount * uint64(len(catalog.GetScenarios())) * uint64(repetitionCount)
	if uint64(len(assignments)) != expected {
		return fmt.Errorf("evaluation: validate homogeneous assignment matrix: expected %d assignments, got %d", expected, len(assignments))
	}
	seen := make(map[string]struct{}, len(assignments))
	for _, assignment := range assignments {
		if assignment == nil || assignment.GetAssignmentId() == "" {
			return fmt.Errorf("evaluation: validate homogeneous assignment matrix: %w", constants.ErrMissingRequiredField)
		}
		if assignment.GetLane() != evalv1.EvaluationLane_EVALUATION_LANE_MODEL_ROLE {
			return fmt.Errorf("evaluation: validate homogeneous assignment matrix: assignment %s has unexpected lane", assignment.GetAssignmentId())
		}
		if assignment.GetLifecycleStatus() != evalv1.EvaluationAssignmentLifecycleStatus_EVALUATION_ASSIGNMENT_LIFECYCLE_STATUS_QUEUED {
			return fmt.Errorf("evaluation: validate homogeneous assignment matrix: assignment %s must start queued", assignment.GetAssignmentId())
		}
		identity, err := ComputeAssignmentDeterministicIdentity(assignment)
		if err != nil {
			return err
		}
		if assignment.GetDeterministicIdentity() != identity || assignment.GetAssignmentId() != identity {
			return fmt.Errorf("evaluation: validate homogeneous assignment matrix: assignment %s identity mismatch", assignment.GetAssignmentId())
		}
		if _, exists := seen[identity]; exists {
			return fmt.Errorf("evaluation: validate homogeneous assignment matrix: duplicate assignment identity %s", identity)
		}
		seen[identity] = struct{}{}
	}
	return nil
}
