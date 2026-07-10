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

// AuthControllerDeps groups all dependencies for AuthController to reduce
// constructor bloat.
type AuthControllerDeps struct {
	Cfg                *config.Config
	Logger             *slog.Logger
	DB                 *CanonicalDBService
	Auth               *AuthService
	Passkey            *PasskeyHandler
	UserSvc            *UserService
	Reg                *RegistrationService
	PKI                *PKIAuthority
	WebSessionSvc      *WebSessionService
	CLISessionSvc      *CLISessionService
	OperatorSessionSvc *OperatorSessionService
	EnrollmentTokenSvc *EnrollmentTokenService
	Responder          *response.Writer
	ActuatorKeyReader  actuatorKeyReader
	CrossOrigin        bool
}

// AuthController handles authentication, passkey, and approval endpoints.
type AuthController struct {
	cfg                *config.Config
	logger             *slog.Logger
	db                 *CanonicalDBService
	auth               *AuthService
	passkey            *PasskeyHandler
	userSvc            *UserService
	reg                *RegistrationService
	pki                *PKIAuthority
	webSessionSvc      *WebSessionService
	cliSessionSvc      *CLISessionService
	operatorSessionSvc *OperatorSessionService
	enrollmentTokenSvc *EnrollmentTokenService
	responder          *response.Writer
	actuatorKeyReader  actuatorKeyReader
	crossOrigin        bool
}

func newAuthController(deps AuthControllerDeps) *AuthController {
	return &AuthController{
		cfg:                deps.Cfg,
		logger:             deps.Logger,
		db:                 deps.DB,
		auth:               deps.Auth,
		passkey:            deps.Passkey,
		userSvc:            deps.UserSvc,
		reg:                deps.Reg,
		pki:                deps.PKI,
		webSessionSvc:      deps.WebSessionSvc,
		cliSessionSvc:      deps.CLISessionSvc,
		operatorSessionSvc: deps.OperatorSessionSvc,
		enrollmentTokenSvc: deps.EnrollmentTokenSvc,
		responder:          deps.Responder,
		actuatorKeyReader:  deps.ActuatorKeyReader,
		crossOrigin:        deps.CrossOrigin,
	}
}

func (c *AuthController) readBody(w http.ResponseWriter, r *http.Request) ([]byte, error) {
	r.Body = http.MaxBytesReader(w, r.Body, c.cfg.Gateway.MaxPayloadBytes)
	return io.ReadAll(r.Body)
}
