// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package auth

import (
	"context"
	"fmt"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/models"
	"github.com/g8e-ai/g8e/v2/internal/services/fs"
)

// CLISessionLogout is the result of a successful server-side logout.
type CLISessionLogout struct {
	UserID                    string
	Scope                     constants.LogoutScope
	WebSessionsTerminated     int
	CLISessionsTerminated     int
	CLICertificatesRevoked    int
	UnboundOperatorSessionIDs []string
}

// ValidateCLILogoutResponse validates an already-decoded logout response.
func ValidateCLILogoutResponse(resp models.CLILogoutResponse) (CLISessionLogout, error) {
	if !resp.Success {
		return CLISessionLogout{}, fmt.Errorf("%w: logout unsuccessful", constants.ErrCLIRefreshFailed)
	}
	if resp.UserID == "" {
		return CLISessionLogout{}, constants.ErrMissingRequiredField
	}
	if !resp.Scope.Valid() {
		return CLISessionLogout{}, fmt.Errorf("%w: %q", constants.ErrLogoutScopeInvalid, resp.Scope)
	}
	return CLISessionLogout{
		UserID:                    resp.UserID,
		Scope:                     resp.Scope,
		WebSessionsTerminated:     resp.WebSessionsTerminated,
		CLISessionsTerminated:     resp.CLISessionsTerminated,
		CLICertificatesRevoked:    resp.CLICertificatesRevoked,
		UnboundOperatorSessionIDs: resp.UnboundOperatorSessionIDs,
	}, nil
}

// Logout asks the Gateway to terminate the authenticated user's sessions
// (POST /api/v1/auth/cli/logout over the public HTTPS surface): unbind the
// operators bound to them, end them, and for the CLI scope revoke the CLI
// certificates behind them. It does not touch local credentials; the caller
// clears those only after the Gateway confirms.
func (c *EnrollmentClient) Logout(ctx context.Context, fileSvc fs.RuntimeFileService, scope constants.LogoutScope) (CLISessionLogout, error) {
	mtlsClient, err := BuildMTLSClient(fileSvc, c.cfg, httpTimeout)
	if err != nil {
		return CLISessionLogout{}, err
	}

	publicURL := c.cfg.OperatorPublicURL()

	store := NewCredentialStore(fileSvc, c.cfg)
	creds, _ := store.LoadCredentials(ctx)
	headers := map[string]string{}
	if creds != nil && creds.CLISessionID != "" {
		headers[constants.HeaderCLISessionID] = creds.CLISessionID
	}

	var respBody models.CLILogoutResponse
	if err := postJSON(ctx, mtlsClient, publicURL+constants.APIPaths.AuthCLILogout, models.CLILogoutRequest{Scope: scope}, &respBody, headers); err != nil {
		return CLISessionLogout{}, err
	}
	return ValidateCLILogoutResponse(respBody)
}
