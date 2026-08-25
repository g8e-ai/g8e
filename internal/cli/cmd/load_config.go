// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package cmd

import (
	"fmt"

	"github.com/g8e-ai/g8e/v2/internal/cli/config"
	"github.com/g8e-ai/g8e/v2/internal/constants"
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
