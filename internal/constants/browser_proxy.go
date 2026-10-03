// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package constants

const (
	// BrowserProxyStampDomain prefixes the canonical bytes the Gateway signs for a
	// proxied browser request. It separates these signatures from every other use
	// of the Gateway's Actuator key (receipts), so a signature minted for one
	// purpose can never verify for the other.
	BrowserProxyStampDomain = "g8e.browser-proxy-stamp.v1"

	// BrowserProxyStampMaxSkewSeconds is how far a stamp's issued-at may differ
	// from the verifier's clock. The verifier also remembers nonces for this
	// window, so a captured stamp cannot be replayed.
	BrowserProxyStampMaxSkewSeconds = 30

	HeaderProxyUserID         = "X-Proxy-User-Id"
	HeaderProxyUserEmail      = "X-Proxy-User-Email"
	HeaderProxyWebSessionID   = "X-Proxy-Web-Session-Id"
	HeaderProxyOrganizationID = "X-Proxy-Organization-Id"
	HeaderProxyCLISessionID   = "X-Proxy-CLI-Session-Id"

	// DefaultEnsembleUpstreamURL is the default g8ee HTTP surface for browser proxy.
	DefaultEnsembleUpstreamURL = "http://127.0.0.1:8000"
)
