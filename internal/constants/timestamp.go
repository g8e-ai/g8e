// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

// Package constants provides timestamp-related constants.
//
// Single source of truth: internal/timesvc (the Go mirror of protocol/constants/timestamp.json).
// This file re-exports the canonical format strings so protocol constants remain usable.
package constants

import "github.com/g8e-ai/g8e/internal/timesvc"

// FormatRFC3339 is the canonical timestamp format string for RFC3339 with timezone offset.
//
// Source: internal/timesvc
const FormatRFC3339 = timesvc.FormatRFC3339

// TimestampFormat is the canonical timestamp format for RFC3339 with fixed microsecond precision.
// Used throughout the platform for consistent timestamp representation.
// The fixed 6-digit fractional seconds guarantee lexicographic ordering for SQLite indices.
//
// Source: internal/timesvc
const TimestampFormat = timesvc.Format
