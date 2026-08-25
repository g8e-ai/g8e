// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package governance

import (
	"testing"

	"github.com/stretchr/testify/assert"

	operatorv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/operator/v1"
)

func TestL1Doctrine_ValidatePayload_McpCallRequested(t *testing.T) {
	doctrine := NewL1Doctrine()

	tests := []struct {
		name       string
		toolName   string
		shouldFail bool
	}{
		{"safe_tool", "pseudo", false},
		{"safe_tool_2", "issue_tracker", false},
		{"safe_substring_sudo", "pseudo_sudo_tool", false},
		{"safe_substring_su", "summary", false},
		{"blocked_sudo", "sudo", true},
		{"blocked_SUDO_case_insensitive", "SUDO", true},
		{"blocked_su", "su", true},
		{"blocked_rm_rf_root", "rm-rf-root", true},
		{"blocked_rm_rf_root_spaces", "rm rf root", true},
		{"safe_substring_rm_rf", "format_rm_rf_root_tool", false},
		{"blocked_drop_database", "drop_database", true},
		{"safe_substring_drop_database", "my_drop_database_test", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			payload := &operatorv1.McpCallRequested{
				ToolName:      tt.toolName,
				ArgumentsJson: "{}",
			}
			violations := doctrine.ValidatePayload(payload)
			if tt.shouldFail {
				assert.NotEmpty(t, violations, "Expected violations for tool_name: %s", tt.toolName)
			} else {
				assert.Empty(t, violations, "Expected NO violations for tool_name: %s", tt.toolName)
			}
		})
	}
}

func TestL1Doctrine_ValidatePayload_A2ACallRequested(t *testing.T) {
	doctrine := NewL1Doctrine()

	tests := []struct {
		name       string
		skillName  string
		shouldFail bool
	}{
		{"safe_skill", "pseudo", false},
		{"safe_substring_sudo", "pseudo_sudo_skill", false},
		{"safe_substring_su", "summary", false},
		{"blocked_sudo", "sudo", true},
		{"blocked_SUDO_case_insensitive", "SUDO", true},
		{"blocked_su", "su", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			payload := &operatorv1.A2ACallRequested{
				SkillName:   tt.skillName,
				PayloadJson: "{}",
			}
			violations := doctrine.ValidatePayload(payload)
			if tt.shouldFail {
				assert.NotEmpty(t, violations, "Expected violations for skill_name: %s", tt.skillName)
			} else {
				assert.Empty(t, violations, "Expected NO violations for skill_name: %s", tt.skillName)
			}
		})
	}
}
