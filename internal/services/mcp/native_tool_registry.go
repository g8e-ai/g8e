// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package mcp

import (
	"fmt"

	"github.com/g8e-ai/g8e/internal/constants"
)

// RegisterNativeTools explicitly registers all native tools into the provided
// registry. This function replaces the previous init()-based auto-registration
// to comply with the prohibition on init() functions in service packages.
func RegisterNativeTools(registry *ToolRegistry) error {
	if registry == nil {
		return fmt.Errorf("native_tool_registry: register: %w", constants.ErrMCPRegistryNil)
	}
	tools := []NativeTool{
		&DBDiscoverTopologyTool{},
		&DBQueryValidateTool{},
		&DBIsolatedReadTool{},
		&DBIndexTriageTool{},
		&LogStreamFilterTool{},
		&SysOOMDetectTool{},
		&ConfigDiffMaskTool{},
		&ProcMetricTopTool{},
		&FSDiskProfileTool{},
		&ProcSignalSafeTool{},
		&NetSocketAuditTool{},
		&NetEndpointPingTool{},
		&NetHTTPProbeTool{},
		&SysInfoTool{},
		&NetDNSResolveTool{},
		&TLSCertInspectTool{},
		&SysEnvVarsTool{},
		&FSFileChecksumTool{},
		&SysServiceStatusTool{},
		&SysContainerStatusTool{},
		&FSDiskUsageTool{},
		&SysTimeClockTool{},
		&ProcTreeTool{},
		&GitOpsTool{},
		&CloudMetadataTool{},
		&K8sInspectTool{},
		&RunShellCommandTool{},
		&NetSSHKnownHostsTool{},
		&OperatorDeployTool{},
		&FileReadTool{},
		&AuditReceiptListTool{},
		&AuditReceiptGetTool{},
	}

	for _, tool := range tools {
		if err := registry.Register(tool); err != nil {
			return fmt.Errorf("native_tool_registry: register tool %q: %w", tool.Name(), err)
		}
	}

	return nil
}
