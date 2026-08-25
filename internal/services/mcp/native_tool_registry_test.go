// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package mcp

import (
	"errors"
	"strings"
	"testing"

	"github.com/g8e-ai/g8e/v2/internal/constants"
)

func TestRegisterNativeTools_Success(t *testing.T) {
	registry := NewToolRegistry()
	err := RegisterNativeTools(registry)
	if err != nil {
		t.Fatalf("RegisterNativeTools failed: %v", err)
	}

	// Verify all expected tools are registered
	expectedTools := []string{
		"db_discover_topology",
		"db_query_validate",
		"db_isolated_read",
		"db_index_triage",
		"log_stream_filter",
		"sys_oom_detect",
		"config_diff_mask",
		"proc_metric_top",
		"fs_disk_profile",
		"proc_signal_safe",
		"net_socket_audit",
		"net_endpoint_ping",
		"net_http_probe",
		"sys_info",
		"net_dns_resolve",
		"tls_cert_inspect",
		"sys_env_vars",
		"fs_file_checksum",
		"sys_service_status",
		"sys_container_status",
		"fs_disk_usage",
		"sys_time_clock",
		"proc_tree",
		"git_ops",
		"cloud_metadata",
		"k8s_inspect",
		"run_shell_command",
		"net_ssh_known_hosts",
		"operator_deploy",
		"read_file",
		"audit_receipt_list",
		"audit_receipt_get",
	}

	count := registry.Count()
	if count != len(expectedTools) {
		t.Errorf("Expected %d tools, got %d", len(expectedTools), count)
	}

	for _, name := range expectedTools {
		if _, ok := registry.Get(name); !ok {
			t.Errorf("Tool %q not found in registry", name)
		}
	}
}

func TestRegisterNativeTools_DuplicateError(t *testing.T) {
	registry := NewToolRegistry()

	// Register once
	if err := RegisterNativeTools(registry); err != nil {
		t.Fatalf("First registration failed: %v", err)
	}

	// Register again - should fail
	err := RegisterNativeTools(registry)
	if err == nil {
		t.Fatal("Expected error on duplicate registration, got nil")
	}

	if !strings.Contains(err.Error(), "already registered") {
		t.Errorf("Expected duplicate registration error, got: %v", err)
	}
}

func TestRegisterNativeTools_NilRegistry(t *testing.T) {
	err := RegisterNativeTools(nil)
	if err == nil {
		t.Fatal("Expected error when registry is nil, got nil")
	}

	if !errors.Is(err, constants.ErrMCPRegistryNil) {
		t.Errorf("Expected error wrapping %v, got %q", constants.ErrMCPRegistryNil, err.Error())
	}
}

func TestNativeTools_InterfaceCompliance(t *testing.T) {
	registry := NewToolRegistry()
	_ = RegisterNativeTools(registry)

	tools := registry.List()
	for _, tool := range tools {
		if tool.Name() == "" {
			t.Errorf("Tool %T has empty name", tool)
		}
		if tool.Description() == "" {
			t.Errorf("Tool %q has empty description", tool.Name())
		}
		if tool.InputSchema() == nil {
			t.Errorf("Tool %q has nil input schema", tool.Name())
		}
		// Basic validation of schema
		if err := validateInputSchema(tool.InputSchema()); err != nil {
			t.Errorf("Tool %q has invalid input schema: %v", tool.Name(), err)
		}
	}
}
