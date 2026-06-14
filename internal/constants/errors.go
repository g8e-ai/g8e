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
)
