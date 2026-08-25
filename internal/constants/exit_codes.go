// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

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

	// ExitEnrollmentPending indicates the Operator has credentials but the
	// gateway has not approved its platform enrollment posture. The Operator
	// cannot start until the owner approves the pending enrollment request.
	// Resolution: run 'g8e auth pending-platform-enrollments' and
	// 'g8e auth approve-platform-enrollment <request-id> --yes' on the host,
	// then restart the Operator. Container orchestrators should treat this
	// as a non-restartable exit code (or back off) to avoid crash-loops.
	ExitEnrollmentPending = 8
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
