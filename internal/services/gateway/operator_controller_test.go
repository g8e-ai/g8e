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
	"github.com/g8e-ai/g8e/internal/responder"
	"github.com/stretchr/testify/assert"
)

func TestNewOperatorController(t *testing.T) {
	t.Parallel()

	cfg := &config.Config{}
	logger := slog.New(slog.NewTextHandler(nil, nil))
	reg := &RegistrationService{}
	auth := &AuthService{}
	resp := &responder.Responder{}

	controller := newOperatorController(cfg, logger, reg, auth, resp)

	assert.NotNil(t, controller)
	assert.Equal(t, cfg, controller.cfg)
	assert.Equal(t, logger, controller.logger)
	assert.Equal(t, reg, controller.reg)
	assert.Equal(t, auth, controller.auth)
	assert.Equal(t, resp, controller.responder)
}
