// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package constants

// Flag groups CLI flag name constants. Registered on the 'mcp stdio'
// subcommand, not as root persistent flags — see plan:
// replace-env-vars-with-global-flags.
var Flag = struct {
	ClientCert string
	ClientKey  string
	CABundle   string
	GatewayURL string
	AppCert    string
	AppKey     string
}{
	ClientCert: "client-cert",
	ClientKey:  "client-key",
	CABundle:   "ca-bundle",
	GatewayURL: "gateway-url",
	AppCert:    "app-cert",
	AppKey:     "app-key",
}
