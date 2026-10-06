// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package operator

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/models"
)

// CheckStoppable reports why an Operator cannot receive a governed shutdown:
// only a remote Operator can, which the Gateway enforces again on the request.
func CheckStoppable(op models.OperatorDocumentGo) error {
	switch op.OperatorType {
	case constants.OperatorTypeRemote:
		return nil
	case constants.OperatorTypeEmbedded:
		return constants.ErrOperatorStopEmbedded
	default:
		return constants.ErrOperatorStopNotRemote
	}
}

// NewStopRequest builds the body of POST APIPaths.OperatorsStop, shared by
// 'g8e operator stop' and the TUI.
func NewStopRequest(operatorSessionID, reason string) models.StopOperatorRequest {
	return models.StopOperatorRequest{OperatorSessionID: operatorSessionID, Reason: strings.TrimSpace(reason)}
}

// DecodeStopResponse parses the Gateway's reply to a shutdown request and
// rejects an unsuccessful one.
func DecodeStopResponse(body []byte) (models.StopOperatorResponse, error) {
	var response models.StopOperatorResponse
	if err := json.Unmarshal(body, &response); err != nil {
		return response, fmt.Errorf("operator stop: parse response: %w", err)
	}
	if !response.Success {
		return response, fmt.Errorf("operator stop: shutdown request was unsuccessful")
	}
	return response, nil
}
