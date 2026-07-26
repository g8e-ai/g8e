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

//go:build integration

package gateway

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/internal/services/storage"
)

func TestGatewayModeService_StartStop(t *testing.T) {
	ls := newTestGatewayService(t, testGatewayOpts{httpPort: 0})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	errChan := make(chan error, 1)
	go func() {
		errChan <- ls.Start(ctx)
	}()

	require.Eventually(t, func() bool {
		return ls.IsReady()
	}, 5*time.Second, 100*time.Millisecond, "service should become ready")

	httpPort := ls.GetHTTPPort()
	httpsPort := ls.GetHTTPSPort()
	assert.NotZero(t, httpPort, "HTTP port should be assigned")
	assert.NotZero(t, httpsPort, "HTTPS port should be assigned")
	assert.True(t, ls.running)

	err := ls.Start(ctx)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "already running")

	cancel()
	err = <-errChan
	assert.Error(t, err)

	stopErr := ls.Stop(context.Background())
	assert.NoError(t, stopErr)

	assert.False(t, ls.running)
	assert.False(t, ls.IsReady())
}

func TestGatewayModeService_StopWhenNotRunning(t *testing.T) {
	ls := newTestGatewayService(t, testGatewayOpts{})

	err := ls.Stop(context.Background())
	assert.NoError(t, err)
}

func TestGatewayModeService_SuspendedTxServiceSingleField(t *testing.T) {
	ls := newTestGatewayService(t, testGatewayOpts{})

	t.Run("suspendedTxService is non-nil after construction", func(t *testing.T) {
		assert.NotNil(t, ls.suspendedTxService)
	})

	t.Run("suspendedTxService satisfies SuspendedTransactionStore interface", func(t *testing.T) {
		var _ storage.SuspendedTransactionStore = ls.suspendedTxService
	})

	t.Run("Stop when not running closes suspendedTxService without error", func(t *testing.T) {
		err := ls.Stop(context.Background())
		assert.NoError(t, err)
	})

	t.Run("Stop when not running is idempotent for suspendedTxService", func(t *testing.T) {
		err := ls.Stop(context.Background())
		assert.NoError(t, err)
	})
}
