# Copyright (c) 2026 Lateralus Labs, LLC.
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

from enum import Enum

class EventType(str, Enum):
    # app.case
    CASE_CREATED = "g8e.v1.app.case.created"
    CASE_UPDATED = "g8e.v1.app.case.updated"
    CASE_ASSIGNED = "g8e.v1.app.case.assigned"
    CASE_ESCALATED = "g8e.v1.app.case.escalated"
    CASE_RESOLVED = "g8e.v1.app.case.resolved"
    CASE_CLOSED = "g8e.v1.app.case.closed"
    CASE_SELECTED = "g8e.v1.app.case.selected"
    CASE_CLEARED = "g8e.v1.app.case.cleared"
    CASE_SWITCHED = "g8e.v1.app.case.switched"
    CASE_CREATION_REQUESTED = "g8e.v1.app.case.creation.requested"
    CASE_UPDATE_REQUESTED = "g8e.v1.app.case.update.requested"

    # ai.llm.config
    LLM_CONFIG_REQUESTED = "g8e.v1.ai.llm.config.requested"
    LLM_CONFIG_RECEIVED = "g8e.v1.ai.llm.config.received"
    LLM_CONFIG_FAILED = "g8e.v1.ai.llm.config.failed"

    # ai.llm.chat
    LLM_CHAT_SUBMITTED = "g8e.v1.ai.llm.chat.submitted"
    LLM_CHAT_STOP_SHOW = "g8e.v1.ai.llm.chat.stop.show"
    LLM_CHAT_STOP_HIDE = "g8e.v1.ai.llm.chat.stop.hide"
    LLM_CHAT_FILTER_EVENT = "g8e.v1.ai.llm.chat.filter.event"

    LLM_CHAT_MESSAGE_SENT = "g8e.v1.ai.llm.chat.message.sent"
    LLM_CHAT_MESSAGE_REPLAYED = "g8e.v1.ai.llm.chat.message.replayed"
    LLM_CHAT_MESSAGE_PROCESSING_FAILED = "g8e.v1.ai.llm.chat.message.processing.failed"
    LLM_CHAT_MESSAGE_DEAD_LETTERED = "g8e.v1.ai.llm.chat.message.dead.lettered"

    LLM_CHAT_ITERATION_STARTED = "g8e.v1.ai.llm.chat.iteration.started"
    LLM_CHAT_ITERATION_COMPLETED = "g8e.v1.ai.llm.chat.iteration.completed"
    LLM_CHAT_ITERATION_FAILED = "g8e.v1.ai.llm.chat.iteration.failed"
    LLM_CHAT_ITERATION_STOPPED = "g8e.v1.ai.llm.chat.iteration.stopped"
    LLM_CHAT_ITERATION_THINKING_STARTED = "g8e.v1.ai.llm.chat.iteration.thinking.started"
    LLM_CHAT_ITERATION_CITATIONS_RECEIVED = "g8e.v1.ai.llm.chat.iteration.citations.received"

    LLM_CHAT_ITERATION_TEXT_RECEIVED = "g8e.v1.ai.llm.chat.iteration.text.received"
    LLM_CHAT_ITERATION_TEXT_CHUNK_RECEIVED = "g8e.v1.ai.llm.chat.iteration.text.chunk.received"
    LLM_CHAT_ITERATION_TEXT_COMPLETED = "g8e.v1.ai.llm.chat.iteration.text.completed"
    LLM_CHAT_ITERATION_TEXT_TRUNCATED = "g8e.v1.ai.llm.chat.iteration.text.truncated"
    LLM_CHAT_ITERATION_RETRY = "g8e.v1.ai.llm.chat.iteration.retry"
    LLM_CHAT_ITERATION_STREAM_STARTED = "g8e.v1.ai.llm.chat.iteration.stream.started"
    LLM_CHAT_ITERATION_STREAM_DELTA_RECEIVED = "g8e.v1.ai.llm.chat.iteration.stream.delta.received"
    LLM_CHAT_ITERATION_STREAM_COMPLETED = "g8e.v1.ai.llm.chat.iteration.stream.completed"
    LLM_CHAT_ITERATION_STREAM_FAILED = "g8e.v1.ai.llm.chat.iteration.stream.failed"

    # ai.llm.lifecycle
    LLM_LIFECYCLE_REQUESTED = "g8e.v1.ai.llm.lifecycle.requested"
    LLM_LIFECYCLE_STARTED = "g8e.v1.ai.llm.lifecycle.started"
    LLM_LIFECYCLE_COMPLETED = "g8e.v1.ai.llm.lifecycle.completed"
    LLM_LIFECYCLE_FAILED = "g8e.v1.ai.llm.lifecycle.failed"
    LLM_LIFECYCLE_STOPPED = "g8e.v1.ai.llm.lifecycle.stopped"
    LLM_LIFECYCLE_ERROR_OCCURRED = "g8e.v1.ai.llm.lifecycle.error.occurred"

    # app.task
    TASK_CREATED = "g8e.v1.app.task.created"
    TASK_UPDATED = "g8e.v1.app.task.updated"
    TASK_ASSIGNED = "g8e.v1.app.task.assigned"
    TASK_STARTED = "g8e.v1.app.task.started"
    TASK_COMPLETED = "g8e.v1.app.task.completed"
    TASK_FAILED = "g8e.v1.app.task.failed"

    # app.investigation
    INVESTIGATION_CREATED = "g8e.v1.app.investigation.created"
    INVESTIGATION_UPDATED = "g8e.v1.app.investigation.updated"
    INVESTIGATION_LOADED = "g8e.v1.app.investigation.loaded"
    INVESTIGATION_REQUESTED = "g8e.v1.app.investigation.requested"
    INVESTIGATION_STARTED = "g8e.v1.app.investigation.started"
    INVESTIGATION_CLOSED = "g8e.v1.app.investigation.closed"
    INVESTIGATION_ESCALATED = "g8e.v1.app.investigation.escalated"

    INVESTIGATION_LIST_REQUESTED = "g8e.v1.app.investigation.list.requested"
    INVESTIGATION_LIST_RECEIVED = "g8e.v1.app.investigation.list.received"
    INVESTIGATION_LIST_COMPLETED = "g8e.v1.app.investigation.list.completed"
    INVESTIGATION_LIST_FAILED = "g8e.v1.app.investigation.list.failed"

    INVESTIGATION_STATUS_UPDATED_OPEN = "g8e.v1.app.investigation.status.updated.open"
    INVESTIGATION_STATUS_UPDATED_CLOSED = "g8e.v1.app.investigation.status.updated.closed"
    INVESTIGATION_STATUS_UPDATED_ESCALATED = "g8e.v1.app.investigation.status.updated.escalated"
    INVESTIGATION_STATUS_UPDATED_RESOLVED = "g8e.v1.app.investigation.status.updated.resolved"

    INVESTIGATION_CHAT_MESSAGE_USER = "g8e.v1.app.investigation.chat.message.user"
    INVESTIGATION_CHAT_MESSAGE_AI = "g8e.v1.app.investigation.chat.message.ai"
    INVESTIGATION_CHAT_MESSAGE_SYSTEM = "g8e.v1.app.investigation.chat.message.system"

    # g8e.heartbeat
    OPERATOR_HEARTBEAT_SENT = "g8e.v1.operator.heartbeat.sent"
    OPERATOR_HEARTBEAT_REQUESTED = "g8e.v1.operator.heartbeat.requested"
    OPERATOR_HEARTBEAT_RECEIVED = "g8e.v1.operator.heartbeat.received"
    OPERATOR_HEARTBEAT_MISSED = "g8e.v1.operator.heartbeat.missed"

    # g8e.shutdown
    OPERATOR_SHUTDOWN_REQUESTED = "g8e.v1.operator.shutdown.requested"
    OPERATOR_SHUTDOWN_ACKNOWLEDGED = "g8e.v1.operator.shutdown.acknowledged"

    # g8e.panel
    OPERATOR_PANEL_LIST_UPDATED = "g8e.v1.operator.panel.list.updated"

    # g8e.status
    OPERATOR_STATUS_UPDATED_ACTIVE = "g8e.v1.operator.status.updated.active"
    OPERATOR_STATUS_UPDATED_AVAILABLE = "g8e.v1.operator.status.updated.available"
    OPERATOR_STATUS_UPDATED_UNAVAILABLE = "g8e.v1.operator.status.updated.unavailable"
    OPERATOR_STATUS_UPDATED_BOUND = "g8e.v1.operator.status.updated.bound"
    OPERATOR_STATUS_UPDATED_OFFLINE = "g8e.v1.operator.status.updated.offline"
    OPERATOR_STATUS_UPDATED_STALE = "g8e.v1.operator.status.updated.stale"
    OPERATOR_STATUS_UPDATED_STOPPED = "g8e.v1.operator.status.updated.stopped"
    OPERATOR_STATUS_UPDATED_TERMINATED = "g8e.v1.operator.status.updated.terminated"

    # g8e.api
    OPERATOR_API_KEY_REFRESHED = "g8e.v1.operator.api.key.refreshed"

    # g8e.device
    OPERATOR_DEVICE_REGISTERED = "g8e.v1.operator.device.registered"

    # operator.command
    OPERATOR_COMMAND_REQUESTED = "g8e.v1.operator.command.requested"
    OPERATOR_COMMAND_STARTED = "g8e.v1.operator.command.started"
    OPERATOR_COMMAND_COMPLETED = "g8e.v1.operator.command.completed"
    OPERATOR_COMMAND_FAILED = "g8e.v1.operator.command.failed"
    OPERATOR_COMMAND_CANCELLED = "g8e.v1.operator.command.cancelled"
    OPERATOR_COMMAND_EXECUTION = "g8e.v1.operator.command.execution"
    OPERATOR_COMMAND_RESULT = "g8e.v1.operator.command.result"
    OPERATOR_COMMAND_OUTPUT_RECEIVED = "g8e.v1.operator.command.output.received"

    OPERATOR_COMMAND_STATUS_UPDATED_QUEUED = "g8e.v1.operator.command.status.updated.queued"
    OPERATOR_COMMAND_STATUS_UPDATED_RUNNING = "g8e.v1.operator.command.status.updated.running"
    OPERATOR_COMMAND_STATUS_UPDATED_COMPLETED = "g8e.v1.operator.command.status.updated.completed"
    OPERATOR_COMMAND_STATUS_UPDATED_FAILED = "g8e.v1.operator.command.status.updated.failed"
    OPERATOR_COMMAND_STATUS_UPDATED_CANCELLED = "g8e.v1.operator.command.status.updated.cancelled"

    OPERATOR_COMMAND_CANCEL_REQUESTED = "g8e.v1.operator.command.cancel.requested"
    OPERATOR_COMMAND_CANCEL_ACKNOWLEDGED = "g8e.v1.operator.command.cancel.acknowledged"
    OPERATOR_COMMAND_CANCEL_FAILED = "g8e.v1.operator.command.cancel.failed"

    OPERATOR_COMMAND_APPROVAL_PREPARING = "g8e.v1.operator.command.approval.preparing"
    OPERATOR_COMMAND_APPROVAL_REQUESTED = "g8e.v1.operator.command.approval.requested"
    OPERATOR_COMMAND_APPROVAL_GRANTED = "g8e.v1.operator.command.approval.granted"
    OPERATOR_COMMAND_APPROVAL_REJECTED = "g8e.v1.operator.command.approval.rejected"

    # operator.file
    OPERATOR_FILE_EDIT_REQUESTED = "g8e.v1.operator.file.edit.requested"
    OPERATOR_FILE_EDIT_STARTED = "g8e.v1.operator.file.edit.started"
    OPERATOR_FILE_EDIT_COMPLETED = "g8e.v1.operator.file.edit.completed"
    OPERATOR_FILE_EDIT_FAILED = "g8e.v1.operator.file.edit.failed"
    OPERATOR_FILE_EDIT_TIMEOUT = "g8e.v1.operator.file.edit.timeout"

    OPERATOR_FILE_EDIT_APPROVAL_REQUESTED = "g8e.v1.operator.file.edit.approval.requested"
    OPERATOR_FILE_EDIT_APPROVAL_GRANTED = "g8e.v1.operator.file.edit.approval.granted"
    OPERATOR_FILE_EDIT_APPROVAL_REJECTED = "g8e.v1.operator.file.edit.approval.rejected"
    OPERATOR_FILE_EDIT_APPROVAL_FEEDBACK = "g8e.v1.operator.file.edit.approval.feedback"

    OPERATOR_FILE_HISTORY_FETCH_STARTED = "g8e.v1.operator.file.history.fetch.started"
    OPERATOR_FILE_HISTORY_FETCH_REQUESTED = "g8e.v1.operator.file.history.fetch.requested"
    OPERATOR_FILE_HISTORY_FETCH_RECEIVED = "g8e.v1.operator.file.history.fetch.received"
    OPERATOR_FILE_HISTORY_FETCH_COMPLETED = "g8e.v1.operator.file.history.fetch.completed"
    OPERATOR_FILE_HISTORY_FETCH_FAILED = "g8e.v1.operator.file.history.fetch.failed"

    OPERATOR_FILE_DIFF_FETCH_STARTED = "g8e.v1.operator.file.diff.fetch.started"
    OPERATOR_FILE_DIFF_FETCH_REQUESTED = "g8e.v1.operator.file.diff.fetch.requested"
    OPERATOR_FILE_DIFF_FETCH_RECEIVED = "g8e.v1.operator.file.diff.fetch.received"
    OPERATOR_FILE_DIFF_FETCH_COMPLETED = "g8e.v1.operator.file.diff.fetch.completed"
    OPERATOR_FILE_DIFF_FETCH_FAILED = "g8e.v1.operator.file.diff.fetch.failed"

    OPERATOR_FILE_RESTORE_REQUESTED = "g8e.v1.operator.file.restore.requested"
    OPERATOR_FILE_RESTORE_RECEIVED = "g8e.v1.operator.file.restore.received"
    OPERATOR_FILE_RESTORE_COMPLETED = "g8e.v1.operator.file.restore.completed"
    OPERATOR_FILE_RESTORE_FAILED = "g8e.v1.operator.file.restore.failed"

    # g8e.filesystem
    OPERATOR_FILESYSTEM_LIST_STARTED = "g8e.v1.operator.filesystem.list.started"
    OPERATOR_FILESYSTEM_LIST_REQUESTED = "g8e.v1.operator.filesystem.list.requested"
    OPERATOR_FILESYSTEM_LIST_RECEIVED = "g8e.v1.operator.filesystem.list.received"
    OPERATOR_FILESYSTEM_LIST_COMPLETED = "g8e.v1.operator.filesystem.list.completed"
    OPERATOR_FILESYSTEM_LIST_FAILED = "g8e.v1.operator.filesystem.list.failed"

    OPERATOR_FILESYSTEM_READ_STARTED = "g8e.v1.operator.filesystem.read.started"
    OPERATOR_FILESYSTEM_READ_REQUESTED = "g8e.v1.operator.filesystem.read.requested"
    OPERATOR_FILESYSTEM_READ_RECEIVED = "g8e.v1.operator.filesystem.read.received"
    OPERATOR_FILESYSTEM_READ_COMPLETED = "g8e.v1.operator.filesystem.read.completed"
    OPERATOR_FILESYSTEM_READ_FAILED = "g8e.v1.operator.filesystem.read.failed"

    # g8e.logs
    OPERATOR_LOGS_FETCH_REQUESTED = "g8e.v1.operator.logs.fetch.requested"
    OPERATOR_LOGS_FETCH_RECEIVED = "g8e.v1.operator.logs.fetch.received"
    OPERATOR_LOGS_FETCH_COMPLETED = "g8e.v1.operator.logs.fetch.completed"
    OPERATOR_LOGS_FETCH_FAILED = "g8e.v1.operator.logs.fetch.failed"

    # g8e.history
    OPERATOR_HISTORY_FETCH_REQUESTED = "g8e.v1.operator.history.fetch.requested"
    OPERATOR_HISTORY_FETCH_RECEIVED = "g8e.v1.operator.history.fetch.received"
    OPERATOR_HISTORY_FETCH_COMPLETED = "g8e.v1.operator.history.fetch.completed"
    OPERATOR_HISTORY_FETCH_FAILED = "g8e.v1.operator.history.fetch.failed"

    # g8e.intent
    OPERATOR_INTENT_GRANTED = "g8e.v1.operator.intent.granted"
    OPERATOR_INTENT_DENIED = "g8e.v1.operator.intent.denied"
    OPERATOR_INTENT_REVOKED = "g8e.v1.operator.intent.revoked"

    OPERATOR_INTENT_APPROVAL_REQUESTED = "g8e.v1.operator.intent.approval.requested"
    OPERATOR_INTENT_APPROVAL_GRANTED = "g8e.v1.operator.intent.approval.granted"
    OPERATOR_INTENT_APPROVAL_REJECTED = "g8e.v1.operator.intent.approval.rejected"

    # g8e.network
    OPERATOR_NETWORK_PING_REQUESTED = "g8e.v1.operator.network.ping.requested"
    OPERATOR_NETWORK_PING_RECEIVED = "g8e.v1.operator.network.ping.received"
    OPERATOR_NETWORK_PING_COMPLETED = "g8e.v1.operator.network.ping.completed"
    OPERATOR_NETWORK_PING_FAILED = "g8e.v1.operator.network.ping.failed"

    OPERATOR_NETWORK_PORT_CHECK_REQUESTED = "g8e.v1.operator.network.port.check.requested"
    OPERATOR_NETWORK_PORT_CHECK_RECEIVED = "g8e.v1.operator.network.port.check.received"
    OPERATOR_NETWORK_PORT_CHECK_STARTED = "g8e.v1.operator.network.port.check.started"
    OPERATOR_NETWORK_PORT_CHECK_COMPLETED = "g8e.v1.operator.network.port.check.completed"
    OPERATOR_NETWORK_PORT_CHECK_FAILED = "g8e.v1.operator.network.port.check.failed"

    # g8e.audit
    OPERATOR_AUDIT_USER_RECORDED = "g8e.v1.operator.audit.user.recorded"
    OPERATOR_AUDIT_AI_RECORDED = "g8e.v1.operator.audit.ai.recorded"
    OPERATOR_AUDIT_COMMAND_RECORDED = "g8e.v1.operator.audit.command.recorded"
    OPERATOR_AUDIT_DIRECT_COMMAND_RECORDED = "g8e.v1.operator.audit.direct.command.recorded"
    OPERATOR_AUDIT_DIRECT_COMMAND_RESULT_RECORDED = "g8e.v1.operator.audit.direct.command.result.recorded"

    # g8e.bootstrap
    OPERATOR_BOOTSTRAP_REQUESTED = "g8e.v1.operator.bootstrap.requested"
    OPERATOR_BOOTSTRAP_RECEIVED = "g8e.v1.operator.bootstrap.received"
    OPERATOR_BOOTSTRAP_COMPLETED = "g8e.v1.operator.bootstrap.completed"
    OPERATOR_BOOTSTRAP_FAILED = "g8e.v1.operator.bootstrap.failed"
    OPERATOR_BOOTSTRAP_CONFIG_RECEIVED = "g8e.v1.operator.bootstrap.config.received"

    # operator (misc)
    OPERATOR_BOUND = "g8e.v1.operator.bound"
    OPERATOR_UNBOUND = "g8e.v1.operator.unbound"
    OPERATOR_TERMINAL_THINKING_APPEND = "g8e.v1.operator.terminal.thinking.append"
    OPERATOR_TERMINAL_THINKING_COMPLETE = "g8e.v1.operator.terminal.thinking.complete"
    OPERATOR_TERMINAL_APPROVAL_DENIED = "g8e.v1.operator.terminal.approval.denied"
    OPERATOR_TERMINAL_AUTH_STATE_CHANGED = "g8e.v1.operator.terminal.auth.state.changed"

    # ai.agent
    AI_AGENT_CONTINUE_APPROVAL_REQUESTED = "g8e.v1.ai.agent.continue.approval.requested"
    AI_AGENT_CONTINUE_APPROVAL_GRANTED = "g8e.v1.ai.agent.continue.approval.granted"
    AI_AGENT_CONTINUE_APPROVAL_REJECTED = "g8e.v1.ai.agent.continue.approval.rejected"

    # ai.tribunal
    # Terminal states are expressed as distinct event types, one per scenario.
    # The event type itself is the discriminator; there is no shared "failure"
    # payload with a reason enum.
    TRIBUNAL_SESSION_STARTED              = "g8e.v1.ai.tribunal.session.started"
    TRIBUNAL_SESSION_COMPLETED            = "g8e.v1.ai.tribunal.session.completed"
    TRIBUNAL_SESSION_DISABLED             = "g8e.v1.ai.tribunal.session.disabled"
    TRIBUNAL_SESSION_MODEL_NOT_CONFIGURED = "g8e.v1.ai.tribunal.session.model.not_configured"
    TRIBUNAL_SESSION_PROVIDER_UNAVAILABLE = "g8e.v1.ai.tribunal.session.provider.unavailable"
    TRIBUNAL_SESSION_SYSTEM_ERROR         = "g8e.v1.ai.tribunal.session.system.error"
    TRIBUNAL_SESSION_GENERATION_FAILED    = "g8e.v1.ai.tribunal.session.generation.failed"
    TRIBUNAL_SESSION_AUDITOR_FAILED       = "g8e.v1.ai.tribunal.session.auditor.failed"

    TRIBUNAL_VOTING_STARTED = "g8e.v1.ai.tribunal.voting.started"
    TRIBUNAL_VOTING_FAILED = "g8e.v1.ai.tribunal.voting.failed"
    TRIBUNAL_VOTING_PASS_COMPLETED = "g8e.v1.ai.tribunal.voting.pass.completed"
    TRIBUNAL_VOTING_PASS_FAILED = "g8e.v1.ai.tribunal.voting.pass.failed"
    TRIBUNAL_VOTING_CONSENSUS_REACHED = "g8e.v1.ai.tribunal.voting.consensus.reached"
    TRIBUNAL_VOTING_CONSENSUS_NOT_REACHED = "g8e.v1.ai.tribunal.voting.consensus.not_reached"
    TRIBUNAL_VOTING_CONSENSUS_FAILED = "g8e.v1.ai.tribunal.voting.consensus.failed"
    TRIBUNAL_VOTING_DISSENT_RECORDED = "g8e.v1.ai.tribunal.voting.dissent.recorded"
    TRIBUNAL_VOTING_AUDIT_STARTED = "g8e.v1.ai.tribunal.voting.audit.started"
    TRIBUNAL_VOTING_AUDIT_COMPLETED = "g8e.v1.ai.tribunal.voting.audit.completed"

    # platform
    PLATFORM_USAGE_UPDATED = "g8e.v1.platform.usage.updated"
    PLATFORM_NOTIFICATION = "g8e.v1.platform.notification"

    AUTH_LOGIN_REQUESTED = "g8e.v1.platform.auth.login.requested"
    AUTH_LOGIN_SUCCEEDED = "g8e.v1.platform.auth.login.succeeded"
    AUTH_LOGIN_FAILED = "g8e.v1.platform.auth.login.failed"

    AUTH_LOGOUT_REQUESTED = "g8e.v1.platform.auth.logout.requested"
    AUTH_LOGOUT_SUCCEEDED = "g8e.v1.platform.auth.logout.succeeded"
    AUTH_LOGOUT_FAILED = "g8e.v1.platform.auth.logout.failed"

    AUTH_SESSION_VALIDATION_REQUESTED = "g8e.v1.platform.auth.session.validation.requested"
    AUTH_SESSION_VALIDATION_SUCCEEDED = "g8e.v1.platform.auth.session.validation.succeeded"
    AUTH_SESSION_VALIDATION_FAILED = "g8e.v1.platform.auth.session.validation.failed"
    AUTH_SESSION_EXPIRED = "g8e.v1.platform.auth.session.expired"

    AUTH_USER_AUTHENTICATED = "g8e.v1.platform.auth.user.authenticated"
    AUTH_USER_UNAUTHENTICATED = "g8e.v1.platform.auth.user.unauthenticated"

    AUTH_COMPONENT_INITIALIZED_AUTHSTATE = "g8e.v1.platform.auth.component.initialized.authstate"
    AUTH_COMPONENT_INITIALIZED_CHAT = "g8e.v1.platform.auth.component.initialized.chat"
    AUTH_COMPONENT_INITIALIZED_OPERATOR = "g8e.v1.platform.auth.component.initialized.operator"
    AUTH_INFO = "g8e.v1.platform.auth.info"

    PLATFORM_SSE_KEEPALIVE_SENT = "g8e.v1.platform.sse.keepalive.sent"
    PLATFORM_SSE_CONNECTION_ESTABLISHED = "g8e.v1.platform.sse.connection.established"
    PLATFORM_SSE_CONNECTION_OPENED = "g8e.v1.platform.sse.connection.opened"
    PLATFORM_SSE_CONNECTION_CLOSED = "g8e.v1.platform.sse.connection.closed"
    PLATFORM_SSE_CONNECTION_FAILED = "g8e.v1.platform.sse.connection.failed"
    PLATFORM_SSE_CONNECTION_ERROR = "g8e.v1.platform.sse.connection.error"

    PLATFORM_TERMINAL_OPENED = "g8e.v1.platform.terminal.opened"
    PLATFORM_TERMINAL_MINIMIZED = "g8e.v1.platform.terminal.minimized"
    PLATFORM_TERMINAL_MAXIMIZED = "g8e.v1.platform.terminal.maximized"
    PLATFORM_TERMINAL_CLOSED = "g8e.v1.platform.terminal.closed"

    PLATFORM_SENTINEL_MODE_CHANGED = "g8e.v1.platform.sentinel.mode.changed"
    PLATFORM_EXTERNAL_SERVICE_CONFIGURED = "g8e.v1.platform.external.service.configured"

    PLATFORM_TELEMETRY_HEALTH_REPORTED = "g8e.v1.platform.telemetry.health.reported"
    PLATFORM_TELEMETRY_PERFORMANCE_RECORDED = "g8e.v1.platform.telemetry.performance.recorded"
    PLATFORM_TELEMETRY_ERROR_LOGGED = "g8e.v1.platform.telemetry.error.logged"
    PLATFORM_TELEMETRY_AUDIT_LOGGED = "g8e.v1.platform.telemetry.audit.logged"

    PLATFORM_CONSOLE_LOG_ENTRY_RECEIVED = "g8e.v1.platform.console.log.entry.received"
    PLATFORM_CONSOLE_LOG_CONNECTED_CONFIRMED = "g8e.v1.platform.console.log.connected.confirmed"

    EVENT_SOURCE_AI_PRIMARY = "g8e.v1.source.ai.primary"
    EVENT_SOURCE_AI_ASSISTANT = "g8e.v1.source.ai.assistant"
    EVENT_SOURCE_USER_CHAT = "g8e.v1.source.user.chat"
    EVENT_SOURCE_USER_TERMINAL = "g8e.v1.source.user.terminal"
    EVENT_SOURCE_SYSTEM = "g8e.v1.source.system"

    LLM_TOOL_G8E_WEB_SEARCH_REQUESTED = "g8e.v1.ai.llm.tool.g8e.web.search.requested"
    LLM_TOOL_G8E_WEB_SEARCH_RECEIVED = "g8e.v1.ai.llm.tool.g8e.web.search.received"
    LLM_TOOL_G8E_WEB_SEARCH_COMPLETED = "g8e.v1.ai.llm.tool.g8e.web.search.completed"
    LLM_TOOL_G8E_WEB_SEARCH_FAILED = "g8e.v1.ai.llm.tool.g8e.web.search.failed"

    LLM_TOOL_G8E_INVESTIGATION_QUERY_REQUESTED = "g8e.v1.ai.llm.tool.g8e.investigation.query.requested"
    LLM_TOOL_G8E_INVESTIGATION_QUERY_RECEIVED = "g8e.v1.ai.llm.tool.g8e.investigation.query.received"
    LLM_TOOL_G8E_INVESTIGATION_QUERY_COMPLETED = "g8e.v1.ai.llm.tool.g8e.investigation.query.completed"
    LLM_TOOL_G8E_INVESTIGATION_QUERY_FAILED = "g8e.v1.ai.llm.tool.g8e.investigation.query.failed"

    LLM_TOOL_G8E_COMMAND_CONSTRAINTS_REQUESTED = "g8e.v1.ai.llm.tool.g8e.command.constraints.requested"
    LLM_TOOL_G8E_COMMAND_CONSTRAINTS_RECEIVED = "g8e.v1.ai.llm.tool.g8e.command.constraints.received"
    LLM_TOOL_G8E_COMMAND_CONSTRAINTS_COMPLETED = "g8e.v1.ai.llm.tool.g8e.command.constraints.completed"
    LLM_TOOL_G8E_COMMAND_CONSTRAINTS_FAILED = "g8e.v1.ai.llm.tool.g8e.command.constraints.failed"
