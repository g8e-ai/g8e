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
	"fmt"
	"os/exec"
	"strings"
	"time"
)

// SysTimeClockTool provides NTP sync status and system time verification.
type SysTimeClockTool struct{}

// Name returns the tool identifier.
func (t *SysTimeClockTool) Name() string {
	return "sys_time_clock"
}

// Description returns a human-readable description.
func (t *SysTimeClockTool) Description() string {
	return "Provides NTP sync status and system time verification."
}

// InputSchema returns the JSON Schema for tool validation.
func (t *SysTimeClockTool) InputSchema() map[string]interface{} {
	return map[string]interface{}{
		"type":       "object",
		"properties": map[string]interface{}{},
	}
}

// Execute implements the tool logic.
func (t *SysTimeClockTool) Execute(ctx context.Context, args json.RawMessage) (CallToolResult, error) {
	now := time.Now()
	_, offset := now.Zone()

	result := SysTimeClockResult{
		SystemTime: SystemTimeInfo{
			UTC:       now.UTC().Format(time.RFC3339),
			Local:     now.Format(time.RFC3339),
			Unix:      now.Unix(),
			UnixNano:  now.UnixNano(),
			Timezone:  now.Location().String(),
			Offset:    formatOffset(offset),
		},
		NTP: getNTPStatus(),
	}

	resultJSON, err := json.Marshal(result)
	if err != nil {
		return CallToolResult{}, fmt.Errorf("sys_time_clock: failed to marshal result: %w", err)
	}

	return CallToolResult{
		Content: []TextContent{
			{
				Type: "text",
				Text: string(resultJSON),
			},
		},
	}, nil
}

// formatOffset converts seconds offset to ±HH:MM format.
func formatOffset(offset int) string {
	sign := "+"
	if offset < 0 {
		sign = "-"
		offset = -offset
	}
	hours := offset / 3600
	minutes := (offset % 3600) / 60
	return fmt.Sprintf("%s%02d:%02d", sign, hours, minutes)
}

func getNTPStatus() NTPStatus {
	ntpData := NTPStatus{
		Synced: false,
		Status: "unknown",
	}

	if _, err := exec.LookPath("timedatectl"); err == nil {
		cmd := exec.Command("timedatectl", "status", "--no-pager")
		output, err := cmd.CombinedOutput()
		if err == nil {
			ntpData = parseTimedatectlOutput(string(output))
		}
	} else if _, err := exec.LookPath("chronyc"); err == nil {
		cmd := exec.Command("chronyc", "tracking")
		output, err := cmd.CombinedOutput()
		if err == nil {
			ntpData = parseChronycOutput(string(output))
		}
	} else if _, err := exec.LookPath("ntpq"); err == nil {
		cmd := exec.Command("ntpq", "-p")
		output, err := cmd.CombinedOutput()
		if err == nil {
			ntpData = parseNtpqOutput(string(output))
		}
	}

	return ntpData
}

func parseTimedatectlOutput(output string) NTPStatus {
	result := NTPStatus{
		Synced: false,
		Status: "unknown",
	}

	lines := strings.Split(output, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.Contains(line, "System clock synchronized:") {
			if strings.Contains(line, "yes") {
				result.Synced = true
				result.Status = "synchronized"
			} else {
				result.Synced = false
				result.Status = "not synchronized"
			}
		}
		if strings.Contains(line, "NTP service:") {
			parts := strings.SplitN(line, ":", 2)
			if len(parts) == 2 {
				result.NTPService = strings.TrimSpace(parts[1])
			}
		}
		if strings.Contains(line, "NTP synchronized:") {
			parts := strings.SplitN(line, ":", 2)
			if len(parts) == 2 {
				result.NTPSynchronized = strings.TrimSpace(parts[1])
			}
		}
	}

	return result
}

func parseChronycOutput(output string) NTPStatus {
	result := NTPStatus{
		Synced: false,
		Status: "unknown",
	}

	lines := strings.Split(output, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "Reference ID") {
			parts := strings.SplitN(line, ":", 2)
			if len(parts) == 2 {
				result.ReferenceID = strings.TrimSpace(parts[1])
			}
		}
		if strings.HasPrefix(line, "Stratum") {
			parts := strings.SplitN(line, ":", 2)
			if len(parts) == 2 {
				result.Stratum = strings.TrimSpace(parts[1])
			}
		}
		if strings.HasPrefix(line, "System time") {
			parts := strings.SplitN(line, ":", 2)
			if len(parts) == 2 {
				result.SystemTimeOffset = strings.TrimSpace(parts[1])
				result.Synced = true
				result.Status = "synchronized"
			}
		}
	}

	return result
}

func parseNtpqOutput(output string) NTPStatus {
	result := NTPStatus{
		Synced: false,
		Status: "unknown",
	}

	lines := strings.Split(output, "\n")
	if len(lines) > 2 {
		for _, line := range lines[2:] {
			fields := strings.Fields(line)
			if len(fields) >= 9 {
				if fields[0] == "*" {
					result.Synced = true
					result.Status = "synchronized"
					result.SelectedPeer = &NTPSelectedPeer{
						Remote: fields[0],
						RefID:  fields[1],
						Stratum: fields[2],
						When:   fields[3],
						Poll:   fields[4],
						Reach:  fields[5],
						Delay:  fields[6],
						Offset: fields[7],
						Jitter: fields[8],
					}
				}
			}
		}
	}

	return result
}
