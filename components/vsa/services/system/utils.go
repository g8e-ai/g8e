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

package system

import (
	"strconv"
)

func StringPtr(s string) *string {
	return &s
}

func StringPtrValue(ptr *string) string {
	if ptr == nil {
		return "<nil>"
	}
	return *ptr
}

func IntPtr(i int) *int {
	return &i
}

func IntPtrValue(ptr *int) string {
	if ptr == nil {
		return "<nil>"
	}
	return strconv.Itoa(*ptr)
}
