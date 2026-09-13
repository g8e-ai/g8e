// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package auth

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/v2/internal/constants"
)

// TestValidateLocalCLI_OperatorBindingRequired verifies that bootstrap and
// recovery artifacts must carry the authoritative operator binding pair the
// gateway issued, while rotation artifacts may omit it — rotation inherits
// the persisted binding from local credentials. An empty pair fails closed
// with ErrMissingRequiredField so a CLI can never persist an unbound
// credential set.
func TestValidateLocalCLI_OperatorBindingRequired(t *testing.T) {
	tests := []struct {
		name      string
		source    EnrollmentSource
		sessionID string
		opID      string
		wantErr   bool
	}{
		{name: "bootstrap with full binding validates", source: EnrollmentSourceBootstrap, sessionID: "op-sess-1", opID: "embedded-operator"},
		{name: "bootstrap rejects empty operator session", source: EnrollmentSourceBootstrap, opID: "embedded-operator", wantErr: true},
		{name: "bootstrap rejects empty operator id", source: EnrollmentSourceBootstrap, sessionID: "op-sess-1", wantErr: true},
		{name: "recovery rejects empty operator binding", source: EnrollmentSourceRecovery, wantErr: true},
		{name: "rotation tolerates absent operator binding", source: EnrollmentSourceRotation},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			artifacts := buildTestArtifacts(t, tt.source)
			artifacts.OperatorSessionID = tt.sessionID
			artifacts.OperatorID = tt.opID

			err := validateLocalCLI(artifacts, "")
			if tt.wantErr {
				require.Error(t, err)
				assert.ErrorIs(t, err, constants.ErrMissingRequiredField)
				return
			}
			require.NoError(t, err)
		})
	}
}
