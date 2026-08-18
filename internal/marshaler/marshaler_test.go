// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package marshaler

import (
	"testing"

	"github.com/g8e-ai/g8e/internal/constants"
)

func TestCollectionName(t *testing.T) {
	tests := []struct {
		name string
		c    constants.CollectionName
		want string
	}{
		{"users", constants.CollectionUsers, "users"},
		{"operators", constants.CollectionOperators, "operators"},
		{"web_sessions", constants.CollectionWebSessions, "web_sessions"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := CollectionName(tt.c); got != tt.want {
				t.Errorf("CollectionName() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestDocumentID(t *testing.T) {
	tests := []struct {
		name string
		d    constants.DocumentID
		want string
	}{
		{"platform_settings", constants.DocIDPlatformSettings, "platform_settings"},
		{"user_settings_prefix", constants.DocIDUserSettingsPrefix, "user_settings_"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := DocumentID(tt.d); got != tt.want {
				t.Errorf("DocumentID() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestStatus(t *testing.T) {
	// Test the generic Status function with different types
	type customStatus string
	const (
		customActive customStatus = "active"
		customIdle   customStatus = "idle"
	)

	tests := []struct {
		name string
		s    any
		want string
	}{
		{"operator_status", constants.OperatorStatusActive, "active"},
		{"user_status", constants.UserStatusActive, "active"},
		{"execution_status", constants.ExecutionStatusPending, "pending"},
		{"custom_type", customActive, "active"},
		{"custom_idle", customIdle, "idle"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got string
			switch v := tt.s.(type) {
			case constants.OperatorStatus:
				got = Status(v)
			case constants.UserStatus:
				got = Status(v)
			case constants.ExecutionStatus:
				got = Status(v)
			case customStatus:
				got = Status(v)
			default:
				t.Fatalf("unsupported type %T", tt.s)
			}
			if got != tt.want {
				t.Errorf("Status() = %v, want %v", got, tt.want)
			}
		})
	}
}
