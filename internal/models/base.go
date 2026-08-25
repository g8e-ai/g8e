// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package models

import (
	"github.com/g8e-ai/g8e/v2/internal/constants"
)

// Removing aliases

type TerminalOutput struct {
	Command             string   `json:"command"`
	CommandWithArgs     string   `json:"command_with_args"`
	CombinedOutput      string   `json:"combined_output"`
	LastLines           []string `json:"last_lines"`
	TruncatedStdout     bool     `json:"truncated_stdout"`
	TruncatedStderr     bool     `json:"truncated_stderr"`
	OriginalStdoutLines int      `json:"original_stdout_lines"`
	OriginalStderrLines int      `json:"original_stderr_lines"`
	TotalOriginalLines  int      `json:"total_original_lines"`
}

type ExecutionSystemInfo struct {
	Hostname     string             `json:"hostname"`
	OS           constants.Platform `json:"os"`
	Architecture string             `json:"architecture"`
	NumCPU       int                `json:"num_cpu"`
	GoVersion    string             `json:"go_version"`
	CurrentUser  string             `json:"current_user"`
	LoadAverage  []float64          `json:"load_average,omitempty"`
	Memory       *MemoryInfo        `json:"memory,omitempty"`
}

type MemoryInfo struct {
	MemTotal     int64 `json:"MemTotal"`
	MemFree      int64 `json:"MemFree"`
	MemAvailable int64 `json:"MemAvailable"`
	Buffers      int64 `json:"Buffers"`
	Cached       int64 `json:"Cached"`
	SwapTotal    int64 `json:"SwapTotal"`
	SwapFree     int64 `json:"SwapFree"`
}

type ExecutionEnvironmentInfo struct {
	ComponentName constants.ComponentName `json:"component_name"`
	ProjectID     string                  `json:"project_id"`
	MaxMemoryMB   int                     `json:"max_memory_mb"`
}
