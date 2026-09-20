// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package gateway

import (
	"fmt"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/models"
	"github.com/g8e-ai/g8e/v2/internal/services/governance"
	"github.com/g8e-ai/g8e/v2/internal/services/operatorcapability"
	operatorv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/operator/v1"
)

func validateWitnessCommandDispatch(op *models.OperatorDocumentGo, actionType string, payload []byte) error {
	if op == nil || actionType != string(constants.ActionTypeExecuteBash) {
		return nil
	}
	decoded, err := governance.DecodePayloadForAction(constants.ActionTypeExecuteBash, payload)
	if err != nil {
		return fmt.Errorf("dispatch: %w", constants.ErrTxPayloadDecodeFailed)
	}
	cmdReq, ok := decoded.(*operatorv1.CommandRequested)
	if !ok || cmdReq == nil {
		return nil
	}
	return operatorcapability.ValidateWitnessCommand(op.RuntimeConfig, cmdReq.GetCommand())
}
