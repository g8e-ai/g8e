// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 0.

//go:build integration

package gateway

import (
	"net/http/httptest"

	"github.com/g8e-ai/g8e/v2/internal/models"
)

// PublicFeedRecordTypeInvalid is a sentinel invalid record type used by tests
// to verify that the publisher rejects unknown record types.
const PublicFeedRecordTypeInvalid models.PublicFeedRecordType = "invalid_type"

// httptestResponseWriter returns a fresh httptest.ResponseRecorder for
// StreamProof tests that need to inspect response headers and body.
func httptestResponseWriter() *httptest.ResponseRecorder {
	return httptest.NewRecorder()
}
