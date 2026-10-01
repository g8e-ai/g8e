// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package evaluation

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestBuildCampaignChatRequest_MessageIncludesInlineContext guards against the
// homogeneous-lane message builder silently dropping InlineContext (as it did
// when the field was named Attachments but was never read): a scenario whose
// prompt says content is provided "below" must actually carry that content in
// the message sent to the model.
func TestBuildCampaignChatRequest_MessageIncludesInlineContext(t *testing.T) {
	t.Parallel()
	req := homogeneousAssignmentExecutionRequest(t, "primary")
	req.ScenarioInput.UserPrompt = "Identify the failing service named in the synthetic log excerpt below."
	req.ScenarioInput.InlineContext = []ScenarioInlineContent{
		syntheticInlineContent("log", "synthetic-service-log", "2026-09-16T08:05:11Z ERROR service=checkout-api upstream=payments.internal.example reason=timeout"),
	}

	chatReq, err := BuildCampaignChatRequest(req.Assignment, req.AttemptID, req.ScenarioInput, req.Binding, CampaignChatGradingContext{GradingMethod: req.GradingMethod})
	require.NoError(t, err)

	assert.True(t, strings.HasPrefix(chatReq.Message, req.ScenarioInput.UserPrompt))
	assert.Contains(t, chatReq.Message, "service=checkout-api")
	assert.Contains(t, chatReq.Message, "synthetic-service-log")
}

func TestBuildCampaignChatRequest_MessageIsBarePromptWithoutInlineContext(t *testing.T) {
	t.Parallel()
	req := homogeneousAssignmentExecutionRequest(t, "primary")

	chatReq, err := BuildCampaignChatRequest(req.Assignment, req.AttemptID, req.ScenarioInput, req.Binding, CampaignChatGradingContext{GradingMethod: req.GradingMethod})
	require.NoError(t, err)

	assert.Equal(t, req.ScenarioInput.UserPrompt, chatReq.Message)
}
