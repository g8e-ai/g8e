// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package storage

import (
	"testing"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestComputeAuditEventHash_IsDeterministic(t *testing.T) {
	hash1 := computeAuditEventHash(1, auditChainGenesisPrevHash, "g8e.v1.operator.audit.command", "session-1", "2026-01-01T00:00:00.000000Z", "digest", "tx-1")
	hash2 := computeAuditEventHash(1, auditChainGenesisPrevHash, "g8e.v1.operator.audit.command", "session-1", "2026-01-01T00:00:00.000000Z", "digest", "tx-1")
	assert.Equal(t, hash1, hash2)
	assert.Len(t, hash1, 64)
}

func TestComputeEventContentDigest_ChangesWithPayload(t *testing.T) {
	eventA := &Event{ContentText: "hello", CommandExitCode: constants.ExitCodeNone}
	eventB := &Event{ContentText: "world", CommandExitCode: constants.ExitCodeNone}

	digestA, err := computeEventContentDigest(eventA)
	require.NoError(t, err)
	digestB, err := computeEventContentDigest(eventB)
	require.NoError(t, err)
	assert.NotEqual(t, digestA, digestB)
}
