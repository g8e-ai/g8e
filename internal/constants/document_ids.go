// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

// Package constants provides Go registry files generated from protocol/constants/ JSON.
//
// This file contains document ID constants used throughout the platform for
// identifying canonical documents in the governance system. These constants
// are generated from protocol/constants/document_ids.json (SSOT).
//
// Adding new document IDs:
// 1. Add to protocol/constants/document_ids.json
// 2. Run `make constants` to regenerate this file
// 3. Run `go run ./internal/constants/check_registry.go` to verify
package constants

// DocumentID is a typed string for canonical document IDs.
type DocumentID string

const (
	DocIDPlatformSettings   DocumentID = "platform_settings"
	DocIDUserSettingsPrefix DocumentID = "user_settings_"
)
