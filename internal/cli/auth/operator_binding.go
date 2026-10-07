// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package auth

import (
	"encoding/json"
	"fmt"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/models"
)

// DecodeCLIBindResponse validates the typed response shared by the CLI bind
// command and the TUI. A replacement session is required on every response.
func DecodeCLIBindResponse(body []byte, requested []string) (CLISessionBind, error) {
	var resp models.CLIBindResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return CLISessionBind{}, fmt.Errorf("%w: %w", constants.ErrInvalidJSONResponse, err)
	}
	return ValidateCLIBindResponse(resp, requested)
}

// ValidateCLIBindResponse validates an already-decoded bind response.
func ValidateCLIBindResponse(resp models.CLIBindResponse, requested []string) (CLISessionBind, error) {
	if !resp.Success {
		return CLISessionBind{}, fmt.Errorf("%w: bind unsuccessful", constants.ErrCLIRefreshFailed)
	}
	if resp.CLISessionID == "" || resp.UserID == "" || resp.OperatorSessionID == "" || resp.OperatorID == "" {
		return CLISessionBind{}, constants.ErrMissingRequiredField
	}
	if len(resp.Bound) != len(requested) {
		return CLISessionBind{}, fmt.Errorf("%w: gateway bound %d of %d operator sessions", constants.ErrCLIRefreshFailed, len(resp.Bound), len(requested))
	}
	return CLISessionBind{
		CLISessionID:      resp.CLISessionID,
		UserID:            resp.UserID,
		OperatorSessionID: resp.OperatorSessionID,
		OperatorID:        resp.OperatorID,
		AlreadyBound:      resp.AlreadyBound,
		Bound:             resp.Bound,
	}, nil
}

// DecodeCLIUnbindResponse validates the typed response shared by the CLI
// unbind command and the TUI.
func DecodeCLIUnbindResponse(body []byte) (CLISessionUnbind, error) {
	var resp models.CLIUnbindResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return CLISessionUnbind{}, fmt.Errorf("%w: %w", constants.ErrInvalidJSONResponse, err)
	}
	return ValidateCLIUnbindResponse(resp)
}

// ValidateCLIUnbindResponse validates an already-decoded unbind response.
func ValidateCLIUnbindResponse(resp models.CLIUnbindResponse) (CLISessionUnbind, error) {
	if !resp.Success {
		return CLISessionUnbind{}, fmt.Errorf("%w: unbind unsuccessful", constants.ErrCLIRefreshFailed)
	}
	if resp.CLISessionID == "" || resp.UserID == "" {
		return CLISessionUnbind{}, constants.ErrMissingRequiredField
	}
	return CLISessionUnbind{CLISessionID: resp.CLISessionID, UserID: resp.UserID, AlreadyUnbound: resp.AlreadyUnbound}, nil
}
