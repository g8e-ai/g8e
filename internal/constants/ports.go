// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package constants

// Ports defines canonical G8E networking ports.
// These values are generated from protocol/constants/ports.json (SSOT).
var Ports = struct {
	OperatorHttp  int `json:"OperatorHttp"`
	OperatorHttps int `json:"OperatorHttps"`
}{
	OperatorHttp:  8080,
	OperatorHttps: 8443,
}

// Deployment default ports for auxiliary services that are not part of the
// gateway protocol but are started by docker-compose.yml. These are
// Go-only constants (not mirrored in protocol/constants/ports.json) because
// they are deployment defaults, not protocol-level ports.
const (
	EnsembleDefaultPort  = 8000
	DashboardDefaultPort = 3000
)
