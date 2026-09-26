// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package gwremote

import "testing"

// WithGatewayHealthCheck stubs gateway publication health checks for t.
func WithGatewayHealthCheck(t *testing.T, healthy bool) {
	t.Helper()
	original := GatewayHealthCheck
	GatewayHealthCheck = func() bool { return healthy }
	t.Cleanup(func() { GatewayHealthCheck = original })
}
