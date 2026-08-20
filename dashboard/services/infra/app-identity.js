// Copyright (c) 2026 Lateralus Labs, LLC.
// Licensed under the Business Source License 1.1 — see LICENSE for details.

/**
 * app-identity — Module-level holder for the resolved AppIdentity.
 *
 * `server.js` startup resolves the dashboard's app identity (via
 * `AppEnrollmentService.loadIdentity()` or `.enroll()`) and stores it here
 * via `setAppIdentity()`. Future backend service wiring reads it via
 * `getAppIdentity()` to construct mTLS-configured g8eg clients.
 *
 * This decouples enrollment from client construction so the enrollment plan
 * does not need a service factory — `server.js` remains a static SPA host
 * with no client construction; the stored identity is consumed by
 * downstream backend-service-layer work.
 */

/** @type {import('./app-enrollment-service.js').AppIdentity | null} */
let _appIdentity = null;

/**
 * Store the resolved app identity. Called once at startup.
 *
 * @param {import('./app-enrollment-service.js').AppIdentity} identity
 */
export function setAppIdentity(identity) {
    _appIdentity = identity;
}

/**
 * Read the resolved app identity.
 *
 * @returns {import('./app-enrollment-service.js').AppIdentity | null}
 *   The identity, or null if startup has not yet resolved it.
 */
export function getAppIdentity() {
    return _appIdentity;
}
