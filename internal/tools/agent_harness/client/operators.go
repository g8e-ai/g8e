// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package client

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/models"
)

// OperatorBySession resolves an exact pinned Operator session through
// GET /api/v1/operators/session/{id} and returns the typed operator document.
// The returned document must echo the requested session ID; a mismatched
// response is rejected as invalid rather than silently rebound.
func (c *Client) OperatorBySession(ctx context.Context, operatorSessionID string) (*models.OperatorDocumentGo, []byte, error) {
	if operatorSessionID == "" {
		return nil, nil, constants.ErrOperatorSessionIDRequired
	}
	u := c.cfg.MTLSBaseURL + constants.APIPaths.OperatorsSession + url.PathEscape(operatorSessionID)
	status, body, err := c.do(ctx, c.auditorPersona(), http.MethodGet, u, nil)
	if err != nil {
		return nil, body, err
	}
	if status < http.StatusOK || status >= http.StatusMultipleChoices {
		return nil, body, fmt.Errorf("%w: operator session lookup returned status %d", constants.ErrHTTPStatusError, status)
	}
	response := &models.OperatorResponse{}
	if err := json.Unmarshal(body, response); err != nil {
		return nil, body, fmt.Errorf("%w: decode operator session response: %v", constants.ErrInvalidJSONResponse, err)
	}
	if !response.Success || response.Operator == nil {
		return nil, body, fmt.Errorf("%w: operator session response is incomplete", constants.ErrInvalidJSONResponse)
	}
	if response.Operator.OperatorSessionID != operatorSessionID {
		return nil, body, fmt.Errorf("%w: operator session response is bound to a different session", constants.ErrInvalidJSONResponse)
	}
	return response.Operator, body, nil
}

// DiscoverRemoteOperator lists the authenticated user's Operators through
// GET /api/v1/operators and selects exactly one active remote Operator with a
// non-empty session. Zero matches return ErrEvaluationTargetUnavailable and
// multiple matches return ErrEvaluationTargetAmbiguous; discovery never picks
// arbitrarily between candidate execution boundaries.
func (c *Client) DiscoverRemoteOperator(ctx context.Context) (*models.OperatorDocumentGo, []byte, error) {
	if c.cfg.UserID == "" {
		return nil, nil, fmt.Errorf("%w: authenticated user id is required for operator discovery", constants.ErrMissingRequiredField)
	}
	u := c.cfg.MTLSBaseURL + constants.APIPaths.Operators + "?" + url.Values{"user_id": {c.cfg.UserID}}.Encode()
	status, body, err := c.do(ctx, c.auditorPersona(), http.MethodGet, u, nil)
	if err != nil {
		return nil, body, err
	}
	if status < http.StatusOK || status >= http.StatusMultipleChoices {
		return nil, body, fmt.Errorf("%w: operator list returned status %d", constants.ErrHTTPStatusError, status)
	}
	response := &models.OperatorSlotResponse{}
	if err := json.Unmarshal(body, response); err != nil {
		return nil, body, fmt.Errorf("%w: decode operator list: %v", constants.ErrInvalidJSONResponse, err)
	}
	if !response.Success {
		return nil, body, fmt.Errorf("%w: operator list response is incomplete", constants.ErrInvalidJSONResponse)
	}
	var matches []models.OperatorDocumentGo
	for _, op := range response.Operators {
		if op.Status == constants.OperatorStatusActive && op.OperatorType == constants.OperatorTypeRemote && op.OperatorSessionID != "" {
			matches = append(matches, op)
		}
	}
	switch len(matches) {
	case 0:
		return nil, body, fmt.Errorf("%w: no active remote operator session", constants.ErrEvaluationTargetUnavailable)
	case 1:
		selected := matches[0]
		return &selected, body, nil
	default:
		return nil, body, fmt.Errorf("%w: %d active remote operator sessions", constants.ErrEvaluationTargetAmbiguous, len(matches))
	}
}
