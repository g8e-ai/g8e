// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package evaluation

import (
	"context"
	"fmt"
	"time"
)

const campaignTracePollInterval = 500 * time.Millisecond

// WaitForCampaignTrace polls one evaluation trace until it reaches a terminal
// status. onProgress is invoked after each non-terminal fetch so publication can
// emit mid-run live events when scored model telemetry first appears.
func WaitForCampaignTrace(
	ctx context.Context,
	fetch func(context.Context) (EvaluationTrace, error),
	onProgress func(context.Context, EvaluationTrace) error,
) (EvaluationTrace, error) {
	ticker := time.NewTicker(campaignTracePollInterval)
	defer ticker.Stop()
	for {
		trace, err := fetch(ctx)
		if err == nil {
			status, _ := trace["status"].(string)
			if status == "completed" || status == "failed" {
				return trace, nil
			}
			if onProgress != nil {
				if progressErr := onProgress(ctx, trace); progressErr != nil {
					return nil, fmt.Errorf("evaluation: wait for campaign trace: progress hook: %w", progressErr)
				}
			}
		}
		select {
		case <-ctx.Done():
			if err != nil {
				return nil, fmt.Errorf("evaluation: wait for campaign trace: %w", err)
			}
			return nil, fmt.Errorf("evaluation: wait for campaign trace: %w", ctx.Err())
		case <-ticker.C:
		}
	}
}
