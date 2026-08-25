// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package mcp

import "github.com/g8e-ai/g8e/v2/internal/constants"

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
