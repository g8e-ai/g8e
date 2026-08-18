// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

//go:build integration
// +build integration

package cmd

import (
	"testing"
)

// Integration test placeholder for when gateway is running
func TestAuditReceiptsCmd_Integration(t *testing.T) {
	t.Run("receipts with running gateway", func(t *testing.T) {
		t.Skip("Integration test requiring running gateway - test with ./g8e test e2e")
	})
}

func TestAuditExportCmd_Integration(t *testing.T) {
	t.Run("export with running gateway", func(t *testing.T) {
		t.Skip("Integration test requiring running gateway - test with ./g8e test e2e")
	})
}

func TestAuditReportCmd_Integration(t *testing.T) {
	t.Run("report with running gateway", func(t *testing.T) {
		t.Skip("Integration test requiring running gateway - test with ./g8e test e2e")
	})
}

func TestAuditEventsCmd_Integration(t *testing.T) {
	t.Run("events with running gateway", func(t *testing.T) {
		t.Skip("Integration test requiring running gateway - test with ./g8e test e2e")
	})
}

func TestAuditSummaryCmd_Integration(t *testing.T) {
	t.Run("summary with running gateway", func(t *testing.T) {
		t.Skip("Integration test requiring running gateway - test with ./g8e test e2e")
	})
}
