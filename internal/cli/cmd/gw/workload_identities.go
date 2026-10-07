// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package gw

import (
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/g8e-ai/g8e/v2/internal/constants"
)

// Only explicitly recorded workloads and fixed launcher defaults are inspected.
// This is advisory: gateway cleanup never changes workload directories.
func reportWorkloadIdentities(cmd *cobra.Command) {
	var workloads []struct {
		Role      string `json:"role"`
		Directory string `json:"directory"`
	}
	if data, err := os.ReadFile(".local.dev/full/workloads.json"); err == nil {
		if err := json.Unmarshal(data, &workloads); err != nil {
			cmd.PrintErrln("Could not read local workload registry; workload identities may require reset after gateway cleanup.")
		}
	}
	home, _ := os.UserHomeDir()
	for _, role := range []string{"provenance", "observer", "inference", "data", "g8ee"} {
		dir := filepath.Join(home, ".ollama/g8e", role)
		if role == "g8ee" {
			dir, _ = filepath.Abs(".local.dev/full/ensemble")
		}
		workloads = append(workloads, struct {
			Role      string `json:"role"`
			Directory string `json:"directory"`
		}{role, dir})
	}
	seen := map[string]bool{}
	printed := false
	for _, workload := range workloads {
		if !filepath.IsAbs(workload.Directory) || seen[workload.Directory] {
			continue
		}
		switch workload.Role {
		case "provenance", "observer", "inference", "data", "g8ee":
		default:
			continue
		}
		seen[workload.Directory] = true
		if _, err := os.Stat(filepath.Join(workload.Directory, constants.RuntimeDirname, constants.PkiDirname, constants.PkiSubdirTrust, constants.PkiFileGatewayBundle)); err != nil {
			continue
		}
		if !printed {
			cmd.Println("Local workload identities will require reset and fresh enrollment after the gateway starts over:")
			printed = true
		}
		cmd.Printf("  %s: %s\n", workload.Role, workload.Directory)
	}
	if printed {
		cmd.Println("Recover with make full RESET_IDENTITIES=1 or interactive make full-setup; operator/ensemble reset-identity can reset individual workloads. Working data is preserved.")
	}
}
