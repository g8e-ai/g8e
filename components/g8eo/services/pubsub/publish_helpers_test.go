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

package pubsub

import (
	"testing"

	"github.com/g8e-ai/g8e/components/g8eo/constants"
	"github.com/g8e-ai/g8e/components/g8eo/models"
	"github.com/stretchr/testify/assert"
)

func TestSetExecutionIDOnPayload_LFAAErrorPayload(t *testing.T) {
	payload := &models.LFAAErrorPayload{
		Success: false,
		Error:   "test error",
	}
	setExecutionIDOnPayload(payload, "msg-abc")
	assert.Equal(t, "msg-abc", payload.ExecutionID)
}

func TestSetExecutionIDOnPayload_FileEditResultPayload(t *testing.T) {
	payload := &models.FileEditResultPayload{
		ExecutionID: "original-id",
		Status:      constants.ExecutionStatusCompleted,
	}
	setExecutionIDOnPayload(payload, "msg-abc")
	assert.Equal(t, "msg-abc", payload.ExecutionID)
}

func TestSetExecutionIDOnPayload_FsListResultPayload(t *testing.T) {
	payload := &models.FsListResultPayload{
		ExecutionID: "",
		Status:      constants.ExecutionStatusCompleted,
	}
	setExecutionIDOnPayload(payload, "msg-abc")
	assert.Equal(t, "msg-abc", payload.ExecutionID)
}

func TestSetExecutionIDOnPayload_PortCheckResultPayload(t *testing.T) {
	payload := &models.PortCheckResultPayload{
		ExecutionID: "existing-id",
		Status:      constants.ExecutionStatusCompleted,
	}
	setExecutionIDOnPayload(payload, "msg-abc")
	assert.Equal(t, "msg-abc", payload.ExecutionID)
}

func TestSetExecutionIDOnPayload_EmptyExecutionID(t *testing.T) {
	payload := &models.LFAAErrorPayload{
		Success: false,
		Error:   "test error",
	}
	setExecutionIDOnPayload(payload, "")
	assert.Equal(t, "", payload.ExecutionID)
}

func TestSetExecutionIDOnPayload_UnsupportedType(t *testing.T) {
	type unsupportedPayload struct {
		Field string
	}
	payload := &unsupportedPayload{Field: "value"}
	setExecutionIDOnPayload(payload, "msg-abc")
	assert.Equal(t, "value", payload.Field)
}

func TestSetExecutionIDOnPayload_FetchFileDiffResultPayload(t *testing.T) {
	payload := &models.FetchFileDiffResultPayload{
		Success:     true,
		Error:       nil,
		ExecutionID: "original-id",
	}
	setExecutionIDOnPayload(payload, "msg-xyz")
	assert.Equal(t, "msg-xyz", payload.ExecutionID)
}

func TestSetExecutionIDOnPayload_FetchHistoryResultPayload(t *testing.T) {
	payload := &models.FetchHistoryResultPayload{
		Success:     true,
		ExecutionID: "",
	}
	setExecutionIDOnPayload(payload, "msg-xyz")
	assert.Equal(t, "msg-xyz", payload.ExecutionID)
}

func TestSetExecutionIDOnPayload_FetchFileHistoryResultPayload(t *testing.T) {
	payload := &models.FetchFileHistoryResultPayload{
		Success:     false,
		Error:       "test error",
		ExecutionID: "old-id",
	}
	setExecutionIDOnPayload(payload, "msg-xyz")
	assert.Equal(t, "msg-xyz", payload.ExecutionID)
}

func TestSetExecutionIDOnPayload_RestoreFileResultPayload(t *testing.T) {
	payload := &models.RestoreFileResultPayload{
		Success:     true,
		ExecutionID: "",
	}
	setExecutionIDOnPayload(payload, "msg-xyz")
	assert.Equal(t, "msg-xyz", payload.ExecutionID)
}
