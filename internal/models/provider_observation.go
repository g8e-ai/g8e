// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package models

import "encoding/json"

// ProviderObservationResponse bundles the canonical provider-boundary
// observation window and durable provider-attempt record for gateway-mediated
// campaign verification reads.
type ProviderObservationResponse struct {
	Window          json.RawMessage `json:"window"`
	ProviderAttempt json.RawMessage `json:"provider_attempt"`
}
