// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package cmdtest

const (
	// RegressionMarkerAfterFix indicates the expected behavior after a fix is implemented.
	RegressionMarkerAfterFix = "REGRESSION: AFTER FIX"
	// RegressionMarkerBeforeFix indicates the current (broken) behavior before a fix.
	RegressionMarkerBeforeFix = "REGRESSION: BEFORE FIX"
	// RegressionMarkerIssue identifies a specific issue being tracked.
	RegressionMarkerIssue = "REGRESSION: ISSUE"
)
