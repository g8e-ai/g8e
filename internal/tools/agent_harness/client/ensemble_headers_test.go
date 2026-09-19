// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package client

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestApplyEnsemblePersonaHeaders_BearerIncludesProxyIdentity(t *testing.T) {
	req, err := http.NewRequest(http.MethodGet, "http://localhost:8000/api/v1/evaluation/trace/a/b", nil)
	assert.NoError(t, err)

	applyEnsemblePersonaHeaders(req, Persona{
		ID:                "g8e-chat-acceptance",
		UserAgent:         "g8e-eval-chat-acceptance",
		UserID:            "user-123",
		CLISessionID:      "cli-session-789",
		OperatorSessionID: "operator-session-123",
	})

	assert.Equal(t, "Bearer operator-session-123", req.Header.Get("Authorization"))
	assert.Equal(t, "user-123", req.Header.Get(HeaderProxyUserID))
	assert.Equal(t, "user-123@g8e.local", req.Header.Get(HeaderProxyUserEmail))
	assert.Equal(t, "cli-session-789", req.Header.Get(HeaderProxyCLISessionID))
}
