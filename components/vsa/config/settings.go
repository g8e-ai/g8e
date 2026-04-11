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

	"github.com/g8e-ai/g8e/components/vsa/constants"
)

// Settings holds all process-level configuration values read at startup.
// This is the ONE place in the application that calls os.Getenv.
// All other code receives these values via dependency injection.
type Settings struct {
	// g8e operator configuration — mirrors shared/constants/env_vars.json "vsa" section
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

// LoadSettings reads all process-level configuration consumed by VSA.
// It is the single authoritative source — no other package may call os.Getenv.
func LoadSettings() Settings {
	user := readVar(constants.EnvVar.User)
	if user == "" {
		user = readVar(constants.EnvVar.Username)
	}

	return Settings{
		OperatorAPIKey:       readVar(constants.EnvVar.OperatorAPIKey),
		OperatorEndpoint:     readVar(constants.EnvVar.OperatorEndpoint),
		OperatorSessionID:    readVar(constants.EnvVar.OperatorSessionID),
		InternalAuthToken:    readVar(constants.EnvVar.InternalAuthToken),
		SSLDir:               readVar(constants.EnvVar.SSLDir),
		PubSubCACert:         readVar(constants.EnvVar.PubSubCACert),
		DeviceToken:          readVar(constants.EnvVar.DeviceToken),
		LogLevel:             readVar(constants.EnvVar.LogLevel),
		DataDir:              readVar(constants.EnvVar.DataDir),
		IPService:            readVar(constants.EnvVar.IPService),
		IPResolver:           readVar(constants.EnvVar.IPResolver),
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
