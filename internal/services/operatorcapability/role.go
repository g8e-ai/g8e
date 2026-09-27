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

// ResolveOperatorRole determines the primary operational role from runtime configuration.
// Inference, provenance, observer, and data operators are separated by their role and
// responsibilities triggered by the startup flags.
func ResolveOperatorRole(cfg *models.RuntimeConfig) constants.OperatorRole {
	if cfg == nil {
		return constants.OperatorRoleData
	}
	if cfg.Role != "" {
		return cfg.Role
	}
	if cfg.InferenceEnabled {
		return constants.OperatorRoleInference
	}
	if cfg.ProvenanceOperatorEnabled {
		return constants.OperatorRoleProvenance
	}
	if cfg.ProviderBoundaryObserverEnabled {
		return constants.OperatorRoleObserver
	}
	return constants.OperatorRoleData
}

// GetOperatorRole returns the operational role of an operator document.
func GetOperatorRole(op models.OperatorDocumentGo) constants.OperatorRole {
	if op.OperatorRole != "" {
		return op.OperatorRole
	}
	return ResolveOperatorRole(op.RuntimeConfig)
}

// RoleResponsibilities returns a human-readable summary of the role's responsibilities.
func RoleResponsibilities(role constants.OperatorRole) string {
	switch role {
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
	actualRole := GetOperatorRole(op)
	if actualRole != requiredRole {
		return fmt.Errorf("%w: operator %s has role %q (%s), but required role is %q (%s)",
			constants.ErrWitnessCommandNotCapable,
			op.ID,
			actualRole,
			RoleResponsibilities(actualRole),
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

	role1 := GetOperatorRole(op1)
	role2 := GetOperatorRole(op2)
	if role1 != role2 {
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
