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

// Package exitcode maps errors to g8e Operator exit codes.
package exitcode

import (
	"strings"

	"github.com/g8e-ai/g8e/internal/constants"
)

// FromError analyzes an error and returns the appropriate exit code.
func FromError(err error) int {
	if err == nil {
		return constants.ExitSuccess
	}

	errStr := err.Error()

	if containsAny(errStr, []string{
		"permission denied",
		"access denied",
		"not writable",
		"cannot write",
	}) {
		return constants.ExitPermissionDenied
	}

	// TLS certificate trust failures are non-retryable (stale CA).
	if containsAny(errStr, []string{
		"certificate signed by unknown authority",
		"certificate has expired",
		"certificate is not trusted",
		"tls: bad certificate",
		"tls: unknown certificate authority",
		"x509: certificate",
		"cert trust failure",
	}) {
		return constants.ExitCertTrustFailure
	}

	if containsAny(errStr, []string{
		"authentication failed",
		"unauthorized",
		"401",
	}) {
		return constants.ExitAuthFailure
	}

	if containsAny(errStr, []string{
		"connection refused",
		"no such host",
		"network unreachable",
		"timeout",
		"dial tcp",
		"connectivity failed",
	}) {
		return constants.ExitNetworkError
	}

	if containsAny(errStr, []string{
		"failed to initialize audit vault",
		"failed to initialize database",
		"failed to create directory",
		"git init failed",
		"disk full",
		"no space left",
	}) {
		return constants.ExitStorageError
	}

	if containsAny(errStr, []string{
		"failed to load configuration",
		"missing required",
		"invalid config",
	}) {
		return constants.ExitConfigError
	}

	return constants.ExitGeneralError
}

func containsAny(s string, substrings []string) bool {
	sLower := strings.ToLower(s)
	for _, sub := range substrings {
		if strings.Contains(sLower, strings.ToLower(sub)) {
			return true
		}
	}
	return false
}
