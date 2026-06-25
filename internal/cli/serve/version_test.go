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

func TestVersionInfo_ZeroValue(t *testing.T) {
	var vi VersionInfo

	assert.Equal(t, "", vi.Version)
	assert.Equal(t, "", vi.BuildID)
	assert.Equal(t, "", vi.BuildTime)
	assert.Equal(t, "", vi.Platform)
}

func TestVersionInfo_FieldAssignment(t *testing.T) {
	vi := VersionInfo{
		Version:   "1.2.0",
		BuildID:   "abc123def456",
		BuildTime: "2026-06-25T04:56:00Z",
		Platform:  "linux/amd64",
	}

	assert.Equal(t, "1.2.0", vi.Version)
	assert.Equal(t, "abc123def456", vi.BuildID)
	assert.Equal(t, "2026-06-25T04:56:00Z", vi.BuildTime)
	assert.Equal(t, "linux/amd64", vi.Platform)
}

func TestVersionInfo_Equality(t *testing.T) {
	a := VersionInfo{Version: "1.0.0", BuildID: "b1", BuildTime: "t1", Platform: "linux/amd64"}
	b := VersionInfo{Version: "1.0.0", BuildID: "b1", BuildTime: "t1", Platform: "linux/amd64"}
	c := VersionInfo{Version: "1.0.0", BuildID: "b1", BuildTime: "t1", Platform: "darwin/arm64"}

	require.True(t, a == b, "structs with identical fields should be equal")
	require.False(t, a == c, "structs differing in any field should not be equal")
}

func TestVersionInfo_PartialAssignment(t *testing.T) {
	vi := VersionInfo{Version: "0.0.0-dev"}

	assert.Equal(t, "0.0.0-dev", vi.Version)
	assert.Equal(t, "", vi.BuildID, "unassigned BuildID should be zero value")
	assert.Equal(t, "", vi.BuildTime, "unassigned BuildTime should be zero value")
	assert.Equal(t, "", vi.Platform, "unassigned Platform should be zero value")
}

func TestVersionInfo_AllFieldsExported(t *testing.T) {
	vi := VersionInfo{}

	vi.Version = "v2.0.0"
	vi.BuildID = "build-xyz"
	vi.BuildTime = "2026-01-01T00:00:00Z"
	vi.Platform = "windows/amd64"

	assert.Equal(t, "v2.0.0", vi.Version)
	assert.Equal(t, "build-xyz", vi.BuildID)
	assert.Equal(t, "2026-01-01T00:00:00Z", vi.BuildTime)
	assert.Equal(t, "windows/amd64", vi.Platform)
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
