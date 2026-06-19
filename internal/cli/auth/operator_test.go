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

package auth

import (
	"crypto/x509"
	"fmt"
	"net"
	"runtime"
	"strconv"
	"testing"

	"github.com/g8e-ai/g8e/internal/constants"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCheckOperatorRunning_NotRunning(t *testing.T) {
	t.Parallel()

	// Test with a non-existent port to ensure error
	err := CheckOperatorRunningAtURL("http://localhost:99999")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "g8e Gateway is not running or not responding")
}

func TestCheckOperatorRunning_HealthCheckFailed(t *testing.T) {
	t.Parallel()

	// Test with a non-existent port
	err := CheckOperatorRunningAtURL("https://localhost:99999")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not running or not responding")
}

func TestCheckOperatorRunning_Success(t *testing.T) {
	t.Parallel()

	// Start a test server on a random port
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer listener.Close()

	_, port, err := net.SplitHostPort(listener.Addr().String())
	require.NoError(t, err)

	url := fmt.Sprintf("http://127.0.0.1:%s", port)
	err = CheckOperatorRunningAtURL(url)
	require.NoError(t, err)
}

func TestCheckOperatorRunning_InvalidURL(t *testing.T) {
	t.Parallel()

	err := CheckOperatorRunningAtURL("invalid-url")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid Operator URL")
}

func TestCheckOperatorRunning_URLWithoutProtocol(t *testing.T) {
	t.Parallel()

	err := CheckOperatorRunningAtURL("localhost:" + strconv.Itoa(constants.Ports.OperatorHttp))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid Operator URL")
}

func TestIsCertificateVerificationError_UnknownAuthorityError(t *testing.T) {
	t.Parallel()

	// Create a real UnknownAuthorityError
	err := x509.UnknownAuthorityError{}
	assert.True(t, isCertificateVerificationError(err))
}

func TestIsCertificateVerificationError_HostnameError(t *testing.T) {
	t.Parallel()

	// Create a real HostnameError with required fields
	err := x509.HostnameError{
		Host: "test.com",
	}
	assert.True(t, isCertificateVerificationError(err))
}

func TestIsCertificateVerificationError_CertificateInvalidError(t *testing.T) {
	t.Parallel()

	// Create a real CertificateInvalidError with required fields
	err := x509.CertificateInvalidError{
		Reason: x509.NotAuthorizedToSign,
	}
	assert.True(t, isCertificateVerificationError(err))
}

func TestIsCertificateVerificationError_WrappedError(t *testing.T) {
	t.Parallel()

	innerErr := x509.UnknownAuthorityError{}
	wrappedErr := fmt.Errorf("wrapped: %w", innerErr)
	assert.True(t, isCertificateVerificationError(wrappedErr))
}

func TestIsCertificateVerificationError_NonCertError(t *testing.T) {
	t.Parallel()

	err := fmt.Errorf("some other error")
	assert.False(t, isCertificateVerificationError(err))
}

func TestIsCertificateVerificationError_Nil(t *testing.T) {
	t.Parallel()

	assert.False(t, isCertificateVerificationError(nil))
}

func TestGetLocalOSUser(t *testing.T) {
	t.Parallel()

	user := getLocalOSUser()
	require.NotNil(t, user)
	assert.NotEmpty(t, user.Username)
	assert.NotEmpty(t, user.UID)
	assert.NotEmpty(t, user.GID)

	// On Windows, SID should be populated
	if runtime.GOOS == "windows" {
		assert.NotEmpty(t, user.SID)
	}
}
