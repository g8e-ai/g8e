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

// EnvVarKey is a typed string for environment variable names.
// No package outside of config/ may call os.Getenv directly — use these
// constants as the keys passed to config.LoadEnv().
type EnvVarKey string

// envVarKeys groups all environment variable name constants consumed by g8eo.
type envVarKeys struct {
	// g8e operator configuration
	OperatorAPIKey          EnvVarKey
	OperatorSessionID       EnvVarKey
	OperatorEndpoint        EnvVarKey
	InternalAuthToken       EnvVarKey
	SSLDir                  EnvVarKey
	PubSubCACert            EnvVarKey
	DeviceToken             EnvVarKey
	LogLevel                EnvVarKey
	LocalStoreEnabled       EnvVarKey
	LocalDBPath             EnvVarKey
	LocalStoreMaxSizeMB     EnvVarKey
	LocalStoreRetentionDays EnvVarKey
	DataDir                 EnvVarKey
	IPService               EnvVarKey
	IPResolver              EnvVarKey

	// OpenClaw
	OpenClawGatewayToken EnvVarKey

	// VSODB configuration
	OperatorPubSubURL EnvVarKey

	// Test environment
	TestTmpDir EnvVarKey

	Shell       EnvVarKey
	Lang        EnvVarKey
	Term        EnvVarKey
	TZ          EnvVarKey
	Path        EnvVarKey
	SSHAuthSock EnvVarKey
	User        EnvVarKey
	Username    EnvVarKey
	LogName     EnvVarKey
}

// EnvVar is the package-level entry point for all environment variable name constants.
// Usage: constants.EnvVar.OperatorAPIKey
var EnvVar = envVarKeys{
	OperatorAPIKey:          "G8E_OPERATOR_API_KEY",
	OperatorSessionID:       "G8E_OPERATOR_SESSION_ID",
	OperatorEndpoint:        "G8E_OPERATOR_ENDPOINT",
	InternalAuthToken:       "G8E_INTERNAL_AUTH_TOKEN",
	SSLDir:                  "G8E_SSL_DIR",
	PubSubCACert:            "G8E_PUBSUB_CA_CERT",
	DeviceToken:             "G8E_DEVICE_TOKEN",
	LogLevel:                "G8E_LOG_LEVEL",
	LocalStoreEnabled:       "G8E_LOCAL_STORE_ENABLED",
	LocalDBPath:             "G8E_LOCAL_DB_PATH",
	LocalStoreMaxSizeMB:     "G8E_LOCAL_STORE_MAX_SIZE_MB",
	LocalStoreRetentionDays: "G8E_LOCAL_STORE_RETENTION_DAYS",
	DataDir:                 "G8E_DATA_DIR",
	IPService:               "G8E_IP_SERVICE",
	IPResolver:              "G8E_IP_RESOLVER",

	OpenClawGatewayToken: "OPENCLAW_GATEWAY_TOKEN",

	OperatorPubSubURL: "G8E_OPERATOR_PUBSUB_URL",
	TestTmpDir:        "G8E_TEST_TMPDIR",
	Shell:             "SHELL",
	Lang:              "LANG",
	Term:              "TERM",
	TZ:                "TZ",
	Path:              "PATH",
	SSHAuthSock:       "SSH_AUTH_SOCK",
	User:              "USER",
	Username:          "USERNAME",
	LogName:           "LOGNAME",
}
