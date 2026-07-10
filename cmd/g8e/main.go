// Copyright (c) 2026 Lateralus Labs, LLC.
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

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
