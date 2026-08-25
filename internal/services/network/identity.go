// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

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

	"github.com/g8e-ai/g8e/v2/internal/constants"
)

// netInterfacesFunc is a function type for getting network interfaces.
// This allows dependency injection for testing.
type netInterfacesFunc func() ([]net.Interface, error)

// interfaceAddrsFunc is a function type for getting addresses of a network interface.
// This allows dependency injection for testing.
type interfaceAddrsFunc func(iface net.Interface) ([]net.Addr, error)

// defaultNetInterfaces is the default implementation using net.Interfaces.
func defaultNetInterfaces() ([]net.Interface, error) {
	return net.Interfaces()
}

// defaultInterfaceAddrs is the default implementation using net.Interface.Addrs.
func defaultInterfaceAddrs(iface net.Interface) ([]net.Addr, error) {
	return iface.Addrs()
}

// GetExternalInterfaceIP returns the first non-loopback IPv4 address found on the host.
// This is used for the Operator Bootstrap endpoint which remote operators rely on.
func GetExternalInterfaceIP() string {
	return getExternalInterfaceIPWithFunc(defaultNetInterfaces, defaultInterfaceAddrs)
}

// getExternalInterfaceIPWithFunc is the testable implementation that accepts injectable functions.
func getExternalInterfaceIPWithFunc(getInterfaces netInterfacesFunc, getAddrs interfaceAddrsFunc) string {
	ifaces, err := getInterfaces()
	if err != nil {
		return "localhost"
	}

	for _, iface := range ifaces {
		addrs, err := getAddrs(iface)
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
				firstErr = fmt.Errorf("network: detect IPs: %w", err)
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
				firstErr = fmt.Errorf("network: detect hostnames: %w", err)
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
				firstErr = fmt.Errorf("network: detect hosts file aliases: %w", err)
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
				firstErr = fmt.Errorf("network: detect mDNS names: %w", err)
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
				firstErr = fmt.Errorf("network: detect DNS PTR records: %w", err)
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
				firstErr = fmt.Errorf("network: detect SSH known hosts: %w", err)
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
			winID, err := d.detectWindowsIdentity(ctx)
			if err != nil {
				mu.Lock()
				if firstErr == nil {
					firstErr = fmt.Errorf("network: detect Windows identity: %w", err)
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
		return nil, fmt.Errorf("network: detect interfaces: %w", err)
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
				// IPv4 only — IPv6 is not supported
				if ip.To4() != nil && !ip.IsLoopback() {
					ips = append(ips, ip.String())
				}
			}
		}
	}

	// Always add localhost (IPv4 only)
	ips = append(ips, "127.0.0.1")

	return ips, nil
}

// detectHostnames detects hostnames from /etc/hostname and hostname command.
func (d *Detector) detectHostnames() ([]string, error) {
	hostnameSet := make(map[string]bool)

	// Try /etc/hostname first
	if hostname, err := os.ReadFile(constants.PathEtcHostname); err == nil {
		hn := strings.TrimSpace(string(hostname))
		if hn != "" {
			hostnameSet[hn] = true
		}
	}

	// Try hostname command as fallback
	if hn, err := exec.Command("hostname").Output(); err == nil {
		hostname := strings.TrimSpace(string(hn))
		if hostname != "" {
			hostnameSet[hostname] = true
		}
	}

	// Try hostname -f for FQDN
	if fqdn, err := exec.Command("hostname", "-f").Output(); err == nil {
		fqdnStr := strings.TrimSpace(string(fqdn))
		if fqdnStr != "" {
			hostnameSet[fqdnStr] = true
		}
	}

	// Convert set to slice
	hostnames := make([]string, 0, len(hostnameSet))
	for hn := range hostnameSet {
		hostnames = append(hostnames, hn)
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
		return nil, fmt.Errorf("network: open hosts file: %w", err)
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
		return nil, fmt.Errorf("network: scan hosts file: %w", err)
	}

	return aliases, nil
}

// detectMDNS detects mDNS/Bonjour *.local names.
func (d *Detector) detectMDNS(ctx context.Context) ([]string, error) {
	mdnsSet := make(map[string]bool)

	// Get hostname and append .local
	hostnames, err := d.detectHostnames()
	if err != nil {
		return nil, fmt.Errorf("network: detect mDNS names: %w", err)
	}

	for _, hn := range hostnames {
		// Skip if already has .local
		if !strings.HasSuffix(hn, ".local") {
			mdnsSet[hn+".local"] = true
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
						if strings.HasSuffix(field, ".local") {
							mdnsSet[field] = true
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
							if strings.HasSuffix(field, ".local") {
								mdnsSet[field] = true
							}
						}
					}
				}
			}
		}
	}

	// Convert set to slice
	mdnsNames := make([]string, 0, len(mdnsSet))
	for name := range mdnsSet {
		mdnsNames = append(mdnsNames, name)
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
	hostnameSet := make(map[string]bool)

	// Get local IPs
	localIPs, err := d.detectIPs()
	if err != nil {
		return nil, fmt.Errorf("network: detect IPs: %w", err)
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
					if !localIPSet[part] {
						hostnameSet[part] = true
					}
				}
			} else if !localIPSet[hostPattern] {
				hostnameSet[hostPattern] = true
			}
		}
		if err := scanner.Err(); err != nil {
			file.Close()
			d.logger.Debug("detectSSHKnownHosts: error scanning known_hosts file", "path", path, "error", err)
			continue
		}
		file.Close()
	}

	// Convert set to slice
	hostnames := make([]string, 0, len(hostnameSet))
	for hn := range hostnameSet {
		hostnames = append(hostnames, hn)
	}

	// Return empty slice if no hostnames found (not an error - files may not exist)
	return hostnames, nil
}

// commandExecutor is a function type for executing commands.
// This allows dependency injection for testing.
type commandExecutor func(ctx context.Context, name string, args ...string) ([]byte, error)

// defaultCommandExecutor is the default implementation using exec.CommandContext.
// The context allows cancellation of long-running commands (e.g. systeminfo on Windows).
func defaultCommandExecutor(ctx context.Context, name string, args ...string) ([]byte, error) {
	return exec.CommandContext(ctx, name, args...).Output()
}

// detectWindowsIdentity detects Windows-specific network identities.
func (d *Detector) detectWindowsIdentity(ctx context.Context) (WindowsIdentity, error) {
	return d.detectWindowsIdentityWithExecutor(ctx, defaultCommandExecutor)
}

// detectWindowsIdentityWithExecutor is the testable implementation that accepts a command executor.
func (d *Detector) detectWindowsIdentityWithExecutor(ctx context.Context, executor commandExecutor) (WindowsIdentity, error) {
	var winID WindowsIdentity

	// Try to get NetBIOS name using hostname command
	hn, err := executor(ctx, "hostname")
	if err != nil {
		return winID, fmt.Errorf("network: get hostname: %w", err)
	}
	winID.NetBIOSName = strings.TrimSpace(string(hn))

	// Try to get AD FQDN using systeminfo
	info, err := executor(ctx, "systeminfo")
	if err != nil {
		return winID, fmt.Errorf("network: get systeminfo: %w", err)
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
	seen := make(map[string]bool)
	var unique []string
	for _, name := range names {
		if !seen[name] {
			seen[name] = true
			unique = append(unique, name)
		}
	}
	return unique
}

// GetAllIPs returns all IP addresses that should be included in the certificate.
func (ni *NetworkIdentity) GetAllIPs() []net.IP {
	var ips []net.IP

	for _, ipStr := range ni.IPs {
		ip := net.ParseIP(ipStr)
		if ip != nil && ip.To4() != nil {
			ips = append(ips, ip)
		}
	}

	return ips
}

// FormatForDisplay formats the network identity for user display.
func (ni *NetworkIdentity) FormatForDisplay() string {
	var lines []string

	lines = append(lines, "Detected network identity:\n")

	// Show primary external IP first, then remaining IPs collapsed
	primaryIP := getExternalIP(ni.IPs)
	if primaryIP != "" {
		lines = append(lines, "  IP           "+primaryIP)
		rest := filterOutIP(ni.IPs, primaryIP)
		if len(rest) > 0 {
			lines = append(lines, "  Other IPs    "+strings.Join(rest, ", "))
		}
	} else if len(ni.IPs) > 0 {
		lines = append(lines, "  IPs          "+strings.Join(ni.IPs, ", "))
	}

	lines = append(lines, "  Hostnames    "+strings.Join(ni.Hostnames, ", "))

	if len(ni.EtcHosts) > 0 {
		var hostEntries []string
		for _, alias := range ni.EtcHosts {
			hostEntries = append(hostEntries, fmt.Sprintf("%s → %s", alias.IP, strings.Join(alias.Aliases, ", ")))
		}
		hostsLabel := constants.PathEtcHosts
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

// getExternalIP returns the first non-loopback, non-link-local IPv4 from the list.
func getExternalIP(ips []string) string {
	for _, ip := range ips {
		if ip == "127.0.0.1" || strings.HasPrefix(ip, "169.254.") {
			continue
		}
		parsed := net.ParseIP(ip)
		if parsed == nil || parsed.IsLoopback() || parsed.IsLinkLocalUnicast() {
			continue
		}
		return ip
	}
	return ""
}

// filterOutIP returns ips with the given IP removed.
func filterOutIP(ips []string, exclude string) []string {
	var result []string
	for _, ip := range ips {
		if ip != exclude {
			result = append(result, ip)
		}
	}
	return result
}
