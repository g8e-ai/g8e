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

package wizard

import (
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"strings"
)

// validatePublicBaseURL validates a public base URL.
// Must be an absolute https URL; http is allowed only for exact loopback hosts.
// Rejects user info, query, fragment, and unsupported schemes.
func validatePublicBaseURL(s string) error {
	if s == "" {
		return fmt.Errorf("public base URL is required")
	}
	u, err := url.Parse(s)
	if err != nil {
		return fmt.Errorf("wizard: validate public base URL: %w", err)
	}
	if u.Scheme != "https" && u.Scheme != "http" {
		return fmt.Errorf("URL must use http or https scheme, got %q", u.Scheme)
	}
	if u.Scheme == "http" && !isLoopbackHost(u.Hostname()) {
		return fmt.Errorf("http scheme is only allowed for loopback hosts (localhost, 127.0.0.1, ::1)")
	}
	if u.User != nil {
		return fmt.Errorf("URL must not contain user info")
	}
	if u.RawQuery != "" {
		return fmt.Errorf("URL must not contain a query string")
	}
	if u.Fragment != "" {
		return fmt.Errorf("URL must not contain a fragment")
	}
	if u.Host == "" {
		return fmt.Errorf("URL must have a host")
	}
	return nil
}

// validateTribunalURL validates an optional tribunal service URL.
// Must be an absolute https URL when supplied. Rejects user info and fragments.
func validateTribunalURL(s string) error {
	if s == "" {
		return nil
	}
	u, err := url.Parse(s)
	if err != nil {
		return fmt.Errorf("wizard: validate tribunal URL: %w", err)
	}
	if u.Scheme != "https" {
		return fmt.Errorf("tribunal URL must use https scheme, got %q", u.Scheme)
	}
	if u.User != nil {
		return fmt.Errorf("tribunal URL must not contain user info")
	}
	if u.Fragment != "" {
		return fmt.Errorf("tribunal URL must not contain a fragment")
	}
	if u.Host == "" {
		return fmt.Errorf("tribunal URL must have a host")
	}
	return nil
}

// validateTribunalID validates a tribunal policy ID.
// Required for L2 postures; letters, digits, hyphens, and underscores only.
func validateTribunalID(s string) error {
	if s == "" {
		return fmt.Errorf("tribunal ID is required for consensus and notary postures")
	}
	for _, r := range s {
		if (r < 'a' || r > 'z') && (r < 'A' || r > 'Z') &&
			(r < '0' || r > '9') && r != '-' && r != '_' {
			return fmt.Errorf("tribunal ID must contain only letters, digits, hyphens, and underscores")
		}
	}
	return nil
}

// validateTribunalBootstrap validates a tribunal bootstrap JSON file.
// Must be a readable regular JSON file with a tribunal_id field matching the given ID.
//
// This function uses os.Stat/os.ReadFile directly because the bootstrap path is
// user-supplied and may live outside the .g8e/ runtime tree. RuntimeFileService is
// scoped to .g8e/ paths and is not appropriate for arbitrary filesystem paths.
func validateTribunalBootstrap(path, tribunalID string) error {
	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("wizard: validate bootstrap: %w", err)
	}
	if info.IsDir() {
		return fmt.Errorf("wizard: validate bootstrap: path must be a file, not a directory")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("wizard: validate bootstrap: %w", err)
	}
	var doc struct {
		TribunalID string `json:"tribunal_id"`
	}
	if err := json.Unmarshal(data, &doc); err != nil {
		return fmt.Errorf("wizard: validate bootstrap: not valid JSON: %w", err)
	}
	if doc.TribunalID == "" {
		return fmt.Errorf("wizard: validate bootstrap: file must contain a tribunal_id field")
	}
	if doc.TribunalID != tribunalID {
		return fmt.Errorf("wizard: validate bootstrap: tribunal_id %q does not match configured tribunal ID %q", doc.TribunalID, tribunalID)
	}
	return nil
}

// validatePasskeyRP validates the passkey RP ID and origin together.
// The RP ID must equal or be a registrable suffix of the origin hostname.
func validatePasskeyRP(rpID, origin string) error {
	if rpID == "" {
		return fmt.Errorf("passkey RP ID is required")
	}
	u, err := url.Parse(origin)
	if err != nil {
		return fmt.Errorf("wizard: validate passkey RP: %w", err)
	}
	if u.Scheme != "https" && u.Scheme != "http" {
		return fmt.Errorf("passkey origin must use http or https scheme")
	}
	originHost := u.Hostname()
	if originHost == "" {
		return fmt.Errorf("passkey origin must have a host")
	}
	// RP ID must equal the origin host or be a registrable suffix
	if rpID == originHost {
		return nil
	}
	if strings.HasSuffix(originHost, "."+rpID) {
		return nil
	}
	return fmt.Errorf("passkey RP ID %q must match or be a registrable suffix of origin host %q", rpID, originHost)
}

// validateDownstreamURL validates an optional downstream server URL.
// Must be an absolute http/https URL with a host. Rejects credentials and fragments.
func validateDownstreamURL(s string) error {
	if s == "" {
		return fmt.Errorf("downstream URL is required when routing is enabled")
	}
	u, err := url.Parse(s)
	if err != nil {
		return fmt.Errorf("wizard: validate downstream URL: %w", err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return fmt.Errorf("downstream URL must use http or https scheme, got %q", u.Scheme)
	}
	if u.User != nil {
		return fmt.Errorf("downstream URL must not contain credentials")
	}
	if u.Fragment != "" {
		return fmt.Errorf("downstream URL must not contain a fragment")
	}
	if u.Host == "" {
		return fmt.Errorf("downstream URL must have a host")
	}
	return nil
}

// validateCORSOrigin validates a CORS origin.
// Must be an exact origin: scheme://host[:port] with no path, query, fragment, or user info.
func validateCORSOrigin(s string) error {
	if s == "" {
		return fmt.Errorf("CORS origin is required")
	}
	u, err := url.Parse(s)
	if err != nil {
		return fmt.Errorf("wizard: validate CORS origin: %w", err)
	}
	if u.Scheme != "https" && u.Scheme != "http" {
		return fmt.Errorf("CORS origin must use http or https scheme, got %q", u.Scheme)
	}
	if u.User != nil {
		return fmt.Errorf("CORS origin must not contain user info")
	}
	if u.Path != "" && u.Path != "/" {
		return fmt.Errorf("CORS origin must not contain a path")
	}
	if u.RawQuery != "" {
		return fmt.Errorf("CORS origin must not contain a query string")
	}
	if u.Fragment != "" {
		return fmt.Errorf("CORS origin must not contain a fragment")
	}
	if u.Host == "" {
		return fmt.Errorf("CORS origin must have a host")
	}
	return nil
}

// isLoopbackHost returns true for localhost, 127.0.0.1, and ::1.
func isLoopbackHost(host string) bool {
	return host == "localhost" || host == "127.0.0.1" || host == "::1"
}
