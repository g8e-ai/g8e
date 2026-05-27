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

func TestEnvVar(t *testing.T) {
	tests := []struct {
		name string
		e    constants.EnvVarKey
		want string
	}{
		{"log_level", constants.EnvVar.LogLevel, "G8E_LOG_LEVEL"},
		{"data_dir", constants.EnvVar.DataDir, "G8E_DATA_DIR"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := EnvVar(tt.e); got != tt.want {
				t.Errorf("EnvVar() = %v, want %v", got, tt.want)
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

func TestOperatorStatus(t *testing.T) {
	tests := []struct {
		name string
		s    constants.OperatorStatus
		want string
	}{
		{"active", constants.OperatorStatusActive, "active"},
		{"available", constants.OperatorStatusAvailable, "available"},
		{"bound", constants.OperatorStatusBound, "bound"},
		{"offline", constants.OperatorStatusOffline, "offline"},
		{"stale", constants.OperatorStatusStale, "stale"},
		{"stopped", constants.OperatorStatusStopped, "stopped"},
		{"terminated", constants.OperatorStatusTerminated, "terminated"},
		{"unavailable", constants.OperatorStatusUnavailable, "unavailable"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := OperatorStatus(tt.s); got != tt.want {
				t.Errorf("OperatorStatus() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestOperatorType(t *testing.T) {
	tests := []struct {
		name string
		t    constants.OperatorType
		want string
	}{
		{"system", constants.OperatorTypeSystem, "system"},
		{"cloud", constants.OperatorTypeCloud, "cloud"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := OperatorType(tt.t); got != tt.want {
				t.Errorf("OperatorType() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestExecutionStatus(t *testing.T) {
	tests := []struct {
		name string
		s    constants.ExecutionStatus
		want string
	}{
		{"pending", constants.ExecutionStatusPending, "pending"},
		{"executing", constants.ExecutionStatusExecuting, "executing"},
		{"completed", constants.ExecutionStatusCompleted, "completed"},
		{"failed", constants.ExecutionStatusFailed, "failed"},
		{"cancelled", constants.ExecutionStatusCancelled, "cancelled"},
		{"timeout", constants.ExecutionStatusTimeout, "timeout"},
		{"denied", constants.ExecutionStatusDenied, "denied"},
		{"feedback", constants.ExecutionStatusFeedback, "feedback"},
		{"cancel_requested", constants.ExecutionStatusCancelRequested, "cancel_requested"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ExecutionStatus(tt.s); got != tt.want {
				t.Errorf("ExecutionStatus() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestActionType(t *testing.T) {
	tests := []struct {
		name string
		a    constants.ActionType
		want string
	}{
		{"execute_bash", constants.ActionTypeExecuteBash, "EXECUTE_BASH"},
		{"file_edit", constants.ActionTypeFileEdit, "FILE_EDIT"},
		{"fs_read", constants.ActionTypeFsRead, "FS_READ"},
		{"fs_list", constants.ActionTypeFsList, "FS_LIST"},
		{"fs_grep", constants.ActionTypeFsGrep, "FS_GREP"},
		{"mcp_call", constants.ActionTypeMcpCall, "MCP_CALL"},
		{"a2a_call", constants.ActionTypeA2aCall, "A2A_CALL"},
		{"heartbeat", constants.ActionTypeHeartbeat, "HEARTBEAT"},
		{"shutdown", constants.ActionTypeShutdown, "SHUTDOWN"},
		{"port_check", constants.ActionTypePortCheck, "PORT_CHECK"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ActionType(tt.a); got != tt.want {
				t.Errorf("ActionType() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestEvent(t *testing.T) {
	tests := []struct {
		name string
		e    constants.EventType
		want string
	}{
		{"command_requested", constants.Event.Operator.Command.Requested, "g8e.v1.operator.command.requested"},
		{"heartbeat", constants.Event.Operator.Heartbeat, "g8e.v1.operator.heartbeat.sent"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Event(tt.e); got != tt.want {
				t.Errorf("Event() = %v, want %v", got, tt.want)
			}
		})
	}
}
