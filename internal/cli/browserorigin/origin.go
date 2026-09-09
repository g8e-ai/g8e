// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

// Package browserorigin provides the single typed browser-origin parser and
// exact-host WebAuthn RP ID derivation used by the gateway wizard, explicit
// gateway flags, and the guided `gw connect` command. No command package owns
// URL security rules.
package browserorigin

import (
	"fmt"
	"net"
	"net/url"
	"strings"

	"golang.org/x/net/idna"
	"golang.org/x/net/publicsuffix"

	"github.com/g8e-ai/g8e/v2/internal/constants"
)

// Origin is the canonical, validated browser frontend origin. It is the
// result of Parse and the input to ValidateRPID. All fields are lowercase
// canonical forms; the URL field carries no trailing slash and no path,
// query, fragment, or user information.
type Origin struct {
	// URL is the canonical origin string: scheme://host[:port]. When the
	// port is the default for the scheme it is omitted.
	URL string

	// Hostname is the host without a port. For IPv6 literals the brackets
	// are stripped.
	Hostname string

	// Port is the explicit port string, or empty when the URL used the
	// scheme default.
	Port string

	// RPID is the derived WebAuthn RP ID: the exact hostname, never the
	// host:port pair. For loopback origins this is "localhost" (IP
	// addresses are normalized to localhost because WebAuthn RP IDs must
	// be valid domains, not IP literals).
	RPID string

	// Loopback is true for localhost, 127.0.0.1, and ::1 origins. HTTP is
	// permitted only when Loopback is true.
	Loopback bool
}

// Parse validates and canonicalizes a raw frontend origin string. It enforces
// the browser-origin rules shared by the gateway wizard, explicit flags, and
// `gw connect`:
//   - Absolute http or https URL.
//   - HTTPS for non-loopback hosts; HTTP only for localhost, 127.0.0.1, ::1.
//   - No user information, path other than "/", query, or fragment.
//   - Lowercase canonical scheme and hostname.
//   - No trailing slash in canonical output.
//   - An explicit non-default port is preserved in the origin string.
//   - RP ID is derived from Hostname, never Host, so a port is never included.
//   - IP-address hosts are rejected except the loopback development cases.
//
// Parse never guesses a Lovable project name from editor URLs. Users supply
// the top-level app or preview origin they actually test.
func Parse(raw string) (Origin, error) {
	if strings.TrimSpace(raw) == "" {
		return Origin{}, fmt.Errorf("%w: frontend origin is required", constants.ErrValidationFailed)
	}

	u, err := url.Parse(raw)
	if err != nil {
		return Origin{}, fmt.Errorf("%w: invalid frontend origin URL: %w", constants.ErrValidationFailed, err)
	}

	if u.Scheme != "http" && u.Scheme != "https" {
		return Origin{}, fmt.Errorf("%w: frontend origin must use http or https scheme, got %q", constants.ErrValidationFailed, u.Scheme)
	}

	if u.Host == "" {
		return Origin{}, fmt.Errorf("%w: frontend origin must have a host", constants.ErrValidationFailed)
	}

	if u.User != nil {
		return Origin{}, fmt.Errorf("%w: frontend origin must not contain user information", constants.ErrValidationFailed)
	}

	if u.Path != "" && u.Path != "/" {
		return Origin{}, fmt.Errorf("%w: frontend origin must not contain a path, got %q", constants.ErrValidationFailed, u.Path)
	}

	if u.RawQuery != "" {
		return Origin{}, fmt.Errorf("%w: frontend origin must not contain a query string", constants.ErrValidationFailed)
	}

	if u.Fragment != "" {
		return Origin{}, fmt.Errorf("%w: frontend origin must not contain a fragment", constants.ErrValidationFailed)
	}

	hostname := u.Hostname()
	if hostname == "" {
		return Origin{}, fmt.Errorf("%w: frontend origin must have a host", constants.ErrValidationFailed)
	}

	// Canonicalize scheme and hostname to lowercase.
	scheme := strings.ToLower(u.Scheme)
	hostname = strings.ToLower(hostname)
	if net.ParseIP(hostname) == nil {
		hostname, err = idna.Lookup.ToASCII(hostname)
		if err != nil {
			return Origin{}, fmt.Errorf("%w: frontend origin hostname is invalid: %w", constants.ErrValidationFailed, err)
		}
	}

	loopback := isLoopback(hostname)

	// HTTPS is required for non-loopback hosts.
	if scheme == "http" && !loopback {
		return Origin{}, fmt.Errorf("%w: http scheme is only allowed for loopback hosts (localhost, 127.0.0.1, ::1), got %q", constants.ErrValidationFailed, hostname)
	}

	// Reject non-loopback IP-address hosts. WebAuthn RP IDs must be valid
	// domains; an IP literal is not a registrable domain. Loopback IPs are
	// normalized to "localhost" for the RP ID only — the canonical URL
	// preserves the original host so the browser reaches the right address.
	rpID := hostname
	if ip := net.ParseIP(hostname); ip != nil {
		if !loopback {
			return Origin{}, fmt.Errorf("%w: IP-address frontend origins are only supported for loopback development, got %q", constants.ErrValidationFailed, hostname)
		}
		rpID = "localhost"
	}

	port := u.Port()
	if isDefaultPort(scheme, port) {
		port = ""
	}

	// Build the canonical origin string. Omit the port when it is the
	// scheme default so the canonical form matches the browser's origin
	// serialization.
	host := hostname
	if port != "" {
		if strings.Contains(hostname, ":") {
			// IPv6 literal: re-bracket for the host component.
			host = "[" + hostname + "]:" + port
		} else {
			host = hostname + ":" + port
		}
	} else if strings.Contains(hostname, ":") {
		// IPv6 literal without an explicit port: bracket the host.
		host = "[" + hostname + "]"
	}

	canonicalURL := scheme + "://" + host

	return Origin{
		URL:      canonicalURL,
		Hostname: hostname,
		Port:     port,
		RPID:     rpID,
		Loopback: loopback,
	}, nil
}

// ValidateRPID validates an advanced RP ID override against the parsed origin.
// The override is accepted when it equals the origin hostname (exact-host
// default) or is a registrable parent suffix of the origin hostname. Public
// suffixes and unrelated domains are rejected.
//
// A parent suffix is valid only when the origin hostname is a subdomain of it
// AND it is not itself a public suffix. For example, for origin
// https://your-app.lovable.app, the override "lovable.app" is a valid parent
// suffix only if "lovable.app" is not a public suffix. The override
// "your-app.lovable.app" (exact match) is always valid.
func ValidateRPID(origin Origin, rpID string) (string, error) {
	if rpID == "" {
		return "", fmt.Errorf("%w: passkey RP ID is required", constants.ErrValidationFailed)
	}

	rpID, err := idna.Lookup.ToASCII(strings.ToLower(strings.TrimSpace(rpID)))
	if err != nil {
		return "", fmt.Errorf("%w: passkey RP ID is invalid: %w", constants.ErrValidationFailed, err)
	}

	if rpID == origin.Hostname {
		return rpID, nil
	}

	// The override must be a parent suffix: origin must end with ".<rpID>".
	if !strings.HasSuffix(origin.Hostname, "."+rpID) {
		return "", fmt.Errorf("%w: passkey RP ID %q must match or be a registrable suffix of origin host %q", constants.ErrValidationFailed, rpID, origin.Hostname)
	}

	// Reject public suffixes. A public suffix (e.g., "lovable.app" if it
	// is in the Public Suffix List, or "com", "co.uk") is not a registrable
	// domain and must not be used as an RP ID because it would scope
	// credentials across unrelated tenants.
	suffix, icann := publicsuffix.PublicSuffix(rpID)
	if icann && rpID == suffix {
		return "", fmt.Errorf("%w: passkey RP ID %q is a public suffix and cannot scope credentials across unrelated tenants", constants.ErrValidationFailed, rpID)
	}
	// Also reject known multi-part public suffixes from private registries
	// (e.g., "lovable.app" if listed as a private suffix). publicsuffix
	// returns icann=false for private entries, so check the non-ICANN case
	// too.
	if !icann && rpID == suffix {
		return "", fmt.Errorf("%w: passkey RP ID %q is a private public suffix and cannot scope credentials across unrelated tenants", constants.ErrValidationFailed, rpID)
	}

	return rpID, nil
}

// isLoopback reports whether host is a loopback development host.
func isLoopback(host string) bool {
	return host == "localhost" || host == "127.0.0.1" || host == "::1"
}

// isDefaultPort reports whether port is the default port for the scheme.
func isDefaultPort(scheme, port string) bool {
	switch scheme {
	case "https":
		return port == "443"
	case "http":
		return port == "80"
	default:
		return false
	}
}
