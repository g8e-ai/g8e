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

package gateway

import (
	"context"
	"encoding/json"
	"log/slog"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/g8e-ai/g8e/internal/services/network"
	"github.com/g8e-ai/g8e/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type staticNetworkIdentityDetector struct {
	identity *network.NetworkIdentity
	err      error
}

func (d staticNetworkIdentityDetector) DetectAll(context.Context) (*network.NetworkIdentity, error) {
	return d.identity, d.err
}

func TestResolveGatewayCertificateIdentity_LoadsIdentityFromFile(t *testing.T) {
	t.Parallel()

	logger := testutil.NewTestLogger()
	dir := t.TempDir()
	identityFile := filepath.Join(dir, "network-identity.json")

	identity := network.NetworkIdentity{
		IPs:       []string{"192.0.2.10", "2001:db8::10"},
		Hostnames: []string{"gateway.test.internal"},
	}
	identityData, err := json.Marshal(identity)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(identityFile, identityData, 0600))

	extraIPs, extraDNSNames, err := resolveGatewayCertificateIdentity("full", identityFile, staticNetworkIdentityDetector{}, logger)
	require.NoError(t, err)
	assert.Equal(t, []net.IP{net.ParseIP("192.0.2.10"), net.ParseIP("2001:db8::10")}, extraIPs)
	assert.Contains(t, extraDNSNames, "gateway.test.internal")
	assert.Contains(t, extraDNSNames, "localhost")
}

func TestResolveGatewayCertificateIdentity_FallsBackToDetectionWhenFileMissing(t *testing.T) {
	t.Parallel()

	logger := testutil.NewTestLogger()
	identity := &network.NetworkIdentity{
		IPs:       []string{"198.51.100.20"},
		Hostnames: []string{"detected.example"},
	}

	extraIPs, extraDNSNames, err := resolveGatewayCertificateIdentity("full", "", staticNetworkIdentityDetector{identity: identity}, logger)
	require.NoError(t, err)
	assert.Equal(t, []net.IP{net.ParseIP("198.51.100.20")}, extraIPs)
	assert.Contains(t, extraDNSNames, "detected.example")
	assert.Contains(t, extraDNSNames, "localhost")
}

func TestResolveGatewayCertificateIdentity_LocalhostFiltersLoopbackOnly(t *testing.T) {
	t.Parallel()

	logger := testutil.NewTestLogger()
	identity := &network.NetworkIdentity{
		IPs: []string{"127.0.0.1", "::1", "192.0.2.33"},
	}

	extraIPs, extraDNSNames, err := resolveGatewayCertificateIdentity("localhost", "", staticNetworkIdentityDetector{identity: identity}, logger)
	require.NoError(t, err)
	assert.Equal(t, []net.IP{net.ParseIP("127.0.0.1"), net.ParseIP("::1")}, extraIPs)
	assert.Equal(t, []string{"localhost"}, extraDNSNames)
}

func TestResolveGatewayCertificateIdentity_MalformedIdentityFile(t *testing.T) {
	t.Parallel()

	logger := testutil.NewTestLogger()
	dir := t.TempDir()
	identityFile := filepath.Join(dir, "network-identity.json")
	require.NoError(t, os.WriteFile(identityFile, []byte("not-json"), 0600))

	_, _, err := resolveGatewayCertificateIdentity("full", identityFile, staticNetworkIdentityDetector{}, logger)
	require.Error(t, err)
	assert.Contains(t, strings.ToLower(err.Error()), "unmarshal")
}

func TestResolveGatewayCertificateIdentity_DetectFallbackErrorUsesBasicIPv4(t *testing.T) {
	t.Parallel()

	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	_, extraDNSNames, err := resolveGatewayCertificateIdentity("full", "", staticNetworkIdentityDetector{err: context.DeadlineExceeded}, logger)
	require.NoError(t, err)
	assert.Nil(t, extraDNSNames)
}
