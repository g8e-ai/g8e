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

package errors

import (
	"errors"
	"testing"
)

func TestErrorsDefined(t *testing.T) {
	tests := []struct {
		name  string
		err   error
		want  string
	}{
		{
			name: "ErrNotAuthenticated",
			err:  ErrNotAuthenticated,
			want: "not authenticated",
		},
		{
			name: "ErrFailedToLoadCredentials",
			err:  ErrFailedToLoadCredentials,
			want: "failed to load credentials",
		},
		{
			name: "ErrFailedToLoadClientCertificate",
			err:  ErrFailedToLoadClientCertificate,
			want: "failed to load client certificate",
		},
		{
			name: "ErrFailedToReadTrustBundle",
			err:  ErrFailedToReadTrustBundle,
			want: "failed to read trust bundle",
		},
		{
			name: "ErrFailedToParseTrustBundle",
			err:  ErrFailedToParseTrustBundle,
			want: "failed to parse trust bundle",
		},
		{
			name: "ErrFailedToParsePaths",
			err:  ErrFailedToParsePaths,
			want: "failed to parse paths.json",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.err == nil {
				t.Errorf("error variable %s is nil", tt.name)
			}
			if tt.err.Error() != tt.want {
				t.Errorf("error message mismatch: got %q, want %q", tt.err.Error(), tt.want)
			}
		})
	}
}

func TestErrorsIs(t *testing.T) {
	tests := []struct {
		name  string
		err   error
		target error
		want  bool
	}{
		{
			name:  "ErrNotAuthenticated matches itself",
			err:   ErrNotAuthenticated,
			target: ErrNotAuthenticated,
			want:  true,
		},
		{
			name:  "ErrFailedToLoadCredentials matches itself",
			err:   ErrFailedToLoadCredentials,
			target: ErrFailedToLoadCredentials,
			want:  true,
		},
		{
			name:  "ErrFailedToLoadClientCertificate matches itself",
			err:   ErrFailedToLoadClientCertificate,
			target: ErrFailedToLoadClientCertificate,
			want:  true,
		},
		{
			name:  "ErrFailedToReadTrustBundle matches itself",
			err:   ErrFailedToReadTrustBundle,
			target: ErrFailedToReadTrustBundle,
			want:  true,
		},
		{
			name:  "ErrFailedToParseTrustBundle matches itself",
			err:   ErrFailedToParseTrustBundle,
			target: ErrFailedToParseTrustBundle,
			want:  true,
		},
		{
			name:  "ErrFailedToParsePaths matches itself",
			err:   ErrFailedToParsePaths,
			target: ErrFailedToParsePaths,
			want:  true,
		},
		{
			name:  "ErrNotAuthenticated does not match other error",
			err:   ErrNotAuthenticated,
			target: ErrFailedToLoadCredentials,
			want:  false,
		},
		{
			name:  "wrapped error matches target",
			err:   errors.New("wrapped: not authenticated"),
			target: ErrNotAuthenticated,
			want:  false,
		},
		{
			name:  "wrapped error with ErrNotAuthenticated matches",
			err:   errors.New("context: not authenticated"),
			target: ErrNotAuthenticated,
			want:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := errors.Is(tt.err, tt.target)
			if got != tt.want {
				t.Errorf("errors.Is(%v, %v) = %v, want %v", tt.err, tt.target, got, tt.want)
			}
		})
	}
}

func TestErrorUniqueness(t *testing.T) {
	tests := []struct {
		name  string
		err1  error
		err2  error
		want  bool
	}{
		{
			name:  "ErrNotAuthenticated and ErrFailedToLoadCredentials are different",
			err1:  ErrNotAuthenticated,
			err2:  ErrFailedToLoadCredentials,
			want:  false,
		},
		{
			name:  "ErrFailedToLoadCredentials and ErrFailedToLoadClientCertificate are different",
			err1:  ErrFailedToLoadCredentials,
			err2:  ErrFailedToLoadClientCertificate,
			want:  false,
		},
		{
			name:  "ErrFailedToReadTrustBundle and ErrFailedToParseTrustBundle are different",
			err1:  ErrFailedToReadTrustBundle,
			err2:  ErrFailedToParseTrustBundle,
			want:  false,
		},
		{
			name:  "ErrNotAuthenticated matches itself",
			err1:  ErrNotAuthenticated,
			err2:  ErrNotAuthenticated,
			want:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := errors.Is(tt.err1, tt.err2)
			if got != tt.want {
				t.Errorf("errors.Is(%v, %v) = %v, want %v", tt.err1, tt.err2, got, tt.want)
			}
		})
	}
}
