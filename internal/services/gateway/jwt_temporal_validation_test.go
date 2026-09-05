// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package gateway

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/g8e-ai/g8e/v2/internal/constants"
)

func TestValidateJWTTemporalClaims_EnforcesSymmetricClockSkewBoundaries(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	insideSkew := constants.JWTClockSkew - time.Second
	outsideSkew := constants.JWTClockSkew + time.Second

	tests := []struct {
		name    string
		claims  JWTClaims
		wantErr error
	}{
		{
			name:   "expiration immediately inside allowed skew",
			claims: JWTClaims{Exp: now.Add(-insideSkew).Unix()},
		},
		{
			name:   "expiration exactly at allowed skew",
			claims: JWTClaims{Exp: now.Add(-constants.JWTClockSkew).Unix()},
		},
		{
			name:    "expiration immediately outside allowed skew",
			claims:  JWTClaims{Exp: now.Add(-outsideSkew).Unix()},
			wantErr: constants.ErrExpired,
		},
		{
			name:   "not before immediately inside allowed skew",
			claims: JWTClaims{Nbf: now.Add(insideSkew).Unix()},
		},
		{
			name:   "not before exactly at allowed skew",
			claims: JWTClaims{Nbf: now.Add(constants.JWTClockSkew).Unix()},
		},
		{
			name:    "not before immediately outside allowed skew",
			claims:  JWTClaims{Nbf: now.Add(outsideSkew).Unix()},
			wantErr: constants.ErrJWTNotYetValid,
		},
		{
			name:   "issued at immediately inside allowed skew",
			claims: JWTClaims{Iat: now.Add(insideSkew).Unix()},
		},
		{
			name:   "issued at exactly at allowed skew",
			claims: JWTClaims{Iat: now.Add(constants.JWTClockSkew).Unix()},
		},
		{
			name:    "issued at immediately outside allowed skew",
			claims:  JWTClaims{Iat: now.Add(outsideSkew).Unix()},
			wantErr: constants.ErrJWTIssuedInFuture,
		},
		{
			name:   "zero expiration remains optional",
			claims: JWTClaims{Nbf: now.Unix(), Iat: now.Unix()},
		},
		{
			name:   "zero not before remains optional",
			claims: JWTClaims{Exp: now.Add(time.Hour).Unix(), Iat: now.Unix()},
		},
		{
			name:   "zero issued at remains optional",
			claims: JWTClaims{Exp: now.Add(time.Hour).Unix(), Nbf: now.Unix()},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateJWTTemporalClaims(tt.claims, now)
			if tt.wantErr == nil {
				assert.NoError(t, err)
				return
			}
			assert.ErrorIs(t, err, tt.wantErr)
		})
	}
}
