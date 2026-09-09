// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package browserorigin

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/v2/internal/constants"
)

func TestParse_HTTPS_HostedOrigin(t *testing.T) {
	o, err := Parse("https://your-app.lovable.app")
	require.NoError(t, err)
	assert.Equal(t, "https://your-app.lovable.app", o.URL)
	assert.Equal(t, "your-app.lovable.app", o.Hostname)
	assert.Equal(t, "", o.Port)
	assert.Equal(t, "your-app.lovable.app", o.RPID)
	assert.False(t, o.Loopback)
}

func TestParse_HTTPS_HostedOrigin_WithExplicitPort(t *testing.T) {
	o, err := Parse("https://your-app.lovable.app:8443")
	require.NoError(t, err)
	assert.Equal(t, "https://your-app.lovable.app:8443", o.URL)
	assert.Equal(t, "your-app.lovable.app", o.Hostname)
	assert.Equal(t, "8443", o.Port)
	assert.Equal(t, "your-app.lovable.app", o.RPID)
}

func TestParse_HTTPS_DefaultPortOmitted(t *testing.T) {
	o, err := Parse("https://your-app.lovable.app:443")
	require.NoError(t, err)
	assert.Equal(t, "https://your-app.lovable.app", o.URL, "default port 443 must be omitted")
	assert.Equal(t, "", o.Port)
}

func TestParse_HTTP_Loopback(t *testing.T) {
	o, err := Parse("http://localhost:3003")
	require.NoError(t, err)
	assert.Equal(t, "http://localhost:3003", o.URL)
	assert.Equal(t, "localhost", o.Hostname)
	assert.Equal(t, "3003", o.Port)
	assert.Equal(t, "localhost", o.RPID)
	assert.True(t, o.Loopback)
}

func TestParse_HTTP_127_IPLoopback(t *testing.T) {
	o, err := Parse("http://127.0.0.1:8080")
	require.NoError(t, err)
	assert.Equal(t, "http://127.0.0.1:8080", o.URL)
	assert.Equal(t, "localhost", o.RPID, "loopback IP must normalize to localhost RP ID")
	assert.True(t, o.Loopback)
}

func TestParse_HTTP_IPv6Loopback(t *testing.T) {
	o, err := Parse("http://[::1]:8080")
	require.NoError(t, err)
	assert.Equal(t, "localhost", o.RPID)
	assert.True(t, o.Loopback)
}

func TestParse_HTTP_DefaultPortOmitted(t *testing.T) {
	o, err := Parse("http://localhost:80")
	require.NoError(t, err)
	assert.Equal(t, "http://localhost", o.URL, "default port 80 must be omitted")
}

func TestParse_LowercaseCanonicalization(t *testing.T) {
	o, err := Parse("HTTPS://Your-App.Lovable.APP")
	require.NoError(t, err)
	assert.Equal(t, "https://your-app.lovable.app", o.URL)
	assert.Equal(t, "your-app.lovable.app", o.Hostname)
	assert.Equal(t, "your-app.lovable.app", o.RPID)
}

func TestParse_TrailingSlashNormalized(t *testing.T) {
	o, err := Parse("https://your-app.lovable.app/")
	require.NoError(t, err)
	assert.Equal(t, "https://your-app.lovable.app", o.URL, "trailing slash must be stripped")
}

func TestParse_RejectsPath(t *testing.T) {
	_, err := Parse("https://your-app.lovable.app/app")
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrValidationFailed)
}

func TestParse_RejectsQuery(t *testing.T) {
	_, err := Parse("https://your-app.lovable.app?q=1")
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrValidationFailed)
}

func TestParse_RejectsFragment(t *testing.T) {
	_, err := Parse("https://your-app.lovable.app#frag")
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrValidationFailed)
}

func TestParse_RejectsUserInfo(t *testing.T) {
	_, err := Parse("https://user:pass@your-app.lovable.app")
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrValidationFailed)
}

func TestParse_RejectsNonLoopbackHTTP(t *testing.T) {
	_, err := Parse("http://your-app.lovable.app")
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrValidationFailed)
	assert.Contains(t, err.Error(), "loopback")
}

func TestParse_RejectsNonLoopbackIP(t *testing.T) {
	_, err := Parse("https://192.168.1.1")
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrValidationFailed)
	assert.Contains(t, err.Error(), "IP-address")
}

func TestParse_RejectsUnsupportedScheme(t *testing.T) {
	_, err := Parse("ftp://your-app.lovable.app")
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrValidationFailed)
}

func TestParse_RejectsEmpty(t *testing.T) {
	_, err := Parse("")
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrValidationFailed)
}

func TestParse_RejectsWhitespaceOnly(t *testing.T) {
	_, err := Parse("   ")
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrValidationFailed)
}

func TestParse_RejectsNoHost(t *testing.T) {
	_, err := Parse("https://")
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrValidationFailed)
}

func TestParse_RPIDNeverContainsPort(t *testing.T) {
	o, err := Parse("https://your-app.lovable.app:8443")
	require.NoError(t, err)
	assert.NotContains(t, o.RPID, ":")
	assert.Equal(t, "your-app.lovable.app", o.RPID)
}

func TestValidateRPID_ExactMatch(t *testing.T) {
	o, err := Parse("https://your-app.lovable.app")
	require.NoError(t, err)
	assert.NoError(t, ValidateRPID(o, "your-app.lovable.app"))
}

func TestValidateRPID_ParentSuffix(t *testing.T) {
	o, err := Parse("https://api.example.com")
	require.NoError(t, err)
	assert.NoError(t, ValidateRPID(o, "example.com"))
}

func TestValidateRPID_Empty(t *testing.T) {
	o, err := Parse("https://your-app.lovable.app")
	require.NoError(t, err)
	err = ValidateRPID(o, "")
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrValidationFailed)
}

func TestValidateRPID_UnrelatedDomain(t *testing.T) {
	o, err := Parse("https://your-app.lovable.app")
	require.NoError(t, err)
	err = ValidateRPID(o, "other.com")
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrValidationFailed)
}

func TestValidateRPID_PublicSuffixICANN(t *testing.T) {
	o, err := Parse("https://your-app.example.com")
	require.NoError(t, err)
	err = ValidateRPID(o, "com")
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrValidationFailed)
	assert.Contains(t, err.Error(), "public suffix")
}

func TestValidateRPID_PublicSuffixMultiPart(t *testing.T) {
	o, err := Parse("https://your-app.co.uk")
	require.NoError(t, err)
	err = ValidateRPID(o, "co.uk")
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrValidationFailed)
	assert.Contains(t, err.Error(), "public suffix")
}

func TestValidateRPID_LoopbackExactMatch(t *testing.T) {
	o, err := Parse("http://localhost:3003")
	require.NoError(t, err)
	assert.NoError(t, ValidateRPID(o, "localhost"))
}

func TestValidateRPID_CaseInsensitive(t *testing.T) {
	o, err := Parse("https://your-app.lovable.app")
	require.NoError(t, err)
	assert.NoError(t, ValidateRPID(o, "YOUR-APP.LOVABLE.APP"))
}
