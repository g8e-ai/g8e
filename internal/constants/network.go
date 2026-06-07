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

// Package constants provides network-related constants.
//
// Single source of truth: protocol/constants/network.json
// This file is manually maintained to match the JSON SSOT.
package constants

// DefaultEndpoint is the default g8e Operator endpoint hostname.
// It is also the TLS ServerName used when connecting to a raw IP address,
// because the embedded CA certificate is issued to this hostname.
//
// Source: protocol/constants/network.json
const DefaultEndpoint = "localhost"
