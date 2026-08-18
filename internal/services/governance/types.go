// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package governance

import (
	"context"

	operatorv1 "github.com/g8e-ai/g8e/protocol/proto/g8e/operator/v1"
)

// EnvelopeProcessor is the minimal interface for envelope processing.
// This allows test doubles in tests while using the concrete OperatorPubSubService in production.
type EnvelopeProcessor interface {
	ProcessEnvelope(ctx context.Context, payload []byte) (*operatorv1.ActionReceipt, error)
}
