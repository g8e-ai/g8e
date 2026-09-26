// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package models

import "encoding/json"

// ModelProvenanceResponse bundles the canonical model provenance attestation
// window for gateway-mediated campaign verification reads.
type ModelProvenanceResponse struct {
	Window json.RawMessage `json:"window"`
}

// ModelProvenanceAttestResponse bundles one synchronous storage attestation
// preflight result for campaign execute and formation smoke paths.
type ModelProvenanceAttestResponse struct {
	Status              string          `json:"status"`
	ServedModelTag      string          `json:"served_model_tag"`
	ExpectedModelDigest string          `json:"expected_model_digest"`
	Window              json.RawMessage `json:"window"`
}
