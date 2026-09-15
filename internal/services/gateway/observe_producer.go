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
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"

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

	// sourceSeq is the monotonic source sequence counter for live campaign
	// events. Each live event gets the next sequence number for ordering
	// and replay.
	sourceSeq atomic.Int64

	// seenEventIDs tracks emitted event_ids for duplicate suppression. A
	// duplicate event_id is rejected with ErrObserveDuplicateEventID.
	seenMu       sync.Mutex
	seenEventIDs map[string]struct{}
}

// NewObserveProducerService creates a new ObserveProducerService backed by the
// given document store, SSE event store, pubsub handler, and runtime file
// service. The file service is used to persist download artifact bytes under
// the runtime downloads directory.
func NewObserveProducerService(docStore *DocumentStoreService, sseStore *SSEEventService, pubsub *GatewayWebSocketHandler, fileSvc fs.RuntimeFileService, logger *slog.Logger) *ObserveProducerService {
	return &ObserveProducerService{
		docStore:     docStore,
		sseStore:     sseStore,
		pubsub:       pubsub,
		fileSvc:      fileSvc,
		logger:       logger,
		seenEventIDs: make(map[string]struct{}),
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
		return fmt.Errorf("observe producer: stream download: %w: %q", constants.ErrObserveDownloadRestrictedArtifact, artifactID)
	}

	// Resolve the on-disk file path and verify it stays within the downloads
	// directory. The artifact ID is a document key, not a filesystem path;
	// fileSvc.Resolve enforces runtime-dir containment.
	relPath := filepath.Join(constants.ObserveDownloadsDirname, artifactID)
	absPath := s.fileSvc.Resolve(relPath)
	// Use Lstat to detect symlinks without following them. fileSvc.Stat uses
	// os.Stat which follows symlinks, so a symlink pointing outside the
	// downloads directory would evade the symlink check. Lstat returns the
	// symlink's own mode, not the target's.
	linfo, err := os.Lstat(absPath)
	if err != nil {
		return fmt.Errorf("observe producer: stream download: stat artifact %q: %w", artifactID, err)
	}
	// Reject symlinks (regular files only).
	if linfo.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("observe producer: stream download: %w: %q", constants.ErrObserveDownloadSymlinkRejected, artifactID)
	}
	// Reject oversized files.
	if linfo.Size() > constants.ObserveDownloadArtifactMaxBytes {
		return fmt.Errorf("observe producer: stream download: %w: %d > %d", constants.ErrObserveDownloadOversized, linfo.Size(), constants.ObserveDownloadArtifactMaxBytes)
	}
	// Verify on-disk file size matches the catalog size.
	if linfo.Size() != proj.ByteSize {
		return fmt.Errorf("observe producer: stream download: %w: catalog=%d disk=%d", constants.ErrObserveDownloadSizeMismatch, proj.ByteSize, linfo.Size())
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

// ---------------------------------------------------------------------------
// Live campaign producer methods (O2-live)
//
// Each method persists the corresponding projection before emitting the SSE
// event (persist-before-event). Live events carry a monotonic source_sequence
// and a unique event_id for duplicate suppression. The emitLiveSSEEvent
// method assigns the sequence, generates the event_id, checks for duplicates,
// and delegates to emitSSEEvent for the actual SSE emission.
// ---------------------------------------------------------------------------

// nextSourceSequence returns the next monotonic source sequence number.
func (s *ObserveProducerService) nextSourceSequence() int64 {
	return s.sourceSeq.Add(1)
}

// emitLiveSSEEvent generates a unique event_id, assigns the next monotonic
// source_sequence, checks for duplicate event_ids, and emits the SSE event
// via emitSSEEvent. The eventID is returned so callers can set it on the
// payload before marshaling. This method handles duplicate suppression: if
// the event_id has already been emitted, it returns
// ErrObserveDuplicateEventID.
//
// The payload must be a pointer to a struct that has SourceSequence (int64)
// and EventID (string) fields. This method sets those fields before
// marshaling.
func (s *ObserveProducerService) emitLiveSSEEvent(route SSERoute, eventType string, eventID string, payload any) error {
	// Duplicate suppression.
	s.seenMu.Lock()
	if _, ok := s.seenEventIDs[eventID]; ok {
		s.seenMu.Unlock()
		return fmt.Errorf("observe producer: emit live sse: %w: %s", constants.ErrObserveDuplicateEventID, eventID)
	}
	s.seenEventIDs[eventID] = struct{}{}
	s.seenMu.Unlock()

	return s.emitSSEEvent(route, eventType, payload)
}

// persistLiveProjection persists a live campaign projection to the document
// store with the user_id ownership field.
func (s *ObserveProducerService) persistLiveProjection(collection string, docID string, proj any) error {
	projBytes, err := json.Marshal(proj)
	if err != nil {
		return fmt.Errorf("marshal projection: %w", err)
	}
	if err := s.docStore.DocSet(collection, docID, projBytes); err != nil {
		return fmt.Errorf("persist projection: %w", err)
	}
	return nil
}

// StartCycle persists the cycle state projection and emits an
// ai.eval.cycle.started SSE event after successful persistence.
func (s *ObserveProducerService) StartCycle(ctx context.Context, userID string, route SSERoute, proj models.CycleStateProjection) error {
	if proj.CycleID == "" {
		return fmt.Errorf("observe producer: start cycle: %w", constants.ErrObserveCycleIDRequired)
	}
	if proj.CampaignID == "" {
		return fmt.Errorf("observe producer: start cycle: %w", constants.ErrObserveCampaignIDRequired)
	}
	if err := route.validate(); err != nil {
		return fmt.Errorf("observe producer: start cycle: %w", err)
	}

	proj.SchemaVersion = constants.ObserveAPIReadModelSchemaVersion
	proj.Status = models.CampaignCycleStatusRunning
	proj.Freshness = models.CampaignFreshnessActive
	now := time.Now().UTC()
	proj.ObservedAt = now
	startedAt := now
	proj.StartedAt = &startedAt

	collection := marshaler.CollectionName(constants.CollectionObserveCycleStates)
	wrapped := struct {
		UserID string `json:"user_id"`
		models.CycleStateProjection
	}{UserID: userID, CycleStateProjection: proj}
	if err := s.persistLiveProjection(collection, proj.CycleID, wrapped); err != nil {
		return fmt.Errorf("observe producer: start cycle: %w", err)
	}

	seq := s.nextSourceSequence()
	eventID := uuid.NewString()
	payload := models.EvalCycleStartedPayload{
		SchemaVersion:     constants.ObserveEventPayloadSchemaVersion,
		SourceSequence:    seq,
		EventID:           eventID,
		CycleID:           proj.CycleID,
		CampaignID:        proj.CampaignID,
		CampaignRevision:  proj.CampaignRevision,
		RoleCombinationID: proj.RoleCombinationID,
		StartedAt:         startedAt,
		ObservedAt:        now,
	}
	if err := s.emitLiveSSEEvent(route, string(constants.EventAiEvalCycleStarted), eventID, payload); err != nil {
		return fmt.Errorf("observe producer: start cycle: emit sse: %w", err)
	}
	return nil
}

// CompleteCycle persists the cycle state projection and emits an
// ai.eval.cycle.completed SSE event after successful persistence.
func (s *ObserveProducerService) CompleteCycle(ctx context.Context, userID string, route SSERoute, proj models.CycleStateProjection, terminalAttempts, assignedTasks int) error {
	if proj.CycleID == "" {
		return fmt.Errorf("observe producer: complete cycle: %w", constants.ErrObserveCycleIDRequired)
	}
	if proj.CampaignID == "" {
		return fmt.Errorf("observe producer: complete cycle: %w", constants.ErrObserveCampaignIDRequired)
	}
	if err := route.validate(); err != nil {
		return fmt.Errorf("observe producer: complete cycle: %w", err)
	}

	proj.SchemaVersion = constants.ObserveAPIReadModelSchemaVersion
	proj.Freshness = models.CampaignFreshnessActive
	now := time.Now().UTC()
	proj.ObservedAt = now
	completedAt := now
	proj.CompletedAt = &completedAt

	collection := marshaler.CollectionName(constants.CollectionObserveCycleStates)
	wrapped := struct {
		UserID string `json:"user_id"`
		models.CycleStateProjection
	}{UserID: userID, CycleStateProjection: proj}
	if err := s.persistLiveProjection(collection, proj.CycleID, wrapped); err != nil {
		return fmt.Errorf("observe producer: complete cycle: %w", err)
	}

	seq := s.nextSourceSequence()
	eventID := uuid.NewString()
	payload := models.EvalCycleCompletedPayload{
		SchemaVersion:      constants.ObserveEventPayloadSchemaVersion,
		SourceSequence:     seq,
		EventID:            eventID,
		CycleID:            proj.CycleID,
		CampaignID:         proj.CampaignID,
		CampaignRevision:   proj.CampaignRevision,
		RoleCombinationID:  proj.RoleCombinationID,
		VerificationStatus: proj.VerificationStatus,
		TerminalAttempts:   terminalAttempts,
		AssignedTasks:      assignedTasks,
		CompletedAt:        completedAt,
		ObservedAt:         now,
	}
	if err := s.emitLiveSSEEvent(route, string(constants.EventAiEvalCycleCompleted), eventID, payload); err != nil {
		return fmt.Errorf("observe producer: complete cycle: emit sse: %w", err)
	}
	return nil
}

// StartAssignment persists the assignment progress projection and emits an
// ai.eval.assignment.started SSE event after successful persistence.
func (s *ObserveProducerService) StartAssignment(ctx context.Context, userID string, route SSERoute, proj models.AssignmentProgressProjection) error {
	if proj.AssignmentID == "" {
		return fmt.Errorf("observe producer: start assignment: %w", constants.ErrObserveAssignmentIDRequired)
	}
	if proj.CampaignID == "" {
		return fmt.Errorf("observe producer: start assignment: %w", constants.ErrObserveCampaignIDRequired)
	}
	if err := route.validate(); err != nil {
		return fmt.Errorf("observe producer: start assignment: %w", err)
	}

	proj.SchemaVersion = constants.ObserveAPIReadModelSchemaVersion
	proj.Status = models.AssignmentProgressStatusRunning
	proj.Freshness = models.CampaignFreshnessActive
	now := time.Now().UTC()
	proj.ObservedAt = now
	startedAt := now
	proj.StartedAt = &startedAt

	collection := marshaler.CollectionName(constants.CollectionObserveAssignmentProgress)
	wrapped := struct {
		UserID string `json:"user_id"`
		models.AssignmentProgressProjection
	}{UserID: userID, AssignmentProgressProjection: proj}
	if err := s.persistLiveProjection(collection, proj.AssignmentID, wrapped); err != nil {
		return fmt.Errorf("observe producer: start assignment: %w", err)
	}

	seq := s.nextSourceSequence()
	eventID := uuid.NewString()
	payload := models.EvalAssignmentStartedPayload{
		SchemaVersion:  constants.ObserveEventPayloadSchemaVersion,
		SourceSequence: seq,
		EventID:        eventID,
		CycleID:        proj.CycleID,
		CampaignID:     proj.CampaignID,
		AssignmentID:   proj.AssignmentID,
		VariantID:      proj.VariantID,
		Role:           proj.Role,
		TaskID:         proj.TaskID,
		ArmID:          proj.ArmID,
		Repetition:     proj.Repetition,
		StartedAt:      startedAt,
		ObservedAt:     now,
	}
	if err := s.emitLiveSSEEvent(route, string(constants.EventAiEvalAssignmentStarted), eventID, payload); err != nil {
		return fmt.Errorf("observe producer: start assignment: emit sse: %w", err)
	}
	return nil
}

// CompleteAssignment persists the assignment progress projection and emits an
// ai.eval.assignment.completed SSE event after successful persistence.
func (s *ObserveProducerService) CompleteAssignment(ctx context.Context, userID string, route SSERoute, proj models.AssignmentProgressProjection) error {
	if proj.AssignmentID == "" {
		return fmt.Errorf("observe producer: complete assignment: %w", constants.ErrObserveAssignmentIDRequired)
	}
	if proj.CampaignID == "" {
		return fmt.Errorf("observe producer: complete assignment: %w", constants.ErrObserveCampaignIDRequired)
	}
	if err := route.validate(); err != nil {
		return fmt.Errorf("observe producer: complete assignment: %w", err)
	}

	proj.SchemaVersion = constants.ObserveAPIReadModelSchemaVersion
	proj.Freshness = models.CampaignFreshnessActive
	now := time.Now().UTC()
	proj.ObservedAt = now
	completedAt := now
	proj.CompletedAt = &completedAt

	collection := marshaler.CollectionName(constants.CollectionObserveAssignmentProgress)
	wrapped := struct {
		UserID string `json:"user_id"`
		models.AssignmentProgressProjection
	}{UserID: userID, AssignmentProgressProjection: proj}
	if err := s.persistLiveProjection(collection, proj.AssignmentID, wrapped); err != nil {
		return fmt.Errorf("observe producer: complete assignment: %w", err)
	}

	seq := s.nextSourceSequence()
	eventID := uuid.NewString()
	payload := models.EvalAssignmentCompletedPayload{
		SchemaVersion:  constants.ObserveEventPayloadSchemaVersion,
		SourceSequence: seq,
		EventID:        eventID,
		CycleID:        proj.CycleID,
		CampaignID:     proj.CampaignID,
		AssignmentID:   proj.AssignmentID,
		VariantID:      proj.VariantID,
		Role:           proj.Role,
		TaskID:         proj.TaskID,
		ArmID:          proj.ArmID,
		Repetition:     proj.Repetition,
		TerminalStatus: proj.TerminalStatus,
		CompletedAt:    completedAt,
		ObservedAt:     now,
	}
	if err := s.emitLiveSSEEvent(route, string(constants.EventAiEvalAssignmentCompleted), eventID, payload); err != nil {
		return fmt.Errorf("observe producer: complete assignment: emit sse: %w", err)
	}
	return nil
}

// RecordModelRoleInvocation persists the assignment progress projection and
// emits an ai.eval.model_role.invoked SSE event after successful persistence.
func (s *ObserveProducerService) RecordModelRoleInvocation(ctx context.Context, userID string, route SSERoute, proj models.AssignmentProgressProjection, servedModelTag, backendName, quantization string) error {
	if proj.AssignmentID == "" {
		return fmt.Errorf("observe producer: record model role: %w", constants.ErrObserveAssignmentIDRequired)
	}
	if proj.VariantID == "" {
		return fmt.Errorf("observe producer: record model role: %w", constants.ErrObserveVariantIDRequired)
	}
	if err := route.validate(); err != nil {
		return fmt.Errorf("observe producer: record model role: %w", err)
	}

	// Persist the assignment progress projection (refresh).
	proj.SchemaVersion = constants.ObserveAPIReadModelSchemaVersion
	now := time.Now().UTC()
	proj.ObservedAt = now

	collection := marshaler.CollectionName(constants.CollectionObserveAssignmentProgress)
	wrapped := struct {
		UserID string `json:"user_id"`
		models.AssignmentProgressProjection
	}{UserID: userID, AssignmentProgressProjection: proj}
	if err := s.persistLiveProjection(collection, proj.AssignmentID, wrapped); err != nil {
		return fmt.Errorf("observe producer: record model role: %w", err)
	}

	seq := s.nextSourceSequence()
	eventID := uuid.NewString()
	payload := models.EvalModelRoleInvokedPayload{
		SchemaVersion:  constants.ObserveEventPayloadSchemaVersion,
		SourceSequence: seq,
		EventID:        eventID,
		CycleID:        proj.CycleID,
		CampaignID:     proj.CampaignID,
		AssignmentID:   proj.AssignmentID,
		VariantID:      proj.VariantID,
		Role:           proj.Role,
		ServedModelTag: servedModelTag,
		BackendName:    backendName,
		Quantization:   quantization,
		InvokedAt:      now,
		ObservedAt:     now,
	}
	if err := s.emitLiveSSEEvent(route, string(constants.EventAiEvalModelRoleInvoked), eventID, payload); err != nil {
		return fmt.Errorf("observe producer: record model role: emit sse: %w", err)
	}
	return nil
}

// RecordMetricAvailability emits an ai.eval.metric.available SSE event. The
// metric projection is not persisted separately; the event carries the
// per-variant metric aggregation.
func (s *ObserveProducerService) RecordMetricAvailability(ctx context.Context, userID string, route SSERoute, payload models.EvalMetricAvailablePayload) error {
	if payload.CampaignID == "" {
		return fmt.Errorf("observe producer: record metric: %w", constants.ErrObserveCampaignIDRequired)
	}
	if payload.AssignmentID == "" {
		return fmt.Errorf("observe producer: record metric: %w", constants.ErrObserveAssignmentIDRequired)
	}
	if err := route.validate(); err != nil {
		return fmt.Errorf("observe producer: record metric: %w", err)
	}

	payload.SchemaVersion = constants.ObserveEventPayloadSchemaVersion
	seq := s.nextSourceSequence()
	payload.SourceSequence = seq
	eventID := uuid.NewString()
	payload.EventID = eventID
	if payload.ObservedAt.IsZero() {
		payload.ObservedAt = time.Now().UTC()
	}

	if err := s.emitLiveSSEEvent(route, string(constants.EventAiEvalMetricAvailable), eventID, payload); err != nil {
		return fmt.Errorf("observe producer: record metric: emit sse: %w", err)
	}
	return nil
}

// CompleteVerifier persists the verification progress projection and emits an
// ai.eval.verifier.completed SSE event after successful persistence.
func (s *ObserveProducerService) CompleteVerifier(ctx context.Context, userID string, route SSERoute, proj models.VerificationProgressProjection) error {
	if proj.CycleID == "" {
		return fmt.Errorf("observe producer: complete verifier: %w", constants.ErrObserveCycleIDRequired)
	}
	if proj.CampaignID == "" {
		return fmt.Errorf("observe producer: complete verifier: %w", constants.ErrObserveCampaignIDRequired)
	}
	if err := route.validate(); err != nil {
		return fmt.Errorf("observe producer: complete verifier: %w", err)
	}

	proj.SchemaVersion = constants.ObserveAPIReadModelSchemaVersion
	proj.Freshness = models.CampaignFreshnessActive
	now := time.Now().UTC()
	proj.ObservedAt = now
	completedAt := now
	proj.CompletedAt = &completedAt

	collection := marshaler.CollectionName(constants.CollectionObserveVerificationProgress)
	wrapped := struct {
		UserID string `json:"user_id"`
		models.VerificationProgressProjection
	}{UserID: userID, VerificationProgressProjection: proj}
	if err := s.persistLiveProjection(collection, proj.CycleID, wrapped); err != nil {
		return fmt.Errorf("observe producer: complete verifier: %w", err)
	}

	seq := s.nextSourceSequence()
	eventID := uuid.NewString()
	payload := models.EvalVerifierCompletedPayload{
		SchemaVersion:               constants.ObserveEventPayloadSchemaVersion,
		SourceSequence:              seq,
		EventID:                     eventID,
		CycleID:                     proj.CycleID,
		CampaignID:                  proj.CampaignID,
		VerificationStatus:          proj.VerificationStatus,
		VerifiedIndexGenerationHash: proj.VerifiedIndexGenerationHash,
		LayerCount:                  proj.LayerCount,
		FailureCount:                proj.FailureCount,
		CompletedAt:                 completedAt,
		ObservedAt:                  now,
	}
	if err := s.emitLiveSSEEvent(route, string(constants.EventAiEvalVerifierCompleted), eventID, payload); err != nil {
		return fmt.Errorf("observe producer: complete verifier: emit sse: %w", err)
	}
	return nil
}

// RecordProofAvailability persists the publication progress projection and
// emits an ai.eval.proof.available SSE event after successful persistence.
func (s *ObserveProducerService) RecordProofAvailability(ctx context.Context, userID string, route SSERoute, cycleID, campaignID, proofRootSHA256 string, artifactCount int) error {
	if cycleID == "" {
		return fmt.Errorf("observe producer: record proof: %w", constants.ErrObserveCycleIDRequired)
	}
	if campaignID == "" {
		return fmt.Errorf("observe producer: record proof: %w", constants.ErrObserveCampaignIDRequired)
	}
	if err := route.validate(); err != nil {
		return fmt.Errorf("observe producer: record proof: %w", err)
	}

	now := time.Now().UTC()

	// Persist the publication progress projection.
	pubProj := models.PublicationProgressProjection{
		SchemaVersion:     constants.ObserveAPIReadModelSchemaVersion,
		CycleID:           cycleID,
		CampaignID:        campaignID,
		PublicationStatus: models.PublicationStatusPending,
		Freshness:         models.CampaignFreshnessActive,
		ObservedAt:        now,
	}
	collection := marshaler.CollectionName(constants.CollectionObservePublicationProgress)
	wrapped := struct {
		UserID string `json:"user_id"`
		models.PublicationProgressProjection
	}{UserID: userID, PublicationProgressProjection: pubProj}
	if err := s.persistLiveProjection(collection, cycleID, wrapped); err != nil {
		return fmt.Errorf("observe producer: record proof: %w", err)
	}

	seq := s.nextSourceSequence()
	eventID := uuid.NewString()
	payload := models.EvalProofAvailablePayload{
		SchemaVersion:   constants.ObserveEventPayloadSchemaVersion,
		SourceSequence:  seq,
		EventID:         eventID,
		CycleID:         cycleID,
		CampaignID:      campaignID,
		ProofRootSHA256: proofRootSHA256,
		ArtifactCount:   artifactCount,
		AvailableAt:     now,
		ObservedAt:      now,
	}
	if err := s.emitLiveSSEEvent(route, string(constants.EventAiEvalProofAvailable), eventID, payload); err != nil {
		return fmt.Errorf("observe producer: record proof: emit sse: %w", err)
	}
	return nil
}

// CompletePublication persists the publication progress projection and emits
// an ai.eval.publication.completed SSE event after successful persistence.
func (s *ObserveProducerService) CompletePublication(ctx context.Context, userID string, route SSERoute, proj models.PublicationProgressProjection) error {
	if proj.CycleID == "" {
		return fmt.Errorf("observe producer: complete publication: %w", constants.ErrObserveCycleIDRequired)
	}
	if proj.CampaignID == "" {
		return fmt.Errorf("observe producer: complete publication: %w", constants.ErrObserveCampaignIDRequired)
	}
	if err := route.validate(); err != nil {
		return fmt.Errorf("observe producer: complete publication: %w", err)
	}

	proj.SchemaVersion = constants.ObserveAPIReadModelSchemaVersion
	proj.PublicationStatus = models.PublicationStatusPublished
	proj.Freshness = models.CampaignFreshnessActive
	now := time.Now().UTC()
	proj.ObservedAt = now
	completedAt := now
	proj.CompletedAt = &completedAt

	collection := marshaler.CollectionName(constants.CollectionObservePublicationProgress)
	wrapped := struct {
		UserID string `json:"user_id"`
		models.PublicationProgressProjection
	}{UserID: userID, PublicationProgressProjection: proj}
	if err := s.persistLiveProjection(collection, proj.CycleID, wrapped); err != nil {
		return fmt.Errorf("observe producer: complete publication: %w", err)
	}

	seq := s.nextSourceSequence()
	eventID := uuid.NewString()
	payload := models.EvalPublicationCompletedPayload{
		SchemaVersion:             constants.ObserveEventPayloadSchemaVersion,
		SourceSequence:            seq,
		EventID:                   eventID,
		CycleID:                   proj.CycleID,
		CampaignID:                proj.CampaignID,
		PublicationSchemaVersion:  proj.PublicationSchemaVersion,
		PublishedProjectionSHA256: proj.PublishedProjectionSHA256,
		CompletedAt:               completedAt,
		ObservedAt:                now,
	}
	if err := s.emitLiveSSEEvent(route, string(constants.EventAiEvalPublicationCompleted), eventID, payload); err != nil {
		return fmt.Errorf("observe producer: complete publication: emit sse: %w", err)
	}
	return nil
}

// EmitHeartbeat persists the source freshness projection and emits an
// ai.eval.heartbeat SSE event after successful persistence. A heartbeat
// proves source liveness only, not that any specific work is progressing.
func (s *ObserveProducerService) EmitHeartbeat(ctx context.Context, userID string, route SSERoute, sourceID, campaignID string) error {
	if sourceID == "" {
		return fmt.Errorf("observe producer: emit heartbeat: %w", constants.ErrObserveSourceIDRequired)
	}
	if err := route.validate(); err != nil {
		return fmt.Errorf("observe producer: emit heartbeat: %w", err)
	}

	now := time.Now().UTC()

	// Persist the source freshness projection.
	freshProj := models.SourceFreshnessProjection{
		SchemaVersion:   constants.ObserveAPIReadModelSchemaVersion,
		SourceID:        sourceID,
		CampaignID:      campaignID,
		Freshness:       models.CampaignFreshnessActive,
		LastHeartbeatAt: now,
		ObservedAt:      now,
	}
	collection := marshaler.CollectionName(constants.CollectionObserveSourceFreshness)
	wrapped := struct {
		UserID string `json:"user_id"`
		models.SourceFreshnessProjection
	}{UserID: userID, SourceFreshnessProjection: freshProj}
	if err := s.persistLiveProjection(collection, sourceID, wrapped); err != nil {
		return fmt.Errorf("observe producer: emit heartbeat: %w", err)
	}

	seq := s.nextSourceSequence()
	eventID := uuid.NewString()
	payload := models.EvalHeartbeatPayload{
		SchemaVersion:  constants.ObserveEventPayloadSchemaVersion,
		SourceSequence: seq,
		EventID:        eventID,
		SourceID:       sourceID,
		ObservedAt:     now,
	}
	if err := s.emitLiveSSEEvent(route, string(constants.EventAiEvalHeartbeat), eventID, payload); err != nil {
		return fmt.Errorf("observe producer: emit heartbeat: emit sse: %w", err)
	}
	return nil
}

// RequestStop persists the supervisor state projection and emits an
// ai.eval.stop.requested SSE event after successful persistence.
func (s *ObserveProducerService) RequestStop(ctx context.Context, userID string, route SSERoute, supervisorID, campaignID string, reason models.StopReason, scope models.StopScope) error {
	if campaignID == "" {
		return fmt.Errorf("observe producer: request stop: %w", constants.ErrObserveCampaignIDRequired)
	}
	if err := route.validate(); err != nil {
		return fmt.Errorf("observe producer: request stop: %w", err)
	}

	now := time.Now().UTC()

	// Determine supervisor status from the stop reason.
	status := models.SupervisorStatusStopped
	if reason != models.StopReasonGraceful {
		status = models.SupervisorStatusSafetyStopped
	}
	freshness := models.CampaignFreshnessIntentionallyStopped
	if reason != models.StopReasonGraceful {
		freshness = models.CampaignFreshnessSafetyStopped
	}

	// Persist the supervisor state projection.
	supProj := models.SupervisorStateProjection{
		SchemaVersion: constants.ObserveAPIReadModelSchemaVersion,
		SupervisorID:  supervisorID,
		CampaignID:    campaignID,
		Status:        status,
		StopReason:    reason,
		Freshness:     freshness,
		ObservedAt:    now,
	}
	collection := marshaler.CollectionName(constants.CollectionObserveSupervisorStates)
	docID := campaignID
	if supervisorID != "" {
		docID = supervisorID
	}
	wrapped := struct {
		UserID string `json:"user_id"`
		models.SupervisorStateProjection
	}{UserID: userID, SupervisorStateProjection: supProj}
	if err := s.persistLiveProjection(collection, docID, wrapped); err != nil {
		return fmt.Errorf("observe producer: request stop: %w", err)
	}

	seq := s.nextSourceSequence()
	eventID := uuid.NewString()
	payload := models.EvalStopRequestedPayload{
		SchemaVersion:  constants.ObserveEventPayloadSchemaVersion,
		SourceSequence: seq,
		EventID:        eventID,
		CampaignID:     campaignID,
		StopReason:     reason,
		StopScope:      scope,
		RequestedAt:    now,
		ObservedAt:     now,
	}
	if err := s.emitLiveSSEEvent(route, string(constants.EventAiEvalStopRequested), eventID, payload); err != nil {
		return fmt.Errorf("observe producer: request stop: emit sse: %w", err)
	}
	return nil
}
