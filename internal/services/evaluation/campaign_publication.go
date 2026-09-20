// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package evaluation

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/services/fs"
	evalv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/eval/v1"
	"google.golang.org/protobuf/proto"
)

// CampaignPublicFeedRecord is one append-only public projection payload ready
// for signing by the host publisher.
type CampaignPublicFeedRecord struct {
	Sequence    int64
	RecordHash  string
	RecordBytes string
}

// CampaignFeedExporter publishes signed public feed batches. The publisher
// remains the sole sequence owner.
type CampaignFeedExporter interface {
	HighWaterSequence(ctx context.Context) (int64, error)
	ExportBatch(ctx context.Context, records []CampaignPublicFeedRecord) error
}

type campaignFeedPublishRequest struct {
	IdempotencyKey string
	Body           []byte
}

// CampaignPublicationCoordinator projects canonical campaign state into typed
// public records and coordinates idempotent publisher export.
type CampaignPublicationCoordinator struct {
	store             CampaignStore
	files             fs.RuntimeFileService
	publicationState  CampaignPublicationStateStore
	exporter          CampaignFeedExporter
	observationRemote ProviderObservationRemote
	mirrorProbe       CampaignMirrorProbe
}

func NewCampaignPublicationCoordinator(store CampaignStore, files fs.RuntimeFileService, publicationState CampaignPublicationStateStore, exporter CampaignFeedExporter, observationRemote ProviderObservationRemote) *CampaignPublicationCoordinator {
	return &CampaignPublicationCoordinator{
		store:             store,
		files:             files,
		publicationState:  publicationState,
		exporter:          exporter,
		observationRemote: observationRemote,
	}
}

// WithMirrorProbe enables drift-aware catch-up when the gateway mirror volume
// was wiped but host publication idempotency state remains.
func (c *CampaignPublicationCoordinator) WithMirrorProbe(probe CampaignMirrorProbe) *CampaignPublicationCoordinator {
	if c != nil {
		c.mirrorProbe = probe
	}
	return c
}

// PublishAssignmentLifecycle emits one lifecycle projection when it has not yet
// been published for the run.
func (c *CampaignPublicationCoordinator) PublishAssignmentLifecycle(ctx context.Context, assignment *evalv1.EvaluationAssignment, scenarioCategory evalv1.EvaluationScenarioCategory, observedAt time.Time) error {
	if c == nil || c.store == nil || c.files == nil || c.exporter == nil || assignment == nil {
		return fmt.Errorf("evaluation: publish assignment lifecycle: %w", constants.ErrMissingRequiredField)
	}
	projection, err := BuildAssignmentLifecycleProjection(assignment, scenarioCategory, observedAt)
	if err != nil {
		return err
	}
	idempotencyKey := AssignmentLifecycleIdempotencyKey(assignment.GetRunId(), assignment.GetAssignmentId(), assignment.GetLifecycleStatus())
	return c.publishEnvelope(ctx, assignment.GetRunId(), idempotencyKey, publicMessageTypeAssignmentLifecycle, projection)
}

// PublishAssignmentResult emits one terminal result projection when it has not
// yet been published for the run.
func (c *CampaignPublicationCoordinator) PublishAssignmentResult(ctx context.Context, assignment *evalv1.EvaluationAssignment, result *evalv1.EvaluationAssignmentResult, scenarioCategory evalv1.EvaluationScenarioCategory, verificationStatus string) error {
	if c == nil || c.store == nil || c.files == nil || c.exporter == nil || assignment == nil || result == nil {
		return fmt.Errorf("evaluation: publish assignment result: %w", constants.ErrMissingRequiredField)
	}
	idempotencyKey := AssignmentResultIdempotencyKey(assignment.GetRunId(), assignment.GetAssignmentId())
	return c.publishAssignmentResultWithKey(ctx, assignment, result, scenarioCategory, verificationStatus, idempotencyKey)
}

func (c *CampaignPublicationCoordinator) publishAssignmentResultWithKey(
	ctx context.Context,
	assignment *evalv1.EvaluationAssignment,
	result *evalv1.EvaluationAssignmentResult,
	scenarioCategory evalv1.EvaluationScenarioCategory,
	verificationStatus string,
	idempotencyKey string,
) error {
	if c == nil || c.store == nil || c.files == nil || c.exporter == nil || assignment == nil || result == nil || idempotencyKey == "" {
		return fmt.Errorf("evaluation: publish assignment result: %w", constants.ErrMissingRequiredField)
	}
	run, err := c.store.LoadRun(ctx, assignment.GetRunId())
	if err != nil {
		return fmt.Errorf("evaluation: publish assignment result: load run: %w", err)
	}
	catalog, err := c.store.LoadScenarioCatalog(ctx, run.GetCampaignBinding().GetCampaignId())
	if err != nil {
		return err
	}
	_, artifacts, err := LoadScenarioCatalog()
	if err != nil {
		return fmt.Errorf("evaluation: publish assignment result: load scenario artifacts: %w", err)
	}
	store, ok := c.store.(*Store)
	if !ok {
		return fmt.Errorf("evaluation: publish assignment result: resolve scenario context: %w", constants.ErrEvidenceScopeMismatch)
	}
	scenario, scenarioErr := ResolvePublicScenarioContext(ctx, store, run, catalog, assignment, artifacts)
	if scenarioErr != nil && !errors.Is(scenarioErr, constants.ErrEvidenceArtifactMalformed) {
		return scenarioErr
	}
	benchmark, err := c.buildAssignmentBenchmarkObservations(ctx, result)
	if err != nil {
		return err
	}
	resources, err := BuildPublicResourceSummary(result)
	if err != nil {
		return err
	}
	var record *PublicAssignmentRecord
	if scenarioErr == nil {
		record, err = BuildPublicAssignmentProjection(ctx, PublicAssignmentBuildInput{
			Assignment:         assignment,
			Result:             result,
			ScenarioContext:    scenario,
			VerificationStatus: verificationStatus,
			Extensions:         PublicAssignmentRecordExtensions{BenchmarkObservations: benchmark, ResourceSummary: resources},
		})
	} else {
		projection, projectionErr := BuildAssignmentResultProjection(assignment, result, scenarioCategory, DerivePublicSummaryStatus(result), verificationStatus)
		if projectionErr != nil {
			return projectionErr
		}
		projection.VerificationMetadata = nil
		record = &PublicAssignmentRecord{Projection: projection, Extensions: PublicAssignmentRecordExtensions{BenchmarkObservations: benchmark, ResourceSummary: resources}}
	}
	if err != nil {
		return err
	}
	body, err := MarshalAssignmentResultProjectionEnvelope(idempotencyKey, record)
	if err != nil {
		return err
	}
	_, err = c.exportFeedRecords(ctx, assignment.GetRunId(), []campaignFeedPublishRequest{{IdempotencyKey: idempotencyKey, Body: body}})
	return err
}

func (c *CampaignPublicationCoordinator) buildAssignmentBenchmarkObservations(ctx context.Context, result *evalv1.EvaluationAssignmentResult) (*PublicBenchmarkObservations, error) {
	if c == nil || c.files == nil || result == nil {
		return nil, fmt.Errorf("evaluation: build assignment benchmark observations: %w", constants.ErrMissingRequiredField)
	}
	reader, err := NewCampaignProviderObservationReaderWithRemote(c.files, c.observationRemote)
	if err != nil {
		return nil, err
	}
	return reader.BuildPublicBenchmarkObservations(ctx, result)
}

// PublishRunAggregates emits explorer evaluation_summary, catalog, model, and
// methodology snapshot records derived from canonical assignment and result state.
func (c *CampaignPublicationCoordinator) PublishRunAggregates(ctx context.Context, runID string, observedAt time.Time) (int, error) {
	if c == nil || c.store == nil || c.files == nil || c.exporter == nil || runID == "" {
		return 0, fmt.Errorf("evaluation: publish run aggregates: %w", constants.ErrMissingRequiredField)
	}
	run, _, _, state, err := c.loadRunAggregateState(ctx, runID)
	if err != nil {
		return 0, err
	}
	if state.Scheduled == 0 {
		return 0, nil
	}
	records, err := BuildRunAggregateViewRecords(run, state, observedAt)
	if err != nil {
		return 0, err
	}
	requests := make([]campaignFeedPublishRequest, 0, len(records))
	for _, record := range records {
		requests = append(requests, campaignFeedPublishRequest(record))
	}
	return c.exportFeedRecords(ctx, runID, requests)
}

// PublishRunVerification emits the post-verify evaluation_summary revision and
// republicates terminal assignment results with verification_status=verified.
func (c *CampaignPublicationCoordinator) PublishRunVerification(ctx context.Context, runID string, report *evalv1.EvaluationVerificationReport) (int, error) {
	if c == nil || c.store == nil || c.files == nil || c.exporter == nil || runID == "" || report == nil || report.GetRunId() != runID {
		return 0, fmt.Errorf("evaluation: publish run verification: %w", constants.ErrMissingRequiredField)
	}
	run, assignments, results, state, err := c.loadRunAggregateState(ctx, runID)
	if err != nil {
		return 0, err
	}
	spec, err := c.store.LoadCampaignSpec(ctx, run.GetCampaignBinding().GetCampaignId())
	if err != nil {
		return 0, fmt.Errorf("evaluation: publish run verification: load campaign spec: %w", err)
	}
	catalog, err := c.store.LoadScenarioCatalog(ctx, run.GetCampaignBinding().GetCampaignId())
	if err != nil {
		return 0, err
	}
	applicability, err := BuildRunVerificationApplicability(run, spec, catalog, assignments, results, report)
	if err != nil {
		return 0, err
	}
	verified := report.GetStatus() == evalv1.EvaluationVerdictStatus_EVALUATION_VERDICT_STATUS_PASS
	if verified && !applicability.Applicable {
		return 0, fmt.Errorf("evaluation: publish run verification: report does not apply to persisted run evidence: %w", constants.ErrEvidenceScopeMismatch)
	}
	observedAt := time.Now().UTC()
	if report.GetVerifiedAt() != nil {
		observedAt = report.GetVerifiedAt().AsTime().UTC()
	}
	verifiedRequests := make([]campaignFeedPublishRequest, 0, len(assignments))
	if verified {
		_, artifacts, err := LoadScenarioCatalog()
		if err != nil {
			return 0, fmt.Errorf("evaluation: publish run verification: load scenario artifacts: %w", err)
		}
		store, ok := c.store.(*Store)
		if !ok {
			return 0, fmt.Errorf("evaluation: publish run verification: resolve scenario context: %w", constants.ErrEvidenceScopeMismatch)
		}
		for _, assignment := range assignments {
			result := results[assignment.GetAssignmentId()]
			if result == nil {
				continue
			}
			scenario, scenarioErr := ResolvePublicScenarioContext(ctx, store, run, catalog, assignment, artifacts)
			if scenarioErr != nil && !errors.Is(scenarioErr, constants.ErrEvidenceArtifactMalformed) {
				return 0, scenarioErr
			}
			benchmark, err := c.buildAssignmentBenchmarkObservations(ctx, result)
			if err != nil {
				return 0, err
			}
			resources, err := BuildPublicResourceSummary(result)
			if err != nil {
				return 0, err
			}
			key := AssignmentVerifiedResultIdempotencyKey(runID, assignment.GetAssignmentId())
			var record *PublicAssignmentRecord
			if scenarioErr == nil {
				record, err = BuildPublicAssignmentProjection(ctx, PublicAssignmentBuildInput{
					Assignment:           assignment,
					Result:               result,
					ScenarioContext:      scenario,
					VerificationMetadata: exportVerificationMetadata(report, true),
					VerificationStatus:   "verified",
					Extensions:           PublicAssignmentRecordExtensions{BenchmarkObservations: benchmark, ResourceSummary: resources},
				})
			} else {
				category, categoryErr := ScenarioCategoryForAssignment(catalog, assignment)
				if categoryErr != nil {
					return 0, categoryErr
				}
				projection, projectionErr := BuildAssignmentResultProjection(assignment, result, category, DerivePublicSummaryStatus(result), "verified")
				if projectionErr != nil {
					return 0, projectionErr
				}
				projection.VerificationMetadata = exportVerificationMetadata(report, true)
				record = &PublicAssignmentRecord{Projection: projection, Extensions: PublicAssignmentRecordExtensions{BenchmarkObservations: benchmark, ResourceSummary: resources}}
			}
			if err != nil {
				return 0, err
			}
			body, err := MarshalAssignmentResultProjectionEnvelope(key, record)
			if err != nil {
				return 0, err
			}
			verifiedRequests = append(verifiedRequests, campaignFeedPublishRequest{IdempotencyKey: key, Body: body})
		}
	}
	published, err := c.exportFeedRecords(ctx, runID, verifiedRequests)
	if err != nil {
		return published, err
	}
	records, err := BuildRunVerificationViewRecords(run, state, report, observedAt)
	if err != nil {
		return published, err
	}
	summaryRequests := make([]campaignFeedPublishRequest, 0, len(records))
	for _, record := range records {
		summaryRequests = append(summaryRequests, campaignFeedPublishRequest(record))
	}
	summaryCount, err := c.exportFeedRecords(ctx, runID, summaryRequests)
	if err != nil {
		return published, err
	}
	return published + summaryCount, nil
}

// PublishRunCompletion emits the terminal evaluation_summary and completion
// aggregate snapshots once every scheduled assignment is settled.
func (c *CampaignPublicationCoordinator) PublishRunCompletion(ctx context.Context, runID string, observedAt time.Time) (int, error) {
	if c == nil || c.store == nil || c.files == nil || c.exporter == nil || runID == "" {
		return 0, fmt.Errorf("evaluation: publish run completion: %w", constants.ErrMissingRequiredField)
	}
	run, assignments, results, state, err := c.loadRunAggregateState(ctx, runID)
	if err != nil {
		return 0, err
	}
	if !RunAggregateComplete(assignments, results, state) {
		return 0, nil
	}
	records, err := BuildRunCompletionViewRecords(run, assignments, results, state, observedAt)
	if err != nil {
		return 0, err
	}
	requests := make([]campaignFeedPublishRequest, 0, len(records))
	for _, record := range records {
		requests = append(requests, campaignFeedPublishRequest(record))
	}
	return c.exportFeedRecords(ctx, runID, requests)
}

// ResetPublicationIdempotency clears gateway-owned publication idempotency so a
// run can be republished after the mirror volume was wiped.
func (c *CampaignPublicationCoordinator) ResetPublicationIdempotency(ctx context.Context, runID string) error {
	if c == nil || c.publicationState == nil || runID == "" {
		return fmt.Errorf("evaluation: reset publication idempotency: %w", constants.ErrMissingRequiredField)
	}
	state, err := c.publicationState.Load(ctx, runID)
	if err != nil {
		return err
	}
	state.PublishedIdempotency = []string{}
	state.LastPublishedSequence = 0
	return c.publicationState.Save(ctx, state)
}

// PublishRunCatchUpWithVerification republishes one run and, when the persisted
// verification report passed, emits the post-verify explorer revisions.
func (c *CampaignPublicationCoordinator) PublishRunCatchUpWithVerification(ctx context.Context, runID string, report *evalv1.EvaluationVerificationReport) (int, error) {
	if report != nil && report.GetRunId() != runID {
		return 0, fmt.Errorf("evaluation: publish run catch-up with verification: report run mismatch: %w", constants.ErrEvidenceScopeMismatch)
	}
	if report != nil && report.GetStatus() == evalv1.EvaluationVerdictStatus_EVALUATION_VERDICT_STATUS_PASS {
		if err := c.validateRunVerificationApplicability(ctx, runID, report); err != nil {
			return 0, err
		}
	}
	published, err := c.PublishRunCatchUp(ctx, runID)
	if err != nil {
		return published, err
	}
	if report == nil || report.GetStatus() != evalv1.EvaluationVerdictStatus_EVALUATION_VERDICT_STATUS_PASS {
		return published, nil
	}
	verificationCount, err := c.PublishRunVerification(ctx, runID, report)
	if err != nil {
		return published, err
	}
	return published + verificationCount, nil
}

func (c *CampaignPublicationCoordinator) validateRunVerificationApplicability(ctx context.Context, runID string, report *evalv1.EvaluationVerificationReport) error {
	run, assignments, results, _, err := c.loadRunAggregateState(ctx, runID)
	if err != nil {
		return err
	}
	spec, err := c.store.LoadCampaignSpec(ctx, run.GetCampaignBinding().GetCampaignId())
	if err != nil {
		return fmt.Errorf("evaluation: validate run verification applicability: load campaign spec: %w", err)
	}
	catalog, err := c.store.LoadScenarioCatalog(ctx, run.GetCampaignBinding().GetCampaignId())
	if err != nil {
		return err
	}
	applicability, err := BuildRunVerificationApplicability(run, spec, catalog, assignments, results, report)
	if err != nil {
		return err
	}
	if !applicability.Applicable {
		return fmt.Errorf("evaluation: validate run verification applicability: %w", constants.ErrEvidenceScopeMismatch)
	}
	return nil
}

// PublishRunCatchUp scans one run and publishes any missing lifecycle and
// terminal result projections derived from canonical records.
func (c *CampaignPublicationCoordinator) PublishRunCatchUp(ctx context.Context, runID string) (int, error) {
	if c == nil || c.store == nil || c.files == nil || c.exporter == nil || runID == "" {
		return 0, fmt.Errorf("evaluation: publish run catch-up: %w", constants.ErrMissingRequiredField)
	}
	if err := c.ensureMirrorCatchUpReady(ctx, runID); err != nil {
		return 0, err
	}
	run, err := c.store.LoadRun(ctx, runID)
	if err != nil {
		return 0, err
	}
	catalog, err := c.store.LoadScenarioCatalog(ctx, run.GetCampaignBinding().GetCampaignId())
	if err != nil {
		return 0, err
	}
	assignments, err := c.store.ListAssignments(ctx, runID)
	if err != nil {
		return 0, err
	}
	published := 0
	for _, assignment := range assignments {
		category, err := ScenarioCategoryForAssignment(catalog, assignment)
		if err != nil {
			return published, err
		}
		if err := c.PublishAssignmentLifecycle(ctx, assignment, category, assignmentLifecycleObservedAt(assignment)); err != nil {
			return published, err
		}
		published++
		if exists, err := c.store.AssignmentResultExists(ctx, runID, assignment.GetAssignmentId()); err != nil {
			return published, err
		} else if !exists {
			continue
		}
		result, err := c.store.LoadAssignmentResult(ctx, runID, assignment.GetAssignmentId())
		if err != nil {
			return published, err
		}
		if err := c.PublishAssignmentResult(ctx, assignment, result, category, "unverified"); err != nil {
			return published, err
		}
		published++
	}
	aggregateCount, err := c.PublishRunAggregates(ctx, runID, time.Now().UTC())
	if err != nil {
		return published, err
	}
	published += aggregateCount
	completionCount, err := c.PublishRunCompletion(ctx, runID, time.Now().UTC())
	if err != nil {
		return published, err
	}
	return published + completionCount, nil
}

func (c *CampaignPublicationCoordinator) ensureMirrorCatchUpReady(ctx context.Context, runID string) error {
	if c == nil || c.mirrorProbe == nil || runID == "" {
		return nil
	}
	present, err := c.mirrorProbe.DatasetPresent(ctx, CampaignDatasetID(runID))
	if err != nil {
		return fmt.Errorf("evaluation: publish run catch-up: mirror probe: %w", err)
	}
	if present {
		return nil
	}
	state, err := c.publicationState.Load(ctx, runID)
	if err != nil {
		return err
	}
	if len(state.PublishedIdempotency) == 0 {
		return nil
	}
	return c.ResetPublicationIdempotency(ctx, runID)
}

func (c *CampaignPublicationCoordinator) loadRunAggregateState(ctx context.Context, runID string) (*evalv1.EvaluationRun, []*evalv1.EvaluationAssignment, map[string]*evalv1.EvaluationAssignmentResult, *runAggregateState, error) {
	run, err := c.store.LoadRun(ctx, runID)
	if err != nil {
		return nil, nil, nil, nil, err
	}
	assignments, err := c.store.ListAssignments(ctx, runID)
	if err != nil {
		return nil, nil, nil, nil, err
	}
	if len(assignments) == 0 {
		return run, assignments, map[string]*evalv1.EvaluationAssignmentResult{}, &runAggregateState{VariantRoles: map[string]*variantRoleAggregate{}}, nil
	}
	results := make(map[string]*evalv1.EvaluationAssignmentResult, len(assignments))
	for _, assignment := range assignments {
		exists, err := c.store.AssignmentResultExists(ctx, runID, assignment.GetAssignmentId())
		if err != nil {
			return nil, nil, nil, nil, err
		}
		if !exists {
			continue
		}
		result, err := c.store.LoadAssignmentResult(ctx, runID, assignment.GetAssignmentId())
		if err != nil {
			return nil, nil, nil, nil, err
		}
		results[assignment.GetAssignmentId()] = result
	}
	state, err := CollectRunAggregateState(assignments, results)
	if err != nil {
		return nil, nil, nil, nil, err
	}
	return run, assignments, results, state, nil
}

func (c *CampaignPublicationCoordinator) publishAssignmentResultEnvelope(ctx context.Context, runID, idempotencyKey string, projection *evalv1.PublicAssignmentResultProjection, benchmark *PublicBenchmarkObservations) error {
	body, err := MarshalAssignmentResultProjectionEnvelope(idempotencyKey, &PublicAssignmentRecord{
		Projection: projection,
		Extensions: PublicAssignmentRecordExtensions{BenchmarkObservations: benchmark},
	})
	if err != nil {
		return err
	}
	_, err = c.exportFeedRecords(ctx, runID, []campaignFeedPublishRequest{{
		IdempotencyKey: idempotencyKey,
		Body:           body,
	}})
	return err
}

func (c *CampaignPublicationCoordinator) publishEnvelope(ctx context.Context, runID, idempotencyKey, messageType string, record proto.Message) error {
	body, err := MarshalCampaignProjectionEnvelope(messageType, idempotencyKey, record)
	if err != nil {
		return err
	}
	_, err = c.exportFeedRecords(ctx, runID, []campaignFeedPublishRequest{{
		IdempotencyKey: idempotencyKey,
		Body:           body,
	}})
	return err
}

func (c *CampaignPublicationCoordinator) exportFeedRecords(ctx context.Context, runID string, requests []campaignFeedPublishRequest) (int, error) {
	if c == nil || c.exporter == nil || runID == "" {
		return 0, fmt.Errorf("evaluation: export feed records: %w", constants.ErrMissingRequiredField)
	}
	if len(requests) == 0 {
		return 0, nil
	}
	if c.publicationState == nil {
		return 0, fmt.Errorf("evaluation: export feed records: %w", constants.ErrMissingRequiredField)
	}
	state, err := c.publicationState.Load(ctx, runID)
	if err != nil {
		return 0, err
	}
	pending := make([]campaignFeedPublishRequest, 0, len(requests))
	for _, request := range requests {
		if request.IdempotencyKey == "" || len(request.Body) == 0 {
			return 0, fmt.Errorf("evaluation: export feed records: %w", constants.ErrMissingRequiredField)
		}
		if containsString(state.PublishedIdempotency, request.IdempotencyKey) {
			continue
		}
		pending = append(pending, request)
	}
	if len(pending) == 0 {
		return 0, nil
	}
	for attempt := 0; attempt < 2; attempt++ {
		nextSequence, err := c.exporter.HighWaterSequence(ctx)
		if err != nil {
			return 0, err
		}
		records := make([]CampaignPublicFeedRecord, len(pending))
		for index, request := range pending {
			records[index] = buildCampaignPublicFeedRecord(nextSequence+1+int64(index), request.Body)
		}
		if err := c.exportFeedRecordsInBatches(ctx, records); err != nil {
			if attempt == 0 && isCampaignFeedSequenceOutOfOrder(err) {
				continue
			}
			return 0, err
		}
		for _, request := range pending {
			state.PublishedIdempotency = append(state.PublishedIdempotency, request.IdempotencyKey)
		}
		sort.Strings(state.PublishedIdempotency)
		state.LastPublishedSequence = records[len(records)-1].Sequence
		if err := c.publicationState.Save(ctx, state); err != nil {
			return 0, err
		}
		return len(pending), nil
	}
	return 0, fmt.Errorf("evaluation: export feed records: %w", constants.ErrPublicFeedSequenceOutOfOrder)
}

func (c *CampaignPublicationCoordinator) exportFeedRecordsInBatches(ctx context.Context, records []CampaignPublicFeedRecord) error {
	for start := 0; start < len(records); start += constants.PublicFeedBatchMaxRecords {
		end := start + constants.PublicFeedBatchMaxRecords
		if end > len(records) {
			end = len(records)
		}
		if err := c.exporter.ExportBatch(ctx, records[start:end]); err != nil {
			return err
		}
	}
	return nil
}

func isCampaignFeedSequenceOutOfOrder(err error) bool {
	return err != nil && strings.Contains(err.Error(), constants.ErrPublicFeedSequenceOutOfOrder.Error())
}

func buildCampaignPublicFeedRecord(sequence int64, body []byte) CampaignPublicFeedRecord {
	digest := sha256.Sum256(body)
	return CampaignPublicFeedRecord{
		Sequence:    sequence,
		RecordHash:  hex.EncodeToString(digest[:]),
		RecordBytes: string(body),
	}
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
