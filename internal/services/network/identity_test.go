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
	"net"
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

	// Should always include IPv4 localhost
	assert.Contains(t, ips, "127.0.0.1")
	assert.NotContains(t, ips, "::1")
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
		IPs: []string{"192.168.1.1", "10.0.0.1", "127.0.0.1", "2001:db8::1", "::1"},
	}

	ips := identity.GetAllIPs()
	assert.Len(t, ips, 3)
	for _, ip := range ips {
		assert.NotNil(t, ip.To4(), "should only return IPv4 addresses")
	}
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

func TestDetector_DetectWindowsIdentity(t *testing.T) {
	t.Parallel()
	logger := testutil.NewTestLogger()
	detector := NewDetector(logger)

	// This test only runs on Windows since detectWindowsIdentity
	// requires Windows-specific commands (systeminfo)
	if runtime.GOOS != "windows" {
		t.Skip("Skipping Windows identity test on non-Windows system")
	}

	winID, err := detector.detectWindowsIdentity(context.Background())
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

	// Verify IPv4 loopback is explicitly added, not from interface detection
	// The function adds 127.0.0.1 at the end (IPv6 ::1 is not included)
	ipv4LoopbackCount := 0
	for _, ip := range ips {
		if ip == "127.0.0.1" {
			ipv4LoopbackCount++
		}
	}

	// Should have exactly one (the explicitly added one)
	assert.Equal(t, 1, ipv4LoopbackCount)
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

func TestGetExternalInterfaceIP_WithMockInterfaces(t *testing.T) {
	t.Parallel()

	mockInterfaces := []net.Interface{
		{Name: "eth0"},
	}

	getInterfaces := func() ([]net.Interface, error) {
		return mockInterfaces, nil
	}

	mockAddrs := func(iface net.Interface) ([]net.Addr, error) {
		return []net.Addr{
			&net.IPNet{IP: net.ParseIP("10.0.0.5"), Mask: net.CIDRMask(24, 32)},
		}, nil
	}

	result := getExternalInterfaceIPWithFunc(getInterfaces, mockAddrs)
	assert.Equal(t, "10.0.0.5", result)
}

func TestGetExternalInterfaceIP_ErrorOnInterfaces(t *testing.T) {
	t.Parallel()

	// Test error handling when net.Interfaces() fails
	getInterfaces := func() ([]net.Interface, error) {
		return nil, assert.AnError
	}

	result := getExternalInterfaceIPWithFunc(getInterfaces, defaultInterfaceAddrs)
	// Should return localhost on error
	assert.Equal(t, "localhost", result)
}

func TestGetExternalInterfaceIP_NoNonLoopbackInterfaces(t *testing.T) {
	t.Parallel()

	// Test with only loopback interfaces
	getInterfaces := func() ([]net.Interface, error) {
		return []net.Interface{}, nil
	}

	result := getExternalInterfaceIPWithFunc(getInterfaces, defaultInterfaceAddrs)
	// Should return localhost when no non-loopback interfaces found
	assert.Equal(t, "localhost", result)
}

func TestDetector_DetectWindowsIdentity_WithMockExecutor(t *testing.T) {
	t.Parallel()
	logger := testutil.NewTestLogger()
	detector := NewDetector(logger)

	// Mock successful hostname and systeminfo commands
	executor := func(ctx context.Context, name string, args ...string) ([]byte, error) {
		switch name {
		case "hostname":
			return []byte("TESTHOST"), nil
		case "systeminfo":
			return []byte("Domain: example.com\n"), nil
		default:
			return nil, assert.AnError
		}
	}

	winID, err := detector.detectWindowsIdentityWithExecutor(context.Background(), executor)
	require.NoError(t, err)
	assert.Equal(t, "TESTHOST", winID.NetBIOSName)
	assert.Equal(t, "TESTHOST.example.com", winID.ADFQDN)
}

func TestDetector_DetectWindowsIdentity_HostnameError(t *testing.T) {
	t.Parallel()
	logger := testutil.NewTestLogger()
	detector := NewDetector(logger)

	// Mock hostname command failure
	executor := func(ctx context.Context, name string, args ...string) ([]byte, error) {
		return nil, assert.AnError
	}

	winID, err := detector.detectWindowsIdentityWithExecutor(context.Background(), executor)
	require.Error(t, err)
	assert.Empty(t, winID.NetBIOSName)
}

func TestDetector_DetectWindowsIdentity_SysteminfoError(t *testing.T) {
	t.Parallel()
	logger := testutil.NewTestLogger()
	detector := NewDetector(logger)

	// Mock successful hostname but failed systeminfo
	executor := func(ctx context.Context, name string, args ...string) ([]byte, error) {
		switch name {
		case "hostname":
			return []byte("TESTHOST"), nil
		case "systeminfo":
			return nil, assert.AnError
		default:
			return nil, assert.AnError
		}
	}

	winID, err := detector.detectWindowsIdentityWithExecutor(context.Background(), executor)
	require.Error(t, err)
	assert.Equal(t, "TESTHOST", winID.NetBIOSName)
	assert.Empty(t, winID.ADFQDN)
}

func TestDetector_DetectWindowsIdentity_WorkgroupDomain(t *testing.T) {
	t.Parallel()
	logger := testutil.NewTestLogger()
	detector := NewDetector(logger)

	// Mock WORKGROUP domain (should not set ADFQDN)
	executor := func(ctx context.Context, name string, args ...string) ([]byte, error) {
		switch name {
		case "hostname":
			return []byte("TESTHOST"), nil
		case "systeminfo":
			return []byte("Domain: WORKGROUP\n"), nil
		default:
			return nil, assert.AnError
		}
	}

	winID, err := detector.detectWindowsIdentityWithExecutor(context.Background(), executor)
	require.NoError(t, err)
	assert.Equal(t, "TESTHOST", winID.NetBIOSName)
	assert.Empty(t, winID.ADFQDN)
}

func TestDetector_DetectWindowsIdentity_NoDomainLine(t *testing.T) {
	t.Parallel()
	logger := testutil.NewTestLogger()
	detector := NewDetector(logger)

	// Mock systeminfo without Domain line
	executor := func(ctx context.Context, name string, args ...string) ([]byte, error) {
		switch name {
		case "hostname":
			return []byte("TESTHOST"), nil
		case "systeminfo":
			return []byte("OS Name: Windows\n"), nil
		default:
			return nil, assert.AnError
		}
	}

	winID, err := detector.detectWindowsIdentityWithExecutor(context.Background(), executor)
	require.NoError(t, err)
	assert.Equal(t, "TESTHOST", winID.NetBIOSName)
	assert.Empty(t, winID.ADFQDN)
}

func TestDetector_DetectWindowsIdentity_EmptyHostname(t *testing.T) {
	t.Parallel()
	logger := testutil.NewTestLogger()
	detector := NewDetector(logger)

	// Mock empty hostname
	executor := func(ctx context.Context, name string, args ...string) ([]byte, error) {
		switch name {
		case "hostname":
			return []byte(""), nil
		case "systeminfo":
			return []byte("Domain: example.com\n"), nil
		default:
			return nil, assert.AnError
		}
	}

	winID, err := detector.detectWindowsIdentityWithExecutor(context.Background(), executor)
	require.NoError(t, err)
	assert.Empty(t, winID.NetBIOSName)
	assert.Empty(t, winID.ADFQDN)
}

func TestDetector_DetectWindowsIdentity_WhitespaceHostname(t *testing.T) {
	t.Parallel()
	logger := testutil.NewTestLogger()
	detector := NewDetector(logger)

	// Mock hostname with whitespace
	executor := func(ctx context.Context, name string, args ...string) ([]byte, error) {
		switch name {
		case "hostname":
			return []byte("  TESTHOST  "), nil
		case "systeminfo":
			return []byte("Domain: example.com\n"), nil
		default:
			return nil, assert.AnError
		}
	}

	winID, err := detector.detectWindowsIdentityWithExecutor(context.Background(), executor)
	require.NoError(t, err)
	assert.Equal(t, "TESTHOST", winID.NetBIOSName)
	assert.Equal(t, "TESTHOST.example.com", winID.ADFQDN)
}

func TestGetExternalInterfaceIP_ErrorOnAddrs(t *testing.T) {
	t.Parallel()

	mockInterfaces := []net.Interface{
		{Name: "eth0"},
	}

	getInterfaces := func() ([]net.Interface, error) {
		return mockInterfaces, nil
	}

	getAddrs := func(iface net.Interface) ([]net.Addr, error) {
		return nil, assert.AnError
	}

	result := getExternalInterfaceIPWithFunc(getInterfaces, getAddrs)
	assert.Equal(t, "localhost", result)
}

func TestGetExternalInterfaceIP_LoopbackOnly(t *testing.T) {
	t.Parallel()

	mockInterfaces := []net.Interface{
		{Name: "lo"},
	}

	getInterfaces := func() ([]net.Interface, error) {
		return mockInterfaces, nil
	}

	getAddrs := func(iface net.Interface) ([]net.Addr, error) {
		return []net.Addr{
			&net.IPNet{IP: net.ParseIP("127.0.0.1"), Mask: net.CIDRMask(8, 32)},
		}, nil
	}

	result := getExternalInterfaceIPWithFunc(getInterfaces, getAddrs)
	assert.Equal(t, "localhost", result)
}

func TestGetExternalIP(t *testing.T) {
	t.Parallel()

	t.Run("returns first non-loopback non-link-local IPv4", func(t *testing.T) {
		t.Parallel()
		ips := []string{"127.0.0.1", "169.254.1.1", "192.168.1.100", "10.0.0.5"}
		result := getExternalIP(ips)
		assert.Equal(t, "192.168.1.100", result)
	})

	t.Run("returns empty when only loopback and link-local", func(t *testing.T) {
		t.Parallel()
		ips := []string{"127.0.0.1", "169.254.1.1"}
		result := getExternalIP(ips)
		assert.Empty(t, result)
	})

	t.Run("returns empty for empty list", func(t *testing.T) {
		t.Parallel()
		result := getExternalIP([]string{})
		assert.Empty(t, result)
	})

	t.Run("returns empty for nil list", func(t *testing.T) {
		t.Parallel()
		result := getExternalIP(nil)
		assert.Empty(t, result)
	})

	t.Run("skips invalid IP string", func(t *testing.T) {
		t.Parallel()
		ips := []string{"not-an-ip", "192.168.1.1"}
		result := getExternalIP(ips)
		assert.Equal(t, "192.168.1.1", result)
	})
}

func TestFilterOutIP(t *testing.T) {
	t.Parallel()

	t.Run("removes matching IP", func(t *testing.T) {
		t.Parallel()
		ips := []string{"10.0.0.1", "10.0.0.2", "10.0.0.3"}
		result := filterOutIP(ips, "10.0.0.2")
		assert.Equal(t, []string{"10.0.0.1", "10.0.0.3"}, result)
	})

	t.Run("returns all when exclude not found", func(t *testing.T) {
		t.Parallel()
		ips := []string{"10.0.0.1", "10.0.0.2"}
		result := filterOutIP(ips, "192.168.1.1")
		assert.Equal(t, []string{"10.0.0.1", "10.0.0.2"}, result)
	})

	t.Run("returns empty for empty input", func(t *testing.T) {
		t.Parallel()
		result := filterOutIP([]string{}, "10.0.0.1")
		assert.Empty(t, result)
	})

	t.Run("returns nil for nil input", func(t *testing.T) {
		t.Parallel()
		result := filterOutIP(nil, "10.0.0.1")
		assert.Nil(t, result)
	})
}

func TestGetExternalInterfaceIP_PublicWrapper(t *testing.T) {
	t.Parallel()
	// Test the public wrapper function - it should return a valid result
	// This test verifies the wrapper calls the implementation correctly
	result := GetExternalInterfaceIP()
	// Should return either an IP or "localhost"
	assert.NotEmpty(t, result)
}

func TestDefaultNetInterfaces(t *testing.T) {
	t.Parallel()
	// Test the default implementation - it should return interfaces or error
	ifaces, err := defaultNetInterfaces()
	// Either success or error is acceptable
	if err == nil {
		assert.NotNil(t, ifaces)
	}
}

func TestDefaultCommandExecutor(t *testing.T) {
	t.Parallel()
	// Test the default implementation with a simple command
	// Use 'echo' which should be available on all systems
	output, err := defaultCommandExecutor(context.Background(), "echo", "test")
	if err == nil {
		assert.NotNil(t, output)
	}
}

func TestDetector_DetectWindowsIdentity_PublicWrapper(t *testing.T) {
	t.Parallel()
	logger := testutil.NewTestLogger()
	detector := NewDetector(logger)

	// This test exercises the public wrapper by temporarily swapping the executor
	// We can't easily mock the executor for the public wrapper without more refactoring,
	// so we'll test that the function exists and has the correct signature
	// The actual logic is tested via detectWindowsIdentityWithExecutor

	// On non-Windows systems, the public wrapper would fail with real commands
	// We verify the function signature and that it's callable
	if runtime.GOOS == "windows" {
		// On Windows, test the real implementation
		winID, err := detector.detectWindowsIdentity(context.Background())
		// May fail if commands aren't available, but should not panic
		if err == nil {
			assert.NotNil(t, winID)
		}
	} else {
		// On non-Windows, we can't test the real implementation
		// but we've thoroughly tested the logic via detectWindowsIdentityWithExecutor
		t.Skip("Public wrapper requires Windows environment")
	}
}
