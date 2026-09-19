// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package models

// EvalCampaignPublicationState tracks which public projection idempotency keys
// the gateway has already exported for one campaign run.
type EvalCampaignPublicationState struct {
	SchemaVersion         string   `json:"schema_version"`
	RunID                 string   `json:"run_id"`
	PublishedIdempotency  []string `json:"published_idempotency_keys"`
	LastPublishedSequence int64    `json:"last_published_sequence"`
}
