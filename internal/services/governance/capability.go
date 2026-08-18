// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package governance

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"sync"
	"time"

	"github.com/g8e-ai/g8e/internal/constants"
)

// Capability is a just-in-time, single-action, self-dissolving permission derived
// from a verified governance envelope. It is minted at L5 (Actuator) before dispatch
// and dissolved immediately after execution completes or fails.
//
// The capability binds:
//   - The action type (what the holder may do)
//   - The target resource (what the holder may act upon)
//   - The transaction hash (cryptographic binding to the verified intent)
//   - An expiry (inherited from the envelope's ExpiresAt)
//
// No standing credentials exist outside the lifetime of a single Execute() call.
type Capability struct {
	TransactionHash string
	ActionType      constants.ActionType
	TargetResource  string
	OperatorID      string
	OperatorSession string
	ExpiresAt       time.Time
	Token           string // random single-use token
	KeyID           string // L5 actuator key ID that minted this capability

	mu        sync.Mutex
	dissolved bool
}

// IsDissolved reports whether the capability has been dissolved and is no longer valid.
func (c *Capability) IsDissolved() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.dissolved
}

// IsExpired reports whether the capability has passed its expiry.
func (c *Capability) IsExpired(now time.Time) bool {
	return now.After(c.ExpiresAt)
}

// IsValid reports whether the capability is both un-dissolved and un-expired.
func (c *Capability) IsValid(now time.Time) bool {
	return !c.IsDissolved() && !c.IsExpired(now)
}

// Dissolve marks the capability as no longer valid. This is called by the Actuator
// after execution completes (success or failure). Once dissolved, the capability
// cannot be reused.
func (c *Capability) Dissolve() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.dissolved = true
}

// Verify checks that the capability is valid and matches the expected action type
// and transaction hash. This can be called by downstream handlers to verify that
// the execution was properly authorized.
func (c *Capability) Verify(actionType constants.ActionType, txHash string, now time.Time) error {
	if c == nil {
		return fmt.Errorf("capability: nil capability")
	}
	if c.IsDissolved() {
		return fmt.Errorf("capability: dissolved")
	}
	if c.IsExpired(now) {
		return fmt.Errorf("capability: expired")
	}
	if c.ActionType != actionType {
		return fmt.Errorf("capability: action type mismatch (have %s, want %s)", c.ActionType, actionType)
	}
	if c.TransactionHash != txHash {
		return fmt.Errorf("capability: transaction hash mismatch")
	}
	return nil
}

// CapabilityFromContext extracts a capability from a context, if present.
func CapabilityFromContext(ctx context.Context) *Capability {
	v := ctx.Value(constants.ContextKeyCapability)
	if v == nil {
		return nil
	}
	c, ok := v.(*Capability)
	if !ok {
		return nil
	}
	return c
}

// MintCapability derives a scoped, single-action, self-dissolving capability from
// a VerifiedTransaction. The capability is signed with the actuator's key to bind
// it to the L5 identity that minted it.
func MintCapability(vt *VerifiedTransaction, signingKey ed25519.PrivateKey, keyID string) (*Capability, error) {
	if vt == nil || vt.Envelope == nil {
		return nil, constants.ErrL5ActuatorCapabilityMint
	}

	// Generate a random single-use token
	tokenBytes := make([]byte, 32)
	if _, err := rand.Read(tokenBytes); err != nil {
		return nil, fmt.Errorf("%w: %v", constants.ErrL5ActuatorCapabilityMint, err)
	}
	token := hex.EncodeToString(tokenBytes)

	// Sign the token with the actuator key to bind it to L5
	sig := ed25519.Sign(signingKey, []byte(token+vt.Envelope.TransactionHash))

	cap := &Capability{
		TransactionHash: vt.Envelope.TransactionHash,
		ActionType:      vt.ActionType,
		TargetResource:  vt.Envelope.TargetResource,
		OperatorID:      vt.Envelope.OperatorId,
		OperatorSession: vt.Envelope.OperatorSessionId,
		ExpiresAt:       vt.ExpiresAt,
		Token:           token + ":" + hex.EncodeToString(sig),
		KeyID:           keyID,
	}

	return cap, nil
}

// ContextWithCapability returns a new context with the capability attached.
func ContextWithCapability(ctx context.Context, cap *Capability) context.Context {
	return context.WithValue(ctx, constants.ContextKeyCapability, cap)
}
