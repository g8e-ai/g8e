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

package pubsub

import (
	"crypto/tls"
	"crypto/x509"
	"errors"
	"net"
)

func containsAny(s string, patterns []string) bool {
	for _, p := range patterns {
		if len(s) >= len(p) && findSubstring(s, p) {
			return true
		}
	}
	return false
}

func findSubstring(s, substr string) bool {
	sLower := toLower(s)
	subLower := toLower(substr)
	return len(sLower) >= len(subLower) && hasSubstring(sLower, subLower)
}

func toLower(s string) string {
	result := make([]byte, len(s))
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c >= 'A' && c <= 'Z' {
			result[i] = c + 32
		} else {
			result[i] = c
		}
	}
	return string(result)
}

func hasSubstring(s, substr string) bool {
	if len(substr) == 0 {
		return true
	}
	if len(s) < len(substr) {
		return false
	}
	for i := 0; i <= len(s)-len(substr); i++ {
		match := true
		for j := 0; j < len(substr); j++ {
			if s[i+j] != substr[j] {
				match = false
				break
			}
		}
		if match {
			return true
		}
	}
	return false
}

// IsTLSCertError returns true if the error indicates a TLS certificate trust failure.
// These are non-retryable conditions caused by stale embedded CA certificates or
// expired/revoked server certificates. The operator must self-terminate when this
// occurs to prevent noisy retry loops against the server.
func IsTLSCertError(err error) bool {
	if err == nil {
		return false
	}

	// Check for x509 certificate verification errors (most common case)
	// Go wraps these as x509.UnknownAuthorityError, x509.CertificateInvalidError, etc.
	var unknownAuthErr x509.UnknownAuthorityError
	if errors.As(err, &unknownAuthErr) {
		return true
	}

	var certInvalidErr x509.CertificateInvalidError
	if errors.As(err, &certInvalidErr) {
		return true
	}

	var hostnameErr x509.HostnameError
	if errors.As(err, &hostnameErr) {
		return true
	}

	// Check for tls.RecordHeaderError (malformed TLS)
	var recordErr tls.RecordHeaderError
	if errors.As(err, &recordErr) {
		return true
	}

	// Check for tls.AlertError (TLS alert from peer, e.g. bad_certificate = alert 42)
	var alertErr *tls.AlertError
	if errors.As(err, &alertErr) {
		return true
	}

	// Check for net.OpError wrapping a TLS error
	var opErr *net.OpError
	if errors.As(err, &opErr) {
		return IsTLSCertError(opErr.Err)
	}

	// Fallback: string matching for edge cases where error types aren't exported
	errStr := err.Error()
	return containsAny(errStr, []string{
		"certificate signed by unknown authority",
		"certificate has expired",
		"certificate is not trusted",
		"tls: bad certificate",
		"tls: unknown certificate authority",
		"x509: certificate",
		"tls: failed to verify certificate",
		"tls: handshake failure",
	})
}
