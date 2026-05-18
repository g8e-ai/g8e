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

	"github.com/g8e-ai/g8e/services/g8eo/internal/constants"
)

func TestCollectionName(t *testing.T) {
	tests := []struct {
		name string
		c    constants.CollectionName
		want string
	}{
		{"users", constants.CollectionUsers, "users"},
		{"operators", constants.CollectionOperators, "operators"},
		{"api_keys", constants.CollectionAPIKeys, "api_keys"},
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
		{"operator_api_key", constants.EnvVar.OperatorAPIKey, "G8E_OPERATOR_API_KEY"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := EnvVar(tt.e); got != tt.want {
				t.Errorf("EnvVar() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestOperatorStatus(t *testing.T) {
	// Test that the function compiles and returns a string
	result := OperatorStatus(constants.Status.OperatorStatus.Active)
	if result == "" {
		t.Error("OperatorStatus() returned empty string")
	}
}

func TestOperatorType(t *testing.T) {
	// Test that the function compiles and returns a string
	result := OperatorType(constants.Status.OperatorType.System)
	if result == "" {
		t.Error("OperatorType() returned empty string")
	}
}

func TestExecutionStatus(t *testing.T) {
	// Test that the function compiles and returns a string
	result := ExecutionStatus(constants.Status.ExecutionStatus.Pending)
	if result == "" {
		t.Error("ExecutionStatus() returned empty string")
	}
}

func TestEvent(t *testing.T) {
	tests := []struct {
		name string
		e    string
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
