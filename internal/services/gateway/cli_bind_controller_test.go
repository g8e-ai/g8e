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
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

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
	opDoc := &models.OperatorDocumentGo{
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
