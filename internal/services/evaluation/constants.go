// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package evaluation

const (
	// heterogeneousPipelineDescription describes the heterogeneous-pipeline
	// criterion: all three roles completed and each role received prior role
	// outputs as handoff evidence.
	heterogeneousPipelineDescription = "Heterogeneous lane executes all three roles (Lite → Assistant → Primary) with verified handoff of prior role outputs at each step."

	// heterogeneousPipelineHandoffVerifiedDetail is the detail message when
	// heterogeneous pipeline grading passes with verified handoff.
	heterogeneousPipelineHandoffVerifiedDetail = "heterogeneous pipeline completed Lite → Assistant → Primary with verified role handoff evidence"

	// heterogeneousPipelineHandoffIncompleteDetail is the detail message when
	// handoff chain fails: a downstream role did not receive prior outputs.
	heterogeneousPipelineHandoffIncompleteDetail = "heterogeneous pipeline roles invoked but handoff chain incomplete: a downstream role did not receive prior role outputs"

	// heterogeneousPipelineNotCompletedDetail is the detail message when the
	// pipeline did not complete all three role invocations.
	heterogeneousPipelineNotCompletedDetail = "heterogeneous pipeline did not complete with all three role handoffs"
)
