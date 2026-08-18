// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

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
//	// Instead of: string(constants.OperatorStatusActive)
//	status := marshaler.Status(constants.OperatorStatusActive)
package marshaler

import (
	"github.com/g8e-ai/g8e/internal/constants"
)

// CollectionName converts a CollectionName constant to string for database operations.
func CollectionName(c constants.CollectionName) string {
	return string(c)
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
