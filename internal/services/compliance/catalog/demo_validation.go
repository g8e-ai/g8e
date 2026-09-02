// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package catalog

import (
	"fmt"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	compliancev1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/compliance/v1"
)

var demoTerminalStatuses = []string{"passed", "failed", "skipped", "cancelled", "timed_out", "unverifiable"}
var demoExpectedOutcomes = []string{"allowed", "blocked"}
var demoFailurePolicies = []string{"fail_closed"}

func ValidateDemoManifest(manifest *compliancev1.DemoManifest, definitions []*compliancev1.DemoScenarioDefinition, frameworks *compliancev1.FrameworkCatalog) error {
	if manifest == nil || manifest.DemoId == "" || manifest.DemoVersion == "" || manifest.RunId == "" || manifest.ScopeId == "" || manifest.GeneratedAt == nil || manifest.GeneratedAt.CheckValid() != nil || len(manifest.ScenarioDefinitionRefs) == 0 || len(manifest.ProvenanceHashes) == 0 || len(manifest.RequiredEnvironment) == 0 || len(manifest.SupportedLanes) == 0 {
		return fmt.Errorf("%w: demo manifest is incomplete", constants.ErrInvalidEvidenceGraph)
	}
	if err := validateVersionedReferences(manifest.ScenarioDefinitionRefs); err != nil {
		return fmt.Errorf("%w: demo manifest scenario references: %v", constants.ErrInvalidEvidenceGraph, err)
	}
	definitionSet := make(map[string]struct{}, len(definitions))
	for _, definition := range definitions {
		if definition != nil {
			definitionSet[versionedKey(definition.ScenarioId, definition.ScenarioVersion)] = struct{}{}
		}
	}
	for _, reference := range manifest.ScenarioDefinitionRefs {
		if _, exists := definitionSet[referenceKey(reference)]; !exists {
			return fmt.Errorf("%w: demo scenario %s", constants.ErrUnresolvedReference, referenceKey(reference))
		}
	}
	if err := validateNamedDigests(manifest.ProvenanceHashes); err != nil {
		return err
	}
	if err := validateUniqueStrings(manifest.RequiredEnvironment); err != nil {
		return fmt.Errorf("%w: demo manifest environment: %v", constants.ErrInvalidEvidenceGraph, err)
	}
	if err := validateUniqueStrings(manifest.SupportedLanes); err != nil {
		return fmt.Errorf("%w: demo manifest lanes: %v", constants.ErrInvalidEvidenceGraph, err)
	}
	return validateFrameworkControlReferences(manifest.FrameworkControlRefs, frameworks)
}

func ValidateDemoScenarioDefinition(definition *compliancev1.DemoScenarioDefinition, assertions *compliancev1.ControlAssertionCatalog, frameworks *compliancev1.FrameworkCatalog) error {
	if definition == nil || definition.ScenarioId == "" || definition.ScenarioVersion == "" || definition.DisplayNumber == "" || definition.Title == "" || definition.Purpose == "" || definition.RiskCategory == "" || len(definition.ExpectedActionClasses) == 0 || definition.ExpectedOutcome == "" || definition.InitialStateFixtureRef == "" || len(definition.TerminalStateAssertions) == 0 || len(definition.RequiredReceipts) == 0 || len(definition.RequiredDeterministicStages) == 0 || len(definition.AssertionRefs) == 0 || len(definition.FrameworkControlRefs) == 0 || len(definition.RequiredEvidenceTypes) == 0 || definition.RequiredEvidenceLevel == "" || definition.TimeoutSeconds == 0 || definition.FailurePolicy == "" || definition.HarnessScenario == "" {
		return fmt.Errorf("%w: demo scenario definition is incomplete", constants.ErrInvalidEvidenceGraph)
	}
	if !contains(demoExpectedOutcomes, definition.ExpectedOutcome) || !contains(evidenceLevels, definition.RequiredEvidenceLevel) || !contains(demoFailurePolicies, definition.FailurePolicy) {
		return fmt.Errorf("%w: demo scenario %s has invalid semantics", constants.ErrInvalidEvidenceGraph, definition.ScenarioId)
	}
	if definition.ExpectedOutcome == "blocked" && definition.ExpectedRejectionLayer == "" {
		return fmt.Errorf("%w: blocked demo scenario requires a rejection layer", constants.ErrInvalidEvidenceGraph)
	}
	for _, values := range [][]string{definition.ExpectedActionClasses, definition.TerminalStateAssertions, definition.RequiredReceipts, definition.RequiredDeterministicStages, definition.RequiredEvidenceTypes} {
		if err := validateUniqueStrings(values); err != nil {
			return fmt.Errorf("%w: demo scenario %s requirements: %v", constants.ErrInvalidEvidenceGraph, definition.ScenarioId, err)
		}
	}
	if err := validateVersionedReferences(definition.AssertionRefs); err != nil {
		return fmt.Errorf("%w: demo scenario %s assertion references: %v", constants.ErrInvalidEvidenceGraph, definition.ScenarioId, err)
	}
	for _, reference := range definition.AssertionRefs {
		if FindAssertion(assertions, reference.Id, reference.Version) == nil {
			return fmt.Errorf("%w: %s", constants.ErrUnsupportedAssertion, referenceKey(reference))
		}
	}
	return validateFrameworkControlReferences(definition.FrameworkControlRefs, frameworks)
}

func ValidateDemoStepResult(step *compliancev1.DemoStepResult) error {
	if step == nil || step.StepId == "" || step.Operation == "" || step.StartedAt == nil || step.CompletedAt == nil || step.StartedAt.CheckValid() != nil || step.CompletedAt.CheckValid() != nil || step.CompletedAt.AsTime().Before(step.StartedAt.AsTime()) || !contains(demoTerminalStatuses, step.Status) {
		return fmt.Errorf("%w: demo step result is incomplete or invalid", constants.ErrInvalidEvidenceGraph)
	}
	if err := validateUniqueStrings(step.EvidenceRefs); err != nil {
		return fmt.Errorf("%w: demo step %s evidence references: %v", constants.ErrInvalidEvidenceGraph, step.StepId, err)
	}
	if step.Status == "passed" && step.Failure != "" {
		return fmt.Errorf("%w: passed demo step %s carries a failure", constants.ErrInvalidEvidenceGraph, step.StepId)
	}
	if step.Status != "passed" && step.Failure == "" {
		return fmt.Errorf("%w: non-passing demo step %s requires a failure", constants.ErrInvalidEvidenceGraph, step.StepId)
	}
	return nil
}

func ValidateDemoScenarioResult(result *compliancev1.DemoScenarioResult, definition *compliancev1.DemoScenarioDefinition, scopeID string) error {
	if result == nil || result.ResultId == "" || result.ScenarioRef == nil || result.DemoId == "" || result.ScopeId == "" || result.RunId == "" || result.StartedAt == nil || result.CompletedAt == nil || result.StartedAt.CheckValid() != nil || result.CompletedAt.CheckValid() != nil || result.CompletedAt.AsTime().Before(result.StartedAt.AsTime()) || result.DisplayNumber == "" || result.Title == "" || !contains(demoTerminalStatuses, result.Status) {
		return fmt.Errorf("%w: demo scenario result is incomplete or invalid", constants.ErrInvalidEvidenceGraph)
	}
	if result.ScopeId != scopeID {
		return fmt.Errorf("%w: demo result %s belongs to scope %s", constants.ErrEvidenceScopeMismatch, result.ResultId, result.ScopeId)
	}
	if definition == nil || result.ScenarioRef.Id != definition.ScenarioId || result.ScenarioRef.Version != definition.ScenarioVersion {
		return fmt.Errorf("%w: demo scenario %s", constants.ErrUnresolvedReference, referenceKey(result.ScenarioRef))
	}
	for _, values := range [][]string{result.InvestigationIds, result.ExecutionIds, result.TransactionIds, result.ReceiptRefs, result.StateObservationRefs, result.MetricRefs, result.KsiRefs} {
		if err := validateUniqueStrings(values); err != nil {
			return fmt.Errorf("%w: demo result %s references: %v", constants.ErrInvalidEvidenceGraph, result.ResultId, err)
		}
	}
	if err := validateVersionedReferences(result.AssertionRefs); err != nil {
		return fmt.Errorf("%w: demo result %s assertion references: %v", constants.ErrInvalidEvidenceGraph, result.ResultId, err)
	}
	if !containsAllVersionedReferences(result.AssertionRefs, definition.AssertionRefs) || !containsAllFrameworkControlReferences(result.FrameworkControlRefs, definition.FrameworkControlRefs) {
		return fmt.Errorf("%w: demo result %s omits required control references", constants.ErrUnresolvedReference, result.ResultId)
	}
	for _, step := range result.StepResults {
		if err := ValidateDemoStepResult(step); err != nil {
			return err
		}
		if result.Status == "passed" && step.Required && step.Status != "passed" {
			return fmt.Errorf("%w: passed demo result %s has a non-passing required step", constants.ErrInvalidEvidenceGraph, result.ResultId)
		}
	}
	if result.Status == "passed" {
		if result.VerificationStatus != "verified" || result.Failure != "" || len(result.StepResults) == 0 || len(result.TransactionIds) == 0 {
			return fmt.Errorf("%w: passed demo result %s lacks verified execution evidence", constants.ErrInvalidEvidenceGraph, result.ResultId)
		}
		for _, evidenceType := range definition.RequiredEvidenceTypes {
			switch evidenceType {
			case "action_receipt":
				if len(result.ReceiptRefs) == 0 {
					return fmt.Errorf("%w: passed demo result %s lacks receipt evidence", constants.ErrInvalidEvidenceGraph, result.ResultId)
				}
			case "state_observation":
				if len(result.StateObservationRefs) == 0 {
					return fmt.Errorf("%w: passed demo result %s lacks state-observation evidence", constants.ErrInvalidEvidenceGraph, result.ResultId)
				}
			case "grader_metric":
				if len(result.MetricRefs) == 0 {
					return fmt.Errorf("%w: passed demo result %s lacks metric evidence", constants.ErrInvalidEvidenceGraph, result.ResultId)
				}
			}
		}
	} else if result.Failure == "" {
		return fmt.Errorf("%w: non-passing demo result %s requires a failure", constants.ErrInvalidEvidenceGraph, result.ResultId)
	}
	return nil
}

func validateFrameworkControlReferences(references []*compliancev1.FrameworkControlReference, frameworks *compliancev1.FrameworkCatalog) error {
	seen := make(map[string]struct{}, len(references))
	for _, reference := range references {
		if reference == nil || reference.FrameworkRef == nil || reference.ControlId == "" {
			return fmt.Errorf("%w: framework control reference is incomplete", constants.ErrInvalidEvidenceGraph)
		}
		framework := FindFramework(frameworks, reference.FrameworkRef.Id, reference.FrameworkRef.Version)
		if framework == nil {
			return fmt.Errorf("%w: %s", constants.ErrUnsupportedFramework, referenceKey(reference.FrameworkRef))
		}
		if FindFrameworkControl(framework, reference.ControlId) == nil {
			return fmt.Errorf("%w: framework control %s", constants.ErrUnresolvedReference, reference.ControlId)
		}
		key := referenceKey(reference.FrameworkRef) + ":" + reference.ControlId
		if _, exists := seen[key]; exists {
			return fmt.Errorf("%w: duplicate framework control reference %s", constants.ErrInvalidEvidenceGraph, key)
		}
		seen[key] = struct{}{}
	}
	return nil
}

func containsAllVersionedReferences(actual, required []*compliancev1.VersionedReference) bool {
	actualSet := make(map[string]struct{}, len(actual))
	for _, reference := range actual {
		actualSet[referenceKey(reference)] = struct{}{}
	}
	for _, reference := range required {
		if _, exists := actualSet[referenceKey(reference)]; !exists {
			return false
		}
	}
	return true
}

func containsAllFrameworkControlReferences(actual, required []*compliancev1.FrameworkControlReference) bool {
	actualSet := make(map[string]struct{}, len(actual))
	for _, reference := range actual {
		if reference != nil {
			actualSet[referenceKey(reference.FrameworkRef)+":"+reference.ControlId] = struct{}{}
		}
	}
	for _, reference := range required {
		if reference == nil {
			return false
		}
		if _, exists := actualSet[referenceKey(reference.FrameworkRef)+":"+reference.ControlId]; !exists {
			return false
		}
	}
	return true
}
