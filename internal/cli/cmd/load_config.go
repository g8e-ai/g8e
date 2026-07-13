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

package cmd

import (
	"fmt"

	"github.com/g8e-ai/g8e/internal/cli/config"
	"github.com/g8e-ai/g8e/internal/constants"
	"github.com/g8e-ai/g8e/internal/services/fs"
)

// configLoad is the underlying config loader function. It is a package-level
// variable so tests can swap it for a mock that returns a controlled error.
var configLoad = config.Load

// loadConfig is a shared helper that wraps configLoad with the standard
// ErrConfigLoadFailed error wrapping. It is used by all CLI commands that
// need a *config.Config, eliminating the repeated 4-line boilerplate.
//
// The signature accepts a projectRoot string (matching config.Load) so it
// can also be passed as the default configLoader in injectable command
// constructors (e.g. dataUsersCmdWithConfig(loadConfig, ...)).
func loadConfig(projectRoot string) (*config.Config, error) {
	cfg, err := configLoad(projectRoot)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", constants.ErrConfigLoadFailed, err)
	}
	return cfg, nil
}

// relFromAbs converts an absolute path within .g8e/ to a relative path
// suitable for fileSvc methods. Panics if the path is outside the runtime
// directory — all config-derived paths are expected to be within .g8e/.
func relFromAbs(fileSvc fs.RuntimeFileService, absPath string) string {
	rel, err := fileSvc.Rel(absPath)
	if err != nil {
		panic(fmt.Sprintf("relFromAbs: %s is not within .g8e/ runtime dir: %v", absPath, err))
	}
	return rel
}
