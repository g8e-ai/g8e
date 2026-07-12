// Copyright (c) 2026 Lateralus Labs, LLC.
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package models

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDocumentForWire_EmptyDataReturnsSystemFieldsOnly(t *testing.T) {
	t.Parallel()

	d := &Document{
		ID:        "doc-1",
		CreatedAt: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		UpdatedAt: time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC),
	}

	out := d.ForWire()

	assert.Len(t, out, 3)
	assert.Contains(t, out, "id")
	assert.Contains(t, out, "created_at")
	assert.Contains(t, out, "updated_at")
}

func TestDocumentForWire_MergesDataAndSystemFields(t *testing.T) {
	t.Parallel()

	d := &Document{
		ID: "doc-2",
		Data: map[string]json.RawMessage{
			"name": json.RawMessage(`"test"`),
			"age":  json.RawMessage(`42`),
		},
		CreatedAt: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		UpdatedAt: time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC),
	}

	out := d.ForWire()

	assert.Len(t, out, 5)
	assert.Contains(t, out, "name")
	assert.Contains(t, out, "age")
	assert.Contains(t, out, "id")
	assert.Contains(t, out, "created_at")
	assert.Contains(t, out, "updated_at")
}

func TestDocumentForWire_IDIsJSONString(t *testing.T) {
	t.Parallel()

	d := &Document{
		ID: "doc-3",
	}

	out := d.ForWire()

	var idVal string
	require.NoError(t, json.Unmarshal(out["id"], &idVal))
	assert.Equal(t, "doc-3", idVal)
}

func TestDocumentForWire_TimestampsAreFixedMicrosecondUTC(t *testing.T) {
	t.Parallel()

	ts := time.Date(2026, 1, 15, 14, 30, 45, 123456789, time.UTC)
	d := &Document{
		ID:        "doc-4",
		CreatedAt: ts,
		UpdatedAt: ts,
	}

	out := d.ForWire()

	var createdStr string
	require.NoError(t, json.Unmarshal(out["created_at"], &createdStr))
	assert.Equal(t, "2026-01-15T14:30:45.123456Z", createdStr)

	var updatedStr string
	require.NoError(t, json.Unmarshal(out["updated_at"], &updatedStr))
	assert.Equal(t, "2026-01-15T14:30:45.123456Z", updatedStr)
}

func TestDocumentForWire_TimestampsConvertedToUTC(t *testing.T) {
	t.Parallel()

	loc, err := time.LoadLocation("America/New_York")
	require.NoError(t, err)

	ts := time.Date(2026, 1, 15, 9, 30, 45, 0, loc)
	d := &Document{
		ID:        "doc-5",
		CreatedAt: ts,
		UpdatedAt: ts,
	}

	out := d.ForWire()

	var createdStr string
	require.NoError(t, json.Unmarshal(out["created_at"], &createdStr))
	assert.Contains(t, createdStr, "T14:30:45")
	assert.Contains(t, createdStr, "Z")
}

func TestDocumentForWire_DataFieldsPreserved(t *testing.T) {
	t.Parallel()

	originalData := map[string]json.RawMessage{
		"field1": json.RawMessage(`"value1"`),
		"field2": json.RawMessage(`123`),
		"field3": json.RawMessage(`true`),
	}

	d := &Document{
		ID:        "doc-6",
		Data:      originalData,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	out := d.ForWire()

	for k, v := range originalData {
		assert.Equal(t, v, out[k], "data field %q should be preserved", k)
	}
}

func TestDocumentForWire_SystemFieldsOverrideDataFieldsWithSameKey(t *testing.T) {
	t.Parallel()

	d := &Document{
		ID: "doc-7",
		Data: map[string]json.RawMessage{
			"id": json.RawMessage(`"should-be-overridden"`),
		},
		CreatedAt: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		UpdatedAt: time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC),
	}

	out := d.ForWire()

	var idVal string
	require.NoError(t, json.Unmarshal(out["id"], &idVal))
	assert.Equal(t, "doc-7", idVal)
}
