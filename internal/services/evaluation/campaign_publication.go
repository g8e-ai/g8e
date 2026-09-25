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
	stdfs "io/fs"
	"sort"
	"strings"
	"time"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/models"
	"github.com/g8e-ai/g8e/v2/internal/services/fs"
	evalv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/eval/v1"
	"google.golang.org/protobuf/proto"
)

// CampaignPublicFeedRecord is one append-only public feed payload ready for
// signing by the host publisher.
type CampaignPublicFeedRecord struct {
	Sequence    int64
	RecordType  models.PublicFeedRecordType
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
	RecordType     models.PublicFeedRecordType
	Body           []byte
}

// CampaignPublicationCoordinator projects canonical campaign state into typed
// public records and coordinates idempotent publisher export.
type CampaignPublicationCoordinator struct {
	store             CampaignStore
	files             fs.RuntimeFileService
	publicationState  CampaignPublicationStateStore
	exporter          CampaignFeedExporter
	proofPublisher    CampaignProofPublisher
	observationRemote ProviderObservationRemote
	mirrorProbe       CampaignMirrorProbe
	deferProofMirrorPush   bool
	pendingProofMirrorPush bool
	pendingProofInputs     []AssignmentAuditProofInput
	publicationStateCache  *CampaignPublicationState
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

// WithProofPublisher attaches an optional proof-catalog publisher used to ingest
// per-assignment audit slice artifacts at terminal publication time.
func (c *CampaignPublicationCoordinator) WithProofPublisher(publisher CampaignProofPublisher) *CampaignPublicationCoordinator {
	if c != nil {
		c.proofPublisher = publisher
	}
	return c
}

// PublishAssignmentLiveEvents emits disclosure-safe stage_updated and
// metric_updated records for one terminal assignment result.
func (c *CampaignPublicationCoordinator) PublishAssignmentLiveEvents(ctx context.Context, assignment *evalv1.EvaluationAssignment, result *evalv1.EvaluationAssignmentResult) error {
	if c == nil || c.store == nil || c.files == nil || c.exporter == nil || assignment == nil || result == nil {
		return fmt.Errorf("evaluation: publish assignment live events: %w", constants.ErrMissingRequiredField)
	}
	requests, err := c.buildAssignmentLiveEventPublishRequests(ctx, assignment, result)
	if err != nil {
		return err
	}
	_, err = c.exportFeedRecords(ctx, assignment.GetRunId(), requests)
	return err
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
	liveRequests, err := c.buildAssignmentLiveEventPublishRequests(ctx, assignment, result)
	if err != nil {
		return err
	}
	auditBindings, err := c.BuildAssignmentAuditBindings(ctx, assignment, result, liveRequests, true)
	if err != nil {
		return err
	}
	return c.publishAssignmentResultWithKey(ctx, assignment, result, scenarioCategory, verificationStatus, idempotencyKey, auditBindings)
}

func (c *CampaignPublicationCoordinator) publishAssignmentResultWithKey(
	ctx context.Context,
	assignment *evalv1.EvaluationAssignment,
	result *evalv1.EvaluationAssignmentResult,
	scenarioCategory evalv1.EvaluationScenarioCategory,
	verificationStatus string,
	idempotencyKey string,
	auditBindings []*evalv1.PublicEvidenceBinding,
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
	request, err := c.buildAssignmentResultPublishRequest(ctx, run, catalog, artifacts, assignment, result, scenarioCategory, verificationStatus, idempotencyKey, auditBindings)
	if err != nil {
		return err
	}
	_, err = c.exportFeedRecords(ctx, assignment.GetRunId(), []campaignFeedPublishRequest{request})
	return err
}

func (c *CampaignPublicationCoordinator) buildAssignmentResultPublishRequest(
	ctx context.Context,
	run *evalv1.EvaluationRun,
	catalog *evalv1.EvaluationScenarioCatalog,
	artifacts map[string]ScenarioArtifacts,
	assignment *evalv1.EvaluationAssignment,
	result *evalv1.EvaluationAssignmentResult,
	scenarioCategory evalv1.EvaluationScenarioCategory,
	verificationStatus string,
	idempotencyKey string,
	auditBindings []*evalv1.PublicEvidenceBinding,
) (campaignFeedPublishRequest, error) {
	if c == nil || c.files == nil || run == nil || catalog == nil || assignment == nil || result == nil || idempotencyKey == "" {
		return campaignFeedPublishRequest{}, fmt.Errorf("evaluation: publish assignment result: %w", constants.ErrMissingRequiredField)
	}
	store, ok := c.store.(*Store)
	if !ok {
		return campaignFeedPublishRequest{}, fmt.Errorf("evaluation: publish assignment result: resolve scenario context: %w", constants.ErrEvidenceScopeMismatch)
	}
	scenario, scenarioErr := ResolvePublicScenarioContext(ctx, store, run, catalog, assignment, artifacts)
	if scenarioErr != nil && !errors.Is(scenarioErr, constants.ErrEvidenceArtifactMalformed) {
		return campaignFeedPublishRequest{}, scenarioErr
	}
	benchmark, err := c.buildAssignmentBenchmarkObservations(ctx, result)
	if err != nil {
		return campaignFeedPublishRequest{}, err
	}
	resources, err := BuildPublicResourceSummary(result)
	if err != nil {
		return campaignFeedPublishRequest{}, err
	}
	var record *PublicAssignmentRecord
	if scenarioErr == nil {
		record, err = BuildPublicAssignmentProjection(ctx, PublicAssignmentBuildInput{
			Assignment:         assignment,
			Result:             result,
			ScenarioContext:    scenario,
			EvidenceBindings:   auditBindings,
			VerificationStatus: verificationStatus,
			Extensions:         PublicAssignmentRecordExtensions{BenchmarkObservations: benchmark, ResourceSummary: resources},
		})
	} else {
		projection, projectionErr := BuildAssignmentResultProjection(assignment, result, scenarioCategory, DerivePublicSummaryStatus(result), verificationStatus)
		if projectionErr != nil {
			return campaignFeedPublishRequest{}, projectionErr
		}
		projection.VerificationMetadata = nil
		if len(auditBindings) > 0 {
			_, bindings, evidenceErr := BuildPublicAssignmentEvidence(PublicAssignmentEvidenceInput{
				Result:       result,
				PublicProofs: auditBindings,
			})
			if evidenceErr != nil {
				return campaignFeedPublishRequest{}, evidenceErr
			}
			projection.EvidenceBindings = bindings
		}
		record = &PublicAssignmentRecord{Projection: projection, Extensions: PublicAssignmentRecordExtensions{BenchmarkObservations: benchmark, ResourceSummary: resources}}
	}
	if err != nil {
		return campaignFeedPublishRequest{}, err
	}
	body, err := MarshalAssignmentResultProjectionEnvelope(idempotencyKey, record)
	if err != nil {
		return campaignFeedPublishRequest{}, err
	}
	return campaignFeedPublishRequest{IdempotencyKey: idempotencyKey, RecordType: models.PublicFeedRecordTypeProjection, Body: body}, nil
}

func buildAssignmentLifecyclePublishRequest(assignment *evalv1.EvaluationAssignment, scenarioCategory evalv1.EvaluationScenarioCategory, observedAt time.Time) (campaignFeedPublishRequest, error) {
	if assignment == nil {
		return campaignFeedPublishRequest{}, fmt.Errorf("evaluation: publish assignment lifecycle: %w", constants.ErrMissingRequiredField)
	}
	projection, err := BuildAssignmentLifecycleProjection(assignment, scenarioCategory, observedAt)
	if err != nil {
		return campaignFeedPublishRequest{}, err
	}
	idempotencyKey := AssignmentLifecycleIdempotencyKey(assignment.GetRunId(), assignment.GetAssignmentId(), assignment.GetLifecycleStatus())
	body, err := MarshalCampaignProjectionEnvelope(publicMessageTypeAssignmentLifecycle, idempotencyKey, projection)
	if err != nil {
		return campaignFeedPublishRequest{}, err
	}
	return campaignFeedPublishRequest{IdempotencyKey: idempotencyKey, RecordType: models.PublicFeedRecordTypeProjection, Body: body}, nil
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
// A bound, applicable persisted verification report keeps the emitted summary in
// its verified state so later aggregate revisions cannot regress verification.
func (c *CampaignPublicationCoordinator) PublishRunAggregates(ctx context.Context, runID string, observedAt time.Time) (int, error) {
	if c == nil || c.store == nil || c.files == nil || c.exporter == nil || runID == "" {
		return 0, fmt.Errorf("evaluation: publish run aggregates: %w", constants.ErrMissingRequiredField)
	}
	run, assignments, results, state, err := c.loadRunAggregateState(ctx, runID)
	if err != nil {
		return 0, err
	}
	if state.Scheduled == 0 {
		return 0, nil
	}
	report, err := c.loadBoundRunVerification(ctx, runID, run, assignments, results)
	if err != nil {
		return 0, err
	}
	records, err := BuildRunAggregateViewRecords(run, state, report, observedAt)
	if err != nil {
		return 0, err
	}
	requests := viewRecordsToPublishRequests(records)
	return c.exportFeedRecords(ctx, runID, requests)
}

// PublishRunVerification emits the post-verify evaluation_summary revision and
// republicates terminal assignment results with verification_status=verified.
func (c *CampaignPublicationCoordinator) PublishRunVerification(ctx context.Context, runID string, report *evalv1.EvaluationVerificationReport) (int, error) {
	if c == nil || c.store == nil || c.files == nil || c.exporter == nil || runID == "" || report == nil || report.GetRunId() != runID {
		return 0, fmt.Errorf("evaluation: publish run verification: %w", constants.ErrMissingRequiredField)
	}
	c.deferProofMirrorPush = true
	defer func() {
		c.deferProofMirrorPush = false
		c.pendingProofInputs = nil
		c.clearPublicationStateCache()
	}()
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
	if !applicability.Applicable {
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
			liveRequests, err := c.buildAssignmentLiveEventPublishRequests(ctx, assignment, result)
			if err != nil {
				return 0, err
			}
			auditBindings, err := c.BuildAssignmentAuditBindings(ctx, assignment, result, liveRequests, false)
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
					EvidenceBindings:     auditBindings,
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
				if len(auditBindings) > 0 {
					_, bindings, evidenceErr := BuildPublicAssignmentEvidence(PublicAssignmentEvidenceInput{
						Result:       result,
						PublicProofs: auditBindings,
					})
					if evidenceErr != nil {
						return 0, evidenceErr
					}
					projection.EvidenceBindings = bindings
				}
				record = &PublicAssignmentRecord{Projection: projection, Extensions: PublicAssignmentRecordExtensions{BenchmarkObservations: benchmark, ResourceSummary: resources}}
			}
			if err != nil {
				return 0, err
			}
			body, err := MarshalAssignmentResultProjectionEnvelope(key, record)
			if err != nil {
				return 0, err
			}
			verifiedRequests = append(verifiedRequests, campaignFeedPublishRequest{IdempotencyKey: key, RecordType: models.PublicFeedRecordTypeProjection, Body: body})
		}
	}
	published, err := c.exportFeedRecords(ctx, runID, verifiedRequests)
	if err != nil {
		return published, err
	}
	if err := c.flushProofCatalog(ctx); err != nil {
		return published, err
	}
	records, err := BuildRunVerificationViewRecords(run, state, report, observedAt)
	if err != nil {
		return published, err
	}
	summaryRequests := viewRecordsToPublishRequests(records)
	summaryCount, err := c.exportFeedRecords(ctx, runID, summaryRequests)
	if err != nil {
		return published, err
	}
	return published + summaryCount, nil
}

// PublishRunCompletion emits the terminal evaluation_summary and completion
// aggregate snapshots once every scheduled assignment is settled. A bound,
// applicable persisted verification report keeps the emitted summary in its
// verified state so completion revisions cannot regress verification.
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
	report, err := c.loadBoundRunVerification(ctx, runID, run, assignments, results)
	if err != nil {
		return 0, err
	}
	records, err := BuildRunCompletionViewRecords(run, assignments, results, state, report, observedAt)
	if err != nil {
		return 0, err
	}
	requests := viewRecordsToPublishRequests(records)
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
	state.PublishedProofArtifacts = map[string]CampaignPublishedProofArtifacts{}
	state.LastPublishedSequence = 0
	if err := c.savePublicationState(ctx, state); err != nil {
		return err
	}
	c.clearPublicationStateCache()
	return nil
}

// ResetFeedPublicationIdempotency clears feed export idempotency while
// preserving published proof artifact hashes so force restore can republish
// projections without rebuilding unchanged audit exports.
func (c *CampaignPublicationCoordinator) ResetFeedPublicationIdempotency(ctx context.Context, runID string) error {
	if c == nil || c.publicationState == nil || runID == "" {
		return fmt.Errorf("evaluation: reset feed publication idempotency: %w", constants.ErrMissingRequiredField)
	}
	state, err := c.publicationState.Load(ctx, runID)
	if err != nil {
		return err
	}
	preserved := make([]string, 0)
	for _, key := range state.PublishedIdempotency {
		if strings.HasSuffix(key, ":audit-proof") {
			preserved = append(preserved, key)
		}
	}
	state.PublishedIdempotency = preserved
	state.LastPublishedSequence = 0
	if err := c.savePublicationState(ctx, state); err != nil {
		return err
	}
	c.clearPublicationStateCache()
	return nil
}

// PublishRunCatchUpWithVerification republishes one run and, when the persisted
// verification report carries a bound verdict, emits the post-verify explorer
// revisions. Aggregate and completion revisions inside the catch-up already
// project the persisted report, so a verified state cannot regress.
func (c *CampaignPublicationCoordinator) PublishRunCatchUpWithVerification(ctx context.Context, runID string, report *evalv1.EvaluationVerificationReport) (int, error) {
	if report != nil && report.GetSchemaVersion() == CampaignSchemaVersion {
		report = nil
	}
	if report != nil && report.GetSchemaVersion() != campaignVerificationSchemaVersion {
		return 0, fmt.Errorf("evaluation: publish run catch-up with verification: unsupported report schema: %w", constants.ErrEvidenceArtifactMalformed)
	}
	if report != nil && report.GetRunId() != runID {
		return 0, fmt.Errorf("evaluation: publish run catch-up with verification: report run mismatch: %w", constants.ErrEvidenceScopeMismatch)
	}
	if report != nil && report.GetStatus() != evalv1.EvaluationVerdictStatus_EVALUATION_VERDICT_STATUS_UNSPECIFIED {
		if err := c.validateRunVerificationApplicability(ctx, runID, report); err != nil {
			return 0, err
		}
	}
	published, err := c.PublishRunCatchUp(ctx, runID)
	if err != nil {
		return published, err
	}
	if report == nil || report.GetStatus() == evalv1.EvaluationVerdictStatus_EVALUATION_VERDICT_STATUS_UNSPECIFIED {
		return published, nil
	}
	verificationCount, err := c.PublishRunVerification(ctx, runID, report)
	if err != nil {
		return published, err
	}
	return published + verificationCount, nil
}

// loadBoundRunVerification resolves the persisted run-level verification report
// for summary construction. A missing or legacy unbound report yields nil. A
// bound report that no longer applies on an in-progress run is ignored so
// partial campaign verify does not block continued execute/publish. Once every
// scheduled assignment is settled, a non-applicable bound report fails closed
// rather than producing a downgrade.
func (c *CampaignPublicationCoordinator) loadBoundRunVerification(ctx context.Context, runID string, run *evalv1.EvaluationRun, assignments []*evalv1.EvaluationAssignment, results map[string]*evalv1.EvaluationAssignmentResult) (*evalv1.EvaluationVerificationReport, error) {
	report, err := c.store.LoadCampaignVerification(ctx, runID)
	if err != nil {
		if errors.Is(err, constants.ErrNotFound) || errors.Is(err, stdfs.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	if report == nil || report.GetSchemaVersion() != campaignVerificationSchemaVersion {
		return nil, nil
	}
	if !boundVerificationMetadataComplete(report) {
		return nil, fmt.Errorf("evaluation: load bound run verification: persisted report is malformed: %w", constants.ErrEvidenceArtifactMalformed)
	}
	spec, err := c.store.LoadCampaignSpec(ctx, run.GetCampaignBinding().GetCampaignId())
	if err != nil {
		return nil, fmt.Errorf("evaluation: load bound run verification: load campaign spec: %w", err)
	}
	catalog, err := c.store.LoadScenarioCatalog(ctx, run.GetCampaignBinding().GetCampaignId())
	if err != nil {
		return nil, err
	}
	applicability, err := BuildRunVerificationApplicability(run, spec, catalog, assignments, results, report)
	if err != nil {
		return nil, err
	}
	if applicability.Applicable {
		return report, nil
	}
	state, err := CollectRunAggregateState(assignments, results)
	if err != nil {
		return nil, err
	}
	if !RunAggregateComplete(assignments, results, state) {
		return nil, nil
	}
	return nil, fmt.Errorf("evaluation: load bound run verification: persisted report does not apply to run evidence: %w", constants.ErrEvidenceScopeMismatch)
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

func (c *CampaignPublicationCoordinator) PruneRunProofCatalog(ctx context.Context, runID string) error {
	if c == nil || c.proofPublisher == nil || runID == "" {
		return nil
	}
	if err := c.proofPublisher.PruneRunProofCatalog(ctx, runID); err != nil {
		return fmt.Errorf("evaluation: prune run proof catalog: %w", err)
	}
	return nil
}

func (c *CampaignPublicationCoordinator) flushProofCatalog(ctx context.Context) error {
	if c == nil || c.proofPublisher == nil {
		return nil
	}
	if len(c.pendingProofInputs) > 0 {
		pending := c.pendingProofInputs
		if err := c.proofPublisher.IngestAssignmentAuditSlices(ctx, pending, true); err != nil {
			return fmt.Errorf("evaluation: flush proof catalog: ingest assignment audit slices: %w", err)
		}
		for _, input := range pending {
			if err := c.recordPublishedProofArtifacts(ctx, input); err != nil {
				return fmt.Errorf("evaluation: flush proof catalog: record proof artifacts: %w", err)
			}
		}
		c.pendingProofInputs = nil
	}
	if !c.pendingProofMirrorPush {
		return nil
	}
	if err := c.proofPublisher.FlushProofCatalog(ctx); err != nil {
		return fmt.Errorf("evaluation: flush proof catalog: %w", err)
	}
	c.pendingProofMirrorPush = false
	return nil
}

func (c *CampaignPublicationCoordinator) clearPublicationStateCache() {
	if c != nil {
		c.publicationStateCache = nil
	}
}

func (c *CampaignPublicationCoordinator) recordPublishedProofArtifacts(ctx context.Context, input AssignmentAuditProofInput) error {
	if c == nil || c.publicationState == nil || input.RunID == "" || input.AssignmentID == "" {
		return nil
	}
	if input.Artifacts.DatabaseSHA256 == "" || input.Artifacts.VaultKeySHA256 == "" {
		return nil
	}
	state, err := c.loadPublicationState(ctx, input.RunID)
	if err != nil {
		return err
	}
	proofKey := AssignmentAuditProofIdempotencyKey(input.RunID, input.AssignmentID)
	if !containsString(state.PublishedIdempotency, proofKey) {
		state.PublishedIdempotency = append(state.PublishedIdempotency, proofKey)
		sort.Strings(state.PublishedIdempotency)
	}
	if state.PublishedProofArtifacts == nil {
		state.PublishedProofArtifacts = map[string]CampaignPublishedProofArtifacts{}
	}
	state.PublishedProofArtifacts[input.AssignmentID] = CampaignPublishedProofArtifacts{
		DatabaseSHA256: input.Artifacts.DatabaseSHA256,
		VaultKeySHA256: input.Artifacts.VaultKeySHA256,
	}
	return c.savePublicationState(ctx, state)
}

func (c *CampaignPublicationCoordinator) loadPublicationState(ctx context.Context, runID string) (*CampaignPublicationState, error) {
	if c == nil || c.publicationState == nil || runID == "" {
		return nil, fmt.Errorf("evaluation: load publication state: %w", constants.ErrMissingRequiredField)
	}
	if c.publicationStateCache != nil && c.publicationStateCache.RunID == runID {
		return cloneCampaignPublicationState(c.publicationStateCache), nil
	}
	state, err := c.publicationState.Load(ctx, runID)
	if err != nil {
		return nil, err
	}
	c.publicationStateCache = cloneCampaignPublicationState(state)
	return state, nil
}

func (c *CampaignPublicationCoordinator) savePublicationState(ctx context.Context, state *CampaignPublicationState) error {
	if c == nil || c.publicationState == nil || state == nil || state.RunID == "" {
		return fmt.Errorf("evaluation: save publication state: %w", constants.ErrMissingRequiredField)
	}
	if err := c.publicationState.Save(ctx, state); err != nil {
		return err
	}
	c.publicationStateCache = cloneCampaignPublicationState(state)
	return nil
}

// PublishRunCatchUp scans one run and publishes any missing lifecycle and
// terminal result projections derived from canonical records.
func (c *CampaignPublicationCoordinator) PublishRunCatchUp(ctx context.Context, runID string) (int, error) {
	if c == nil || c.store == nil || c.files == nil || c.exporter == nil || runID == "" {
		return 0, fmt.Errorf("evaluation: publish run catch-up: %w", constants.ErrMissingRequiredField)
	}
	c.deferProofMirrorPush = true
	defer func() {
		c.deferProofMirrorPush = false
		c.pendingProofInputs = nil
		c.clearPublicationStateCache()
	}()
	resetRequired, err := c.mirrorCatchUpResetRequired(ctx, runID)
	if err != nil {
		return 0, err
	}
	if resetRequired {
		if err := c.ResetPublicationIdempotency(ctx, runID); err != nil {
			return 0, err
		}
	}
	run, assignments, results, _, err := c.loadRunAggregateState(ctx, runID)
	if err != nil {
		return 0, err
	}
	catalog, err := c.store.LoadScenarioCatalog(ctx, run.GetCampaignBinding().GetCampaignId())
	if err != nil {
		return 0, err
	}
	_, artifacts, err := LoadScenarioCatalog()
	if err != nil {
		return 0, fmt.Errorf("evaluation: publish run catch-up: load scenario artifacts: %w", err)
	}
	publicationState, err := c.loadPublicationState(ctx, runID)
	if err != nil {
		return 0, err
	}
	requests := make([]campaignFeedPublishRequest, 0, len(assignments)*2)
	for _, assignment := range assignments {
		category, err := ScenarioCategoryForAssignment(catalog, assignment)
		if err != nil {
			return 0, err
		}
		lifecycleRequest, err := buildAssignmentLifecyclePublishRequest(assignment, category, assignmentLifecycleObservedAt(assignment))
		if err != nil {
			return 0, err
		}
		requests = append(requests, lifecycleRequest)
		result := results[assignment.GetAssignmentId()]
		if result == nil {
			continue
		}
		resultKey := AssignmentResultIdempotencyKey(runID, assignment.GetAssignmentId())
		if publicationState != nil && containsString(publicationState.PublishedIdempotency, resultKey) {
			continue
		}
		proofKey := AssignmentAuditProofIdempotencyKey(runID, assignment.GetAssignmentId())
		var auditBindings []*evalv1.PublicEvidenceBinding
		if publicationState != nil && containsString(publicationState.PublishedIdempotency, proofKey) {
			if stored, ok := publicationState.PublishedProofArtifacts[assignment.GetAssignmentId()]; ok {
				auditBindings = AssignmentAuditEvidenceBindingsFromHashes(stored.DatabaseSHA256, stored.VaultKeySHA256)
			}
		}
		if auditBindings == nil {
			liveRequests, err := c.buildAssignmentLiveEventPublishRequests(ctx, assignment, result)
			if err != nil {
				return 0, err
			}
			auditBindings, err = c.BuildAssignmentAuditBindings(ctx, assignment, result, liveRequests, true)
			if err != nil {
				return 0, err
			}
		}
		resultRequest, err := c.buildAssignmentResultPublishRequest(
			ctx,
			run,
			catalog,
			artifacts,
			assignment,
			result,
			category,
			"unverified",
			AssignmentResultIdempotencyKey(runID, assignment.GetAssignmentId()),
			auditBindings,
		)
		if err != nil {
			return 0, err
		}
		requests = append(requests, resultRequest)
	}
	published, err := c.exportFeedRecords(ctx, runID, requests)
	if err != nil {
		return published, err
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
	published += completionCount
	if err := c.flushProofCatalog(ctx); err != nil {
		return published, err
	}
	return published, nil
}

func (c *CampaignPublicationCoordinator) mirrorCatchUpResetRequired(ctx context.Context, runID string) (bool, error) {
	if c == nil || c.mirrorProbe == nil || runID == "" {
		return false, nil
	}
	present, err := c.mirrorProbe.DatasetPresent(ctx, CampaignDatasetID(runID))
	if err != nil {
		return false, fmt.Errorf("evaluation: publish run catch-up: mirror probe: %w", err)
	}
	if present {
		return false, nil
	}
	state, err := c.publicationState.Load(ctx, runID)
	if err != nil {
		return false, err
	}
	return len(state.PublishedIdempotency) > 0, nil
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
	results, err := c.store.LoadAssignmentResults(ctx, runID, assignments)
	if err != nil {
		return nil, nil, nil, nil, err
	}
	state, err := CollectRunAggregateState(assignments, results)
	if err != nil {
		return nil, nil, nil, nil, err
	}
	return run, assignments, results, state, nil
}

func (c *CampaignPublicationCoordinator) publishEnvelope(ctx context.Context, runID, idempotencyKey, messageType string, record proto.Message) error {
	body, err := MarshalCampaignProjectionEnvelope(messageType, idempotencyKey, record)
	if err != nil {
		return err
	}
	_, err = c.exportFeedRecords(ctx, runID, []campaignFeedPublishRequest{{
		IdempotencyKey: idempotencyKey,
		RecordType:     models.PublicFeedRecordTypeProjection,
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
	state, err := c.loadPublicationState(ctx, runID)
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
			records[index] = buildCampaignPublicFeedRecord(nextSequence+1+int64(index), request.RecordType, request.Body)
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
		if err := c.savePublicationState(ctx, state); err != nil {
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

func viewRecordsToPublishRequests(records []CampaignViewRecord) []campaignFeedPublishRequest {
	requests := make([]campaignFeedPublishRequest, 0, len(records))
	for _, record := range records {
		requests = append(requests, campaignFeedPublishRequest{
			IdempotencyKey: record.IdempotencyKey,
			RecordType:     models.PublicFeedRecordTypeProjection,
			Body:           record.Body,
		})
	}
	return requests
}

func buildCampaignPublicFeedRecord(sequence int64, recordType models.PublicFeedRecordType, body []byte) CampaignPublicFeedRecord {
	if recordType == "" {
		recordType = models.PublicFeedRecordTypeProjection
	}
	digest := sha256.Sum256(body)
	return CampaignPublicFeedRecord{
		Sequence:    sequence,
		RecordType:  recordType,
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
