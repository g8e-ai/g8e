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

package governancetest

import (
	"testing"

	"github.com/g8e-ai/g8e/internal/constants"
	"github.com/g8e-ai/g8e/internal/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSimpleAppPolicyStore_NilMap(t *testing.T) {
	t.Parallel()
	s := &SimpleAppPolicyStore{}
	policy, err := s.GetAppPolicy("app1")
	require.NoError(t, err)
	assert.Nil(t, policy)
}

func TestSimpleAppPolicyStore_NotFound(t *testing.T) {
	t.Parallel()
	s := &SimpleAppPolicyStore{Policies: map[string]*models.AppPolicy{}}
	policy, err := s.GetAppPolicy("nonexistent")
	require.NoError(t, err)
	assert.Nil(t, policy)
}

func TestSimpleAppPolicyStore_Found(t *testing.T) {
	t.Parallel()
	expected := &models.AppPolicy{AppID: "app1"}
	s := &SimpleAppPolicyStore{Policies: map[string]*models.AppPolicy{"app1": expected}}
	policy, err := s.GetAppPolicy("app1")
	require.NoError(t, err)
	assert.Equal(t, expected, policy)
}

func TestSimpleStateRootProvider_EmptyRoot(t *testing.T) {
	t.Parallel()
	s := &SimpleStateRootProvider{Root: ""}
	_, err := s.GetCurrentStateRoot()
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrTxProviderMisconfigured)
}

func TestSimpleStateRootProvider_ValidRoot(t *testing.T) {
	t.Parallel()
	s := &SimpleStateRootProvider{Root: "abc123"}
	root, err := s.GetCurrentStateRoot()
	require.NoError(t, err)
	assert.Equal(t, "abc123", root)
}

func TestSimpleTribunalStore_NilMap(t *testing.T) {
	t.Parallel()
	s := &SimpleTribunalStore{}
	tribunal, err := s.GetTribunal("trib1")
	require.NoError(t, err)
	assert.Nil(t, tribunal)
}

func TestSimpleTribunalStore_NotFound(t *testing.T) {
	t.Parallel()
	s := &SimpleTribunalStore{Tribunals: map[string]*models.TribunalPolicy{}}
	tribunal, err := s.GetTribunal("nonexistent")
	require.NoError(t, err)
	assert.Nil(t, tribunal)
}
