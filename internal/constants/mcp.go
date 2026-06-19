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

import "time"

// MCP service constants
const (
	// DefaultLogFilterLimit is the default number of log lines to return
	DefaultLogFilterLimit = 100

	// DefaultProcessLimit is the default number of processes to return in metric tools
	DefaultProcessLimit = 10

	// DefaultDiskProfileDepth is the default directory depth for disk profiling
	DefaultDiskProfileDepth = 2

	// DefaultNetworkTimeout is the default timeout for network operations
	DefaultNetworkTimeout = 5 * time.Second

	// DefaultHTTPTimeout is the default timeout for HTTP operations
	DefaultHTTPTimeout = 10 * time.Second
)
