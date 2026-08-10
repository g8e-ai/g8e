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

import "net/http"

// PasskeyController handles passkey registration, authentication, approval, and
// credential management endpoints. It is a thin wrapper around PasskeyHandler
// that exposes its HTTP-facing methods through the controller pattern, so
// HTTPHandler routes through a controller slot rather than a direct field
// reference.
type PasskeyController struct {
	handler *PasskeyHandler
}

// PasskeyControllerDeps groups all dependencies for PasskeyController.
type PasskeyControllerDeps struct {
	Handler *PasskeyHandler
}

func newPasskeyController(d PasskeyControllerDeps) *PasskeyController {
	return &PasskeyController{handler: d.Handler}
}

// registerChallenge delegates to PasskeyHandler.RegisterChallenge.
func (c *PasskeyController) registerChallenge(cfg passkeyHandlerConfig) http.HandlerFunc {
	return c.handler.RegisterChallenge(cfg)
}

// registerVerify delegates to PasskeyHandler.RegisterVerify.
func (c *PasskeyController) registerVerify(cfg passkeyHandlerConfig) http.HandlerFunc {
	return c.handler.RegisterVerify(cfg)
}

// authenticateChallenge delegates to PasskeyHandler.AuthenticateChallenge.
func (c *PasskeyController) authenticateChallenge(cfg passkeyHandlerConfig) http.HandlerFunc {
	return c.handler.AuthenticateChallenge(cfg)
}

// authenticateVerify delegates to PasskeyHandler.AuthenticateVerify.
func (c *PasskeyController) authenticateVerify(cfg passkeyHandlerConfig) http.HandlerFunc {
	return c.handler.AuthenticateVerify(cfg)
}

// cliStatus delegates to PasskeyHandler.CLIStatus.
func (c *PasskeyController) cliStatus(w http.ResponseWriter, r *http.Request) {
	c.handler.CLIStatus(w, r)
}

// handleApprovalPage delegates to PasskeyHandler.handleApprovalPage.
func (c *PasskeyController) handleApprovalPage(w http.ResponseWriter, r *http.Request) {
	c.handler.handleApprovalPage(w, r)
}

// handleCLIApprovalStatus delegates to PasskeyHandler.handleCLIApprovalStatus.
func (c *PasskeyController) handleCLIApprovalStatus(w http.ResponseWriter, r *http.Request) {
	c.handler.handleCLIApprovalStatus(w, r)
}

// handleCLIListSuspended delegates to PasskeyHandler.handleCLIListSuspended.
func (c *PasskeyController) handleCLIListSuspended(w http.ResponseWriter, r *http.Request) {
	c.handler.handleCLIListSuspended(w, r)
}

// handleApprovalAction delegates to PasskeyHandler.handleApprovalAction.
func (c *PasskeyController) handleApprovalAction(w http.ResponseWriter, r *http.Request) {
	c.handler.handleApprovalAction(w, r)
}

// handleListSuspendedTransactions delegates to PasskeyHandler.handleListSuspendedTransactions.
func (c *PasskeyController) handleListSuspendedTransactions(w http.ResponseWriter, r *http.Request) {
	c.handler.handleListSuspendedTransactions(w, r)
}

// listCredentials delegates to PasskeyHandler.ListCredentials.
func (c *PasskeyController) listCredentials(w http.ResponseWriter, r *http.Request) {
	c.handler.ListCredentials(w, r)
}

// revokeCredential delegates to PasskeyHandler.RevokeCredential.
func (c *PasskeyController) revokeCredential(w http.ResponseWriter, r *http.Request) {
	c.handler.RevokeCredential(w, r)
}

// PasskeyHandler returns the underlying PasskeyHandler for callers that need
// direct access (e.g., GatewayModeService.GetPasskeyHandler).
func (c *PasskeyController) PasskeyHandler() *PasskeyHandler {
	return c.handler
}
