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

// Package marshaler provides clean conversion utilities for typed constants
// at protocol boundaries (JSON, Proto, database, environment variables).
//
// This package eliminates the "String Casting Bridge" pattern where
// string(constants.SomeType) was used throughout the codebase, providing
// type-safe conversion functions instead.
//
// Usage:
//
//	// Instead of: string(constants.CollectionUsers)
//	collection := marshaler.CollectionName(constants.CollectionUsers)
//
//	// Instead of: string(constants.EnvVar.LogLevel)
//	envKey := marshaler.EnvVar(constants.EnvVar.LogLevel)
//
//	// Instead of: string(constants.Status.OperatorStatus.Active)
//	status := marshaler.Status(constants.Status.OperatorStatus.Active)
package marshaler

import (
	"github.com/g8e-ai/g8e/services/g8eo/internal/constants"
)

// CollectionName converts a CollectionName constant to string for database operations.
func CollectionName(c constants.CollectionName) string {
	return string(c)
}

// EnvVar converts an EnvVarKey constant to string for environment variable access.
func EnvVar(e constants.EnvVarKey) string {
	return string(e)
}

// DocumentID converts a DocumentID constant to string for database lookups.
func DocumentID(d constants.DocumentID) string {
	return string(d)
}

// Status converts status-type constants to string for JSON/Proto serialization.
// This covers OperatorStatus, UserStatus, ExecutionStatus, etc.
func Status[T ~string](s T) string {
	return string(s)
}

// OperatorStatus converts OperatorStatus to string.
func OperatorStatus(s constants.OperatorStatus) string {
	return string(s)
}

// OperatorType converts OperatorType to string.
func OperatorType(t constants.OperatorType) string {
	return string(t)
}

// ExecutionStatus converts ExecutionStatus to string.
func ExecutionStatus(s constants.ExecutionStatus) string {
	return string(s)
}

// ActionType converts ActionType to string.
func ActionType(a constants.ActionType) string {
	return string(a)
}

// Event converts event-type constants to string for pub/sub and logging.
func Event(e constants.EventType) string {
	return string(e)
}
