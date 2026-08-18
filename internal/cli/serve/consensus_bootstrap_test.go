// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package serve

import (
	"crypto/ed25519"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/internal/config"
	"github.com/g8e-ai/g8e/internal/constants"
	"github.com/g8e-ai/g8e/internal/testutil"
)

// ---------------------------------------------------------------------------
// consensusBootstrapConfig JSON tag and round-trip tests (Tier 1 — no DB)
// ---------------------------------------------------------------------------

func TestConsensusBootstrapConfig_JSONTags(t *testing.T) {
	boot := consensusBootstrapConfig{
		ConsensusID:  "test-consensus",
		MemberAppIDs: []string{"alpha", "beta"},
		Quorum:       2,
		SeedHex:      "deadbeef",
	}

	data, err := json.Marshal(boot)
	require.NoError(t, err)

	var raw map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(data, &raw))

	assert.Contains(t, raw, "consensus_id")
	assert.Contains(t, raw, "member_app_ids")
	assert.Contains(t, raw, "quorum")
	assert.Contains(t, raw, "seed_hex")
}

func TestConsensusBootstrapConfig_RoundTripMarshalUnmarshal(t *testing.T) {
	original := consensusBootstrapConfig{
		ConsensusID:  "round-trip-consensus",
		MemberAppIDs: []string{"member-a", "member-b", "member-c"},
		Quorum:       2,
		SeedHex:      "87278693f5894d8de5d28401c923e0c3fea9ae7c35f467065954eecbc85b2e77",
	}

	data, err := json.Marshal(original)
	require.NoError(t, err)

	var decoded consensusBootstrapConfig
	require.NoError(t, json.Unmarshal(data, &decoded))

	assert.Equal(t, original, decoded)
}

func TestConsensusBootstrapConfig_EmptySeedHex(t *testing.T) {
	boot := consensusBootstrapConfig{
		ConsensusID:  "no-seed-consensus",
		MemberAppIDs: []string{"solo"},
		Quorum:       1,
	}

	data, err := json.Marshal(boot)
	require.NoError(t, err)

	var raw map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(data, &raw))

	var seedHex string
	require.NoError(t, json.Unmarshal(raw["seed_hex"], &seedHex))
	assert.Equal(t, "", seedHex)
}

// ---------------------------------------------------------------------------
// consensusBootstrapConfig member_seeds JSON tag and round-trip tests (Tier 1 — no DB)
// ---------------------------------------------------------------------------

func TestConsensusBootstrapConfig_MemberSeedsJSONTag(t *testing.T) {
	boot := consensusBootstrapConfig{
		ConsensusID:  "tribunal",
		MemberAppIDs: []string{"alpha", "beta", "gamma"},
		Quorum:       2,
		MemberSeeds: map[string]string{
			"alpha": "b194523218024feacafef9acf9e557f9c2e6ed71e87c8c97e5a4fc61e624ea42",
			"beta":  "20544a8efd3f30188095dae9d42993f320fbcdcbd924f88b2c56edfdc719e357",
			"gamma": "06946f1a26896983176f6d40b0a734136dd58b16fe502d4b5688bf7db1b97662",
		},
	}

	data, err := json.Marshal(boot)
	require.NoError(t, err)

	var raw map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(data, &raw))

	assert.Contains(t, raw, "member_seeds", "member_seeds key must be present when populated")
}

func TestConsensusBootstrapConfig_MemberSeedsOmitWhenEmpty(t *testing.T) {
	boot := consensusBootstrapConfig{
		ConsensusID:  "tribunal",
		MemberAppIDs: []string{"alpha"},
		Quorum:       1,
		// MemberSeeds intentionally nil — should be omitted due to omitempty
	}

	data, err := json.Marshal(boot)
	require.NoError(t, err)

	var raw map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(data, &raw))

	_, present := raw["member_seeds"]
	assert.False(t, present, "member_seeds must be omitted when nil/empty due to omitempty tag")
}

func TestConsensusBootstrapConfig_MemberSeedsRoundTrip(t *testing.T) {
	original := consensusBootstrapConfig{
		ConsensusID:  "round-trip-tribunal",
		MemberAppIDs: []string{"fedramp-csp-auditor", "fedramp-3pao", "fedramp-jab"},
		Quorum:       2,
		MemberSeeds: map[string]string{
			"fedramp-csp-auditor": "b194523218024feacafef9acf9e557f9c2e6ed71e87c8c97e5a4fc61e624ea42",
			"fedramp-3pao":        "20544a8efd3f30188095dae9d42993f320fbcdcbd924f88b2c56edfdc719e357",
			"fedramp-jab":         "06946f1a26896983176f6d40b0a734136dd58b16fe502d4b5688bf7db1b97662",
		},
	}

	data, err := json.Marshal(original)
	require.NoError(t, err)

	var decoded consensusBootstrapConfig
	require.NoError(t, json.Unmarshal(data, &decoded))

	assert.Equal(t, original, decoded)
	assert.Len(t, decoded.MemberSeeds, 3)
	assert.Equal(t, original.MemberSeeds["fedramp-3pao"], decoded.MemberSeeds["fedramp-3pao"])
}

// ---------------------------------------------------------------------------
// parseConsensusBootstrapConfig — additional edge cases (Tier 1 — no DB)
// ---------------------------------------------------------------------------

func TestParseConsensusBootstrapConfig_NullJSON(t *testing.T) {
	_, err := parseConsensusBootstrapConfig([]byte("null"))
	require.Error(t, err)
	assert.True(t, errors.Is(err, constants.ErrConsensusBootstrapMissingFields),
		"null JSON produces zero-value struct which fails validation")
}

func TestParseConsensusBootstrapConfig_EmptyJSONObject(t *testing.T) {
	_, err := parseConsensusBootstrapConfig([]byte("{}"))
	require.Error(t, err)
	assert.True(t, errors.Is(err, constants.ErrConsensusBootstrapMissingFields))
}

func TestParseConsensusBootstrapConfig_WhitespaceOnlyData(t *testing.T) {
	_, err := parseConsensusBootstrapConfig([]byte("   \n\t  "))
	require.Error(t, err)
	assert.True(t, errors.Is(err, constants.ErrConsensusBootstrapParseConfig))
}

func TestParseConsensusBootstrapConfig_UnknownFieldsIgnored(t *testing.T) {
	data := []byte(`{
		"consensus_id": "test-consensus",
		"member_app_ids": ["member-a"],
		"quorum": 1,
		"seed_hex": "87278693f5894d8de5d28401c923e0c3fea9ae7c35f467065954eecbc85b2e77",
		"unknown_field": "should be ignored",
		"extra": 42
	}`)

	boot, err := parseConsensusBootstrapConfig(data)
	require.NoError(t, err)
	assert.Equal(t, "test-consensus", boot.ConsensusID)
	assert.Equal(t, []string{"member-a"}, boot.MemberAppIDs)
	assert.Equal(t, 1, boot.Quorum)
}

func TestParseConsensusBootstrapConfig_QuorumExceedsMemberCount(t *testing.T) {
	data := []byte(`{
		"consensus_id": "test-consensus",
		"member_app_ids": ["solo"],
		"quorum": 5
	}`)

	boot, err := parseConsensusBootstrapConfig(data)
	require.NoError(t, err)
	assert.Equal(t, 5, boot.Quorum, "parseConsensusBootstrapConfig does not validate quorum vs member count")
}

func TestParseConsensusBootstrapConfig_LargeQuorum(t *testing.T) {
	data := []byte(`{
		"consensus_id": "test-consensus",
		"member_app_ids": ["a", "b"],
		"quorum": 999999
	}`)

	boot, err := parseConsensusBootstrapConfig(data)
	require.NoError(t, err)
	assert.Equal(t, 999999, boot.Quorum)
}

func TestParseConsensusBootstrapConfig_SeedHexWithWhitespace(t *testing.T) {
	data := []byte(`{
		"consensus_id": "test-consensus",
		"member_app_ids": ["member-a"],
		"quorum": 1,
		"seed_hex": "  87278693f5894d8de5d28401c923e0c3fea9ae7c35f467065954eecbc85b2e77  "
	}`)

	boot, err := parseConsensusBootstrapConfig(data)
	require.NoError(t, err)
	assert.Equal(t, "  87278693f5894d8de5d28401c923e0c3fea9ae7c35f467065954eecbc85b2e77  ", boot.SeedHex,
		"parseConsensusBootstrapConfig does not trim seed_hex; trimming happens in deriveSeedPublicKey")
}

func TestParseConsensusBootstrapConfig_ManyMembers(t *testing.T) {
	members := make([]string, 100)
	for i := range members {
		members[i] = "member-" + string(rune('a'+i%26)) + string(rune('0'+i/26))
	}

	boot := consensusBootstrapConfig{
		ConsensusID:  "large-consensus",
		MemberAppIDs: members,
		Quorum:       50,
		SeedHex:      "",
	}

	data, err := json.Marshal(boot)
	require.NoError(t, err)

	parsed, err := parseConsensusBootstrapConfig(data)
	require.NoError(t, err)
	assert.Len(t, parsed.MemberAppIDs, 100)
	assert.Equal(t, 50, parsed.Quorum)
}

func TestParseConsensusBootstrapConfig_DuplicateMembersAllowed(t *testing.T) {
	data := []byte(`{
		"consensus_id": "test-consensus",
		"member_app_ids": ["dup", "dup", "dup"],
		"quorum": 1
	}`)

	boot, err := parseConsensusBootstrapConfig(data)
	require.NoError(t, err)
	assert.Len(t, boot.MemberAppIDs, 3, "parseConsensusBootstrapConfig does not deduplicate members")
}

func TestParseConsensusBootstrapConfig_MemberSeedsFormat(t *testing.T) {
	data := []byte(`{
		"consensus_id": "tribunal",
		"member_app_ids": ["alpha", "beta", "gamma"],
		"quorum": 2,
		"member_seeds": {
			"alpha": "b194523218024feacafef9acf9e557f9c2e6ed71e87c8c97e5a4fc61e624ea42",
			"beta":  "20544a8efd3f30188095dae9d42993f320fbcdcbd924f88b2c56edfdc719e357",
			"gamma": "06946f1a26896983176f6d40b0a734136dd58b16fe502d4b5688bf7db1b97662"
		}
	}`)

	boot, err := parseConsensusBootstrapConfig(data)
	require.NoError(t, err)
	assert.Equal(t, "tribunal", boot.ConsensusID)
	assert.Equal(t, []string{"alpha", "beta", "gamma"}, boot.MemberAppIDs)
	assert.Equal(t, 2, boot.Quorum)
	assert.Empty(t, boot.SeedHex, "seed_hex should be empty when only member_seeds is provided")
	assert.Len(t, boot.MemberSeeds, 3)
	assert.Equal(t, "b194523218024feacafef9acf9e557f9c2e6ed71e87c8c97e5a4fc61e624ea42",
		boot.MemberSeeds["alpha"])
}

func TestParseConsensusBootstrapConfig_MemberSeedsAndSeedHexBothPresent(t *testing.T) {
	// parseConsensusBootstrapConfig itself does not enforce precedence between
	// member_seeds and seed_hex — both fields populate cleanly. Precedence
	// (member_seeds wins) is enforced in consensusPolicyBootstrap via
	// `usePerMemberKeys := len(boot.MemberSeeds) > 0`, and is exercised at the
	// Tier 2 bootstrap level, not here.
	data := []byte(`{
		"consensus_id": "tribunal",
		"member_app_ids": ["alpha", "beta"],
		"quorum": 1,
		"seed_hex": "87278693f5894d8de5d28401c923e0c3fea9ae7c35f467065954eecbc85b2e77",
		"member_seeds": {
			"alpha": "b194523218024feacafef9acf9e557f9c2e6ed71e87c8c97e5a4fc61e624ea42",
			"beta":  "20544a8efd3f30188095dae9d42993f320fbcdcbd924f88b2c56edfdc719e357"
		}
	}`)

	boot, err := parseConsensusBootstrapConfig(data)
	require.NoError(t, err)
	assert.Equal(t, "87278693f5894d8de5d28401c923e0c3fea9ae7c35f467065954eecbc85b2e77", boot.SeedHex,
		"seed_hex populates independently of member_seeds at the parse layer")
	assert.Len(t, boot.MemberSeeds, 2, "member_seeds populates independently of seed_hex at the parse layer")
}

func TestParseConsensusBootstrapConfig_MemberSeedsEmptyMap(t *testing.T) {
	// An empty member_seeds object should parse cleanly and behave like the
	// single-key fallback path (usePerMemberKeys = len(MemberSeeds) > 0 == false).
	data := []byte(`{
		"consensus_id": "tribunal",
		"member_app_ids": ["alpha"],
		"quorum": 1,
		"member_seeds": {}
	}`)

	boot, err := parseConsensusBootstrapConfig(data)
	require.NoError(t, err)
	assert.Empty(t, boot.MemberSeeds, "empty member_seeds map should not trigger per-member key mode")
}

// ---------------------------------------------------------------------------
// deriveSeedPublicKey — additional edge cases (Tier 1 — no DB)
// ---------------------------------------------------------------------------

func TestDeriveSeedPublicKey_OddLengthHex(t *testing.T) {
	_, err := deriveSeedPublicKey("abc")
	require.Error(t, err)
	assert.True(t, errors.Is(err, constants.ErrConsensusBootstrapDecodeSeed),
		"odd-length hex should fail at decode step, not length check")
}

func TestDeriveSeedPublicKey_UppercaseHex(t *testing.T) {
	seed := make([]byte, ed25519.SeedSize)
	for i := range seed {
		seed[i] = byte(i + 1)
	}
	seedHexUpper := hex.EncodeToString(seed)

	// Convert to uppercase
	upper := make([]byte, len(seedHexUpper))
	for i, c := range []byte(seedHexUpper) {
		if c >= 'a' && c <= 'f' {
			upper[i] = c - 32
		} else {
			upper[i] = c
		}
	}

	pubHex, err := deriveSeedPublicKey(string(upper))
	require.NoError(t, err)
	assert.Len(t, pubHex, 64)

	priv := ed25519.NewKeyFromSeed(seed)
	expectedPub := hex.EncodeToString(priv.Public().(ed25519.PublicKey))
	assert.Equal(t, expectedPub, pubHex, "uppercase hex should produce same key as lowercase")
}

func TestDeriveSeedPublicKey_31BytesTooShort(t *testing.T) {
	shortSeed := make([]byte, 31)
	for i := range shortSeed {
		shortSeed[i] = byte(i + 1)
	}
	shortHex := hex.EncodeToString(shortSeed)

	_, err := deriveSeedPublicKey(shortHex)
	require.Error(t, err)
	assert.True(t, errors.Is(err, constants.ErrInvalidSeedLength))
}

func TestDeriveSeedPublicKey_33BytesTooLong(t *testing.T) {
	longSeed := make([]byte, 33)
	for i := range longSeed {
		longSeed[i] = byte(i + 1)
	}
	longHex := hex.EncodeToString(longSeed)

	_, err := deriveSeedPublicKey(longHex)
	require.Error(t, err)
	assert.True(t, errors.Is(err, constants.ErrInvalidSeedLength))
}

func TestDeriveSeedPublicKey_AllZeroSeed(t *testing.T) {
	zeroSeed := make([]byte, ed25519.SeedSize)
	zeroHex := hex.EncodeToString(zeroSeed)

	pubHex, err := deriveSeedPublicKey(zeroHex)
	require.NoError(t, err)
	assert.Len(t, pubHex, 64)

	priv := ed25519.NewKeyFromSeed(zeroSeed)
	expectedPub := hex.EncodeToString(priv.Public().(ed25519.PublicKey))
	assert.Equal(t, expectedPub, pubHex)
}

func TestDeriveSeedPublicKey_DifferentSeedsProduceDifferentKeys(t *testing.T) {
	seed1 := make([]byte, ed25519.SeedSize)
	seed1[0] = 1
	seed2 := make([]byte, ed25519.SeedSize)
	seed2[0] = 2

	pub1, err := deriveSeedPublicKey(hex.EncodeToString(seed1))
	require.NoError(t, err)

	pub2, err := deriveSeedPublicKey(hex.EncodeToString(seed2))
	require.NoError(t, err)

	assert.NotEqual(t, pub1, pub2, "different seeds must produce different public keys")
}

func TestDeriveSeedPublicKey_TabAndNewlineTrimmed(t *testing.T) {
	seed := make([]byte, ed25519.SeedSize)
	for i := range seed {
		seed[i] = byte(i + 10)
	}
	seedHex := hex.EncodeToString(seed)

	padded := "\t\n" + seedHex + "\r\n\t"

	pubHex, err := deriveSeedPublicKey(padded)
	require.NoError(t, err)

	priv := ed25519.NewKeyFromSeed(seed)
	expectedPub := hex.EncodeToString(priv.Public().(ed25519.PublicKey))
	assert.Equal(t, expectedPub, pubHex)
}

func TestDeriveSeedPublicKey_PublicKeyIs64HexChars(t *testing.T) {
	seed := make([]byte, ed25519.SeedSize)
	for i := range seed {
		seed[i] = byte(0xFF)
	}
	seedHex := hex.EncodeToString(seed)

	pubHex, err := deriveSeedPublicKey(seedHex)
	require.NoError(t, err)
	assert.Len(t, pubHex, 64, "Ed25519 public key is 32 bytes = 64 hex chars")
}

// ---------------------------------------------------------------------------
// consensusPolicyBootstrap — additional error-path tests (Tier 1 — file I/O)
// ---------------------------------------------------------------------------

func TestBootstrapConsensusPolicy_PathIsDirectory(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{Level: slog.LevelError}))

	tmpDir := testutil.TempDir(t)
	// Pass the directory itself as the config path — os.ReadFile should fail
	err := consensusPolicyBootstrap(nil, tmpDir, constants.TestPathShortSecrets, logger)
	require.Error(t, err)
	assert.True(t, errors.Is(err, constants.ErrConsensusBootstrapReadConfig),
		"reading a directory should produce ErrConsensusBootstrapReadConfig")
}

func TestBootstrapConsensusPolicy_EmptyFile(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{Level: slog.LevelError}))

	tmpDir := testutil.TempDir(t)
	configPath := filepath.Join(tmpDir, constants.ConsensusBootstrapConfigFilename)
	require.NoError(t, os.WriteFile(configPath, []byte{}, 0600))

	err := consensusPolicyBootstrap(nil, configPath, constants.TestPathShortSecrets, logger)
	require.Error(t, err)
	// Empty file produces a JSON parse error (unexpected end of JSON input)
	assert.True(t, errors.Is(err, constants.ErrConsensusBootstrapParseConfig))
}

func TestBootstrapConsensusPolicy_ValidConfigNilServiceOnlyChecksServiceAfterParse(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{Level: slog.LevelError}))

	tmpDir := testutil.TempDir(t)
	configPath := filepath.Join(tmpDir, constants.ConsensusBootstrapConfigFilename)
	err := os.WriteFile(configPath, []byte(`{
		"consensus_id": "test-consensus",
		"member_app_ids": ["auditor-ensemble"],
		"quorum": 1,
		"seed_hex": "87278693f5894d8de5d28401c923e0c3fea9ae7c35f467065954eecbc85b2e77"
	}`), 0600)
	require.NoError(t, err)

	// With nil service, the file is read and parsed successfully, then
	// svc == nil check triggers ErrGatewayServiceNil (not a DB error).
	err = consensusPolicyBootstrap(nil, configPath, constants.TestPathShortSecrets, logger)
	require.Error(t, err)
	assert.True(t, errors.Is(err, constants.ErrGatewayServiceNil))
}

func TestBootstrapConsensusPolicy_NilServiceWithInvalidSeedHex(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{Level: slog.LevelError}))

	tmpDir := testutil.TempDir(t)
	configPath := filepath.Join(tmpDir, constants.ConsensusBootstrapConfigFilename)
	err := os.WriteFile(configPath, []byte(`{
		"consensus_id": "test-consensus",
		"member_app_ids": ["member-a"],
		"quorum": 1,
		"seed_hex": "not-valid-hex"
	}`), 0600)
	require.NoError(t, err)

	// Even with nil service, the nil check happens after parse but before
	// seed derivation. So we should get ErrGatewayServiceNil, not a seed error.
	err = consensusPolicyBootstrap(nil, configPath, constants.TestPathShortSecrets, logger)
	require.Error(t, err)
	assert.True(t, errors.Is(err, constants.ErrGatewayServiceNil),
		"nil service check occurs before seed derivation")
}

// TestBootstrapConsensusPolicy_NilServiceWithMemberSeeds proves the
// member_seeds JSON format parses cleanly through parseConsensusBootstrapConfig
// before the nil-service guard fires. This is the Tier 1 proof that
// member_seeds is a valid config format; validation that every member has a
// seed (ErrConsensusBootstrapMissingFields) is only reachable past the
// nil-service guard and is exercised at the Tier 2 bootstrap level with a
// real GatewayModeService fixture.
func TestBootstrapConsensusPolicy_NilServiceWithMemberSeeds(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{Level: slog.LevelError}))

	tmpDir := testutil.TempDir(t)
	configPath := filepath.Join(tmpDir, constants.ConsensusBootstrapConfigFilename)
	err := os.WriteFile(configPath, []byte(`{
		"consensus_id": "tribunal",
		"member_app_ids": ["alpha", "beta", "gamma"],
		"quorum": 2,
		"member_seeds": {
			"alpha": "b194523218024feacafef9acf9e557f9c2e6ed71e87c8c97e5a4fc61e624ea42",
			"beta":  "20544a8efd3f30188095dae9d42993f320fbcdcbd924f88b2c56edfdc719e357",
			"gamma": "06946f1a26896983176f6d40b0a734136dd58b16fe502d4b5688bf7db1b97662"
		}
	}`), 0600)
	require.NoError(t, err)

	// With nil service, the file is read and parsed successfully, then
	// svc == nil check triggers ErrGatewayServiceNil (not a parse error).
	// This proves the member_seeds format parses cleanly.
	err = consensusPolicyBootstrap(nil, configPath, constants.TestPathShortSecrets, logger)
	require.Error(t, err)
	assert.True(t, errors.Is(err, constants.ErrGatewayServiceNil),
		"member_seeds format must parse cleanly; nil service check fires after parse")
}

// ---------------------------------------------------------------------------
// ConsensusBootstrap — additional nil-service edge cases (Tier 1 — no DB)
// ---------------------------------------------------------------------------

func TestConsensusBootstrap_NilServiceWithNonEmptyConsensusID(t *testing.T) {
	_, priv, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)

	logger := slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{Level: slog.LevelError}))

	_, err = ConsensusBootstrap(nil, "some-consensus-id", priv, "actuator-key-id",
		constants.TestPathShortSecrets, logger)
	require.Error(t, err)
	assert.True(t, errors.Is(err, constants.ErrGatewayServiceNil))
}

func TestConsensusBootstrap_NilServiceWithEmptySecretsDir(t *testing.T) {
	_, priv, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)

	logger := slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{Level: slog.LevelError}))

	_, err = ConsensusBootstrap(nil, "trib-001", priv, "key-id", "", logger)
	require.Error(t, err)
	assert.True(t, errors.Is(err, constants.ErrGatewayServiceNil),
		"nil service check happens before secretsDir is used")
}

func TestConsensusBootstrap_NilServiceWithNilPrivateKey(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{Level: slog.LevelError}))

	_, err := ConsensusBootstrap(nil, "trib-001", nil, "key-id",
		constants.TestPathShortSecrets, logger)
	require.Error(t, err)
	assert.True(t, errors.Is(err, constants.ErrGatewayServiceNil),
		"nil service check happens before private key is used")
}

// ---------------------------------------------------------------------------
// GatewayConfig — ConsensusBootstrap field tests
// ---------------------------------------------------------------------------

func TestGatewayConfig_ConsensusBootstrapField(t *testing.T) {
	t.Run("default empty", func(t *testing.T) {
		var cfg GatewayConfig
		assert.Equal(t, "", cfg.ConsensusBootstrap)
	})

	t.Run("set to file path", func(t *testing.T) {
		cfg := GatewayConfig{
			ConsensusBootstrap: "/etc/g8e/consensus-bootstrap.json",
		}
		assert.Equal(t, "/etc/g8e/consensus-bootstrap.json", cfg.ConsensusBootstrap)
	})

	t.Run("not mapped to GatewayOptions", func(t *testing.T) {
		cfg := GatewayConfig{
			ConsensusBootstrap: "/etc/g8e/consensus-bootstrap.json",
		}
		opts := gatewayConfigToOptions(cfg)
		// ConsensusBootstrap is consumed directly by RunGateway, not passed to
		// config.LoadGateway via GatewayOptions.
		_ = opts // no ConsensusBootstrap field in GatewayOptions
	})
}

func TestGatewayConfig_ConsensusBootstrapDoesNotAffectOptions(t *testing.T) {
	cfgWithBootstrap := GatewayConfig{
		ConsensusBootstrap: "/path/to/bootstrap.json",
		Posture:            config.PostureConsensus,
	}
	cfgWithoutBootstrap := GatewayConfig{
		Posture: config.PostureConsensus,
	}

	optsWith := gatewayConfigToOptions(cfgWithBootstrap)
	optsWithout := gatewayConfigToOptions(cfgWithoutBootstrap)

	// The only difference should be ConsensusBootstrap, which is not in GatewayOptions
	// So the options should be identical (minus the ConsensusBootstrap field which doesn't exist in opts)
	assert.Equal(t, optsWith.Posture, optsWithout.Posture)
	assert.Equal(t, optsWith, optsWithout,
		"ConsensusBootstrap field should not affect GatewayOptions mapping")
}
