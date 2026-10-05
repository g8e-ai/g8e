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
	"crypto/ed25519"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/v2/internal/constants"
)

func TestBrowserProxyRouter_ActiveEmbeddedOperatorIsBoundInChatContext(t *testing.T) {
	forwarded := make(chan []byte, 1)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		forwarded <- body
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(upstream.Close)
	h, cfg, infra := setupTestHTTPHandler(t)
	cfg.Gateway.EnsembleUpstreamURL = upstream.URL
	_, key, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)
	signer, err := NewBrowserProxySigner(key, "binding-test-key")
	require.NoError(t, err)
	controller := h.ensembleBrowserProxyController
	controller.cfg = cfg
	controller.logger = infra.Logger
	controller.responder = infra.Responder
	controller.operators = &gatewayOperatorListerAdapter{svc: infra.Reg}
	controller.signer = signer

	const userID = "proxy-binding-user"
	seedActiveUser(t, infra, userID)
	operatorID, operatorSessionID, err := infra.Embedded.ClaimEmbeddedOperator(userID)
	require.NoError(t, err)
	webSessionID := seedWebSession(t, infra, userID)
	bound, err := infra.Reg.BindEmbeddedOperatorToWebSession(userID, webSessionID)
	require.NoError(t, err)
	require.True(t, bound)

	op := loadEmbeddedOperatorDoc(t, infra.DocStore)
	require.Equal(t, constants.OperatorStatusActive, op.Status, "binding must preserve lifecycle status")
	require.Equal(t, webSessionID, op.BoundWebSessionID)

	req := httptest.NewRequest(http.MethodPost, constants.APIPaths.EnsembleChatPrefix+"/send", bytes.NewBufferString(
		`{"message":"what is taking up RAM on the embedded operator?","context":{"bound_operators":[]}}`,
	))
	req.AddCookie(&http.Cookie{Name: constants.WebSessionCookieName, Value: webSessionID})
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	require.Equal(t, http.StatusNoContent, w.Code, w.Body.String())

	var payload struct {
		Context struct {
			UserID         string                 `json:"user_id"`
			WebSessionID   string                 `json:"web_session_id"`
			BoundOperators []browserBoundOperator `json:"bound_operators"`
		} `json:"context"`
	}
	require.NoError(t, json.Unmarshal(<-forwarded, &payload))
	require.Equal(t, userID, payload.Context.UserID)
	require.Equal(t, webSessionID, payload.Context.WebSessionID)
	require.Equal(t, []browserBoundOperator{{
		BoundWebSessionID: webSessionID,
		OperatorID:        operatorID,
		OperatorSessionID: operatorSessionID,
		Status:            string(constants.OperatorStatusBound),
	}}, payload.Context.BoundOperators, "g8ee must select bound prompts and execution tools for the embedded operator")
}
