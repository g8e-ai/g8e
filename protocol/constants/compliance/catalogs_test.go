// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package complianceconstants

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCatalogJSONAccessorsReturnIndependentCopies(t *testing.T) {
	tests := []struct {
		name    string
		access  func() []byte
		minimum int
	}{
		{name: "assertion catalog", access: AssertionCatalogJSON, minimum: 1},
		{name: "framework catalog", access: FrameworkCatalogJSON, minimum: 1},
		{name: "FedRAMP NIST crosswalk", access: FedRAMPAndNISTCrosswalkJSON, minimum: 1},
		{name: "demo scenario catalog", access: DemoScenarioCatalogJSON, minimum: 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			first := tt.access()
			second := tt.access()
			require.Len(t, first, len(second))
			assert.GreaterOrEqual(t, len(first), tt.minimum)
			assert.Equal(t, first, second)
			first[0] ^= 0xff
			assert.NotEqual(t, first, second, "accessor must return a defensive copy")
			assert.Equal(t, second, tt.access(), "mutating one result must not alter embedded data")
		})
	}
}
