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
	"log/slog"
	"testing"

	"github.com/g8e-ai/g8e/internal/config"
	"github.com/stretchr/testify/assert"
)

func TestNewRegistrationService(t *testing.T) {
	t.Parallel()

	db := &GatewayDBService{}
	pki := &PKIAuthority{}
	logger := slog.New(slog.NewTextHandler(nil, nil))
	userSvc := &UserService{}
	sessionSvc := &SessionService{}
	cfg := &config.GatewayConfig{}

	service := NewRegistrationService(db, pki, logger, userSvc, sessionSvc, cfg)

	assert.NotNil(t, service)
	assert.Equal(t, db, service.db)
	assert.Equal(t, pki, service.pki)
	assert.Equal(t, logger, service.logger)
	assert.Equal(t, userSvc, service.userSvc)
	assert.Equal(t, sessionSvc, service.sessionSvc)
	assert.Equal(t, cfg, service.cfg)
}

func TestSessionWebBindKey(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		sessionID string
		expected string
	}{
		{"Valid session ID", "web-session-123", "g8e:session:web:web-session-123:bind"},
		{"Empty session ID", "", "g8e:session:web::bind"},
		{"Session with special chars", "session-abc-123", "g8e:session:web:session-abc-123:bind"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			result := sessionWebBindKey(tt.sessionID)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestSessionOperatorBindKey(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		sessionID string
		expected string
	}{
		{"Valid session ID", "op-session-456", "g8e:session:operator:op-session-456:bind"},
		{"Empty session ID", "", "g8e:session:operator::bind"},
		{"Session with special chars", "operator-xyz-789", "g8e:session:operator:operator-xyz-789:bind"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			result := sessionOperatorBindKey(tt.sessionID)
			assert.Equal(t, tt.expected, result)
		})
	}
}
