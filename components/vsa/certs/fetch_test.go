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

package certs

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFetchAndSetCA_Success(t *testing.T) {
	saveAndRestoreCA(t)
	SetCA(nil)

	caBytes := generateTestCAPEM(t)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write(caBytes) //nolint:errcheck
	}))
	defer srv.Close()

	err := FetchAndSetCA(context.Background(), srv.URL+"/ssl/ca.crt")
	require.NoError(t, err)
	assert.Equal(t, caBytes, GetRawCA())
}

func TestFetchAndSetCA_Non200Status(t *testing.T) {
	saveAndRestoreCA(t)
	SetCA(nil)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	err := FetchAndSetCA(context.Background(), srv.URL+"/ssl/ca.crt")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "404")
	assert.Nil(t, GetRawCA())
}

func TestFetchAndSetCA_EmptyBody(t *testing.T) {
	saveAndRestoreCA(t)
	SetCA(nil)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	err := FetchAndSetCA(context.Background(), srv.URL+"/ssl/ca.crt")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "is empty")
	assert.Nil(t, GetRawCA())
}

func TestFetchAndSetCA_InvalidPEM(t *testing.T) {
	saveAndRestoreCA(t)
	SetCA(nil)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("this is not a valid PEM certificate")) //nolint:errcheck
	}))
	defer srv.Close()

	err := FetchAndSetCA(context.Background(), srv.URL+"/ssl/ca.crt")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not a valid PEM-encoded certificate")
	assert.Nil(t, GetRawCA())
}

func TestFetchAndSetCA_UnreachableURL(t *testing.T) {
	saveAndRestoreCA(t)
	SetCA(nil)

	err := FetchAndSetCA(context.Background(), "https://127.0.0.1:19999/ssl/ca.crt")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to fetch CA certificate")
	assert.Nil(t, GetRawCA())
}

func TestFetchAndSetCA_ContextCancelled(t *testing.T) {
	saveAndRestoreCA(t)
	SetCA(nil)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write(generateTestCAPEM(t)) //nolint:errcheck
	}))
	defer srv.Close()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := FetchAndSetCA(ctx, srv.URL+"/ssl/ca.crt")
	require.Error(t, err)
}

func TestFetchAndSetCA_InvalidURL(t *testing.T) {
	saveAndRestoreCA(t)
	SetCA(nil)

	err := FetchAndSetCA(context.Background(), "://invalid-url")
	require.Error(t, err)
}
