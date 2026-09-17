// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package evaluation

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	evalv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/eval/v1"
)

// CampaignInitRequest carries the frozen campaign inputs for controller startup.
type CampaignInitRequest struct {
	CampaignID                 string
	RunID                      string
	Catalog                    *evalv1.EvaluationScenarioCatalog
	Inventory                  *ModelInventoryFreeze
	ScenarioArtifacts          map[string]ScenarioArtifacts
	RepetitionCount            uint32
	InferenceOperatorSessionID string
	DataOperatorSessionID      string
	Deployment                 *evalv1.EvaluationDeploymentIdentity
	Lane                       evalv1.EvaluationLane
}

// CampaignRunSummary reports resumable controller state for one run.
type CampaignRunSummary struct {
	Run                *evalv1.EvaluationRun
	ExpectedAssignment uint64
	QueuedCount        uint32
	RunningCount       uint32
	TerminalCount      uint32
	NextAssignmentID   string
}

// CampaignStore is the canonical persistence surface used by the campaign controller.
type CampaignStore interface {
	SaveCampaignSpec(ctx context.Context, spec *evalv1.EvaluationCampaignSpec) error
	LoadCampaignSpec(ctx context.Context, campaignID string) (*evalv1.EvaluationCampaignSpec, error)
	SaveScenarioCatalog(ctx context.Context, campaignID string, catalog *evalv1.EvaluationScenarioCatalog) error
	LoadScenarioCatalog(ctx context.Context, campaignID string) (*evalv1.EvaluationScenarioCatalog, error)
	SaveRun(ctx context.Context, run *evalv1.EvaluationRun) error
	LoadRun(ctx context.Context, runID string) (*evalv1.EvaluationRun, error)
	SaveAssignment(ctx context.Context, assignment *evalv1.EvaluationAssignment) error
	LoadAssignment(ctx context.Context, runID, assignmentID string) (*evalv1.EvaluationAssignment, error)
	ListAssignments(ctx context.Context, runID string) ([]*evalv1.EvaluationAssignment, error)
	AssignmentResultExists(ctx context.Context, runID, assignmentID string) (bool, error)
	SaveAssignmentTrace(ctx context.Context, runID, assignmentID string, body []byte) error
	LoadAssignmentTrace(ctx context.Context, runID, assignmentID string) (map[string]any, error)
	SaveAssignmentResult(ctx context.Context, result *evalv1.EvaluationAssignmentResult) error
	LoadAssignmentResult(ctx context.Context, runID, assignmentID string) (*evalv1.EvaluationAssignmentResult, error)
	SaveHeterogeneousStackSet(ctx context.Context, campaignID string, stackSet *HeterogeneousStackSet) error
	LoadHeterogeneousStackSet(ctx context.Context, campaignID string) (*HeterogeneousStackSet, error)
}

// CampaignController owns deterministic scheduling, canonical assignment
// persistence, and resumable execution coordination for North Star model campaigns.
type CampaignController struct {
	store       CampaignStore
	executor    CampaignAssignmentExecutor
	publication *CampaignPublicationCoordinator
	now         func() time.Time
	newID       func(string) string
}

func NewCampaignController(store CampaignStore, executor CampaignAssignmentExecutor, now func() time.Time, newID func(string) string) *CampaignController {
	if now == nil {
		now = time.Now
	}
	if newID == nil {
		newID = func(prefix string) string { return prefix }
	}
	return &CampaignController{store: store, executor: executor, now: now, newID: newID}
}

// WithPublication attaches an optional publication coordinator used to emit
// typed public lifecycle projections after canonical state is persisted.
func (c *CampaignController) WithPublication(publication *CampaignPublicationCoordinator) *CampaignController {
	if c != nil {
		c.publication = publication
	}
	return c
}

// InitializeCampaign persists the frozen campaign spec, catalog, and run record.
func (c *CampaignController) InitializeCampaign(ctx context.Context, req CampaignInitRequest) (*evalv1.EvaluationRun, error) {
	if c == nil || c.store == nil {
		return nil, fmt.Errorf("evaluation: initialize campaign: %w", constants.ErrMissingRequiredField)
	}
	if req.CampaignID == "" || req.RunID == "" || req.Catalog == nil || req.Inventory == nil {
		return nil, fmt.Errorf("evaluation: initialize campaign: %w", constants.ErrMissingRequiredField)
	}
	spec, err := MaterializeNorthStarCampaignSpec(req.CampaignID, req.Catalog, req.Inventory, req.RepetitionCount)
	if err != nil {
		return nil, err
	}
	if err := c.store.SaveCampaignSpec(ctx, spec); err != nil {
		return nil, err
	}
	if err := c.store.SaveScenarioCatalog(ctx, req.CampaignID, req.Catalog); err != nil {
		return nil, err
	}
	lane := req.Lane
	if lane == evalv1.EvaluationLane_EVALUATION_LANE_UNSPECIFIED {
		lane = evalv1.EvaluationLane_EVALUATION_LANE_MODEL_ROLE
	}
	run := &evalv1.EvaluationRun{
		SchemaVersion: CampaignSchemaVersion,
		RunId:         req.RunID,
		SuiteRef:      req.Catalog.GetCatalogRef(),
		Deployment:    req.Deployment,
		ActivePosture: spec.GetGovernancePosture(),
		Lane:          lane,
		StartedAt:     timestamppb.New(c.now().UTC()),
		CampaignBinding: &evalv1.ModelCampaignBinding{
			CampaignId:                 req.CampaignID,
			CampaignDigest:             spec.GetCampaignDigest(),
			CatalogRef:                 req.Catalog.GetCatalogRef(),
			CatalogDigest:              req.Catalog.GetCatalogDigest(),
			ModelRegistryDigest:        req.Inventory.RegistryDigest,
			InferenceOperatorSessionId: req.InferenceOperatorSessionID,
			DataOperatorSessionId:      req.DataOperatorSessionID,
		},
	}
	if err := c.store.SaveRun(ctx, run); err != nil {
		return nil, err
	}
	return run, nil
}

// ScheduleHomogeneousRun materializes and persists the complete queued assignment
// matrix before any scored call begins.
func (c *CampaignController) ScheduleHomogeneousRun(ctx context.Context, runID string) (int, error) {
	if c == nil || c.store == nil {
		return 0, fmt.Errorf("evaluation: schedule homogeneous run: %w", constants.ErrMissingRequiredField)
	}
	run, err := c.store.LoadRun(ctx, runID)
	if err != nil {
		return 0, err
	}
	spec, err := c.store.LoadCampaignSpec(ctx, run.GetCampaignBinding().GetCampaignId())
	if err != nil {
		return 0, err
	}
	catalog, err := c.store.LoadScenarioCatalog(ctx, run.GetCampaignBinding().GetCampaignId())
	if err != nil {
		return 0, err
	}
	assignments, err := BuildHomogeneousAssignmentMatrix(HomogeneousScheduleRequest{
		CampaignID:      spec.GetCampaignId(),
		RunID:           runID,
		Catalog:         catalog,
		Variants:        spec.GetModelRegistry(),
		RepetitionCount: spec.GetRepetitionCount(),
		QueuedAt:        c.now().UTC(),
	})
	if err != nil {
		return 0, err
	}
	inventory := &ModelInventoryFreeze{
		CampaignID:           spec.GetCampaignId(),
		RegistryDigest:       spec.GetModelRegistryDigest(),
		Variants:             spec.GetModelRegistry(),
		HomogeneousCellCount: ComputeNorthStarHomogeneousMatrixSize(uint64(len(spec.GetModelRegistry()))) * uint64(spec.GetRepetitionCount()),
	}
	if err := ValidateHomogeneousAssignmentMatrix(catalog, inventory, spec.GetRepetitionCount(), assignments); err != nil {
		return 0, err
	}
	existing, err := c.store.ListAssignments(ctx, runID)
	if err != nil {
		return 0, err
	}
	if len(existing) > 0 {
		return 0, fmt.Errorf("evaluation: schedule homogeneous run: assignments already materialized for run %s", runID)
	}
	for _, assignment := range assignments {
		if err := c.store.SaveAssignment(ctx, assignment); err != nil {
			return 0, err
		}
	}
	if err := c.publishQueuedAssignments(ctx, runID, assignments); err != nil {
		return 0, err
	}
	return len(assignments), nil
}

// GenerateHeterogeneousStackSet materializes and persists the preregistered
// heterogeneous stack set for one frozen campaign registry.
func (c *CampaignController) GenerateHeterogeneousStackSet(ctx context.Context, campaignID string, seed uint64) (*HeterogeneousStackSet, error) {
	if c == nil || c.store == nil {
		return nil, fmt.Errorf("evaluation: generate heterogeneous stack set: %w", constants.ErrMissingRequiredField)
	}
	spec, err := c.store.LoadCampaignSpec(ctx, campaignID)
	if err != nil {
		return nil, err
	}
	stackSet, err := GenerateNorthStarHeterogeneousStackSet(HeterogeneousStackGenerationRequest{
		CampaignID: campaignID,
		Seed:       seed,
		Variants:   spec.GetModelRegistry(),
	})
	if err != nil {
		return nil, err
	}
	if err := c.store.SaveHeterogeneousStackSet(ctx, campaignID, stackSet); err != nil {
		return nil, err
	}
	return stackSet, nil
}

// ScheduleHeterogeneousRun materializes and persists the complete heterogeneous
// system-lane assignment matrix before any scored call begins.
func (c *CampaignController) ScheduleHeterogeneousRun(ctx context.Context, runID string) (int, error) {
	if c == nil || c.store == nil {
		return 0, fmt.Errorf("evaluation: schedule heterogeneous run: %w", constants.ErrMissingRequiredField)
	}
	run, err := c.store.LoadRun(ctx, runID)
	if err != nil {
		return 0, err
	}
	if run.GetLane() != evalv1.EvaluationLane_EVALUATION_LANE_SYSTEM {
		return 0, fmt.Errorf("evaluation: schedule heterogeneous run: run %s is not a system lane run", runID)
	}
	spec, err := c.store.LoadCampaignSpec(ctx, run.GetCampaignBinding().GetCampaignId())
	if err != nil {
		return 0, err
	}
	catalog, err := c.store.LoadScenarioCatalog(ctx, run.GetCampaignBinding().GetCampaignId())
	if err != nil {
		return 0, err
	}
	stackSet, err := c.store.LoadHeterogeneousStackSet(ctx, spec.GetCampaignId())
	if err != nil {
		return 0, err
	}
	assignments, err := BuildHeterogeneousAssignmentMatrix(HeterogeneousScheduleRequest{
		CampaignID: spec.GetCampaignId(),
		RunID:      runID,
		Catalog:    catalog,
		StackSet:   stackSet,
		QueuedAt:   c.now().UTC(),
	})
	if err != nil {
		return 0, err
	}
	if err := ValidateHeterogeneousAssignmentMatrix(catalog, stackSet, assignments); err != nil {
		return 0, err
	}
	existing, err := c.store.ListAssignments(ctx, runID)
	if err != nil {
		return 0, err
	}
	if len(existing) > 0 {
		return 0, fmt.Errorf("evaluation: schedule heterogeneous run: assignments already materialized for run %s", runID)
	}
	for _, assignment := range assignments {
		if err := c.store.SaveAssignment(ctx, assignment); err != nil {
			return 0, err
		}
	}
	if err := c.publishQueuedAssignments(ctx, runID, assignments); err != nil {
		return 0, err
	}
	return len(assignments), nil
}

// RunSummary returns resumable controller state derived from canonical records.
func (c *CampaignController) RunSummary(ctx context.Context, runID string) (*CampaignRunSummary, error) {
	if c == nil || c.store == nil {
		return nil, fmt.Errorf("evaluation: campaign run summary: %w", constants.ErrMissingRequiredField)
	}
	run, err := c.store.LoadRun(ctx, runID)
	if err != nil {
		return nil, err
	}
	assignments, err := c.store.ListAssignments(ctx, runID)
	if err != nil {
		return nil, err
	}
	spec, err := c.store.LoadCampaignSpec(ctx, run.GetCampaignBinding().GetCampaignId())
	if err != nil {
		return nil, err
	}
	repetition := spec.GetRepetitionCount()
	if repetition == 0 {
		repetition = 1
	}
	var expected uint64
	switch run.GetLane() {
	case evalv1.EvaluationLane_EVALUATION_LANE_SYSTEM:
		stackSet, err := c.store.LoadHeterogeneousStackSet(ctx, run.GetCampaignBinding().GetCampaignId())
		if err != nil {
			return nil, err
		}
		expected = ComputeNorthStarHeterogeneousMatrixSize(uint64(len(stackSet.Stacks)))
	default:
		expected = ComputeNorthStarHomogeneousMatrixSize(uint64(len(spec.GetModelRegistry()))) * uint64(repetition)
	}
	summary := &CampaignRunSummary{
		Run:                run,
		ExpectedAssignment: expected,
	}
	var nextID string
	for _, assignment := range assignments {
		terminal, err := c.store.AssignmentResultExists(ctx, runID, assignment.GetAssignmentId())
		if err != nil {
			return nil, err
		}
		if terminal {
			summary.TerminalCount++
			continue
		}
		switch assignment.GetLifecycleStatus() {
		case evalv1.EvaluationAssignmentLifecycleStatus_EVALUATION_ASSIGNMENT_LIFECYCLE_STATUS_QUEUED:
			summary.QueuedCount++
			if nextID == "" {
				nextID = assignment.GetAssignmentId()
			}
		case evalv1.EvaluationAssignmentLifecycleStatus_EVALUATION_ASSIGNMENT_LIFECYCLE_STATUS_RUNNING:
			summary.RunningCount++
			if nextID == "" {
				nextID = assignment.GetAssignmentId()
			}
		default:
			summary.TerminalCount++
		}
	}
	summary.NextAssignmentID = nextID
	return summary, nil
}

// ResumeNextAssignment returns the next queued or interrupted assignment that
// lacks a terminal persisted result. Completed assignments are never re-selected.
func (c *CampaignController) ResumeNextAssignment(ctx context.Context, runID string) (*evalv1.EvaluationAssignment, bool, error) {
	if c == nil || c.store == nil {
		return nil, false, fmt.Errorf("evaluation: resume next assignment: %w", constants.ErrMissingRequiredField)
	}
	assignments, err := c.store.ListAssignments(ctx, runID)
	if err != nil {
		return nil, false, err
	}
	for _, assignment := range assignments {
		exists, err := c.store.AssignmentResultExists(ctx, runID, assignment.GetAssignmentId())
		if err != nil {
			return nil, false, err
		}
		if exists {
			continue
		}
		switch assignment.GetLifecycleStatus() {
		case evalv1.EvaluationAssignmentLifecycleStatus_EVALUATION_ASSIGNMENT_LIFECYCLE_STATUS_QUEUED,
			evalv1.EvaluationAssignmentLifecycleStatus_EVALUATION_ASSIGNMENT_LIFECYCLE_STATUS_RUNNING:
			return assignment, true, nil
		}
	}
	return nil, false, nil
}

// ExecuteNextAssignment resumes one assignment through the configured executor,
// persists the terminal result, and updates assignment lifecycle state.
func (c *CampaignController) ExecuteNextAssignment(ctx context.Context, runID string, binding CampaignExecutionBinding, artifacts map[string]ScenarioArtifacts) (*evalv1.EvaluationAssignmentResult, bool, error) {
	if c == nil || c.executor == nil {
		return nil, false, fmt.Errorf("evaluation: execute next assignment: executor is required")
	}
	assignment, ok, err := c.ResumeNextAssignment(ctx, runID)
	if err != nil || !ok {
		return nil, ok, err
	}
	if exists, err := c.store.AssignmentResultExists(ctx, runID, assignment.GetAssignmentId()); err != nil {
		return nil, false, err
	} else if exists {
		return nil, false, nil
	}
	assignment.LifecycleStatus = evalv1.EvaluationAssignmentLifecycleStatus_EVALUATION_ASSIGNMENT_LIFECYCLE_STATUS_RUNNING
	assignment.StartedAt = timestamppb.New(c.now().UTC())
	if err := c.store.SaveAssignment(ctx, assignment); err != nil {
		return nil, false, err
	}
	if err := c.publishAssignmentLifecycle(ctx, assignment); err != nil {
		return nil, false, err
	}
	artifact, found := artifacts[assignment.GetScenarioId()]
	if !found {
		return nil, false, fmt.Errorf("evaluation: execute next assignment: missing scenario artifacts for %s", assignment.GetScenarioId())
	}
	var scenarioInput ScenarioInputFixture
	if err := json.Unmarshal(artifact.Input.Body, &scenarioInput); err != nil {
		return nil, false, fmt.Errorf("evaluation: execute next assignment: decode scenario input: %w", err)
	}
	var scenarioGold ScenarioGoldCriteria
	if err := json.Unmarshal(artifact.Gold.Body, &scenarioGold); err != nil {
		return nil, false, fmt.Errorf("evaluation: execute next assignment: decode scenario gold: %w", err)
	}
	run, err := c.store.LoadRun(ctx, runID)
	if err != nil {
		return nil, false, err
	}
	catalog, err := c.store.LoadScenarioCatalog(ctx, run.GetCampaignBinding().GetCampaignId())
	if err != nil {
		return nil, false, err
	}
	gradingMethod, err := scenarioGradingMethodForAssignment(catalog, assignment)
	if err != nil {
		return nil, false, err
	}
	scenarioTools, err := scenarioToolsForAssignment(catalog, assignment)
	if err != nil {
		return nil, false, err
	}
	requiredConcepts, err := scenarioRequiredConceptsForAssignment(catalog, assignment)
	if err != nil {
		return nil, false, err
	}
	attemptID := c.newID("attempt")
	execReq := AssignmentExecutionRequest{
		Assignment:       assignment,
		AttemptID:        attemptID,
		ScenarioInput:    scenarioInput,
		ScenarioGold:     scenarioGold,
		ScenarioTools:    scenarioTools,
		RequiredConcepts: requiredConcepts,
		GradingMethod:    gradingMethod,
		Binding:          binding,
	}
	result, err := c.executor.ExecuteAssignment(ctx, execReq)
	if err != nil {
		if !isRecoverableAssignmentExecutionError(err) {
			return nil, false, fmt.Errorf("evaluation: execute next assignment: %w", err)
		}
		failureResult, buildErr := c.buildFailureAssignmentResult(ctx, execReq, err)
		if buildErr != nil {
			return nil, false, fmt.Errorf("evaluation: execute next assignment: %w", buildErr)
		}
		if err := c.persistTerminalAssignment(ctx, assignment, failureResult); err != nil {
			return nil, false, err
		}
		return failureResult, true, nil
	}
	if result == nil {
		return nil, false, fmt.Errorf("evaluation: execute next assignment: executor returned nil result")
	}
	if err := c.persistTerminalAssignment(ctx, assignment, result); err != nil {
		return nil, false, err
	}
	return result, true, nil
}

func scenarioGradingMethodForAssignment(catalog *evalv1.EvaluationScenarioCatalog, assignment *evalv1.EvaluationAssignment) (evalv1.EvaluationGradingMethod, error) {
	if catalog == nil || assignment == nil || assignment.GetScenarioId() == "" {
		return evalv1.EvaluationGradingMethod_EVALUATION_GRADING_METHOD_UNSPECIFIED, fmt.Errorf("evaluation: scenario grading method lookup: %w", constants.ErrMissingRequiredField)
	}
	for _, scenario := range catalog.GetScenarios() {
		if scenario.GetScenarioId() == assignment.GetScenarioId() {
			return scenario.GetGradingMethod(), nil
		}
	}
	return evalv1.EvaluationGradingMethod_EVALUATION_GRADING_METHOD_UNSPECIFIED, fmt.Errorf("evaluation: scenario grading method lookup: unknown scenario %s", assignment.GetScenarioId())
}

func scenarioRequiredConceptsForAssignment(catalog *evalv1.EvaluationScenarioCatalog, assignment *evalv1.EvaluationAssignment) ([]string, error) {
	if catalog == nil || assignment == nil || assignment.GetScenarioId() == "" {
		return nil, fmt.Errorf("evaluation: scenario concept lookup: %w", constants.ErrMissingRequiredField)
	}
	for _, scenario := range catalog.GetScenarios() {
		if scenario.GetScenarioId() == assignment.GetScenarioId() {
			return append([]string(nil), scenario.GetRequiredConcepts()...), nil
		}
	}
	return nil, fmt.Errorf("evaluation: scenario concept lookup: unknown scenario %s", assignment.GetScenarioId())
}

func scenarioToolsForAssignment(catalog *evalv1.EvaluationScenarioCatalog, assignment *evalv1.EvaluationAssignment) (ScenarioToolExpectations, error) {
	if catalog == nil || assignment == nil || assignment.GetScenarioId() == "" {
		return ScenarioToolExpectations{}, fmt.Errorf("evaluation: scenario tool lookup: %w", constants.ErrMissingRequiredField)
	}
	for _, scenario := range catalog.GetScenarios() {
		if scenario.GetScenarioId() == assignment.GetScenarioId() {
			return ScenarioToolExpectations{
				ExpectedTools:  append([]string(nil), scenario.GetExpectedTools()...),
				ForbiddenTools: append([]string(nil), scenario.GetForbiddenTools()...),
			}, nil
		}
	}
	return ScenarioToolExpectations{}, fmt.Errorf("evaluation: scenario tool lookup: unknown scenario %s", assignment.GetScenarioId())
}

func (c *CampaignController) publishQueuedAssignments(ctx context.Context, runID string, assignments []*evalv1.EvaluationAssignment) error {
	if c == nil || c.publication == nil {
		return nil
	}
	run, err := c.store.LoadRun(ctx, runID)
	if err != nil {
		return err
	}
	catalog, err := c.store.LoadScenarioCatalog(ctx, run.GetCampaignBinding().GetCampaignId())
	if err != nil {
		return err
	}
	for _, assignment := range assignments {
		category, err := ScenarioCategoryForAssignment(catalog, assignment)
		if err != nil {
			return err
		}
		if err := c.publication.PublishAssignmentLifecycle(ctx, assignment, category, assignmentLifecycleObservedAt(assignment)); err != nil {
			return err
		}
	}
	_, err = c.publication.PublishRunAggregates(ctx, runID, c.now().UTC())
	return err
}

func (c *CampaignController) publishAssignmentLifecycle(ctx context.Context, assignment *evalv1.EvaluationAssignment) error {
	if c == nil || c.publication == nil || assignment == nil {
		return nil
	}
	run, err := c.store.LoadRun(ctx, assignment.GetRunId())
	if err != nil {
		return err
	}
	catalog, err := c.store.LoadScenarioCatalog(ctx, run.GetCampaignBinding().GetCampaignId())
	if err != nil {
		return err
	}
	category, err := ScenarioCategoryForAssignment(catalog, assignment)
	if err != nil {
		return err
	}
	return c.publication.PublishAssignmentLifecycle(ctx, assignment, category, assignmentLifecycleObservedAt(assignment))
}

func (c *CampaignController) publishAssignmentTerminal(ctx context.Context, assignment *evalv1.EvaluationAssignment, result *evalv1.EvaluationAssignmentResult) error {
	if c == nil || c.publication == nil || assignment == nil || result == nil {
		return nil
	}
	if err := c.publishAssignmentLifecycle(ctx, assignment); err != nil {
		return err
	}
	run, err := c.store.LoadRun(ctx, assignment.GetRunId())
	if err != nil {
		return err
	}
	catalog, err := c.store.LoadScenarioCatalog(ctx, run.GetCampaignBinding().GetCampaignId())
	if err != nil {
		return err
	}
	category, err := ScenarioCategoryForAssignment(catalog, assignment)
	if err != nil {
		return err
	}
	if err := c.publication.PublishAssignmentResult(ctx, assignment, result, category, "unverified"); err != nil {
		return err
	}
	_, err = c.publication.PublishRunAggregates(ctx, assignment.GetRunId(), c.now().UTC())
	return err
}
