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
	"time"
)

// Document is the internal representation of a stored document.
// Timestamps are native time.Time — convert to wire format via ForWire().
type Document struct {
	ID         string
	Collection string
	Data       map[string]json.RawMessage
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

// ForWire serializes Document to a JSON-encodable map for the HTTP response boundary.
// Timestamps are formatted as RFC3339Nano UTC strings. The caller's data fields are
// merged with system fields (id, created_at, updated_at).
func (d *Document) ForWire() map[string]json.RawMessage {
	out := make(map[string]json.RawMessage, len(d.Data)+3)
	for k, v := range d.Data {
		out[k] = v
	}
	out["id"], _ = json.Marshal(d.ID)
	out["created_at"], _ = json.Marshal(d.CreatedAt.UTC().Format(time.RFC3339Nano))
	out["updated_at"], _ = json.Marshal(d.UpdatedAt.UTC().Format(time.RFC3339Nano))
	return out
}

// DocFilter represents a single filter condition for DocQuery.
// Op must be one of: ==, !=, <, >, <=, >=
type DocFilter struct {
	Field string          `json:"field"`
	Op    string          `json:"op"`
	Value json.RawMessage `json:"value"`
}

// DocQueryRequest is the typed body for POST /db/{collection}/_query.
type DocQueryRequest struct {
	Filters []DocFilter `json:"filters,omitempty"`
	OrderBy string      `json:"order_by,omitempty"`
	Limit   int         `json:"limit,omitempty"`
}

// KVSetRequest is the typed body for PUT /kv/{key}.
type KVSetRequest struct {
	Value string `json:"value"`
	TTL   int    `json:"ttl,omitempty"`
}

// KVExpireRequest is the typed body for PUT /kv/{key}/_expire.
type KVExpireRequest struct {
	TTL int `json:"ttl"`
}

// KVPatternRequest is the typed body for POST /kv/_keys, /kv/_scan, /kv/_delete_pattern.
type KVPatternRequest struct {
	Pattern string `json:"pattern,omitempty"`
	Cursor  int    `json:"cursor,omitempty"`
	Count   int    `json:"count,omitempty"`
}

// PubSubPublishRequest is the typed body for POST /pubsub/publish.
type PubSubPublishRequest struct {
	Channel string          `json:"channel"`
	Data    json.RawMessage `json:"data"`
}

// HealthResponse is the typed response for GET /health.
type HealthResponse struct {
	Status  string `json:"status"`
	Mode    string `json:"mode"`
	Version string `json:"version"`
}

// StatusResponse is the typed response for simple ok/error replies.
type StatusResponse struct {
	Status string `json:"status"`
}

// KVGetResponse is the typed response for GET /kv/{key}.
type KVGetResponse struct {
	Value string `json:"value"`
}

// KVTTLResponse is the typed response for GET /kv/{key}/_ttl.
type KVTTLResponse struct {
	TTL int `json:"ttl"`
}

// KVKeysResponse is the typed response for POST /kv/_keys.
type KVKeysResponse struct {
	Keys []string `json:"keys"`
}

// KVScanResponse is the typed response for POST /kv/_scan.
type KVScanResponse struct {
	Cursor int      `json:"cursor"`
	Keys   []string `json:"keys"`
}

// KVDeletePatternResponse is the typed response for POST /kv/_delete_pattern.
type KVDeletePatternResponse struct {
	Deleted int64 `json:"deleted"`
}

// PubSubPublishResponse is the typed response for POST /pubsub/publish.
type PubSubPublishResponse struct {
	Receivers int `json:"receivers"`
}

// BlobMetaResponse is the typed response for GET /blob/{namespace}/{id}/meta.
type BlobMetaResponse struct {
	ID          string    `json:"id"`
	Namespace   string    `json:"namespace"`
	Size        int64     `json:"size"`
	ContentType string    `json:"content_type"`
	CreatedAt   time.Time `json:"created_at"`
}

// BlobDeleteResponse is the typed response for DELETE /blob/{namespace}/{id} and DELETE /blob/{namespace}.
type BlobDeleteResponse struct {
	Deleted int64 `json:"deleted"`
}

// SettingsDocument represents the platform_settings document structure.
// Authority: shared/models/platform_settings.json
type SettingsDocument struct {
	Settings  map[string]interface{} `json:"settings"`
	CreatedAt time.Time              `json:"created_at"`
	UpdatedAt time.Time              `json:"updated_at"`
}

// UserSettingsDocument represents the user_settings document structure.
// Authority: shared/models/user_settings.json
type UserSettingsDocument struct {
	Settings  map[string]interface{} `json:"settings"`
	CreatedAt time.Time              `json:"created_at"`
	UpdatedAt time.Time              `json:"updated_at"`
}
