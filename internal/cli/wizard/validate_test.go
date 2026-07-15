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

package wizard

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- validatePublicBaseURL ---

func TestValidatePublicBaseURL_ValidHTTPS(t *testing.T) {
	assert.NoError(t, validatePublicBaseURL("https://demo.g8e.ai"))
}

func TestValidatePublicBaseURL_HttpLocalhost(t *testing.T) {
	assert.NoError(t, validatePublicBaseURL("http://localhost:8080"))
}

func TestValidatePublicBaseURL_Http127(t *testing.T) {
	assert.NoError(t, validatePublicBaseURL("http://127.0.0.1:8080"))
}

func TestValidatePublicBaseURL_HttpIPv6Loopback(t *testing.T) {
	assert.NoError(t, validatePublicBaseURL("http://[::1]:8080"))
}

func TestValidatePublicBaseURL_RejectsEmpty(t *testing.T) {
	assert.Error(t, validatePublicBaseURL(""))
}

func TestValidatePublicBaseURL_RejectsNonURL(t *testing.T) {
	assert.Error(t, validatePublicBaseURL("not-a-url"))
}

func TestValidatePublicBaseURL_RejectsNonLoopbackHTTP(t *testing.T) {
	assert.Error(t, validatePublicBaseURL("http://demo.g8e.ai"))
}

func TestValidatePublicBaseURL_RejectsQuery(t *testing.T) {
	assert.Error(t, validatePublicBaseURL("https://demo.g8e.ai?foo=bar"))
}

func TestValidatePublicBaseURL_RejectsFragment(t *testing.T) {
	assert.Error(t, validatePublicBaseURL("https://demo.g8e.ai#section"))
}

func TestValidatePublicBaseURL_RejectsUserInfo(t *testing.T) {
	assert.Error(t, validatePublicBaseURL("https://user@demo.g8e.ai"))
}

func TestValidatePublicBaseURL_RejectsUnsupportedScheme(t *testing.T) {
	assert.Error(t, validatePublicBaseURL("ftp://demo.g8e.ai"))
}

// --- validateTribunalURL ---

func TestValidateTribunalURL_EmptyIsOptional(t *testing.T) {
	assert.NoError(t, validateTribunalURL(""))
}

func TestValidateTribunalURL_ValidHTTPS(t *testing.T) {
	assert.NoError(t, validateTribunalURL("https://tribunal.g8e.ai"))
}

func TestValidateTribunalURL_RejectsHTTP(t *testing.T) {
	assert.Error(t, validateTribunalURL("http://tribunal.g8e.ai"))
}

func TestValidateTribunalURL_RejectsFragment(t *testing.T) {
	assert.Error(t, validateTribunalURL("https://tribunal.g8e.ai#frag"))
}

func TestValidateTribunalURL_RejectsUserInfo(t *testing.T) {
	assert.Error(t, validateTribunalURL("https://user@tribunal.g8e.ai"))
}

func TestValidateTribunalURL_RejectsUnsupportedScheme(t *testing.T) {
	assert.Error(t, validateTribunalURL("ftp://tribunal.g8e.ai"))
}

// --- validateTribunalID ---

func TestValidateTribunalID_EmptyRejected(t *testing.T) {
	assert.Error(t, validateTribunalID(""))
}

func TestValidateTribunalID_ValidAlphaNumeric(t *testing.T) {
	assert.NoError(t, validateTribunalID("trib-prod-01"))
}

func TestValidateTribunalID_ValidUnderscore(t *testing.T) {
	assert.NoError(t, validateTribunalID("trib_prod_01"))
}

func TestValidateTribunalID_RejectsSpecialChars(t *testing.T) {
	assert.Error(t, validateTribunalID("trib!prod"))
}

func TestValidateTribunalID_RejectsSpaces(t *testing.T) {
	assert.Error(t, validateTribunalID("trib prod"))
}

func TestValidateTribunalID_RejectsDots(t *testing.T) {
	assert.Error(t, validateTribunalID("trib.prod"))
}

// --- validateTribunalBootstrap ---

func TestValidateTribunalBootstrap_MatchingID(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bootstrap.json")
	require.NoError(t, os.WriteFile(path, []byte(`{"tribunal_id":"trib-001","members":[]}`), 0644))
	assert.NoError(t, validateTribunalBootstrap(path, "trib-001"))
}

func TestValidateTribunalBootstrap_MismatchedID(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bootstrap.json")
	require.NoError(t, os.WriteFile(path, []byte(`{"tribunal_id":"trib-001","members":[]}`), 0644))
	assert.Error(t, validateTribunalBootstrap(path, "trib-999"))
}

func TestValidateTribunalBootstrap_NotJSON(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bootstrap.json")
	require.NoError(t, os.WriteFile(path, []byte("not json"), 0644))
	assert.Error(t, validateTribunalBootstrap(path, "trib-001"))
}

func TestValidateTribunalBootstrap_MissingFile(t *testing.T) {
	assert.Error(t, validateTribunalBootstrap("/nonexistent/path.json", "trib-001"))
}

func TestValidateTribunalBootstrap_Directory(t *testing.T) {
	dir := t.TempDir()
	assert.Error(t, validateTribunalBootstrap(dir, "trib-001"))
}

func TestValidateTribunalBootstrap_MissingTribunalIDField(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bootstrap.json")
	require.NoError(t, os.WriteFile(path, []byte(`{"members":[]}`), 0644))
	assert.Error(t, validateTribunalBootstrap(path, "trib-001"))
}

// --- validatePasskeyRP ---

func TestValidatePasskeyRP_ExactMatch(t *testing.T) {
	assert.NoError(t, validatePasskeyRP("demo.g8e.ai", "https://demo.g8e.ai"))
}

func TestValidatePasskeyRP_SuffixMatch(t *testing.T) {
	assert.NoError(t, validatePasskeyRP("g8e.ai", "https://api.g8e.ai"))
}

func TestValidatePasskeyRP_Mismatch(t *testing.T) {
	assert.Error(t, validatePasskeyRP("other.com", "https://demo.g8e.ai"))
}

func TestValidatePasskeyRP_EmptyRpID(t *testing.T) {
	assert.Error(t, validatePasskeyRP("", "https://demo.g8e.ai"))
}

func TestValidatePasskeyRP_InvalidOrigin(t *testing.T) {
	assert.Error(t, validatePasskeyRP("demo.g8e.ai", "not-a-url"))
}

func TestValidatePasskeyRP_HttpOriginAllowed(t *testing.T) {
	assert.NoError(t, validatePasskeyRP("localhost", "http://localhost:8443"))
}

func TestValidatePasskeyRP_UnsupportedScheme(t *testing.T) {
	assert.Error(t, validatePasskeyRP("demo.g8e.ai", "ftp://demo.g8e.ai"))
}

// --- validateDownstreamURL ---

func TestValidateDownstreamURL_EmptyRejected(t *testing.T) {
	assert.Error(t, validateDownstreamURL(""))
}

func TestValidateDownstreamURL_ValidHTTP(t *testing.T) {
	assert.NoError(t, validateDownstreamURL("http://mcp:3000"))
}

func TestValidateDownstreamURL_ValidHTTPS(t *testing.T) {
	assert.NoError(t, validateDownstreamURL("https://mcp.example.com"))
}

func TestValidateDownstreamURL_InvalidURL(t *testing.T) {
	assert.Error(t, validateDownstreamURL("not-a-url"))
}

func TestValidateDownstreamURL_RejectsFragment(t *testing.T) {
	assert.Error(t, validateDownstreamURL("https://mcp.example.com#frag"))
}

func TestValidateDownstreamURL_RejectsCredentials(t *testing.T) {
	assert.Error(t, validateDownstreamURL("https://user:pass@mcp.example.com"))
}

func TestValidateDownstreamURL_RejectsUnsupportedScheme(t *testing.T) {
	assert.Error(t, validateDownstreamURL("ftp://mcp.example.com"))
}

// --- validateCORSOrigin ---

func TestValidateCORSOrigin_ValidHTTPS(t *testing.T) {
	assert.NoError(t, validateCORSOrigin("https://console.g8e.ai"))
}

func TestValidateCORSOrigin_ValidHTTP(t *testing.T) {
	assert.NoError(t, validateCORSOrigin("http://localhost:8080"))
}

func TestValidateCORSOrigin_RejectsEmpty(t *testing.T) {
	assert.Error(t, validateCORSOrigin(""))
}

func TestValidateCORSOrigin_RejectsPath(t *testing.T) {
	assert.Error(t, validateCORSOrigin("https://console.g8e.ai/path"))
}

func TestValidateCORSOrigin_RejectsQuery(t *testing.T) {
	assert.Error(t, validateCORSOrigin("https://console.g8e.ai?q=1"))
}

func TestValidateCORSOrigin_RejectsFragment(t *testing.T) {
	assert.Error(t, validateCORSOrigin("https://console.g8e.ai#frag"))
}

func TestValidateCORSOrigin_RejectsUserInfo(t *testing.T) {
	assert.Error(t, validateCORSOrigin("https://user@console.g8e.ai"))
}

func TestValidateCORSOrigin_RejectsUnsupportedScheme(t *testing.T) {
	assert.Error(t, validateCORSOrigin("ftp://console.g8e.ai"))
}

func TestValidateCORSOrigin_AllowsRootPath(t *testing.T) {
	assert.NoError(t, validateCORSOrigin("https://console.g8e.ai/"))
}

// --- isLoopbackHost ---

func TestIsLoopbackHost_Localhost(t *testing.T) {
	assert.True(t, isLoopbackHost("localhost"))
}

func TestIsLoopbackHost_127001(t *testing.T) {
	assert.True(t, isLoopbackHost("127.0.0.1"))
}

func TestIsLoopbackHost_IPv6Loopback(t *testing.T) {
	assert.True(t, isLoopbackHost("::1"))
}

func TestIsLoopbackHost_ExternalHost(t *testing.T) {
	assert.False(t, isLoopbackHost("demo.g8e.ai"))
}

func TestIsLoopbackHost_Empty(t *testing.T) {
	assert.False(t, isLoopbackHost(""))
}

func TestIsLoopbackHost_127002(t *testing.T) {
	assert.False(t, isLoopbackHost("127.0.0.2"))
}
