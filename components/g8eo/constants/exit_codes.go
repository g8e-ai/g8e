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

import "strings"

// Exit codes for the g8e Operator
// These enable the g8e script to provide accurate error messages
const (
	// ExitSuccess indicates the Operator exited normally
	ExitSuccess = 0

	// ExitGeneralError is the default error code for unspecified errors
	ExitGeneralError = 1

	// ExitAuthFailure indicates authentication with g8e failed
	// (invalid/expired API key, wrong key type, deleted Operator slot)
	ExitAuthFailure = 2

	// ExitPermissionDenied indicates a filesystem permission error
	// (cannot create directories, cannot write files)
	ExitPermissionDenied = 3

	// ExitNetworkError indicates a network connectivity issue
	// (cannot reach g8e servers, DNS failure, timeout)
	ExitNetworkError = 4

	// ExitConfigError indicates a configuration error
	// (missing required config, invalid config values)
	ExitConfigError = 5

	// ExitStorageError indicates a storage/database initialization error
	// (SQLite init failed, git init failed, disk full)
	ExitStorageError = 6

	// ExitCertTrustFailure indicates the operator cannot verify the server's TLS certificate.
	// This is a non-retryable condition caused by a stale embedded CA certificate.
	// The operator must self-terminate to prevent noisy retry loops against the server.
	// Resolution: download a new operator binary with updated certificates.
	ExitCertTrustFailure = 7
)

// ExitCodeFromError analyzes an error and returns the appropriate exit code
func ExitCodeFromError(err error) int {
	if err == nil {
		return ExitSuccess
	}

	errStr := err.Error()

	// Check for permission denied errors
	if containsAny(errStr, []string{
		"permission denied",
		"access denied",
		"not writable",
		"cannot write",
	}) {
		return ExitPermissionDenied
	}

	// Check for TLS certificate trust failures (non-retryable, stale CA)
	if containsAny(errStr, []string{
		"certificate signed by unknown authority",
		"certificate has expired",
		"certificate is not trusted",
		"tls: bad certificate",
		"tls: unknown certificate authority",
		"x509: certificate",
		"cert trust failure",
	}) {
		return ExitCertTrustFailure
	}

	// Check for authentication errors
	if containsAny(errStr, []string{
		"authentication failed",
		"invalid api key",
		"api key expired",
		"unauthorized",
		"401",
	}) {
		return ExitAuthFailure
	}

	// Check for network errors
	if containsAny(errStr, []string{
		"connection refused",
		"no such host",
		"network unreachable",
		"timeout",
		"dial tcp",
		"connectivity failed",
	}) {
		return ExitNetworkError
	}

	// Check for storage errors (database, git, filesystem)
	if containsAny(errStr, []string{
		"failed to initialize audit vault",
		"failed to initialize database",
		"failed to create directory",
		"git init failed",
		"disk full",
		"no space left",
	}) {
		return ExitStorageError
	}

	// Check for config errors
	if containsAny(errStr, []string{
		"failed to load configuration",
		"missing required",
		"invalid config",
	}) {
		return ExitConfigError
	}

	return ExitGeneralError
}

// containsAny checks if s contains any of the substrings (case-insensitive).
// All error-text substrings in ExitCodeFromError are ASCII, so ToLower is safe.
func containsAny(s string, substrings []string) bool {
	sLower := strings.ToLower(s)
	for _, sub := range substrings {
		if strings.Contains(sLower, strings.ToLower(sub)) {
			return true
		}
	}
	return false
}
