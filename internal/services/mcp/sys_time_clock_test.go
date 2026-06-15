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
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSysTimeClockTool_Name(t *testing.T) {
	tool := &SysTimeClockTool{}
	assert.Equal(t, "sys_time_clock", tool.Name())
}

func TestSysTimeClockTool_Description(t *testing.T) {
	tool := &SysTimeClockTool{}
	assert.NotEmpty(t, tool.Description())
	assert.Contains(t, tool.Description(), "time")
}

func TestSysTimeClockTool_InputSchema(t *testing.T) {
	tool := &SysTimeClockTool{}
	schema := tool.InputSchema()

	assert.Equal(t, "object", schema.Type)
	assert.NotNil(t, schema.Properties)
}

func TestFormatOffset(t *testing.T) {
	tests := []struct {
		offset   int
		expected string
	}{
		{0, "+00:00"},
		{3600, "+01:00"},
		{-3600, "-01:00"},
		{3660, "+01:01"},
		{-3660, "-01:01"},
		{3540, "+00:59"},
		{-3540, "-00:59"},
		{46800, "+13:00"},  // UTC+13
		{-39600, "-11:00"}, // UTC-11
	}

	for _, tt := range tests {
		assert.Equal(t, tt.expected, formatOffset(tt.offset))
	}
}

func TestParseTimedatectlOutput(t *testing.T) {
	tests := []struct {
		name     string
		output   string
		expected NTPStatus
	}{
		{
			name: "synchronized yes",
			output: `               Local time: Fri 2026-06-12 10:00:00 UTC
           Universal time: Fri 2026-06-12 10:00:00 UTC
                 RTC time: Fri 2026-06-12 10:00:00
                Time zone: UTC (UTC, +0000)
System clock synchronized: yes
              NTP service: active
          RTC in local TZ: no`,
			expected: NTPStatus{
				Synced:     true,
				Status:     "synchronized",
				NTPService: "active",
			},
		},
		{
			name: "synchronized no",
			output: `               Local time: Fri 2026-06-12 10:00:00 UTC
           Universal time: Fri 2026-06-12 10:00:00 UTC
                 RTC time: Fri 2026-06-12 10:00:00
                Time zone: UTC (UTC, +0000)
System clock synchronized: no
              NTP service: inactive
          RTC in local TZ: no`,
			expected: NTPStatus{
				Synced:     false,
				Status:     "not synchronized",
				NTPService: "inactive",
			},
		},
		{
			name: "NTP synchronized yes",
			output: `               Local time: Fri 2026-06-12 10:00:00 UTC
           Universal time: Fri 2026-06-12 10:00:00 UTC
                 RTC time: Fri 2026-06-12 10:00:00
                Time zone: UTC (UTC, +0000)
System clock synchronized: yes
              NTP service: active
         NTP synchronized: yes`,
			expected: NTPStatus{
				Synced:          true,
				Status:          "synchronized",
				NTPService:      "active",
				NTPSynchronized: "yes",
			},
		},
		{
			name:   "empty output",
			output: "",
			expected: NTPStatus{
				Synced: false,
				Status: "unknown",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseTimedatectlOutput(tt.output)
			assert.Equal(t, tt.expected, got)
		})
	}
}

func TestParseChronycOutput(t *testing.T) {
	tests := []struct {
		name     string
		output   string
		expected NTPStatus
	}{
		{
			name: "synchronized chrony",
			output: `Reference ID    : 10.0.0.1 (ntp.example.com)
Stratum         : 2
Ref time (UTC)  : Fri Jun 12 10:00:00 2026
System time     : 0.000001234 seconds fast of NTP time
Last offset     : +0.000000123 seconds
RMS offset      : 0.000000456 seconds
Frequency       : 1.234 ppm fast
Residual freq   : +0.000 ppm
Skew            : 0.001 ppm
Root delay      : 0.012345 seconds
Root dispersion : 0.000123 seconds
Update interval : 64.0 seconds
Leap status     : Normal`,
			expected: NTPStatus{
				Synced:           true,
				Status:           "synchronized",
				ReferenceID:      "10.0.0.1 (ntp.example.com)",
				Stratum:          "2",
				SystemTimeOffset: "0.000001234 seconds fast of NTP time",
			},
		},
		{
			name:   "empty output",
			output: "",
			expected: NTPStatus{
				Synced: false,
				Status: "unknown",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseChronycOutput(tt.output)
			assert.Equal(t, tt.expected, got)
		})
	}
}

func TestParseNtpqOutput(t *testing.T) {
	tests := []struct {
		name     string
		output   string
		expected NTPStatus
	}{
		{
			name: "synchronized ntpq",
			output: `     remote           refid      st t when poll reach   delay   offset  jitter
==============================================================================
*10.0.0.1        .GPS.            1 u   45   64  377    0.123    0.456   0.789
+10.0.0.2        10.0.0.1         2 u   12   64  377    1.234    0.567   0.890`,
			expected: NTPStatus{
				Synced: true,
				Status: "synchronized",
				SelectedPeer: &NTPSelectedPeer{
					Remote:  "*10.0.0.1",
					RefID:   ".GPS.",
					Stratum: "1",
					When:    "45",
					Poll:    "64",
					Reach:   "377",
					Delay:   "0.123",
					Offset:  "0.456",
					Jitter:  "0.789",
				},
			},
		},
		{
			name: "no synchronized peer",
			output: `     remote           refid      st t when poll reach   delay   offset  jitter
==============================================================================
 10.0.0.1        .GPS.            1 u   45   64    0    0.000    0.000   0.000`,
			expected: NTPStatus{
				Synced: false,
				Status: "unknown",
			},
		},
		{
			name: "malformed output",
			output: `     remote           refid      st t when poll reach   delay   offset  jitter
==============================================================================`,
			expected: NTPStatus{
				Synced: false,
				Status: "unknown",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseNtpqOutput(tt.output)
			assert.Equal(t, tt.expected, got)
		})
	}
}

func TestSysTimeClockTool_Execute(t *testing.T) {
	tool := &SysTimeClockTool{}
	ctx := context.Background()

	result, err := tool.Execute(ctx, json.RawMessage("{}"))
	require.NoError(t, err)
	require.Len(t, result.Content, 1)
	require.Equal(t, "text", result.Content[0].Type)

	var clockResult SysTimeClockResult
	err = json.Unmarshal([]byte(result.Content[0].Text), &clockResult)
	require.NoError(t, err)

	// Verify basic structure
	assert.NotEmpty(t, clockResult.SystemTime.UTC)
	assert.NotEmpty(t, clockResult.SystemTime.Local)
	assert.NotZero(t, clockResult.SystemTime.Unix)
	assert.NotEmpty(t, clockResult.SystemTime.Timezone)
	assert.NotEmpty(t, clockResult.SystemTime.Offset)

	// Verify time formatting
	_, err = time.Parse(time.RFC3339, clockResult.SystemTime.UTC)
	assert.NoError(t, err)
	_, err = time.Parse(time.RFC3339, clockResult.SystemTime.Local)
	assert.NoError(t, err)

	// NTP status should at least have a default state
	assert.NotEmpty(t, clockResult.NTP.Status)
}
