// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package evaluation

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCampaignModelBindingsFromFormation_OmitsDelegatedRoles(t *testing.T) {
	t.Parallel()
	formation := Formation{
		Primary: FormationModel{
			ServedModelTag: "gemini-1.5-pro",
			ModelDigest:    FormationDelegatedRegistryDigestPlaceholder,
			Trust:          FormationTrustDelegated,
		},
		Assistant: FormationModel{
			ServedModelTag: "llama3.1:8b-instruct-q4_K_M",
			ModelDigest:    repeatHex('a', 64),
			Trust:          FormationTrustSovereign,
		},
		Lite: FormationModel{
			ServedModelTag: "qwen2.5:1.5b-instruct-q4_K_M",
			ModelDigest:    repeatHex('b', 64),
			Trust:          FormationTrustSovereign,
		},
	}
	bindings := CampaignModelBindingsFromFormation(formation)
	requireLen := 2
	assert.Len(t, bindings, requireLen)
	assert.Equal(t, "llama3.1:8b-instruct-q4_K_M", bindings[0].ServedModelTag)
	assert.Equal(t, "qwen2.5:1.5b-instruct-q4_K_M", bindings[1].ServedModelTag)
}
