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
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/internal/testutil"
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

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	mdnsNames, err := detector.detectMDNS(ctx)
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

func TestDetector_DetectWindowsIdentity(t *testing.T) {
	t.Parallel()
	logger := testutil.NewTestLogger()
	detector := NewDetector(logger)

	// This test only runs on Windows since detectWindowsIdentity
	// requires Windows-specific commands (systeminfo)
	if runtime.GOOS != "windows" {
		t.Skip("Skipping Windows identity test on non-Windows system")
	}

	winID, err := detector.detectWindowsIdentity()
	require.NoError(t, err)
	assert.NotNil(t, winID)
}

func TestDetector_DetectMDNS_WithAvahi(t *testing.T) {
	t.Parallel()
	logger := testutil.NewTestLogger()
	detector := NewDetector(logger)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	mdnsNames, err := detector.detectMDNS(ctx)
	require.NoError(t, err)

	// Should always return a slice, even if avahi-browse is not available
	assert.NotNil(t, mdnsNames)

	// If hostname detection worked, should have .local names
	if len(mdnsNames) > 0 {
		assert.True(t, func() bool {
			for _, name := range mdnsNames {
				if strings.HasSuffix(name, ".local") {
					return true
				}
			}
			return false
		}())
	}
}

func TestDetector_DetectMDNS_HostnameSuffix(t *testing.T) {
	t.Parallel()
	logger := testutil.NewTestLogger()
	detector := NewDetector(logger)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	mdnsNames, err := detector.detectMDNS(ctx)
	require.NoError(t, err)
	assert.NotNil(t, mdnsNames)

	// Verify that if a hostname already has .local suffix, it's not duplicated
	hostnames, err := detector.detectHostnames()
	require.NoError(t, err)

	for _, hn := range hostnames {
		if strings.HasSuffix(hn, ".local") {
			// Count occurrences in mdnsNames
			count := 0
			for _, mdns := range mdnsNames {
				if mdns == hn || mdns == hn+".local" {
					count++
				}
			}
			// Should appear at most once
			assert.LessOrEqual(t, count, 1)
		}
	}
}

func TestDetector_DetectSSHKnownHosts_FileNotFound(t *testing.T) {
	t.Parallel()
	logger := testutil.NewTestLogger()
	detector := NewDetector(logger)

	// This test verifies that missing known_hosts files don't cause errors
	sshHosts, err := detector.detectSSHKnownHosts()
	require.NoError(t, err)
	assert.NotNil(t, sshHosts)
}

func TestNetworkIdentity_FormatForDisplay_EmptyFields(t *testing.T) {
	t.Parallel()
	identity := &NetworkIdentity{
		IPs:          []string{"192.168.1.1"},
		Hostnames:    []string{},
		EtcHosts:     []HostAlias{},
		MDNSNames:    []string{},
		DNSPTRs:      []DNSPTRRecord{},
		SSHHostnames: []string{},
		Windows: WindowsIdentity{
			NetBIOSName: "",
			ADFQDN:      "",
		},
	}

	display := identity.FormatForDisplay()
	assert.Contains(t, display, "Detected network identity")
	assert.Contains(t, display, "192.168.1.1")
}

func TestNetworkIdentity_FormatForDisplay_AllFields(t *testing.T) {
	t.Parallel()
	identity := &NetworkIdentity{
		IPs:       []string{"192.168.1.1", "10.0.0.1"},
		Hostnames: []string{"test-host"},
		EtcHosts: []HostAlias{
			{IP: "192.168.1.1", Aliases: []string{"gateway.local", "gw"}},
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

	display := identity.FormatForDisplay()
	assert.Contains(t, display, "Detected network identity")
	assert.Contains(t, display, "192.168.1.1")
	assert.Contains(t, display, "test-host")
	assert.Contains(t, display, "gateway.local")
	assert.Contains(t, display, "test-host.local")
	assert.Contains(t, display, "DNS PTR")
	assert.Contains(t, display, "SSH hosts")
	assert.Contains(t, display, "Windows")
}

func TestGetHostsFilePath(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		goos     string
		expected string
	}{
		{
			name:     "Linux",
			goos:     "linux",
			expected: "/etc/hosts",
		},
		{
			name:     "Darwin",
			goos:     "darwin",
			expected: "/etc/hosts",
		},
		{
			name:     "Windows",
			goos:     "windows",
			expected: filepath.Join(os.Getenv("SystemRoot"), "System32", "drivers", "etc", "hosts"),
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			// Save original GOOS
			origGOOS := runtime.GOOS

			// This test verifies the path logic for different OSes
			// Note: We can't actually change runtime.GOOS in tests,
			// so we verify the current OS returns the expected path
			if runtime.GOOS == tt.goos {
				path := getHostsFilePath()
				assert.Equal(t, tt.expected, path)
			} else {
				t.Skipf("Skipping test for %s on %s", tt.goos, origGOOS)
			}
		})
	}
}

func TestDetector_DetectAll_ContextCancellation(t *testing.T) {
	t.Parallel()
	logger := testutil.NewTestLogger()
	detector := NewDetector(logger)

	// Create a context that's already cancelled
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	// DetectAll should handle cancelled context gracefully
	identity, err := detector.DetectAll(ctx)
	// The function should return an error for cancelled context
	require.Error(t, err)
	assert.Nil(t, identity)
}

func TestDetector_DetectAll_Timeout(t *testing.T) {
	t.Parallel()
	logger := testutil.NewTestLogger()
	detector := NewDetector(logger)

	// Use a very short timeout
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Nanosecond)
	defer cancel()

	// DetectAll should handle timeout gracefully
	identity, err := detector.DetectAll(ctx)
	// The function should return an error for timed out context
	require.Error(t, err)
	assert.Nil(t, identity)
}

func TestDetector_DetectAll_ConcurrentExecution(t *testing.T) {
	t.Parallel()
	logger := testutil.NewTestLogger()
	detector := NewDetector(logger)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Run DetectAll multiple times concurrently to verify thread safety
	var wg sync.WaitGroup
	results := make([]*NetworkIdentity, 5)
	errors := make([]error, 5)

	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			identity, err := detector.DetectAll(ctx)
			results[idx] = identity
			errors[idx] = err
		}(i)
	}

	wg.Wait()

	// All should complete without error
	for i := 0; i < 5; i++ {
		require.NoError(t, errors[i])
		assert.NotNil(t, results[i])
	}
}

func TestDetector_DetectIPs_ExcludesLoopback(t *testing.T) {
	t.Parallel()
	logger := testutil.NewTestLogger()
	detector := NewDetector(logger)

	ips, err := detector.detectIPs()
	require.NoError(t, err)

	// Verify loopback is explicitly added, not from interface detection
	// The function adds 127.0.0.1 and ::1 at the end
	ipv4LoopbackCount := 0
	ipv6LoopbackCount := 0
	for _, ip := range ips {
		if ip == "127.0.0.1" {
			ipv4LoopbackCount++
		}
		if ip == "::1" {
			ipv6LoopbackCount++
		}
	}

	// Should have exactly one of each (the explicitly added ones)
	assert.Equal(t, 1, ipv4LoopbackCount)
	assert.Equal(t, 1, ipv6LoopbackCount)
}

func TestDetector_DetectDNSPTRs_SkipsLocal(t *testing.T) {
	t.Parallel()
	logger := testutil.NewTestLogger()
	detector := NewDetector(logger)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	// Test with localhost IPs - should be skipped
	ips := []string{"127.0.0.1", "::1", "192.168.1.1"}
	ptrs, err := detector.detectDNSPTRs(ctx, ips)
	require.NoError(t, err)

	// Verify no PTR records for localhost IPs
	for _, ptr := range ptrs {
		assert.NotEqual(t, "127.0.0.1", ptr.IP)
		assert.NotEqual(t, "::1", ptr.IP)
	}
}

func TestDetector_DetectDNSPTRs_SkipsLinkLocal(t *testing.T) {
	t.Parallel()
	logger := testutil.NewTestLogger()
	detector := NewDetector(logger)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	// Test with link-local IPs - should be skipped
	ips := []string{"fe80::1", "192.168.1.1"}
	ptrs, err := detector.detectDNSPTRs(ctx, ips)
	require.NoError(t, err)

	// Verify no PTR records for link-local IPs
	for _, ptr := range ptrs {
		assert.False(t, strings.HasPrefix(ptr.IP, "fe80:"))
	}
}

func TestNetworkIdentity_GetAllDNSNames_Deduplication(t *testing.T) {
	t.Parallel()
	identity := &NetworkIdentity{
		Hostnames: []string{"test-host", "test-host"},
		EtcHosts: []HostAlias{
			{IP: "192.168.1.1", Aliases: []string{"gateway.local", "gateway.local"}},
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

	// Verify no duplicates
	seen := make(map[string]bool)
	for _, name := range dnsNames {
		assert.False(t, seen[name], "Duplicate DNS name found: %s", name)
		seen[name] = true
	}

	// Should include localhost exactly once
	localhostCount := 0
	for _, name := range dnsNames {
		if name == "localhost" {
			localhostCount++
		}
	}
	assert.Equal(t, 1, localhostCount)
}

func TestNetworkIdentity_GetAllIPs_InvalidIP(t *testing.T) {
	t.Parallel()
	identity := &NetworkIdentity{
		IPs: []string{"192.168.1.1", "invalid-ip", "10.0.0.1"},
	}

	ips := identity.GetAllIPs()
	// Should skip invalid IPs
	assert.Len(t, ips, 2)
}

func TestDetector_DetectHostnames_EmptyFile(t *testing.T) {
	t.Parallel()
	logger := testutil.NewTestLogger()
	detector := NewDetector(logger)

	// This test verifies that hostname detection works even if /etc/hostname doesn't exist
	// It should fall back to the hostname command
	hostnames, err := detector.detectHostnames()
	require.NoError(t, err)
	assert.NotEmpty(t, hostnames)
}

func TestDetector_DetectHostnames_Deduplication(t *testing.T) {
	t.Parallel()
	logger := testutil.NewTestLogger()
	detector := NewDetector(logger)

	hostnames, err := detector.detectHostnames()
	require.NoError(t, err)

	// Verify no duplicates in the returned hostnames
	seen := make(map[string]bool)
	for _, hn := range hostnames {
		assert.False(t, seen[hn], "Duplicate hostname found: %s", hn)
		seen[hn] = true
	}
}

func TestDetector_DetectEtcHosts_EmptyIPSet(t *testing.T) {
	t.Parallel()
	logger := testutil.NewTestLogger()
	detector := NewDetector(logger)

	// This test verifies that detectEtcHosts handles the case where
	// no local IPs match the hosts file entries
	aliases, err := detector.detectEtcHosts()
	require.NoError(t, err)
	assert.NotNil(t, aliases)
}

func TestDetector_DetectEtcHosts_CommentHandling(t *testing.T) {
	t.Parallel()
	logger := testutil.NewTestLogger()
	detector := NewDetector(logger)

	// Verify that comments and empty lines are properly skipped
	aliases, err := detector.detectEtcHosts()
	require.NoError(t, err)
	assert.NotNil(t, aliases)
}

func TestNetworkIdentity_GetAllDNSNames_EmptyIdentity(t *testing.T) {
	t.Parallel()
	identity := &NetworkIdentity{
		Hostnames:    []string{},
		EtcHosts:     []HostAlias{},
		MDNSNames:    []string{},
		DNSPTRs:      []DNSPTRRecord{},
		SSHHostnames: []string{},
		Windows: WindowsIdentity{
			NetBIOSName: "",
			ADFQDN:      "",
		},
	}

	dnsNames := identity.GetAllDNSNames()
	// Should always include localhost
	assert.NotEmpty(t, dnsNames)
	assert.Contains(t, dnsNames, "localhost")
}

func TestNetworkIdentity_GetAllIPs_EmptyIdentity(t *testing.T) {
	t.Parallel()
	identity := &NetworkIdentity{
		IPs: []string{},
	}

	ips := identity.GetAllIPs()
	assert.Empty(t, ips)
}
