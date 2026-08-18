// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package gateway

import (
	"reflect"
	"testing"

	"github.com/g8e-ai/g8e/internal/services/governance"
	"github.com/stretchr/testify/assert"
)

// TestPasskeyService_DoesNotImplementL3Notary asserts that PasskeyService is
// not coupled to the governance.L3Notary interface shape. PasskeyService is
// domain logic (WebAuthn registration/authentication/proof verification); the
// L3 notary composes a PasskeyVerifier rather than PasskeyService itself
// implementing L3Notary. If this assertion fails, a method named VerifyL3Proof
// was (re)added to PasskeyService, re-coupling the passkey domain to the
// governance interface.
func TestPasskeyService_DoesNotImplementL3Notary(t *testing.T) {
	l3NotaryType := reflect.TypeOf((*governance.L3Notary)(nil)).Elem()
	passkeyType := reflect.TypeOf((*PasskeyService)(nil))
	assert.False(t, passkeyType.Implements(l3NotaryType),
		"PasskeyService must not implement governance.L3Notary; the gateway L3 notary composes a PasskeyVerifier instead")
}

// TestPasskeyService_ImplementsPasskeyVerifier asserts that PasskeyService
// exposes the passkey-domain verification primitive that the gateway L3 notary
// composes. This is the positive counterpart to the negative L3Notary
// assertion above and locks in the composing-notary contract.
func TestPasskeyService_ImplementsPasskeyVerifier(t *testing.T) {
	verifierType := reflect.TypeOf((*governance.PasskeyVerifier)(nil)).Elem()
	passkeyType := reflect.TypeOf((*PasskeyService)(nil))
	assert.True(t, passkeyType.Implements(verifierType),
		"PasskeyService must implement governance.PasskeyVerifier so the gateway L3 notary can compose it")
}
