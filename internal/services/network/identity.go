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
	"bufio"
	"context"
	"fmt"
	"log/slog"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/g8e-ai/g8e/internal/constants"
	"github.com/g8e-ai/g8e/internal/sliceutil"
)

// GetExternalInterfaceIP returns the first non-loopback IPv4 address found on the host.
// This is used for the Operator Bootstrap endpoint which remote operators rely on.
func GetExternalInterfaceIP() string {
	ifaces, err := net.Interfaces()
	if err != nil {
		return "localhost"
	}

	for _, iface := range ifaces {
		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}
		for _, addr := range addrs {
			var ip net.IP
			switch v := addr.(type) {
			case *net.IPNet:
				ip = v.IP
			case *net.IPAddr:
				ip = v.IP
			}
			if ip != nil && !ip.IsLoopback() && ip.To4() != nil {
				return ip.String()
			}
		}
	}

	return "localhost"
}

// NetworkIdentity represents all detected network identities for the machine.
type NetworkIdentity struct {
	IPs          []string
	Hostnames    []string
	EtcHosts     []HostAlias
	MDNSNames    []string
	DNSPTRs      []DNSPTRRecord
	SSHHostnames []string
	Windows      WindowsIdentity
}

// HostAlias represents an entry from the hosts file pointing to this machine.
type HostAlias struct {
	IP      string
	Aliases []string
}

// DNSPTRRecord represents a DNS PTR record for an IP.
type DNSPTRRecord struct {
	IP       string
	Hostname string
}

// WindowsIdentity represents Windows-specific network identities.
type WindowsIdentity struct {
	NetBIOSName string
	ADFQDN      string
}

// Detector handles network identity detection.
type Detector struct {
	logger *slog.Logger
}

// NewDetector creates a new network identity detector.
func NewDetector(logger *slog.Logger) *Detector {
	return &Detector{
		logger: logger,
	}
}

// DetectAll performs comprehensive network identity detection.
func (d *Detector) DetectAll(ctx context.Context) (*NetworkIdentity, error) {
	var identity NetworkIdentity
	var wg sync.WaitGroup
	var mu sync.Mutex
	var firstErr error

	// Detect network interfaces (IPs)
	wg.Add(1)
	go func() {
		defer wg.Done()
		ips, err := d.detectIPs()
		if err != nil {
			mu.Lock()
			if firstErr == nil {
				firstErr = fmt.Errorf("network: %s: %w", constants.ErrNetworkDetectIPs, err)
			}
			mu.Unlock()
			return
		}
		mu.Lock()
		identity.IPs = ips
		mu.Unlock()
	}()

	// Detect hostnames
	wg.Add(1)
	go func() {
		defer wg.Done()
		hostnames, err := d.detectHostnames()
		if err != nil {
			mu.Lock()
			if firstErr == nil {
				firstErr = fmt.Errorf("network: %s: %w", constants.ErrNetworkDetectHostnames, err)
			}
			mu.Unlock()
			return
		}
		mu.Lock()
		identity.Hostnames = hostnames
		mu.Unlock()
	}()

	// Detect hosts file aliases
	wg.Add(1)
	go func() {
		defer wg.Done()
		aliases, err := d.detectEtcHosts()
		if err != nil {
			mu.Lock()
			if firstErr == nil {
				firstErr = fmt.Errorf("network: %s: %w", constants.ErrNetworkDetectHostsAliases, err)
			}
			mu.Unlock()
			return
		}
		mu.Lock()
		identity.EtcHosts = aliases
		mu.Unlock()
	}()

	// Detect mDNS/Bonjour names
	wg.Add(1)
	go func() {
		defer wg.Done()
		mdnsNames, err := d.detectMDNS(ctx)
		if err != nil {
			mu.Lock()
			if firstErr == nil {
				firstErr = fmt.Errorf("network: %s: %w", constants.ErrNetworkDetectMDNS, err)
			}
			mu.Unlock()
			return
		}
		mu.Lock()
		identity.MDNSNames = mdnsNames
		mu.Unlock()
	}()

	// Detect DNS PTR records (after IPs are detected)
	wg.Add(1)
	go func() {
		defer wg.Done()
		mu.Lock()
		ips := identity.IPs
		mu.Unlock()
		ptrs, err := d.detectDNSPTRs(ctx, ips)
		if err != nil {
			mu.Lock()
			if firstErr == nil {
				firstErr = fmt.Errorf("network: %s: %w", constants.ErrNetworkDetectDNSPTR, err)
			}
			mu.Unlock()
			return
		}
		mu.Lock()
		identity.DNSPTRs = ptrs
		mu.Unlock()
	}()

	// Detect SSH known hosts
	wg.Add(1)
	go func() {
		defer wg.Done()
		sshHosts, err := d.detectSSHKnownHosts()
		if err != nil {
			mu.Lock()
			if firstErr == nil {
				firstErr = fmt.Errorf("network: %s: %w", constants.ErrNetworkDetectSSHKnownHosts, err)
			}
			mu.Unlock()
			return
		}
		mu.Lock()
		identity.SSHHostnames = sshHosts
		mu.Unlock()
	}()

	// Detect Windows identities
	if runtime.GOOS == "windows" {
		wg.Add(1)
		go func() {
			defer wg.Done()
			winID, err := d.detectWindowsIdentity()
			if err != nil {
				mu.Lock()
				if firstErr == nil {
					firstErr = fmt.Errorf("network: %s: %w", constants.ErrNetworkDetectWindows, err)
				}
				mu.Unlock()
				return
			}
			mu.Lock()
			identity.Windows = winID
			mu.Unlock()
		}()
	}

	wg.Wait()

	if ctx.Err() != nil {
		return nil, ctx.Err()
	}

	if firstErr != nil {
		return nil, firstErr
	}

	return &identity, nil
}

// detectIPs detects all IP addresses on all network interfaces.
func (d *Detector) detectIPs() ([]string, error) {
	ips := make([]string, 0)

	interfaces, err := net.Interfaces()
	if err != nil {
		return nil, fmt.Errorf("network: %s: %w", constants.ErrNetworkDetectInterfaces, err)
	}

	for _, iface := range interfaces {
		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}

		for _, addr := range addrs {
			var ip net.IP
			switch v := addr.(type) {
			case *net.IPNet:
				ip = v.IP
			case *net.IPAddr:
				ip = v.IP
			}

			if ip != nil {
				// Include both IPv4 and IPv6, but exclude loopback for now
				// (localhost is added separately)
				if !ip.IsLoopback() {
					ips = append(ips, ip.String())
				}
			}
		}
	}

	// Always add localhost
	ips = append(ips, "127.0.0.1", "::1")

	return ips, nil
}

// detectHostnames detects hostnames from /etc/hostname and hostname command.
func (d *Detector) detectHostnames() ([]string, error) {
	hostnames := make([]string, 0)

	// Try /etc/hostname first
	if hostname, err := os.ReadFile(constants.PathEtcHostname); err == nil {
		hn := strings.TrimSpace(string(hostname))
		if hn != "" {
			hostnames = append(hostnames, hn)
		}
	}

	// Try hostname command as fallback
	if hn, err := exec.Command("hostname").Output(); err == nil {
		hostname := strings.TrimSpace(string(hn))
		if hostname != "" && !sliceutil.Contains(hostnames, hostname) {
			hostnames = append(hostnames, hostname)
		}
	}

	// Try hostname -f for FQDN
	if fqdn, err := exec.Command("hostname", "-f").Output(); err == nil {
		fqdnStr := strings.TrimSpace(string(fqdn))
		if fqdnStr != "" && !sliceutil.Contains(hostnames, fqdnStr) {
			hostnames = append(hostnames, fqdnStr)
		}
	}

	return hostnames, nil
}

// getHostsFilePath returns the OS-specific hosts file path.
func getHostsFilePath() string {
	if runtime.GOOS == "windows" {
		return filepath.Join(os.Getenv(constants.PathWindowsSystemRoot), constants.PathWindowsHostsFile)
	}
	return constants.PathEtcHosts
}

// detectEtcHosts parses the hosts file for aliases pointing to this machine's IPs.
func (d *Detector) detectEtcHosts() ([]HostAlias, error) {
	aliases := make([]HostAlias, 0)

	// Get local IPs first
	localIPs, err := d.detectIPs()
	if err != nil {
		return nil, err
	}

	localIPSet := make(map[string]bool)
	for _, ip := range localIPs {
		localIPSet[ip] = true
	}

	// Parse hosts file (OS-specific path)
	hostsPath := getHostsFilePath()
	file, err := os.Open(hostsPath)
	if err != nil {
		return nil, fmt.Errorf("network: %s: %w", constants.ErrNetworkOpenHostsFile, err)
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		line = strings.TrimSpace(line)

		// Skip comments and empty lines
		if strings.HasPrefix(line, "#") || line == "" {
			continue
		}

		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}

		ip := fields[0]
		if localIPSet[ip] {
			aliases = append(aliases, HostAlias{
				IP:      ip,
				Aliases: fields[1:],
			})
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("network: %s: %w", constants.ErrNetworkScanHostsFile, err)
	}

	return aliases, nil
}

// detectMDNS detects mDNS/Bonjour *.local names.
func (d *Detector) detectMDNS(ctx context.Context) ([]string, error) {
	mdnsNames := make([]string, 0)

	// Get hostname and append .local
	hostnames, err := d.detectHostnames()
	if err != nil {
		return nil, fmt.Errorf("network: %s: %w", constants.ErrNetworkDetectMDNS, err)
	}

	for _, hn := range hostnames {
		// Skip if already has .local
		if !strings.HasSuffix(hn, ".local") {
			mdnsNames = append(mdnsNames, hn+".local")
		}
	}

	// Try to use avahi-browse if available (Linux)
	if _, err := exec.LookPath("avahi-browse"); err == nil {
		lookupCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
		defer cancel()

		cmd := exec.CommandContext(lookupCtx, "avahi-browse", "-ar", "-t")
		if output, err := cmd.Output(); err == nil {
			// Parse avahi-browse output for .local names
			lines := strings.Split(string(output), "\n")
			for _, line := range lines {
				if strings.Contains(line, ".local") {
					fields := strings.Fields(line)
					for _, field := range fields {
						if strings.HasSuffix(field, ".local") && !sliceutil.Contains(mdnsNames, field) {
							mdnsNames = append(mdnsNames, field)
						}
					}
				}
			}
		}
	}

	// Try dns-sd on macOS
	if runtime.GOOS == "darwin" {
		if _, err := exec.LookPath("dns-sd"); err == nil {
			lookupCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
			defer cancel()

			cmd := exec.CommandContext(lookupCtx, "dns-sd", "-B", "_services._local")
			if output, err := cmd.Output(); err == nil {
				lines := strings.Split(string(output), "\n")
				for _, line := range lines {
					if strings.Contains(line, ".local") {
						fields := strings.Fields(line)
						for _, field := range fields {
							if strings.HasSuffix(field, ".local") && !sliceutil.Contains(mdnsNames, field) {
								mdnsNames = append(mdnsNames, field)
							}
						}
					}
				}
			}
		}
	}

	return mdnsNames, nil
}

// detectDNSPTRs performs reverse DNS lookups on detected IPs.
func (d *Detector) detectDNSPTRs(ctx context.Context, ips []string) ([]DNSPTRRecord, error) {
	ptrs := make([]DNSPTRRecord, 0)

	for _, ip := range ips {
		// Skip localhost and link-local
		if strings.HasPrefix(ip, "127.") || strings.HasPrefix(ip, "::1") || strings.HasPrefix(ip, "fe80:") {
			continue
		}

		// Perform reverse lookup with timeout
		lookupCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
		names, err := net.DefaultResolver.LookupAddr(lookupCtx, ip)
		cancel()

		if err == nil && len(names) > 0 {
			for _, name := range names {
				// Remove trailing dot if present
				name = strings.TrimSuffix(name, ".")
				ptrs = append(ptrs, DNSPTRRecord{
					IP:       ip,
					Hostname: name,
				})
			}
		}
	}

	return ptrs, nil
}

// detectSSHKnownHosts checks SSH known_hosts for hostnames pointing to this machine.
func (d *Detector) detectSSHKnownHosts() ([]string, error) {
	hostnames := make([]string, 0)

	// Get local IPs
	localIPs, err := d.detectIPs()
	if err != nil {
		return nil, fmt.Errorf("network: %s: %w", constants.ErrNetworkDetectSSHKnownHosts, err)
	}

	localIPSet := make(map[string]bool)
	for _, ip := range localIPs {
		localIPSet[ip] = true
	}

	// Check common known_hosts locations
	knownHostsPaths := []string{
		os.ExpandEnv(constants.PathHomeSshKnownHosts),
		constants.PathEtcSshKnownHosts,
		constants.PathEtcSshSshKnownHosts,
	}
	if runtime.GOOS == "windows" {
		knownHostsPaths = []string{
			os.ExpandEnv(constants.PathWindowsSshKnownHosts),
			constants.PathWindowsProgramDataSsh,
		}
	}

	for _, path := range knownHostsPaths {
		// Check if file exists before attempting to open
		if _, err := os.Stat(path); os.IsNotExist(err) {
			// File doesn't exist, skip it silently (this is normal)
			continue
		}

		file, err := os.Open(path)
		if err != nil {
			// Real error (e.g., permission denied), log and continue
			d.logger.Debug("detectSSHKnownHosts: failed to open known_hosts file", "path", path, "error", err)
			continue
		}

		scanner := bufio.NewScanner(file)
		for scanner.Scan() {
			line := scanner.Text()
			line = strings.TrimSpace(line)

			// Skip comments and hashed lines
			if strings.HasPrefix(line, "#") || strings.HasPrefix(line, "|1|") {
				continue
			}

			fields := strings.Fields(line)
			if len(fields) == 0 {
				continue
			}

			// Parse host pattern (could be hostname, IP, or pattern)
			hostPattern := fields[0]
			if strings.Contains(hostPattern, ",") {
				parts := strings.Split(hostPattern, ",")
				for _, part := range parts {
					if !localIPSet[part] && !sliceutil.Contains(hostnames, part) {
						hostnames = append(hostnames, part)
					}
				}
			} else if !localIPSet[hostPattern] && !sliceutil.Contains(hostnames, hostPattern) {
				hostnames = append(hostnames, hostPattern)
			}
		}
		if err := scanner.Err(); err != nil {
			file.Close()
			d.logger.Debug("detectSSHKnownHosts: error scanning known_hosts file", "path", path, "error", err)
			continue
		}
		file.Close()
	}

	// Return empty slice if no hostnames found (not an error - files may not exist)
	return hostnames, nil
}

// detectWindowsIdentity detects Windows-specific network identities.
func (d *Detector) detectWindowsIdentity() (WindowsIdentity, error) {
	var winID WindowsIdentity

	// Try to get NetBIOS name using hostname command
	hn, err := exec.Command("hostname").Output()
	if err != nil {
		return winID, fmt.Errorf("network: %s: %w", constants.ErrNetworkGetHostname, err)
	}
	winID.NetBIOSName = strings.TrimSpace(string(hn))

	// Try to get AD FQDN using systeminfo
	info, err := exec.Command("systeminfo").Output()
	if err != nil {
		return winID, fmt.Errorf("network: %s: %w", constants.ErrNetworkGetSysteminfo, err)
	}
	lines := strings.Split(string(info), "\n")
	for _, line := range lines {
		if strings.Contains(line, "Domain:") {
			fields := strings.Fields(line)
			if len(fields) >= 2 {
				domain := fields[len(fields)-1]
				if winID.NetBIOSName != "" && domain != "WORKGROUP" {
					winID.ADFQDN = winID.NetBIOSName + "." + domain
				}
			}
		}
	}

	return winID, nil
}

// GetAllDNSNames returns all DNS names that should be included in the certificate.
func (ni *NetworkIdentity) GetAllDNSNames() []string {
	var names []string

	// Add hostnames
	names = append(names, ni.Hostnames...)

	// Add /etc/hosts aliases
	for _, alias := range ni.EtcHosts {
		names = append(names, alias.Aliases...)
	}

	// Add mDNS names
	names = append(names, ni.MDNSNames...)

	// Add DNS PTR hostnames
	for _, ptr := range ni.DNSPTRs {
		names = append(names, ptr.Hostname)
	}

	// Add SSH known hostnames
	names = append(names, ni.SSHHostnames...)

	// Add Windows identities
	if ni.Windows.NetBIOSName != "" {
		names = append(names, ni.Windows.NetBIOSName)
	}
	if ni.Windows.ADFQDN != "" {
		names = append(names, ni.Windows.ADFQDN)
	}

	// Always add localhost
	names = append(names, "localhost")

	// Deduplicate
	return sliceutil.Unique(names)
}

// GetAllIPs returns all IP addresses that should be included in the certificate.
func (ni *NetworkIdentity) GetAllIPs() []net.IP {
	var ips []net.IP

	for _, ipStr := range ni.IPs {
		ip := net.ParseIP(ipStr)
		if ip != nil {
			ips = append(ips, ip)
		}
	}

	return ips
}

// FormatForDisplay formats the network identity for user display.
func (ni *NetworkIdentity) FormatForDisplay() string {
	var lines []string

	lines = append(lines, "Detected network identity:\n")

	lines = append(lines, "  IPs          "+strings.Join(ni.IPs, ", "))

	lines = append(lines, "  Hostnames    "+strings.Join(ni.Hostnames, ", "))

	if len(ni.EtcHosts) > 0 {
		var hostEntries []string
		for _, alias := range ni.EtcHosts {
			hostEntries = append(hostEntries, fmt.Sprintf("%s → %s", alias.IP, strings.Join(alias.Aliases, ", ")))
		}
		hostsLabel := "/etc/hosts"
		if runtime.GOOS == "windows" {
			hostsLabel = "hosts"
		}
		lines = append(lines, "  "+hostsLabel+"   "+strings.Join(hostEntries, ", "))
	}

	if len(ni.MDNSNames) > 0 {
		lines = append(lines, "  mDNS         "+strings.Join(ni.MDNSNames, ", "))
	}

	if len(ni.DNSPTRs) > 0 {
		var ptrEntries []string
		for _, ptr := range ni.DNSPTRs {
			ptrEntries = append(ptrEntries, fmt.Sprintf("%s → %s", ptr.IP, ptr.Hostname))
		}
		lines = append(lines, "  DNS PTR      "+strings.Join(ptrEntries, ", "))
	}

	if len(ni.SSHHostnames) > 0 {
		lines = append(lines, "  SSH hosts    "+strings.Join(ni.SSHHostnames, ", "))
	}

	if ni.Windows.NetBIOSName != "" || ni.Windows.ADFQDN != "" {
		var winNames []string
		if ni.Windows.NetBIOSName != "" {
			winNames = append(winNames, ni.Windows.NetBIOSName)
		}
		if ni.Windows.ADFQDN != "" {
			winNames = append(winNames, ni.Windows.ADFQDN)
		}
		lines = append(lines, "  Windows      "+strings.Join(winNames, ", "))
	}

	return strings.Join(lines, "\n")
}
