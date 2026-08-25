// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package ssh

import (
	"bufio"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/agent"
	"golang.org/x/crypto/ssh/knownhosts"

	"github.com/g8e-ai/g8e/v2/internal/paths"
)

// ConfigBlock holds parsed values for a single Host block from SSH config.
type ConfigBlock struct {
	Hostname      string
	User          string
	Port          string
	IdentityFiles []string
	ProxyCommand  string
}

// HostConfig holds resolved SSH connection parameters for a target.
type HostConfig struct {
	Original     string
	Hostname     string
	User         string
	Port         string
	KeyFiles     []string
	ProxyCommand string
}

// ParseConfig reads an OpenSSH-format config file and returns a map of
// pattern → block for the fields relevant to SSH connections:
// HostName, User, Port, IdentityFile, ProxyCommand.
//
// This is a minimal parser that handles the subset of directives we need.
// It does not handle Match blocks, Include, or multi-value canonicalisation.
func ParseConfig(path string) (map[string]*ConfigBlock, error) {
	blocks := make(map[string]*ConfigBlock)

	// path is validated by caller (validateSSHConfigPath) to satisfy CodeQL uncontrolled-data-in-path-expression rule.
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return blocks, nil
		}
		return blocks, fmt.Errorf("ssh: open config file %s: %w", path, err)
	}
	defer f.Close()

	var current *ConfigBlock
	var currentPattern string

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		// Split key and value (supports both "Key Value" and "Key=Value")
		line = strings.ReplaceAll(line, "=", " ")
		parts := strings.Fields(line)
		if len(parts) < 2 {
			continue
		}
		key := strings.ToLower(parts[0])
		val := strings.Join(parts[1:], " ")

		switch key {
		case "host":
			// New Host block - save previous, start fresh
			currentPattern = val
			b := &ConfigBlock{}
			blocks[currentPattern] = b
			current = b
		case "hostname":
			if current != nil {
				current.Hostname = val
			}
		case "user":
			if current != nil {
				current.User = val
			}
		case "port":
			if current != nil {
				current.Port = val
			}
		case "identityfile":
			if current != nil {
				expanded, err := ExpandTilde(val)
				if err != nil {
					return blocks, fmt.Errorf("ssh: expand tilde for %s: %w", val, err)
				}
				current.IdentityFiles = append(current.IdentityFiles, expanded)
			}
		case "proxycommand":
			if current != nil {
				current.ProxyCommand = val
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return blocks, fmt.Errorf("ssh: scan config file %s: %w", path, err)
	}
	return blocks, nil
}

// MatchBlock finds the first Host block in blocks whose pattern matches
// the given alias. Supports exact match and simple wildcard (*/?).
func MatchBlock(blocks map[string]*ConfigBlock, alias string) *ConfigBlock {
	// Exact match first
	if b, ok := blocks[alias]; ok {
		return b
	}
	// Wildcard patterns
	for pattern, b := range blocks {
		if PatternMatch(pattern, alias) {
			return b
		}
	}
	return nil
}

// PatternMatch implements the OpenSSH Host pattern matching rules:
// '*' matches any sequence of characters, '?' matches a single character.
// Multiple patterns in one Host line are space-separated.
func PatternMatch(patterns, alias string) bool {
	for _, p := range strings.Fields(patterns) {
		if p == "*" {
			return true
		}
		if MatchGlob(p, alias) {
			return true
		}
	}
	return false
}

// MatchGlob matches s against a simple glob (*, ?) pattern.
func MatchGlob(pattern, s string) bool {
	for len(pattern) > 0 {
		switch pattern[0] {
		case '*':
			pattern = pattern[1:]
			if len(pattern) == 0 {
				return true
			}
			for i := 0; i <= len(s); i++ {
				if MatchGlob(pattern, s[i:]) {
					return true
				}
			}
			return false
		case '?':
			if len(s) == 0 {
				return false
			}
			pattern = pattern[1:]
			s = s[1:]
		default:
			if len(s) == 0 || pattern[0] != s[0] {
				return false
			}
			pattern = pattern[1:]
			s = s[1:]
		}
	}
	return len(s) == 0
}

// ResolveHost reads ~/.ssh/config (or the provided path) and resolves SSH
// connection parameters for the given alias or user@host[:port] string.
func ResolveHost(target, sshConfigPath, username, sshIdentityFile, sshUser string) (HostConfig, error) {
	r := HostConfig{Original: target}

	// Parse user@host:port if present
	hostPart := target
	if idx := strings.LastIndex(target, "@"); idx >= 0 {
		r.User = target[:idx]
		hostPart = target[idx+1:]
	}
	if host, port, err := net.SplitHostPort(hostPart); err == nil {
		r.Hostname = host
		r.Port = port
	} else {
		r.Hostname = hostPart
	}

	// Locate SSH config file
	configPath := sshConfigPath
	if configPath == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return r, fmt.Errorf("ssh: resolve home dir: %w", err)
		}
		sshPaths := paths.GetSSHConfigPaths(home)
		configPath = sshPaths.ConfigPath
	}

	blocks, err := ParseConfig(configPath)
	if err != nil {
		return r, fmt.Errorf("ssh: parse config: %w", err)
	}
	if block := MatchBlock(blocks, r.Hostname); block != nil {
		if r.User == "" && block.User != "" {
			r.User = block.User
		}
		if r.Port == "" && block.Port != "" && block.Port != "22" {
			r.Port = block.Port
		}
		if block.Hostname != "" {
			r.Hostname = block.Hostname
		}
		r.KeyFiles = append(r.KeyFiles, block.IdentityFiles...)
		if block.ProxyCommand != "" {
			r.ProxyCommand = block.ProxyCommand
		}
	}

	// Explicit SSH user flag overrides config and parsed user@host
	if sshUser != "" {
		r.User = sshUser
	}

	// Explicit SSH identity file flag overrides config
	if sshIdentityFile != "" {
		r.KeyFiles = []string{sshIdentityFile}
	}

	// Defaults
	if r.User == "" {
		r.User = username
	}
	if r.Port == "" {
		r.Port = "22"
	}

	// Fall back to standard key paths if none found in config
	if len(r.KeyFiles) == 0 {
		home, err := os.UserHomeDir()
		if err != nil {
			return r, fmt.Errorf("ssh: resolve home dir: %w", err)
		}
		sshPaths := paths.GetSSHConfigPaths(home)
		candidates := []string{
			sshPaths.IDE25519KeyPath,
			sshPaths.IDECDSAKeyPath,
			sshPaths.IDRSAKeyPath,
		}
		for _, p := range candidates {
			if _, err := os.Stat(p); err == nil {
				r.KeyFiles = append(r.KeyFiles, p)
			}
		}
	}

	return r, nil
}

// BuildAuthMethods returns the SSH auth methods for a resolved host.
// Priority: explicit identity files → SSH agent → default key paths.
// If passphrase is provided, it will be used to decrypt encrypted keys.
func BuildAuthMethods(r HostConfig, sshAuthSock, passphrase string) ([]ssh.AuthMethod, error) {
	var methods []ssh.AuthMethod

	// SSH agent
	if sshAuthSock != "" {
		conn, err := net.Dial("unix", sshAuthSock)
		if err != nil {
			return nil, fmt.Errorf("ssh: dial agent socket %s: %w", sshAuthSock, err)
		}
		defer conn.Close()
		agentClient := agent.NewClient(conn)
		_, err = agentClient.Signers()
		if err != nil {
			return nil, fmt.Errorf("ssh: get agent signers: %w", err)
		}
		methods = append(methods, ssh.PublicKeysCallback(agentClient.Signers))
	}

	// Identity files
	for _, keyPath := range r.KeyFiles {
		data, err := os.ReadFile(keyPath)
		if err != nil {
			return nil, fmt.Errorf("ssh: read key file %s: %w", keyPath, err)
		}
		var signer ssh.Signer
		if passphrase != "" {
			// Try with passphrase first
			signer, err = ssh.ParsePrivateKeyWithPassphrase(data, []byte(passphrase))
			if err != nil {
				// Fall back to no passphrase if passphrase provided but wrong
				signer, err = ssh.ParsePrivateKey(data)
				if err != nil {
					return nil, fmt.Errorf("ssh: parse private key %s: %w", keyPath, err)
				}
			}
		} else {
			// No passphrase provided, try without
			signer, err = ssh.ParsePrivateKey(data)
			if err != nil {
				return nil, fmt.Errorf("ssh: parse private key %s: %w", keyPath, err)
			}
		}
		methods = append(methods, ssh.PublicKeys(signer))
	}

	return methods, nil
}

// BuildHostKeyCallback returns a strict known_hosts-backed host-key callback.
//
// If khPath is empty, it defaults to ~/.ssh/known_hosts.
//
// Strict-only by design: there is no accept-new fallback. The caller MUST have
// pre-populated ~/.ssh/known_hosts with every target host's key. Any unknown host
// fails the SSH handshake immediately. Any I/O error reading the file is returned
// to the caller, which surfaces it as a per-host failure rather than
// silently degrading security.
func BuildHostKeyCallback(khPath string) (ssh.HostKeyCallback, error) {
	if khPath == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, fmt.Errorf("ssh: resolve home dir: %w", err)
		}
		sshPaths := paths.GetSSHConfigPaths(home)
		khPath = sshPaths.KnownHostsPath
	}
	if _, err := os.Stat(khPath); err != nil {
		return nil, fmt.Errorf("ssh: known hosts not found %s: %w", khPath, err)
	}
	cb, err := knownhosts.New(khPath)
	if err != nil {
		return nil, fmt.Errorf("ssh: parse known hosts %s: %w", khPath, err)
	}
	return cb, nil
}

// ExpandTilde replaces a leading ~ with the user's home directory.
func ExpandTilde(path string) (string, error) {
	if !strings.HasPrefix(path, "~") {
		return path, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("ssh: resolve home dir: %w", err)
	}
	return filepath.Join(home, path[1:]), nil
}
