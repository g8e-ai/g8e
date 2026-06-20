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

	operatorv1 "github.com/g8e-ai/g8e/protocol/proto/g8e/operator/v1"
	"github.com/stretchr/testify/assert"
)

func TestExecutionStatus(t *testing.T) {
	// Test that protobuf enum values are properly typed
	// This test ensures type safety at the boundary
	tests := []struct {
		name   string
		status operatorv1.ExecutionStatus
	}{
		{"unspecified", operatorv1.ExecutionStatus_EXECUTION_STATUS_UNSPECIFIED},
		{"executing", operatorv1.ExecutionStatus_EXECUTION_STATUS_EXECUTING},
		{"completed", operatorv1.ExecutionStatus_EXECUTION_STATUS_COMPLETED},
		{"failed", operatorv1.ExecutionStatus_EXECUTION_STATUS_FAILED},
		{"cancelled", operatorv1.ExecutionStatus_EXECUTION_STATUS_CANCELLED},
		{"timeout", operatorv1.ExecutionStatus_EXECUTION_STATUS_TIMEOUT},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Verify the enum value is a valid protobuf enum
			// The zero value (UNSPECIFIED) is valid for all other values
			if tt.name != "unspecified" {
				assert.NotEqual(t, operatorv1.ExecutionStatus_EXECUTION_STATUS_UNSPECIFIED, tt.status)
			}
		})
	}
}
