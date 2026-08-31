// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package governance

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/testutil"
)

// TestVerifyEnvelope_ReadsPostureFromEnvelope verifies that the warden reads
// posture per-envelope from GovernanceEnvelope.Posture rather than from any
// constructor-supplied value. Under notary posture, a mutation requires L3
// proof; under doctrine posture, the same mutation passes without L3. The
// warden is constructed identically in both cases — only the envelope's
// Posture field differs, proving the envelope is the authoritative source.
func TestVerifyEnvelope_ReadsPostureFromEnvelope(t *testing.T) {
	t.Parallel()

	t.Run("notary posture requires L3 for mutation", func(t *testing.T) {
		t.Parallel()
		verifier, privKey := createStrictVerifier(t, testutil.NewStatefulMockReplayStore(), testutil.NewMockStateRootProvider("root-1"), testutil.NewConfigurableMockL3Notary(true))
		env := signedEnvelope(t, constants.ActionTypeExecuteBash, typedPayload(t, constants.ActionTypeExecuteBash), privKey, "notary")
		env.Governance.L3 = nil

		_, err := verifier.VerifyEnvelope(context.Background(), env)
		assert.ErrorIs(t, err, constants.ErrTxL3ProofMissing)
	})

	t.Run("doctrine posture does not require L3 for mutation", func(t *testing.T) {
		t.Parallel()
		verifier, privKey := createStrictVerifier(t, testutil.NewStatefulMockReplayStore(), testutil.NewMockStateRootProvider("root-1"), testutil.NewConfigurableMockL3Notary(true))
		env := signedEnvelope(t, constants.ActionTypeExecuteBash, typedPayload(t, constants.ActionTypeExecuteBash), privKey, "doctrine")
		env.Governance.L3 = nil

		verified, err := verifier.VerifyEnvelope(context.Background(), env)
		assert.NoError(t, err)
		assert.NotNil(t, verified)
		assert.False(t, verified.L3Valid)
		assert.Equal(t, "doctrine", verified.Posture.Name())
	})
}

// TestVerifyEnvelope_MissingEnvelopePostureFailsClosed verifies that an
// envelope with an empty Posture field is rejected with
// ErrEnvelopePostureMissing. This indicates a gateway bug (the constructor
// was called without wiring cfg.Gateway.Posture), not an operator config
// gap. The warden fails closed per-transaction rather than inventing a posture.
func TestVerifyEnvelope_RatifyRequiresL3WithoutL2(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		invalidL2 bool
		removeL3  bool
		wantErr   error
	}{
		{name: "valid L3 without L2 passes"},
		{name: "valid L3 with invalid audited L2 passes", invalidL2: true},
		{name: "missing L3 rejects mutation", removeL3: true, wantErr: constants.ErrTxL3ProofMissing},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			verifier, privKey := createStrictVerifier(t, testutil.NewStatefulMockReplayStore(), testutil.NewMockStateRootProvider("root-1"), testutil.NewConfigurableMockL3Notary(true))
			env := signedEnvelope(t, constants.ActionTypeExecuteBash, typedPayload(t, constants.ActionTypeExecuteBash), privKey, "ratify")
			if tc.invalidL2 {
				env.Governance.L2.Votes = append(env.Governance.L2.Votes, env.Governance.L2.Votes[0])
			} else {
				env.Governance.L2 = nil
			}
			if tc.removeL3 {
				env.Governance.L3 = nil
			}

			verified, err := verifier.VerifyEnvelope(context.Background(), env)
			if tc.wantErr != nil {
				assert.ErrorIs(t, err, tc.wantErr)
				return
			}
			if !assert.NoError(t, err) || !assert.NotNil(t, verified) {
				return
			}
			assert.False(t, verified.L2Valid)
			assert.True(t, verified.L3Valid)
			assert.Equal(t, "ratify", verified.Posture.Name())
		})
	}
}

func TestVerifyEnvelope_MissingEnvelopePostureFailsClosed(t *testing.T) {
	t.Parallel()

	verifier, privKey := createStrictVerifier(t, testutil.NewStatefulMockReplayStore(), testutil.NewMockStateRootProvider("root-1"), testutil.NewConfigurableMockL3Notary(true))
	env := signedEnvelope(t, constants.ActionTypeFsList, typedPayload(t, constants.ActionTypeFsList), privKey, "")
	env.Posture = ""

	_, err := verifier.VerifyEnvelope(context.Background(), env)
	assert.ErrorIs(t, err, constants.ErrEnvelopePostureMissing)
}

// TestVerifyEnvelope_EnvelopePostureIsTheOnlyPosture verifies that the
// envelope-derived posture is the sole authority for L2/L3 gating. The
// warden constructor no longer accepts a posture parameter, so there is no
// config-derived posture to conflict with. This test proves the envelope
// value flows through to VerifiedTransaction.Posture unchanged.
func TestVerifyEnvelope_EnvelopePostureIsTheOnlyPosture(t *testing.T) {
	t.Parallel()

	for _, posture := range []string{constants.PostureDoctrine, constants.PostureConsensus, constants.PostureRatify, constants.PostureNotary} {
		t.Run(posture, func(t *testing.T) {
			t.Parallel()
			verifier, privKey := createStrictVerifier(t, testutil.NewStatefulMockReplayStore(), testutil.NewMockStateRootProvider("root-1"), testutil.NewConfigurableMockL3Notary(true))
			env := signedEnvelope(t, constants.ActionTypeFsList, typedPayload(t, constants.ActionTypeFsList), privKey, posture)

			verified, err := verifier.VerifyEnvelope(context.Background(), env)
			assert.NoError(t, err)
			assert.NotNil(t, verified)
			assert.Equal(t, posture, verified.Posture.Name())
		})
	}
}

// TestVerifyEnvelope_InvalidEnvelopePostureFailsClosed verifies that an
// envelope with an unrecognized posture string is rejected. The warden
// parses the posture via ParseGovernancePosture, which fails closed on
// unknown values.
func TestVerifyEnvelope_InvalidEnvelopePostureFailsClosed(t *testing.T) {
	t.Parallel()

	verifier, privKey := createStrictVerifier(t, testutil.NewStatefulMockReplayStore(), testutil.NewMockStateRootProvider("root-1"), testutil.NewConfigurableMockL3Notary(true))
	env := signedEnvelope(t, constants.ActionTypeFsList, typedPayload(t, constants.ActionTypeFsList), privKey, "bogus-posture")

	_, err := verifier.VerifyEnvelope(context.Background(), env)
	assert.Error(t, err)
	assert.False(t, errors.Is(err, nil))
}
