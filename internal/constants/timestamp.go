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
// Single source of truth: protocol/constants/timestamp.json
// This file is manually maintained to match the JSON SSOT.
package constants

import "time"

// FormatRFC3339 is the canonical timestamp format string for RFC3339 with timezone offset.
// Used throughout the platform for consistent timestamp representation.
//
// Source: protocol/constants/timestamp.json
const FormatRFC3339 = "2006-01-02T15:04:05Z07:00"

// TimestampFormat is the canonical timestamp format for RFC3339 with nanosecond precision.
// Used throughout the platform for consistent timestamp representation.
const TimestampFormat = time.RFC3339Nano
