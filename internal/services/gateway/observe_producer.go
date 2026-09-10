// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package gateway

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/marshaler"
	"github.com/g8e-ai/g8e/v2/internal/models"
	"github.com/g8e-ai/g8e/v2/internal/services/fs"
)

// observeProducerID is the producer_id stamped into SSE event rows emitted by
// the in-process observe producer. It identifies the gateway-internal
// projection producer so consumers and audit paths can distinguish
// gateway-produced observe events from external app-workload pushes.
const observeProducerID = "g8e-gateway-observe-producer"

// agentTransitions defines the authoritative persisted agent lifecycle state
// transitions. A transition is valid only if the target status is in the set
// for the current status. Same-status transitions are idempotent and always
// valid (they refresh the projection without changing lifecycle state).
//
// Terminal states (completed, failed) can reset to idle or go offline. The
// offline state can recover to idle, queued, or running.
var agentTransitions = map[models.AgentLifecycleStatus]map[models.AgentLifecycleStatus]struct{}{
	models.AgentLifecycleStatusIdle: {
		models.AgentLifecycleStatusIdle:      {},
		models.AgentLifecycleStatusQueued:    {},
		models.AgentLifecycleStatusRunning:   {},
		models.AgentLifecycleStatusWaiting:   {},
		models.AgentLifecycleStatusOffline:   {},
		models.AgentLifecycleStatusCompleted: {},
		models.AgentLifecycleStatusFailed:    {},
	},
	models.AgentLifecycleStatusQueued: {
		models.AgentLifecycleStatusQueued:  {},
		models.AgentLifecycleStatusRunning: {},
		models.AgentLifecycleStatusIdle:    {},
		models.AgentLifecycleStatusFailed:  {},
		models.AgentLifecycleStatusOffline: {},
	},
	models.AgentLifecycleStatusRunning: {
		models.AgentLifecycleStatusRunning:   {},
		models.AgentLifecycleStatusWaiting:   {},
		models.AgentLifecycleStatusCompleted: {},
		models.AgentLifecycleStatusFailed:    {},
		models.AgentLifecycleStatusIdle:      {},
		models.AgentLifecycleStatusOffline:   {},
	},
	models.AgentLifecycleStatusWaiting: {
		models.AgentLifecycleStatusWaiting:   {},
		models.AgentLifecycleStatusRunning:   {},
		models.AgentLifecycleStatusCompleted: {},
		models.AgentLifecycleStatusFailed:    {},
		models.AgentLifecycleStatusIdle:      {},
		models.AgentLifecycleStatusOffline:   {},
	},
	models.AgentLifecycleStatusCompleted: {
		models.AgentLifecycleStatusCompleted: {},
		models.AgentLifecycleStatusIdle:      {},
		models.AgentLifecycleStatusOffline:   {},
	},
	models.AgentLifecycleStatusFailed: {
		models.AgentLifecycleStatusFailed:  {},
		models.AgentLifecycleStatusIdle:    {},
		models.AgentLifecycleStatusOffline: {},
	},
	models.AgentLifecycleStatusOffline: {
		models.AgentLifecycleStatusOffline: {},
		models.AgentLifecycleStatusIdle:    {},
		models.AgentLifecycleStatusQueued:  {},
		models.AgentLifecycleStatusRunning: {},
	},
}

// runTransitions defines the authoritative persisted run lifecycle state
// transitions. Terminal states (completed, failed, cancelled) have no valid
// outgoing transitions to non-terminal states. Same-status transitions are
// idempotent for non-terminal states and valid for terminal states (refresh
// without regression).
var runTransitions = map[models.RunLifecycleStatus]map[models.RunLifecycleStatus]struct{}{
	models.RunLifecycleStatusQueued: {
		models.RunLifecycleStatusQueued:    {},
		models.RunLifecycleStatusRunning:   {},
		models.RunLifecycleStatusWaiting:   {},
		models.RunLifecycleStatusCompleted: {},
		models.RunLifecycleStatusFailed:    {},
		models.RunLifecycleStatusCancelled: {},
	},
	models.RunLifecycleStatusRunning: {
		models.RunLifecycleStatusRunning:   {},
		models.RunLifecycleStatusWaiting:   {},
		models.RunLifecycleStatusCompleted: {},
		models.RunLifecycleStatusFailed:    {},
		models.RunLifecycleStatusCancelled: {},
	},
	models.RunLifecycleStatusWaiting: {
		models.RunLifecycleStatusWaiting:   {},
		models.RunLifecycleStatusRunning:   {},
		models.RunLifecycleStatusCompleted: {},
		models.RunLifecycleStatusFailed:    {},
		models.RunLifecycleStatusCancelled: {},
	},
	models.RunLifecycleStatusCompleted: {
		models.RunLifecycleStatusCompleted: {},
	},
	models.RunLifecycleStatusFailed: {
		models.RunLifecycleStatusFailed: {},
	},
	models.RunLifecycleStatusCancelled: {
		models.RunLifecycleStatusCancelled: {},
	},
}

// isValidAgentTransition returns true if transitioning from current to next is
// allowed by agentTransitions. A zero-value current (no existing projection)
// accepts any target status.
func isValidAgentTransition(current, next models.AgentLifecycleStatus) bool {
	if current == "" {
		return true
	}
	allowed, ok := agentTransitions[current]
	if !ok {
		return false
	}
	_, ok = allowed[next]
	return ok
}

// isValidRunTransition returns true if transitioning from current to next is
// allowed by runTransitions. A zero-value current (no existing projection)
// accepts any target status.
func isValidRunTransition(current, next models.RunLifecycleStatus) bool {
	if current == "" {
		return true
	}
	allowed, ok := runTransitions[current]
	if !ok {
		return false
	}
	_, ok = allowed[next]
	return ok
}

// sseEventEnvelope is the nested event object inside an SSEPushPayload. The
// type field is the event type constant and data is the typed payload JSON.
type sseEventEnvelope struct {
	Type string          `json:"type"`
	Data json.RawMessage `json:"data"`
}

// ObserveProducerService persists agent and run state projections to the
// document store and emits typed SSE events after successful persistence. It
// enforces the persist-before-publish ordering: if persistence fails, no SSE
// event is emitted. It validates state transitions and rejects stale updates
// (older observed_at overwriting a newer one).
//
// The producer never mutates governance state. Projections are read-only
// documents in the observe collections; they are not governed records and do
// not flow through the GovernanceEnvelope. SSE events are telemetry, not
// receipts — consumers reconcile against read API snapshots, not against the
// event stream alone.
type ObserveProducerService struct {
	docStore *DocumentStoreService
	sseStore *SSEEventService
	pubsub   *GatewayWebSocketHandler
	fileSvc  fs.RuntimeFileService
	logger   *slog.Logger
}

// NewObserveProducerService creates a new ObserveProducerService backed by the
// given document store, SSE event store, pubsub handler, and runtime file
// service. The file service is used to persist download artifact bytes under
// the runtime downloads directory.
func NewObserveProducerService(docStore *DocumentStoreService, sseStore *SSEEventService, pubsub *GatewayWebSocketHandler, fileSvc fs.RuntimeFileService, logger *slog.Logger) *ObserveProducerService {
	return &ObserveProducerService{
		docStore: docStore,
		sseStore: sseStore,
		pubsub:   pubsub,
		fileSvc:  fileSvc,
		logger:   logger,
	}
}

// UpdateAgentState validates the agent state transition, persists the agent
// state projection, and emits an app.agent.status.updated SSE event after
// successful persistence. Returns an error if the transition is invalid, the
// update is stale, or persistence or emission fails. No SSE event is emitted
// if persistence fails (persist-before-publish).
func (s *ObserveProducerService) UpdateAgentState(ctx context.Context, userID string, route SSERoute, payload models.AgentStatusUpdatedPayload) error {
	if payload.AgentID == "" {
		return fmt.Errorf("observe producer: update agent state: %w", constants.ErrObserveAgentIDRequired)
	}
	if payload.ObservedAt.IsZero() {
		return fmt.Errorf("observe producer: update agent state: %w", constants.ErrObserveObservedAtRequired)
	}
	if err := route.validate(); err != nil {
		return fmt.Errorf("observe producer: update agent state: %w", err)
	}

	collection := marshaler.CollectionName(constants.CollectionObserveAgentStates)

	// Read the existing projection to validate the transition and check
	// staleness. A nil projection means this is the first state for this
	// agent; any target status is accepted.
	existing, err := s.docStore.DocGet(collection, payload.AgentID)
	if err != nil {
		return fmt.Errorf("observe producer: update agent state: read existing: %w", err)
	}
	if existing != nil {
		var proj agentStateProjection
		if err := unmarshalDocData(existing, &proj); err != nil {
			return fmt.Errorf("observe producer: update agent state: unmarshal existing: %w", err)
		}
		if proj.UserID != userID {
			return fmt.Errorf("observe producer: update agent state: %w: agent %s owned by different user", constants.ErrObserveAgentNotFound, payload.AgentID)
		}
		if !isValidAgentTransition(proj.Status, payload.Status) {
			return fmt.Errorf("observe producer: update agent state: %w: %s -> %s for agent %s", constants.ErrObserveInvalidTransition, proj.Status, payload.Status, payload.AgentID)
		}
		if payload.ObservedAt.Before(proj.ObservedAt) {
			return fmt.Errorf("observe producer: update agent state: %w: new observed_at %s before existing %s for agent %s", constants.ErrObserveStaleUpdate, payload.ObservedAt.Format(time.RFC3339Nano), proj.ObservedAt.Format(time.RFC3339Nano), payload.AgentID)
		}
	}

	// Persist the projection. DocSet upserts with managed timestamps.
	proj := agentStateProjection{
		UserID: userID,
		AgentStateProjection: models.AgentStateProjection{
			SchemaVersion: constants.ObserveEventPayloadSchemaVersion,
			AgentID:       payload.AgentID,
			DisplayName:   payload.DisplayName,
			Role:          payload.Role,
			Status:        payload.Status,
			RunID:         payload.RunID,
			TaskID:        payload.TaskID,
			Model:         payload.Model,
			Freshness:     models.SnapshotFreshnessObserved,
			ObservedAt:    payload.ObservedAt,
		},
	}
	projBytes, err := json.Marshal(proj)
	if err != nil {
		return fmt.Errorf("observe producer: update agent state: marshal projection: %w", err)
	}
	if err := s.docStore.DocSet(collection, payload.AgentID, projBytes); err != nil {
		return fmt.Errorf("observe producer: update agent state: persist: %w", err)
	}

	// Emit the SSE event only after successful persistence.
	if err := s.emitSSEEvent(route, string(constants.EventAppAgentStatusUpdated), payload); err != nil {
		s.logger.Error("observe producer: update agent state: sse emission failed", "error", err, "agent_id", payload.AgentID)
		return fmt.Errorf("observe producer: update agent state: emit sse: %w", err)
	}
	return nil
}

// UpdateRunState validates the run state transition, persists the run
// projection, and emits an app.run.status.updated SSE event after successful
// persistence. Returns an error if the transition is invalid, the update is
// stale, or persistence or emission fails. No SSE event is emitted if
// persistence fails (persist-before-publish).
func (s *ObserveProducerService) UpdateRunState(ctx context.Context, userID string, route SSERoute, payload models.RunStatusUpdatedPayload) error {
	if payload.RunID == "" {
		return fmt.Errorf("observe producer: update run state: %w", constants.ErrObserveRunIDRequired)
	}
	if payload.ObservedAt.IsZero() {
		return fmt.Errorf("observe producer: update run state: %w", constants.ErrObserveObservedAtRequired)
	}
	if err := route.validate(); err != nil {
		return fmt.Errorf("observe producer: update run state: %w", err)
	}

	collection := marshaler.CollectionName(constants.CollectionObserveRuns)

	// Read the existing projection to validate the transition, check
	// staleness, and preserve tasks and evidence-safe links.
	existing, err := s.docStore.DocGet(collection, payload.RunID)
	if err != nil {
		return fmt.Errorf("observe producer: update run state: read existing: %w", err)
	}
	var existingProj *runProjection
	if existing != nil {
		var proj runProjection
		if err := unmarshalDocData(existing, &proj); err != nil {
			return fmt.Errorf("observe producer: update run state: unmarshal existing: %w", err)
		}
		if proj.UserID != userID {
			return fmt.Errorf("observe producer: update run state: %w: run %s owned by different user", constants.ErrObserveRunNotFound, payload.RunID)
		}
		if !isValidRunTransition(proj.Status, payload.Status) {
			return fmt.Errorf("observe producer: update run state: %w: %s -> %s for run %s", constants.ErrObserveInvalidTransition, proj.Status, payload.Status, payload.RunID)
		}
		if payload.ObservedAt.Before(proj.ObservedAt) {
			return fmt.Errorf("observe producer: update run state: %w: new observed_at %s before existing %s for run %s", constants.ErrObserveStaleUpdate, payload.ObservedAt.Format(time.RFC3339Nano), proj.ObservedAt.Format(time.RFC3339Nano), payload.RunID)
		}
		existingProj = &proj
	}

	// Persist the projection. Preserve tasks and evidence-safe links from
	// the existing projection (the SSE payload carries summary fields, not
	// the full detail).
	proj := runProjection{
		UserID:         userID,
		SchemaVersion:  constants.ObserveAPIReadModelSchemaVersion,
		RunID:          payload.RunID,
		RunKind:        payload.RunKind,
		DisplayName:    payload.DisplayName,
		Status:         payload.Status,
		ActiveTaskID:   payload.ActiveTaskID,
		CompletedTasks: payload.CompletedTasks,
		TotalTasks:     payload.TotalTasks,
		StartedAt:      payload.StartedAt,
		EndedAt:        payload.EndedAt,
		ObservedAt:     payload.ObservedAt,
	}
	if existingProj != nil {
		proj.Tasks = existingProj.Tasks
		proj.EvidenceSafeLinks = existingProj.EvidenceSafeLinks
		proj.HasReceipts = existingProj.HasReceipts
		proj.EvidenceCount = existingProj.EvidenceCount
	}
	projBytes, err := json.Marshal(proj)
	if err != nil {
		return fmt.Errorf("observe producer: update run state: marshal projection: %w", err)
	}
	if err := s.docStore.DocSet(collection, payload.RunID, projBytes); err != nil {
		return fmt.Errorf("observe producer: update run state: persist: %w", err)
	}

	// Emit the SSE event only after successful persistence.
	if err := s.emitSSEEvent(route, string(constants.EventAppRunStatusUpdated), payload); err != nil {
		s.logger.Error("observe producer: update run state: sse emission failed", "error", err, "run_id", payload.RunID)
		return fmt.Errorf("observe producer: update run state: emit sse: %w", err)
	}
	return nil
}

// emitSSEEvent constructs the nested SSE event envelope, appends a durable
// row to the SSE event store, and publishes the live event to the pubsub
// channel for the given route. The stored payload matches the SSEPushPayload
// wire shape so consumers parse it identically whether it arrived via the
// HTTP push endpoint or the in-process producer.
func (s *ObserveProducerService) emitSSEEvent(route SSERoute, eventType string, payload any) error {
	dataBytes, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal event data: %w", err)
	}
	envelope := sseEventEnvelope{
		Type: eventType,
		Data: dataBytes,
	}
	eventBytes, err := json.Marshal(envelope)
	if err != nil {
		return fmt.Errorf("marshal event envelope: %w", err)
	}
	pushPayload := models.SSEPushPayload{
		UserID: route.UserID,
		Event:  eventBytes,
	}
	if route.WebSessionID != "" {
		pushPayload.WebSessionID = route.WebSessionID
	} else {
		pushPayload.CliSessionID = route.CLISessionID
	}
	payloadBytes, err := json.Marshal(pushPayload)
	if err != nil {
		return fmt.Errorf("marshal push payload: %w", err)
	}

	rowID, err := s.sseStore.SSEEventsAppend(route, eventType, string(payloadBytes), observeProducerID)
	if err != nil {
		return fmt.Errorf("append sse event: %w", err)
	}

	// Publish to pubsub for real-time delivery. The channel matches the
	// SSE stream handler's subscription channel.
	var channel string
	switch {
	case route.CLISessionID != "":
		channel = "sse:cli:" + route.CLISessionID
	case route.WebSessionID != "":
		channel = "sse:web:" + route.WebSessionID
	}
	if channel != "" && s.pubsub != nil {
		pubEvent := models.SSEPublishedEvent{ID: rowID, Payload: json.RawMessage(payloadBytes)}
		envelopeJSON, err := json.Marshal(pubEvent)
		if err != nil {
			return fmt.Errorf("marshal published event: %w", err)
		}
		s.pubsub.Publish(channel, envelopeJSON)
	}
	return nil
}

// PublishEval publishes a verified eval bundle into disclosure-safe browser
// projections and authenticated downloads. It builds the eval projection from
// the verified request fields, canonically serializes it, computes the
// projection SHA-256, persists the eval projection and download catalog to the
// document store, writes download artifact bytes to the runtime downloads
// directory, updates or creates the run projection with run_kind="eval", and
// emits ai.eval.run.completed and one ai.eval.metric.recorded SSE event per
// eligible metric — all after successful persistence (persist-before-publish).
// If any persistence step fails, no SSE event is emitted.
func (s *ObserveProducerService) PublishEval(ctx context.Context, userID string, route SSERoute, req models.ObserveProducerEvalPublicationRequest) error {
	if err := route.validate(); err != nil {
		return fmt.Errorf("observe producer: publish eval: %w", err)
	}
	if !req.VerificationReport.OK {
		return fmt.Errorf("observe producer: publish eval: %w", constants.ErrObservePublicationNotVerified)
	}

	now := time.Now().UTC()
	completedAt := now

	// Build the eval projection from the verified request fields.
	proj := evalProjection{
		UserID: userID,
		EvalDetail: models.EvalDetail{
			SchemaVersion:             constants.ObserveAPIReadModelSchemaVersion,
			RunID:                     req.RunID,
			SuiteID:                   req.SuiteID,
			SuiteVersion:              req.SuiteVersion,
			ArmID:                     req.ArmID,
			ModelID:                   req.ModelID,
			ModelProvider:             req.ModelProvider,
			Status:                    models.RunLifecycleStatusCompleted,
			VerificationStatus:        models.EvalVerificationVerified,
			ReceiptCount:              req.ReceiptCount,
			AssignedTasks:             req.AssignedTasks,
			TerminalAttempts:          req.TerminalAttempts,
			Metrics:                   req.Metrics,
			CompletedAt:               &completedAt,
			ObservedAt:                now,
		},
	}

	// Canonically serialize the projection and compute SHA-256.
	projBytes, err := json.Marshal(proj)
	if err != nil {
		return fmt.Errorf("observe producer: publish eval: marshal projection: %w", err)
	}
	projHash := sha256.Sum256(projBytes)
	proj.PublishedProjectionSHA256 = hex.EncodeToString(projHash[:])

	// Re-marshal with the hash populated.
	projBytes, err = json.Marshal(proj)
	if err != nil {
		return fmt.Errorf("observe producer: publish eval: marshal projection with hash: %w", err)
	}

	// Persist the eval projection.
	evalCollection := marshaler.CollectionName(constants.CollectionObserveEvals)
	if err := s.docStore.DocSet(evalCollection, req.RunID, projBytes); err != nil {
		return fmt.Errorf("observe producer: publish eval: persist eval projection: %w", err)
	}

	// Persist download artifacts: decode base64 content, verify hash and size,
	// write bytes to the runtime downloads directory, and persist the
	// download projection.
	downloadCollection := marshaler.CollectionName(constants.CollectionObserveDownloads)
	for _, dl := range req.Downloads {
		contentBytes, err := base64.StdEncoding.DecodeString(dl.Content)
		if err != nil {
			return fmt.Errorf("observe producer: publish eval: decode artifact %q: %w", dl.ArtifactID, err)
		}
		// Verify content hash matches the declared SHA-256.
		contentHash := sha256.Sum256(contentBytes)
		contentHashHex := hex.EncodeToString(contentHash[:])
		if contentHashHex != dl.SHA256 {
			return fmt.Errorf("observe producer: publish eval: artifact %q: %w: declared=%s actual=%s", dl.ArtifactID, constants.ErrObservePublicationContentHashMismatch, dl.SHA256, contentHashHex)
		}
		// Verify content size matches the declared byte_size.
		if int64(len(contentBytes)) != dl.ByteSize {
			return fmt.Errorf("observe producer: publish eval: artifact %q: %w: declared=%d actual=%d", dl.ArtifactID, constants.ErrObservePublicationContentSizeMismatch, dl.ByteSize, len(contentBytes))
		}
		// Reject oversized artifacts.
		if dl.ByteSize > constants.ObserveDownloadArtifactMaxBytes {
			return fmt.Errorf("observe producer: publish eval: artifact %q: %w: %d > %d", dl.ArtifactID, constants.ErrObservePublicationArtifactOversized, dl.ByteSize, constants.ObserveDownloadArtifactMaxBytes)
		}
		// Write bytes to the runtime downloads directory.
		relPath := filepath.Join(constants.ObserveDownloadsDirname, dl.ArtifactID)
		if err := s.fileSvc.WriteFile(ctx, relPath, contentBytes, constants.PermFilePrivate); err != nil {
			return fmt.Errorf("observe producer: publish eval: write artifact %q: %w", dl.ArtifactID, err)
		}
		// Persist the download projection.
		dlProj := downloadProjection{
			UserID: userID,
			DownloadArtifact: models.DownloadArtifact{
				SchemaVersion:         constants.ObserveAPIReadModelSchemaVersion,
				ArtifactID:            dl.ArtifactID,
				Filename:              dl.Filename,
				MediaType:             dl.MediaType,
				ByteSize:              dl.ByteSize,
				SHA256:                dl.SHA256,
				PrivacyClassification: dl.PrivacyClassification,
				SourceRunID:           dl.SourceRunID,
				DownloadURL:           constants.APIPaths.ObserveDownloadsByID + dl.ArtifactID,
				GeneratedAt:           now,
			},
		}
		dlProjBytes, err := json.Marshal(dlProj)
		if err != nil {
			return fmt.Errorf("observe producer: publish eval: marshal download projection %q: %w", dl.ArtifactID, err)
		}
		if err := s.docStore.DocSet(downloadCollection, dl.ArtifactID, dlProjBytes); err != nil {
			return fmt.Errorf("observe producer: publish eval: persist download projection %q: %w", dl.ArtifactID, err)
		}
	}

	// Update or create the run projection with run_kind="eval" and
	// status="completed".
	runCollection := marshaler.CollectionName(constants.CollectionObserveRuns)
	runProj := runProjection{
		UserID:        userID,
		SchemaVersion: constants.ObserveAPIReadModelSchemaVersion,
		RunID:         req.RunID,
		RunKind:       models.RunKindEval,
		DisplayName:   fmt.Sprintf("Eval %s", req.SuiteID),
		Status:        models.RunLifecycleStatusCompleted,
		ObservedAt:    now,
	}
	runProjBytes, err := json.Marshal(runProj)
	if err != nil {
		return fmt.Errorf("observe producer: publish eval: marshal run projection: %w", err)
	}
	if err := s.docStore.DocSet(runCollection, req.RunID, runProjBytes); err != nil {
		return fmt.Errorf("observe producer: publish eval: persist run projection: %w", err)
	}

	// All persistence succeeded. Emit SSE events (persist-before-publish).

	// Emit ai.eval.run.completed.
	runCompletedPayload := models.EvalRunCompletedPayload{
		SchemaVersion:             constants.ObserveEventPayloadSchemaVersion,
		RunID:                     req.RunID,
		SuiteID:                   req.SuiteID,
		SuiteVersion:              req.SuiteVersion,
		ArmID:                     req.ArmID,
		TerminalAttempts:           req.TerminalAttempts,
		AssignedTasks:             req.AssignedTasks,
		ReceiptCount:              req.ReceiptCount,
		VerificationStatus:        models.EvalVerificationVerified,
		PublishedProjectionSHA256: proj.PublishedProjectionSHA256,
		CompletedAt:               completedAt,
	}
	if err := s.emitSSEEvent(route, string(constants.EventAiEvalRunCompleted), runCompletedPayload); err != nil {
		s.logger.Error("observe producer: publish eval: emit run completed sse failed", "error", err, "run_id", req.RunID)
		return fmt.Errorf("observe producer: publish eval: emit run completed sse: %w", err)
	}

	// Emit one ai.eval.metric.recorded per eligible metric.
	for _, metric := range req.Metrics {
		metricPayload := models.EvalMetricRecordedPayload{
			SchemaVersion:      constants.ObserveEventPayloadSchemaVersion,
			RunID:              req.RunID,
			MetricID:           metric.MetricID,
			MetricVersion:      metric.MetricVersion,
			Value:              metric.Value,
			Unit:               metric.Unit,
			Eligible:           metric.Eligible,
			Denominator:        metric.Denominator,
			VerificationStatus: models.EvalVerificationVerified,
			RecordedAt:         now,
		}
		if err := s.emitSSEEvent(route, string(constants.EventAiEvalMetricRecorded), metricPayload); err != nil {
			s.logger.Error("observe producer: publish eval: emit metric recorded sse failed", "error", err, "run_id", req.RunID, "metric_id", metric.MetricID)
			return fmt.Errorf("observe producer: publish eval: emit metric recorded sse: %w", err)
		}
	}

	return nil
}

// StreamDownload streams the bytes of a download artifact to the given
// ResponseWriter. It verifies ownership, checks the on-disk file hash and size
// against the catalog, rejects symlinks and oversized files, and sets the
// correct Content-Type, Content-Length, and Content-Disposition headers before
// streaming the artifact bytes.
func (s *ObserveProducerService) StreamDownload(ctx context.Context, userID, artifactID string, w http.ResponseWriter) error {
	if artifactID == "" {
		return fmt.Errorf("observe producer: stream download: %w", constants.ErrObserveDownloadNotFound)
	}

	// Look up the download projection and verify ownership.
	downloadCollection := marshaler.CollectionName(constants.CollectionObserveDownloads)
	doc, err := s.docStore.DocGet(downloadCollection, artifactID)
	if err != nil {
		return fmt.Errorf("observe producer: stream download: read projection: %w", err)
	}
	if doc == nil {
		return fmt.Errorf("observe producer: stream download: %w: %s", constants.ErrObserveDownloadNotFound, artifactID)
	}
	var proj downloadProjection
	if err := unmarshalDocData(doc, &proj); err != nil {
		return fmt.Errorf("observe producer: stream download: unmarshal projection: %w", err)
	}
	if proj.UserID != userID {
		return fmt.Errorf("observe producer: stream download: %w: artifact %s owned by different user", constants.ErrObserveDownloadNotFound, artifactID)
	}

	// Reject restricted artifacts (the catalog should only contain public_safe,
	// but fail closed if a restricted artifact somehow appears).
	if proj.PrivacyClassification != models.DownloadPrivacyPublicSafe {
		return fmt.Errorf("observe producer: stream download: %w: %q", constants.ErrObservePublicationRestrictedArtifact, artifactID)
	}

	// Resolve the on-disk file path and verify it stays within the downloads
	// directory. The artifact ID is a document key, not a filesystem path;
	// fileSvc.Resolve enforces runtime-dir containment.
	relPath := filepath.Join(constants.ObserveDownloadsDirname, artifactID)
	info, err := s.fileSvc.Stat(ctx, relPath)
	if err != nil {
		return fmt.Errorf("observe producer: stream download: stat artifact %q: %w", artifactID, err)
	}
	// Reject symlinks (regular files only).
	if info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("observe producer: stream download: %w: %q", constants.ErrObserveDownloadSymlinkRejected, artifactID)
	}
	// Reject oversized files.
	if info.Size() > constants.ObserveDownloadArtifactMaxBytes {
		return fmt.Errorf("observe producer: stream download: %w: %d > %d", constants.ErrObserveDownloadOversized, info.Size(), constants.ObserveDownloadArtifactMaxBytes)
	}
	// Verify on-disk file size matches the catalog size.
	if info.Size() != proj.ByteSize {
		return fmt.Errorf("observe producer: stream download: %w: catalog=%d disk=%d", constants.ErrObserveDownloadSizeMismatch, proj.ByteSize, info.Size())
	}

	// Read the file bytes and verify the hash.
	contentBytes, err := s.fileSvc.ReadFile(ctx, relPath)
	if err != nil {
		return fmt.Errorf("observe producer: stream download: read artifact %q: %w", artifactID, err)
	}
	contentHash := sha256.Sum256(contentBytes)
	contentHashHex := hex.EncodeToString(contentHash[:])
	if contentHashHex != proj.SHA256 {
		return fmt.Errorf("observe producer: stream download: %w: catalog=%s disk=%s", constants.ErrObserveDownloadHashMismatch, proj.SHA256, contentHashHex)
	}

	// Set headers and stream bytes.
	w.Header().Set("Content-Type", proj.MediaType)
	w.Header().Set("Content-Length", fmt.Sprintf("%d", len(contentBytes)))
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, proj.Filename))
	w.WriteHeader(http.StatusOK)
	if _, err := w.Write(contentBytes); err != nil {
		return fmt.Errorf("observe producer: stream download: write bytes: %w", err)
	}
	return nil
}
