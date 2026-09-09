// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package constants

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestEventDashboardClassificationJSONValidity asserts the classification
// inventory file is valid JSON and contains the expected top-level keys.
func TestEventDashboardClassificationJSONValidity(t *testing.T) {
	data, err := os.ReadFile("../../protocol/constants/event_dashboard_classification.json")
	require.NoError(t, err)
	assert.True(t, json.Valid(data), "event_dashboard_classification.json is valid JSON")

	var raw struct {
		Families             map[string]struct{} `json:"families"`
		ClassificationLegend map[string]string   `json:"_classification_legend"`
	}
	require.NoError(t, json.Unmarshal(data, &raw))
	assert.NotEmpty(t, raw.ClassificationLegend, "classification legend must be present")
	assert.NotEmpty(t, raw.Families, "families must be present")
}

// TestEventDashboardClassificationCoversAllRegisteredFamilies asserts that
// every event family present in events.json has a corresponding entry in the
// dashboard classification inventory. This prevents silent drift when new
// event families are registered.
func TestEventDashboardClassificationCoversAllRegisteredFamilies(t *testing.T) {
	eventsData, err := os.ReadFile("../../protocol/constants/events.json")
	require.NoError(t, err)
	var events struct {
		Events map[string]struct {
			Value string `json:"value"`
		} `json:"events"`
	}
	require.NoError(t, json.Unmarshal(eventsData, &events.Events))

	classData, err := os.ReadFile("../../protocol/constants/event_dashboard_classification.json")
	require.NoError(t, err)
	var classRaw struct {
		Families map[string]struct {
			Classification string `json:"classification"`
		} `json:"families"`
	}
	require.NoError(t, json.Unmarshal(classData, &classRaw))

	// Extract the set of family prefixes from registered events.
	registeredFamilies := make(map[string]bool)
	for _, meta := range events.Events {
		parts := strings.Split(meta.Value, ".")
		if len(parts) >= 4 {
			family := strings.Join(parts[:4], ".")
			registeredFamilies[family] = true
		}
	}

	missing := []string{}
	for fam := range registeredFamilies {
		if _, ok := classRaw.Families[fam]; !ok {
			missing = append(missing, fam)
		}
	}
	assert.Empty(t, missing,
		"event families registered in events.json but missing from dashboard classification: %v", missing)
}

// TestEventDashboardClassificationValues asserts that every classification
// value in the inventory is one of the four documented categories.
func TestEventDashboardClassificationValues(t *testing.T) {
	data, err := os.ReadFile("../../protocol/constants/event_dashboard_classification.json")
	require.NoError(t, err)
	var raw struct {
		Families map[string]struct {
			Classification string `json:"classification"`
		} `json:"families"`
	}
	require.NoError(t, json.Unmarshal(data, &raw))

	validClassifications := map[string]bool{
		"produced_to_sse":      true,
		"governed_record_only": true,
		"dashboard_safe":       true,
		"unsupported":          true,
		"mixed":                true,
	}
	for family, meta := range raw.Families {
		assert.True(t, validClassifications[meta.Classification],
			"family %s has invalid classification %q", family, meta.Classification)
	}
}
