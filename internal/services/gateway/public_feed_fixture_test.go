// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

//go:build integration

package gateway

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/v2/internal/models"
)

func applyProjectionDefaults(proj models.PublicFeedObject) models.PublicFeedObject {
	merged := models.NewPublicFeedObject(map[string]string{
		"schema_version": "1.3.0",
		"kind":           "catalog_snapshot",
		"dataset_id":     "test-dataset",
		"quality_state":  "live_in_progress",
		"observed_at":    "2026-09-21T00:00:00Z",
	})
	for key, value := range proj {
		merged[key] = value
	}
	return merged
}

func projectionRecordFromObject(t *testing.T, seq int64, proj models.PublicFeedObject) models.PublicFeedRecord {
	t.Helper()
	merged := applyProjectionDefaults(proj)
	recordBytes, err := json.Marshal(merged)
	require.NoError(t, err)
	recordHash := sha256.Sum256(recordBytes)
	return models.PublicFeedRecord{
		Sequence:    seq,
		RecordType:  models.PublicFeedRecordTypeProjection,
		RecordHash:  hex.EncodeToString(recordHash[:]),
		RecordBytes: string(recordBytes),
	}
}

func mustPublicFeedObjectFromJSON(t *testing.T, body string) models.PublicFeedObject {
	t.Helper()
	var obj models.PublicFeedObject
	require.NoError(t, json.Unmarshal([]byte(body), &obj))
	return obj
}

func projectionWithInt64Field(name string, value int64) models.PublicFeedObject {
	obj := models.PublicFeedObject{}
	obj.SetInt64Field(name, value)
	return obj
}
