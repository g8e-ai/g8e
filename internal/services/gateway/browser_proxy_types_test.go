// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package gateway

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestInvestigationsQueryBody_JSONMatchesSortedMapShape(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/api/v1/investigations?case_id=c1&status=open&limit=5&order_by=updated&order_direction=desc&priority=high&investigation_type=triage&web_session_id=from-query&ignored=1", nil)

	body, err := (&EnsembleBrowserProxyController{}).investigationsQueryBody(req, "user-1", "web-1")
	require.NoError(t, err)

	const want = `{"case_id":"c1","context":{"user_id":"user-1","web_session_id":"web-1"},"investigation_type":"triage","limit":5,"order_by":"updated","order_direction":"desc","priority":"high","status":"open","web_session_id":"from-query"}`
	assert.JSONEq(t, want, string(body))
	assert.Equal(t, want, string(body))
}

func TestInvestigationsQueryBody_InvalidLimitKeepsDefault(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/api/v1/investigations?limit=nope", nil)

	body, err := (&EnsembleBrowserProxyController{}).investigationsQueryBody(req, "user-1", "web-1")
	require.NoError(t, err)
	assert.Equal(t, `{"context":{"user_id":"user-1","web_session_id":"web-1"},"limit":20}`, string(body))
}

func TestInjectBrowserContext_PreservesUnknownJSON(t *testing.T) {
	in := []byte(`{"list":[1],"extra":"keep","context":{"keep":true,"user_id":"old"},"case_id":"c","investigation_id":"i"}`)

	out, err := injectBrowserContext(in, "user-1", "web-1")
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
	out, err := injectBrowserContext(in, "user-1", "web-1")
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
