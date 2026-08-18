// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package main

import (
	clicmd "github.com/g8e-ai/g8e/internal/cli/cmd"
	"github.com/g8e-ai/g8e/internal/constants"
)

// Version information (set via ldflags during build)
var (
	version   string = string(constants.VersionStabilityDev)
	buildID   string = string(constants.SystemHealthUnknown)
	buildTime string = string(constants.SystemHealthUnknown)
	platform  string = string(constants.SystemHealthUnknown)
)

func main() {
	clicmd.ExecuteWithVersionInfo(version, buildID, buildTime, platform)
}
