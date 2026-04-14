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

package constants

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestExitCodes_UniqueValues(t *testing.T) {
	codes := map[int]string{
		ExitSuccess:          "ExitSuccess",
		ExitGeneralError:     "ExitGeneralError",
		ExitAuthFailure:      "ExitAuthFailure",
		ExitPermissionDenied: "ExitPermissionDenied",
		ExitNetworkError:     "ExitNetworkError",
		ExitConfigError:      "ExitConfigError",
		ExitStorageError:     "ExitStorageError",
		ExitCertTrustFailure: "ExitCertTrustFailure",
	}

	assert.Len(t, codes, 8, "all exit codes should have unique values")
}

func TestExitCodes_ExpectedValues(t *testing.T) {
	assert.Equal(t, 0, ExitSuccess)
	assert.Equal(t, 1, ExitGeneralError)
	assert.Equal(t, 2, ExitAuthFailure)
	assert.Equal(t, 3, ExitPermissionDenied)
	assert.Equal(t, 4, ExitNetworkError)
	assert.Equal(t, 5, ExitConfigError)
	assert.Equal(t, 6, ExitStorageError)
	assert.Equal(t, 7, ExitCertTrustFailure)
}

func TestExitCodeFromError_NilError(t *testing.T) {
	assert.Equal(t, ExitSuccess, ExitCodeFromError(nil))
}

func TestExitCodeFromError_CertTrustFailure(t *testing.T) {
	tests := []struct {
		name string
		err  error
	}{
		{
			name: "x509 certificate signed by unknown authority",
			err:  errors.New("tls: failed to verify certificate: x509: certificate signed by unknown authority"),
		},
		{
			name: "certificate has expired",
			err:  errors.New("x509: certificate has expired or is not yet valid"),
		},
		{
			name: "tls bad certificate",
			err:  errors.New("tls: bad certificate"),
		},
		{
			name: "tls unknown certificate authority",
			err:  errors.New("tls: unknown certificate authority"),
		},
		{
			name: "x509 certificate generic",
			err:  errors.New("x509: certificate is valid for example.com, not g8e.local"),
		},
		{
			name: "cert trust failure marker",
			err:  errors.New("cert trust failure: stale CA"),
		},
		{
			name: "certificate is not trusted",
			err:  errors.New("certificate is not trusted"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, ExitCertTrustFailure, ExitCodeFromError(tt.err),
				"error %q should map to ExitCertTrustFailure", tt.err)
		})
	}
}

func TestExitCodeFromError_CertTrustTakesPriorityOverNetwork(t *testing.T) {
	// An error containing both "timeout" and "x509: certificate" should be
	// classified as cert trust failure, not network error, because cert trust
	// is checked first and is non-retryable.
	err := errors.New("dial tcp: x509: certificate signed by unknown authority")
	assert.Equal(t, ExitCertTrustFailure, ExitCodeFromError(err))
}

func TestExitCodeFromError_PermissionDenied(t *testing.T) {
	assert.Equal(t, ExitPermissionDenied, ExitCodeFromError(errors.New("permission denied")))
	assert.Equal(t, ExitPermissionDenied, ExitCodeFromError(errors.New("access denied")))
}

func TestExitCodeFromError_AuthFailure(t *testing.T) {
	assert.Equal(t, ExitAuthFailure, ExitCodeFromError(errors.New("authentication failed: invalid api key")))
	assert.Equal(t, ExitAuthFailure, ExitCodeFromError(errors.New("unauthorized")))
}

func TestExitCodeFromError_NetworkError(t *testing.T) {
	assert.Equal(t, ExitNetworkError, ExitCodeFromError(errors.New("connection refused")))
	assert.Equal(t, ExitNetworkError, ExitCodeFromError(errors.New("no such host")))
	assert.Equal(t, ExitNetworkError, ExitCodeFromError(errors.New("timeout")))
}

func TestExitCodeFromError_StorageError(t *testing.T) {
	assert.Equal(t, ExitStorageError, ExitCodeFromError(errors.New("failed to initialize audit vault")))
	assert.Equal(t, ExitStorageError, ExitCodeFromError(errors.New("disk full")))
}

func TestExitCodeFromError_ConfigError(t *testing.T) {
	assert.Equal(t, ExitConfigError, ExitCodeFromError(errors.New("failed to load configuration")))
	assert.Equal(t, ExitConfigError, ExitCodeFromError(errors.New("missing required field")))
}

func TestExitCodeFromError_GeneralError(t *testing.T) {
	assert.Equal(t, ExitGeneralError, ExitCodeFromError(errors.New("something unexpected happened")))
}
