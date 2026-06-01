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

package config

// Settings is deprecated - g8e uses ZERO environment variables.
// All configuration is via CLI flags or config files.
// This struct is kept for backwards compatibility during migration.
type Settings struct {
	// Deprecated: No environment variables are used
	DataDir         string
	GatewayEndpoint string
	PKIDir          string
	ProtocolDir     string
	SecretsDir      string
}

// LoadSettings is deprecated - g8e uses ZERO environment variables.
// Returns empty Settings struct.
// All configuration should use CLI flags or config files.
func LoadSettings() Settings {
	return Settings{}
}
