// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package inference

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	operatorv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/operator/v1"
)

func TestValidateKeepAlive(t *testing.T) {
	t.Parallel()
	assert.NoError(t, validateKeepAlive(""))
	assert.NoError(t, validateKeepAlive("-1"))
	assert.NoError(t, validateKeepAlive("5m"))
	assert.ErrorIs(t, validateKeepAlive("not-a-duration"), constants.ErrInferenceGenerationOptionsInvalid)
	assert.ErrorIs(t, validateKeepAlive(strings.Repeat("x", maxInferenceKeepAliveBytes+1)), constants.ErrInferenceGenerationOptionsInvalid)
}

func TestValidateInferenceRequestBounds_RejectsOversizedText(t *testing.T) {
	t.Parallel()
	req := &operatorv1.InferenceRequested{
		Messages: []*operatorv1.InferenceMessage{{
			Role: operatorv1.InferenceMessageRole_INFERENCE_MESSAGE_ROLE_USER,
			Parts: []*operatorv1.InferenceMessagePart{{
				Part: &operatorv1.InferenceMessagePart_Text{Text: strings.Repeat("a", maxInferenceMessageTextBytes+1)},
			}},
		}},
	}
	err := validateInferenceRequestBounds(req)
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrInferenceRequestTooLarge)
}
