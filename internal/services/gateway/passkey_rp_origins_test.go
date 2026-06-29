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

package gateway

import (
	"strconv"
	"testing"

	"github.com/g8e-ai/g8e/internal/constants"
	"github.com/stretchr/testify/assert"
)

func TestBuildRPOrigins_LocalhostDefaults(t *testing.T) {
	cfg := &PasskeyConfig{RpID: "localhost"}

	origins := buildRPOrigins(cfg)

	assert.Contains(t, origins, "localhost")
	assert.Contains(t, origins, "http://localhost")
	assert.Contains(t, origins, "http://localhost:8080")
	assert.Contains(t, origins, "http://127.0.0.1")
	assert.Contains(t, origins, "http://127.0.0.1:8080")
	assert.Contains(t, origins, "https://localhost")
	assert.Contains(t, origins, "https://localhost:8443")
	assert.Contains(t, origins, "https://127.0.0.1")
	assert.Contains(t, origins, "https://127.0.0.1:8443")
}

func TestBuildRPOrigins_LocalhostWithCustomPorts(t *testing.T) {
	cfg := &PasskeyConfig{
		RpID:      "localhost",
		HTTPPort:  8087,
		HTTPSPort: 8450,
	}

	origins := buildRPOrigins(cfg)

	assert.Contains(t, origins, "http://localhost:8087")
	assert.Contains(t, origins, "https://localhost:8450")
	assert.NotContains(t, origins, "http://localhost:8080")
	assert.NotContains(t, origins, "https://localhost:8443")
}

func TestBuildRPOrigins_LocalhostWithInjectedOrigins(t *testing.T) {
	cfg := &PasskeyConfig{
		RpID:      "localhost",
		HTTPPort:  8080,
		HTTPSPort: 8443,
		RpOrigins: []string{"http://localhost:8087", "https://localhost:8450"},
	}

	origins := buildRPOrigins(cfg)

	assert.Contains(t, origins, "http://localhost:8087", "injected origin should be present")
	assert.Contains(t, origins, "https://localhost:8450", "injected origin should be present")
	assert.Contains(t, origins, "http://localhost:8080", "default origin should be preserved")
	assert.Contains(t, origins, "https://localhost:8443", "default origin should be preserved")
}

func TestBuildRPOrigins_CustomRpID(t *testing.T) {
	cfg := &PasskeyConfig{RpID: "operator.example.com"}

	origins := buildRPOrigins(cfg)

	assert.Equal(t, []string{"https://operator.example.com"}, origins)
}

func TestBuildRPOrigins_CustomRpIDWithInjectedOrigins(t *testing.T) {
	cfg := &PasskeyConfig{
		RpID:      "operator.example.com",
		RpOrigins: []string{"http://localhost:8082", "https://localhost:8445"},
	}

	origins := buildRPOrigins(cfg)

	assert.Contains(t, origins, "https://operator.example.com")
	assert.Contains(t, origins, "http://localhost:8082", "injected origin should survive the else branch")
	assert.Contains(t, origins, "https://localhost:8445", "injected origin should survive the else branch")
}

func TestBuildRPOrigins_EmptyRpOriginsBackCompat(t *testing.T) {
	cfg := &PasskeyConfig{RpID: "localhost"}

	origins := buildRPOrigins(cfg)

	assert.Len(t, origins, 9)
	assert.NotContains(t, origins, "")
}

func TestBuildRPOrigins_127001Defaults(t *testing.T) {
	cfg := &PasskeyConfig{RpID: "127.0.0.1"}

	origins := buildRPOrigins(cfg)

	assert.Contains(t, origins, "127.0.0.1")
	assert.Contains(t, origins, "http://localhost")
	assert.Contains(t, origins, "https://127.0.0.1:8443")
}

func TestBuildRPOrigins_DefaultPortsUseConstants(t *testing.T) {
	cfg := &PasskeyConfig{RpID: "localhost"}

	origins := buildRPOrigins(cfg)

	assert.Contains(t, origins, "http://localhost:"+strconv.Itoa(constants.Ports.OperatorHttp))
	assert.Contains(t, origins, "https://localhost:"+strconv.Itoa(constants.Ports.OperatorHttps))
}
