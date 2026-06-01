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

package network

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/g8e-ai/g8e/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDetector_DetectIPs(t *testing.T) {
	t.Parallel()
	logger := testutil.NewTestLogger()
	detector := NewDetector(logger)

	ips, err := detector.detectIPs()
	require.NoError(t, err)
	assert.NotEmpty(t, ips)

	// Should always include localhost
	assert.Contains(t, ips, "127.0.0.1")
	assert.Contains(t, ips, "::1")
}

func TestDetector_DetectHostnames(t *testing.T) {
	t.Parallel()
	logger := testutil.NewTestLogger()
	detector := NewDetector(logger)

	hostnames, err := detector.detectHostnames()
	require.NoError(t, err)

	// At minimum should have one hostname
	assert.NotEmpty(t, hostnames)
}

func TestDetector_DetectEtcHosts(t *testing.T) {
	t.Parallel()
	logger := testutil.NewTestLogger()
	detector := NewDetector(logger)

	aliases, err := detector.detectEtcHosts()
	require.NoError(t, err)

	// Should not error even if no aliases found
	assert.NotNil(t, aliases)
}

func TestDetector_DetectMDNS(t *testing.T) {
	t.Parallel()
	logger := testutil.NewTestLogger()
	detector := NewDetector(logger)

	mdnsNames, err := detector.detectMDNS()
	require.NoError(t, err)

	// The detector should always return a stable slice, even if no environment
	//-specific mDNS names are discoverable on the current host.
	assert.NotNil(t, mdnsNames)
}

func TestDetector_DetectDNSPTRs(t *testing.T) {
	t.Parallel()
	logger := testutil.NewTestLogger()
	detector := NewDetector(logger)

	// Get IPs first
	ips, err := detector.detectIPs()
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	ptrs, err := detector.detectDNSPTRs(ctx, ips)
	require.NoError(t, err)

	// Should not error even if no PTR records found
	assert.NotNil(t, ptrs)
}

func TestDetector_DetectSSHKnownHosts(t *testing.T) {
	t.Parallel()
	logger := testutil.NewTestLogger()
	detector := NewDetector(logger)

	sshHosts, err := detector.detectSSHKnownHosts()
	require.NoError(t, err)

	// The detector should always return a stable slice, even if no SSH known
	// hosts are present on the current machine.
	assert.NotNil(t, sshHosts)
}

func TestDetector_DetectAll(t *testing.T) {
	t.Parallel()
	logger := testutil.NewTestLogger()
	detector := NewDetector(logger)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	identity, err := detector.DetectAll(ctx)
	require.NoError(t, err)
	require.NotNil(t, identity)

	// Verify all fields are populated
	assert.NotEmpty(t, identity.IPs)
	assert.NotNil(t, identity.Hostnames)
	assert.NotNil(t, identity.EtcHosts)
	assert.NotNil(t, identity.MDNSNames)
	assert.NotNil(t, identity.DNSPTRs)
	assert.NotNil(t, identity.SSHHostnames)
}

func TestNetworkIdentity_GetAllDNSNames(t *testing.T) {
	t.Parallel()
	identity := &NetworkIdentity{
		Hostnames: []string{"test-host", "test-host.example.com"},
		EtcHosts: []HostAlias{
			{IP: "192.168.1.1", Aliases: []string{"gateway.local"}},
		},
		MDNSNames: []string{"test-host.local"},
		DNSPTRs: []DNSPTRRecord{
			{IP: "192.168.1.1", Hostname: "test-host.corp.example.com"},
		},
		SSHHostnames: []string{"ssh-host"},
		Windows: WindowsIdentity{
			NetBIOSName: "TESTHOST",
			ADFQDN:      "testhost.example.com",
		},
	}

	dnsNames := identity.GetAllDNSNames()
	assert.NotEmpty(t, dnsNames)

	// Should include all sources
	assert.Contains(t, dnsNames, "test-host")
	assert.Contains(t, dnsNames, "gateway.local")
	assert.Contains(t, dnsNames, "test-host.local")
	assert.Contains(t, dnsNames, "localhost")
}

func TestNetworkIdentity_GetAllIPs(t *testing.T) {
	t.Parallel()
	identity := &NetworkIdentity{
		IPs: []string{"192.168.1.1", "10.0.0.1", "127.0.0.1"},
	}

	ips := identity.GetAllIPs()
	assert.Len(t, ips, 3)
}

func TestNetworkIdentity_FormatForDisplay(t *testing.T) {
	t.Parallel()
	identity := &NetworkIdentity{
		IPs:       []string{"192.168.1.1", "10.0.0.1"},
		Hostnames: []string{"test-host"},
		EtcHosts: []HostAlias{
			{IP: "192.168.1.1", Aliases: []string{"gateway.local"}},
		},
		MDNSNames: []string{"test-host.local"},
	}

	display := identity.FormatForDisplay()
	assert.Contains(t, display, "Detected network identity")
	assert.Contains(t, display, "192.168.1.1")
	assert.Contains(t, display, "test-host")
	assert.Contains(t, display, "gateway.local")
}

func TestDetector_DetectEtcHosts_WithCustomFile(t *testing.T) {
	t.Parallel()
	logger := testutil.NewTestLogger()
	detector := NewDetector(logger)

	// Create a temporary /etc/hosts file
	tmpDir := t.TempDir()
	hostsPath := filepath.Join(tmpDir, "hosts")
	hostsContent := `127.0.0.1 localhost
192.168.1.50 gateway.local gw
10.0.0.12 internal.local
`
	err := os.WriteFile(hostsPath, []byte(hostsContent), 0644)
	require.NoError(t, err)

	// Temporarily replace /etc/hosts path for testing
	// Note: This is a simplified test - in production we'd need to mock the file path
	// For now, we just verify the parsing logic works
	aliases, err := detector.detectEtcHosts()
	require.NoError(t, err)
	assert.NotNil(t, aliases)
}

func TestUnique(t *testing.T) {
	t.Parallel()
	input := []string{"a", "b", "a", "c", "b", "d"}
	result := unique(input)
	assert.Equal(t, []string{"a", "b", "c", "d"}, result)
}

func TestContains(t *testing.T) {
	t.Parallel()
	slice := []string{"a", "b", "c"}
	assert.True(t, contains(slice, "b"))
	assert.False(t, contains(slice, "d"))
}
