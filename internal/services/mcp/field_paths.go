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

package mcp

import "github.com/g8e-ai/g8e/internal/constants"

// getFieldPaths returns the field path registry for all collections.
// Canonical source: protocol/constants/field_paths.json (mirrored in internal/constants).
// Returns a deep copy to prevent mutation of canonical data.
func getFieldPaths() map[string]CollectionFieldPaths {
	canonical := map[string]CollectionFieldPaths{
		constants.FieldPathInvestigations: {
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
		constants.FieldPathMemories: {
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
		constants.FieldPathCases: {
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

	result := make(map[string]CollectionFieldPaths, len(canonical))
	for k, v := range canonical {
		result[k] = CollectionFieldPaths{
			AllowedPaths:   append([]string(nil), v.AllowedPaths...),
			ForbiddenPaths: append([]string(nil), v.ForbiddenPaths...),
		}
	}
	return result
}
