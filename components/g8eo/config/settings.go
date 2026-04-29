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

import (
	"os"
	"path/filepath"

	"github.com/g8e-ai/g8e/components/g8eo/constants"
)

// Settings holds all process-level configuration values read at startup.
// This is the ONE place in the application that calls os.Getenv.
// All other code receives these values via dependency injection.
type Settings struct {
	// g8e operator configuration — mirrors shared/constants/env_vars.json "g8eo" section
	OperatorAPIKey    string
	OperatorEndpoint  string
	OperatorSessionID string
	InternalAuthToken string
	SSLDir            string
	PubSubCACert      string
	DeviceToken       string
	LogLevel          string
	DataDir           string
	IPService         string
	IPResolver        string

	// Per-operator mTLS credentials (persisted from registration)
	OperatorCert    string
	OperatorCertKey string

	// OpenClaw
	OpenClawGatewayToken string

	// System / process environment
	Shell       string
	Lang        string
	Term        string
	TZ          string
	Path        string
	SSHAuthSock string
	User        string
	LogName     string
}

// readVar is the single call site for os.Getenv within this package.
// All reads are keyed by typed constants.EnvVarKey values — no raw strings.
func readVar(key constants.EnvVarKey) string {
	return os.Getenv(string(key))
}

// LoadSettings reads all process-level configuration consumed by g8eo.
// It is the single authoritative source — no other package may call os.Getenv.
func LoadSettings() Settings {
	user := readVar(constants.EnvVar.User)
	if user == "" {
		user = readVar(constants.EnvVar.Username)
	}

	apiKey := readVar(constants.EnvVar.OperatorAPIKey)
	dataDir := readVar(constants.EnvVar.DataDir)

	// If no explicit data-dir was provided, we must check the default location
	// (.g8e/data relative to current working directory) for persisted credentials.
	effectiveDataDir := dataDir
	if effectiveDataDir == "" {
		if cwd, err := os.Getwd(); err == nil {
			effectiveDataDir = filepath.Join(cwd, ".g8e", "data")
		}
	}

	// Load persisted API key if not in environment
	if apiKey == "" && effectiveDataDir != "" {
		keyPath := filepath.Join(effectiveDataDir, "operator.key")
		if data, err := os.ReadFile(keyPath); err == nil {
			apiKey = string(data)
		}
	}

	// Load persisted certificates if they exist
	var opCert, opCertKey string
	if effectiveDataDir != "" {
		certPath := filepath.Join(effectiveDataDir, "ssl", "operator.crt")
		keyPath := filepath.Join(effectiveDataDir, "ssl", "operator.key")
		if certData, err := os.ReadFile(certPath); err == nil {
			if keyData, err := os.ReadFile(keyPath); err == nil {
				opCert = string(certData)
				opCertKey = string(keyData)
			}
		}
	}

	return Settings{
		OperatorAPIKey:       apiKey,
		OperatorEndpoint:     readVar(constants.EnvVar.OperatorEndpoint),
		OperatorSessionID:    readVar(constants.EnvVar.OperatorSessionID),
		InternalAuthToken:    readVar(constants.EnvVar.InternalAuthToken),
		SSLDir:               readVar(constants.EnvVar.SSLDir),
		PubSubCACert:         readVar(constants.EnvVar.PubSubCACert),
		DeviceToken:          readVar(constants.EnvVar.DeviceToken),
		LogLevel:             readVar(constants.EnvVar.LogLevel),
		DataDir:              dataDir,
		IPService:            readVar(constants.EnvVar.IPService),
		IPResolver:           readVar(constants.EnvVar.IPResolver),
		OperatorCert:         opCert,
		OperatorCertKey:      opCertKey,
		OpenClawGatewayToken: readVar(constants.EnvVar.OpenClawGatewayToken),
		Shell:                readVar(constants.EnvVar.Shell),
		Lang:                 readVar(constants.EnvVar.Lang),
		Term:                 readVar(constants.EnvVar.Term),
		TZ:                   readVar(constants.EnvVar.TZ),
		Path:                 readVar(constants.EnvVar.Path),
		SSHAuthSock:          readVar(constants.EnvVar.SSHAuthSock),
		User:                 user,
		LogName:              readVar(constants.EnvVar.LogName),
	}
}
