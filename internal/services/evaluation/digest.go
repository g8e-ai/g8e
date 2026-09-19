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
	"strings"

	"google.golang.org/protobuf/proto"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/models"
	evalv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/eval/v1"
	operatorv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/operator/v1"
)

const CampaignSchemaVersion = "1.0.0"

// ComputeCampaignSpecDigest returns the immutable digest for a campaign spec
// with campaign_digest cleared.
func ComputeCampaignSpecDigest(spec *evalv1.EvaluationCampaignSpec) (string, error) {
	if spec == nil || spec.GetCampaignId() == "" || spec.GetCatalogRef() == nil || spec.GetCatalogDigest() == "" || len(spec.GetModelRegistry()) == 0 || spec.GetModelRegistryDigest() == "" {
		return "", fmt.Errorf("evaluation: compute campaign spec digest: %w", constants.ErrMissingRequiredField)
	}
	clone, ok := proto.Clone(spec).(*evalv1.EvaluationCampaignSpec)
	if !ok {
		return "", fmt.Errorf("evaluation: compute campaign spec digest: invalid clone")
	}
	clone.CampaignDigest = ""
	return digestProto(clone)
}

// ValidateCampaignSpecDigest verifies the declared campaign_digest binding.
func ValidateCampaignSpecDigest(spec *evalv1.EvaluationCampaignSpec) error {
	if spec == nil {
		return fmt.Errorf("evaluation: validate campaign spec digest: %w", constants.ErrMissingRequiredField)
	}
	expected, err := ComputeCampaignSpecDigest(spec)
	if err != nil {
		return err
	}
	if spec.GetCampaignDigest() != expected {
		return fmt.Errorf("evaluation: validate campaign spec digest: digest mismatch")
	}
	return nil
}

// ComputeScenarioCatalogDigest returns the immutable digest for a scenario
// catalog with catalog_digest cleared and scenarios sorted by scenario_id.
func ComputeScenarioCatalogDigest(catalog *evalv1.EvaluationScenarioCatalog) (string, error) {
	if catalog == nil || catalog.GetCatalogRef() == nil || len(catalog.GetScenarios()) == 0 {
		return "", fmt.Errorf("evaluation: compute scenario catalog digest: %w", constants.ErrMissingRequiredField)
	}
	clone, ok := proto.Clone(catalog).(*evalv1.EvaluationScenarioCatalog)
	if !ok {
		return "", fmt.Errorf("evaluation: compute scenario catalog digest: invalid clone")
	}
	clone.CatalogDigest = ""
	sort.Slice(clone.Scenarios, func(i, j int) bool {
		return clone.Scenarios[i].GetScenarioId() < clone.Scenarios[j].GetScenarioId()
	})
	return digestProto(clone)
}

// ValidateScenarioCatalogDigest verifies the declared catalog_digest binding.
func ValidateScenarioCatalogDigest(catalog *evalv1.EvaluationScenarioCatalog) error {
	if catalog == nil {
		return fmt.Errorf("evaluation: validate scenario catalog digest: %w", constants.ErrMissingRequiredField)
	}
	expected, err := ComputeScenarioCatalogDigest(catalog)
	if err != nil {
		return err
	}
	if catalog.GetCatalogDigest() != expected {
		return fmt.Errorf("evaluation: validate scenario catalog digest: digest mismatch")
	}
	return nil
}

// ComputeModelVariantRegistryDigest returns the campaign registry digest for
// eval ModelVariant records using the same binding as governed inference.
func ComputeModelVariantRegistryDigest(campaignID string, variants []*evalv1.ModelVariant) (string, error) {
	if campaignID == "" || len(variants) == 0 {
		return "", fmt.Errorf("evaluation: compute model variant registry digest: %w", constants.ErrMissingRequiredField)
	}
	inferenceVariants := make([]*operatorv1.InferenceModelVariant, 0, len(variants))
	for _, variant := range variants {
		if variant == nil || variant.GetServedModelTag() == "" || variant.GetModelDigest() == "" {
			return "", fmt.Errorf("evaluation: compute model variant registry digest: %w", constants.ErrMissingRequiredField)
		}
		inferenceVariants = append(inferenceVariants, &operatorv1.InferenceModelVariant{
			Model:  variant.GetServedModelTag(),
			Digest: variant.GetModelDigest(),
		})
	}
	return models.ComputeInferenceModelRegistryDigest(campaignID, inferenceVariants)
}

// ComputeHeterogeneousStackDigest returns the immutable digest for one stack
// definition with stack_digest cleared.
func ComputeHeterogeneousStackDigest(stack *evalv1.HeterogeneousStackDefinition) (string, error) {
	if stack == nil || stack.GetStackId() == "" || stack.GetPrimarySlot() == nil || stack.GetAssistantSlot() == nil || stack.GetLiteSlot() == nil {
		return "", fmt.Errorf("evaluation: compute heterogeneous stack digest: %w", constants.ErrMissingRequiredField)
	}
	clone, ok := proto.Clone(stack).(*evalv1.HeterogeneousStackDefinition)
	if !ok {
		return "", fmt.Errorf("evaluation: compute heterogeneous stack digest: invalid clone")
	}
	clone.StackDigest = ""
	return digestProto(clone)
}

// ValidateHeterogeneousStackDigest verifies the declared stack_digest binding.
func ValidateHeterogeneousStackDigest(stack *evalv1.HeterogeneousStackDefinition) error {
	if stack == nil {
		return fmt.Errorf("evaluation: validate heterogeneous stack digest: %w", constants.ErrMissingRequiredField)
	}
	expected, err := ComputeHeterogeneousStackDigest(stack)
	if err != nil {
		return err
	}
	if stack.GetStackDigest() != expected {
		return fmt.Errorf("evaluation: validate heterogeneous stack digest: digest mismatch")
	}
	return nil
}

// ComputeHeterogeneousStackSetDigest returns the immutable digest for one stack
// set with set_digest cleared.
func ComputeHeterogeneousStackSetDigest(set *HeterogeneousStackSet) (string, error) {
	if set == nil || set.CampaignID == "" || set.GenerationRule == "" || len(set.Stacks) == 0 || len(set.VariantIDs) == 0 {
		return "", fmt.Errorf("evaluation: compute heterogeneous stack set digest: %w", constants.ErrMissingRequiredField)
	}
	stackDigests := make([]string, 0, len(set.Stacks))
	for _, stack := range set.Stacks {
		if stack == nil {
			return "", fmt.Errorf("evaluation: compute heterogeneous stack set digest: %w", constants.ErrMissingRequiredField)
		}
		digest := stack.GetStackDigest()
		if digest == "" {
			digest, err := ComputeHeterogeneousStackDigest(stack)
			if err != nil {
				return "", err
			}
			stackDigests = append(stackDigests, digest)
			continue
		}
		stackDigests = append(stackDigests, digest)
	}
	sort.Strings(stackDigests)
	variantIDs := append([]string(nil), set.VariantIDs...)
	sort.Strings(variantIDs)
	parts := []string{
		set.GenerationRule,
		set.CampaignID,
		fmt.Sprintf("%d", set.Seed),
		strings.Join(variantIDs, "\x1f"),
		strings.Join(stackDigests, "\x1f"),
	}
	return models.SHA256Hex([]byte(strings.Join(parts, "\x1e"))), nil
}

// ComputeAssignmentDeterministicIdentity derives the stable assignment cell
// identity from campaign, scenario, lane, repetition, and target binding.
func ComputeAssignmentDeterministicIdentity(assignment *evalv1.EvaluationAssignment) (string, error) {
	if assignment == nil || assignment.GetCampaignId() == "" || assignment.GetScenarioId() == "" || assignment.GetLane() == evalv1.EvaluationLane_EVALUATION_LANE_UNSPECIFIED {
		return "", fmt.Errorf("evaluation: compute assignment identity: %w", constants.ErrMissingRequiredField)
	}
	parts := []string{
		assignment.GetCampaignId(),
		assignment.GetScenarioId(),
		assignment.GetLane().String(),
		fmt.Sprintf("%d", assignment.GetRepetition()),
	}
	switch target := assignment.GetTarget().(type) {
	case *evalv1.EvaluationAssignment_Homogeneous:
		if target.Homogeneous == nil || target.Homogeneous.GetCandidateVariant() == nil || target.Homogeneous.GetDesignatedRole() == evalv1.ModelCampaignRole_MODEL_CAMPAIGN_ROLE_UNSPECIFIED {
			return "", fmt.Errorf("evaluation: compute assignment identity: %w", constants.ErrMissingRequiredField)
		}
		parts = append(parts, "homogeneous", target.Homogeneous.GetCandidateVariant().GetVariantId(), target.Homogeneous.GetDesignatedRole().String())
	case *evalv1.EvaluationAssignment_Heterogeneous:
		if target.Heterogeneous == nil || target.Heterogeneous.GetStack() == nil || target.Heterogeneous.GetStack().GetStackId() == "" {
			return "", fmt.Errorf("evaluation: compute assignment identity: %w", constants.ErrMissingRequiredField)
		}
		parts = append(parts, "heterogeneous", target.Heterogeneous.GetStack().GetStackId())
	default:
		return "", fmt.Errorf("evaluation: compute assignment identity: missing target binding")
	}
	return models.SHA256Hex([]byte(strings.Join(parts, "\x1e"))), nil
}

// ComputeAssignmentResultDigest returns the immutable digest for a terminal
// assignment result with result_digest cleared.
func ComputeAssignmentResultDigest(result *evalv1.EvaluationAssignmentResult) (string, error) {
	if result == nil || result.GetAssignmentId() == "" || result.GetRunId() == "" || result.GetCampaignId() == "" {
		return "", fmt.Errorf("evaluation: compute assignment result digest: %w", constants.ErrMissingRequiredField)
	}
	clone, ok := proto.Clone(result).(*evalv1.EvaluationAssignmentResult)
	if !ok {
		return "", fmt.Errorf("evaluation: compute assignment result digest: invalid clone")
	}
	clone.ResultDigest = ""
	return digestProto(clone)
}

// ValidateAssignmentResultDigest verifies the declared result_digest binding.
func ValidateAssignmentResultDigest(result *evalv1.EvaluationAssignmentResult) error {
	if result == nil {
		return fmt.Errorf("evaluation: validate assignment result digest: %w", constants.ErrMissingRequiredField)
	}
	expected, err := ComputeAssignmentResultDigest(result)
	if err != nil {
		return err
	}
	if result.GetResultDigest() != expected {
		return fmt.Errorf("evaluation: validate assignment result digest: digest mismatch")
	}
	return nil
}

func digestProto(message proto.Message) (string, error) {
	data, err := proto.MarshalOptions{Deterministic: true}.Marshal(message)
	if err != nil {
		return "", fmt.Errorf("evaluation: digest proto: marshal: %w", err)
	}
	return models.SHA256Hex(data), nil
}
