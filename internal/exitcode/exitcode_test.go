// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package exitcode

import (
	"errors"
	"testing"

	"github.com/g8e-ai/g8e/internal/constants"
)

func TestFromError_NilReturnsSuccess(t *testing.T) {
	got := FromError(nil)
	if got != constants.ExitSuccess {
		t.Errorf("FromError(nil) = %d, want %d (ExitSuccess)", got, constants.ExitSuccess)
	}
}

func TestFromError_GeneralError(t *testing.T) {
	err := errors.New("something unexpected happened")
	got := FromError(err)
	if got != constants.ExitGeneralError {
		t.Errorf("FromError(unrecognized) = %d, want %d (ExitGeneralError)", got, constants.ExitGeneralError)
	}
}

func TestFromError_EmptyString(t *testing.T) {
	err := errors.New("")
	got := FromError(err)
	if got != constants.ExitGeneralError {
		t.Errorf("FromError(empty string) = %d, want %d (ExitGeneralError)", got, constants.ExitGeneralError)
	}
}

func TestFromError_PermissionDenied(t *testing.T) {
	tests := []struct {
		name string
		msg  string
	}{
		{"permission denied", "open /etc/g8e/config.yaml: permission denied"},
		{"access denied", "access denied to resource"},
		{"not writable", "target directory is not writable"},
		{"cannot write", "cannot write to file"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := FromError(errors.New(tc.msg))
			if got != constants.ExitPermissionDenied {
				t.Errorf("FromError(%q) = %d, want %d (ExitPermissionDenied)", tc.msg, got, constants.ExitPermissionDenied)
			}
		})
	}
}

func TestFromError_PermissionDenied_CaseInsensitive(t *testing.T) {
	tests := []string{
		"PERMISSION DENIED",
		"Access Denied",
		"NOT WRITABLE",
		"Cannot Write",
	}
	for _, msg := range tests {
		t.Run(msg, func(t *testing.T) {
			got := FromError(errors.New(msg))
			if got != constants.ExitPermissionDenied {
				t.Errorf("FromError(%q) = %d, want %d (ExitPermissionDenied)", msg, got, constants.ExitPermissionDenied)
			}
		})
	}
}

func TestFromError_CertTrustFailure(t *testing.T) {
	tests := []struct {
		name string
		msg  string
	}{
		{"certificate signed by unknown authority", "certificate signed by unknown authority"},
		{"certificate has expired", "certificate has expired or is not yet valid"},
		{"certificate is not trusted", "certificate is not trusted"},
		{"tls bad certificate", "remote error: tls: bad certificate"},
		{"tls unknown certificate authority", "tls: unknown certificate authority"},
		{"x509 certificate", "x509: certificate signed by unknown authority"},
		{"cert trust failure", "cert trust failure: stale CA"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := FromError(errors.New(tc.msg))
			if got != constants.ExitCertTrustFailure {
				t.Errorf("FromError(%q) = %d, want %d (ExitCertTrustFailure)", tc.msg, got, constants.ExitCertTrustFailure)
			}
		})
	}
}

func TestFromError_CertTrustFailure_CaseInsensitive(t *testing.T) {
	tests := []string{
		"CERTIFICATE SIGNED BY UNKNOWN AUTHORITY",
		"Certificate Has Expired",
		"TLS: BAD CERTIFICATE",
		"X509: CERTIFICATE",
	}
	for _, msg := range tests {
		t.Run(msg, func(t *testing.T) {
			got := FromError(errors.New(msg))
			if got != constants.ExitCertTrustFailure {
				t.Errorf("FromError(%q) = %d, want %d (ExitCertTrustFailure)", msg, got, constants.ExitCertTrustFailure)
			}
		})
	}
}

func TestFromError_AuthFailure(t *testing.T) {
	tests := []struct {
		name string
		msg  string
	}{
		{"authentication failed", "authentication failed: invalid credentials"},
		{"unauthorized", "HTTP 403: unauthorized access"},
		{"401", "server returned 401 Unauthorized"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := FromError(errors.New(tc.msg))
			if got != constants.ExitAuthFailure {
				t.Errorf("FromError(%q) = %d, want %d (ExitAuthFailure)", tc.msg, got, constants.ExitAuthFailure)
			}
		})
	}
}

func TestFromError_AuthFailure_CaseInsensitive(t *testing.T) {
	tests := []string{
		"AUTHENTICATION FAILED",
		"Unauthorized",
		"401",
	}
	for _, msg := range tests {
		t.Run(msg, func(t *testing.T) {
			got := FromError(errors.New(msg))
			if got != constants.ExitAuthFailure {
				t.Errorf("FromError(%q) = %d, want %d (ExitAuthFailure)", msg, got, constants.ExitAuthFailure)
			}
		})
	}
}

func TestFromError_NetworkError(t *testing.T) {
	tests := []struct {
		name string
		msg  string
	}{
		{"connection refused", "dial tcp 127.0.0.1:8080: connect: connection refused"},
		{"no such host", "dial tcp: lookup example.invalid: no such host"},
		{"network unreachable", "dial tcp: network unreachable"},
		{"timeout", "i/o timeout"},
		{"dial tcp", "dial tcp 10.0.0.1:8443: connect: connection refused"},
		{"connectivity failed", "connectivity failed: cannot reach gateway"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := FromError(errors.New(tc.msg))
			if got != constants.ExitNetworkError {
				t.Errorf("FromError(%q) = %d, want %d (ExitNetworkError)", tc.msg, got, constants.ExitNetworkError)
			}
		})
	}
}

func TestFromError_NetworkError_CaseInsensitive(t *testing.T) {
	tests := []string{
		"CONNECTION REFUSED",
		"No Such Host",
		"NETWORK UNREACHABLE",
		"Timeout",
	}
	for _, msg := range tests {
		t.Run(msg, func(t *testing.T) {
			got := FromError(errors.New(msg))
			if got != constants.ExitNetworkError {
				t.Errorf("FromError(%q) = %d, want %d (ExitNetworkError)", msg, got, constants.ExitNetworkError)
			}
		})
	}
}

func TestFromError_StorageError(t *testing.T) {
	tests := []struct {
		name string
		msg  string
	}{
		{"failed to initialize audit vault", "failed to initialize audit vault: disk error"},
		{"failed to initialize database", "failed to initialize database: SQLite error"},
		{"failed to create directory", "failed to create directory: /var/lib/g8e"},
		{"git init failed", "git init failed in /data/vault"},
		{"disk full", "write failed: disk full"},
		{"no space left", "write failed: no space left on device"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := FromError(errors.New(tc.msg))
			if got != constants.ExitStorageError {
				t.Errorf("FromError(%q) = %d, want %d (ExitStorageError)", tc.msg, got, constants.ExitStorageError)
			}
		})
	}
}

func TestFromError_StorageError_CaseInsensitive(t *testing.T) {
	tests := []string{
		"FAILED TO INITIALIZE AUDIT VAULT",
		"Disk Full",
		"NO SPACE LEFT",
	}
	for _, msg := range tests {
		t.Run(msg, func(t *testing.T) {
			got := FromError(errors.New(msg))
			if got != constants.ExitStorageError {
				t.Errorf("FromError(%q) = %d, want %d (ExitStorageError)", msg, got, constants.ExitStorageError)
			}
		})
	}
}

func TestFromError_ConfigError(t *testing.T) {
	tests := []struct {
		name string
		msg  string
	}{
		{"failed to load configuration", "failed to load configuration: parse error"},
		{"missing required", "missing required field: api_key"},
		{"invalid config", "invalid config: port must be > 0"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := FromError(errors.New(tc.msg))
			if got != constants.ExitConfigError {
				t.Errorf("FromError(%q) = %d, want %d (ExitConfigError)", tc.msg, got, constants.ExitConfigError)
			}
		})
	}
}

func TestFromError_ConfigError_CaseInsensitive(t *testing.T) {
	tests := []string{
		"FAILED TO LOAD CONFIGURATION",
		"Missing Required",
		"INVALID CONFIG",
	}
	for _, msg := range tests {
		t.Run(msg, func(t *testing.T) {
			got := FromError(errors.New(msg))
			if got != constants.ExitConfigError {
				t.Errorf("FromError(%q) = %d, want %d (ExitConfigError)", msg, got, constants.ExitConfigError)
			}
		})
	}
}

func TestFromError_PriorityOrdering(t *testing.T) {
	tests := []struct {
		name     string
		msg      string
		wantCode int
	}{
		{
			name:     "permission beats cert",
			msg:      "permission denied: certificate has expired",
			wantCode: constants.ExitPermissionDenied,
		},
		{
			name:     "cert beats auth",
			msg:      "x509: certificate unauthorized",
			wantCode: constants.ExitCertTrustFailure,
		},
		{
			name:     "auth beats network",
			msg:      "unauthorized: connection refused",
			wantCode: constants.ExitAuthFailure,
		},
		{
			name:     "network beats storage",
			msg:      "connection refused: failed to initialize database",
			wantCode: constants.ExitNetworkError,
		},
		{
			name:     "storage beats config",
			msg:      "disk full: invalid config",
			wantCode: constants.ExitStorageError,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := FromError(errors.New(tc.msg))
			if got != tc.wantCode {
				t.Errorf("FromError(%q) = %d, want %d", tc.msg, got, tc.wantCode)
			}
		})
	}
}

func TestContainsAny(t *testing.T) {
	tests := []struct {
		name       string
		s          string
		substrings []string
		want       bool
	}{
		{"exact match", "permission denied", []string{"permission denied"}, true},
		{"substring match", "open file: permission denied", []string{"permission denied"}, true},
		{"case insensitive", "PERMISSION DENIED", []string{"permission denied"}, true},
		{"mixed case substring", "Access Denied", []string{"access denied"}, true},
		{"no match", "file not found", []string{"permission denied"}, false},
		{"empty substrings", "some error", []string{}, false},
		{"empty string", "", []string{"permission denied"}, false},
		{"match in list", "timeout", []string{"connection refused", "timeout", "no such host"}, true},
		{"no match in list", "timeout", []string{"connection refused", "no such host"}, false},
		{"substring case insensitive both ways", "DIAL TCP", []string{"dial tcp"}, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := containsAny(tc.s, tc.substrings)
			if got != tc.want {
				t.Errorf("containsAny(%q, %v) = %v, want %v", tc.s, tc.substrings, got, tc.want)
			}
		})
	}
}
