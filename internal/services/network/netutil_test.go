// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

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
