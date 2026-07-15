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

package network

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestLocalhostHTTPSURL(t *testing.T) {
	tests := []struct {
		name string
		port int
		want string
	}{
		{name: "default HTTPS port", port: 8443, want: "https://localhost:8443"},
		{name: "custom port", port: 9000, want: "https://localhost:9000"},
		{name: "port 80", port: 80, want: "https://localhost:80"},
		{name: "high port", port: 65535, want: "https://localhost:65535"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, LocalhostHTTPSURL(tt.port))
		})
	}
}

func TestLocalhostHTTPURL(t *testing.T) {
	tests := []struct {
		name string
		port int
		want string
	}{
		{name: "default HTTP port", port: 8080, want: "http://localhost:8080"},
		{name: "custom port", port: 3000, want: "http://localhost:3000"},
		{name: "port 80", port: 80, want: "http://localhost:80"},
		{name: "high port", port: 65535, want: "http://localhost:65535"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, LocalhostHTTPURL(tt.port))
		})
	}
}
