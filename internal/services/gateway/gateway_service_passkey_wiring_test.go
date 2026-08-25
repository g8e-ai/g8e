// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

//go:build integration

package gateway

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestGatewayModeService_PasskeyHandlerMaxPayloadWiring is a regression test
// for the bug where NewGatewayModeService omitted MaxPayload from
// PasskeyHandlerDeps. With MaxPayload unset, http.MaxBytesReader is
// constructed with a limit of 0, which rejects every request body on the
// first read and surfaces as a 400 "invalid JSON body" from all
// body-reading passkey endpoints (register/auth challenge and verify,
// console/JIT/enrollment-token variants). The dashboard SPA cannot enroll
// or authenticate passkeys in that state.
//
// Regression marker: RegressionMarkerIssue == "passkey-maxpayload-wiring".
func TestGatewayModeService_PasskeyHandlerMaxPayloadWiring(t *testing.T) {
	ls := newTestGatewayService(t, testGatewayOpts{})

	handler := ls.GetHTTPHandler()
	require.NotNil(t, handler, "GetHTTPHandler must return the wired handler")

	passkey := handler.GetPasskeyHandler()
	require.NotNil(t, passkey, "GetPasskeyHandler must return the wired passkey handler")

	assert.Greater(t, passkey.MaxPayloadBytes(), int64(0),
		"production passkey handler must have a non-zero MaxPayload so http.MaxBytesReader does not reject every body (regression: passkey-maxpayload-wiring)")
}
