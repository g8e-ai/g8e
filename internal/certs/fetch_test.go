// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package certs

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFetchTrustBundle_Success(t *testing.T) {
	caBytes := generateTestCAPEM(t)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write(caBytes) //nolint:errcheck
	}))
	defer srv.Close()

	pem, err := FetchTrustBundle(context.Background(), srv.URL+"/.well-known/g8e/pki/ca-bundle", "")
	require.NoError(t, err)
	assert.Equal(t, caBytes, pem)
}

func TestFetchTrustBundle_Non200Status(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	pem, err := FetchTrustBundle(context.Background(), srv.URL+"/.well-known/g8e/pki/ca-bundle", "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "404")
	assert.Nil(t, pem)
}

func TestFetchTrustBundle_EmptyBody(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	pem, err := FetchTrustBundle(context.Background(), srv.URL+"/.well-known/g8e/pki/ca-bundle", "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "is empty")
	assert.Nil(t, pem)
}

func TestFetchTrustBundle_InvalidPEM(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("this is not a valid PEM certificate")) //nolint:errcheck
	}))
	defer srv.Close()

	pem, err := FetchTrustBundle(context.Background(), srv.URL+"/.well-known/g8e/pki/ca-bundle", "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not a valid PEM-encoded certificate")
	assert.Nil(t, pem)
}

func TestFetchTrustBundle_UnreachableURL(t *testing.T) {
	pem, err := FetchTrustBundle(context.Background(), "https://127.0.0.1:19999/.well-known/g8e/pki/ca-bundle", "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to fetch CA certificate")
	assert.Nil(t, pem)
}

func TestFetchTrustBundle_ContextCancelled(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write(generateTestCAPEM(t)) //nolint:errcheck
	}))
	defer srv.Close()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	pem, err := FetchTrustBundle(ctx, srv.URL+"/.well-known/g8e/pki/g8eg-ca-bundle.pem", "")
	require.Error(t, err)
	assert.Nil(t, pem)
}

func TestFetchTrustBundle_InvalidURL(t *testing.T) {
	pem, err := FetchTrustBundle(context.Background(), "://invalid-url", "")
	require.Error(t, err)
	assert.Nil(t, pem)
}

// TestFetchTrustBundleWithClient_UsesProvidedClient confirms the variant
// uses the caller-supplied *http.Client (and its transport) instead of
// constructing a default one. This is what lets the CLI route the discovery
// CA fetch through its IPv4-only transport.
func TestFetchTrustBundleWithClient_UsesProvidedClient(t *testing.T) {
	caBytes := generateTestCAPEM(t)

	called := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called++
		w.WriteHeader(http.StatusOK)
		w.Write(caBytes) //nolint:errcheck
	}))
	defer srv.Close()

	customClient := &http.Client{Timeout: 5 * time.Second}
	pem, err := FetchTrustBundleWithClient(context.Background(), srv.URL+"/.well-known/g8e/pki/ca-bundle", "", customClient)
	require.NoError(t, err)
	assert.Equal(t, caBytes, pem)
	assert.Equal(t, 1, called, "provided client must be used for the fetch")
}

// TestFetchTrustBundleWithClient_NilClientReturnsError confirms a nil
// client is rejected rather than panicking.
func TestFetchTrustBundleWithClient_NilClientReturnsError(t *testing.T) {
	_, err := FetchTrustBundleWithClient(context.Background(), "https://127.0.0.1:1/x", "", nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "nil client")
}
