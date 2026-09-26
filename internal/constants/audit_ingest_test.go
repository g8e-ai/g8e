// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package constants

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidateAuditRecordRequest(t *testing.T) {
	recorded, err := ValidateAuditRecordRequest(EventOperatorAuditDirectCommandRecordRequested)
	require.NoError(t, err)
	assert.Equal(t, EventOperatorAuditDirectCommandRecorded, recorded)

	_, err = ValidateAuditRecordRequest(EventOperatorCommandRequested)
	assert.ErrorIs(t, err, ErrAuditIngestInvalidRequest)
}
