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
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/marshaler"
	"github.com/g8e-ai/g8e/v2/internal/models"
	"github.com/g8e-ai/g8e/v2/internal/response"
	"github.com/g8e-ai/g8e/v2/internal/services/sqliteutil"
	"github.com/g8e-ai/g8e/v2/internal/testutil"
)

func TestEncodeDecodeCursor_RoundTrip(t *testing.T) {
	original := observeCursor{
		ObservedAt: time.Date(2026, 9, 9, 12, 0, 0, 0, time.UTC),
		ID:         "run-123",
	}
	encoded, err := encodeCursor(original)
	require.NoError(t, err)
	assert.NotEmpty(t, encoded)

	decoded, err := decodeCursor(encoded)
	require.NoError(t, err)
	assert.Equal(t, original, decoded)
}

func TestDecodeCursor_EmptyStringIsStart(t *testing.T) {
	decoded, err := decodeCursor("")
	require.NoError(t, err)
	assert.True(t, decoded.ObservedAt.IsZero())
	assert.Empty(t, decoded.ID)
}

func TestDecodeCursor_InvalidBase64Rejected(t *testing.T) {
	_, err := decodeCursor("!!!not-base64!!!")
	require.Error(t, err)
	assert.True(t, errors.Is(err, constants.ErrObserveCursorInvalid), "invalid cursor should wrap ErrObserveCursorInvalid")
}

func TestDecodeCursor_InvalidJSONRejected(t *testing.T) {
	// Valid base64url but not valid cursor JSON.
	encoded := mustEncodeBase64(t, []byte("not-json"))
	_, err := decodeCursor(encoded)
	require.Error(t, err)
	assert.True(t, errors.Is(err, constants.ErrObserveCursorInvalid), "invalid cursor JSON should wrap ErrObserveCursorInvalid")
}

func TestClampLimit_Bounds(t *testing.T) {
	tests := []struct {
		name  string
		input int
		want  int
	}{
		{"zero defaults", 0, ObserveDefaultLimit},
		{"negative defaults", -5, ObserveDefaultLimit},
		{"one clamped to min", 1, ObserveMinLimit},
		{"max allowed", ObserveMaxLimit, ObserveMaxLimit},
		{"over max clamped", ObserveMaxLimit + 1, ObserveMaxLimit},
		{"within range passes", 50, 50},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, clampLimit(tc.input))
		})
	}
}

func TestExtractID_PathParameterExtraction(t *testing.T) {
	tests := []struct {
		name   string
		prefix string
		path   string
		want   string
	}{
		{"runs id", constants.APIPaths.ObserveRunsByID, constants.APIPaths.ObserveRunsByID + "run-abc", "run-abc"},
		{"evals id", constants.APIPaths.ObserveEvalsByID, constants.APIPaths.ObserveEvalsByID + "eval-xyz", "eval-xyz"},
		{"downloads id", constants.APIPaths.ObserveDownloadsByID, constants.APIPaths.ObserveDownloadsByID + "art-1", "art-1"},
		{"no id empty", constants.APIPaths.ObserveRunsByID, constants.APIPaths.ObserveRunsByID, ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, extractID(tc.prefix, tc.path))
		})
	}
}

func TestCompareSortKey_DescOrdering(t *testing.T) {
	newer := time.Date(2026, 9, 9, 12, 0, 0, 0, time.UTC)
	older := time.Date(2026, 9, 9, 11, 0, 0, 0, time.UTC)
	// Newer timestamp sorts before older (DESC).
	assert.Greater(t, compareSortKey(newer, "a", older, "b"), 0)
	// Equal timestamp: larger ID sorts first (DESC).
	assert.Greater(t, compareSortKey(newer, "b", newer, "a"), 0)
	// Fully equal keys compare equal.
	assert.Equal(t, 0, compareSortKey(newer, "a", newer, "a"))
}

func TestFilterByCursor_EmptyCursorReturnsAll(t *testing.T) {
	items := []runProjection{
		{RunID: "r1", ObservedAt: time.Date(2026, 9, 9, 12, 0, 0, 0, time.UTC)},
		{RunID: "r2", ObservedAt: time.Date(2026, 9, 9, 11, 0, 0, 0, time.UTC)},
	}
	got := filterByCursor(items, observeCursor{}, func(p runProjection) (time.Time, string) {
		return p.ObservedAt, p.RunID
	})
	assert.Len(t, got, 2)
}

func TestFilterByCursor_ExcludesAtAndBeforeCursor(t *testing.T) {
	t1 := time.Date(2026, 9, 9, 12, 0, 0, 0, time.UTC)
	t2 := time.Date(2026, 9, 9, 11, 0, 0, 0, time.UTC)
	t3 := time.Date(2026, 9, 9, 10, 0, 0, 0, time.UTC)
	items := []runProjection{
		{RunID: "r1", ObservedAt: t1},
		{RunID: "r2", ObservedAt: t2},
		{RunID: "r3", ObservedAt: t3},
	}
	// Cursor at t2/r2 (the last item on the current page). In DESC ordering,
	// the next page returns items that come AFTER the cursor, i.e. items with
	// a strictly smaller sort key (older). r1 is newer (excluded), r2 is the
	// cursor itself (excluded), r3 is older (included).
	cursor := observeCursor{ObservedAt: t2, ID: "r2"}
	got := filterByCursor(items, cursor, func(p runProjection) (time.Time, string) {
		return p.ObservedAt, p.RunID
	})
	require.Len(t, got, 1)
	assert.Equal(t, "r3", got[0].RunID)
}

func TestSortRuns_DescByObservedAtThenRunID(t *testing.T) {
	t1 := time.Date(2026, 9, 9, 12, 0, 0, 0, time.UTC)
	t2 := time.Date(2026, 9, 9, 12, 0, 0, 0, time.UTC)
	runs := []runProjection{
		{RunID: "b", ObservedAt: t2},
		{RunID: "a", ObservedAt: t1},
		{RunID: "c", ObservedAt: t2},
	}
	sortRuns(runs)
	require.Len(t, runs, 3)
	// Equal timestamps: larger run_id first (DESC).
	assert.Equal(t, "c", runs[0].RunID)
	assert.Equal(t, "b", runs[1].RunID)
	assert.Equal(t, "a", runs[2].RunID)
}

func TestObserveController_ParseLimit_DefaultsWhenEmpty(t *testing.T) {
	c := newTestObserveController(t)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/observe/runs", nil)
	rec := httptest.NewRecorder()
	limit, ok := c.parseLimit(rec, req)
	assert.True(t, ok)
	assert.Equal(t, ObserveDefaultLimit, limit)
	assert.Equal(t, http.StatusOK, rec.Code, "no error response written on default")
}

func TestObserveController_ParseLimit_RejectsOutOfBounds(t *testing.T) {
	c := newTestObserveController(t)
	tests := []string{"0", "-1", "abc", "101"}
	for _, raw := range tests {
		t.Run(raw, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/api/v1/observe/runs?limit="+raw, nil)
			rec := httptest.NewRecorder()
			_, ok := c.parseLimit(rec, req)
			assert.False(t, ok)
			assert.Equal(t, http.StatusBadRequest, rec.Code)
		})
	}
}

func TestObserveController_ParseLimit_AcceptsBounds(t *testing.T) {
	c := newTestObserveController(t)
	tests := []struct {
		raw  string
		want int
	}{
		{"1", 1},
		{"100", 100},
		{"50", 50},
	}
	for _, tc := range tests {
		t.Run(tc.raw, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/api/v1/observe/runs?limit="+tc.raw, nil)
			rec := httptest.NewRecorder()
			limit, ok := c.parseLimit(rec, req)
			assert.True(t, ok)
			assert.Equal(t, tc.want, limit)
		})
	}
}

func TestObserveController_ParseCursor_EmptyIsValid(t *testing.T) {
	c := newTestObserveController(t)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/observe/runs", nil)
	rec := httptest.NewRecorder()
	cursor, ok := c.parseCursor(rec, req)
	assert.True(t, ok)
	assert.Empty(t, cursor)
	assert.Equal(t, http.StatusOK, rec.Code)
}

func TestObserveController_ParseCursor_RejectsInvalid(t *testing.T) {
	c := newTestObserveController(t)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/observe/runs?cursor=!!!invalid", nil)
	rec := httptest.NewRecorder()
	_, ok := c.parseCursor(rec, req)
	assert.False(t, ok)
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestObserveController_RequireUserID_MissingContextReturns401(t *testing.T) {
	c := newTestObserveController(t)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/observe/bootstrap", nil)
	rec := httptest.NewRecorder()
	_, ok := c.requireUserID(rec, req)
	assert.False(t, ok)
	assert.Equal(t, http.StatusUnauthorized, rec.Code)
}

func TestObserveController_RequireUserID_PresentContextReturnsID(t *testing.T) {
	c := newTestObserveController(t)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/observe/bootstrap", nil)
	ctx := context.WithValue(req.Context(), constants.ContextKeyUserID, "user-123")
	req = req.WithContext(ctx)
	rec := httptest.NewRecorder()
	id, ok := c.requireUserID(rec, req)
	assert.True(t, ok)
	assert.Equal(t, "user-123", id)
	assert.Equal(t, http.StatusOK, rec.Code)
}

func TestObserveController_RequireUserID_EmptyStringReturns401(t *testing.T) {
	c := newTestObserveController(t)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/observe/bootstrap", nil)
	ctx := context.WithValue(req.Context(), constants.ContextKeyUserID, "")
	req = req.WithContext(ctx)
	rec := httptest.NewRecorder()
	_, ok := c.requireUserID(rec, req)
	assert.False(t, ok)
	assert.Equal(t, http.StatusUnauthorized, rec.Code)
}

// newTestObserveController builds a controller with a real responder and a nil
// observe service. The controller helpers under test (parseLimit, parseCursor,
// requireUserID) do not touch observeSvc, so a nil service is safe.
func newTestObserveController(t *testing.T) *ObserveController {
	t.Helper()
	return &ObserveController{
		cfg:        nil,
		logger:     testutil.NewTestLogger(),
		observeSvc: nil,
		responder:  response.NewWriter(testutil.NewTestLogger()),
	}
}

// mustEncodeBase64 encodes b as unpadded base64url, failing the test on error.
func mustEncodeBase64(t *testing.T, b []byte) string {
	t.Helper()
	return base64.RawURLEncoding.EncodeToString(b)
}

func TestRouteAuthRegistry_ObservePrefixClassifiedWebSession(t *testing.T) {
	registry := NewRouteAuthRegistry(false)

	observePaths := []string{
		constants.APIPaths.ObserveBootstrap,
		constants.APIPaths.ObserveRuns,
		constants.APIPaths.ObserveEvals,
		constants.APIPaths.ObserveDownloads,
	}
	for _, path := range observePaths {
		assert.Equal(t, RouteAuthWebSession, registry.AuthMode(path),
			"observe path %s should be RouteAuthWebSession", path)
	}
}

func TestRouteAuthRegistry_ObserveByIDPathsClassifiedWebSession(t *testing.T) {
	registry := NewRouteAuthRegistry(false)

	byIDPaths := []string{
		constants.APIPaths.ObserveRunsByID + "run-123",
		constants.APIPaths.ObserveEvalsByID + "eval-abc",
		constants.APIPaths.ObserveDownloadsByID + "art-1",
	}
	for _, path := range byIDPaths {
		assert.Equal(t, RouteAuthWebSession, registry.AuthMode(path),
			"observe by-id path %s should inherit RouteAuthWebSession from prefix", path)
	}
}

func TestRouteAuthRegistry_ObservePrefixDoesNotLeakToOtherRoutes(t *testing.T) {
	registry := NewRouteAuthRegistry(false)

	// Paths outside the observe prefix must remain fail-closed (RouteAuthMTLS)
	// or their explicit classification; the observe prefix must not widen auth.
	outsidePaths := []string{
		"/api/v1/data/settings",
		"/api/v1/audit/receipts",
		constants.APIPaths.GovernanceEnvelopes,
	}
	for _, path := range outsidePaths {
		assert.NotEqual(t, RouteAuthWebSession, registry.AuthMode(path),
			"non-observe path %s must not inherit RouteAuthWebSession", path)
	}
}

func TestRouteAuthRegistry_ObservePrefixFailClosedForUnknownSubPaths(t *testing.T) {
	registry := NewRouteAuthRegistry(false)

	// Unknown sub-paths under the observe prefix inherit RouteAuthWebSession
	// (the prefix is intentionally broad for the read-only surface).
	assert.Equal(t, RouteAuthWebSession, registry.AuthMode(constants.APIPaths.ObservePrefix+"unknown-sub-path"))

	// A path that merely starts with "/api/v1/obser" (not the full prefix)
	// must not match the observe prefix and must fail closed to mTLS.
	assert.Equal(t, RouteAuthMTLS, registry.AuthMode("/api/v1/observer-lookalike"))
}

// TestRouteAuthRegistry_ObserveProducerRoutesClassifiedMTLS asserts the
// mTLS producer endpoints classify as RouteAuthMTLS so only app workloads
// can push agent and run state projections.
func TestRouteAuthRegistry_ObserveProducerRoutesClassifiedMTLS(t *testing.T) {
	registry := NewRouteAuthRegistry(false)

	producerPaths := []string{
		constants.APIPaths.ObserveProducerAgentState,
		constants.APIPaths.ObserveProducerRunState,
	}
	for _, path := range producerPaths {
		assert.Equal(t, RouteAuthMTLS, registry.AuthMode(path),
			"producer path %s should be RouteAuthMTLS", path)
	}
}

// TestRouteAuthRegistry_ObserveProducerPrefixClassifiedMTLS asserts the
// producer prefix classifies as RouteAuthMTLS so unknown producer sub-paths
// fail closed to mTLS.
func TestRouteAuthRegistry_ObserveProducerPrefixClassifiedMTLS(t *testing.T) {
	registry := NewRouteAuthRegistry(false)

	assert.Equal(t, RouteAuthMTLS, registry.AuthMode(constants.APIPaths.ObserveProducerPrefix+"unknown-sub-path"),
		"unknown producer sub-path should inherit RouteAuthMTLS from prefix")
}

// TestRouteAuthRegistry_ObserveProducerRoutesDoNotLeakWebSession asserts the
// producer routes do not inherit RouteAuthWebSession from the broader observe
// prefix, and that lookalike prefixes fail closed.
func TestRouteAuthRegistry_ObserveProducerRoutesDoNotLeakWebSession(t *testing.T) {
	registry := NewRouteAuthRegistry(false)

	// The producer routes are under /api/v1/observe/producer/ which is a
	// sub-prefix of /api/v1/observe/ but must NOT inherit RouteAuthWebSession.
	producerPaths := []string{
		constants.APIPaths.ObserveProducerAgentState,
		constants.APIPaths.ObserveProducerRunState,
		constants.APIPaths.ObserveProducerPrefix + "other",
	}
	for _, path := range producerPaths {
		assert.NotEqual(t, RouteAuthWebSession, registry.AuthMode(path),
			"producer path %s must not inherit RouteAuthWebSession", path)
	}

	// A lookalike prefix that is not the exact observe prefix must fail closed.
	assert.Equal(t, RouteAuthMTLS, registry.AuthMode("/api/v1/observe-producer/agent-state"),
		"lookalike prefix must fail closed to RouteAuthMTLS")
}

func TestRouteAuthRegistry_GenericRoutesRemainMTLS(t *testing.T) {
	registry := NewRouteAuthRegistry(false)

	// Generic data, audit, blob, KV, pubsub, SSE push, governance, and PKI
	// management routes must remain RouteAuthMTLS (fail-closed default).
	// The observe prefix must not widen auth for any of these surfaces.
	mtlsPaths := []string{
		// Data routes
		constants.APIPaths.DataSettings,
		constants.APIPaths.DataDB + "some-collection",
		constants.APIPaths.DataItems,
		constants.APIPaths.DataBlobs + "some-blob",
		// Audit routes
		constants.APIPaths.AuditReceipts,
		constants.APIPaths.AuditReceiptsExport,
		constants.APIPaths.AuditEvents,
		constants.APIPaths.AuditSummary,
		constants.APIPaths.AuditReport,
		constants.APIPaths.AuditStream,
		// KV routes
		constants.APIPaths.KV + "some-key",
		// PubSub routes
		constants.APIPaths.PubSubPublish,
		// SSE push (producer-only, never browser-accessible)
		constants.APIPaths.SSEPush,
		// Governance routes
		constants.APIPaths.GovernanceEnvelopes,
		constants.APIPaths.GovernanceSigners,
		constants.APIPaths.GovernanceSignersByID + "some-signer",
		// PKI management routes
		constants.APIPaths.PKICSRSign,
		constants.APIPaths.PKIAppsDelegated,
		constants.APIPaths.PKICertificatesRevoke,
		constants.APIPaths.PKIRevocationBundle,
		// Operator routes
		constants.APIPaths.Operators,
		constants.APIPaths.OperatorsByID + "some-operator",
		constants.APIPaths.OperatorsValidate,
		constants.APIPaths.OperatorsBind,
		// Admin routes
		constants.APIPaths.AdminConsensus,
		constants.APIPaths.AdminAppsRevoke,
		constants.APIPaths.AdminAppPoliciesBySigner + "some-signer",
	}
	for _, path := range mtlsPaths {
		assert.Equal(t, RouteAuthMTLS, registry.AuthMode(path),
			"generic route %s must remain RouteAuthMTLS (fail-closed)", path)
	}
}

func TestObserveService_PaginateEvals(t *testing.T) {
	svc := &ObserveService{}
	now := time.Date(2026, 9, 19, 12, 0, 0, 0, time.UTC)
	evals := []models.EvalSummary{
		{RunID: "run-1", ObservedAt: now},
		{RunID: "run-2", ObservedAt: now.Add(-time.Minute)},
		{RunID: "run-3", ObservedAt: now.Add(-2 * time.Minute)},
	}
	page, err := svc.paginateEvals(evals, 2)
	require.NoError(t, err)
	require.NotNil(t, page)
	assert.True(t, page.HasMore)
	assert.Equal(t, 2, page.Limit)
	assert.NotEmpty(t, page.Cursor)

	var decoded []models.EvalSummary
	require.NoError(t, json.Unmarshal(page.Items, &decoded))
	require.Len(t, decoded, 2)
}

func TestObserveService_PaginateDownloads(t *testing.T) {
	svc := &ObserveService{}
	now := time.Date(2026, 9, 19, 12, 0, 0, 0, time.UTC)
	downloads := []models.DownloadArtifact{
		{ArtifactID: "art-1", GeneratedAt: now},
		{ArtifactID: "art-2", GeneratedAt: now.Add(-time.Minute)},
	}
	page, err := svc.paginateDownloads(downloads, 1)
	require.NoError(t, err)
	require.NotNil(t, page)
	assert.True(t, page.HasMore)
	assert.NotEmpty(t, page.Cursor)
}

func TestObserveController_MapDownloadStreamError(t *testing.T) {
	c := newTestObserveController(t)
	tests := []struct {
		name       string
		err        error
		wantStatus int
	}{
		{name: "not found", err: constants.ErrObserveDownloadNotFound, wantStatus: http.StatusNotFound},
		{name: "restricted", err: constants.ErrObserveDownloadRestrictedArtifact, wantStatus: http.StatusNotFound},
		{name: "internal", err: fmt.Errorf("unexpected"), wantStatus: http.StatusInternalServerError},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			c.mapDownloadStreamError(rec, test.err, "user-1", "art-1")
			assert.Equal(t, test.wantStatus, rec.Code)
		})
	}
}

func TestObserveController_HandleListEvals_RequiresAuth(t *testing.T) {
	c := newTestObserveController(t)
	req := httptest.NewRequest(http.MethodGet, constants.APIPaths.ObserveEvals, nil)
	rec := httptest.NewRecorder()
	c.handleListEvals(rec, req)
	assert.Equal(t, http.StatusUnauthorized, rec.Code)
}

func TestObserveController_HandleListEvals_RejectsNonGet(t *testing.T) {
	c := newTestObserveController(t)
	req := httptest.NewRequest(http.MethodPost, constants.APIPaths.ObserveEvals, nil)
	rec := httptest.NewRecorder()
	c.handleListEvals(rec, req)
	assert.Equal(t, http.StatusMethodNotAllowed, rec.Code)
}

func newObserveServiceUnitTest(t *testing.T) *ObserveService {
	t.Helper()
	logger := testutil.NewTestLogger()
	db, err := sqliteutil.OpenDB(sqliteutil.DefaultDBConfig(":memory:"), logger)
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	_, err = db.Exec(gatewaySchema)
	require.NoError(t, err)
	return NewObserveService(NewDocumentStoreService(db, logger), logger)
}

func seedObserveEvalProjection(t *testing.T, svc *ObserveService, userID, runID string, observedAt time.Time) {
	t.Helper()
	proj := evalProjection{
		UserID: userID,
		EvalDetail: models.EvalDetail{
			SchemaVersion:      constants.ObserveAPIReadModelSchemaVersion,
			RunID:              runID,
			SuiteID:            "ifeval_subset",
			SuiteVersion:       "1.0.0",
			Status:             models.RunLifecycleStatusCompleted,
			VerificationStatus: models.EvalVerificationProjectionValidated,
			ObservedAt:         observedAt,
		},
	}
	payload, err := json.Marshal(proj)
	require.NoError(t, err)
	require.NoError(t, svc.docStore.DocSet(marshaler.CollectionName(constants.CollectionObserveEvals), runID, payload))
}

func seedObserveDownloadProjection(t *testing.T, svc *ObserveService, userID, artifactID string, generatedAt time.Time) {
	t.Helper()
	proj := downloadProjection{
		UserID: userID,
		DownloadArtifact: models.DownloadArtifact{
			SchemaVersion:         constants.ObserveAPIReadModelSchemaVersion,
			ArtifactID:            artifactID,
			Filename:              artifactID + ".jsonl",
			MediaType:             "application/jsonl",
			ByteSize:              128,
			SHA256:                "abc123",
			PrivacyClassification: models.DownloadPrivacyPublicSafe,
			SourceRunID:           "eval-run-1",
			DownloadURL:           constants.APIPaths.ObserveDownloadsByID + artifactID,
			GeneratedAt:           generatedAt,
		},
	}
	payload, err := json.Marshal(proj)
	require.NoError(t, err)
	require.NoError(t, svc.docStore.DocSet(marshaler.CollectionName(constants.CollectionObserveDownloads), artifactID, payload))
}

func TestObserveService_ListEvalsAndDownloads(t *testing.T) {
	svc := newObserveServiceUnitTest(t)
	now := time.Date(2026, 9, 19, 12, 0, 0, 0, time.UTC)
	seedObserveEvalProjection(t, svc, "user-1", "eval-run-1", now)
	seedObserveDownloadProjection(t, svc, "user-1", "artifact-1", now)

	evals, err := svc.ListEvals(context.Background(), "user-1", "", 20)
	require.NoError(t, err)
	require.NotNil(t, evals)
	assert.False(t, evals.HasMore)

	downloads, err := svc.ListDownloads(context.Background(), "user-1", "", 20)
	require.NoError(t, err)
	require.NotNil(t, downloads)
	assert.False(t, downloads.HasMore)
}

func TestObserveController_HandleListEvals_ReturnsPage(t *testing.T) {
	svc := newObserveServiceUnitTest(t)
	seedObserveEvalProjection(t, svc, "user-1", "eval-run-1", time.Now().UTC())
	c := &ObserveController{
		logger:     testutil.NewTestLogger(),
		observeSvc: svc,
		responder:  response.NewWriter(testutil.NewTestLogger()),
	}
	req := httptest.NewRequest(http.MethodGet, constants.APIPaths.ObserveEvals, nil)
	req = req.WithContext(context.WithValue(req.Context(), constants.ContextKeyUserID, "user-1"))
	rec := httptest.NewRecorder()
	c.handleListEvals(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)

	var page models.ObservePage
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &page))
	assert.Equal(t, constants.ObserveAPIReadModelSchemaVersion, page.SchemaVersion)
}

func TestObserveController_HandleListDownloads_ReturnsPage(t *testing.T) {
	svc := newObserveServiceUnitTest(t)
	seedObserveDownloadProjection(t, svc, "user-1", "artifact-1", time.Now().UTC())
	c := &ObserveController{
		logger:     testutil.NewTestLogger(),
		observeSvc: svc,
		responder:  response.NewWriter(testutil.NewTestLogger()),
	}
	req := httptest.NewRequest(http.MethodGet, constants.APIPaths.ObserveDownloads, nil)
	req = req.WithContext(context.WithValue(req.Context(), constants.ContextKeyUserID, "user-1"))
	rec := httptest.NewRecorder()
	c.handleListDownloads(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)
}

func seedObserveRunProjection(t *testing.T, svc *ObserveService, userID, runID string, observedAt time.Time) {
	t.Helper()
	proj := runProjection{
		UserID:         userID,
		SchemaVersion:  constants.ObserveAPIReadModelSchemaVersion,
		RunID:          runID,
		RunKind:        models.RunKindInvestigation,
		DisplayName:    "run " + runID,
		Status:         models.RunLifecycleStatusRunning,
		TotalTasks:     2,
		CompletedTasks: 1,
		ObservedAt:     observedAt,
	}
	payload, err := json.Marshal(proj)
	require.NoError(t, err)
	require.NoError(t, svc.docStore.DocSet(marshaler.CollectionName(constants.CollectionObserveRuns), runID, payload))
}

func TestObserveController_HandleListRuns_ReturnsPage(t *testing.T) {
	svc := newObserveServiceUnitTest(t)
	seedObserveRunProjection(t, svc, "user-1", "run-1", time.Now().UTC())
	c := &ObserveController{
		logger:     testutil.NewTestLogger(),
		observeSvc: svc,
		responder:  response.NewWriter(testutil.NewTestLogger()),
	}
	req := httptest.NewRequest(http.MethodGet, constants.APIPaths.ObserveRuns, nil)
	req = req.WithContext(context.WithValue(req.Context(), constants.ContextKeyUserID, "user-1"))
	rec := httptest.NewRecorder()
	c.handleListRuns(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)

	var page models.ObservePage
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &page))
	assert.Equal(t, constants.ObserveAPIReadModelSchemaVersion, page.SchemaVersion)
}

func TestObserveController_HandleListRuns_RequiresAuth(t *testing.T) {
	c := newTestObserveController(t)
	req := httptest.NewRequest(http.MethodGet, constants.APIPaths.ObserveRuns, nil)
	rec := httptest.NewRecorder()
	c.handleListRuns(rec, req)
	assert.Equal(t, http.StatusUnauthorized, rec.Code)
}

func TestObserveController_HandleGetRun_ReturnsDetail(t *testing.T) {
	svc := newObserveServiceUnitTest(t)
	seedObserveRunProjection(t, svc, "user-1", "run-detail-1", time.Now().UTC())
	c := &ObserveController{
		logger:     testutil.NewTestLogger(),
		observeSvc: svc,
		responder:  response.NewWriter(testutil.NewTestLogger()),
	}
	req := httptest.NewRequest(http.MethodGet, constants.APIPaths.ObserveRunsByID+"run-detail-1", nil)
	req = req.WithContext(context.WithValue(req.Context(), constants.ContextKeyUserID, "user-1"))
	rec := httptest.NewRecorder()
	c.handleGetRun(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)

	var detail models.RunDetail
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &detail))
	assert.Equal(t, "run-detail-1", detail.RunID)
}

func TestObserveController_HandleGetRun_NotFound(t *testing.T) {
	svc := newObserveServiceUnitTest(t)
	c := &ObserveController{
		logger:     testutil.NewTestLogger(),
		observeSvc: svc,
		responder:  response.NewWriter(testutil.NewTestLogger()),
	}
	req := httptest.NewRequest(http.MethodGet, constants.APIPaths.ObserveRunsByID+"missing-run", nil)
	req = req.WithContext(context.WithValue(req.Context(), constants.ContextKeyUserID, "user-1"))
	rec := httptest.NewRecorder()
	c.handleGetRun(rec, req)
	assert.Equal(t, http.StatusNotFound, rec.Code)
}

func TestObserveController_HandleGetEval_ReturnsDetail(t *testing.T) {
	svc := newObserveServiceUnitTest(t)
	seedObserveEvalProjection(t, svc, "user-1", "eval-detail-1", time.Now().UTC())
	c := &ObserveController{
		logger:     testutil.NewTestLogger(),
		observeSvc: svc,
		responder:  response.NewWriter(testutil.NewTestLogger()),
	}
	req := httptest.NewRequest(http.MethodGet, constants.APIPaths.ObserveEvalsByID+"eval-detail-1", nil)
	req = req.WithContext(context.WithValue(req.Context(), constants.ContextKeyUserID, "user-1"))
	rec := httptest.NewRecorder()
	c.handleGetEval(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)

	var detail models.EvalDetail
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &detail))
	assert.Equal(t, "eval-detail-1", detail.RunID)
}

func TestObserveController_HandleGetDownload_ReturnsMetadata(t *testing.T) {
	svc := newObserveServiceUnitTest(t)
	seedObserveDownloadProjection(t, svc, "user-1", "artifact-meta-1", time.Now().UTC())
	c := &ObserveController{
		logger:     testutil.NewTestLogger(),
		observeSvc: svc,
		responder:  response.NewWriter(testutil.NewTestLogger()),
	}
	req := httptest.NewRequest(http.MethodGet, constants.APIPaths.ObserveDownloadsByID+"artifact-meta-1", nil)
	req = req.WithContext(context.WithValue(req.Context(), constants.ContextKeyUserID, "user-1"))
	rec := httptest.NewRecorder()
	c.handleGetDownload(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)

	var artifact models.DownloadArtifact
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &artifact))
	assert.Equal(t, "artifact-meta-1", artifact.ArtifactID)
}

type stubDownloadStreamer struct {
	err error
}

func (s *stubDownloadStreamer) StreamDownload(_ context.Context, _ string, artifactID string, w http.ResponseWriter) error {
	if s.err != nil {
		return s.err
	}
	w.WriteHeader(http.StatusOK)
	_, err := w.Write([]byte("payload-" + artifactID))
	return err
}

func TestObserveController_HandleGetDownload_StreamsWhenRequested(t *testing.T) {
	svc := newObserveServiceUnitTest(t)
	seedObserveDownloadProjection(t, svc, "user-1", "artifact-stream-1", time.Now().UTC())
	c := &ObserveController{
		logger:           testutil.NewTestLogger(),
		observeSvc:       svc,
		responder:        response.NewWriter(testutil.NewTestLogger()),
		downloadStreamer: &stubDownloadStreamer{},
	}
	req := httptest.NewRequest(http.MethodGet, constants.APIPaths.ObserveDownloadsByID+"artifact-stream-1?download=1", nil)
	req = req.WithContext(context.WithValue(req.Context(), constants.ContextKeyUserID, "user-1"))
	rec := httptest.NewRecorder()
	c.handleGetDownload(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "payload-artifact-stream-1", rec.Body.String())
}

func TestObserveController_HandleBootstrap_ReturnsSnapshot(t *testing.T) {
	svc := newObserveServiceUnitTest(t)
	seedObserveRunProjection(t, svc, "user-1", "run-bootstrap-1", time.Now().UTC())
	c := &ObserveController{
		logger:     testutil.NewTestLogger(),
		observeSvc: svc,
		responder:  response.NewWriter(testutil.NewTestLogger()),
	}
	req := httptest.NewRequest(http.MethodGet, constants.APIPaths.ObserveBootstrap, nil)
	req = req.WithContext(context.WithValue(req.Context(), constants.ContextKeyUserID, "user-1"))
	rec := httptest.NewRecorder()
	c.handleBootstrap(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)

	var snapshot models.ObserveBootstrapSnapshot
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &snapshot))
	assert.Equal(t, constants.ObserveAPIReadModelSchemaVersion, snapshot.SchemaVersion)
}

func TestSortEvals_DescByObservedAtThenRunID(t *testing.T) {
	t1 := time.Date(2026, 9, 9, 12, 0, 0, 0, time.UTC)
	evals := []evalProjection{
		{EvalDetail: models.EvalDetail{RunID: "b", ObservedAt: t1}},
		{EvalDetail: models.EvalDetail{RunID: "c", ObservedAt: t1}},
		{EvalDetail: models.EvalDetail{RunID: "a", ObservedAt: t1.Add(-time.Hour)}},
	}
	sortEvals(evals)
	assert.Equal(t, "c", evals[0].RunID)
	assert.Equal(t, "b", evals[1].RunID)
	assert.Equal(t, "a", evals[2].RunID)
}

func TestSortDownloads_DescByGeneratedAtThenArtifactID(t *testing.T) {
	t1 := time.Date(2026, 9, 9, 12, 0, 0, 0, time.UTC)
	downloads := []downloadProjection{
		{DownloadArtifact: models.DownloadArtifact{ArtifactID: "art-b", GeneratedAt: t1}},
		{DownloadArtifact: models.DownloadArtifact{ArtifactID: "art-c", GeneratedAt: t1}},
		{DownloadArtifact: models.DownloadArtifact{ArtifactID: "art-a", GeneratedAt: t1.Add(-time.Hour)}},
	}
	sortDownloads(downloads)
	assert.Equal(t, "art-c", downloads[0].ArtifactID)
	assert.Equal(t, "art-b", downloads[1].ArtifactID)
	assert.Equal(t, "art-a", downloads[2].ArtifactID)
}
