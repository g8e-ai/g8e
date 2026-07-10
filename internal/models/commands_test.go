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

package models

import (
	"testing"
	"time"

	"github.com/g8e-ai/g8e/internal/constants"
	operatorv1 "github.com/g8e-ai/g8e/protocol/proto/g8e/operator/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPayloadFromDomainResult_NilReturnsNil(t *testing.T) {
	t.Parallel()

	assert.Nil(t, PayloadFromDomainResult(nil))
}

func TestPayloadFromDomainResult_PopulatesRequiredFields(t *testing.T) {
	t.Parallel()

	r := &ExecutionResult{
		ExecutionID:       "exec-1",
		CaseID:            "case-1",
		InvestigationID:   "inv-1",
		Command:           "ls",
		Args:              []string{"-la"},
		Status:            operatorv1.ExecutionStatus_EXECUTION_STATUS_COMPLETED,
		DurationSeconds:   1.5,
		OperatorID:        "op-1",
		OperatorSessionID: "sess-1",
		Stdout:            "output",
		Stderr:            "error",
		StdoutSize:        6,
		StderrSize:        5,
		StdoutHash:        "abc123",
		StderrHash:        "def456",
		StoredLocally:     true,
	}

	p := PayloadFromDomainResult(r)
	require.NotNil(t, p)

	assert.Equal(t, "execution_result", p.PayloadType)
	assert.Equal(t, r.ExecutionID, p.ExecutionID)
	assert.Equal(t, r.CaseID, p.CaseID)
	assert.Equal(t, r.InvestigationID, p.InvestigationID)
	assert.Equal(t, r.Command, p.Command)
	assert.Equal(t, r.Args, p.Args)
	assert.Equal(t, r.Status, p.Status)
	assert.Equal(t, r.DurationSeconds, p.DurationSeconds)
	assert.Equal(t, r.OperatorID, p.OperatorID)
	assert.Equal(t, r.OperatorSessionID, p.OperatorSessionID)
	assert.Equal(t, r.Stdout, p.Stdout)
	assert.Equal(t, r.Stderr, p.Stderr)
	assert.Equal(t, r.StdoutSize, p.StdoutSize)
	assert.Equal(t, r.StderrSize, p.StderrSize)
	assert.Equal(t, r.StdoutHash, p.StdoutHash)
	assert.Equal(t, r.StderrHash, p.StderrHash)
	assert.Equal(t, r.StoredLocally, p.StoredLocally)
}

func TestPayloadFromDomainResult_OptionalFieldsOmittedWhenZero(t *testing.T) {
	t.Parallel()

	r := &ExecutionResult{
		ExecutionID: "exec-2",
	}

	p := PayloadFromDomainResult(r)
	require.NotNil(t, p)

	assert.Nil(t, p.TaskID)
	assert.Nil(t, p.ReturnCode)
	assert.Nil(t, p.ErrorMessage)
	assert.Nil(t, p.ErrorType)
	assert.Nil(t, p.StartTime)
	assert.Nil(t, p.EndTime)
}

func TestPayloadFromDomainResult_OptionalFieldsPopulatedWhenNonZero(t *testing.T) {
	t.Parallel()

	taskID := "task-1"
	retCode := 42
	errMsg := "command failed"
	errType := "timeout"
	startTime := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	endTime := time.Date(2026, 1, 1, 12, 0, 5, 0, time.UTC)

	r := &ExecutionResult{
		ExecutionID:  "exec-3",
		TaskID:       taskID,
		ReturnCode:   retCode,
		ErrorMessage: errMsg,
		ErrorType:    errType,
		StartTime:    startTime,
		EndTime:      endTime,
	}

	p := PayloadFromDomainResult(r)
	require.NotNil(t, p)

	require.NotNil(t, p.TaskID)
	assert.Equal(t, taskID, *p.TaskID)

	require.NotNil(t, p.ReturnCode)
	assert.Equal(t, retCode, *p.ReturnCode)

	require.NotNil(t, p.ErrorMessage)
	assert.Equal(t, errMsg, *p.ErrorMessage)

	require.NotNil(t, p.ErrorType)
	assert.Equal(t, errType, *p.ErrorType)

	require.NotNil(t, p.StartTime)
	assert.Equal(t, startTime, *p.StartTime)

	require.NotNil(t, p.EndTime)
	assert.Equal(t, endTime, *p.EndTime)
}

func TestPayloadFromDomainResult_ReturnCodeZeroIsOmitted(t *testing.T) {
	t.Parallel()

	r := &ExecutionResult{
		ExecutionID: "exec-4",
		ReturnCode:  0,
	}

	p := PayloadFromDomainResult(r)
	require.NotNil(t, p)
	assert.Nil(t, p.ReturnCode)
}

func TestPayloadFromDomainResult_PreservesTerminalOutputAndSystemInfo(t *testing.T) {
	t.Parallel()

	termOut := &TerminalOutput{
		Command:         "ls -la",
		CombinedOutput:  "total 0",
	}
	sysInfo := &ExecutionSystemInfo{
		Hostname: "test-host",
	}
	envInfo := &ExecutionEnvironmentInfo{
		ComponentName: "g8eo",
	}

	r := &ExecutionResult{
		ExecutionID:    "exec-5",
		TerminalOutput: termOut,
		SystemInfo:     sysInfo,
		EnvironmentInfo: envInfo,
	}

	p := PayloadFromDomainResult(r)
	require.NotNil(t, p)

	require.NotNil(t, p.TerminalOutput)
	assert.Equal(t, "ls -la", p.TerminalOutput.Command)

	require.NotNil(t, p.SystemInfo)
	assert.Equal(t, "test-host", p.SystemInfo.Hostname)

	require.NotNil(t, p.EnvironmentInfo)
	assert.Equal(t, constants.ComponentName("g8eo"), p.EnvironmentInfo.ComponentName)
}
