// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

//go:build e2e

package e2e

// e2eConfig holds resolved platform endpoints and owner credentials loaded
// from the local .g8e/ runtime tree. It is constructed once by loadE2EConfig
// and shared across all E2E test functions via the package-level e2eCfg
// variable set in TestMain.
// Note: the type is defined in config_helpers.go (non-e2e-tagged) to make it
// available to unit tests. This file only imports it for the e2e-tagged tests.