// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package lattice

import (
	"time"

	latticeconfig "github.com/g8e-ai/g8e/v2/internal/adapters/lattice/config"
)

// LatticeConfig is an alias for latticeconfig.LatticeConfig, re-exported so
// existing callers of the lattice package continue to work without change.
// The canonical definition lives in the config sub-package to break an import
// cycle: config → lattice → fs would cycle if lattice owned the config types.
type LatticeConfig = latticeconfig.LatticeConfig

// EntityConfig is an alias for latticeconfig.EntityConfig.
type EntityConfig = latticeconfig.EntityConfig

// ValidateHeartbeatInterval wraps latticeconfig.ValidateHeartbeatInterval so
// callers importing the lattice package can use it without a separate import.
func ValidateHeartbeatInterval(interval time.Duration) error {
	return latticeconfig.ValidateHeartbeatInterval(interval)
}
