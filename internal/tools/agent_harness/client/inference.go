// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package client

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"google.golang.org/protobuf/encoding/protojson"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/models"
	operatorv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/operator/v1"
)

// ListOperators returns the authenticated user's Operator documents through
// GET /api/v1/operators.
func (c *Client) ListOperators(ctx context.Context) ([]models.OperatorDocumentGo, []byte, error) {
	if c.cfg.UserID == "" {
		return nil, nil, fmt.Errorf("%w: authenticated user id is required for operator list", constants.ErrMissingRequiredField)
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
	return response.Operators, body, nil
}

// DiscoverInferenceOperator selects exactly one active inference-capable remote
// Operator. When operatorSessionID is non-empty it must match that session.
func (c *Client) DiscoverInferenceOperator(ctx context.Context, operatorSessionID string) (*models.OperatorDocumentGo, []byte, error) {
	operators, body, err := c.ListOperators(ctx)
	if err != nil {
		return nil, body, err
	}
	var matches []models.OperatorDocumentGo
	for _, op := range operators {
		if op.Status != constants.OperatorStatusActive || op.OperatorType != constants.OperatorTypeRemote {
			continue
		}
		if op.RuntimeConfig == nil || !op.RuntimeConfig.InferenceEnabled {
			continue
		}
		if op.OperatorSessionID == "" {
			continue
		}
		if operatorSessionID != "" && op.OperatorSessionID != operatorSessionID {
			continue
		}
		matches = append(matches, op)
	}
	switch len(matches) {
	case 0:
		if operatorSessionID != "" {
			return nil, body, fmt.Errorf("%w: session %s", constants.ErrInferenceOperatorNotCapable, operatorSessionID)
		}
		return nil, body, constants.ErrInferenceOperatorNotFound
	case 1:
		selected := matches[0]
		return &selected, body, nil
	default:
		return nil, body, constants.ErrInferenceOperatorAmbiguous
	}
}

// DispatchInference sends POST /api/v1/inference/dispatch using the configured
// delegated app workload certificate. The caller must configure Auth with an
// enrolled app cert such as g8ee; CLI session headers are not attached.
func (c *Client) DispatchInference(ctx context.Context, req *operatorv1.InferenceDispatchRequest) (*operatorv1.InferenceDispatchResponse, []byte, error) {
	if req == nil {
		return nil, nil, constants.ErrMissingRequiredField
	}
	body, err := protojson.Marshal(req)
	if err != nil {
		return nil, nil, fmt.Errorf("%w: marshal inference dispatch request: %v", constants.ErrRequestMarshalFailed, err)
	}
	status, respBody, err := c.doInference(ctx, c.appWorkloadPersona(), http.MethodPost, c.cfg.MTLSBaseURL+constants.APIPaths.InferenceDispatch, body, nil)
	if err != nil {
		return nil, respBody, err
	}
	if status < http.StatusOK || status >= http.StatusMultipleChoices {
		return nil, respBody, fmt.Errorf("%w: inference dispatch returned status %d", constants.ErrHTTPStatusError, status)
	}
	response := &operatorv1.InferenceDispatchResponse{}
	if err := protojson.Unmarshal(respBody, response); err != nil {
		return nil, respBody, fmt.Errorf("%w: decode inference dispatch response: %v", constants.ErrInvalidJSONResponse, err)
	}
	return response, respBody, nil
}

// InferenceDispatchStreamResult carries the terminal governed response and any
// live progress events observed during a streaming dispatch.
type InferenceDispatchStreamResult struct {
	Progress   []*operatorv1.InferenceProgressEvent
	Completion *operatorv1.InferenceDispatchResponse
}

// DispatchInferenceStream sends POST /api/v1/inference/dispatch with stream=true
// and consumes NDJSON InferenceDispatchStreamFrame records until completion.
func (c *Client) DispatchInferenceStream(ctx context.Context, req *operatorv1.InferenceDispatchRequest) (*InferenceDispatchStreamResult, error) {
	if req == nil {
		return nil, constants.ErrMissingRequiredField
	}
	req.Stream = true
	body, err := protojson.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("%w: marshal inference dispatch request: %v", constants.ErrRequestMarshalFailed, err)
	}
	accept := constants.HeaderValueApplicationNDJSON
	status, respBody, err := c.doInference(ctx, c.appWorkloadPersona(), http.MethodPost, c.cfg.MTLSBaseURL+constants.APIPaths.InferenceDispatch, body, &accept)
	if err != nil {
		return nil, err
	}
	if status < http.StatusOK || status >= http.StatusMultipleChoices {
		return nil, fmt.Errorf("%w: inference dispatch stream returned status %d", constants.ErrHTTPStatusError, status)
	}
	result := &InferenceDispatchStreamResult{}
	reader := bufio.NewReader(bytes.NewReader(respBody))
	unmarshal := protojson.UnmarshalOptions{DiscardUnknown: true}
	for {
		line, readErr := reader.ReadString('\n')
		if readErr != nil && readErr != io.EOF {
			return nil, fmt.Errorf("%w: read inference dispatch stream: %v", constants.ErrInvalidJSONResponse, readErr)
		}
		line = strings.TrimSpace(line)
		if line != "" {
			frame := &operatorv1.InferenceDispatchStreamFrame{}
			if err := unmarshal.Unmarshal([]byte(line), frame); err != nil {
				return nil, fmt.Errorf("%w: decode inference dispatch stream frame: %v", constants.ErrInvalidJSONResponse, err)
			}
			switch payload := frame.GetFrame().(type) {
			case *operatorv1.InferenceDispatchStreamFrame_Progress:
				if payload.Progress != nil {
					result.Progress = append(result.Progress, payload.Progress)
				}
			case *operatorv1.InferenceDispatchStreamFrame_Completion:
				result.Completion = payload.Completion
			case *operatorv1.InferenceDispatchStreamFrame_Failure:
				if payload.Failure == nil || payload.Failure.GetReason() == "" {
					return nil, fmt.Errorf("%w: inference dispatch stream failed", constants.ErrInferenceProviderResponseInvalid)
				}
				return nil, fmt.Errorf("%w: %s", constants.ErrInferenceProviderResponseInvalid, payload.Failure.GetReason())
			}
		}
		if readErr == io.EOF {
			break
		}
	}
	if result.Completion == nil {
		return nil, fmt.Errorf("%w: inference dispatch stream missing completion", constants.ErrMissingRequiredField)
	}
	return result, nil
}

func (c *Client) inferenceHTTP() *http.Client {
	return &http.Client{
		Timeout:   0,
		Transport: c.http.Transport,
	}
}

func (c *Client) doInference(ctx context.Context, p Persona, method, url string, body []byte, accept *string) (int, []byte, error) {
	req, err := http.NewRequestWithContext(ctx, method, url, bytes.NewReader(body))
	if err != nil {
		return 0, nil, err
	}
	req.Header.Set("Content-Type", constants.HeaderValueApplicationJSON)
	if accept != nil {
		req.Header.Set("Accept", *accept)
	}
	if p.UserAgent != "" {
		req.Header.Set("User-Agent", p.UserAgent)
	}
	if p.OperatorSessionID != "" {
		req.Header.Set(constants.HeaderOperatorSessionID, p.OperatorSessionID)
	}
	start := time.Now()
	resp, err := c.inferenceHTTP().Do(req)
	ex := Exchange{Persona: p.ID, Method: method, URL: url, At: start}
	attachBody(&ex.ReqBody, &ex.ReqRaw, body)
	if err != nil {
		ex.Err = err.Error()
		ex.LatencyMS = time.Since(start).Milliseconds()
		c.append(ex, c.cfg.Verbose)
		return 0, nil, err
	}
	defer resp.Body.Close()
	out, _ := io.ReadAll(resp.Body)
	ex.Status = resp.StatusCode
	ex.LatencyMS = time.Since(start).Milliseconds()
	attachBody(&ex.RespBody, &ex.RespRaw, out)
	c.append(ex, c.cfg.Verbose)
	return resp.StatusCode, out, nil
}

func (c *Client) appWorkloadPersona() Persona {
	return Persona{ID: "g8ee-probe", UserAgent: "g8e-eval-inference-probe"}
}
