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
