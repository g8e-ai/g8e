// Copyright (c) 2026 Lateralus Labs, LLC.
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

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
// tribunalBootstrapConfig JSON tag and round-trip tests (Tier 1 — no DB)
// ---------------------------------------------------------------------------

func TestTribunalBootstrapConfig_JSONTags(t *testing.T) {
	boot := tribunalBootstrapConfig{
		TribunalID:   "test-tribunal",
		MemberAppIDs: []string{"alpha", "beta"},
		Quorum:       2,
		SeedHex:      "deadbeef",
	}

	data, err := json.Marshal(boot)
	require.NoError(t, err)

	var raw map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(data, &raw))

	assert.Contains(t, raw, "tribunal_id")
	assert.Contains(t, raw, "member_app_ids")
	assert.Contains(t, raw, "quorum")
	assert.Contains(t, raw, "seed_hex")
}

func TestTribunalBootstrapConfig_RoundTripMarshalUnmarshal(t *testing.T) {
	original := tribunalBootstrapConfig{
		TribunalID:   "round-trip-tribunal",
		MemberAppIDs: []string{"member-a", "member-b", "member-c"},
		Quorum:       2,
		SeedHex:      "87278693f5894d8de5d28401c923e0c3fea9ae7c35f467065954eecbc85b2e77",
	}

	data, err := json.Marshal(original)
	require.NoError(t, err)

	var decoded tribunalBootstrapConfig
	require.NoError(t, json.Unmarshal(data, &decoded))

	assert.Equal(t, original, decoded)
}

func TestTribunalBootstrapConfig_EmptySeedHex(t *testing.T) {
	boot := tribunalBootstrapConfig{
		TribunalID:   "no-seed-tribunal",
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
// parseTribunalBootstrapConfig — additional edge cases (Tier 1 — no DB)
// ---------------------------------------------------------------------------

func TestParseTribunalBootstrapConfig_NullJSON(t *testing.T) {
	_, err := parseTribunalBootstrapConfig([]byte("null"))
	require.Error(t, err)
	assert.True(t, errors.Is(err, constants.ErrTribunalBootstrapMissingFields),
		"null JSON produces zero-value struct which fails validation")
}

func TestParseTribunalBootstrapConfig_EmptyJSONObject(t *testing.T) {
	_, err := parseTribunalBootstrapConfig([]byte("{}"))
	require.Error(t, err)
	assert.True(t, errors.Is(err, constants.ErrTribunalBootstrapMissingFields))
}

func TestParseTribunalBootstrapConfig_WhitespaceOnlyData(t *testing.T) {
	_, err := parseTribunalBootstrapConfig([]byte("   \n\t  "))
	require.Error(t, err)
	assert.True(t, errors.Is(err, constants.ErrTribunalBootstrapParseConfig))
}

func TestParseTribunalBootstrapConfig_UnknownFieldsIgnored(t *testing.T) {
	data := []byte(`{
		"tribunal_id": "test-tribunal",
		"member_app_ids": ["member-a"],
		"quorum": 1,
		"seed_hex": "87278693f5894d8de5d28401c923e0c3fea9ae7c35f467065954eecbc85b2e77",
		"unknown_field": "should be ignored",
		"extra": 42
	}`)

	boot, err := parseTribunalBootstrapConfig(data)
	require.NoError(t, err)
	assert.Equal(t, "test-tribunal", boot.TribunalID)
	assert.Equal(t, []string{"member-a"}, boot.MemberAppIDs)
	assert.Equal(t, 1, boot.Quorum)
}

func TestParseTribunalBootstrapConfig_QuorumExceedsMemberCount(t *testing.T) {
	data := []byte(`{
		"tribunal_id": "test-tribunal",
		"member_app_ids": ["solo"],
		"quorum": 5
	}`)

	boot, err := parseTribunalBootstrapConfig(data)
	require.NoError(t, err)
	assert.Equal(t, 5, boot.Quorum, "parseTribunalBootstrapConfig does not validate quorum vs member count")
}

func TestParseTribunalBootstrapConfig_LargeQuorum(t *testing.T) {
	data := []byte(`{
		"tribunal_id": "test-tribunal",
		"member_app_ids": ["a", "b"],
		"quorum": 999999
	}`)

	boot, err := parseTribunalBootstrapConfig(data)
	require.NoError(t, err)
	assert.Equal(t, 999999, boot.Quorum)
}

func TestParseTribunalBootstrapConfig_SeedHexWithWhitespace(t *testing.T) {
	data := []byte(`{
		"tribunal_id": "test-tribunal",
		"member_app_ids": ["member-a"],
		"quorum": 1,
		"seed_hex": "  87278693f5894d8de5d28401c923e0c3fea9ae7c35f467065954eecbc85b2e77  "
	}`)

	boot, err := parseTribunalBootstrapConfig(data)
	require.NoError(t, err)
	assert.Equal(t, "  87278693f5894d8de5d28401c923e0c3fea9ae7c35f467065954eecbc85b2e77  ", boot.SeedHex,
		"parseTribunalBootstrapConfig does not trim seed_hex; trimming happens in deriveSeedPublicKey")
}

func TestParseTribunalBootstrapConfig_ManyMembers(t *testing.T) {
	members := make([]string, 100)
	for i := range members {
		members[i] = "member-" + string(rune('a'+i%26)) + string(rune('0'+i/26))
	}

	boot := tribunalBootstrapConfig{
		TribunalID:   "large-tribunal",
		MemberAppIDs: members,
		Quorum:       50,
		SeedHex:      "",
	}

	data, err := json.Marshal(boot)
	require.NoError(t, err)

	parsed, err := parseTribunalBootstrapConfig(data)
	require.NoError(t, err)
	assert.Len(t, parsed.MemberAppIDs, 100)
	assert.Equal(t, 50, parsed.Quorum)
}

func TestParseTribunalBootstrapConfig_DuplicateMembersAllowed(t *testing.T) {
	data := []byte(`{
		"tribunal_id": "test-tribunal",
		"member_app_ids": ["dup", "dup", "dup"],
		"quorum": 1
	}`)

	boot, err := parseTribunalBootstrapConfig(data)
	require.NoError(t, err)
	assert.Len(t, boot.MemberAppIDs, 3, "parseTribunalBootstrapConfig does not deduplicate members")
}

// ---------------------------------------------------------------------------
// deriveSeedPublicKey — additional edge cases (Tier 1 — no DB)
// ---------------------------------------------------------------------------

func TestDeriveSeedPublicKey_OddLengthHex(t *testing.T) {
	_, err := deriveSeedPublicKey("abc")
	require.Error(t, err)
	assert.True(t, errors.Is(err, constants.ErrTribunalBootstrapDecodeSeed),
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
// bootstrapTribunalPolicy — additional error-path tests (Tier 1 — file I/O)
// ---------------------------------------------------------------------------

func TestBootstrapTribunalPolicy_PathIsDirectory(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{Level: slog.LevelError}))

	tmpDir := testutil.TempDir(t)
	// Pass the directory itself as the config path — os.ReadFile should fail
	err := bootstrapTribunalPolicy(nil, tmpDir, constants.TestPathShortSecrets, logger)
	require.Error(t, err)
	assert.True(t, errors.Is(err, constants.ErrTribunalBootstrapReadConfig),
		"reading a directory should produce ErrTribunalBootstrapReadConfig")
}

func TestBootstrapTribunalPolicy_EmptyFile(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{Level: slog.LevelError}))

	tmpDir := testutil.TempDir(t)
	configPath := filepath.Join(tmpDir, constants.TribunalBootstrapConfigFilename)
	require.NoError(t, os.WriteFile(configPath, []byte{}, 0600))

	err := bootstrapTribunalPolicy(nil, configPath, constants.TestPathShortSecrets, logger)
	require.Error(t, err)
	// Empty file produces a JSON parse error (unexpected end of JSON input)
	assert.True(t, errors.Is(err, constants.ErrTribunalBootstrapParseConfig))
}

func TestBootstrapTribunalPolicy_ValidConfigNilServiceOnlyChecksServiceAfterParse(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{Level: slog.LevelError}))

	tmpDir := testutil.TempDir(t)
	configPath := filepath.Join(tmpDir, constants.TribunalBootstrapConfigFilename)
	err := os.WriteFile(configPath, []byte(`{
		"tribunal_id": "test-tribunal",
		"member_app_ids": ["auditor-ensemble"],
		"quorum": 1,
		"seed_hex": "87278693f5894d8de5d28401c923e0c3fea9ae7c35f467065954eecbc85b2e77"
	}`), 0600)
	require.NoError(t, err)

	// With nil service, the file is read and parsed successfully, then
	// svc == nil check triggers ErrGatewayServiceNil (not a DB error).
	err = bootstrapTribunalPolicy(nil, configPath, constants.TestPathShortSecrets, logger)
	require.Error(t, err)
	assert.True(t, errors.Is(err, constants.ErrGatewayServiceNil))
}

func TestBootstrapTribunalPolicy_NilServiceWithInvalidSeedHex(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{Level: slog.LevelError}))

	tmpDir := testutil.TempDir(t)
	configPath := filepath.Join(tmpDir, constants.TribunalBootstrapConfigFilename)
	err := os.WriteFile(configPath, []byte(`{
		"tribunal_id": "test-tribunal",
		"member_app_ids": ["member-a"],
		"quorum": 1,
		"seed_hex": "not-valid-hex"
	}`), 0600)
	require.NoError(t, err)

	// Even with nil service, the nil check happens after parse but before
	// seed derivation. So we should get ErrGatewayServiceNil, not a seed error.
	err = bootstrapTribunalPolicy(nil, configPath, constants.TestPathShortSecrets, logger)
	require.Error(t, err)
	assert.True(t, errors.Is(err, constants.ErrGatewayServiceNil),
		"nil service check occurs before seed derivation")
}

// ---------------------------------------------------------------------------
// BootstrapTribunal — additional nil-service edge cases (Tier 1 — no DB)
// ---------------------------------------------------------------------------

func TestBootstrapTribunal_NilServiceWithNonEmptyTribunalID(t *testing.T) {
	_, priv, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)

	logger := slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{Level: slog.LevelError}))

	_, err = BootstrapTribunal(nil, "some-tribunal-id", priv, "actuator-key-id",
		constants.TestPathShortSecrets, logger)
	require.Error(t, err)
	assert.True(t, errors.Is(err, constants.ErrGatewayServiceNil))
}

func TestBootstrapTribunal_NilServiceWithEmptySecretsDir(t *testing.T) {
	_, priv, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)

	logger := slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{Level: slog.LevelError}))

	_, err = BootstrapTribunal(nil, "trib-001", priv, "key-id", "", logger)
	require.Error(t, err)
	assert.True(t, errors.Is(err, constants.ErrGatewayServiceNil),
		"nil service check happens before secretsDir is used")
}

func TestBootstrapTribunal_NilServiceWithNilPrivateKey(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{Level: slog.LevelError}))

	_, err := BootstrapTribunal(nil, "trib-001", nil, "key-id",
		constants.TestPathShortSecrets, logger)
	require.Error(t, err)
	assert.True(t, errors.Is(err, constants.ErrGatewayServiceNil),
		"nil service check happens before private key is used")
}

// ---------------------------------------------------------------------------
// GatewayConfig — TribunalBootstrap field tests
// ---------------------------------------------------------------------------

func TestGatewayConfig_TribunalBootstrapField(t *testing.T) {
	t.Run("default empty", func(t *testing.T) {
		var cfg GatewayConfig
		assert.Equal(t, "", cfg.TribunalBootstrap)
	})

	t.Run("set to file path", func(t *testing.T) {
		cfg := GatewayConfig{
			TribunalBootstrap: "/etc/g8e/tribunal-bootstrap.json",
		}
		assert.Equal(t, "/etc/g8e/tribunal-bootstrap.json", cfg.TribunalBootstrap)
	})

	t.Run("not mapped to GatewayOptions", func(t *testing.T) {
		cfg := GatewayConfig{
			TribunalBootstrap: "/etc/g8e/tribunal-bootstrap.json",
		}
		opts := gatewayConfigToOptions(cfg)
		// TribunalBootstrap is consumed directly by RunGateway, not passed to
		// config.LoadGateway via GatewayOptions.
		_ = opts // no TribunalBootstrap field in GatewayOptions
	})
}

func TestGatewayConfig_TribunalBootstrapDoesNotAffectOptions(t *testing.T) {
	cfgWithBootstrap := GatewayConfig{
		TribunalBootstrap: "/path/to/bootstrap.json",
		Posture:           config.PostureConsensus,
	}
	cfgWithoutBootstrap := GatewayConfig{
		Posture: config.PostureConsensus,
	}

	optsWith := gatewayConfigToOptions(cfgWithBootstrap)
	optsWithout := gatewayConfigToOptions(cfgWithoutBootstrap)

	// The only difference should be TribunalBootstrap, which is not in GatewayOptions
	// So the options should be identical (minus the TribunalBootstrap field which doesn't exist in opts)
	assert.Equal(t, optsWith.Posture, optsWithout.Posture)
	assert.Equal(t, optsWith, optsWithout,
		"TribunalBootstrap field should not affect GatewayOptions mapping")
}
