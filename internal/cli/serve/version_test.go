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

package serve

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestVersionInfo_Fields(t *testing.T) {
	tests := []struct {
		name      string
		version   string
		buildID   string
		buildTime string
		platform  string
	}{
		{
			name: "zero value",
		},
		{
			name:      "full assignment",
			version:   "1.2.0",
			buildID:   "abc123def456",
			buildTime: "2026-06-25T04:56:00Z",
			platform:  "linux/amd64",
		},
		{
			name:    "partial assignment",
			version: "0.0.0-dev",
		},
		{
			name:      "all fields individually settable",
			version:   "v2.0.0",
			buildID:   "build-xyz",
			buildTime: "2026-01-01T00:00:00Z",
			platform:  "windows/amd64",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			vi := VersionInfo{
				Version:   tt.version,
				BuildID:   tt.buildID,
				BuildTime: tt.buildTime,
				Platform:  tt.platform,
			}

			assert.Equal(t, tt.version, vi.Version)
			assert.Equal(t, tt.buildID, vi.BuildID)
			assert.Equal(t, tt.buildTime, vi.BuildTime)
			assert.Equal(t, tt.platform, vi.Platform)
		})
	}
}

func TestVersionInfo_Equality(t *testing.T) {
	a := VersionInfo{Version: "1.0.0", BuildID: "b1", BuildTime: "t1", Platform: "linux/amd64"}
	b := VersionInfo{Version: "1.0.0", BuildID: "b1", BuildTime: "t1", Platform: "linux/amd64"}
	c := VersionInfo{Version: "1.0.0", BuildID: "b1", BuildTime: "t1", Platform: "darwin/arm64"}

	require.True(t, a == b, "structs with identical fields should be equal")
	require.False(t, a == c, "structs differing in any field should not be equal")
}

func TestVersionInfo_Mutation(t *testing.T) {
	vi := VersionInfo{Version: "1.0.0", BuildID: "old", BuildTime: "old-time", Platform: "linux/amd64"}

	vi.Version = "2.0.0"
	vi.BuildID = "new"

	assert.Equal(t, "2.0.0", vi.Version)
	assert.Equal(t, "new", vi.BuildID)
	assert.Equal(t, "old-time", vi.BuildTime, "unmodified fields should retain original values")
	assert.Equal(t, "linux/amd64", vi.Platform)
}
