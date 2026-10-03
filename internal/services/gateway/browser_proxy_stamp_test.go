// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package gateway

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/v2/internal/config"
	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/models"
	"github.com/g8e-ai/g8e/v2/internal/response"
)

type browserProxyStampVector struct {
	KeyID             string `json:"key_id"`
	PrivateSeedHex    string `json:"private_seed_hex"`
	PublicKeyHex      string `json:"public_key_hex"`
	BodyUTF8          string `json:"body_utf8"`
	CanonicalBytesHex string `json:"canonical_bytes_hex"`
	SignatureHex      string `json:"signature_hex"`
	Stamp             struct {
		Method         string `json:"method"`
		RequestURI     string `json:"request_uri"`
		BodySHA256     string `json:"body_sha256"`
		UserID         string `json:"user_id"`
		UserEmail      string `json:"user_email"`
		WebSessionID   string `json:"web_session_id"`
		OrganizationID string `json:"organization_id"`
		CLISessionID   string `json:"cli_session_id"`
		IssuedAtUnix   int64  `json:"issued_at_unix"`
		Nonce          string `json:"nonce"`
	} `json:"stamp"`
}

func loadBrowserProxyStampVector(t *testing.T) browserProxyStampVector {
	t.Helper()
	raw, err := os.ReadFile("../../../protocol/conformance/browser_proxy_stamp_vectors.json")
	require.NoError(t, err)
	var v browserProxyStampVector
	require.NoError(t, json.Unmarshal(raw, &v))
	return v
}

func (v browserProxyStampVector) stamp() BrowserProxyStamp {
	return BrowserProxyStamp{
		Method:         v.Stamp.Method,
		RequestURI:     v.Stamp.RequestURI,
		BodySHA256:     v.Stamp.BodySHA256,
		UserID:         v.Stamp.UserID,
		UserEmail:      v.Stamp.UserEmail,
		WebSessionID:   v.Stamp.WebSessionID,
		OrganizationID: v.Stamp.OrganizationID,
		CLISessionID:   v.Stamp.CLISessionID,
		IssuedAtUnix:   v.Stamp.IssuedAtUnix,
		Nonce:          v.Stamp.Nonce,
	}
}

func TestBrowserProxyStamp_MatchesSharedConformanceVector(t *testing.T) {
	v := loadBrowserProxyStampVector(t)
	assert.Equal(t, hex.EncodeToString(sha256Sum([]byte(v.BodyUTF8))), v.Stamp.BodySHA256)

	canonical, err := v.stamp().CanonicalBytes()
	require.NoError(t, err)
	assert.Equal(t, v.CanonicalBytesHex, hex.EncodeToString(canonical))

	seed, err := hex.DecodeString(v.PrivateSeedHex)
	require.NoError(t, err)
	signer, err := NewBrowserProxySigner(ed25519.NewKeyFromSeed(seed), v.KeyID)
	require.NoError(t, err)
	sig, err := signer.sign(v.stamp())
	require.NoError(t, err)
	assert.Equal(t, v.SignatureHex, sig)
	assert.Equal(t, v.PublicKeyHex, hex.EncodeToString(signer.PublicKey()))
}

func TestBrowserProxyStamp_RejectsFieldsThatCouldForgeAnotherStamp(t *testing.T) {
	for _, field := range []string{"Method", "RequestURI", "UserID", "UserEmail", "WebSessionID", "OrganizationID", "CLISessionID", "Nonce"} {
		t.Run(field, func(t *testing.T) {
			stamp := loadBrowserProxyStampVector(t).stamp()
			switch field {
			case "Method":
				stamp.Method = "POST\nuser-2"
			case "RequestURI":
				stamp.RequestURI = "/a\n/b"
			case "UserID":
				stamp.UserID = "user-1\nweb-2"
			case "UserEmail":
				stamp.UserEmail = "a\rb"
			case "WebSessionID":
				stamp.WebSessionID = "web\n1"
			case "OrganizationID":
				stamp.OrganizationID = "org\n1"
			case "CLISessionID":
				stamp.CLISessionID = "cli\n1"
			case "Nonce":
				stamp.Nonce = "n\nn"
			}
			_, err := stamp.CanonicalBytes()
			require.ErrorIs(t, err, constants.ErrBrowserProxyStampFieldInvalid)
		})
	}
}

func TestNewBrowserProxySigner_FailsClosed(t *testing.T) {
	_, priv, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)

	_, err = NewBrowserProxySigner(nil, "key-1")
	require.ErrorIs(t, err, constants.ErrBrowserProxySignerUnavailable)
	_, err = NewBrowserProxySigner(ed25519.PrivateKey{1, 2, 3}, "key-1")
	require.ErrorIs(t, err, constants.ErrBrowserProxySignerUnavailable)
	_, err = NewBrowserProxySigner(priv, "")
	require.ErrorIs(t, err, constants.ErrBrowserProxySignerUnavailable)
}

func TestBrowserProxySigner_ApplySignsTheRequestActuallySent(t *testing.T) {
	seed := bytes.Repeat([]byte{7}, ed25519.SeedSize)
	priv := ed25519.NewKeyFromSeed(seed)
	signer, err := NewBrowserProxySigner(priv, "key-1")
	require.NoError(t, err)
	now := time.Unix(1790000000, 0)
	signer.now = func() time.Time { return now }

	body := []byte(`{"message":"hi"}`)
	req := httptest.NewRequest(http.MethodPost, "http://ensemble.invalid/api/v1/chat/send?stream=1", bytes.NewReader(body))
	require.NoError(t, signer.Apply(req, body, BrowserProxyIdentity{UserID: "user-1", UserEmail: "user-1@g8e.local", WebSessionID: "web-1"}))

	assert.Equal(t, "key-1", req.Header.Get(constants.HeaderProxyKeyID))
	assert.Equal(t, strconv.FormatInt(now.Unix(), 10), req.Header.Get(constants.HeaderProxyIssuedAt))
	nonce := req.Header.Get(constants.HeaderProxyNonce)
	assert.Len(t, nonce, 32)
	assert.Equal(t, "user-1", req.Header.Get(constants.HeaderProxyUserID))
	assert.Equal(t, "web-1", req.Header.Get(constants.HeaderProxyWebSessionID))

	stamp := BrowserProxyStamp{
		Method: http.MethodPost, RequestURI: "/api/v1/chat/send?stream=1",
		BodySHA256: hex.EncodeToString(sha256Sum(body)), UserID: "user-1", UserEmail: "user-1@g8e.local",
		WebSessionID: "web-1", IssuedAtUnix: now.Unix(), Nonce: nonce,
	}
	canonical, err := stamp.CanonicalBytes()
	require.NoError(t, err)
	sig, err := hex.DecodeString(req.Header.Get(constants.HeaderProxySignature))
	require.NoError(t, err)
	assert.True(t, ed25519.Verify(priv.Public().(ed25519.PublicKey), canonical, sig))

	other := httptest.NewRequest(http.MethodPost, "http://ensemble.invalid/api/v1/chat/send", bytes.NewReader(body))
	require.NoError(t, signer.Apply(other, body, BrowserProxyIdentity{UserID: "user-1", UserEmail: "user-1@g8e.local", WebSessionID: "web-1"}))
	assert.NotEqual(t, nonce, other.Header.Get(constants.HeaderProxyNonce), "every request needs a fresh nonce")
}

func TestBrowserProxySigner_ApplyDoesNotForwardCallerSuppliedIdentityHeaders(t *testing.T) {
	_, priv, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)
	signer, err := NewBrowserProxySigner(priv, "key-1")
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodGet, "http://ensemble.invalid/api/v1/cases", nil)
	req.Header.Set(constants.HeaderProxyOrganizationID, "forged-org")
	req.Header.Set("X-Proxy-CLI-Session-Id", "forged-cli")
	require.NoError(t, signer.Apply(req, nil, BrowserProxyIdentity{UserID: "user-1", UserEmail: "user-1@g8e.local", WebSessionID: "web-1"}))

	assert.Empty(t, req.Header.Get(constants.HeaderProxyOrganizationID))
	assert.Empty(t, req.Header.Get("X-Proxy-CLI-Session-Id"))
}

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

func TestEnsembleBrowserProxyController_ProxySigningKeyEndpointServesOnlyThePublicKey(t *testing.T) {
	c := newTestBrowserProxyController(t, "http://127.0.0.1:1")
	rec := httptest.NewRecorder()
	c.handleProxySigningKey(rec, httptest.NewRequest(http.MethodGet, constants.APIPaths.GatewayProxySigningKey, nil))

	require.Equal(t, http.StatusOK, rec.Code)
	var out models.ActuatorPublicKeyExport
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &out))
	assert.Equal(t, "test-key", out.KeyID)
	assert.Equal(t, "ed25519", out.Algorithm)
	assert.Equal(t, hex.EncodeToString(c.signer.PublicKey()), out.PublicKey)
	assert.NotContains(t, rec.Body.String(), "private")

	rec = httptest.NewRecorder()
	c.handleProxySigningKey(rec, httptest.NewRequest(http.MethodPost, constants.APIPaths.GatewayProxySigningKey, nil))
	assert.Equal(t, http.StatusMethodNotAllowed, rec.Code)

	c.signer = nil
	rec = httptest.NewRecorder()
	c.handleProxySigningKey(rec, httptest.NewRequest(http.MethodGet, constants.APIPaths.GatewayProxySigningKey, nil))
	assert.Equal(t, http.StatusServiceUnavailable, rec.Code)
}

func sha256Sum(b []byte) []byte {
	sum := sha256.Sum256(b)
	return sum[:]
}

func newTestBrowserProxyController(t *testing.T, upstreamURL string) *EnsembleBrowserProxyController {
	t.Helper()
	_, priv, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)
	signer, err := NewBrowserProxySigner(priv, "test-key")
	require.NoError(t, err)
	cfg := &config.Config{}
	cfg.Gateway.EnsembleUpstreamURL = upstreamURL
	return newEnsembleBrowserProxyController(EnsembleBrowserProxyControllerDeps{
		Cfg:       cfg,
		Logger:    slog.Default(),
		Responder: response.NewWriter(slog.Default()),
		Operators: fakeOperatorLister{},
		Signer:    signer,
	})
}

func withBrowserIdentity(ctx context.Context, userID, webSessionID string) context.Context {
	ctx = context.WithValue(ctx, constants.ContextKeyUserID, userID)
	return context.WithValue(ctx, constants.ContextKeyWebSessionID, webSessionID)
}
