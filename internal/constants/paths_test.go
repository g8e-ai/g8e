// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package constants

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestContainerAuditVaultDB_PointsToG8eDB is a regression test for the
// FedRAMP Scenario 4 audit vault DB path mismatch. The gateway's audit
// store uses DefaultAuditStoreConfig() which sets DBPath to DbFilename
// ("g8e.db"), so the container verification path must point to the same
// file. The prior constant used AuditVaultDBFilename ("audit_vault.db"),
// which does not exist on the gateway or operator container, causing the
// independent post-rejection verification step to fail.
func TestContainerAuditVaultDB_PointsToG8eDB(t *testing.T) {
	t.Parallel()

	assert.True(t, strings.HasSuffix(ContainerAuditVaultDB, DbFilename),
		"ContainerAuditVaultDB must point to %s (the actual audit vault DB file), got %s",
		DbFilename, ContainerAuditVaultDB)
	assert.False(t, strings.HasSuffix(ContainerAuditVaultDB, AuditVaultDBFilename),
		"ContainerAuditVaultDB must not point to %s (non-existent file), got %s",
		AuditVaultDBFilename, ContainerAuditVaultDB)
}
