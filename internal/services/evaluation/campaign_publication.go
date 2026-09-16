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
	"encoding/json"
	"fmt"
	"sort"
	"time"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	complianceevidence "github.com/g8e-ai/g8e/v2/internal/services/compliance/evidence"
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

type campaignPublicationState struct {
	SchemaVersion          string   `json:"schema_version"`
	RunID                  string   `json:"run_id"`
	PublishedIdempotency   []string `json:"published_idempotency_keys"`
	LastPublishedSequence  int64    `json:"last_published_sequence"`
}

// CampaignPublicationCoordinator projects canonical campaign state into typed
// public records and coordinates idempotent publisher export.
type CampaignPublicationCoordinator struct {
	store    CampaignStore
	files    fs.RuntimeFileService
	exporter CampaignFeedExporter
}

func NewCampaignPublicationCoordinator(store CampaignStore, files fs.RuntimeFileService, exporter CampaignFeedExporter) *CampaignPublicationCoordinator {
	return &CampaignPublicationCoordinator{store: store, files: files, exporter: exporter}
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
	projection, err := BuildAssignmentResultProjection(assignment, result, scenarioCategory, DerivePublicSummaryStatus(result), verificationStatus)
	if err != nil {
		return err
	}
	idempotencyKey := AssignmentResultIdempotencyKey(assignment.GetRunId(), assignment.GetAssignmentId())
	return c.publishEnvelope(ctx, assignment.GetRunId(), idempotencyKey, publicMessageTypeAssignmentResult, projection)
}

// PublishRunCatchUp scans one run and publishes any missing lifecycle and
// terminal result projections derived from canonical records.
func (c *CampaignPublicationCoordinator) PublishRunCatchUp(ctx context.Context, runID string) (int, error) {
	if c == nil || c.store == nil || c.files == nil || c.exporter == nil || runID == "" {
		return 0, fmt.Errorf("evaluation: publish run catch-up: %w", constants.ErrMissingRequiredField)
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
	return published, nil
}

func (c *CampaignPublicationCoordinator) publishEnvelope(ctx context.Context, runID, idempotencyKey, messageType string, record proto.Message) error {
	state, err := c.loadPublicationState(ctx, runID)
	if err != nil {
		return err
	}
	if containsString(state.PublishedIdempotency, idempotencyKey) {
		return nil
	}
	body, err := MarshalCampaignProjectionEnvelope(messageType, idempotencyKey, record)
	if err != nil {
		return err
	}
	nextSequence, err := c.exporter.HighWaterSequence(ctx)
	if err != nil {
		return err
	}
	nextSequence++
	feedRecord := buildCampaignPublicFeedRecord(nextSequence, body)
	if err := c.exporter.ExportBatch(ctx, []CampaignPublicFeedRecord{feedRecord}); err != nil {
		return err
	}
	state.PublishedIdempotency = append(state.PublishedIdempotency, idempotencyKey)
	sort.Strings(state.PublishedIdempotency)
	state.LastPublishedSequence = nextSequence
	return c.savePublicationState(ctx, state)
}

func buildCampaignPublicFeedRecord(sequence int64, body []byte) CampaignPublicFeedRecord {
	digest := sha256.Sum256(body)
	return CampaignPublicFeedRecord{
		Sequence:    sequence,
		RecordHash:  hex.EncodeToString(digest[:]),
		RecordBytes: string(body),
	}
}

func (c *CampaignPublicationCoordinator) loadPublicationState(ctx context.Context, runID string) (*campaignPublicationState, error) {
	path := campaignPublicationStatePath(runID)
	body, err := c.files.ReadFile(ctx, path)
	if err != nil {
		return &campaignPublicationState{
			SchemaVersion:        CampaignSchemaVersion,
			RunID:                runID,
			PublishedIdempotency: []string{},
		}, nil
	}
	state := &campaignPublicationState{}
	if err := json.Unmarshal(body, state); err != nil {
		return nil, fmt.Errorf("evaluation: load publication state: %w", err)
	}
	if state.SchemaVersion != CampaignSchemaVersion || state.RunID != runID {
		return nil, fmt.Errorf("evaluation: load publication state: scope mismatch")
	}
	if state.PublishedIdempotency == nil {
		state.PublishedIdempotency = []string{}
	}
	return state, nil
}

func (c *CampaignPublicationCoordinator) savePublicationState(ctx context.Context, state *campaignPublicationState) error {
	if state == nil || !complianceevidence.ValidPathElement(state.RunID) {
		return fmt.Errorf("evaluation: save publication state: %w", constants.ErrMissingRequiredField)
	}
	body, err := json.Marshal(state)
	if err != nil {
		return fmt.Errorf("evaluation: save publication state: %w", err)
	}
	if err := complianceevidence.ValidateCanonicalJSON(body); err != nil {
		return fmt.Errorf("evaluation: save publication state: canonical JSON: %w", err)
	}
	path := campaignPublicationStatePath(state.RunID)
	if err := c.files.MkdirAll(ctx, evaluationRunDir(state.RunID), constants.PermDirStandard); err != nil {
		return fmt.Errorf("evaluation: save publication state: %w", err)
	}
	return c.files.WriteFile(ctx, path, body, constants.PermFileReadOnly)
}

func campaignPublicationStatePath(runID string) string {
	return evaluationRunDir(runID) + "/" + constants.EvaluationPublicationStateFilename
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
