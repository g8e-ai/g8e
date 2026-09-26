// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package evaluation

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCampaignMirrorGatewayUnreachable(t *testing.T) {
	assert.True(t, campaignMirrorGatewayUnreachable(errors.New("read tcp 127.0.0.1:1->127.0.0.1:8443: read: connection reset by peer")))
	assert.True(t, campaignMirrorGatewayUnreachable(errors.New("failed to execute HTTP request: EOF")))
	assert.False(t, campaignMirrorGatewayUnreachable(errors.New("public-feed: mirror rejected proof package")))
}
