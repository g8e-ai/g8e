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

//go:build !windows
// +build !windows

package auth

import (
	"fmt"
	"log/slog"
)

// getWindowsMachineID is a stub for non-Windows platforms
// This should never be called on non-Windows platforms
func getWindowsMachineID(logger *slog.Logger) (string, error) {
	return "", fmt.Errorf("windows machine ID not available on this platform")
}
