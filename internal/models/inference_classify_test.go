// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package models

import (
	"testing"

	"github.com/stretchr/testify/assert"

	operatorv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/operator/v1"
)

func TestClassifyRetryAndLoadState(t *testing.T) {
	t.Parallel()
	assert.Equal(t, operatorv1.InferenceRetryClassification_INFERENCE_RETRY_CLASSIFICATION_NONE, ClassifyRetry(0))
	assert.Equal(t, operatorv1.InferenceRetryClassification_INFERENCE_RETRY_CLASSIFICATION_INFRASTRUCTURE_PRE_RESULT, ClassifyRetry(2))

	assert.Equal(t, operatorv1.InferenceLoadState_INFERENCE_LOAD_STATE_UNAVAILABLE, ClassifyLoadState(nil))
	warm := int64(0)
	assert.Equal(t, operatorv1.InferenceLoadState_INFERENCE_LOAD_STATE_WARM, ClassifyLoadState(&warm))
	cold := int64(1_000_000)
	assert.Equal(t, operatorv1.InferenceLoadState_INFERENCE_LOAD_STATE_COLD, ClassifyLoadState(&cold))
}
