// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package gateway

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/marshaler"
	"github.com/g8e-ai/g8e/v2/internal/models"
)

// Observe pagination defaults. Limits are bounded to prevent unbounded
// queries; cursors are opaque base64url-encoded JSON for stable pagination.
const (
	ObserveDefaultLimit    = 20
	ObserveMaxLimit        = 100
	ObserveMinLimit        = 1
	ObserveBootstrapRecent = 10
	ObserveQueryCap        = 500
)

// observeCursor is the stable pagination cursor. It encodes the sort key of
// the last item on the current page so the next page resumes after it. The
// cursor is base64url-encoded JSON, opaque to the client.
type observeCursor struct {
	ObservedAt time.Time `json:"observed_at"`
	ID         string    `json:"id"`
}

// encodeCursor serializes the cursor to a base64url string.
func encodeCursor(c observeCursor) (string, error) {
	b, err := json.Marshal(c)
	if err != nil {
		return "", fmt.Errorf("observe: encode cursor: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

// decodeCursor parses a base64url cursor string. An empty cursor decodes to
// a zero-value cursor (start of list).
func decodeCursor(s string) (observeCursor, error) {
	if s == "" {
		return observeCursor{}, nil
	}
	b, err := base64.RawURLEncoding.DecodeString(s)
	if err != nil {
		return observeCursor{}, fmt.Errorf("%w: %w", constants.ErrObserveCursorInvalid, err)
	}
	var c observeCursor
	if err := json.Unmarshal(b, &c); err != nil {
		return observeCursor{}, fmt.Errorf("%w: %w", constants.ErrObserveCursorInvalid, err)
	}
	return c, nil
}

// unmarshalDocData re-serializes a Document's Data map (map[string]json.RawMessage)
// to JSON bytes and unmarshals into target. Document.Data is a field map, not
// raw JSON, so it must be marshaled back to bytes before decoding into a typed
// struct.
func unmarshalDocData(doc *models.Document, target any) error {
	b, err := json.Marshal(doc.Data)
	if err != nil {
		return fmt.Errorf("observe: marshal document data: %w", err)
	}
	if err := json.Unmarshal(b, target); err != nil {
		return fmt.Errorf("observe: unmarshal document data: %w", err)
	}
	return nil
}

// runProjection is the persisted run projection document. It contains the
// user_id ownership field plus all fields needed to derive both RunSummary
// (list endpoint) and RunDetail (detail endpoint). Phase 3 producers write
// these projections after successful persistence.
type runProjection struct {
	UserID            string                    `json:"user_id"`
	SchemaVersion     string                    `json:"schema_version"`
	RunID             string                    `json:"run_id"`
	RunKind           models.RunKind            `json:"run_kind"`
	DisplayName       string                    `json:"display_name"`
	Status            models.RunLifecycleStatus `json:"status"`
	ActiveTaskID      string                    `json:"active_task_id,omitempty"`
	CompletedTasks    int                       `json:"completed_tasks"`
	TotalTasks        int                       `json:"total_tasks"`
	Tasks             []models.RunTask          `json:"tasks"`
	EvidenceSafeLinks []models.EvidenceSafeLink `json:"evidence_safe_links"`
	StartedAt         *time.Time                `json:"started_at,omitempty"`
	EndedAt           *time.Time                `json:"ended_at,omitempty"`
	HasReceipts       bool                      `json:"has_receipts"`
	EvidenceCount     int                       `json:"evidence_count"`
	ObservedAt        time.Time                 `json:"observed_at"`
}

// toSummary derives a RunSummary from the projection.
func (r *runProjection) toSummary() models.RunSummary {
	return models.RunSummary{
		SchemaVersion:  r.SchemaVersion,
		RunID:          r.RunID,
		RunKind:        r.RunKind,
		DisplayName:    r.DisplayName,
		Status:         r.Status,
		ActiveTaskID:   r.ActiveTaskID,
		CompletedTasks: r.CompletedTasks,
		TotalTasks:     r.TotalTasks,
		StartedAt:      r.StartedAt,
		EndedAt:        r.EndedAt,
		HasReceipts:    r.HasReceipts,
		EvidenceCount:  r.EvidenceCount,
		ObservedAt:     r.ObservedAt,
	}
}

// toDetail derives a RunDetail from the projection.
func (r *runProjection) toDetail() models.RunDetail {
	tasks := r.Tasks
	if tasks == nil {
		tasks = []models.RunTask{}
	}
	links := r.EvidenceSafeLinks
	if links == nil {
		links = []models.EvidenceSafeLink{}
	}
	return models.RunDetail{
		SchemaVersion:     r.SchemaVersion,
		RunID:             r.RunID,
		RunKind:           r.RunKind,
		DisplayName:       r.DisplayName,
		Status:            r.Status,
		ActiveTaskID:      r.ActiveTaskID,
		CompletedTasks:    r.CompletedTasks,
		TotalTasks:        r.TotalTasks,
		Tasks:             tasks,
		EvidenceSafeLinks: links,
		StartedAt:         r.StartedAt,
		EndedAt:           r.EndedAt,
		ObservedAt:        r.ObservedAt,
	}
}

// agentStateProjection is the persisted agent state projection document. It
// contains the user_id ownership field plus all AgentStateProjection fields.
// Phase 3 producers write these projections after successful persistence.
type agentStateProjection struct {
	UserID string `json:"user_id"`
	models.AgentStateProjection
}

// evalProjection is the persisted eval projection document. It contains the
// user_id ownership field plus all fields needed to derive both EvalSummary
// (list endpoint) and EvalDetail (detail endpoint). Phase 4 producers write
// these projections after successful publication.
type evalProjection struct {
	UserID string `json:"user_id"`
	models.EvalDetail
}

// toSummary derives an EvalSummary from the projection.
func (e *evalProjection) toSummary() models.EvalSummary {
	return models.EvalSummary{
		SchemaVersion:             e.SchemaVersion,
		RunID:                     e.RunID,
		SuiteID:                   e.SuiteID,
		SuiteVersion:              e.SuiteVersion,
		ArmID:                     e.ArmID,
		Status:                    e.Status,
		VerificationStatus:        e.VerificationStatus,
		ReceiptCount:              e.ReceiptCount,
		MetricCount:               len(e.Metrics),
		PublishedProjectionSHA256: e.PublishedProjectionSHA256,
		CompletedAt:               e.CompletedAt,
		ObservedAt:                e.ObservedAt,
	}
}

// downloadProjection is the persisted download artifact document. It
// contains the user_id ownership field plus all DownloadArtifact fields.
// Phase 4 producers write these projections after successful publication.
type downloadProjection struct {
	UserID string `json:"user_id"`
	models.DownloadArtifact
}

// ObserveService provides user-scoped, read-only access to observe
// projections. Every method applies user_id ownership scoping and returns
// honest empty or unavailable states when no data exists. The service never
// exposes user_id in its responses; the ownership field is stripped before
// serializing wire models.
type ObserveService struct {
	docStore *DocumentStoreService
	logger   *slog.Logger
}

// NewObserveService creates a new ObserveService backed by the given document
// store.
func NewObserveService(docStore *DocumentStoreService, logger *slog.Logger) *ObserveService {
	return &ObserveService{
		docStore: docStore,
		logger:   logger,
	}
}

// GetBootstrapSnapshot returns one bounded initial snapshot for first paint.
// It contains agents, active run, overview counters, recent runs, latest eval
// summaries, measurements, and download metadata. All measurement cards
// remain unavailable until a real host telemetry collector exists (Phase 3).
func (s *ObserveService) GetBootstrapSnapshot(ctx context.Context, userID string) (*models.ObserveBootstrapSnapshot, error) {
	agents, err := s.listAgentStates(userID)
	if err != nil {
		return nil, fmt.Errorf("observe: bootstrap: list agents: %w", err)
	}

	runs, err := s.listRuns(userID, observeCursor{}, ObserveBootstrapRecent)
	if err != nil {
		return nil, fmt.Errorf("observe: bootstrap: list runs: %w", err)
	}
	if len(runs) > ObserveBootstrapRecent {
		runs = runs[:ObserveBootstrapRecent]
	}

	evals, err := s.listEvals(userID, observeCursor{}, ObserveBootstrapRecent)
	if err != nil {
		return nil, fmt.Errorf("observe: bootstrap: list evals: %w", err)
	}
	if len(evals) > ObserveBootstrapRecent {
		evals = evals[:ObserveBootstrapRecent]
	}

	downloads, err := s.listDownloads(userID, observeCursor{}, ObserveBootstrapRecent)
	if err != nil {
		return nil, fmt.Errorf("observe: bootstrap: list downloads: %w", err)
	}
	if len(downloads) > ObserveBootstrapRecent {
		downloads = downloads[:ObserveBootstrapRecent]
	}

	var activeRun *models.RunSummary
	for i := range runs {
		if runs[i].Status == models.RunLifecycleStatusRunning || runs[i].Status == models.RunLifecycleStatusQueued || runs[i].Status == models.RunLifecycleStatusWaiting {
			activeRun = &runs[i]
			break
		}
	}

	agentsRunning := 0
	for _, a := range agents {
		if a.Status == models.AgentLifecycleStatusRunning || a.Status == models.AgentLifecycleStatusQueued || a.Status == models.AgentLifecycleStatusWaiting {
			agentsRunning++
		}
	}

	tasksInQueue := 0
	for _, r := range runs {
		if r.Status == models.RunLifecycleStatusQueued || r.Status == models.RunLifecycleStatusRunning || r.Status == models.RunLifecycleStatusWaiting {
			tasksInQueue += r.TotalTasks - r.CompletedTasks
		}
	}

	now := time.Now().UTC()
	return &models.ObserveBootstrapSnapshot{
		SchemaVersion: constants.ObserveAPIReadModelSchemaVersion,
		Agents:        agents,
		ActiveRun:     activeRun,
		Overview: models.OverviewCounters{
			SchemaVersion:          constants.ObserveAPIReadModelSchemaVersion,
			AgentsRunning:          agentsRunning,
			AgentsRunningFreshness: s.agentFreshness(agents, now),
			TasksInQueue:           tasksInQueue,
			TasksInQueueFreshness:  s.runsFreshness(runs, now),
			GeneratedAt:            now,
		},
		Measurements: models.OverviewMeasurements{
			SchemaVersion: constants.ObserveMeasurementSchemaVersion,
		},
		RecentRuns:  runs,
		LatestEvals: evals,
		Downloads:   downloads,
		GeneratedAt: now,
	}, nil
}

// ListRuns returns a paginated list of read-only run summaries owned by the
// authenticated user. Ordering is observed_at DESC, run_id DESC for
// deterministic pagination. The cursor is opaque and base64url-encoded.
func (s *ObserveService) ListRuns(ctx context.Context, userID, cursorStr string, limit int) (*models.ObservePage, error) {
	limit = clampLimit(limit)
	cursor, err := decodeCursor(cursorStr)
	if err != nil {
		return nil, err
	}
	runs, err := s.listRuns(userID, cursor, limit)
	if err != nil {
		return nil, fmt.Errorf("observe: list runs: %w", err)
	}
	return s.paginateRuns(runs, limit)
}

// GetRun returns a typed run detail projection with tasks and evidence-safe
// links. Returns constants.ErrObserveRunNotFound if the run does not exist or
// is owned by a different user.
func (s *ObserveService) GetRun(ctx context.Context, userID, runID string) (*models.RunDetail, error) {
	proj, err := s.getRunProjection(userID, runID)
	if err != nil {
		return nil, err
	}
	if proj == nil {
		return nil, constants.ErrObserveRunNotFound
	}
	detail := proj.toDetail()
	return &detail, nil
}

// ListEvals returns a paginated list of eval run projections owned by the
// authenticated user.
func (s *ObserveService) ListEvals(ctx context.Context, userID, cursorStr string, limit int) (*models.ObservePage, error) {
	limit = clampLimit(limit)
	cursor, err := decodeCursor(cursorStr)
	if err != nil {
		return nil, err
	}
	evals, err := s.listEvals(userID, cursor, limit)
	if err != nil {
		return nil, fmt.Errorf("observe: list evals: %w", err)
	}
	return s.paginateEvals(evals, limit)
}

// GetEval returns a typed eval detail projection with metrics, status, and
// verification boundary. Returns constants.ErrObserveEvalNotFound if the eval
// does not exist or is owned by a different user.
func (s *ObserveService) GetEval(ctx context.Context, userID, runID string) (*models.EvalDetail, error) {
	proj, err := s.getEvalProjection(userID, runID)
	if err != nil {
		return nil, err
	}
	if proj == nil {
		return nil, constants.ErrObserveEvalNotFound
	}
	detail := proj.EvalDetail
	return &detail, nil
}

// ListDownloads returns a paginated list of allowlisted download artifacts
// owned by the authenticated user.
func (s *ObserveService) ListDownloads(ctx context.Context, userID, cursorStr string, limit int) (*models.ObservePage, error) {
	limit = clampLimit(limit)
	cursor, err := decodeCursor(cursorStr)
	if err != nil {
		return nil, err
	}
	downloads, err := s.listDownloads(userID, cursor, limit)
	if err != nil {
		return nil, fmt.Errorf("observe: list downloads: %w", err)
	}
	return s.paginateDownloads(downloads, limit)
}

// GetDownload returns a single allowlisted download artifact owned by the
// authenticated user. Returns constants.ErrObserveDownloadNotFound if the
// artifact does not exist or is owned by a different user.
func (s *ObserveService) GetDownload(ctx context.Context, userID, artifactID string) (*models.DownloadArtifact, error) {
	proj, err := s.getDownloadProjection(userID, artifactID)
	if err != nil {
		return nil, err
	}
	if proj == nil {
		return nil, constants.ErrObserveDownloadNotFound
	}
	artifact := proj.DownloadArtifact
	return &artifact, nil
}

// listAgentStates fetches all agent state projections for the user, ordered
// by observed_at DESC. Returns an empty slice (not nil) when no projections
// exist.
func (s *ObserveService) listAgentStates(userID string) ([]models.AgentStateProjection, error) {
	docs, err := s.docStore.DocQuery(
		marshaler.CollectionName(constants.CollectionObserveAgentStates),
		userFilter(userID),
		"observed_at DESC",
		ObserveQueryCap,
	)
	if err != nil {
		return nil, fmt.Errorf("query agent states: %w", err)
	}
	agents := make([]models.AgentStateProjection, 0, len(docs))
	for _, doc := range docs {
		var proj agentStateProjection
		if err := unmarshalDocData(doc, &proj); err != nil {
			s.logger.Warn("observe: unmarshal agent state projection", "error", err, "doc_id", doc.ID)
			continue
		}
		agents = append(agents, proj.AgentStateProjection)
	}
	return agents, nil
}

// listRuns fetches run projections for the user, ordered by observed_at DESC,
// run_id DESC, and applies cursor-based filtering. Returns an empty slice
// (not nil) when no projections exist.
func (s *ObserveService) listRuns(userID string, cursor observeCursor, limit int) ([]models.RunSummary, error) {
	docs, err := s.docStore.DocQuery(
		marshaler.CollectionName(constants.CollectionObserveRuns),
		userFilter(userID),
		"observed_at DESC",
		ObserveQueryCap,
	)
	if err != nil {
		return nil, fmt.Errorf("query runs: %w", err)
	}
	projections := make([]runProjection, 0, len(docs))
	for _, doc := range docs {
		var proj runProjection
		if err := unmarshalDocData(doc, &proj); err != nil {
			s.logger.Warn("observe: unmarshal run projection", "error", err, "doc_id", doc.ID)
			continue
		}
		projections = append(projections, proj)
	}
	sortRuns(projections)
	filtered := filterByCursor(projections, cursor, func(p runProjection) (time.Time, string) {
		return p.ObservedAt, p.RunID
	})
	// Fetch one extra item beyond the requested limit so the paginator can
	// detect has_more. The paginator truncates back to limit.
	fetchLimit := limit
	if limit > 0 {
		fetchLimit = limit + 1
	}
	if fetchLimit > 0 && len(filtered) > fetchLimit {
		filtered = filtered[:fetchLimit]
	}
	summaries := make([]models.RunSummary, 0, len(filtered))
	for i := range filtered {
		summaries = append(summaries, filtered[i].toSummary())
	}
	return summaries, nil
}

// listEvals fetches eval projections for the user, ordered by observed_at
// DESC, run_id DESC, and applies cursor-based filtering.
func (s *ObserveService) listEvals(userID string, cursor observeCursor, limit int) ([]models.EvalSummary, error) {
	docs, err := s.docStore.DocQuery(
		marshaler.CollectionName(constants.CollectionObserveEvals),
		userFilter(userID),
		"observed_at DESC",
		ObserveQueryCap,
	)
	if err != nil {
		return nil, fmt.Errorf("query evals: %w", err)
	}
	projections := make([]evalProjection, 0, len(docs))
	for _, doc := range docs {
		var proj evalProjection
		if err := unmarshalDocData(doc, &proj); err != nil {
			s.logger.Warn("observe: unmarshal eval projection", "error", err, "doc_id", doc.ID)
			continue
		}
		projections = append(projections, proj)
	}
	sortEvals(projections)
	filtered := filterByCursor(projections, cursor, func(p evalProjection) (time.Time, string) {
		return p.ObservedAt, p.RunID
	})
	// Fetch one extra item beyond the requested limit so the paginator can
	// detect has_more. The paginator truncates back to limit.
	fetchLimit := limit
	if limit > 0 {
		fetchLimit = limit + 1
	}
	if fetchLimit > 0 && len(filtered) > fetchLimit {
		filtered = filtered[:fetchLimit]
	}
	summaries := make([]models.EvalSummary, 0, len(filtered))
	for i := range filtered {
		summaries = append(summaries, filtered[i].toSummary())
	}
	return summaries, nil
}

// listDownloads fetches download artifacts for the user, ordered by
// generated_at DESC, artifact_id DESC, and applies cursor-based filtering.
func (s *ObserveService) listDownloads(userID string, cursor observeCursor, limit int) ([]models.DownloadArtifact, error) {
	docs, err := s.docStore.DocQuery(
		marshaler.CollectionName(constants.CollectionObserveDownloads),
		userFilter(userID),
		"generated_at DESC",
		ObserveQueryCap,
	)
	if err != nil {
		return nil, fmt.Errorf("query downloads: %w", err)
	}
	projections := make([]downloadProjection, 0, len(docs))
	for _, doc := range docs {
		var proj downloadProjection
		if err := unmarshalDocData(doc, &proj); err != nil {
			s.logger.Warn("observe: unmarshal download projection", "error", err, "doc_id", doc.ID)
			continue
		}
		projections = append(projections, proj)
	}
	sortDownloads(projections)
	filtered := filterByCursor(projections, cursor, func(p downloadProjection) (time.Time, string) {
		return p.GeneratedAt, p.ArtifactID
	})
	// Fetch one extra item beyond the requested limit so the paginator can
	// detect has_more. The paginator truncates back to limit.
	fetchLimit := limit
	if limit > 0 {
		fetchLimit = limit + 1
	}
	if fetchLimit > 0 && len(filtered) > fetchLimit {
		filtered = filtered[:fetchLimit]
	}
	artifacts := make([]models.DownloadArtifact, 0, len(filtered))
	for i := range filtered {
		artifacts = append(artifacts, filtered[i].DownloadArtifact)
	}
	return artifacts, nil
}

// getRunProjection fetches a single run projection owned by userID. Returns
// nil (not an error) if the run does not exist or is owned by a different
// user.
func (s *ObserveService) getRunProjection(userID, runID string) (*runProjection, error) {
	doc, err := s.docStore.DocGet(marshaler.CollectionName(constants.CollectionObserveRuns), runID)
	if err != nil {
		return nil, fmt.Errorf("observe: get run %s: %w", runID, err)
	}
	if doc == nil {
		return nil, nil
	}
	var proj runProjection
	if err := unmarshalDocData(doc, &proj); err != nil {
		return nil, fmt.Errorf("observe: unmarshal run %s: %w", runID, err)
	}
	if proj.UserID != userID {
		return nil, nil
	}
	return &proj, nil
}

// getEvalProjection fetches a single eval projection owned by userID. Returns
// nil (not an error) if the eval does not exist or is owned by a different
// user.
func (s *ObserveService) getEvalProjection(userID, runID string) (*evalProjection, error) {
	doc, err := s.docStore.DocGet(marshaler.CollectionName(constants.CollectionObserveEvals), runID)
	if err != nil {
		return nil, fmt.Errorf("observe: get eval %s: %w", runID, err)
	}
	if doc == nil {
		return nil, nil
	}
	var proj evalProjection
	if err := unmarshalDocData(doc, &proj); err != nil {
		return nil, fmt.Errorf("observe: unmarshal eval %s: %w", runID, err)
	}
	if proj.UserID != userID {
		return nil, nil
	}
	return &proj, nil
}

// getDownloadProjection fetches a single download artifact owned by userID.
// Returns nil (not an error) if the artifact does not exist or is owned by a
// different user.
func (s *ObserveService) getDownloadProjection(userID, artifactID string) (*downloadProjection, error) {
	doc, err := s.docStore.DocGet(marshaler.CollectionName(constants.CollectionObserveDownloads), artifactID)
	if err != nil {
		return nil, fmt.Errorf("observe: get download %s: %w", artifactID, err)
	}
	if doc == nil {
		return nil, nil
	}
	var proj downloadProjection
	if err := unmarshalDocData(doc, &proj); err != nil {
		return nil, fmt.Errorf("observe: unmarshal download %s: %w", artifactID, err)
	}
	if proj.UserID != userID {
		return nil, nil
	}
	return &proj, nil
}

// paginateRuns wraps a slice of RunSummary items in an ObservePage with
// cursor and has_more.
func (s *ObserveService) paginateRuns(runs []models.RunSummary, limit int) (*models.ObservePage, error) {
	hasMore := len(runs) > limit
	if hasMore {
		runs = runs[:limit]
	}
	items, err := json.Marshal(runs)
	if err != nil {
		return nil, fmt.Errorf("observe: marshal runs page: %w", err)
	}
	page := &models.ObservePage{
		SchemaVersion: constants.ObserveAPIReadModelSchemaVersion,
		Items:         items,
		HasMore:       hasMore,
		Limit:         limit,
	}
	if hasMore && len(runs) > 0 {
		last := runs[len(runs)-1]
		cursor, err := encodeCursor(observeCursor{ObservedAt: last.ObservedAt, ID: last.RunID})
		if err != nil {
			return nil, err
		}
		page.Cursor = cursor
	}
	return page, nil
}

// paginateEvals wraps a slice of EvalSummary items in an ObservePage.
func (s *ObserveService) paginateEvals(evals []models.EvalSummary, limit int) (*models.ObservePage, error) {
	hasMore := len(evals) > limit
	if hasMore {
		evals = evals[:limit]
	}
	items, err := json.Marshal(evals)
	if err != nil {
		return nil, fmt.Errorf("observe: marshal evals page: %w", err)
	}
	page := &models.ObservePage{
		SchemaVersion: constants.ObserveAPIReadModelSchemaVersion,
		Items:         items,
		HasMore:       hasMore,
		Limit:         limit,
	}
	if hasMore && len(evals) > 0 {
		last := evals[len(evals)-1]
		cursor, err := encodeCursor(observeCursor{ObservedAt: last.ObservedAt, ID: last.RunID})
		if err != nil {
			return nil, err
		}
		page.Cursor = cursor
	}
	return page, nil
}

// paginateDownloads wraps a slice of DownloadArtifact items in an ObservePage.
func (s *ObserveService) paginateDownloads(downloads []models.DownloadArtifact, limit int) (*models.ObservePage, error) {
	hasMore := len(downloads) > limit
	if hasMore {
		downloads = downloads[:limit]
	}
	items, err := json.Marshal(downloads)
	if err != nil {
		return nil, fmt.Errorf("observe: marshal downloads page: %w", err)
	}
	page := &models.ObservePage{
		SchemaVersion: constants.ObserveAPIReadModelSchemaVersion,
		Items:         items,
		HasMore:       hasMore,
		Limit:         limit,
	}
	if hasMore && len(downloads) > 0 {
		last := downloads[len(downloads)-1]
		cursor, err := encodeCursor(observeCursor{ObservedAt: last.GeneratedAt, ID: last.ArtifactID})
		if err != nil {
			return nil, err
		}
		page.Cursor = cursor
	}
	return page, nil
}

// agentFreshness returns the freshness status for the agents counter based on
// the most recent agent observation time.
func (s *ObserveService) agentFreshness(agents []models.AgentStateProjection, now time.Time) models.SnapshotFreshness {
	if len(agents) == 0 {
		return models.SnapshotFreshnessUnavailable
	}
	var latest time.Time
	for _, a := range agents {
		if a.ObservedAt.After(latest) {
			latest = a.ObservedAt
		}
	}
	if latest.IsZero() {
		return models.SnapshotFreshnessUnavailable
	}
	if now.Sub(latest) > 5*time.Minute {
		return models.SnapshotFreshnessStale
	}
	return models.SnapshotFreshnessObserved
}

// runsFreshness returns the freshness status for the tasks counter based on
// the most recent run observation time.
func (s *ObserveService) runsFreshness(runs []models.RunSummary, now time.Time) models.SnapshotFreshness {
	if len(runs) == 0 {
		return models.SnapshotFreshnessUnavailable
	}
	var latest time.Time
	for _, r := range runs {
		if r.ObservedAt.After(latest) {
			latest = r.ObservedAt
		}
	}
	if latest.IsZero() {
		return models.SnapshotFreshnessUnavailable
	}
	if now.Sub(latest) > 5*time.Minute {
		return models.SnapshotFreshnessStale
	}
	return models.SnapshotFreshnessObserved
}

// clampLimit bounds the requested page size to the allowed range. A limit of
// 0 defaults to ObserveDefaultLimit.
func clampLimit(limit int) int {
	if limit <= 0 {
		return ObserveDefaultLimit
	}
	if limit > ObserveMaxLimit {
		return ObserveMaxLimit
	}
	if limit < ObserveMinLimit {
		return ObserveMinLimit
	}
	return limit
}

// userFilter returns a DocFilter that scopes a query to documents owned by
// the given user. This is the ownership scoping applied to every observe
// query.
func userFilter(userID string) []models.DocFilter {
	return []models.DocFilter{
		{Field: "user_id", Op: "==", Value: mustJSONString(userID)},
	}
}

// mustJSONString encodes a string as a JSON string value.
func mustJSONString(s string) json.RawMessage {
	b, _ := json.Marshal(s)
	return b
}

// sortRuns sorts run projections by observed_at DESC, run_id DESC in place.
func sortRuns(runs []runProjection) {
	for i := 1; i < len(runs); i++ {
		for j := i; j > 0; j-- {
			if compareSortKey(runs[j].ObservedAt, runs[j].RunID, runs[j-1].ObservedAt, runs[j-1].RunID) > 0 {
				runs[j], runs[j-1] = runs[j-1], runs[j]
			} else {
				break
			}
		}
	}
}

// sortEvals sorts eval projections by observed_at DESC, run_id DESC in place.
func sortEvals(evals []evalProjection) {
	for i := 1; i < len(evals); i++ {
		for j := i; j > 0; j-- {
			if compareSortKey(evals[j].ObservedAt, evals[j].RunID, evals[j-1].ObservedAt, evals[j-1].RunID) > 0 {
				evals[j], evals[j-1] = evals[j-1], evals[j]
			} else {
				break
			}
		}
	}
}

// sortDownloads sorts download projections by generated_at DESC, artifact_id
// DESC in place.
func sortDownloads(downloads []downloadProjection) {
	for i := 1; i < len(downloads); i++ {
		for j := i; j > 0; j-- {
			if compareSortKey(downloads[j].GeneratedAt, downloads[j].ArtifactID, downloads[j-1].GeneratedAt, downloads[j-1].ArtifactID) > 0 {
				downloads[j], downloads[j-1] = downloads[j-1], downloads[j]
			} else {
				break
			}
		}
	}
}

// compareSortKey compares two (timestamp, id) sort keys. Returns >0 if a
// should come before b (DESC ordering), <0 if a should come after b, 0 if
// equal.
func compareSortKey(aTime time.Time, aID string, bTime time.Time, bID string) int {
	if aTime.After(bTime) {
		return 1
	}
	if aTime.Before(bTime) {
		return -1
	}
	if aID > bID {
		return 1
	}
	if aID < bID {
		return -1
	}
	return 0
}

// filterByCursor applies cursor-based pagination to a sorted slice. It
// returns items that come strictly after the cursor position. The getKey
// function extracts the (timestamp, id) sort key from each item.
func filterByCursor[T any](items []T, cursor observeCursor, getKey func(T) (time.Time, string)) []T {
	if cursor.ID == "" && cursor.ObservedAt.IsZero() {
		return items
	}
	result := make([]T, 0, len(items))
	for _, item := range items {
		t, id := getKey(item)
		if compareSortKey(t, id, cursor.ObservedAt, cursor.ID) < 0 {
			result = append(result, item)
		}
	}
	return result
}
