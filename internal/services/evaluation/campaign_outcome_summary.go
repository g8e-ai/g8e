// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package evaluation

import (
	"fmt"
	"strings"

	evalv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/eval/v1"
)

const (
	verdictStatusPrefix     = "EVALUATION_VERDICT_STATUS_"
	lifecycleStatusPrefix   = "EVALUATION_ASSIGNMENT_LIFECYCLE_STATUS_"
	deterministicPassRateID = "deterministic_pass_rate"
)

// AssignmentOutcomeSummary renders one terminal result as the line an operator
// reads in the execution log: lifecycle, verdict, deterministic pass rate, and
// every failed grade with the cause it recorded. A lifecycle of COMPLETED only
// means the run finished; the verdict says whether the model passed.
func AssignmentOutcomeSummary(result *evalv1.EvaluationAssignmentResult) string {
	lifecycle := strings.TrimPrefix(result.GetLifecycleStatus().String(), lifecycleStatusPrefix)
	verdict := strings.TrimPrefix(DerivePublicSummaryStatus(result).String(), verdictStatusPrefix)
	var line strings.Builder
	fmt.Fprintf(&line, "%s verdict=%s", lifecycle, verdict)
	if rate, ok := DeterministicPassRate(result); ok {
		fmt.Fprintf(&line, " pass_rate=%.1f%%", rate*100)
	}
	var failed []string
	for _, grade := range result.GetDeterministicGrades() {
		if grade.GetStatus() == evalv1.EvaluationVerdictStatus_EVALUATION_VERDICT_STATUS_FAIL {
			failed = append(failed, fmt.Sprintf("%s [%s] (%s)", grade.GetCriterionId(), GradeBasisLabel(grade.GetBasis()), grade.GetDetail()))
		}
	}
	if len(failed) > 0 {
		fmt.Fprintf(&line, " failed=[%s]", strings.Join(failed, "; "))
	}
	return line.String()
}
