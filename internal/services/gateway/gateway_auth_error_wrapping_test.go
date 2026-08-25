// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package gateway

import (
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/g8e-ai/g8e/internal/constants"
)

// TestRevocationCheckErrorWrapping_SemanticallyCorrect verifies that the
// revocation-check failure error wrapping (E.7) uses a single %w for the
// real cause and a plain string for the classification constant. A
// revocation-check failure must not match ErrCertParseFailed (the original
// double-%w bug made errors.Is return true for an unrelated classification).
func TestRevocationCheckErrorWrapping_SemanticallyCorrect(t *testing.T) {
	revocationErr := errors.New("certificate revoked: serial 0xabc")
	wrapped := fmt.Errorf("gateway: auth: verify certificate: %w: %s", revocationErr, constants.ErrCertRevocationCheckFailed)

	assert.True(t, errors.Is(wrapped, revocationErr), "real cause must be unwrappable via errors.Is")
	assert.False(t, errors.Is(wrapped, constants.ErrCertParseFailed), "revocation failure must not match ErrCertParseFailed")
	assert.Contains(t, wrapped.Error(), constants.ErrCertRevocationCheckFailed.Error(), "classification constant must appear in message")
}
