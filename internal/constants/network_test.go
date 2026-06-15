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

package constants

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestDefaultEndpoint(t *testing.T) {
	t.Run("has correct value", func(t *testing.T) {
		assert.Equal(t, "localhost", DefaultEndpoint)
	})
}

func TestGatewayInternalHostname(t *testing.T) {
	t.Run("has correct value", func(t *testing.T) {
		assert.Equal(t, "g8e.local", GatewayInternalHostname)
	})
}

func TestLocalhostHTTPSURL(t *testing.T) {
	t.Run("constructs HTTPS URL with port 8443", func(t *testing.T) {
		result := LocalhostHTTPSURL(8443)
		assert.Equal(t, "https://localhost:8443", result)
	})

	t.Run("constructs HTTPS URL with port 443", func(t *testing.T) {
		result := LocalhostHTTPSURL(443)
		assert.Equal(t, "https://localhost:443", result)
	})

	t.Run("constructs HTTPS URL with port 8080", func(t *testing.T) {
		result := LocalhostHTTPSURL(8080)
		assert.Equal(t, "https://localhost:8080", result)
	})

	t.Run("constructs HTTPS URL with port 0", func(t *testing.T) {
		result := LocalhostHTTPSURL(0)
		assert.Equal(t, "https://localhost:0", result)
	})

	t.Run("constructs HTTPS URL with high port number", func(t *testing.T) {
		result := LocalhostHTTPSURL(65535)
		assert.Equal(t, "https://localhost:65535", result)
	})

	t.Run("always uses https scheme", func(t *testing.T) {
		result := LocalhostHTTPSURL(1234)
		assert.Contains(t, result, "https://", "URL should use https scheme")
		assert.NotContains(t, result, "http://", "URL should not use http scheme")
	})

	t.Run("always uses localhost hostname", func(t *testing.T) {
		result := LocalhostHTTPSURL(5678)
		assert.Contains(t, result, "localhost", "URL should use localhost hostname")
	})
}

func TestLocalhostHTTPURL(t *testing.T) {
	t.Run("constructs HTTP URL with port 8080", func(t *testing.T) {
		result := LocalhostHTTPURL(8080)
		assert.Equal(t, "http://localhost:8080", result)
	})

	t.Run("constructs HTTP URL with port 80", func(t *testing.T) {
		result := LocalhostHTTPURL(80)
		assert.Equal(t, "http://localhost:80", result)
	})

	t.Run("constructs HTTP URL with port 3000", func(t *testing.T) {
		result := LocalhostHTTPURL(3000)
		assert.Equal(t, "http://localhost:3000", result)
	})

	t.Run("constructs HTTP URL with port 0", func(t *testing.T) {
		result := LocalhostHTTPURL(0)
		assert.Equal(t, "http://localhost:0", result)
	})

	t.Run("constructs HTTP URL with high port number", func(t *testing.T) {
		result := LocalhostHTTPURL(65535)
		assert.Equal(t, "http://localhost:65535", result)
	})

	t.Run("always uses http scheme", func(t *testing.T) {
		result := LocalhostHTTPURL(1234)
		assert.Contains(t, result, "http://", "URL should use http scheme")
		assert.NotContains(t, result, "https://", "URL should not use https scheme")
	})

	t.Run("always uses localhost hostname", func(t *testing.T) {
		result := LocalhostHTTPURL(5678)
		assert.Contains(t, result, "localhost", "URL should use localhost hostname")
	})
}

func TestNetworkConstantsDistinct(t *testing.T) {
	t.Run("endpoint constants are distinct", func(t *testing.T) {
		assert.NotEqual(t, DefaultEndpoint, GatewayInternalHostname)
	})
}

func TestURLBuilderConsistency(t *testing.T) {
	t.Run("HTTPS and HTTP URLs differ for same port", func(t *testing.T) {
		port := 8443
		httpsURL := LocalhostHTTPSURL(port)
		httpURL := LocalhostHTTPURL(port)
		assert.NotEqual(t, httpsURL, httpURL)
		assert.Contains(t, httpsURL, "https://")
		assert.Contains(t, httpURL, "http://")
	})
}
