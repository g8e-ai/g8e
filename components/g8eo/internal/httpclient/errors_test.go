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
	"testing"
)

func TestExtractErrorMessage(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"empty", ``, ``},
		{"string", `"bare error"`, `bare error`},
		{"envelope full", `{"code":"G8E-1800","message":"already registered","category":"auth"}`, `G8E-1800: already registered`},
		{"envelope message only", `{"message":"boom"}`, `boom`},
		{"envelope code only falls through to raw", `{"code":"G8E-0000"}`, `{"code":"G8E-0000"}`},
		{"unknown shape falls through to raw", `[1,2,3]`, `[1,2,3]`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ExtractErrorMessage(json.RawMessage(tt.in))
			if got != tt.want {
				t.Errorf("ExtractErrorMessage(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}
