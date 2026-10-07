// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package operatorcapability

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/models"
)

// ResolveOperatorRoles returns every enabled role. Ordinary operators default to Data.
func ResolveOperatorRoles(cfg *models.RuntimeConfig) constants.OperatorRoles {
	if cfg == nil {
		return constants.OperatorRoles{constants.OperatorRoleData}
	}
	roles := append(constants.OperatorRoles{}, cfg.Roles...)
	if cfg.InferenceEnabled {
		roles = append(roles, constants.OperatorRoleInference)
	}
	if cfg.ProvenanceOperatorEnabled {
		roles = append(roles, constants.OperatorRoleProvenance)
	}
	if cfg.ProviderBoundaryObserverEnabled {
		roles = append(roles, constants.OperatorRoleObserver)
	}
	if len(roles) == 0 {
		roles = append(roles, constants.OperatorRoleData)
	}
	return roles.Canonical()
}

// GetOperatorRoles resolves runtime capabilities; stored role metadata is used only without runtime configuration.
func GetOperatorRoles(op models.OperatorDocumentGo) constants.OperatorRoles {
	var roles constants.OperatorRoles
	if op.RuntimeConfig != nil {
		roles = ResolveOperatorRoles(op.RuntimeConfig)
	} else if len(op.OperatorRoles) > 0 {
		roles = op.OperatorRoles.Canonical()
	} else {
		roles = constants.OperatorRoles{constants.OperatorRoleData}
	}
	if op.OperatorType == constants.OperatorTypeEmbedded {
		roles = append(roles, constants.OperatorRoleEmbedded).Canonical()
	}
	return roles
}

// IsActiveOperatorSession reports whether op is a live, session-bound Operator
// that may serve governed work: an active remote Operator, or an active
// embedded Operator that has reported its runtime configuration.
func IsActiveOperatorSession(op models.OperatorDocumentGo) bool {
	if op.Status != constants.OperatorStatusActive || op.OperatorSessionID == "" {
		return false
	}
	switch op.OperatorType {
	case constants.OperatorTypeRemote:
		return true
	case constants.OperatorTypeEmbedded:
		return op.RuntimeConfig != nil
	default:
		return false
	}
}

// HasActiveRole reports whether op is an active Operator session whose
// resolved roles include role.
func HasActiveRole(op models.OperatorDocumentGo, role constants.OperatorRole) bool {
	return IsActiveOperatorSession(op) && GetOperatorRoles(op).Has(role)
}

// RoleResponsibilities returns a human-readable summary of the role's responsibilities.
func RoleResponsibilities(role constants.OperatorRole) string {
	switch role {
	case constants.OperatorRoleEmbedded:
		return "Gateway in-process operator substrate"
	case constants.OperatorRoleInference:
		return "Governed model inference backend (g8ellama), model registry and lifecycle management"
	case constants.OperatorRoleProvenance:
		return "Storage-side model provenance attestation over local model weights (read-only witness)"
	case constants.OperatorRoleObserver:
		return "Provider-boundary hardware and telemetry observation on approved provider host (read-only witness)"
	case constants.OperatorRoleData:
		return "Governed tool and command execution, execution vault, and host data triage"
	default:
		return "Standard governed operator operations"
	}
}

// ValidateOperatorRoleCapabilities verifies that an operator document possesses the required role.
func ValidateOperatorRoleCapabilities(op models.OperatorDocumentGo, requiredRole constants.OperatorRole) error {
	actualRole := GetOperatorRoles(op)
	if !actualRole.Has(requiredRole) {
		return fmt.Errorf("%w: operator %s has role %q (%s), but required role is %q (%s)",
			constants.ErrWitnessCommandNotCapable,
			op.ID,
			actualRole,
			actualRole.String(),
			requiredRole,
			RoleResponsibilities(requiredRole),
		)
	}
	return nil
}

// VerifyOperatorSeparation verifies whether two operator instances running on the same host
// are cleanly separated by their local directory, account, port, role, or fingerprint.
// Returns (true, reason) if properly separated, or (false, reason) if colliding.
func VerifyOperatorSeparation(op1, op2 models.OperatorDocumentGo) (bool, string) {
	// If different IDs and different session IDs
	if op1.ID != "" && op2.ID != "" && op1.ID == op2.ID {
		return false, fmt.Sprintf("colliding operator ID: %s", op1.ID)
	}

	role1 := GetOperatorRoles(op1)
	role2 := GetOperatorRoles(op2)
	if role1.String() != role2.String() {
		return true, fmt.Sprintf("separated by role: %s vs %s", role1, role2)
	}

	// Compare local dirs if both are provided
	dir1 := cleanDir(op1)
	dir2 := cleanDir(op2)
	if dir1 != "" && dir2 != "" && dir1 != dir2 {
		return true, fmt.Sprintf("separated by local directory: %s vs %s", dir1, dir2)
	}

	// Compare ports if both are provided
	port1 := opPort(op1)
	port2 := opPort(op2)
	if port1 > 0 && port2 > 0 && port1 != port2 {
		return true, fmt.Sprintf("separated by port: %d vs %d", port1, port2)
	}

	// Compare account if both are provided
	account1 := opAccount(op1)
	account2 := opAccount(op2)
	if account1 != "" && account2 != "" && account1 != account2 {
		return true, fmt.Sprintf("separated by launching account: %s vs %s", account1, account2)
	}

	// Compare fingerprints
	if op1.SystemFingerprint != "" && op2.SystemFingerprint != "" && op1.SystemFingerprint != op2.SystemFingerprint {
		return true, fmt.Sprintf("separated by unique system fingerprint: %s vs %s", op1.SystemFingerprint[:8], op2.SystemFingerprint[:8])
	}

	return false, "operators on same system share identical role, directory, account, port, and fingerprint"
}

func cleanDir(op models.OperatorDocumentGo) string {
	if op.LocalDir != "" {
		return filepath.Clean(op.LocalDir)
	}
	if op.RuntimeConfig != nil && op.RuntimeConfig.LocalDir != "" {
		return filepath.Clean(op.RuntimeConfig.LocalDir)
	}
	return ""
}

func opPort(op models.OperatorDocumentGo) int {
	if op.Port > 0 {
		return op.Port
	}
	if op.RuntimeConfig != nil && op.RuntimeConfig.HTTPPort > 0 {
		return op.RuntimeConfig.HTTPPort
	}
	return 0
}

func opAccount(op models.OperatorDocumentGo) string {
	if op.Account != "" {
		return strings.TrimSpace(op.Account)
	}
	if op.RuntimeConfig != nil && op.RuntimeConfig.Account != "" {
		return strings.TrimSpace(op.RuntimeConfig.Account)
	}
	return ""
}
