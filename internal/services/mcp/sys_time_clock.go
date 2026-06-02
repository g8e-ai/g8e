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
	ntpStatus := getNTPStatus()

	result := map[string]interface{}{
		"system_time": map[string]interface{}{
			"utc":         now.UTC().Format(time.RFC3339),
			"local":       now.Format(time.RFC3339),
			"unix":        now.Unix(),
			"unix_nano":   now.UnixNano(),
			"timezone":    now.Location().String(),
			"offset":      now.Format("-07:00"),
		},
		"ntp": ntpStatus,
	}

	resultJSON, err := json.Marshal(result)
	if err != nil {
		return CallToolResult{}, fmt.Errorf("failed to marshal result: %w", err)
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

func getNTPStatus() map[string]interface{} {
	ntpData := map[string]interface{}{
		"synced": false,
		"status": "unknown",
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

func parseTimedatectlOutput(output string) map[string]interface{} {
	result := map[string]interface{}{
		"synced": false,
		"status": "unknown",
	}

	lines := strings.Split(output, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.Contains(line, "System clock synchronized:") {
			if strings.Contains(line, "yes") {
				result["synced"] = true
				result["status"] = "synchronized"
			} else {
				result["synced"] = false
				result["status"] = "not synchronized"
			}
		}
		if strings.Contains(line, "NTP service:") {
			result["ntp_service"] = strings.TrimSpace(strings.Split(line, ":")[1])
		}
		if strings.Contains(line, "NTP synchronized:") {
			result["ntp_synchronized"] = strings.TrimSpace(strings.Split(line, ":")[1])
		}
	}

	return result
}

func parseChronycOutput(output string) map[string]interface{} {
	result := map[string]interface{}{
		"synced": false,
		"status": "unknown",
	}

	lines := strings.Split(output, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "Reference ID") {
			result["reference_id"] = strings.TrimSpace(strings.Split(line, ":")[1])
		}
		if strings.HasPrefix(line, "Stratum") {
			result["stratum"] = strings.TrimSpace(strings.Split(line, ":")[1])
		}
		if strings.HasPrefix(line, "System time") {
			result["system_time_offset"] = strings.TrimSpace(strings.Split(line, ":")[1])
			result["synced"] = true
			result["status"] = "synchronized"
		}
	}

	return result
}

func parseNtpqOutput(output string) map[string]interface{} {
	result := map[string]interface{}{
		"synced": false,
		"status": "unknown",
	}

	lines := strings.Split(output, "\n")
	if len(lines) > 2 {
		for _, line := range lines[2:] {
			fields := strings.Fields(line)
			if len(fields) > 3 {
				remote := fields[0]
				refid := fields[1]
				st := fields[2]
				when := fields[3]
				poll := fields[4]
				reach := fields[5]
				delay := fields[6]
				offset := fields[7]
				jitter := fields[8]

				if fields[0] == "*" {
					result["synced"] = true
					result["status"] = "synchronized"
					result["selected_peer"] = map[string]interface{}{
						"remote":  remote,
						"refid":   refid,
						"stratum": st,
						"when":    when,
						"poll":    poll,
						"reach":   reach,
						"delay":   delay,
						"offset":  offset,
						"jitter":  jitter,
					}
				}
			}
		}
	}

	return result
}
