// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

// Phase 0 route auth classification inventory for the
// platform-activation-approval-sequence plan.
//
// These tests document the CURRENT route auth classifications of every
// certificate-issuance path so the Phase 3 router-level changes can flip
// them to the secure classifications and prove the change with the same
// assertions. The tests are Tier 1 (no infra) because the RouteAuthRegistry
// is a pure in-memory classifier.
//
// Inventory of certificate-issuing routes and their current classifications:
//
//   Route                                 Current Mode   Secure Mode (Phase 3+)
//   /api/v1/auth/bootstrap                RouteAuthNone  RouteAuthNone (creates first user; no platform cert)
//   /api/v1/auth/operator/enroll          RouteAuthNone  REMOVED (Phase 4)
//   /api/v1/pki/apps/enroll               RouteAuthNone  REMOVED or RouteAuthMTLS (Phase 4)
//   /api/v1/pki/apps/delegated            RouteAuthMTLS  RouteAuthMTLS (retained; short-lived, authenticated)
//   /api/v1/pki/csr/sign                  RouteAuthNone  RouteAuthMTLS with leaf-type authz (Phase 4)
//   /api/v1/pki/devices/enroll            RouteAuthNone  RouteAuthMTLS (Phase 4; mTLS already enforced in handler)
//
// New platform enrollment routes (Phase 3):
//   /api/v1/auth/platform-enrollments/request     RouteAuthNone (token-scoped, like CLI recovery)
//   /api/v1/auth/platform-enrollments/status      RouteAuthNone (token-scoped)
//   /api/v1/auth/platform-enrollments/complete    RouteAuthNone (token-scoped + proof-of-possession)
//   /api/v1/auth/platform-enrollments/pending     RouteAuthDual (owner: web session or mTLS)
//   /api/v1/auth/platform-enrollments/decision    RouteAuthDual (owner: web session or mTLS)

package gateway

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/g8e-ai/g8e/internal/constants"
)

// TestPlatformEnrollmentBypassRouteAuth_CurrentClassifications documents
// the current route auth registry classifications of the certificate
// issuance paths. The bypass is NOT in the registry — it is in the plain
// HTTP router (buildHTTPRouter), which registers these handlers without
// applying the auth middleware at all. On HTTPS (buildPublicRouter), the
// fail-closed RouteAuthMTLS default blocks unauthenticated access to
// routes that are not explicitly classified as RouteAuthNone. The plain
// HTTP router has no such protection.
//
// After Phase 3/4 these assertions will be updated to reflect the secure
// classifications and the plain HTTP router will no longer register the
// bypass routes.
func TestPlatformEnrollmentBypassRouteAuth_CurrentClassifications(t *testing.T) {
	registry := NewRouteAuthRegistry(false)

	// /api/v1/auth/operator/enroll is NOT explicitly classified — it
	// falls through to the fail-closed RouteAuthMTLS default in the
	// registry. The bypass is that buildHTTPRouter registers the handler
	// on plain HTTP without the auth middleware. Phase 4 removes the
	// route entirely.
	assert.Equal(t, RouteAuthMTLS, registry.AuthMode(constants.APIPaths.AuthOperatorEnroll),
		"Phase 0: operator enrollment is RouteAuthMTLS in the registry (fail-closed default); the bypass is the plain HTTP router registration")

	// /api/v1/pki/apps/enroll is NOT explicitly classified — same
	// fail-closed default. The bypass is the plain HTTP router
	// registration. Phase 4 removes or secures the route.
	assert.Equal(t, RouteAuthMTLS, registry.AuthMode(constants.APIPaths.PKIAppsEnroll),
		"Phase 0: app enrollment is RouteAuthMTLS in the registry (fail-closed default); the bypass is the plain HTTP router registration")

	// /api/v1/pki/csr/sign IS explicitly classified as RouteAuthNone.
	// This is the generic CSR signing bypass: the registry itself
	// declares it public, so even on HTTPS the middleware lets it
	// through. Phase 4 must change this to RouteAuthMTLS with leaf-type
	// authorization.
	assert.Equal(t, RouteAuthNone, registry.AuthMode(constants.APIPaths.PKICSRSign),
		"Phase 0: generic CSR sign is explicitly RouteAuthNone in the registry (the bypass); Phase 4 restricts by leaf type")

	// /api/v1/pki/devices/enroll IS explicitly classified as
	// RouteAuthNone, though the handler enforces mTLS internally. Phase
	// 4 aligns the registry classification with the handler enforcement.
	assert.Equal(t, RouteAuthNone, registry.AuthMode(constants.APIPaths.PKIDevicesEnroll),
		"Phase 0: device enrollment is explicitly RouteAuthNone in the registry (handler enforces mTLS internally); Phase 4 aligns the registry")

	// /api/v1/pki/apps/delegated is NOT explicitly classified — it
	// falls through to the fail-closed RouteAuthMTLS default. This is
	// the retained authenticated short-lived path.
	assert.Equal(t, RouteAuthMTLS, registry.AuthMode(constants.APIPaths.PKIAppsDelegated),
		"Phase 0: delegated app enrollment is RouteAuthMTLS (fail-closed default); retained as the authenticated short-lived path")

	// /api/v1/auth/bootstrap IS explicitly classified as RouteAuthNone.
	// It creates the first user and does not issue a platform workload
	// certificate (the operator CSR in the bootstrap body is a separate
	// concern addressed by Phase 4).
	assert.Equal(t, RouteAuthNone, registry.AuthMode(constants.APIPaths.AuthBootstrap),
		"Phase 0: bootstrap is explicitly RouteAuthNone (creates first user); remains RouteAuthNone")
}

// TestPlatformEnrollmentBypassRouteAuth_NewRoutesNotYetClassified proves
// that the new platform enrollment routes do not yet exist in the registry.
// After Phase 3, this test will be replaced with assertions that the new
// routes are classified correctly (request/status/complete as
// RouteAuthNone, pending/decision as RouteAuthDual).
func TestPlatformEnrollmentBypassRouteAuth_NewRoutesNotYetClassified(t *testing.T) {
	registry := NewRouteAuthRegistry(false)

	// The new platform enrollment API paths do not exist yet. They will
	// be added in Phase 1 (constants) and Phase 3 (registry). Until then,
	// any path under /api/v1/auth/platform-enrollments/ falls through to
	// the fail-closed RouteAuthMTLS default.
	newPaths := []string{
		"/api/v1/auth/platform-enrollments/request",
		"/api/v1/auth/platform-enrollments/status",
		"/api/v1/auth/platform-enrollments/complete",
		"/api/v1/auth/platform-enrollments/pending",
		"/api/v1/auth/platform-enrollments/decision",
	}
	for _, path := range newPaths {
		assert.Equal(t, RouteAuthMTLS, registry.AuthMode(path),
			"Phase 0: new platform enrollment path %s is not yet classified (fail-closed default); Phase 3 adds explicit classification", path)
	}
}
