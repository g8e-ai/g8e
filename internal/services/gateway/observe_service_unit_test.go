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
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/models"
	"github.com/g8e-ai/g8e/v2/internal/response"
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
		cfg:       nil,
		logger:    testutil.NewTestLogger(),
		observeSvc: nil,
		responder: response.NewWriter(testutil.NewTestLogger()),
	}
}

// mustEncodeBase64 encodes b as unpadded base64url, failing the test on error.
func mustEncodeBase64(t *testing.T, b []byte) string {
	t.Helper()
	return base64.RawURLEncoding.EncodeToString(b)
}

// keep models import used for future expansion of typed assertions.
var _ = models.ObserveBootstrapSnapshot{}

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
