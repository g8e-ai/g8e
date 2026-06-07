package models

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
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
				assert.Error(t, err)
				if tt.errContains != "" {
					assert.Contains(t, err.Error(), tt.errContains)
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
