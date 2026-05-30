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
type EnvVarKey string

// EnvVar groups all environment variable name constants consumed by g8eo.
var EnvVar = struct {
	OperatorHTTPSPort          EnvVarKey
	OperatorBootstrapHTTPSPort EnvVarKey
	OperatorPublicHTTPSPort    EnvVarKey
	OperatorWSSPort            EnvVarKey
	PIDDir                     EnvVarKey
	LogDir                     EnvVarKey
	OperatorPIDFile            EnvVarKey
	OperatorLogFile            EnvVarKey
	LogMaxBackups              EnvVarKey
	LLMaxTokens                EnvVarKey
	LLMCommandGenEnabled       EnvVarKey
	LLMCommandGenAuditor       EnvVarKey
	LLMCommandGenPasses        EnvVarKey
	SessionEncryptionKey       EnvVarKey
	InternalAPIKey             EnvVarKey
	AllowedOrigins             EnvVarKey
	PasskeyRPName              EnvVarKey
	PasskeyRPID                EnvVarKey
	PasskeyOrigin              EnvVarKey
	VertexSearchEnabled        EnvVarKey
	VertexSearchProjectID      EnvVarKey
	VertexSearchEngineID       EnvVarKey
	VertexSearchLocation       EnvVarKey
	VertexSearchAPIKey         EnvVarKey
	GoogleSearchEnabled        EnvVarKey
	GoogleSearchAPIKey         EnvVarKey
	GoogleSearchEngineID       EnvVarKey
	OperatorURL                EnvVarKey
	OperatorPubSubURL          EnvVarKey
	OperatorBlobURL            EnvVarKey
	DockerGID                  EnvVarKey
	SessionTTL                 EnvVarKey
	AbsoluteSessionTimeout     EnvVarKey
	Environment                EnvVarKey
	UploadPath                 EnvVarKey
	DocsDir                    EnvVarKey
	DashboardURL               EnvVarKey
	OperatorEndpoint           EnvVarKey
	EnableCommandWhitelisting  EnvVarKey
	EnableCommandBlacklisting  EnvVarKey
	PKIDir                     EnvVarKey
	SecretsDir                 EnvVarKey
	PubSubCACert               EnvVarKey
	OperatorSessionID          EnvVarKey
	LogLevel                   EnvVarKey
	DataDir                    EnvVarKey
	LocalStoreEnabled          EnvVarKey
	LocalDBPath                EnvVarKey
	LocalStoreMaxSizeMB        EnvVarKey
	LocalStoreRetentionDays    EnvVarKey
	IPService                  EnvVarKey
	IPResolver                 EnvVarKey
	Shell                      EnvVarKey
	Lang                       EnvVarKey
	Term                       EnvVarKey
	TZ                         EnvVarKey
	Path                       EnvVarKey
	SSHAuthSock                EnvVarKey
	User                       EnvVarKey
	Username                   EnvVarKey
	LogName                    EnvVarKey
	OperatorID                 EnvVarKey
	CLISessionID               EnvVarKey
	ProtocolDir                EnvVarKey
	TestTmpDir                 EnvVarKey
	TestOperatorPubSubURL      EnvVarKey
	StrictConstantsLint        EnvVarKey
	RuntimeDir                 EnvVarKey
	SSHConfigPath              EnvVarKey
	ProjectRoot                EnvVarKey
	TestLLMAssistantProvider   EnvVarKey
	TestLLMAssistantModel      EnvVarKey
	TestLLMLiteProvider        EnvVarKey
	TestLLMLiteModel           EnvVarKey
	TestLLMAssistantAPIKey     EnvVarKey
	TestLLMLiteAPIKey          EnvVarKey
	TestLLMaxTokens            EnvVarKey
	AuditorHMACKey             EnvVarKey
	TestLLMPrimaryProvider     EnvVarKey
	TestLLMPrimaryAPIKey       EnvVarKey
	TestLLMPrimaryModel        EnvVarKey
	TestLLMPrimaryEndpoint     EnvVarKey
	TestLLMAssistantEndpoint   EnvVarKey
	TestLLMLiteEndpoint        EnvVarKey
}{
	OperatorHTTPSPort:          "G8E_OPERATOR_HTTPS_PORT",
	OperatorBootstrapHTTPSPort: "G8E_REMOTE_OPERATOR_BOOTSTRAP_HTTPS_PORT",
	OperatorPublicHTTPSPort:    "G8E_OPERATOR_PUBLIC_HTTPS_PORT",
	OperatorWSSPort:            "G8E_OPERATOR_PUBLIC_WSS_PORT",
	PIDDir:                     "G8E_PID_DIR",
	LogDir:                     "G8E_LOG_DIR",
	OperatorPIDFile:            "G8E_OPERATOR_PID_FILE",
	OperatorLogFile:            "G8E_OPERATOR_LOG_FILE",
	LogMaxBackups:              "G8E_LOG_MAX_BACKUPS",
	LLMaxTokens:                "G8E_LLM_MAX_TOKENS",
	LLMCommandGenEnabled:       "G8E_LLM_COMMAND_GEN_ENABLED",
	LLMCommandGenAuditor:       "G8E_LLM_COMMAND_GEN_AUDITOR",
	LLMCommandGenPasses:        "G8E_LLM_COMMAND_GEN_PASSES",
	SessionEncryptionKey:       "G8E_SESSION_ENCRYPTION_KEY",
	InternalAPIKey:             "G8E_INTERNAL_API_KEY",
	AllowedOrigins:             "G8E_ALLOWED_ORIGINS",
	PasskeyRPName:              "G8E_PASSKEY_RP_NAME",
	PasskeyRPID:                "G8E_PASSKEY_RP_ID",
	PasskeyOrigin:              "G8E_PASSKEY_ORIGIN",
	VertexSearchEnabled:        "G8E_VERTEX_SEARCH_ENABLED",
	VertexSearchProjectID:      "G8E_VERTEX_SEARCH_PROJECT_ID",
	VertexSearchEngineID:       "G8E_VERTEX_SEARCH_ENGINE_ID",
	VertexSearchLocation:       "G8E_VERTEX_SEARCH_LOCATION",
	VertexSearchAPIKey:         "G8E_VERTEX_SEARCH_API_KEY",
	GoogleSearchEnabled:        "G8E_GOOGLE_SEARCH_ENABLED",
	GoogleSearchAPIKey:         "G8E_GOOGLE_SEARCH_API_KEY",
	GoogleSearchEngineID:       "G8E_GOOGLE_SEARCH_ENGINE_ID",
	OperatorURL:                "G8E_OPERATOR_URL",
	OperatorPubSubURL:          "G8E_OPERATOR_PUBSUB_URL",
	OperatorBlobURL:            "G8E_OPERATOR_BLOB_URL",
	DockerGID:                  "G8E_DOCKER_GID",
	SessionTTL:                 "G8E_SESSION_TTL",
	AbsoluteSessionTimeout:     "G8E_ABSOLUTE_SESSION_TIMEOUT",
	Environment:                "G8E_ENVIRONMENT",
	UploadPath:                 "G8E_UPLOAD_PATH",
	DocsDir:                    "G8E_DOCS_DIR",
	DashboardURL:               "G8E_DASHBOARD_URL",
	OperatorEndpoint:           "G8E_OPERATOR_ENDPOINT",
	EnableCommandWhitelisting:  "G8E_ENABLE_COMMAND_WHITELISTING",
	EnableCommandBlacklisting:  "G8E_ENABLE_COMMAND_BLACKLISTING",
	PKIDir:                     "G8E_PKI_DIR",
	SecretsDir:                 "G8E_SECRETS_DIR",
	PubSubCACert:               "G8E_PUBSUB_CA_CERT",
	OperatorSessionID:          "G8E_OPERATOR_SESSION_ID",
	LogLevel:                   "G8E_LOG_LEVEL",
	DataDir:                    "G8E_DATA_DIR",
	LocalStoreEnabled:          "G8E_LOCAL_STORE_ENABLED",
	LocalDBPath:                "G8E_LOCAL_DB_PATH",
	LocalStoreMaxSizeMB:        "G8E_LOCAL_STORE_MAX_SIZE_MB",
	LocalStoreRetentionDays:    "G8E_LOCAL_STORE_RETENTION_DAYS",
	IPService:                  "G8E_IP_SERVICE",
	IPResolver:                 "G8E_IP_RESOLVER",
	Shell:                      "SHELL",
	Lang:                       "LANG",
	Term:                       "TERM",
	TZ:                         "TZ",
	Path:                       "PATH",
	SSHAuthSock:                "SSH_AUTH_SOCK",
	User:                       "USER",
	Username:                   "USERNAME",
	LogName:                    "LOGNAME",
	OperatorID:                 "G8E_OPERATOR_ID",
	CLISessionID:               "G8E_CLI_SESSION_ID",
	ProtocolDir:                "G8E_PROTOCOL_DIR",
	TestTmpDir:                 "G8E_TEST_TMP_DIR",
	TestOperatorPubSubURL:      "G8E_TEST_OPERATOR_PUBSUB_URL",
	StrictConstantsLint:        "G8E_STRICT_CONSTANTS_LINT",
	RuntimeDir:                 "G8E_RUNTIME_DIR",
	SSHConfigPath:              "G8E_SSH_CONFIG_PATH",
	ProjectRoot:                "G8E_PROJECT_ROOT",
	TestLLMAssistantProvider:   "G8E_TEST_LLM_ASSISTANT_PROVIDER",
	TestLLMAssistantModel:      "G8E_TEST_LLM_ASSISTANT_MODEL",
	TestLLMLiteProvider:        "G8E_TEST_LLM_LITE_PROVIDER",
	TestLLMLiteModel:           "G8E_TEST_LLM_LITE_MODEL",
	TestLLMAssistantAPIKey:     "G8E_TEST_LLM_ASSISTANT_API_KEY",
	TestLLMLiteAPIKey:          "G8E_TEST_LLM_LITE_API_KEY",
	TestLLMaxTokens:            "G8E_TEST_LLM_MAX_TOKENS",
	AuditorHMACKey:             "G8E_AUDITOR_HMAC_KEY",
	TestLLMPrimaryProvider:     "G8E_TEST_LLM_PRIMARY_PROVIDER",
	TestLLMPrimaryAPIKey:       "G8E_TEST_LLM_PRIMARY_API_KEY",
	TestLLMPrimaryModel:        "G8E_TEST_LLM_PRIMARY_MODEL",
	TestLLMPrimaryEndpoint:     "G8E_TEST_LLM_PRIMARY_ENDPOINT",
	TestLLMAssistantEndpoint:   "G8E_TEST_LLM_ASSISTANT_ENDPOINT",
	TestLLMLiteEndpoint:        "G8E_TEST_LLM_LITE_ENDPOINT",
}
