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
)

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
	errChan := make(chan error, 10)

	// Detect network interfaces (IPs)
	wg.Add(1)
	go func() {
		defer wg.Done()
		if ips, err := d.detectIPs(); err != nil {
			d.logger.Warn("Failed to detect IPs", "error", err)
		} else {
			mu.Lock()
			identity.IPs = ips
			mu.Unlock()
		}
	}()

	// Detect hostnames
	wg.Add(1)
	go func() {
		defer wg.Done()
		if hostnames, err := d.detectHostnames(); err != nil {
			d.logger.Warn("Failed to detect hostnames", "error", err)
		} else {
			mu.Lock()
			identity.Hostnames = hostnames
			mu.Unlock()
		}
	}()

	// Detect hosts file aliases
	wg.Add(1)
	go func() {
		defer wg.Done()
		if aliases, err := d.detectEtcHosts(); err != nil {
			d.logger.Warn("Failed to detect hosts file aliases", "error", err)
		} else {
			mu.Lock()
			identity.EtcHosts = aliases
			mu.Unlock()
		}
	}()

	// Detect mDNS/Bonjour names
	wg.Add(1)
	go func() {
		defer wg.Done()
		if mdnsNames, err := d.detectMDNS(); err != nil {
			d.logger.Warn("Failed to detect mDNS names", "error", err)
		} else {
			mu.Lock()
			identity.MDNSNames = mdnsNames
			mu.Unlock()
		}
	}()

	// Detect DNS PTR records (after IPs are detected)
	wg.Add(1)
	go func() {
		defer wg.Done()
		// Wait a bit for IPs to be detected
		time.Sleep(100 * time.Millisecond)
		mu.Lock()
		ips := identity.IPs
		mu.Unlock()
		if ptrs, err := d.detectDNSPTRs(ctx, ips); err != nil {
			d.logger.Warn("Failed to detect DNS PTR records", "error", err)
		} else {
			mu.Lock()
			identity.DNSPTRs = ptrs
			mu.Unlock()
		}
	}()

	// Detect SSH known hosts
	wg.Add(1)
	go func() {
		defer wg.Done()
		if sshHosts, err := d.detectSSHKnownHosts(); err != nil {
			d.logger.Warn("Failed to detect SSH known hosts", "error", err)
		} else {
			mu.Lock()
			identity.SSHHostnames = sshHosts
			mu.Unlock()
		}
	}()

	// Detect Windows identities
	if runtime.GOOS == "windows" {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if winID, err := d.detectWindowsIdentity(); err != nil {
				d.logger.Warn("Failed to detect Windows identity", "error", err)
			} else {
				mu.Lock()
				identity.Windows = winID
				mu.Unlock()
			}
		}()
	}

	wg.Wait()

	// Check for errors
	select {
	case err := <-errChan:
		return nil, err
	default:
	}

	return &identity, nil
}

// detectIPs detects all IP addresses on all network interfaces.
func (d *Detector) detectIPs() ([]string, error) {
	var ips []string

	interfaces, err := net.Interfaces()
	if err != nil {
		return nil, fmt.Errorf("failed to get network interfaces: %w", err)
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
	var hostnames []string

	// Try /etc/hostname first
	if hostname, err := os.ReadFile("/etc/hostname"); err == nil {
		hn := strings.TrimSpace(string(hostname))
		if hn != "" {
			hostnames = append(hostnames, hn)
		}
	}

	// Try hostname command as fallback
	if hn, err := exec.Command("hostname").Output(); err == nil {
		hostname := strings.TrimSpace(string(hn))
		if hostname != "" && !contains(hostnames, hostname) {
			hostnames = append(hostnames, hostname)
		}
	}

	// Try hostname -f for FQDN
	if fqdn, err := exec.Command("hostname", "-f").Output(); err == nil {
		fqdnStr := strings.TrimSpace(string(fqdn))
		if fqdnStr != "" && !contains(hostnames, fqdnStr) {
			hostnames = append(hostnames, fqdnStr)
		}
	}

	return hostnames, nil
}

// getHostsFilePath returns the OS-specific hosts file path.
func getHostsFilePath() string {
	if runtime.GOOS == "windows" {
		return filepath.Join(os.Getenv("SystemRoot"), "System32", "drivers", "etc", "hosts")
	}
	return "/etc/hosts"
}

// detectEtcHosts parses the hosts file for aliases pointing to this machine's IPs.
func (d *Detector) detectEtcHosts() ([]HostAlias, error) {
	var aliases []HostAlias

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
		return nil, fmt.Errorf("failed to open hosts file %s: %w", hostsPath, err)
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
		return nil, fmt.Errorf("error scanning /etc/hosts: %w", err)
	}

	return aliases, nil
}

// detectMDNS detects mDNS/Bonjour *.local names.
func (d *Detector) detectMDNS() ([]string, error) {
	mdnsNames := make([]string, 0)

	// Get hostname and append .local
	hostnames, err := d.detectHostnames()
	if err != nil {
		return nil, err
	}

	for _, hn := range hostnames {
		// Skip if already has .local
		if !strings.HasSuffix(hn, ".local") {
			mdnsNames = append(mdnsNames, hn+".local")
		}
	}

	// Try to use avahi-browse if available (Linux)
	if _, err := exec.LookPath("avahi-browse"); err == nil {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()

		cmd := exec.CommandContext(ctx, "avahi-browse", "-ar", "-t")
		if output, err := cmd.Output(); err == nil {
			// Parse avahi-browse output for .local names
			lines := strings.Split(string(output), "\n")
			for _, line := range lines {
				if strings.Contains(line, ".local") {
					fields := strings.Fields(line)
					for _, field := range fields {
						if strings.HasSuffix(field, ".local") && !contains(mdnsNames, field) {
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
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()

			cmd := exec.CommandContext(ctx, "dns-sd", "-B", "_services._local")
			if output, err := cmd.Output(); err == nil {
				lines := strings.Split(string(output), "\n")
				for _, line := range lines {
					if strings.Contains(line, ".local") {
						fields := strings.Fields(line)
						for _, field := range fields {
							if strings.HasSuffix(field, ".local") && !contains(mdnsNames, field) {
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
	var ptrs []DNSPTRRecord

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
		return nil, err
	}

	localIPSet := make(map[string]bool)
	for _, ip := range localIPs {
		localIPSet[ip] = true
	}

	// Check common known_hosts locations
	knownHostsPaths := []string{
		os.ExpandEnv("$HOME/.ssh/known_hosts"),
		"/etc/ssh/known_hosts",
		"/etc/ssh/ssh_known_hosts",
	}

	for _, path := range knownHostsPaths {
		file, err := os.Open(path)
		if err != nil {
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
					if !localIPSet[part] && !contains(hostnames, part) {
						hostnames = append(hostnames, part)
					}
				}
			} else if !localIPSet[hostPattern] && !contains(hostnames, hostPattern) {
				hostnames = append(hostnames, hostPattern)
			}
		}
		if err := scanner.Err(); err != nil {
			file.Close()
			continue
		}
		file.Close()
	}

	return hostnames, nil
}

// detectWindowsIdentity detects Windows-specific network identities.
func (d *Detector) detectWindowsIdentity() (WindowsIdentity, error) {
	var winID WindowsIdentity

	// Try to get NetBIOS name using hostname command
	if hn, err := exec.Command("hostname").Output(); err == nil {
		winID.NetBIOSName = strings.TrimSpace(string(hn))
	}

	// Try to get AD FQDN using systeminfo
	if info, err := exec.Command("systeminfo").Output(); err == nil {
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
	return unique(names)
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

func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}

func unique(slice []string) []string {
	seen := make(map[string]bool)
	var result []string
	for _, item := range slice {
		if !seen[item] {
			seen[item] = true
			result = append(result, item)
		}
	}
	return result
}
