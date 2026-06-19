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

// Ports defines canonical G8E networking ports.
// These values are generated from protocol/constants/ports.json (SSOT).
var Ports = struct {
	OperatorHttp       int `json:"OperatorHttp"`
	OperatorHttps      int `json:"OperatorHttps"`
	LocalHttpStdioGateway int `json:"LocalHttpStdioGateway"`
	// Snake_case variants for protocol compatibility
	LocalHttpStdioGatewaySnake int `json:"local_http_stdio_gateway"`
	OperatorHttpSnake       int `json:"operator_http"`
	OperatorHttpsSnake      int `json:"operator_https"`
}{
	OperatorHttp:            8080,
	OperatorHttps:           8443,
	LocalHttpStdioGateway:      18789,
	LocalHttpStdioGatewaySnake: 18789,
	OperatorHttpSnake:       8080,
	OperatorHttpsSnake:      8443,
}
