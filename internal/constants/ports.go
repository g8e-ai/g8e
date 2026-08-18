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
