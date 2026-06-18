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

import "errors"

// Standard platform errors
var (
	ErrUserNotFound         = errors.New("user not found")
	ErrNoPasskeysRegistered = errors.New("no passkeys registered")
	ErrInvalidJSONBody      = errors.New("invalid JSON body")
	ErrUserIDRequired       = errors.New("user_id required")
	ErrMethodNotAllowed     = errors.New("method not allowed")
	ErrForbidden            = errors.New("forbidden")
	ErrInternal             = errors.New("internal error")
	ErrNotFound             = errors.New("not found")
	ErrAlreadyExists        = errors.New("already exists")
	ErrConstraintViolation  = errors.New("constraint violation")
	ErrDatabaseLocked       = errors.New("database is locked")
	ErrServiceUnavailable   = errors.New("service unavailable")
	ErrDatabaseReplay       = errors.New("database replay detected")
	ErrDuplicateColumn      = errors.New("duplicate column name")
	ErrProcessKilled        = errors.New("process killed")
	ErrTrustBundleStale     = errors.New("trust bundle stale")
	ErrKeyNotFound          = errors.New("key not found")
	ErrExpired              = errors.New("expired")
	ErrAgentNotFound        = errors.New("agent not found")
	ErrAgentNotInPath       = errors.New("agent binary not found in PATH")
	ErrAgentNotSupported    = errors.New("agent auto-launch not supported")
	ErrConfigFileExists     = errors.New("config file already exists")
	ErrEndpointRequired     = errors.New("endpoint required")
	ErrConfigLoadFailed     = errors.New("config load failed")
	ErrCSRGenerationFailed  = errors.New("CSR generation failed")
	ErrEnrollmentFailed     = errors.New("enrollment failed")
	ErrMissingCertificate   = errors.New("missing certificate")
	ErrDirCreateFailed      = errors.New("directory creation failed")
	ErrCertSaveFailed       = errors.New("certificate save failed")
	ErrChainSaveFailed      = errors.New("certificate chain save failed")
	ErrTrustSaveFailed      = errors.New("trust bundle save failed")
	ErrValidationFailed     = errors.New("security validation failed")
	ErrPEMDecodeFailed      = errors.New("failed to decode PEM block")
	ErrInvalidPEMType       = errors.New("invalid PEM block type")
	ErrHTTPStatusError      = errors.New("HTTP status error")
	ErrEmptyTrustBundle     = errors.New("trust bundle is empty")
	ErrCAParseFailed        = errors.New("failed to parse CA certificates")
	ErrMissingRequiredField = errors.New("missing required field")
	ErrInvalidLogLevel      = errors.New("invalid log level")

	// Keystore errors
	ErrKeyStoreKeyNotFound = errors.New("master key not found in OS key store")
	ErrKeyStoreLocked      = errors.New("OS key store is locked/unavailable")
	ErrInvalidCiphertext   = errors.New("invalid ciphertext or authentication failed")
	ErrOSNotSupported      = errors.New("OS not supported for OS-native key store")

	// Ledger errors
	ErrLedgerDisabled       = errors.New("ledger is disabled")
	ErrLedgerConfigRequired = errors.New("ledger config is required")
	ErrLedgerVaultRequired  = errors.New("ledger encryption vault is required")

	// Ledger status messages
	LedgerStatusFileDeleted = "file deleted"

	// CLI approval errors
	ErrKeyReadFailed            = errors.New("failed to read CLI private key")
	ErrKeyParseFailed           = errors.New("failed to parse private key")
	ErrInvalidKeyType           = errors.New("private key is not Ed25519")
	ErrCertReadFailed           = errors.New("failed to read CLI certificate")
	ErrCertParseFailed          = errors.New("failed to parse certificate")
	ErrRequestMarshalFailed     = errors.New("failed to marshal request")
	ErrTransactionApproveFailed = errors.New("failed to approve transaction")
	ErrResponseParseFailed      = errors.New("failed to parse response")

	// Notary errors
	ErrTransactionExpired = errors.New("transaction approval expired")

	// CLI authentication errors
	ErrNotAuthenticated              = errors.New("not authenticated")
	ErrFailedToLoadCredentials       = errors.New("failed to load credentials")
	ErrFailedToLoadClientCertificate = errors.New("failed to load client certificate")
	ErrFailedToReadTrustBundle       = errors.New("failed to read trust bundle")
	ErrFailedToParseTrustBundle      = errors.New("failed to parse trust bundle")
	ErrFailedToParsePaths            = errors.New("failed to parse paths.json")

	// Process manager errors
	ErrProcessStartFailed = errors.New("process start failed")
	ErrProcessStopFailed  = errors.New("process stop failed")
	ErrPortUnavailable    = errors.New("port unavailable")
	ErrInvalidPosture     = errors.New("invalid posture")
	ErrPIDReadFailed      = errors.New("failed to read PID file")
	ErrPIDWriteFailed     = errors.New("failed to write PID file")
	ErrPostureReadFailed  = errors.New("failed to read posture file")
	ErrPostureWriteFailed = errors.New("failed to write posture file")

	// File system errors
	ErrPathNotFound   = errors.New("path not found")
	ErrStatFailed     = errors.New("failed to stat path")
	ErrNotADirectory  = errors.New("path is not a directory")
	ErrPathValidation = errors.New("invalid path")
	ErrDirectoryList  = errors.New("failed to list directory")
	ErrDirectoryRead  = errors.New("failed to read directory")

	// Execution service errors
	ErrExecutionServiceStopping = errors.New("execution service is stopping")
	ErrExecutionNotFound        = errors.New("execution not found")
	ErrEmptyCommand             = errors.New("empty command")
	ErrCommandLookup            = errors.New("command lookup failed")
	ErrShellRequired            = errors.New("shell required but not found")
	ErrCloudCLIBlocked          = errors.New("cloud CLI command blocked")

	// MCP service errors
	ErrMCPUnmarshalArguments = errors.New("unmarshal arguments")
	ErrMCPGetDiskUsage       = errors.New("get disk usage")
	ErrMCPStatFS             = errors.New("statfs")
	ErrMCPParseMounts        = errors.New("parse mounts")
	ErrMCPReadMounts         = errors.New("read /proc/mounts")
	ErrMCPHostPortRequired   = errors.New("host and port required")
	ErrMCPGetAbsolutePath    = errors.New("get absolute path")
	ErrMCPGetDiskFreeSpaceEx = errors.New("GetDiskFreeSpaceExW failed")

	// MCP registry errors
	ErrMCPSchemaNil               = errors.New("schema cannot be nil")
	ErrMCPSchemaMissingType       = errors.New("schema missing required 'type' field")
	ErrMCPSchemaInvalidType       = errors.New("schema 'type' must be 'object'")
	ErrMCPSchemaInvalidProperties = errors.New("schema 'properties' must be an object")
	ErrMCPSchemaInvalidRequired   = errors.New("schema 'required' must be an array")
	ErrMCPToolNil                 = errors.New("cannot register nil tool")
	ErrMCPToolNameEmpty           = errors.New("tool name cannot be empty")
	ErrMCPToolNameInvalid         = errors.New("invalid tool name: must contain only lowercase letters, digits, and underscores")
	ErrMCPToolAlreadyRegistered   = errors.New("tool is already registered")
	ErrMCPRegistryNil             = errors.New("registry cannot be nil")

	// MCP validation errors
	ErrMCPValidateSQLQueryEmpty                 = errors.New("SQL query cannot be empty")
	ErrMCPValidateSQLQueryTrailingSemicolon     = errors.New("SQL query must not end with semicolon")
	ErrMCPValidateURLInvalidScheme              = errors.New("only http and https schemes are allowed")
	ErrMCPValidateURLMissingHost                = errors.New("URL must have a host")
	ErrMCPValidateURLLoopbackAddress            = errors.New("localhost and loopback addresses are not allowed")
	ErrMCPValidateURLPrivateAddress             = errors.New("private and loopback IP addresses are not allowed")
	ErrMCPValidateProcNetInvalidProtocol        = errors.New("invalid protocol")
	ErrMCPValidatePathEmpty                     = errors.New("path cannot be empty")
	ErrMCPValidatePathWhitespace                = errors.New("path must not contain leading/trailing whitespace")
	ErrMCPValidatePathParentDirRef              = errors.New("path must not contain parent directory references (..)")
	ErrMCPValidatePathNullBytes                 = errors.New("path must not contain null bytes")
	ErrMCPValidateRefEmpty                      = errors.New("git reference cannot be empty")
	ErrMCPValidateRefWhitespace                 = errors.New("git reference must not contain leading/trailing whitespace")
	ErrMCPValidateRefNullBytes                  = errors.New("git reference must not contain null bytes")
	ErrMCPValidateRefDangerousChar              = errors.New("git reference contains dangerous character")
	ErrMCPValidateRefAbsolutePath               = errors.New("git reference must not be an absolute path")
	ErrMCPValidateRefInvalidChars               = errors.New("git reference contains invalid characters")
	ErrMCPValidateK8sNameEmpty                  = errors.New("resource name cannot be empty")
	ErrMCPValidateK8sNameWhitespace             = errors.New("resource name must not contain leading/trailing whitespace")
	ErrMCPValidateK8sNameTooLong                = errors.New("resource name must not exceed 253 characters")
	ErrMCPValidateK8sNameInvalidPattern         = errors.New("resource name must consist of lowercase alphanumeric characters, hyphens, or dots, and must start and end with an alphanumeric character")
	ErrMCPValidateK8sNameNullBytes              = errors.New("resource name must not contain null bytes")
	ErrMCPValidateK8sNamespaceEmpty             = errors.New("namespace cannot be empty")
	ErrMCPValidateK8sNamespaceWhitespace        = errors.New("namespace must not contain leading/trailing whitespace")
	ErrMCPValidateK8sNamespaceTooLong           = errors.New("namespace must not exceed 63 characters")
	ErrMCPValidateK8sNamespaceInvalidPattern    = errors.New("namespace must consist of lowercase alphanumeric characters or hyphens, and must start and end with an alphanumeric character")
	ErrMCPValidateK8sNamespaceNullBytes         = errors.New("namespace must not contain null bytes")
	ErrMCPValidateCloudMetadataInvalidOperation = errors.New("invalid operation")
	ErrMCPValidateHostnameEmpty                 = errors.New("hostname cannot be empty")
	ErrMCPValidateHostnameWhitespace            = errors.New("hostname must not contain leading/trailing whitespace")
	ErrMCPValidateHostnameNullBytes             = errors.New("hostname must not contain null bytes")
	ErrMCPValidateHostnameDangerousChar         = errors.New("hostname contains dangerous character")
	ErrMCPValidateHostnamesEmpty                = errors.New("hostnames list cannot be empty")
	ErrMCPValidateOperatorArgsNullBytes         = errors.New("argument must not contain null bytes")
	ErrMCPValidateOperatorArgsDangerousChar     = errors.New("argument contains dangerous character")

	// MCP OOM detection errors
	ErrMCPGetWorkingDirectory = errors.New("get working directory")
	ErrMCPOpenLogFile         = errors.New("open log file")
	ErrMCPReadLogFile         = errors.New("read log file")
	ErrMCPMarshalOOMResult    = errors.New("marshal OOM result")

	// MCP SSH known hosts errors
	ErrMCPGetHomeDirectory = errors.New("get home directory")
	ErrMCPMarshalResult    = errors.New("marshal result")

	// MCP proc signal safe errors
	ErrMCPProcSignalRequired = errors.New("pid and signal required")

	// MCP git ops errors
	ErrMCPGitOpsUnsupportedOperation = errors.New("unsupported operation")

	// MCP TLS cert inspect errors
	ErrMCPTLSCertInspectRequired = errors.New("either cert_path or host must be specified")

	// Run shell command errors
	ErrMCPRunShellCommandRequired              = errors.New("command is required")
	ErrMCPRunShellCommandSafetyRejected        = errors.New("command rejected by safety policy")
	ErrMCPRunShellCommandTimeoutExceeded       = errors.New("timeout cannot exceed 300 seconds")
	ErrMCPRunShellCommandMarshalResult         = errors.New("marshal result")
	ErrMCPRunShellCommandBlocked               = errors.New("command is blocked by safety policy")
	ErrMCPRunShellCommandDangerousPattern      = errors.New("command contains dangerous pattern")
	ErrMCPRunShellCommandShellInjection        = errors.New("command contains shell injection pattern")
	ErrMCPRunShellCommandPathTraversal         = errors.New("working directory contains path traversal")
	ErrMCPRunShellCommandAbsolutePathRequired  = errors.New("working directory must be an absolute path")
	ErrMCPRunShellCommandDirNotAccessible      = errors.New("working directory does not exist or is not accessible")
	ErrMCPRunShellCommandNotDirectory          = errors.New("working directory is not a directory")
	ErrMCPRunShellCommandShellMetacharacter    = errors.New("command contains shell metacharacter which is not allowed for SSH execution")
	ErrMCPRunShellCommandArgMetacharacter      = errors.New("argument contains shell metacharacter which is not allowed for SSH execution")
	ErrMCPRunShellCommandDirMetacharacter      = errors.New("working directory contains shell metacharacter which is not allowed for SSH execution")
	ErrMCPRunShellCommandResolveHost           = errors.New("resolve host")
	ErrMCPRunShellCommandHostnameResolveFailed = errors.New("failed to resolve hostname")
	ErrMCPRunShellCommandBuildAuth             = errors.New("build auth methods")
	ErrMCPRunShellCommandNoAuth                = errors.New("no SSH auth methods available")
	ErrMCPRunShellCommandHostKeyVerification   = errors.New("host key verification failed")
	ErrMCPRunShellCommandSSHDial               = errors.New("SSH dial failed")
	ErrMCPRunShellCommandSSHSession            = errors.New("SSH session creation failed")
	ErrMCPRunShellCommandTimedOut              = errors.New("command timed out")

	// Network identity detection errors
	ErrNetworkDetectInterfaces    = errors.New("failed to get network interfaces")
	ErrNetworkOpenHostsFile       = errors.New("failed to open hosts file")
	ErrNetworkScanHostsFile       = errors.New("error scanning hosts file")
	ErrNetworkDetectMDNS          = errors.New("detect mDNS names")
	ErrNetworkDetectHostnames     = errors.New("detect hostnames")
	ErrNetworkDetectIPs           = errors.New("detect IPs")
	ErrNetworkDetectHostsAliases  = errors.New("detect hosts file aliases")
	ErrNetworkDetectDNSPTR        = errors.New("detect DNS PTR records")
	ErrNetworkDetectSSHKnownHosts = errors.New("detect SSH known hosts")
	ErrNetworkDetectWindows       = errors.New("detect Windows identity")
	ErrNetworkGetHostname         = errors.New("get hostname")
	ErrNetworkGetSysteminfo       = errors.New("get systeminfo")

	// Audit service errors
	ErrAuditUnmarshalUserMsg      = errors.New("failed to unmarshal user message payload")
	ErrAuditUnmarshalAIMsg        = errors.New("failed to unmarshal AI message payload")
	ErrAuditUnmarshalDirectCmd    = errors.New("failed to unmarshal direct command payload")
	ErrAuditUnmarshalDirectResult = errors.New("failed to unmarshal direct command result payload")
	ErrAuditRecordUserMsg         = errors.New("failed to record user message")
	ErrAuditRecordAIMsg           = errors.New("failed to record AI message")
	ErrAuditRecordDirectCmd       = errors.New("failed to record direct command")
	ErrAuditRecordDirectResult    = errors.New("failed to record direct command result")

	// PubSub service errors
	ErrPubSubEmptyPayload         = errors.New("empty payload")
	ErrPubSubTransactionVerifier  = errors.New("transaction verifier not configured")
	ErrPubSubActuator             = errors.New("actuator not configured")
	ErrPubSubL4Warden             = errors.New("L4Warden not configured")
	ErrPubSubMCPGateway           = errors.New("MCP gateway not configured")
	ErrPubSubMCPMissingToolName   = errors.New("MCP call missing tool_name")
	ErrPubSubA2AGateway           = errors.New("A2A gateway not configured")
	ErrPubSubA2AMissingSkillName  = errors.New("A2A call missing skill_name")
	ErrPubSubActuatorOrAuditStore = errors.New("actuator or ConsoleAuditStore not configured")

	// Scrubbing service errors
	ErrScrubbingInvalidPattern = errors.New("invalid custom scrub pattern")
	ErrScrubbingRegexTimeout   = errors.New("regex compilation timeout")
	ErrScrubbingTokenPersist   = errors.New("failed to persist token to local store")
	ErrScrubbingTokenLoad      = errors.New("failed to load persisted tokens from TokenStore")
	ErrScrubbingTokenKeyFormat = errors.New("invalid token key format")
	ErrScrubbingTokenSequence  = errors.New("failed to parse token sequence")

	// Gateway service errors
	ErrGatewayToolNameRequired             = errors.New("tool name required")
	ErrGatewayInvalidToolArguments         = errors.New("invalid tool arguments: must be a valid JSON object")
	ErrGatewayFieldPathRegistryNotInit     = errors.New("field path registry not initialized")
	ErrGatewayDatabaseServiceNotConfigured = errors.New("database service not configured")
	ErrGatewayCollectionRequired           = errors.New("collection required")
	ErrGatewayDocumentIDRequired           = errors.New("document_id required")
	ErrGatewayFieldPathRequired            = errors.New("field_path required")
	ErrGatewayOperatorSessionIDRequired    = errors.New("operator_session_id required")
	ErrGatewayOperatorSessionInvalid       = errors.New("operator session is invalid or expired")
	ErrGatewayURIRequired                  = errors.New("uri required")
	ErrGatewayNameRequired                 = errors.New("name required")
	ErrGatewaySkillNameRequired            = errors.New("skill_name required")
	ErrGatewayNoDownstreamConfigured       = errors.New("no downstream MCP server configured")
	ErrGatewayDownstreamUnavailable        = errors.New("downstream MCP server is temporarily unavailable (circuit open)")
	ErrGatewayNoA2ADownstreamConfigured    = errors.New("no downstream A2A server configured")
	ErrGatewayNotReady                     = errors.New("g8e Gateway not ready")
	ErrGatewayL3ProofRequired              = errors.New("L3 proof required")
	ErrGatewayUserIDRequired               = errors.New("user_id required")
	ErrGatewayInvalidPosture               = errors.New("invalid posture")
	ErrGatewayForbiddenPattern             = errors.New("forbidden pattern detected")
	ErrGatewayDownstreamHTTPError          = errors.New("downstream server returned HTTP error")
	ErrGatewayMCPError                     = errors.New("MCP error")
	ErrGatewayA2AError                     = errors.New("A2A error")

	// MCP native handler errors
	ErrMCPNativeToolRegistration = errors.New("native tool registration failed")
	ErrMCPNativeToolUnknown      = errors.New("unknown native tool")
	ErrMCPParseSocketPort        = errors.New("parse socket port")
	ErrMCPParseSocketIPOctet     = errors.New("parse socket IP octet")
)
