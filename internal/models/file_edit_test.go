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

	"github.com/g8e-ai/g8e/internal/constants"
	"github.com/stretchr/testify/assert"
)

func TestFileEditOperations(t *testing.T) {
	tests := []struct {
		name      string
		operation constants.FileOperation
		expected  string
	}{
		{"read", constants.FileOperationRead, "read"},
		{"write", constants.FileOperationWrite, "write"},
		{"replace", constants.FileOperationReplace, "replace"},
		{"delete", constants.FileOperationDelete, "delete"},
		{"insert", constants.FileOperationInsert, "insert"},
		{"patch", constants.FileOperationPatch, "patch"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, string(tt.operation))
		})
	}
}
