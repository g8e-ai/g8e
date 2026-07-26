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
