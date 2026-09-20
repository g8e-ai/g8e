// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package gateway

import (
	"testing"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/models"
	operatorv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/operator/v1"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
)

func TestValidateWitnessCommandDispatch(t *testing.T) {
	payload, err := proto.Marshal(&operatorv1.CommandRequested{Command: "ollama stop qwen3:0.6b", ExecutionId: "exec-1"})
	require.NoError(t, err)

	ordinary := &models.OperatorDocumentGo{
		RuntimeConfig: &models.RuntimeConfig{InferenceEnabled: true},
	}
	require.NoError(t, validateWitnessCommandDispatch(ordinary, string(constants.ActionTypeExecuteBash), payload))

	observer := &models.OperatorDocumentGo{
		RuntimeConfig: &models.RuntimeConfig{ProviderBoundaryObserverEnabled: true},
	}
	err = validateWitnessCommandDispatch(observer, string(constants.ActionTypeExecuteBash), payload)
	require.ErrorIs(t, err, constants.ErrWitnessCommandNotCapable)

	provenance := &models.OperatorDocumentGo{
		RuntimeConfig: &models.RuntimeConfig{ProvenanceOperatorEnabled: true},
	}
	err = validateWitnessCommandDispatch(provenance, string(constants.ActionTypeExecuteBash), payload)
	require.ErrorIs(t, err, constants.ErrWitnessCommandNotCapable)
}
