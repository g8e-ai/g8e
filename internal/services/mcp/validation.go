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
	queryUpper := strings.ToUpper(query)

	dangerousPatterns := []string{
		";DROP", ";DELETE", ";INSERT", ";UPDATE", ";ALTER", ";CREATE",
		";TRUNCATE", ";EXEC", ";EXECUTE", "--", "/*", "*/",
	}

	for _, pattern := range dangerousPatterns {
		if strings.Contains(queryUpper, pattern) {
			return fmt.Errorf("query contains forbidden pattern: %s", pattern)
		}
	}

	commentPattern := regexp.MustCompile(`--|/\*|\*/`)
	if commentPattern.MatchString(query) {
		return fmt.Errorf("query contains SQL comments which are not allowed")
	}

	semicolonPattern := regexp.MustCompile(`;\s*$`)
	if semicolonPattern.MatchString(query) {
		return fmt.Errorf("query must not end with semicolon")
	}

	return nil
}

func validateHTTPRequestURL(rawURL string) error {
	parsedURL, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("invalid URL format: %w", err)
	}

	if parsedURL.Scheme != "http" && parsedURL.Scheme != "https" {
		return fmt.Errorf("only http and https schemes are allowed")
	}

	if parsedURL.Host == "" {
		return fmt.Errorf("URL must have a host")
	}

	host := strings.ToLower(parsedURL.Hostname())
	if strings.Contains(host, "localhost") || host == "127.0.0.1" || host == "::1" {
		return fmt.Errorf("localhost and loopback addresses are not allowed")
	}

	ip := net.ParseIP(host)
	if ip != nil {
		if ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() {
			return fmt.Errorf("private and loopback IP addresses are not allowed")
		}
	}

	return nil
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
