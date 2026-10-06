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

	"github.com/g8e-ai/g8e/v2/internal/cli/config"
	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/models"
)

// ApprovalPageURL returns the Gateway's public browser approval page for a
// suspended L3 transaction. The page runs the WebAuthn ceremony and redirects
// into the console SPA; completion is announced on the CLI session's SSE
// stream as approval.completed.
func ApprovalPageURL(cfg *config.Config, txHash string) string {
	return cfg.OperatorPublicURL() + constants.APIPaths.ApprovePagePrefix + txHash
}

// ApprovalStatusPath returns the mTLS path that reports a suspended L3
// transaction's approval status to its CLI session.
func ApprovalStatusPath(txHash string) string {
	return constants.APIPaths.ApprovalsCLIStatus + txHash
}

// VerifyApprovalStatus decodes an ApprovalStatusPath response and returns an
// error unless the transaction is approved. Callers verify through this after
// approval.completed so an SSE event alone never counts as an approval.
func VerifyApprovalStatus(txHash string, body []byte) (models.ApprovalStatusResponse, error) {
	var status models.ApprovalStatusResponse
	if err := json.Unmarshal(body, &status); err != nil {
		return status, fmt.Errorf("parse status response: %w", err)
	}
	switch status.Status {
	case string(constants.SuspendedTxStatusApproved):
		return status, nil
	case string(constants.SuspendedTxStatusExpiredOrNotFound):
		return status, fmt.Errorf("transaction %s expired or not found", txHash)
	default:
		return status, fmt.Errorf("unexpected status %q for transaction %s", status.Status, txHash)
	}
}
