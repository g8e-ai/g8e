// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package constants

const (
	PlatformUsageUpdated = "g8e.v1.platform.usage.updated"
	PlatformNotification = "g8e.v1.platform.notification"
)

const (
	PlatformAuthLoginRequested                = "g8e.v1.platform.auth.login.requested"
	PlatformAuthLoginSucceeded                = "g8e.v1.platform.auth.login.succeeded"
	PlatformAuthLoginFailed                   = "g8e.v1.platform.auth.login.failed"
	PlatformAuthLogoutRequested               = "g8e.v1.platform.auth.logout.requested"
	PlatformAuthLogoutSucceeded               = "g8e.v1.platform.auth.logout.succeeded"
	PlatformAuthLogoutFailed                  = "g8e.v1.platform.auth.logout.failed"
	PlatformAuthSessionValidationRequested    = "g8e.v1.platform.auth.session.validation.requested"
	PlatformAuthSessionValidationSucceeded    = "g8e.v1.platform.auth.session.validation.succeeded"
	PlatformAuthSessionValidationFailed       = "g8e.v1.platform.auth.session.validation.failed"
	PlatformAuthSessionExpired                = "g8e.v1.platform.auth.session.expired"
	PlatformAuthUserAuthenticated             = "g8e.v1.platform.auth.user.authenticated"
	PlatformAuthUserUnauthenticated           = "g8e.v1.platform.auth.user.unauthenticated"
	PlatformAuthComponentInitializedAuthstate = "g8e.v1.platform.auth.component.initialized.authstate"
	PlatformAuthComponentInitializedChat      = "g8e.v1.platform.auth.component.initialized.chat"
	PlatformAuthComponentInitializedOperator  = "g8e.v1.platform.auth.component.initialized.operator"
	PlatformAuthInfo                          = "g8e.v1.platform.auth.info"
)

const (
	PlatformSseKeepaliveSent         = "g8e.v1.platform.sse.keepalive.sent"
	PlatformSseConnectionEstablished = "g8e.v1.platform.sse.connection.established"
	PlatformSseConnectionOpened      = "g8e.v1.platform.sse.connection.opened"
	PlatformSseConnectionClosed      = "g8e.v1.platform.sse.connection.closed"
	PlatformSseConnectionFailed      = "g8e.v1.platform.sse.connection.failed"
	PlatformSseConnectionError       = "g8e.v1.platform.sse.connection.error"
)

const (
	PlatformTerminalOpened    = "g8e.v1.platform.terminal.opened"
	PlatformTerminalMinimized = "g8e.v1.platform.terminal.minimized"
	PlatformTerminalMaximized = "g8e.v1.platform.terminal.maximized"
	PlatformTerminalClosed    = "g8e.v1.platform.terminal.closed"
)

const (
	PlatformVaultModeChanged          = "g8e.v1.platform.sentinel.mode.changed"
	PlatformExternalServiceConfigured = "g8e.v1.platform.external.service.configured"
)

const (
	PlatformTelemetryHealthReported      = "g8e.v1.platform.telemetry.health.reported"
	PlatformTelemetryPerformanceRecorded = "g8e.v1.platform.telemetry.performance.recorded"
	PlatformTelemetryErrorLogged         = "g8e.v1.platform.telemetry.error.logged"
	PlatformTelemetryAuditLogged         = "g8e.v1.platform.telemetry.audit.logged"
)

const (
	PlatformConsoleLogEntryReceived      = "g8e.v1.platform.console.log.entry.received"
	PlatformConsoleLogConnectedConfirmed = "g8e.v1.platform.console.log.connected.confirmed"
)

// Platform binary names.
const (
	BinaryNameWindows = "g8e-windows-amd64.exe"
	BinaryNameLinux   = "g8e-linux-amd64"
	BinaryNameDarwin  = "g8e-darwin-amd64"
)

// Supported architectures.
const (
	ArchAMD64 = "amd64"
	ArchARM64 = "arm64"
	Arch386   = "386"
)

// Supported operating systems.
const (
	OSLinux   = "linux"
	OSDarwin  = "darwin"
	OSWindows = "windows"
)

// Governance posture names.
const (
	PostureDoctrine  = "doctrine"
	PostureConsensus = "consensus"
	PostureNotary    = "notary"
)

// Log level names.
const (
	LogLevelInfo    = "info"
	LogLevelError   = "error"
	LogLevelDebug   = "debug"
	LogLevelDefault = LogLevelInfo
)
