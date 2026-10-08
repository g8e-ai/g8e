// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package gateway

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	operatorv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/operator/v1"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/v2/internal/constants"
)

func TestInvestigationsQueryBody_JSONMatchesSortedMapShape(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/api/v1/investigations?case_id=c1&status=open&limit=5&order_by=updated&order_direction=desc&priority=high&investigation_type=triage&web_session_id=from-query&ignored=1", nil)

	body, err := (&EnsembleBrowserProxyController{}).investigationsQueryBody(req, "user-1", "web-1")
	require.NoError(t, err)

	const want = `{"case_id":"c1","context":{"user_id":"user-1","web_session_id":"web-1"},"investigation_type":"triage","limit":5,"order_by":"updated","order_direction":"desc","priority":"high","status":"Open","user_id":"user-1","web_session_id":"from-query"}`
	assert.JSONEq(t, want, string(body))
	assert.Equal(t, want, string(body))
}

func TestInvestigationsQueryBody_InvalidLimitKeepsDefault(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/api/v1/investigations?limit=nope", nil)

	body, err := (&EnsembleBrowserProxyController{}).investigationsQueryBody(req, "user-1", "web-1")
	require.NoError(t, err)
	assert.Equal(t, `{"context":{"user_id":"user-1","web_session_id":"web-1"},"limit":20,"user_id":"user-1"}`, string(body))
}

func TestInvestigationsQueryBody_ScopesToSessionUser(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/api/v1/investigations?user_id=someone-else", nil)

	body, err := (&EnsembleBrowserProxyController{}).investigationsQueryBody(req, "user-1", "web-1")
	require.NoError(t, err)

	var payload browserInvestigationsQuery
	require.NoError(t, json.Unmarshal(body, &payload))
	assert.Equal(t, "user-1", payload.UserID)
}

func TestInjectBrowserContext_PreservesUnknownJSON(t *testing.T) {
	in := []byte(`{"list":[1],"extra":"keep","context":{"keep":true,"user_id":"old"},"case_id":"c","investigation_id":"i"}`)

	out, err := injectBrowserContext(in, "user-1", "web-1", []browserBoundOperator{})
	require.NoError(t, err)

	var payload map[string]interface{}
	require.NoError(t, json.Unmarshal(out, &payload))
	assert.Equal(t, "keep", payload["extra"])
	assert.Equal(t, "c", payload["case_id"])
	ctx := payload["context"].(map[string]interface{})
	assert.Equal(t, true, ctx["keep"])
	assert.Equal(t, "user-1", ctx["user_id"])
	assert.Equal(t, "web-1", ctx["web_session_id"])
	assert.Equal(t, "c", ctx["case_id"])
	assert.Equal(t, "i", ctx["investigation_id"])
}

func TestInjectBrowserContext_NonObjectBodyUnchanged(t *testing.T) {
	in := []byte(`[1,2,3]`)
	out, err := injectBrowserContext(in, "user-1", "web-1", nil)
	require.NoError(t, err)
	assert.Equal(t, in, out)
}

func TestBrowserOperatorStopResponseJSON(t *testing.T) {
	body, err := json.Marshal(browserOperatorStopResponse{
		Message:           "Stop command relayed to orchestrator",
		OperatorID:        "op-1",
		OperatorSessionID: "sess-1",
		Success:           true,
		TransactionID:     "tx-1",
	})
	require.NoError(t, err)
	assert.Equal(t, `{"message":"Stop command relayed to orchestrator","operator_id":"op-1","operator_session_id":"sess-1","success":true,"transaction_id":"tx-1"}`, string(body))
}

type fakeOperatorLister struct {
	ops []*operatorv1.OperatorDocument
	err error
}

func (f fakeOperatorLister) ListUserOperators(context.Context, string) ([]*operatorv1.OperatorDocument, error) {
	return f.ops, f.err
}

func TestInjectBrowserContext_ReplacesBrowserSuppliedBoundOperators(t *testing.T) {
	in := []byte(`{"message":"hi","context":{"bound_operators":[{"operator_id":"forged"}]}}`)
	bound := []browserBoundOperator{{BoundWebSessionID: "web-1", OperatorID: "op-1", OperatorSessionID: "os-1", Status: "bound"}}

	out, err := injectBrowserContext(in, "user-1", "web-1", bound)
	require.NoError(t, err)

	var payload struct {
		Context struct {
			BoundOperators []browserBoundOperator `json:"bound_operators"`
		} `json:"context"`
	}
	require.NoError(t, json.Unmarshal(out, &payload))
	assert.Equal(t, bound, payload.Context.BoundOperators)
}

func TestBoundOperators_FiltersToWebSession(t *testing.T) {
	c := &EnsembleBrowserProxyController{
		logger: slog.Default(),
		operators: fakeOperatorLister{ops: []*operatorv1.OperatorDocument{
			{Id: "op-1", OperatorSessionId: "os-1", BoundWebSessionId: "web-1", Status: string(constants.OperatorStatusBound)},
			{Id: string(constants.DocIDEmbeddedOperator), OperatorSessionId: "os-embedded", BoundWebSessionId: "web-1", Status: string(constants.OperatorStatusActive), OperatorType: string(constants.OperatorTypeEmbedded)},
			{Id: "op-remote", OperatorSessionId: "os-remote", BoundWebSessionId: "web-1", Status: string(constants.OperatorStatusActive), OperatorType: string(constants.OperatorTypeRemote)},
			{Id: "op-2", OperatorSessionId: "os-2", BoundWebSessionId: "web-other", Status: string(constants.OperatorStatusBound)},
			{Id: "op-3", OperatorSessionId: "os-3", Status: string(constants.OperatorStatusActive)},
			{Id: "op-stopped", OperatorSessionId: "os-stopped", BoundWebSessionId: "web-1", Status: string(constants.OperatorStatusStopped)},
		}},
	}

	got := c.boundOperators(context.Background(), "user-1", "web-1")

	assert.Equal(t, []browserBoundOperator{
		{BoundWebSessionID: "web-1", OperatorID: "op-1", OperatorSessionID: "os-1", Status: "bound"},
		{BoundWebSessionID: "web-1", OperatorID: string(constants.DocIDEmbeddedOperator), OperatorSessionID: "os-embedded", Status: "bound"},
		{BoundWebSessionID: "web-1", OperatorID: "op-remote", OperatorSessionID: "os-remote", Status: "bound"},
		{BoundWebSessionID: "web-1", OperatorID: "op-stopped", OperatorSessionID: "os-stopped", Status: "stopped"},
	}, got)
}

func TestBoundOperators_RegistryErrorYieldsEmptyList(t *testing.T) {
	c := &EnsembleBrowserProxyController{
		logger:    slog.Default(),
		operators: fakeOperatorLister{err: errors.New("registry down")},
	}

	got := c.boundOperators(context.Background(), "user-1", "web-1")

	require.NotNil(t, got)
	assert.Empty(t, got)
}
