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
)

// loadConfig is a shared helper that wraps config.Load with the standard
// ErrConfigLoadFailed error wrapping. It is used by all CLI commands that
// need a *config.Config, eliminating the repeated 4-line boilerplate.
//
// The signature accepts a projectRoot string (matching config.Load) so it
// can also be passed as the default configLoader in injectable command
// constructors (e.g. dataUsersCmdWithConfig(loadConfig, ...)).
func loadConfig(projectRoot string) (*config.Config, error) {
	cfg, err := config.Load(projectRoot)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", constants.ErrConfigLoadFailed, err)
	}
	return cfg, nil
}
