// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package certs

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFetchTrustBundle_UnreachableURL(t *testing.T) {
	pem, err := FetchTrustBundle(context.Background(), "https://127.0.0.1:19999/.well-known/g8e/pki/ca-bundle", "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to fetch CA certificate")
	assert.Nil(t, pem)
}

func TestFetchTrustBundle_InvalidURL(t *testing.T) {
	pem, err := FetchTrustBundle(context.Background(), "://invalid-url", "")
	require.Error(t, err)
	assert.Nil(t, pem)
}

// TestFetchTrustBundleWithClient_NilClientReturnsError confirms a nil
// client is rejected rather than panicking.
func TestFetchTrustBundleWithClient_NilClientReturnsError(t *testing.T) {
	_, err := FetchTrustBundleWithClient(context.Background(), "https://127.0.0.1:1/x", "", nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "nil client")
}
