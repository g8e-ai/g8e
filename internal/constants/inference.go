// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package constants

import "time"

// EvaluationAppName is the platform-enrolled application identity the eval
// runner presents for governed inference dispatch. It enrolls once through
// `g8e auth enroll app` and never borrows another component's credentials.
const EvaluationAppName = "g8e-eval"

// InferenceDefaultKeepAlive is the Ollama keep-alive `g8e operator start` uses
// when --inference-keep-alive is not given. The Inference Operator configures
// no models: each governed request carries the model the user chose.
const InferenceDefaultKeepAlive = "-1"

// ModelProvenanceAttestationPreflightTimeout bounds the synchronous storage
// attestation probe: the Gateway waits this long for the Provenance Operator
// to hash the model's blobs, and the CLI client allows this plus
// ModelProvenanceAttestationPreflightClientMargin. Hashing a multi-gigabyte
// model over a mounted filesystem takes well over 30 seconds.
const (
	ModelProvenanceAttestationPreflightTimeout      = 5 * time.Minute
	ModelProvenanceAttestationPreflightClientMargin = 15 * time.Second
)
