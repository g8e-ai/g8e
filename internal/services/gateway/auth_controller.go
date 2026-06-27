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
	"io"
	"log/slog"
	"net/http"
	"os"

	"github.com/g8e-ai/g8e/internal/config"
	"github.com/g8e-ai/g8e/internal/response"
)

// actuatorKeyReader reads the actuator public key from storage.
type actuatorKeyReader interface {
	ReadActuatorPublicKey() (keyID, publicKey string, err error)
}

// fileActuatorKeyReader reads the actuator public key from a file.
type fileActuatorKeyReader struct {
	path string
}

func (r *fileActuatorKeyReader) ReadActuatorPublicKey() (keyID, publicKey string, err error) {
	data, err := os.ReadFile(r.path)
	if err != nil {
		return "", "", err
	}
	var ap struct {
		KeyID     string `json:"key_id"`
		PublicKey string `json:"public_key"`
	}
	if err := json.Unmarshal(data, &ap); err != nil {
		return "", "", err
	}
	return ap.KeyID, ap.PublicKey, nil
}

// AuthController handles authentication, passkey, and approval endpoints.
type AuthController struct {
	cfg                *config.Config
	logger             *slog.Logger
	db                 *CanonicalDBService
	auth               *AuthService
	passkey            *PasskeyService
	userSvc            *UserService
	reg                *RegistrationService
	pki                *PKIAuthority
	webSessionSvc      *WebSessionService
	cliSessionSvc      *CLISessionService
	operatorSessionSvc *OperatorSessionService
	responder          *response.Writer
	actuatorKeyReader  actuatorKeyReader
}

func newAuthController(cfg *config.Config, logger *slog.Logger, db *CanonicalDBService, auth *AuthService, passkey *PasskeyService, userSvc *UserService, reg *RegistrationService, pki *PKIAuthority, webSessionSvc *WebSessionService, cliSessionSvc *CLISessionService, operatorSessionSvc *OperatorSessionService, responder *response.Writer, actuatorKeyReader actuatorKeyReader) *AuthController {
	return &AuthController{
		cfg:                cfg,
		logger:             logger,
		db:                 db,
		auth:               auth,
		passkey:            passkey,
		userSvc:            userSvc,
		reg:                reg,
		pki:                pki,
		webSessionSvc:      webSessionSvc,
		cliSessionSvc:      cliSessionSvc,
		operatorSessionSvc: operatorSessionSvc,
		responder:          responder,
		actuatorKeyReader:  actuatorKeyReader,
	}
}

func (c *AuthController) readBody(r *http.Request) ([]byte, error) {
	r.Body = http.MaxBytesReader(nil, r.Body, c.cfg.Gateway.MaxPayloadBytes)
	return io.ReadAll(r.Body)
}
