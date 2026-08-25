// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

// Route auth classification inventory for the
// platform-enrollment-approval-sequence plan.
//
// These tests document the route auth classifications of every
// certificate-issuance path after Phase 4. The tests are Tier 1 (no infra)
// because the RouteAuthRegistry is a pure in-memory classifier.
//
// Inventory of certificate-issuing routes and their classifications:
//
//   Route                                 Mode           Notes
//   /api/v1/auth/bootstrap                RouteAuthNone  Creates first user; no platform cert
//   /api/v1/auth/operator/enroll          REMOVED        Route gone; fail-closed to RouteAuthMTLS default
//   /api/v1/pki/apps/enroll               REMOVED        Route gone; fail-closed to RouteAuthMTLS default
//   /api/v1/pki/apps/delegated            RouteAuthMTLS  Retained; short-lived, authenticated (fail-closed default)
//   /api/v1/pki/csr/sign                  RouteAuthMTLS  Privileged leaf-type signing; requires validated mTLS identity
//   /api/v1/pki/devices/enroll            RouteAuthNone  Handler enforces mTLS internally; bootstrap path creates identities
//
// Platform enrollment routes (Phase 3):
//   /api/v1/auth/platform-enrollments/request     RouteAuthNone (token-scoped, like CLI recovery)
//   /api/v1/auth/platform-enrollments/status      RouteAuthNone (token-scoped)
//   /api/v1/auth/platform-enrollments/complete    RouteAuthNone (token-scoped + proof-of-possession)
//   /api/v1/auth/platform-enrollments/pending     RouteAuthDual (owner: web session or mTLS)
//   /api/v1/auth/platform-enrollments/decision    RouteAuthDual (owner: web session or mTLS)

package gateway

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/g8e-ai/g8e/v2/internal/constants"
)

// TestPlatformEnrollmentBypassRouteAuth_CurrentClassifications documents
// the route auth registry classifications of the certificate issuance
// paths after Phase 4. The removed bypass routes (operator enrollment,
// app enrollment) are no longer registered on either router and are not
// explicitly classified, so they fall through to the fail-closed
// RouteAuthMTLS default. The retained CSR signing and device enrollment
// routes are explicitly classified as RouteAuthMTLS, aligned with their
// handler-level mTLS enforcement.
func TestPlatformEnrollmentBypassRouteAuth_CurrentClassifications(t *testing.T) {
	registry := NewRouteAuthRegistry(false)

	// /api/v1/pki/csr/sign is explicitly classified as RouteAuthMTLS.
	// It signs privileged platform leaf types (operator, CLI, app,
	// gateway-peer) and requires a validated mTLS identity (operator/CLI
	// session). The handler also enforces mTLS directly (defense-in-depth).
	assert.Equal(t, RouteAuthMTLS, registry.AuthMode(constants.APIPaths.PKICSRSign),
		"generic CSR sign is RouteAuthMTLS (privileged leaf-type signing requires validated mTLS identity)")

	// /api/v1/pki/devices/enroll is explicitly classified as
	// RouteAuthNone. It is the device bootstrap path that creates
	// operator and CLI identities; the caller does not yet have a
	// validated session, so the handler enforces mTLS directly (requires
	// a client certificate and extracts the user ID from it).
	assert.Equal(t, RouteAuthNone, registry.AuthMode(constants.APIPaths.PKIDevicesEnroll),
		"device enrollment is RouteAuthNone (handler enforces mTLS internally; bootstrap path creates identities)")

	// /api/v1/pki/apps/delegated is NOT explicitly classified — it
	// falls through to the fail-closed RouteAuthMTLS default. This is
	// the retained authenticated short-lived path.
	assert.Equal(t, RouteAuthMTLS, registry.AuthMode(constants.APIPaths.PKIAppsDelegated),
		"delegated app enrollment is RouteAuthMTLS (fail-closed default); retained as the authenticated short-lived path")

	// /api/v1/auth/bootstrap IS explicitly classified as RouteAuthNone.
	// It creates the first user and does not issue a platform workload
	// certificate.
	assert.Equal(t, RouteAuthNone, registry.AuthMode(constants.APIPaths.AuthBootstrap),
		"bootstrap is explicitly RouteAuthNone (creates first user); remains RouteAuthNone")
}

// TestPlatformEnrollmentRouteAuth_NewRoutesClassified proves that the
// five platform enrollment routes are explicitly classified in the
// RouteAuthRegistry. request, status, and complete are RouteAuthNone
// (public, token-scoped — the opaque token and proof-of-possession
// signatures provide the authorization context). pending and decision
// are RouteAuthDual (owner: web session cookie or mTLS CLI; the
// controller enforces active-first-user authorization after the
// middleware stamps the user ID).
func TestPlatformEnrollmentRouteAuth_NewRoutesClassified(t *testing.T) {
	registry := NewRouteAuthRegistry(false)

	// Public discovery surface — token-scoped, no mTLS or cookie required.
	assert.Equal(t, RouteAuthNone, registry.AuthMode(constants.APIPaths.AuthPlatformEnrollmentRequest),
		"platform enrollment request is RouteAuthNone (public, token-scoped)")
	assert.Equal(t, RouteAuthNone, registry.AuthMode(constants.APIPaths.AuthPlatformEnrollmentStatus),
		"platform enrollment status is RouteAuthNone (public, token-scoped)")
	assert.Equal(t, RouteAuthNone, registry.AuthMode(constants.APIPaths.AuthPlatformEnrollmentComplete),
		"platform enrollment complete is RouteAuthNone (public, token + proof-of-possession)")

	// Owner surfaces — dual auth (web session or mTLS).
	assert.Equal(t, RouteAuthDual, registry.AuthMode(constants.APIPaths.AuthPlatformEnrollmentPending),
		"platform enrollment pending is RouteAuthDual (owner: web session or mTLS)")
	assert.Equal(t, RouteAuthDual, registry.AuthMode(constants.APIPaths.AuthPlatformEnrollmentDecision),
		"platform enrollment decision is RouteAuthDual (owner: web session or mTLS)")
}
