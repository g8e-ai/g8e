// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package consensus

import (
	"context"
	"crypto/ed25519"
	"io"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/encoding/protojson"

	"github.com/g8e-ai/g8e/internal/constants"
	"github.com/g8e-ai/g8e/internal/governance"
	govsvc "github.com/g8e-ai/g8e/internal/services/governance"
)

// TestLocalDeliberator_HappyPath verifies that LocalDeliberator correctly
// unmarshals envelope bytes, runs deliberation, and returns marshaled bytes
// with L2 votes populated.
func TestLocalDeliberator_HappyPath(t *testing.T) {
	t.Parallel()

	doctrine := govsvc.NewL1Doctrine()
	members := makeMembers(t, 1)
	svc := NewConsensusService("test-consensus", members, doctrine,
		slog.New(slog.NewTextHandler(io.Discard, nil)), newTestResponder())

	env := makeEnvelope(t, string(constants.ActionTypeFetchLogs), []byte("fetch logs"))
	envBytes, err := (protojson.MarshalOptions{Multiline: false}).Marshal(env)
	require.NoError(t, err)

	ld := NewLocalDeliberator(svc)
	result, err := ld.Deliberate(context.Background(), envBytes)
	require.NoError(t, err)
	assert.NotEmpty(t, result)

	// Verify the returned bytes contain L2 votes
	var resultEnv governance.GovernanceEnvelope
	err = protojson.Unmarshal(result, &resultEnv)
	require.NoError(t, err)

	require.NotNil(t, resultEnv.Governance)
	require.NotNil(t, resultEnv.Governance.L2)
	assert.Equal(t, "test-consensus", resultEnv.Governance.L2.ConsensusSetId)
	assert.Len(t, resultEnv.Governance.L2.Votes, 1)
}

// TestLocalDeliberator_InvalidJSON verifies that LocalDeliberator returns
// an error when given invalid JSON.
func TestLocalDeliberator_InvalidJSON(t *testing.T) {
	t.Parallel()

	members := makeMembers(t, 1)
	svc := NewConsensusService("test-consensus", members, nil,
		slog.New(slog.NewTextHandler(io.Discard, nil)), newTestResponder())

	ld := NewLocalDeliberator(svc)
	_, err := ld.Deliberate(context.Background(), []byte("not json"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unmarshal")
}

// TestLocalDeliberator_HashMismatch verifies that LocalDeliberator propagates
// the ErrConsensusHashMismatch error.
func TestLocalDeliberator_HashMismatch(t *testing.T) {
	t.Parallel()

	members := makeMembers(t, 1)
	svc := NewConsensusService("test-consensus", members, nil,
		slog.New(slog.NewTextHandler(io.Discard, nil)), newTestResponder())

	env := makeEnvelope(t, string(constants.ActionTypeFetchLogs), []byte("fetch logs"))
	env.Id = "wrong-id"
	envBytes, err := (protojson.MarshalOptions{Multiline: false}).Marshal(env)
	require.NoError(t, err)

	ld := NewLocalDeliberator(svc)
	_, err = ld.Deliberate(context.Background(), envBytes)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "CONSENSUS_HASH_MISMATCH")
}

// TestLocalDeliberator_SignatureVerifiable verifies that the L2 vote
// signature produced by LocalDeliberator is cryptographically valid.
func TestLocalDeliberator_SignatureVerifiable(t *testing.T) {
	t.Parallel()

	pub, priv, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)

	doctrine := govsvc.NewL1Doctrine()
	members := []ConsensusMember{
		{AppID: "verifiable-member", PrivateKey: priv},
	}
	svc := NewConsensusService("test-consensus", members, doctrine,
		slog.New(slog.NewTextHandler(io.Discard, nil)), newTestResponder())

	env := makeEnvelope(t, string(constants.ActionTypeFetchLogs), []byte("fetch logs"))
	envBytes, err := (protojson.MarshalOptions{Multiline: false}).Marshal(env)
	require.NoError(t, err)

	ld := NewLocalDeliberator(svc)
	result, err := ld.Deliberate(context.Background(), envBytes)
	require.NoError(t, err)

	var resultEnv governance.GovernanceEnvelope
	err = protojson.Unmarshal(result, &resultEnv)
	require.NoError(t, err)

	vote := resultEnv.Governance.L2.Votes[0]
	sigBytes, err := hexDecode(vote.ConsensusSignature)
	require.NoError(t, err)

	payload := env.Id + "|true"
	assert.True(t, ed25519.Verify(pub, []byte(payload), sigBytes), "L2 vote signature must be valid")
}

func hexDecode(s string) ([]byte, error) {
	return hexDecodeString(s)
}

// hexDecodeString is a helper that decodes a hex string.
func hexDecodeString(s string) ([]byte, error) {
	result := make([]byte, len(s)/2)
	for i := 0; i < len(result); i++ {
		var b byte
		for j := 0; j < 2; j++ {
			c := s[i*2+j]
			switch {
			case c >= '0' && c <= '9':
				b = b<<4 | (c - '0')
			case c >= 'a' && c <= 'f':
				b = b<<4 | (c - 'a' + 10)
			case c >= 'A' && c <= 'F':
				b = b<<4 | (c - 'A' + 10)
			}
		}
		result[i] = b
	}
	return result, nil
}
