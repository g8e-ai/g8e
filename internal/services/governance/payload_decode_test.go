// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software
// is released under the Apache License, Version 2.0.

package governance

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	operatorv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/operator/v1"
)

// TestDecodePayloadForAction_Inference verifies that the shared package-level
// decoder decodes ActionTypeInference into a typed *operatorv1.InferenceRequested.
// Before this case existed, inference payloads fell through to the nil default in
// the warden, skipping L1 doctrine validation entirely — a mutation-classified
// action could reach execution without L1 screening.
func TestDecodePayloadForAction_Inference(t *testing.T) {
	t.Parallel()

	infReq := &operatorv1.InferenceRequested{
		Role:  operatorv1.ModelRole_MODEL_ROLE_PRIMARY,
		Model: "gemma3:4b",
		Messages: []*operatorv1.InferenceMessage{{
			Role:  operatorv1.InferenceMessageRole_INFERENCE_MESSAGE_ROLE_USER,
			Parts: []*operatorv1.InferenceMessagePart{{Part: &operatorv1.InferenceMessagePart_Text{Text: "summarize the doctrine"}}},
		}},
		Temperature: 0.7,
		MaxTokens:   256,
		KeepAlive:   "5m",
	}
	payload, err := proto.Marshal(infReq)
	require.NoError(t, err)

	decoded, err := DecodePayloadForAction(constants.ActionTypeInference, payload)
	require.NoError(t, err)
	require.NotNil(t, decoded, "inference payload must decode to a typed message, not nil")

	got, ok := decoded.(*operatorv1.InferenceRequested)
	require.True(t, ok, "decoded payload must be *operatorv1.InferenceRequested, got %T", decoded)
	assert.True(t, proto.Equal(infReq, got))
}

// TestDecodePayloadForAction_Inference_MalformedFails verifies that a malformed
// inference payload fails closed with a decode error rather than returning nil
// (which would skip L1 validation).
func TestDecodePayloadForAction_Inference_MalformedFails(t *testing.T) {
	t.Parallel()

	_, err := DecodePayloadForAction(constants.ActionTypeInference, []byte{0xFF, 0xFF, 0xFF})
	require.Error(t, err, "malformed inference payload must fail closed")
}

// TestDecodePayloadForAction_KnownActionsDecode verifies that the shared decoder
// still decodes every previously-supported action type, proving the promotion
// from the warden method to the package-level function preserved behavior.
func TestDecodePayloadForAction_KnownActionsDecode(t *testing.T) {
	t.Parallel()

	// A representative sample across read, mutation, and bootstrap classes.
	samples := []constants.ActionType{
		constants.ActionTypeFsRead,
		constants.ActionTypeExecuteBash,
		constants.ActionTypeMcpCall,
		constants.ActionTypeHeartbeat,
	}
	for _, at := range samples {
		t.Run(string(at), func(t *testing.T) {
			t.Parallel()
			payload := typedPayload(t, at)
			decoded, err := DecodePayloadForAction(at, payload)
			require.NoError(t, err)
			if at == constants.ActionTypeHeartbeat {
				// HeartbeatRequested has no fields; it still decodes to a non-nil message.
				assert.NotNil(t, decoded)
			} else {
				assert.NotNil(t, decoded, "%s must decode to a typed message", at)
			}
		})
	}
}
