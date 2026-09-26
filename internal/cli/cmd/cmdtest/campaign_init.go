// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package cmdtest

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/v2/internal/services/evaluation"
	evalv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/eval/v1"
)

// EvaluationTestCampaignInitRequest builds the north-star smoke campaign request
// used by eval and compliance command tests.
func EvaluationTestCampaignInitRequest(t *testing.T) evaluation.CampaignInitRequest {
	t.Helper()
	catalog, artifacts, err := evaluation.LoadScenarioCatalog()
	require.NoError(t, err)
	inventory, err := evaluation.MaterializeModelRegistry("north-star-smoke", []*evalv1.ModelVariant{
		{
			VariantId:      "qwen3-4b",
			ServedModelTag: "qwen3:4b",
			ModelDigest:    "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
			ProviderClass:  "ollama",
		},
	})
	require.NoError(t, err)
	return evaluation.CampaignInitRequest{
		CampaignID:                 "north-star-smoke",
		RunID:                      "run-smoke-1",
		Catalog:                    catalog,
		Inventory:                  inventory,
		ScenarioArtifacts:          artifacts,
		RepetitionCount:            1,
		InferenceOperatorSessionID: "inf-session",
		DataOperatorSessionID:      "data-session",
	}
}
