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

package constants

// Field path constants for collection field access control.
// Canonical source: protocol/constants/field_paths.json

// FieldPathInvestigations defines allowed and forbidden field paths for the investigations collection
const FieldPathInvestigations = "investigations"

// FieldPathMemories defines allowed and forbidden field paths for the memories collection
const FieldPathMemories = "memories"

// FieldPathCases defines allowed and forbidden field paths for the cases collection
const FieldPathCases = "cases"

// GetFieldPaths returns the field path registry for all collections.
// This is the canonical in-memory representation of protocol/constants/field_paths.json.
func GetFieldPaths() map[string]FieldPathConfig {
	return map[string]FieldPathConfig{
		FieldPathInvestigations: {
			AllowedPaths: []string{
				"suspect_ip_addresses",
				"suspect_hostnames",
				"suspect_domains",
				"malware_hashes",
				"ioc_sources",
				"attack_patterns",
				"timeline_events",
				"evidence_summary",
				"status",
				"priority",
				"assigned_analyst",
				"created_at",
				"updated_at",
				"metadata",
			},
			ForbiddenPaths: []string{
				"credentials",
				"api_keys",
				"passwords",
				"tokens",
				"private_keys",
				"secrets",
			},
		},
		FieldPathMemories: {
			AllowedPaths: []string{
				"content",
				"summary",
				"tags",
				"source",
				"context",
				"created_at",
				"updated_at",
			},
			ForbiddenPaths: []string{
				"credentials",
				"api_keys",
				"passwords",
				"tokens",
				"private_keys",
				"secrets",
			},
		},
		FieldPathCases: {
			AllowedPaths: []string{
				"title",
				"description",
				"status",
				"priority",
				"assigned_to",
				"created_at",
				"updated_at",
				"resolution_summary",
			},
			ForbiddenPaths: []string{
				"credentials",
				"api_keys",
				"passwords",
				"tokens",
				"private_keys",
				"secrets",
			},
		},
	}
}

// FieldPathConfig defines allowed and forbidden paths for a collection
type FieldPathConfig struct {
	AllowedPaths   []string
	ForbiddenPaths []string
}
