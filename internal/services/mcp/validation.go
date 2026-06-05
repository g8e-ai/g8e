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

package mcp

import (
	"fmt"
	"net"
	"net/url"
	"regexp"
	"strings"
)

func validateSQLQuery(query string) error {
	query = strings.TrimSpace(query)

	// Reject empty queries
	if query == "" {
		return fmt.Errorf("query cannot be empty")
	}

	// Reject queries with trailing semicolons to prevent statement chaining
	semicolonPattern := regexp.MustCompile(`;\s*$`)
	if semicolonPattern.MatchString(query) {
		return fmt.Errorf("query must not end with semicolon")
	}

	return nil
}

func validateHTTPRequestURL(rawURL string) (*url.URL, error) {
	parsedURL, err := url.Parse(rawURL)
	if err != nil {
		return nil, fmt.Errorf("invalid URL format: %w", err)
	}

	if parsedURL.Scheme != "http" && parsedURL.Scheme != "https" {
		return nil, fmt.Errorf("only http and https schemes are allowed")
	}

	if parsedURL.Host == "" {
		return nil, fmt.Errorf("URL must have a host")
	}

	host := strings.ToLower(parsedURL.Hostname())
	if strings.Contains(host, "localhost") || host == "127.0.0.1" || host == "::1" {
		return nil, fmt.Errorf("localhost and loopback addresses are not allowed")
	}

	ip := net.ParseIP(host)
	if ip != nil {
		if ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() {
			return nil, fmt.Errorf("private and loopback IP addresses are not allowed")
		}
	}

	return parsedURL, nil
}

func validateProcNetPath(protocol string) error {
	allowedProtocols := map[string]bool{
		"tcp":  true,
		"udp":  true,
		"tcp6": true,
		"udp6": true,
		"raw":  true,
	}

	if !allowedProtocols[protocol] {
		return fmt.Errorf("invalid protocol: %s", protocol)
	}

	return nil
}

func validateGitRepoPath(path string) error {
	if path == "" {
		return fmt.Errorf("repository path cannot be empty")
	}

	cleanPath := strings.TrimSpace(path)
	if cleanPath != path {
		return fmt.Errorf("repository path must not contain leading/trailing whitespace")
	}

	if strings.Contains(path, "..") {
		return fmt.Errorf("repository path must not contain parent directory references (..)")
	}

	if strings.ContainsAny(path, "\x00") {
		return fmt.Errorf("repository path must not contain null bytes")
	}

	return nil
}

func validateGitRef(ref string) error {
	if ref == "" {
		return fmt.Errorf("git reference cannot be empty")
	}

	cleanRef := strings.TrimSpace(ref)
	if cleanRef != ref {
		return fmt.Errorf("git reference must not contain leading/trailing whitespace")
	}

	if strings.ContainsAny(ref, "\x00") {
		return fmt.Errorf("git reference must not contain null bytes")
	}

	// Reject shell metacharacters and command injection patterns
	dangerousChars := []string{";", "&", "|", "$", "`", "(", ")", "<", ">", "\n", "\r"}
	for _, char := range dangerousChars {
		if strings.Contains(ref, char) {
			return fmt.Errorf("git reference contains dangerous character: %q", char)
		}
	}

	// Git references should be valid: branch names, tags, commit hashes, or special refs
	// Allow: alphanumeric, hyphens, underscores, dots, slashes, and tilde (for HEAD~n patterns)
	// Reject absolute paths and command-like patterns
	if strings.HasPrefix(ref, "/") || strings.HasPrefix(ref, "\\") {
		return fmt.Errorf("git reference must not be an absolute path")
	}

	// Validate ref format - should look like a valid git reference
	// Valid patterns: HEAD, main, feature/branch, origin/main, v1.0.0, HEAD~1, HEAD~n, abc123def
	validRefPattern := regexp.MustCompile(`^[a-zA-Z0-9_\-./~]+$`)
	if !validRefPattern.MatchString(ref) {
		return fmt.Errorf("git reference contains invalid characters")
	}

	return nil
}

func validateK8sResourceName(name string) error {
	if name == "" {
		return fmt.Errorf("resource name cannot be empty")
	}

	cleanName := strings.TrimSpace(name)
	if cleanName != name {
		return fmt.Errorf("resource name must not contain leading/trailing whitespace")
	}

	if len(name) > 253 {
		return fmt.Errorf("resource name must not exceed 253 characters")
	}

	allowedPattern := regexp.MustCompile(`^[a-z0-9]([-a-z0-9]*[a-z0-9])?$`)
	if !allowedPattern.MatchString(name) {
		return fmt.Errorf("resource name must consist of lowercase alphanumeric characters, hyphens, or dots, and must start and end with an alphanumeric character")
	}

	if strings.ContainsAny(name, "\x00") {
		return fmt.Errorf("resource name must not contain null bytes")
	}

	return nil
}

func validateK8sNamespace(namespace string) error {
	if namespace == "" {
		return fmt.Errorf("namespace cannot be empty")
	}

	cleanNamespace := strings.TrimSpace(namespace)
	if cleanNamespace != namespace {
		return fmt.Errorf("namespace must not contain leading/trailing whitespace")
	}

	if len(namespace) > 63 {
		return fmt.Errorf("namespace must not exceed 63 characters")
	}

	allowedPattern := regexp.MustCompile(`^[a-z0-9]([-a-z0-9]*[a-z0-9])?$`)
	if !allowedPattern.MatchString(namespace) {
		return fmt.Errorf("namespace must consist of lowercase alphanumeric characters or hyphens, and must start and end with an alphanumeric character")
	}

	if strings.ContainsAny(namespace, "\x00") {
		return fmt.Errorf("namespace must not contain null bytes")
	}

	return nil
}

func validateCloudMetadataOperation(operation string) error {
	allowedOperations := map[string]bool{
		"detect":            true,
		"instance":          true,
		"region":            true,
		"availability_zone": true,
		"instance_type":     true,
		"all":               true,
	}

	if !allowedOperations[operation] {
		return fmt.Errorf("invalid operation: %s", operation)
	}

	return nil
}

func validateFilePath(path string) error {
	if path == "" {
		return fmt.Errorf("file path cannot be empty")
	}

	cleanPath := strings.TrimSpace(path)
	if cleanPath != path {
		return fmt.Errorf("file path must not contain leading/trailing whitespace")
	}

	if strings.Contains(path, "..") {
		return fmt.Errorf("file path must not contain parent directory references (..)")
	}

	if strings.ContainsAny(path, "\x00") {
		return fmt.Errorf("file path must not contain null bytes")
	}

	return nil
}
