// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package inference

import (
	"fmt"
	"time"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	operatorv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/operator/v1"
)

const (
	maxInferenceTopK                  = 1000
	maxInferenceStopSequences         = 16
	maxInferenceStopSequenceBytes     = 256
	maxInferenceContextLimit          = 1 << 20
	maxInferenceKeepAliveBytes        = 32
	maxInferenceMessageTextBytes      = 256 << 10
	maxInferenceToolSchemaBytes       = 64 << 10
	maxInferenceToolJSONBytes         = 256 << 10
	maxInferenceMessagesCount         = 256
	maxInferenceToolsCount            = 64
	maxInferenceGovernedPayloadBytes  = 4 << 20
	maxInferenceResponsePartTextBytes = 256 << 10
)

func validateKeepAlive(value string) error {
	if value == "" {
		return nil
	}
	if len(value) > maxInferenceKeepAliveBytes {
		return fmt.Errorf("%w: keep_alive", constants.ErrInferenceGenerationOptionsInvalid)
	}
	switch value {
	case "-1", "0":
		return nil
	}
	if _, err := time.ParseDuration(value); err != nil {
		return fmt.Errorf("%w: keep_alive", constants.ErrInferenceGenerationOptionsInvalid)
	}
	return nil
}

func validateInferenceRequestBounds(req *operatorv1.InferenceRequested) error {
	if len(req.GetMessages()) > maxInferenceMessagesCount {
		return fmt.Errorf("%w: messages", constants.ErrInferenceRequestTooLarge)
	}
	if len(req.GetTools()) > maxInferenceToolsCount {
		return fmt.Errorf("%w: tools", constants.ErrInferenceRequestTooLarge)
	}
	if err := validateKeepAlive(req.GetKeepAlive()); err != nil {
		return err
	}
	for messageIndex, message := range req.GetMessages() {
		for partIndex, part := range message.GetParts() {
			switch value := part.GetPart().(type) {
			case *operatorv1.InferenceMessagePart_Text:
				if len(value.Text) > maxInferenceMessageTextBytes {
					return fmt.Errorf("%w: message %d text", constants.ErrInferenceRequestTooLarge, messageIndex)
				}
			case *operatorv1.InferenceMessagePart_ToolCall:
				if len(value.ToolCall.GetArgumentsJson()) > maxInferenceToolJSONBytes {
					return fmt.Errorf("%w: message %d tool call %d arguments", constants.ErrInferenceRequestTooLarge, messageIndex, partIndex)
				}
			case *operatorv1.InferenceMessagePart_ToolResult:
				if len(value.ToolResult.GetResultJson()) > maxInferenceToolJSONBytes {
					return fmt.Errorf("%w: message %d tool result %d", constants.ErrInferenceRequestTooLarge, messageIndex, partIndex)
				}
			}
		}
	}
	for toolIndex, tool := range req.GetTools() {
		if len(tool.GetJsonSchema()) > maxInferenceToolSchemaBytes {
			return fmt.Errorf("%w: tool %d schema", constants.ErrInferenceRequestTooLarge, toolIndex)
		}
	}
	if req.GetResponseFormat() != nil && len(req.GetResponseFormat().GetJsonSchema()) > maxInferenceToolSchemaBytes {
		return fmt.Errorf("%w: response schema", constants.ErrInferenceRequestTooLarge)
	}
	return nil
}
