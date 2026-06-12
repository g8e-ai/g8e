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

import "fmt"

// RegisterNativeTools explicitly registers all native tools into the provided
// registry. This function replaces the previous init()-based auto-registration
// to comply with the prohibition on init() functions in service packages.
func RegisterNativeTools(registry *ToolRegistry) error {
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
	}

	for _, tool := range tools {
		if err := registry.Register(tool); err != nil {
			return fmt.Errorf("register native tool %q: %w", tool.Name(), err)
		}
	}

	return nil
}
