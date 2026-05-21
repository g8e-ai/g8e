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

package constants

import "fmt"

// ApiPaths defines the canonical G8E API paths, now sourced from generated constants.
var ApiPaths = ApiPathsGenerated

// GetG8eePath returns the full internal path for a G8ee API route key.
func GetG8eePath(key string) string {
	path, ok := ApiPaths.G8ee[key]
	if !ok {
		return ""
	}
	return fmt.Sprintf("%s%s", ApiPaths.InternalPrefix, path)
}
