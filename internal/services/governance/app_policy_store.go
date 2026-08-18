// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package governance

import "github.com/g8e-ai/g8e/internal/models"

// AppPolicyStore defines the interface for loading AppPolicies for external apps.
type AppPolicyStore interface {
	GetAppPolicy(appID string) (*models.AppPolicy, error)
}
