// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

//go:build integration

package gateway

import (
	"context"
	"testing"
	"time"
)

func TestGatewayModeService_RenewServiceCertWithIdentity(t *testing.T) {
	ls := newTestGatewayService(t, testGatewayOpts{})

	ctx := context.Background()
	_ = ls.renewServiceCertWithIdentity(ctx)
}

func TestGatewayModeService_RunServiceCertRenewalLoop(t *testing.T) {
	ls := newTestGatewayService(t, testGatewayOpts{})

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	done := make(chan bool)
	go func() {
		ls.runServiceCertRenewalLoop(ctx)
		done <- true
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("runServiceCertRenewalLoop did not return promptly with cancelled context")
	}
}
