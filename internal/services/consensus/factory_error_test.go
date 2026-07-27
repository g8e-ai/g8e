package consensus

import (
	"crypto/ed25519"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/internal/constants"
	"github.com/g8e-ai/g8e/internal/models"
	"github.com/g8e-ai/g8e/internal/response"
)

func TestKeyProviderFunc_GetMemberKey(t *testing.T) {
	t.Parallel()

	_, expectedKey, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)

	fn := KeyProviderFunc(func(appID string) (ed25519.PrivateKey, error) {
		assert.Equal(t, "test-app", appID)
		return expectedKey, nil
	})

	key, err := fn.GetMemberKey("test-app")
	require.NoError(t, err)
	assert.Equal(t, expectedKey, key)
}

func TestKeyProviderFunc_GetMemberKey_Error(t *testing.T) {
	t.Parallel()

	expectedErr := errors.New("key not found")
	fn := KeyProviderFunc(func(appID string) (ed25519.PrivateKey, error) {
		return nil, expectedErr
	})

	key, err := fn.GetMemberKey("missing-app")
	require.Error(t, err)
	assert.Nil(t, key)
	assert.ErrorIs(t, err, expectedErr)
}

func TestNewConsensusFromPolicy_NilPolicy(t *testing.T) {
	t.Parallel()

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	responder := response.NewWriter(logger)

	_, priv, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)

	provider := KeyProviderFunc(func(appID string) (ed25519.PrivateKey, error) {
		return priv, nil
	})

	svc, err := NewConsensusFromPolicy(nil, provider, nil, logger, responder)
	require.Error(t, err)
	assert.Nil(t, svc)
	assert.ErrorIs(t, err, constants.ErrConsensusFactoryNilPolicy)
}

func TestNewConsensusFromPolicy_NilKeyProvider(t *testing.T) {
	t.Parallel()

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	responder := response.NewWriter(logger)

	policy := &models.ConsensusPolicy{
		ID:           "test-consensus",
		MemberAppIDs: []string{"member-1"},
		Quorum:       1,
	}

	svc, err := NewConsensusFromPolicy(policy, nil, nil, logger, responder)
	require.Error(t, err)
	assert.Nil(t, svc)
	assert.ErrorIs(t, err, constants.ErrConsensusFactoryNilKeyProvider)
}

func TestNewConsensusFromPolicy_KeyProviderError(t *testing.T) {
	t.Parallel()

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	responder := response.NewWriter(logger)

	policy := &models.ConsensusPolicy{
		ID:           "test-consensus",
		MemberAppIDs: []string{"member-1", "member-2"},
		Quorum:       1,
	}

	_, priv, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)

	callCount := 0
	provider := KeyProviderFunc(func(appID string) (ed25519.PrivateKey, error) {
		callCount++
		if appID == "member-1" {
			return priv, nil
		}
		return nil, fmt.Errorf("key not available for %s", appID)
	})

	svc, err := NewConsensusFromPolicy(policy, provider, nil, logger, responder)
	require.NoError(t, err)
	require.NotNil(t, svc)
	assert.Equal(t, 2, callCount, "both members should be queried")
}

func TestNewConsensusFromPolicy_AllKeysResolved(t *testing.T) {
	t.Parallel()

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	responder := response.NewWriter(logger)

	_, priv1, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)
	_, priv2, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)

	keys := map[string]ed25519.PrivateKey{
		"member-1": priv1,
		"member-2": priv2,
	}

	policy := &models.ConsensusPolicy{
		ID:           "test-consensus",
		MemberAppIDs: []string{"member-1", "member-2"},
		Quorum:       2,
	}

	provider := KeyProviderFunc(func(appID string) (ed25519.PrivateKey, error) {
		key, ok := keys[appID]
		if !ok {
			return nil, fmt.Errorf("unknown member: %s", appID)
		}
		return key, nil
	})

	svc, err := NewConsensusFromPolicy(policy, provider, nil, logger, responder)
	require.NoError(t, err)
	require.NotNil(t, svc)
	assert.Equal(t, "test-consensus", svc.consensusID)
	assert.Len(t, svc.members, 2)
	assert.Equal(t, "member-1", svc.members[0].AppID)
	assert.Equal(t, priv1, svc.members[0].PrivateKey)
	assert.Equal(t, "member-2", svc.members[1].AppID)
	assert.Equal(t, priv2, svc.members[1].PrivateKey)
}

func TestNewConsensusFromPolicy_EmptyMemberList(t *testing.T) {
	t.Parallel()

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	responder := response.NewWriter(logger)

	policy := &models.ConsensusPolicy{
		ID:           "empty-consensus",
		MemberAppIDs: []string{},
		Quorum:       0,
	}

	provider := KeyProviderFunc(func(appID string) (ed25519.PrivateKey, error) {
		return nil, fmt.Errorf("should not be called")
	})

	svc, err := NewConsensusFromPolicy(policy, provider, nil, logger, responder)
	require.NoError(t, err)
	require.NotNil(t, svc)
	assert.Empty(t, svc.members)
}
