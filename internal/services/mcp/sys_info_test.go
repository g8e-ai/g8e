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

import (
	"context"
	"encoding/json"
	"os"
	"runtime"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestSysInfoTool_Execute_Success(t *testing.T) {
	tool := &SysInfoTool{}
	ctx := context.Background()

	// Empty arguments should work
	result, err := tool.Execute(ctx, json.RawMessage("{}"))
	require.NoError(t, err)
	require.Len(t, result.Content, 1)
	require.Equal(t, "text", result.Content[0].Type)

	var sysInfo SysInfoResult
	err = json.Unmarshal([]byte(result.Content[0].Text), &sysInfo)
	require.NoError(t, err)

	// Verify hostname
	expectedHostname, _ := os.Hostname()
	require.Equal(t, expectedHostname, sysInfo.Hostname)

	// Verify OS info
	require.Equal(t, runtime.GOOS, sysInfo.OS.OS)
	require.Equal(t, runtime.GOARCH, sysInfo.OS.Arch)

	// On Linux, we expect kernel version and OS version to be populated if possible,
	// but "unknown" is also a valid value if files are missing.
	require.NotEmpty(t, sysInfo.OS.Kernel)
	require.NotEmpty(t, sysInfo.OS.OSVersion)
	require.NotEmpty(t, sysInfo.OS.Uptime)
	require.NotEmpty(t, sysInfo.OS.LoadAverage)
}

func TestSysInfoTool_Execute_ContextCancelled(t *testing.T) {
	tool := &SysInfoTool{}
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	_, err := tool.Execute(ctx, json.RawMessage("{}"))
	require.Error(t, err)
	require.Equal(t, context.Canceled, err)
}

func TestSysInfoTool_Execute_InvalidJSON(t *testing.T) {
	tool := &SysInfoTool{}
	ctx := context.Background()

	_, err := tool.Execute(ctx, json.RawMessage("{invalid}"))
	require.Error(t, err)
	require.Contains(t, err.Error(), "unmarshal arguments")
}

func TestGetKernelVersion_Parsing(t *testing.T) {
	// Since getKernelVersion reads /proc/version, we can't easily mock it without refactoring.
	// But we can check that it returns something.
	ctx := context.Background()
	version := getKernelVersion(ctx)
	require.NotEmpty(t, version)
}

func TestGetOSVersion_Parsing(t *testing.T) {
	ctx := context.Background()
	version := getOSVersion(ctx)
	require.NotEmpty(t, version)
}

func TestGetUptime_Parsing(t *testing.T) {
	ctx := context.Background()
	uptime := getUptime(ctx)
	require.NotEmpty(t, uptime)
}

func TestGetLoadAverage_Parsing(t *testing.T) {
	ctx := context.Background()
	load := getLoadAverage(ctx)
	require.NotEmpty(t, load)
}

func TestSysInfoTool_Execute_Timeout(t *testing.T) {
	tool := &SysInfoTool{}
	// Use a deadline in the past so the context is already expired
	// deterministically, without relying on timer resolution or sleep.
	ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(-1*time.Second))
	defer cancel()

	_, err := tool.Execute(ctx, json.RawMessage("{}"))
	require.Error(t, err)
	require.Equal(t, context.DeadlineExceeded, err)
}
