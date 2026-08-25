// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

//go:build integration

package gateway

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/marshaler"
	"github.com/g8e-ai/g8e/v2/internal/models"
	"github.com/g8e-ai/g8e/v2/internal/testutil"
)

func TestAppEnrollmentService_EnrollDelegatedApp(t *testing.T) {
	gateway, _ := setupTestGatewayService(t)
	docStore := gateway.GetDocStore()
	logger := gateway.logger
	pki := gateway.pki

	appEnrollment := NewAppEnrollmentService(docStore, pki, logger)

	csr := testutil.GenerateTestCSRP256(t, "goose")
	userID := "test-user-001"

	resp, err := appEnrollment.EnrollDelegatedApp(AppEnrollRequest{
		CSR:     csr,
		AppName: "goose",
	}, userID)
	require.NoError(t, err)
	require.True(t, resp.Success, "enrollment should succeed, error: %s", resp.Error)
	assert.NotEmpty(t, resp.AppCert)
	assert.NotEmpty(t, resp.AppID)
	assert.Equal(t, "spiffe://g8e.local/app/goose", resp.AppID)

	policyDoc, err := docStore.DocGet(marshaler.CollectionName(constants.CollectionAppPolicies), resp.AppID)
	require.NoError(t, err)
	require.NotNil(t, policyDoc, "AppPolicy must be persisted for delegated enrollment")

	policyBytes, err := json.Marshal(policyDoc.Data)
	require.NoError(t, err)
	var policy models.AppPolicy
	require.NoError(t, json.Unmarshal(policyBytes, &policy))
	assert.Equal(t, resp.AppID, policy.AppID)
	assert.False(t, policy.RequireL3Approval, "delegated enrollment must not require L3 approval")
	assert.Equal(t, 0, policy.RateLimitRPS, "delegated enrollment should have unlimited rate")
	assert.Equal(t, int64(0), policy.MaxPayloadBytes, "delegated enrollment should have no payload cap")
}

// TestAppEnrollmentService_EnrollDelegatedApp_RejectsReservedPlatformNames
// proves that the retained delegated app enrollment path rejects the
// canonical platform component names (g8ed, g8ee, g8eo). Those identities
// are issued only through the owner-approved platform enrollment protocol.
func TestAppEnrollmentService_EnrollDelegatedApp_RejectsReservedPlatformNames(t *testing.T) {
	gateway, _ := setupTestGatewayService(t)
	appEnrollment := NewAppEnrollmentService(gateway.GetDocStore(), gateway.pki, gateway.logger)

	reservedNames := []string{"g8ed", "g8ee", "g8eo"}
	for _, name := range reservedNames {
		t.Run(name, func(t *testing.T) {
			resp, err := appEnrollment.EnrollDelegatedApp(AppEnrollRequest{
				CSR:     testutil.GenerateTestCSRP256(t, name),
				AppName: name,
				AppType: "mcp-client",
			}, "test-user-001")
			require.NoError(t, err)
			assert.False(t, resp.Success, "reserved name %q must be rejected", name)
			assert.Contains(t, resp.Error, constants.ErrPlatformEnrollmentReservedIdentity.Error(),
				"reserved name %q must be rejected with ErrPlatformEnrollmentReservedIdentity", name)
			assert.Empty(t, resp.AppCert, "no certificate must be issued for reserved name %q", name)
		})
	}
}

func TestIsValidAppName(t *testing.T) {

	tests := []struct {
		name  string
		input string
		want  bool
	}{
		{
			name:  "valid alphanumeric",
			input: "testapp123",
			want:  true,
		},
		{
			name:  "valid with hyphens",
			input: "test-app-name",
			want:  true,
		},
		{
			name:  "valid with underscores",
			input: "test_app_name",
			want:  true,
		},
		{
			name:  "valid mixed",
			input: "Test-App_123",
			want:  true,
		},
		{
			name:  "empty string",
			input: "",
			want:  false,
		},
		{
			name:  "contains space",
			input: "test app",
			want:  false,
		},
		{
			name:  "contains special char",
			input: "test@app",
			want:  false,
		},
		{
			name:  "contains slash",
			input: "test/app",
			want:  false,
		},
		{
			name:  "contains dot",
			input: "test.app",
			want:  false,
		},
		{
			name:  "starts with hyphen",
			input: "-testapp",
			want:  true,
		},
		{
			name:  "ends with hyphen",
			input: "testapp-",
			want:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isValidAppName(tt.input)
			assert.Equal(t, tt.want, result)
		})
	}
}
