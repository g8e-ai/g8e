// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

//go:build integration

package gateway

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	operatorv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/operator/v1"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/marshaler"
	"github.com/g8e-ai/g8e/v2/internal/models"
)

func bindRequestWithContext(t *testing.T, userID, oldCLISessionID, operatorSessionID string) *http.Request {
	t.Helper()
	body, err := json.Marshal(models.CLIBindRequest{OperatorSessionID: operatorSessionID})
	require.NoError(t, err)
	req := httptest.NewRequest(http.MethodPost, constants.APIPaths.AuthCLIBind, bytes.NewReader(body))
	ctx := context.WithValue(req.Context(), constants.ContextKeyUserID, userID)
	ctx = context.WithValue(ctx, constants.ContextKeyCLISessionID, oldCLISessionID)
	return req.WithContext(ctx)
}

func persistOperatorForBindController(t *testing.T, c *CLIRefreshController, userID, operatorID, operatorSessionID string) {
	t.Helper()
	now := time.Now().UTC()
	opDoc := &operatorv1.OperatorDocument{
		ID:                operatorID,
		OperatorSessionID: operatorSessionID,
		Status:            constants.OperatorStatusActive,
		UserID:            userID,
		OperatorType:      constants.OperatorTypeRemote,
		CreatedAt:         now,
		UpdatedAt:         now,
	}
	opBytes, err := json.Marshal(opDoc)
	require.NoError(t, err)
	require.NoError(t, c.cliSessionSvc.db.DocSet(marshaler.CollectionName(constants.CollectionOperators), operatorID, opBytes))
}

func TestCLIRefreshController_Bind_Success(t *testing.T) {
	c, user := setupTestCLIRefreshController(t)
	oldSessionID := "bind-ctrl-old-1"
	targetSessionID := "bind-ctrl-target-1"
	persistOperatorForBindController(t, c, user.ID, "bind-op-1", targetSessionID)
	persistCLISessionForController(t, c, user.ID, oldSessionID, "bind-ctrl-stale-1")

	req := bindRequestWithContext(t, user.ID, oldSessionID, targetSessionID)
	rr := httptest.NewRecorder()
	c.handleBind(rr, req)

	require.Equal(t, http.StatusCreated, rr.Code, rr.Body.String())
	var resp models.CLIBindResponse
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &resp))
	assert.True(t, resp.Success)
	assert.NotEmpty(t, resp.CLISessionID)
	assert.NotEqual(t, oldSessionID, resp.CLISessionID)
	assert.Equal(t, targetSessionID, resp.OperatorSessionID)
	assert.Equal(t, "bind-op-1", resp.OperatorID)
}

func TestCLIRefreshController_Bind_AlreadyBound(t *testing.T) {
	c, user := setupTestCLIRefreshController(t)
	oldSessionID := "bind-ctrl-old-2"
	targetSessionID := "bind-ctrl-target-2"
	persistOperatorForBindController(t, c, user.ID, "bind-op-2", targetSessionID)
	persistCLISessionForController(t, c, user.ID, oldSessionID, targetSessionID)

	req := bindRequestWithContext(t, user.ID, oldSessionID, targetSessionID)
	rr := httptest.NewRecorder()
	c.handleBind(rr, req)

	require.Equal(t, http.StatusOK, rr.Code, rr.Body.String())
	var resp models.CLIBindResponse
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &resp))
	assert.True(t, resp.Success)
	assert.True(t, resp.AlreadyBound)
	assert.Equal(t, oldSessionID, resp.CLISessionID)
}

func unbindRequestWithContext(t *testing.T, userID, oldCLISessionID string) *http.Request {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, constants.APIPaths.AuthCLIUnbind, nil)
	ctx := context.WithValue(req.Context(), constants.ContextKeyUserID, userID)
	ctx = context.WithValue(ctx, constants.ContextKeyCLISessionID, oldCLISessionID)
	return req.WithContext(ctx)
}

func TestCLIRefreshController_Unbind_Success(t *testing.T) {
	c, user := setupTestCLIRefreshController(t)
	oldSessionID := "unbind-ctrl-old-1"
	targetSessionID := "unbind-ctrl-target-1"
	persistOperatorForBindController(t, c, user.ID, "unbind-op-1", targetSessionID)
	persistCLISessionForController(t, c, user.ID, oldSessionID, targetSessionID)

	req := unbindRequestWithContext(t, user.ID, oldSessionID)
	rr := httptest.NewRecorder()
	c.handleUnbind(rr, req)

	require.Equal(t, http.StatusCreated, rr.Code, rr.Body.String())
	var resp models.CLIUnbindResponse
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &resp))
	assert.True(t, resp.Success)
	assert.NotEmpty(t, resp.CLISessionID)
	assert.NotEqual(t, oldSessionID, resp.CLISessionID)
}

func TestCLIRefreshController_Unbind_AlreadyUnbound(t *testing.T) {
	c, user := setupTestCLIRefreshController(t)
	oldSessionID := "unbind-ctrl-old-2"
	persistCLISessionForController(t, c, user.ID, oldSessionID, "")

	req := unbindRequestWithContext(t, user.ID, oldSessionID)
	rr := httptest.NewRecorder()
	c.handleUnbind(rr, req)

	require.Equal(t, http.StatusOK, rr.Code, rr.Body.String())
	var resp models.CLIUnbindResponse
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &resp))
	assert.True(t, resp.Success)
	assert.True(t, resp.AlreadyUnbound)
	assert.Equal(t, oldSessionID, resp.CLISessionID)
}

func multiBindRequestWithContext(t *testing.T, userID, oldCLISessionID string, operatorSessionIDs []string) *http.Request {
	t.Helper()
	body, err := json.Marshal(models.CLIBindRequest{OperatorSessionIDs: operatorSessionIDs})
	require.NoError(t, err)
	req := httptest.NewRequest(http.MethodPost, constants.APIPaths.AuthCLIBind, bytes.NewReader(body))
	ctx := context.WithValue(req.Context(), constants.ContextKeyUserID, userID)
	ctx = context.WithValue(ctx, constants.ContextKeyCLISessionID, oldCLISessionID)
	return req.WithContext(ctx)
}

func TestCLIRefreshController_Bind_MultipleOperatorsInOneCall(t *testing.T) {
	c, user := setupTestCLIRefreshController(t)
	oldSessionID := "bind-ctrl-old-multi"
	sessionIDs := []string{"bind-ctrl-multi-a", "bind-ctrl-multi-b", "bind-ctrl-multi-c"}
	for i, sessionID := range sessionIDs {
		persistOperatorForBindController(t, c, user.ID, "bind-op-multi-"+string(rune('a'+i)), sessionID)
	}
	persistCLISessionForController(t, c, user.ID, oldSessionID, "bind-ctrl-stale-multi")

	rr := httptest.NewRecorder()
	c.handleBind(rr, multiBindRequestWithContext(t, user.ID, oldSessionID, sessionIDs))

	require.Equal(t, http.StatusCreated, rr.Code, rr.Body.String())
	var resp models.CLIBindResponse
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &resp))
	assert.True(t, resp.Success)
	assert.False(t, resp.AlreadyBound)
	assert.Equal(t, sessionIDs[0], resp.OperatorSessionID, "first target is the primary binding")
	require.Len(t, resp.Bound, 3)
	for i, sessionID := range sessionIDs {
		assert.Equal(t, sessionID, resp.Bound[i].OperatorSessionID)
	}

	persisted, err := c.cliSessionSvc.loadCLISession(resp.CLISessionID)
	require.NoError(t, err)
	assert.Equal(t, sessionIDs[0], persisted.OperatorSessionID)
	assert.Equal(t, sessionIDs, persisted.BoundOperatorSessionIDs)
}

func TestCLIRefreshController_Bind_MultipleOperatorsAlreadyBound(t *testing.T) {
	c, user := setupTestCLIRefreshController(t)
	oldSessionID := "bind-ctrl-old-multi-again"
	sessionIDs := []string{"bind-ctrl-again-a", "bind-ctrl-again-b"}
	for i, sessionID := range sessionIDs {
		persistOperatorForBindController(t, c, user.ID, "bind-op-again-"+string(rune('a'+i)), sessionID)
	}
	persistCLISessionForController(t, c, user.ID, oldSessionID, "bind-ctrl-stale-again")

	first := httptest.NewRecorder()
	c.handleBind(first, multiBindRequestWithContext(t, user.ID, oldSessionID, sessionIDs))
	require.Equal(t, http.StatusCreated, first.Code, first.Body.String())
	var created models.CLIBindResponse
	require.NoError(t, json.Unmarshal(first.Body.Bytes(), &created))

	second := httptest.NewRecorder()
	c.handleBind(second, multiBindRequestWithContext(t, user.ID, created.CLISessionID, sessionIDs))
	require.Equal(t, http.StatusOK, second.Code, second.Body.String())
	var again models.CLIBindResponse
	require.NoError(t, json.Unmarshal(second.Body.Bytes(), &again))
	assert.True(t, again.AlreadyBound)
	assert.Equal(t, created.CLISessionID, again.CLISessionID, "rebinding the same list must not rotate the session")
}

func TestCLIRefreshController_Bind_OneBadTargetBindsNothing(t *testing.T) {
	c, user := setupTestCLIRefreshController(t)
	oldSessionID := "bind-ctrl-old-partial"
	goodSessionID := "bind-ctrl-partial-good"
	foreignSessionID := "bind-ctrl-partial-foreign"
	persistOperatorForBindController(t, c, user.ID, "bind-op-partial-good", goodSessionID)
	persistOperatorForBindController(t, c, "other-user", "bind-op-partial-foreign", foreignSessionID)
	persistCLISessionForController(t, c, user.ID, oldSessionID, "bind-ctrl-stale-partial")

	rr := httptest.NewRecorder()
	c.handleBind(rr, multiBindRequestWithContext(t, user.ID, oldSessionID, []string{goodSessionID, foreignSessionID}))

	require.Equal(t, http.StatusForbidden, rr.Code, rr.Body.String())
	old, err := c.cliSessionSvc.loadCLISession(oldSessionID)
	require.NoError(t, err)
	assert.True(t, old.IsActive, "a rejected bind must leave the existing CLI session active")
	assert.Equal(t, "bind-ctrl-stale-partial", old.OperatorSessionID)
	assert.Empty(t, old.BoundOperatorSessionIDs)
}

func TestCLIRefreshController_Bind_RejectsTooManyOperators(t *testing.T) {
	c, user := setupTestCLIRefreshController(t)
	oldSessionID := "bind-ctrl-old-toomany"
	persistCLISessionForController(t, c, user.ID, oldSessionID, "bind-ctrl-stale-toomany")

	ids := make([]string, constants.CLIBindMaxOperators+1)
	for i := range ids {
		ids[i] = fmt.Sprintf("bind-ctrl-toomany-%05d", i)
	}
	rr := httptest.NewRecorder()
	c.handleBind(rr, multiBindRequestWithContext(t, user.ID, oldSessionID, ids))

	require.Equal(t, http.StatusBadRequest, rr.Code, rr.Body.String())
}

func TestCLIRefreshController_Bind_RejectsForeignOperator(t *testing.T) {
	c, user := setupTestCLIRefreshController(t)
	oldSessionID := "bind-ctrl-old-3"
	targetSessionID := "bind-ctrl-target-3"
	persistOperatorForBindController(t, c, "other-user", "bind-op-3", targetSessionID)
	persistCLISessionForController(t, c, user.ID, oldSessionID, "bind-ctrl-stale-3")

	req := bindRequestWithContext(t, user.ID, oldSessionID, targetSessionID)
	rr := httptest.NewRecorder()
	c.handleBind(rr, req)

	require.Equal(t, http.StatusForbidden, rr.Code)
}
