// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.

package evaluation

import "github.com/g8e-ai/g8e/v2/internal/services/publicdisclosure"

const (
	campaignProjectionEnvelopeHistoricalVersion = "1.0.0"
	campaignProjectionEnvelopeEnrichedVersion   = "1.1.0"
)

// ValidatePublicAssignmentRecord exposes the shared disclosure validator to
// campaign composition and export without coupling the Gateway package to the
// evaluation service package.
func ValidatePublicAssignmentRecord(envelopeVersion string, recordBytes []byte) error {
	return publicdisclosure.ValidateAssignmentRecord(envelopeVersion, recordBytes)
}
