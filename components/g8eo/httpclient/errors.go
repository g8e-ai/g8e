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

package httpclient

import (
	"encoding/json"
	"fmt"
)

// ExtractErrorMessage returns a human-readable error string from a raw JSON
// `error` field produced by g8ed, accepting either:
//   - a plain JSON string: "some error"
//   - the standard g8ed error envelope object: {"code": "...", "message": "...", ...}
//
// g8eo HTTP response structs should model `error` as json.RawMessage rather
// than `string`, and call this helper when surfacing the error to the user.
// Modeling it as a bare `string` causes a silent decode failure whenever the
// server returns the object form, masking the real server error.
func ExtractErrorMessage(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return s
	}
	var obj struct {
		Message string `json:"message"`
		Code    string `json:"code"`
	}
	if err := json.Unmarshal(raw, &obj); err == nil {
		if obj.Message != "" && obj.Code != "" {
			return fmt.Sprintf("%s: %s", obj.Code, obj.Message)
		}
		if obj.Message != "" {
			return obj.Message
		}
	}
	return string(raw)
}
