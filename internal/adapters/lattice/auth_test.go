// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package lattice

import (
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newInspectOAuthServer returns a server that records request headers for inspection.
type oauthRequestInspector struct {
	sandboxHeader string
	authHeader    string
	contentType   string
	body          string
	requestCount  atomic.Int32
}

func TestRequireTransportSecurity_ReturnsTrue(t *testing.T) {
	t.Parallel()

	auth := NewClientCredentialsAuth("id", "secret", "", "https://example.com")
	assert.True(t, auth.RequireTransportSecurity())
}

func TestNewClientCredentialsAuth_SetsHttpClientTimeout(t *testing.T) {
	t.Parallel()

	auth := NewClientCredentialsAuth("id", "secret", "", "https://example.com")
	require.NotNil(t, auth.httpClient)
	assert.Equal(t, 10*time.Second, auth.httpClient.Timeout)
}
