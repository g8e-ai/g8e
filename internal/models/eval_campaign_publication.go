// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package models

// EvalCampaignPublishedProofArtifacts stores content-addressed proof hashes for
// one assignment so force restore can skip rebuilding unchanged audit exports.
type EvalCampaignPublishedProofArtifacts struct {
	DatabaseSHA256 string `json:"database_sha256"`
	VaultKeySHA256 string `json:"vault_key_sha256"`
}

// EvalCampaignPublicationState tracks which public projection idempotency keys
// the gateway has already exported for one campaign run.
type EvalCampaignPublicationState struct {
	SchemaVersion           string                                         `json:"schema_version"`
	RunID                   string                                         `json:"run_id"`
	PublishedIdempotency    []string                                       `json:"published_idempotency_keys"`
	PublishedProofArtifacts map[string]EvalCampaignPublishedProofArtifacts `json:"published_proof_artifacts"`
	LastPublishedSequence   int64                                          `json:"last_published_sequence"`
}
