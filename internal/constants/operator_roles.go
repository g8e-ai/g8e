// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package constants

import (
	"encoding/json"
	"fmt"
	"slices"
	"strings"
)

// OperatorRoles is the complete set of capabilities enabled in one runtime.
// Embedded selects the Gateway in-process runtime; all other capabilities compose independently.
type OperatorRoles []OperatorRole

func (r OperatorRoles) Has(role OperatorRole) bool { return slices.Contains(r, role) }

func (r OperatorRoles) Validate() error {
	for _, role := range r {
		switch role {
		case OperatorRoleEmbedded, OperatorRoleData, OperatorRoleInference, OperatorRoleProvenance, OperatorRoleObserver:
		default:
			return fmt.Errorf("%w: operator role %q", ErrOperatorRoleInvalid, role)
		}
	}
	return nil
}

// Canonical returns a deduplicated set in stable order for identity and display.
func (r OperatorRoles) Canonical() OperatorRoles {
	result := OperatorRoles{}
	for _, role := range []OperatorRole{OperatorRoleEmbedded, OperatorRoleData, OperatorRoleInference, OperatorRoleProvenance, OperatorRoleObserver} {
		if r.Has(role) {
			result = append(result, role)
		}
	}
	return result
}

// Effective returns the capabilities a runtime actually carries, in canonical
// order. Embedded is the Gateway's own Data operator, so it always includes
// Data; every other capability composes independently.
func (r OperatorRoles) Effective() OperatorRoles {
	roles := append(OperatorRoles{}, r...)
	if roles.Has(OperatorRoleEmbedded) {
		roles = append(roles, OperatorRoleData)
	}
	return roles.Canonical()
}

func (r OperatorRoles) String() string {
	parts := make([]string, 0, len(r))
	for _, role := range r.Canonical() {
		parts = append(parts, string(role))
	}
	return strings.Join(parts, ",")
}

// Set implements pflag.Value and rejects unknown capabilities at the CLI boundary.
func (r *OperatorRoles) Set(value string) error {
	roles := OperatorRoles{}
	for _, part := range strings.Split(value, ",") {
		roles = append(roles, OperatorRole(strings.TrimSpace(part)))
	}
	if err := roles.Validate(); err != nil {
		return err
	}
	*r = append(*r, roles...).Canonical()
	return nil
}
func (r OperatorRoles) Type() string { return "operator-roles" }

func (r *OperatorRoles) UnmarshalJSON(data []byte) error {
	var roles []OperatorRole
	if err := json.Unmarshal(data, &roles); err != nil {
		return err
	}
	if err := OperatorRoles(roles).Validate(); err != nil {
		return err
	}
	*r = OperatorRoles(roles).Canonical()
	return nil
}
