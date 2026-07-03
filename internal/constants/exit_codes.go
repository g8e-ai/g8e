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

// Exit codes for the g8e Operator
// These enable the g8e script to provide accurate error messages
const (
	// ExitSuccess indicates the Operator exited normally
	ExitSuccess = 0

	// ExitGeneralError is the default error code for unspecified errors
	ExitGeneralError = 1

	// ExitAuthFailure indicates authentication with g8e failed
	// (deleted Operator slot, invalid credentials)
	ExitAuthFailure = 2

	// ExitPermissionDenied indicates a filesystem permission error
	// (cannot create directories, cannot write files)
	ExitPermissionDenied = 3

	// ExitNetworkError indicates a network connectivity issue
	// (cannot reach g8e servers, DNS failure, timeout)
	ExitNetworkError = 4

	// ExitConfigError indicates a configuration error
	// (missing required config, invalid config values)
	ExitConfigError = 5

	// ExitStorageError indicates a storage/database initialization error
	// (SQLite init failed, git init failed, disk full)
	ExitStorageError = 6

	// ExitCertTrustFailure indicates the Operator cannot verify the server's TLS certificate.
	// This is a non-retryable condition caused by a stale embedded CA certificate.
	// The Operator must self-terminate to prevent noisy retry loops against the server.
	// Resolution: download a new Node binary with updated certificates.
	ExitCertTrustFailure = 7
)

// Unix shell exit codes for command execution
// These follow standard Unix conventions (see sysexits.h)
const (
	// ExitCodeSuccess indicates successful execution
	ExitCodeSuccess = 0

	// ExitCodeGeneral is the default error code for unspecified errors
	ExitCodeGeneral = 1

	// ExitCodeUsage indicates command line usage error (typically 2)
	ExitCodeUsage = 2

	// ExitCodeTimeout indicates command timed out (typically 124)
	ExitCodeTimeout = 124

	// ExitCodeCannotExecute indicates command invoked cannot execute (typically 126)
	ExitCodeCannotExecute = 126

	// ExitCodeCommandNotFound indicates command not found (typically 127)
	ExitCodeCommandNotFound = 127

	// ExitCodeKilled indicates process was killed (typically 137, 128+9)
	ExitCodeKilled = 137

	// ExitCodeNone indicates no exit code is available (e.g. non-command event)
	ExitCodeNone = -1
)

// Windows process exit codes
const (
	// StillActiveExitCode indicates a Windows process is still running.
	// Equivalent to the Windows STILL_ACTIVE macro (259).
	StillActiveExitCode = 259
)
