// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

//go:build integration

package gateway

import (
	"bytes"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/v2/internal/constants"
)

func TestEnsembleBrowserProxyController_ForwardsSignedStampAndNoStaticMarker(t *testing.T) {
	var got http.Header
	var gotBody []byte
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r.Header.Clone()
		buf := new(bytes.Buffer)
		_, _ = buf.ReadFrom(r.Body)
		gotBody = buf.Bytes()
		w.WriteHeader(http.StatusNoContent)
	}))
	defer upstream.Close()

	c := newTestBrowserProxyController(t, upstream.URL)
	req := httptest.NewRequest(http.MethodPost, constants.APIPaths.EnsembleChatPrefix+"/send", strings.NewReader(`{"message":"hi"}`))
	req = req.WithContext(withBrowserIdentity(req.Context(), "user-1", "web-1"))
	rec := httptest.NewRecorder()
	c.handleProxy(rec, req)

	require.Equal(t, http.StatusNoContent, rec.Code)
	assert.NotEmpty(t, got.Get(constants.HeaderProxySignature))
	assert.NotEmpty(t, got.Get(constants.HeaderProxyKeyID))
	assert.Empty(t, got.Get("X-G8E-Gateway-Browser-Proxy"), "the static marker is removed")
	assert.Equal(t, "user-1", got.Get(constants.HeaderProxyUserID))

	sum := sha256.Sum256(gotBody)
	sig, err := hex.DecodeString(got.Get(constants.HeaderProxySignature))
	require.NoError(t, err)
	issued, err := strconv.ParseInt(got.Get(constants.HeaderProxyIssuedAt), 10, 64)
	require.NoError(t, err)
	canonical, err := BrowserProxyStamp{
		Method: http.MethodPost, RequestURI: constants.APIPaths.EnsembleChatPrefix + "/send",
		BodySHA256: hex.EncodeToString(sum[:]), UserID: "user-1", UserEmail: "user-1@g8e.local",
		WebSessionID: "web-1", IssuedAtUnix: issued, Nonce: got.Get(constants.HeaderProxyNonce),
	}.CanonicalBytes()
	require.NoError(t, err)
	assert.True(t, ed25519.Verify(c.signer.PublicKey(), canonical, sig), "signature must cover the body g8ee actually receives")
}

func TestEnsembleBrowserProxyController_NoSignerFailsClosed(t *testing.T) {
	called := false
	upstream := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { called = true }))
	defer upstream.Close()

	c := newTestBrowserProxyController(t, upstream.URL)
	c.signer = nil
	req := httptest.NewRequest(http.MethodGet, constants.APIPaths.EnsembleCasesPrefix, nil)
	req = req.WithContext(withBrowserIdentity(req.Context(), "user-1", "web-1"))
	rec := httptest.NewRecorder()
	c.handleProxy(rec, req)

	assert.Equal(t, http.StatusServiceUnavailable, rec.Code)
	assert.False(t, called, "an unsigned request must never reach g8ee")
}
