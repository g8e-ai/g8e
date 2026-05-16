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

package models

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
	Hostname     string      `json:"hostname"`
	OS           string      `json:"os"`
	Architecture string      `json:"architecture"`
	NumCPU       int         `json:"num_cpu"`
	GoVersion    string      `json:"go_version"`
	CurrentUser  string      `json:"current_user"`
	LoadAverage  []float64   `json:"load_average,omitempty"`
	Memory       *MemoryInfo `json:"memory,omitempty"`
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
	ComponentName string `json:"component_name"`
	ProjectID     string `json:"project_id"`
	MaxMemoryMB   int    `json:"max_memory_mb"`
}
