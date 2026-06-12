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
