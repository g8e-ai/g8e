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
	"crypto/x509"
	"errors"
	"fmt"
	"net"
	"testing"

	"github.com/g8e-ai/g8e/components/vsa/constants"
	"github.com/stretchr/testify/assert"
)

func TestIsTLSCertError(t *testing.T) {
	t.Run("returns false for nil error", func(t *testing.T) {
		assert.False(t, IsTLSCertError(nil))
	})

	t.Run("returns false for generic errors", func(t *testing.T) {
		assert.False(t, IsTLSCertError(errors.New("connection refused")))
		assert.False(t, IsTLSCertError(errors.New("timeout")))
		assert.False(t, IsTLSCertError(errors.New("EOF")))
		assert.False(t, IsTLSCertError(errors.New("some random error")))
	})

	t.Run("detects x509.UnknownAuthorityError", func(t *testing.T) {
		err := x509.UnknownAuthorityError{}
		assert.True(t, IsTLSCertError(err))
	})

	t.Run("detects x509.CertificateInvalidError", func(t *testing.T) {
		err := x509.CertificateInvalidError{
			Reason: x509.Expired,
		}
		assert.True(t, IsTLSCertError(err))
	})

	t.Run("detects x509.HostnameError", func(t *testing.T) {
		err := x509.HostnameError{
			Host: constants.DefaultEndpoint,
		}
		assert.True(t, IsTLSCertError(err))
	})

	t.Run("detects wrapped x509 errors", func(t *testing.T) {
		innerErr := x509.UnknownAuthorityError{}
		wrappedErr := fmt.Errorf("connection failed: %w", innerErr)
		assert.True(t, IsTLSCertError(wrappedErr))
	})

	t.Run("detects x509 errors wrapped in net.OpError", func(t *testing.T) {
		innerErr := x509.UnknownAuthorityError{}
		opErr := &net.OpError{
			Op:  "read",
			Net: "tcp",
			Err: innerErr,
		}
		assert.True(t, IsTLSCertError(opErr))
	})

	t.Run("detects string-based certificate signed by unknown authority", func(t *testing.T) {
		err := errors.New("tls: failed to verify certificate: x509: certificate signed by unknown authority")
		assert.True(t, IsTLSCertError(err))
	})

	t.Run("detects string-based certificate has expired", func(t *testing.T) {
		err := errors.New("x509: certificate has expired or is not yet valid")
		assert.True(t, IsTLSCertError(err))
	})

	t.Run("detects string-based tls bad certificate", func(t *testing.T) {
		err := errors.New("tls: bad certificate")
		assert.True(t, IsTLSCertError(err))
	})

	t.Run("detects string-based tls handshake failure", func(t *testing.T) {
		err := errors.New("tls: handshake failure")
		assert.True(t, IsTLSCertError(err))
	})

	t.Run("returns false for non-TLS net.OpError", func(t *testing.T) {
		opErr := &net.OpError{
			Op:  "dial",
			Net: "tcp",
			Err: errors.New("connection refused"),
		}
		assert.False(t, IsTLSCertError(opErr))
	})

	t.Run("detects deeply nested x509 error", func(t *testing.T) {
		innerErr := x509.CertificateInvalidError{Reason: x509.NotAuthorizedToSign}
		wrapped1 := fmt.Errorf("redis: %w", innerErr)
		wrapped2 := fmt.Errorf("pubsub receive: %w", wrapped1)
		assert.True(t, IsTLSCertError(wrapped2))
	})
}
