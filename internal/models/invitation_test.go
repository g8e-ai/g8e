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

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestInvitation_IsValid(t *testing.T) {
	now := time.Now().UTC()
	past := now.Add(-1 * time.Hour)
	future := now.Add(1 * time.Hour)

	tests := []struct {
		name        string
		invitation  *Invitation
		wantErr     bool
		errContains string
	}{
		{
			name:        "nil invitation",
			invitation:  nil,
			wantErr:     true,
			errContains: "invitation is nil",
		},
		{
			name: "consumed invitation",
			invitation: &Invitation{
				ID:         "inv-123",
				IsConsumed: true,
				ExpiresAt:  future,
			},
			wantErr:     true,
			errContains: "already consumed",
		},
		{
			name: "expired invitation",
			invitation: &Invitation{
				ID:         "inv-123",
				IsConsumed: false,
				ExpiresAt:  past,
			},
			wantErr:     true,
			errContains: "expired",
		},
		{
			name: "valid invitation",
			invitation: &Invitation{
				ID:         "inv-123",
				IsConsumed: false,
				ExpiresAt:  future,
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.invitation.IsValid()
			if tt.wantErr {
				require.Error(t, err)
				if tt.errContains != "" {
					assert.Contains(t, err.Error(), tt.errContains)
				}
			} else {
				require.NoError(t, err)
			}
		})
	}
}
