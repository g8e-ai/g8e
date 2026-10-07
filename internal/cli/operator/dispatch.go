// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

// Package operator provides native CLI helpers for governed operator command
// dispatch through the gateway POST /api/v1/operators/commands surface.
package operator

import (
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"google.golang.org/protobuf/proto"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	operatorv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/operator/v1"
)

// DispatchRequest is the typed JSON body for POST /api/v1/operators/commands.
type DispatchRequest struct {
	TargetOperatorSessionID string `json:"target_operator_session_id"`
	EventType               string `json:"event_type"`
	Payload                 []byte `json:"payload"`
	TargetResource          string `json:"target_resource,omitempty"`
	CliSessionID            string `json:"cli_session_id,omitempty"`
}

// DispatchResponse mirrors gateway.DispatchResponse for CLI decoding.
type DispatchResponse struct {
	Success       bool   `json:"success"`
	TransactionID string `json:"transaction_id"`
	EventType     string `json:"event_type,omitempty"`
	ActionType    string `json:"action_type,omitempty"`
	ResultPayload []byte `json:"result_payload,omitempty"`
	Error         string `json:"error,omitempty"`
}

// RunResult is the decoded outcome for one operator session dispatch.
type RunResult struct {
	OperatorSessionID string `json:"operator_session_id"`
	OperatorID        string `json:"operator_id"`
	Success           bool   `json:"success"`
	TransactionID     string `json:"transaction_id,omitempty"`
	ExitCode          int32  `json:"exit_code,omitempty"`
	Stdout            string `json:"stdout,omitempty"`
	Stderr            string `json:"stderr,omitempty"`
	Error             string `json:"error,omitempty"`
	// StartedAt is when the dispatch request began; DurationMs is its
	// monotonic-clock elapsed time through response decoding.
	StartedAt  time.Time `json:"started_at"`
	DurationMs float64   `json:"duration_ms"`
}

// RunSummary aggregates dispatch latency for one fan-out. Percentiles use the
// nearest-rank method over successful dispatches only, so fast-failing
// dispatches cannot make the distribution look better than it is.
type RunSummary struct {
	Targets     int     `json:"targets"`
	Succeeded   int     `json:"succeeded"`
	Failed      int     `json:"failed"`
	WallMs      float64 `json:"wall_ms"`
	P50Ms       float64 `json:"p50_ms"`
	P95Ms       float64 `json:"p95_ms"`
	P99Ms       float64 `json:"p99_ms"`
	MaxMs       float64 `json:"max_ms"`
	Concurrency int     `json:"concurrency"`
}

// SummarizeRun builds the RunSummary for results produced with the given
// concurrency over wall of elapsed time.
func SummarizeRun(results []RunResult, wall time.Duration, concurrency int) RunSummary {
	summary := RunSummary{
		Targets:     len(results),
		WallMs:      DurationMs(wall),
		Concurrency: concurrency,
	}
	durations := make([]float64, 0, len(results))
	for _, result := range results {
		if !result.Success {
			summary.Failed++
			continue
		}
		summary.Succeeded++
		durations = append(durations, result.DurationMs)
	}
	if len(durations) == 0 {
		return summary
	}
	sort.Float64s(durations)
	summary.P50Ms = nearestRank(durations, 50)
	summary.P95Ms = nearestRank(durations, 95)
	summary.P99Ms = nearestRank(durations, 99)
	summary.MaxMs = durations[len(durations)-1]
	return summary
}

// DurationMs converts d to fractional milliseconds.
func DurationMs(d time.Duration) float64 {
	return float64(d) / float64(time.Millisecond)
}

// nearestRank returns the percentile p (1..100) of ascending-sorted values.
func nearestRank(sorted []float64, p int) float64 {
	rank := int(math.Ceil(float64(p) / 100 * float64(len(sorted))))
	if rank < 1 {
		rank = 1
	}
	return sorted[rank-1]
}

// MarshalExecuteBashPayload builds the EXECUTE_BASH CommandRequested protobuf payload.
func MarshalExecuteBashPayload(command, executionID, justification string) ([]byte, error) {
	payload, err := proto.Marshal(&operatorv1.CommandRequested{
		Command:       command,
		ExecutionId:   executionID,
		Justification: justification,
	})
	if err != nil {
		return nil, fmt.Errorf("operator dispatch: marshal execute bash payload: %w", err)
	}
	return payload, nil
}

// BuildExecuteBashDispatchRequest assembles a governed shell dispatch request.
func BuildExecuteBashDispatchRequest(operatorSessionID, command, executionID, cliSessionID string) (DispatchRequest, error) {
	command = strings.TrimSpace(command)
	if operatorSessionID == "" || command == "" || executionID == "" {
		return DispatchRequest{}, constants.ErrMissingRequiredField
	}
	payload, err := MarshalExecuteBashPayload(command, executionID, "g8e operator run")
	if err != nil {
		return DispatchRequest{}, err
	}
	return DispatchRequest{
		TargetOperatorSessionID: operatorSessionID,
		EventType:               string(constants.Event.Operator.Command.Requested),
		Payload:                 payload,
		TargetResource:          "cli",
		CliSessionID:            cliSessionID,
	}, nil
}

// DecodeDispatchResponse unmarshals the gateway dispatch JSON response.
func DecodeDispatchResponse(raw []byte) (*DispatchResponse, error) {
	if len(raw) == 0 {
		return nil, fmt.Errorf("%w: empty dispatch response", constants.ErrInvalidJSONResponse)
	}
	response := &DispatchResponse{}
	if err := json.Unmarshal(raw, response); err != nil {
		return nil, fmt.Errorf("%w: decode dispatch response: %w", constants.ErrInvalidJSONResponse, err)
	}
	return response, nil
}

// DecodeCommandResult extracts stdout/stderr/exit code from a dispatch result payload.
func DecodeCommandResult(payload []byte) (*operatorv1.CommandResult, error) {
	return ParseCommandResult(&DispatchResponse{ResultPayload: payload})
}

// ParseCommandResult decodes the governed operator command result from a dispatch
// response. The gateway blocks until a terminal command.completed/failed event
// arrives; an empty payload after success indicates a protocol mismatch.
func ParseCommandResult(response *DispatchResponse) (*operatorv1.CommandResult, error) {
	if response == nil {
		return nil, fmt.Errorf("%w: dispatch response is nil", constants.ErrMissingRequiredField)
	}
	if len(response.ResultPayload) == 0 {
		return nil, fmt.Errorf(
			"operator dispatch: empty command result payload (event_type=%s action_type=%s)",
			response.EventType,
			response.ActionType,
		)
	}
	result := &operatorv1.CommandResult{}
	if err := proto.Unmarshal(response.ResultPayload, result); err != nil {
		return nil, fmt.Errorf("operator dispatch: decode command result: %w", err)
	}
	return result, nil
}
