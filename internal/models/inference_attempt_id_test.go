// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package models

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/g8e-ai/g8e/v2/internal/constants"
)

func TestValidateProviderAttemptID(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		id      string
		wantErr error
	}{
		{name: "uuid with prefix", id: "attempt-3f2b8c1e-9d4a-4b7e-8a11-0c5d6e7f8091"},
		{name: "formation role id", id: "qwen-powerhouse-primary_1.retry"},
		{name: "max length", id: strings.Repeat("a", maxProviderAttemptIDBytes)},
		{name: "empty", id: "", wantErr: constants.ErrInferenceProviderAttemptRequired},
		{name: "over max length", id: strings.Repeat("a", maxProviderAttemptIDBytes+1), wantErr: constants.ErrInferenceProviderAttemptIDInvalid},
		{name: "dot", id: ".", wantErr: constants.ErrInferenceProviderAttemptIDInvalid},
		{name: "dot dot", id: "..", wantErr: constants.ErrInferenceProviderAttemptIDInvalid},
		{name: "forward slash", id: "a/b", wantErr: constants.ErrInferenceProviderAttemptIDInvalid},
		{name: "backslash", id: `a\b`, wantErr: constants.ErrInferenceProviderAttemptIDInvalid},
		{name: "traversal", id: "../escape", wantErr: constants.ErrInferenceProviderAttemptIDInvalid},
		{name: "nul byte", id: "a\x00b", wantErr: constants.ErrInferenceProviderAttemptIDInvalid},
		{name: "colon", id: "a:b", wantErr: constants.ErrInferenceProviderAttemptIDInvalid},
		{name: "space", id: "a b", wantErr: constants.ErrInferenceProviderAttemptIDInvalid},
		{name: "non ascii letter", id: "attempt-é", wantErr: constants.ErrInferenceProviderAttemptIDInvalid},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			err := ValidateProviderAttemptID(test.id)
			if test.wantErr == nil {
				assert.NoError(t, err)
				return
			}
			assert.ErrorIs(t, err, test.wantErr)
		})
	}
}
