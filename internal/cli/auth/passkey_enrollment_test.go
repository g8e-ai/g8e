//go:build integration

package auth

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/internal/constants"
)

func TestGenerateEnrollmentToken_Success(t *testing.T) {
	fileSvc, cfg := newAuthTestEnv(t)
	writeTestCLICert(t, fileSvc, cfg)

	handler := func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPost, r.Method)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(map[string]string{"token": "abc123"})
	}
	startTLSEnrollServer(t, cfg, handler)

	r := newPasskeyRegistrar(fileSvc, cfg, PasskeyRegistrarOptions{})
	mtlsClient, err := BuildMTLSClient(fileSvc, cfg, 0)
	require.NoError(t, err)

	token, err := r.generateEnrollmentToken(context.Background(), mtlsClient, "test-session")
	require.NoError(t, err)
	assert.Equal(t, "abc123", token)
}

func TestGenerateEnrollmentToken_NonCreatedStatusReturnsError(t *testing.T) {
	fileSvc, cfg := newAuthTestEnv(t)
	writeTestCLICert(t, fileSvc, cfg)

	handler := func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}
	startTLSEnrollServer(t, cfg, handler)

	r := newPasskeyRegistrar(fileSvc, cfg, PasskeyRegistrarOptions{})
	mtlsClient, err := BuildMTLSClient(fileSvc, cfg, 0)
	require.NoError(t, err)

	_, err = r.generateEnrollmentToken(context.Background(), mtlsClient, "test-session")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "enrollment token generation failed")
}

func TestGenerateEnrollmentToken_EmptyTokenReturnsErrEnrollmentTokenGenerationFailed(t *testing.T) {
	fileSvc, cfg := newAuthTestEnv(t)
	writeTestCLICert(t, fileSvc, cfg)

	handler := func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(map[string]string{"token": ""})
	}
	startTLSEnrollServer(t, cfg, handler)

	r := newPasskeyRegistrar(fileSvc, cfg, PasskeyRegistrarOptions{})
	mtlsClient, err := BuildMTLSClient(fileSvc, cfg, 0)
	require.NoError(t, err)

	_, err = r.generateEnrollmentToken(context.Background(), mtlsClient, "test-session")
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrEnrollmentTokenGenerationFailed)
}

func TestGenerateEnrollmentToken_InvalidJSONReturnsErrInvalidJSONResponse(t *testing.T) {
	fileSvc, cfg := newAuthTestEnv(t)
	writeTestCLICert(t, fileSvc, cfg)

	handler := func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte("not json"))
	}
	startTLSEnrollServer(t, cfg, handler)

	r := newPasskeyRegistrar(fileSvc, cfg, PasskeyRegistrarOptions{})
	mtlsClient, err := BuildMTLSClient(fileSvc, cfg, 0)
	require.NoError(t, err)

	_, err = r.generateEnrollmentToken(context.Background(), mtlsClient, "test-session")
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrInvalidJSONResponse)
}

func TestVerifyPasskeyRegistration_HasCredentials(t *testing.T) {
	fileSvc, cfg := newAuthTestEnv(t)
	writeTestCLICert(t, fileSvc, cfg)

	handler := func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"success":true,"credentials":[{"id":"abc"}]}`))
	}
	startTLSEnrollServer(t, cfg, handler)

	hasPasskey, err := VerifyPasskeyRegistration(context.Background(), fileSvc, cfg, "test-session")
	require.NoError(t, err)
	assert.True(t, hasPasskey)
}

func TestVerifyPasskeyRegistration_NoCredentials(t *testing.T) {
	fileSvc, cfg := newAuthTestEnv(t)
	writeTestCLICert(t, fileSvc, cfg)

	handler := func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"success":true,"credentials":[]}`))
	}
	startTLSEnrollServer(t, cfg, handler)

	hasPasskey, err := VerifyPasskeyRegistration(context.Background(), fileSvc, cfg, "test-session")
	require.NoError(t, err)
	assert.False(t, hasPasskey)
}

func TestVerifyPasskeyRegistration_UnauthorizedReturnsErrPasskeyStatusUnauthorized(t *testing.T) {
	fileSvc, cfg := newAuthTestEnv(t)
	writeTestCLICert(t, fileSvc, cfg)

	handler := func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}
	startTLSEnrollServer(t, cfg, handler)

	hasPasskey, err := VerifyPasskeyRegistration(context.Background(), fileSvc, cfg, "test-session")
	require.Error(t, err)
	assert.False(t, hasPasskey)
	assert.ErrorIs(t, err, constants.ErrPasskeyStatusUnauthorized)
}

func TestVerifyPasskeyRegistration_ServerErrorReturnsErrHTTPStatusError(t *testing.T) {
	fileSvc, cfg := newAuthTestEnv(t)
	writeTestCLICert(t, fileSvc, cfg)

	handler := func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}
	startTLSEnrollServer(t, cfg, handler)

	hasPasskey, err := VerifyPasskeyRegistration(context.Background(), fileSvc, cfg, "test-session")
	require.Error(t, err)
	assert.False(t, hasPasskey)
	assert.ErrorIs(t, err, constants.ErrHTTPStatusError)
}

func TestVerifyPasskeyRegistration_InvalidJSONReturnsErrInvalidJSONResponse(t *testing.T) {
	fileSvc, cfg := newAuthTestEnv(t)
	writeTestCLICert(t, fileSvc, cfg)

	handler := func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("garbage"))
	}
	startTLSEnrollServer(t, cfg, handler)

	hasPasskey, err := VerifyPasskeyRegistration(context.Background(), fileSvc, cfg, "test-session")
	require.Error(t, err)
	assert.False(t, hasPasskey)
	assert.ErrorIs(t, err, constants.ErrInvalidJSONResponse)
}
