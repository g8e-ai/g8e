// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package pubsub

import (
	"context"
	"fmt"
	"testing"
	"time"
)

func TestListenForCommands_ContextCancellationExitsDuringOutage(t *testing.T) {
	t.Parallel()
	f := newPubsubFixture(t)

	client := newOutageClient(1<<30, fmt.Errorf("connection refused"))
	f.Svc.client = client
	f.Svc.reconnectBaseDelay = time.Millisecond

	ctx, cancel := context.WithCancel(context.Background())
	f.Svc.ctx = ctx

	done := make(chan struct{})
	go func() {
		f.Svc.listenForCommands("test-channel")
		close(done)
	}()

	cancel()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("listenForCommands did not exit after context cancellation")
	}
}
