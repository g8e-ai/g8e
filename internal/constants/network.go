// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

// Package constants provides network-related constants.
//
// Single source of truth: protocol/constants/network.json
// This file is manually maintained to match the JSON SSOT.
package constants

// DefaultEndpoint is the default g8e Operator endpoint hostname.
// It is also the TLS ServerName used when connecting to a raw IP address,
// because the embedded CA certificate is issued to this hostname.
//
// Source: protocol/constants/network.json
const DefaultEndpoint = "localhost"

const (
	GatewayHTTPPort  = "8080"
	GatewayHTTPSPort = "8443"
	GatewayHTTPBase  = "http://" + DefaultEndpoint + ":" + GatewayHTTPPort
	GatewayHTTPSBase = "https://" + DefaultEndpoint + ":" + GatewayHTTPSPort
)

// GatewayInternalHostname is the internal hostname used for Gateway TLS connections.
// When an Operator connects to a Gateway via IP address, it uses this hostname
// for TLS ServerName verification since the Gateway's certificate is issued to this name.
const GatewayInternalHostname = "g8e.local"

// TransientNetworkErrorPatterns is a list of substring patterns that identify
// transient network errors suitable for retry logic.
var TransientNetworkErrorPatterns = []string{
	"timeout",
	"connection refused",
	"temporary failure",
	"network is unreachable",
	"no route to host",
	"i/o timeout",
	"broken pipe",
	"connection reset",
}
