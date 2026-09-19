// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package evaluation

import (
	"fmt"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/g8e-ai/g8e/v2/internal/constants"
)

func TestIsGatewayEvidenceNotFound(t *testing.T) {
	t.Parallel()
	assert.True(t, isGatewayEvidenceNotFound(constants.ErrNotFound))
	assert.True(t, isGatewayEvidenceNotFound(os.ErrNotExist))
	assert.True(t, isGatewayEvidenceNotFound(fmt.Errorf("provider observation: gateway read: %w: status 404: not found", constants.ErrHTTPStatusError)))
	assert.False(t, isGatewayEvidenceNotFound(fmt.Errorf("provider observation: gateway read: %w: status 503: unavailable", constants.ErrHTTPStatusError)))
}
