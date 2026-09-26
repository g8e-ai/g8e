// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software
// is released under the Apache License, Version 2.0.

package governance

import (
	"fmt"

	"google.golang.org/protobuf/proto"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	commonv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/common/v1"
	evalv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/eval/v1"
	operatorv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/operator/v1"
)

// DecodePayloadForAction decodes a governed envelope's raw payload bytes into
// the typed protobuf message for the given action type. It is the single decode
// authority shared by the L4 Warden (operator-side verification) and the
// gateway envelope builder (gateway-side L1 screening). Missing decoders fail
// closed with ErrTxPayloadDecoderMissing; unknown action types must be
// rejected by the caller before reaching this function.
func DecodePayloadForAction(actionType constants.ActionType, payload []byte) (proto.Message, error) {
	var msg proto.Message
	switch actionType {
	case constants.ActionTypeExecuteBash:
		msg = &operatorv1.CommandRequested{}
	case constants.ActionTypeFileEdit:
		msg = &operatorv1.FileEditRequested{}
	case constants.ActionTypeRestoreFile:
		msg = &operatorv1.RestoreFileRequested{}
	case constants.ActionTypeShutdown:
		msg = &operatorv1.ShutdownRequested{}
	case constants.ActionTypeFsList:
		msg = &operatorv1.FsListRequested{}
	case constants.ActionTypeFsRead:
		msg = &operatorv1.FsReadRequested{}
	case constants.ActionTypeFsGrep:
		msg = &operatorv1.FsGrepRequested{}
	case constants.ActionTypePortCheck:
		msg = &operatorv1.CheckPortRequested{}
	case constants.ActionTypeOllamaModelInventory:
		msg = &operatorv1.OllamaModelInventoryRequested{}
	case constants.ActionTypeOllamaModelResidency:
		msg = &operatorv1.OllamaModelResidencyRequested{}
	case constants.ActionTypeFetchLogs:
		msg = &operatorv1.FetchLogsRequested{}
	case constants.ActionTypeFetchHistory:
		msg = &operatorv1.FetchHistoryRequested{}
	case constants.ActionTypeFetchFileHistory:
		msg = &operatorv1.FetchFileHistoryRequested{}
	case constants.ActionTypeEvalAnswer:
		msg = &operatorv1.EvalAnswerRequested{}
	case constants.ActionTypeMcpCall:
		msg = &operatorv1.McpCallRequested{}
	case constants.ActionTypeA2aCall:
		msg = &operatorv1.A2ACallRequested{}
	case constants.ActionTypeMcpResourceRead:
		msg = &operatorv1.McpResourceReadRequested{}
	case constants.ActionTypeMcpPromptGet:
		msg = &operatorv1.McpPromptGetRequested{}
	case constants.ActionTypeFetchFileDiff:
		msg = &operatorv1.FetchFileDiffRequested{}
	case constants.ActionTypeMcpResourceList:
		msg = &operatorv1.McpResourceListRequested{}
	case constants.ActionTypeMcpPromptList:
		msg = &operatorv1.McpPromptListRequested{}
	case constants.ActionTypeHeartbeat:
		msg = &operatorv1.HeartbeatRequested{}
	case constants.ActionTypeCancel:
		msg = &operatorv1.CommandCancelRequested{}
	case constants.ActionTypeDocumentUpdate:
		msg = &operatorv1.DocumentUpdateRequested{}
	case constants.ActionTypeDocumentDelete:
		msg = &operatorv1.DocumentDeleteRequested{}
	case constants.ActionTypeInference:
		msg = &operatorv1.InferenceRequested{}
	case constants.ActionTypeProviderBoundaryObservation:
		msg = &evalv1.ProviderBoundaryObservationCommand{}
	case constants.ActionTypeModelProvenanceObservation:
		msg = &evalv1.ModelProvenanceObservationCommand{}
	case constants.ActionTypePlatformEnrollmentCreate,
		constants.ActionTypePlatformEnrollmentDecide,
		constants.ActionTypePlatformEnrollmentIssue,
		constants.ActionTypePlatformEnrollmentPersistPolicy,
		constants.ActionTypePlatformEnrollmentCreateSession,
		constants.ActionTypePlatformEnrollmentRevoke:
		// All six platform enrollment actions share the same
		// PlatformEnrollmentGovernancePayload proto. The action field
		// inside the payload distinguishes them; L1 doctrine validates
		// the payload shape and the handler layer enforces per-action
		// semantics. CSR PEM, token hashes, and private keys are never
		// placed in the audited envelope payload.
		msg = &commonv1.PlatformEnrollmentGovernancePayload{}

	default:
		return nil, fmt.Errorf("%w: %q", constants.ErrTxPayloadDecoderMissing, actionType)
	}
	if err := proto.Unmarshal(payload, msg); err != nil {
		return nil, err
	}
	return msg, nil
}
