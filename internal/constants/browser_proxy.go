// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package constants

const (
	// HeaderGatewayBrowserProxy marks requests forwarded by the Gateway browser
	// ensemble proxy. g8ee accepts stamped proxy identity only when this header
	// is present.
	HeaderGatewayBrowserProxy = "X-G8E-Gateway-Browser-Proxy"
	GatewayBrowserProxyValue  = "1"

	HeaderProxyUserID        = "X-Proxy-User-Id"
	HeaderProxyUserEmail     = "X-Proxy-User-Email"
	HeaderProxyWebSessionID  = "X-Proxy-Web-Session-Id"
	HeaderProxyOrganizationID = "X-Proxy-Organization-Id"

	// DefaultEnsembleUpstreamURL is the default g8ee HTTP surface for browser proxy.
	DefaultEnsembleUpstreamURL = "http://127.0.0.1:8000"
)
