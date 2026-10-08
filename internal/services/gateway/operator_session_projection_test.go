// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

//go:build integration

package gateway

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/models"
	operatorv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/operator/v1"
)

func TestOperatorController_SessionLookupIncludesObservedHeartbeat(t *testing.T) {
	infra := setupTestInfrastructure(t, false)
	controller := newOperatorController(OperatorControllerDeps{Cfg: infra.Cfg, Logger: infra.Logger, Reg: infra.Reg, Auth: infra.Auth, Responder: infra.Responder})
	op := &operatorv1.OperatorDocument{
		Id: "projection-worker", OperatorSessionId: "projection-session", UserId: "projection-owner",
		Status: string(constants.OperatorStatusActive), OperatorType: string(constants.OperatorTypeRemote),
	}
	body, err := models.MarshalOperatorDocument(op)
	require.NoError(t, err)
	require.NoError(t, infra.DocStore.DocSet(string(constants.CollectionOperators), op.Id, body))
	_, err = infra.Auth.ValidateOperatorSession(op.OperatorSessionId)
	require.NoError(t, err, "authenticate before observed telemetry arrives")
	heartbeatAt := time.Now().UTC()
	require.NoError(t, infra.DocStore.RecordOperatorHeartbeat(op.Id, heartbeatUpdate{
		LastHeartbeatAt: heartbeatAt, CurrentHostname: "current-worker-host",
	}))
	w := httptest.NewRecorder()
	controller.handleGetOperatorBySession(w, httptest.NewRequest(http.MethodGet, constants.APIPaths.OperatorsSession+op.OperatorSessionId, nil))
	require.Equal(t, http.StatusOK, w.Code)
	var response models.OperatorResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))
	require.NotNil(t, response.Operator)
	require.NotNil(t, response.Operator.LastHeartbeatAt, "session validation must not hide observed telemetry from a registry read")
	assert.Equal(t, heartbeatAt, response.Operator.LastHeartbeatAt.AsTime())
	assert.Equal(t, "current-worker-host", response.Operator.CurrentHostname)
}
