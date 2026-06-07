// Copyright (c) 2026 Lateralus Labs, LLC.
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package gateway

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/g8e-ai/g8e/internal/config"
	"github.com/g8e-ai/g8e/internal/constants"
	"github.com/g8e-ai/g8e/internal/marshaler"
	"github.com/g8e-ai/g8e/internal/response"
	"github.com/g8e-ai/g8e/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewOperatorController(t *testing.T) {
	t.Parallel()

	cfg := &config.Config{}
	logger := slog.New(slog.NewTextHandler(nil, nil))
	reg := &RegistrationService{}
	auth := &AuthService{}
	resp := &response.Writer{}

	controller := newOperatorController(cfg, logger, reg, auth, resp)

	assert.NotNil(t, controller)
	assert.Equal(t, cfg, controller.cfg)
	assert.Equal(t, logger, controller.logger)
	assert.Equal(t, reg, controller.reg)
	assert.Equal(t, auth, controller.auth)
	assert.Equal(t, resp, controller.responder)
}

func TestHandleReauth_MalformedJSON(t *testing.T) {
	t.Parallel()

	db := newTestDB(t)
	logger := testutil.NewTestLogger()
	userSvc := NewUserService(db, logger)
	personaSvc := NewPersonaService(db, logger)
	res := response.NewWriter(logger)
	auth := NewAuthService(db, nil, logger, userSvc, personaSvc, res, "", nil, "", "", "")
	reg := &RegistrationService{}
	cfg := &config.Config{Gateway: config.GatewayConfig{MaxPayloadBytes: 1024}}
	controller := newOperatorController(cfg, logger, reg, auth, res)

	// Create a valid Operator session
	operatorSessionID := "test-session-123"
	opDoc := map[string]interface{}{
		"id":                  "op-123",
		"operator_session_id": operatorSessionID,
		"status":              marshaler.Status(constants.OperatorStatusActive),
		"user_id":             "user-123",
		"organization_id":     "org-123",
	}
	opBytes, err := json.Marshal(opDoc)
	require.NoError(t, err)
	require.NoError(t, db.DocSet("operators", "op-123", opBytes))

	req := httptest.NewRequest(http.MethodPost, "/api/v1/operators/reauth", strings.NewReader("{invalid json"))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(constants.HeaderAuthorization, "Bearer "+operatorSessionID)
	w := httptest.NewRecorder()

	controller.handleReauth(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}
