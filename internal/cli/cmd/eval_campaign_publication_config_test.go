// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package cmd

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNewCampaignPublicationCoordinatorFromConfig(t *testing.T) {
	withGatewayHealthCheck(t, true)
	root, deps, _, cleanup := setupCampaignPublishGatewayEnv(t)
	defer cleanup()

	fileSvc, err := deps.fileSvcFactory(root, nil)
	require.NoError(t, err)
	cfg, err := deps.configLoader(root)
	require.NoError(t, err)

	coordinator, err := newCampaignPublicationCoordinatorFromConfig(context.Background(), fileSvc, cfg)
	require.NoError(t, err)
	require.NotNil(t, coordinator)
}
