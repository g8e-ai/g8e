// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package operatorcmd

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	operatorv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/operator/v1"

	"github.com/g8e-ai/g8e/v2/internal/models"
	"github.com/spf13/cobra"
)

type localOperatorProcess struct {
	pid       int
	dir       string
	sessionID string
	wait      func(time.Duration) (bool, error)
	signal    func(bool) error // false: TERM, true: KILL
	close     func()
}

func (p localOperatorProcess) matches(op operatorv1.OperatorDocument) bool {
	if p.sessionID != "" {
		return p.sessionID == op.OperatorSessionID
	}
	hostname, err := os.Hostname()
	return err == nil && operatorHostnameValue(op) == hostname && op.LocalDir != "" && filepath.IsAbs(op.LocalDir) && filepath.Clean(op.LocalDir) == p.dir
}

type operatorStopResult struct {
	models.StopOperatorResponse
	PID           int    `json:"pid"`
	Method        string `json:"method"`
	Error         string `json:"error,omitempty"`
	GovernedError string `json:"governed_error,omitempty"`
}

func stopLocalOperator(cmd *cobra.Command, p localOperatorProcess, response models.StopOperatorResponse, governedErr error, grace time.Duration) operatorStopResult {
	result := operatorStopResult{StopOperatorResponse: response, PID: p.pid}
	if governedErr != nil {
		result.GovernedError = governedErr.Error()
		cmd.PrintErrf("PID %d: governed shutdown unavailable (%v); terminating locally.\n", p.pid, governedErr)
	}
	fail := func(err error) operatorStopResult {
		result.Success = false
		result.Method = "failed"
		result.Error = err.Error()
		return result
	}
	// Even an unsuccessful request might have delivered shutdown before the response failed.
	exited, err := p.wait(grace)
	if err != nil {
		return fail(err)
	}
	if exited {
		result.Success = true
		result.Method = "exited"
		return result
	}
	if err := cmd.Context().Err(); err != nil {
		return fail(err)
	}
	if err := p.signal(false); err != nil {
		return fail(fmt.Errorf("send TERM: %w", err))
	}
	exited, err = p.wait(grace)
	if err != nil {
		return fail(err)
	}
	if exited {
		result.Success = true
		result.Method = "TERM"
		return result
	}
	if err := cmd.Context().Err(); err != nil {
		return fail(err)
	}
	if err := p.signal(true); err != nil {
		return fail(fmt.Errorf("send KILL: %w", err))
	}
	exited, err = p.wait(2 * time.Second)
	if err != nil {
		return fail(err)
	}
	if !exited {
		return fail(errors.New("process did not exit after KILL"))
	}
	result.Success = true
	result.Method = "KILL"
	return result
}
