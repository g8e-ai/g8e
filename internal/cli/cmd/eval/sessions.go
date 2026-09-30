// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package eval

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/g8e-ai/g8e/v2/internal/models"
	"github.com/g8e-ai/g8e/v2/internal/services/evaluation"
	"github.com/g8e-ai/g8e/v2/internal/services/operatorcapability"
)

type operatorRole string

const (
	operatorRoleInference operatorRole = "inference"
	operatorRoleData      operatorRole = "data"
)

// operatorSessions is the pair of operator sessions a dispatching command
// targets: the inference Operator and the stack's data-operator. Fields for
// roles a command did not request are empty.
type operatorSessions struct {
	InferenceSessionID string
	DataSessionID      string
	DataOperatorID     string
}

// resolveOperatorSessions asks the Gateway for active operator sessions and
// resolves each requested role. Only commands that dispatch work call it.
func resolveOperatorSessions(cmd *cobra.Command, deps nativeEvalDeps, roles ...operatorRole) (operatorSessions, error) {
	chatDeps := deps.chatDeps()
	cfg, _, authContext, err := chatEvalEnvironment(cmd, chatDeps)
	if err != nil {
		return operatorSessions{}, err
	}
	operators, err := chatEvalListOperators(cmd, chatDeps, cfg, authContext)
	if err != nil {
		return operatorSessions{}, err
	}
	return resolveOperatorSessionsFrom(operators, roles...)
}

func resolveOperatorSessionsFrom(operators []models.OperatorDocumentGo, roles ...operatorRole) (operatorSessions, error) {
	var sessions operatorSessions
	for _, role := range roles {
		switch role {
		case operatorRoleInference:
			inference, err := evaluation.SelectInferenceOperator(operators, "")
			if err != nil {
				return operatorSessions{}, err
			}
			sessions.InferenceSessionID = inference.OperatorSessionID
		case operatorRoleData:
			data, err := operatorcapability.SelectDataOperator(operators)
			if err != nil {
				return operatorSessions{}, err
			}
			sessions.DataSessionID = data.OperatorSessionID
			sessions.DataOperatorID = data.OperatorID
		default:
			return operatorSessions{}, fmt.Errorf("evaluation: unknown operator role %q", role)
		}
	}
	return sessions, nil
}
