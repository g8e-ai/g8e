// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package operatorcapability

import (
	"fmt"

	"google.golang.org/protobuf/encoding/protojson"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/models"
	operatorv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/operator/v1"
)

// DataOperatorStatus summarizes the active data-operator session discovered
// through the operator registry.
type DataOperatorStatus struct {
	OperatorID        string
	OperatorSessionID string
	Status            string
	WorkingDirectory  string
}

// IsDataOperator reports whether an active remote or configured embedded
// session includes Data, even when other roles are enabled.
func IsDataOperator(op models.OperatorDocumentGo) bool {
	return HasActiveRole(op, constants.OperatorRoleData)
}

// IsDedicatedDataOperator reports whether op is an active Data session other
// than the Gateway's embedded Operator. Selection prefers dedicated Data
// Operators and falls back to the embedded one only when none is active.
func IsDedicatedDataOperator(op models.OperatorDocumentGo) bool {
	return IsDataOperator(op) && op.OperatorType != constants.OperatorTypeEmbedded
}

// PreferDedicatedDataOperators drops the Gateway's embedded Operator when a
// dedicated Data session is also present.
func PreferDedicatedDataOperators(operators []models.OperatorDocumentGo) []models.OperatorDocumentGo {
	dedicated := make([]models.OperatorDocumentGo, 0, len(operators))
	for _, op := range operators {
		if IsDedicatedDataOperator(op) {
			dedicated = append(dedicated, op)
		}
	}
	if len(dedicated) == 0 {
		return operators
	}
	return dedicated
}

// IsStackDataOperator reports whether op is the data-operator the unified
// Docker stack launches: an active data Operator whose heartbeat hostname is
// constants.DataOperatorHostname.
func IsStackDataOperator(op models.OperatorDocumentGo) bool {
	return IsDataOperator(op) && op.CurrentHostname == constants.DataOperatorHostname
}

// ActiveDataOperators returns the active data-operator sessions in preference
// order: stack data-operators (constants.DataOperatorHostname), else dedicated
// Data Operators (for example, in host-native local development), else the
// Gateway's embedded Operator.
func ActiveDataOperators(operators []models.OperatorDocumentGo) []DataOperatorStatus {
	var stackMatches []DataOperatorStatus
	var dedicatedMatches []DataOperatorStatus
	var embeddedMatches []DataOperatorStatus
	for _, op := range operators {
		if !IsDataOperator(op) {
			continue
		}
		wd := extractWorkingDirectory(op.LatestHeartbeat)
		status := DataOperatorStatus{
			OperatorID:        op.ID,
			OperatorSessionID: op.OperatorSessionID,
			Status:            string(op.Status),
			WorkingDirectory:  wd,
		}
		switch {
		case op.CurrentHostname == constants.DataOperatorHostname:
			stackMatches = append(stackMatches, status)
		case op.OperatorType == constants.OperatorTypeEmbedded:
			embeddedMatches = append(embeddedMatches, status)
		default:
			dedicatedMatches = append(dedicatedMatches, status)
		}
	}
	switch {
	case len(stackMatches) > 0:
		return stackMatches
	case len(dedicatedMatches) > 0:
		return dedicatedMatches
	default:
		return embeddedMatches
	}
}

// extractWorkingDirectory reads the pwd from the operator's heartbeat.
// It tries EnvironmentDetails.pwd first, then falls back to SystemIdentity.pwd.
// Returns empty string if the heartbeat cannot be unmarshalled or pwd is absent.
func extractWorkingDirectory(hb []byte) string {
	if len(hb) == 0 {
		return ""
	}
	hr := &operatorv1.HeartbeatResult{}
	if err := protojson.Unmarshal(hb, hr); err != nil {
		return ""
	}
	// Try EnvironmentDetails first
	if env := hr.GetEnvironment(); env != nil && env.GetPwd() != "" {
		return env.GetPwd()
	}
	// Fall back to SystemIdentity
	if si := hr.GetSystemIdentity(); si != nil && si.GetPwd() != "" {
		return si.GetPwd()
	}
	return ""
}

// SelectDataOperator resolves exactly one data-operator. Zero sessions return
// ErrDataOperatorNotFound and several return ErrDataOperatorAmbiguous.
func SelectDataOperator(operators []models.OperatorDocumentGo) (*DataOperatorStatus, error) {
	matches := ActiveDataOperators(operators)
	switch len(matches) {
	case 0:
		return nil, constants.ErrDataOperatorNotFound
	case 1:
		selected := matches[0]
		return &selected, nil
	default:
		return nil, fmt.Errorf("%w: %d sessions", constants.ErrDataOperatorAmbiguous, len(matches))
	}
}
