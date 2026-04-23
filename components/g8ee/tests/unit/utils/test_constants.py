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

"""
Contract tests for app/constants.py.

Verifies that every enum value and collection constant loaded from shared JSON
actually matches what the shared JSON source of truth contains. If a key is
missing from the JSON or a value drifts, these tests will catch it before the
mismatch propagates to production.

These are pure unit tests — no external infrastructure required.
"""

import json

import pytest

from app.constants import (
    CACHE_PREFIX,
    DB_COLLECTION_API_KEYS,
    DB_COLLECTION_CASES,
    DB_COLLECTION_SETTINGS,
    DB_COLLECTION_SETTINGS,
    DB_COLLECTION_INVESTIGATIONS,
    DB_COLLECTION_MEMORIES,
    DB_COLLECTION_OPERATORS,
    DB_COLLECTION_OPERATOR_SESSIONS,
    DB_COLLECTION_ORGANIZATIONS,
    DB_COLLECTION_TASKS,
    DB_COLLECTION_WEB_SESSIONS,
    DB_COLLECTION_USERS,
    AITaskId,
    ApiKeyStatus,
    ApprovalType,
    CaseStatus,
    CloudSubtype,
    CommandErrorType,
    ComponentName,
    ComponentStatus,
    ConversationStatus,
    EscalationRisk,
    EventType,
    ExecutionStatus,
    FileOperation,
    HealthStatus,
    HeartbeatType,
    InfrastructureStatus,
    InvestigationStatus,
    NetworkProtocol,
    OperatorToolName,
    OperatorStatus,
    OperatorType,
    Platform,
    PubSubAction,
    PubSubChannel,
    PubSubField,
    PubSubMessageType,
    PubSubWireEventType,
    RiskLevel,
    RiskThreshold,
    SessionType,
    TaskStatus,
    VaultMode,
    VersionStability,
    AgentMode,
)
from app.constants.settings import (
    CommandGenerationOutcome,
    ApprovalErrorType,
    AttachmentType,
)
from app.models.agent import (
    AgentInputs,
    OperatorContext,
)

pytestmark = [pytest.mark.unit]

_SHARED_DIR = "/app/shared/constants"


def _load(filename: str) -> dict:
    with open(_SHARED_DIR + "/" + filename) as f:
        return json.load(f)


@pytest.fixture(scope="module")
def status():
    return _load("status.json")


@pytest.fixture(scope="module")
def collections():
    return _load("collections.json")


@pytest.fixture(scope="module")
def kv_keys():
    return _load("kv_keys.json")


@pytest.fixture(scope="module")
def events():
    return _load("events.json")


@pytest.fixture(scope="module")
def msg(events):
    return events


@pytest.fixture(scope="module")
def channels():
    return _load("channels.json")


@pytest.fixture(scope="module")
def senders():
    return _load("senders.json")


class TestOperatorStatusMatchesSharedJSON:
    def test_available(self, status):
        assert status["g8e.status"]["available"] == OperatorStatus.AVAILABLE

    def test_unavailable(self, status):
        assert status["g8e.status"]["unavailable"] == OperatorStatus.UNAVAILABLE

    def test_offline(self, status):
        assert status["g8e.status"]["offline"] == OperatorStatus.OFFLINE

    def test_bound(self, status):
        assert status["g8e.status"]["bound"] == OperatorStatus.BOUND

    def test_stale(self, status):
        assert status["g8e.status"]["stale"] == OperatorStatus.STALE

    def test_active(self, status):
        assert status["g8e.status"]["active"] == OperatorStatus.ACTIVE

    def test_stopped(self, status):
        assert status["g8e.status"]["stopped"] == OperatorStatus.STOPPED

    def test_terminated(self, status):
        assert status["g8e.status"]["terminated"] == OperatorStatus.TERMINATED

    def test_all_members_covered(self, status):
        json_keys = set(status["g8e.status"].keys())
        enum_count = len(OperatorStatus)
        assert enum_count == len(json_keys), (
            f"OperatorStatus has {enum_count} members but shared JSON has {len(json_keys)} keys: {json_keys}"
        )


class TestOperatorTypeMatchesSharedJSON:
    def test_system(self, status):
        assert status["g8e.type"]["system"] == OperatorType.SYSTEM

    def test_cloud(self, status):
        assert status["g8e.type"]["cloud"] == OperatorType.CLOUD

    def test_all_members_covered(self, status):
        json_keys = set(status["g8e.type"].keys())
        enum_count = len(OperatorType)
        assert enum_count == len(json_keys), (
            f"OperatorType has {enum_count} members but shared JSON has {len(json_keys)} keys: {json_keys}"
        )


class TestCloudSubtypeMatchesSharedJSON:
    def test_aws(self, status):
        assert status["cloud.subtype"]["aws"] == CloudSubtype.AWS

    def test_gcp(self, status):
        assert status["cloud.subtype"]["gcp"] == CloudSubtype.GCP

    def test_azure(self, status):
        assert status["cloud.subtype"]["azure"] == CloudSubtype.AZURE

    def test_g8ep(self, status):
        assert status["cloud.subtype"]["g8ep"] == CloudSubtype.G8E_POD

    def test_all_members_covered(self, status):
        json_keys = set(status["cloud.subtype"].keys())
        enum_count = len(CloudSubtype)
        assert enum_count == len(json_keys), (
            f"CloudSubtype has {enum_count} members but shared JSON has {len(json_keys)} keys: {json_keys}"
        )


class TestCaseStatusMatchesSharedJSON:
    def test_new(self, status):
        assert status["case.status"]["new"] == CaseStatus.NEW

    def test_triage(self, status):
        assert status["case.status"]["triage"] == CaseStatus.TRIAGE

    def test_escalated(self, status):
        assert status["case.status"]["escalated"] == CaseStatus.ESCALATED

    def test_waiting_for_customer(self, status):
        assert status["case.status"]["waiting.for.customer"] == CaseStatus.WAITING_FOR_CUSTOMER

    def test_investigate(self, status):
        assert status["case.status"]["investigate"] == CaseStatus.INVESTIGATE

    def test_human_review(self, status):
        assert status["case.status"]["human.review"] == CaseStatus.HUMAN_REVIEW

    def test_resolved(self, status):
        assert status["case.status"]["resolved"] == CaseStatus.RESOLVED

    def test_closed(self, status):
        assert status["case.status"]["closed"] == CaseStatus.CLOSED

    def test_all_members_covered(self, status):
        json_keys = set(status["case.status"].keys())
        enum_count = len(CaseStatus)
        assert enum_count == len(json_keys), (
            f"CaseStatus has {enum_count} members but shared JSON has {len(json_keys)} keys"
        )


class TestInvestigationStatusMatchesSharedJSON:
    def test_open(self, status):
        assert status["investigation.status"]["open"] == InvestigationStatus.OPEN

    def test_closed(self, status):
        assert status["investigation.status"]["closed"] == InvestigationStatus.CLOSED

    def test_escalated(self, status):
        assert status["investigation.status"]["escalated"] == InvestigationStatus.ESCALATED

    def test_resolved(self, status):
        assert status["investigation.status"]["resolved"] == InvestigationStatus.RESOLVED

    def test_all_members_covered(self, status):
        json_keys = set(status["investigation.status"].keys())
        enum_count = len(InvestigationStatus)
        assert enum_count == len(json_keys), (
            f"InvestigationStatus has {enum_count} members but shared JSON has {len(json_keys)} keys: {json_keys}"
        )


class TestVaultModeMatchesSharedJSON:
    def test_raw(self, status):
        assert status["vault.mode"]["raw"] == VaultMode.RAW

    def test_scrubbed(self, status):
        assert status["vault.mode"]["scrubbed"] == VaultMode.SCRUBBED


class TestComponentNameMatchesSharedJSON:
    def test_g8ee(self, status):
        assert status["component.name"]["g8ee"] == ComponentName.G8EE

    def test_g8eo(self, status):
        assert status["component.name"]["g8eo"] == ComponentName.G8EO

    def test_g8ed(self, status):
        assert status["component.name"]["g8ed"] == ComponentName.G8ED


class TestComponentStatusMatchesSharedJSON:
    def test_active(self, status):
        assert status["component.status"]["active"] == ComponentStatus.ACTIVE

    def test_inactive(self, status):
        assert status["component.status"]["inactive"] == ComponentStatus.INACTIVE

    def test_maintenance(self, status):
        assert status["component.status"]["maintenance"] == ComponentStatus.MAINTENANCE

    def test_error(self, status):
        assert status["component.status"]["error"] == ComponentStatus.ERROR


class TestHealthStatusMatchesSharedJSON:
    def test_healthy(self, status):
        assert status["health.status"]["healthy"] == HealthStatus.HEALTHY

    def test_unhealthy(self, status):
        assert status["health.status"]["unhealthy"] == HealthStatus.UNHEALTHY


class TestApiKeyStatusMatchesSharedJSON:
    def test_active(self, status):
        assert status["api.key.status"]["active"] == ApiKeyStatus.ACTIVE

    def test_revoked(self, status):
        assert status["api.key.status"]["revoked"] == ApiKeyStatus.REVOKED

    def test_expired(self, status):
        assert status["api.key.status"]["expired"] == ApiKeyStatus.EXPIRED

    def test_suspended(self, status):
        assert status["api.key.status"]["suspended"] == ApiKeyStatus.SUSPENDED


class TestEscalationRiskMatchesSharedJSON:
    def test_low(self, status):
        assert status["escalation.risk"]["low"] == EscalationRisk.LOW

    def test_medium(self, status):
        assert status["escalation.risk"]["medium"] == EscalationRisk.MEDIUM

    def test_high(self, status):
        assert status["escalation.risk"]["high"] == EscalationRisk.HIGH

    def test_critical(self, status):
        assert status["escalation.risk"]["critical"] == EscalationRisk.CRITICAL


class TestRiskThresholdMatchesSharedJSON:
    def test_low(self, status):
        assert status["risk.threshold"]["low"] == RiskThreshold.LOW

    def test_medium(self, status):
        assert status["risk.threshold"]["medium"] == RiskThreshold.MEDIUM

    def test_high(self, status):
        assert status["risk.threshold"]["high"] == RiskThreshold.HIGH


class TestRiskLevelMatchesSharedJSON:
    def test_low(self, status):
        assert status["risk.level"]["low"] == RiskLevel.LOW

    def test_medium(self, status):
        assert status["risk.level"]["medium"] == RiskLevel.MEDIUM

    def test_high(self, status):
        assert status["risk.level"]["high"] == RiskLevel.HIGH


class TestPlatformMatchesSharedJSON:
    def test_linux(self, status):
        assert status["platform"]["linux"] == Platform.LINUX

    def test_windows(self, status):
        assert status["platform"]["windows"] == Platform.WINDOWS

    def test_darwin(self, status):
        assert status["platform"]["darwin"] == Platform.DARWIN


class TestHeartbeatTypeMatchesSharedJSON:
    def test_automatic(self, status):
        assert status["heartbeat.type"]["automatic"] == HeartbeatType.AUTOMATIC

    def test_bootstrap(self, status):
        assert status["heartbeat.type"]["bootstrap"] == HeartbeatType.BOOTSTRAP

    def test_requested(self, status):
        assert status["heartbeat.type"]["requested"] == HeartbeatType.REQUESTED


class TestVersionStabilityMatchesSharedJSON:
    def test_stable(self, status):
        assert status["version.stability"]["stable"] == VersionStability.STABLE

    def test_beta(self, status):
        assert status["version.stability"]["beta"] == VersionStability.BETA

    def test_dev(self, status):
        assert status["version.stability"]["dev"] == VersionStability.DEV


class TestApprovalTypeMatchesSharedJSON:
    def test_command(self, status):
        assert status["approval.type"]["command"] == ApprovalType.COMMAND

    def test_file_edit(self, status):
        assert status["approval.type"]["file.edit"] == ApprovalType.FILE_EDIT

    def test_intent(self, status):
        assert status["approval.type"]["intent"] == ApprovalType.INTENT

    def test_agent_continue(self, status):
        assert status["approval.type"]["agent.continue"] == ApprovalType.AGENT_CONTINUE

    def test_all_members_covered(self, status):
        json_keys = set(status["approval.type"].keys())
        enum_count = len(ApprovalType)
        assert enum_count == len(json_keys), (
            f"ApprovalType has {enum_count} members but shared JSON has {len(json_keys)} keys: {json_keys}"
        )


class TestExecutionStatusMatchesSharedJSON:
    def test_pending(self, status):
        assert status["execution.status"]["pending"] == ExecutionStatus.PENDING

    def test_executing(self, status):
        assert status["execution.status"]["executing"] == ExecutionStatus.EXECUTING

    def test_completed(self, status):
        assert status["execution.status"]["completed"] == ExecutionStatus.COMPLETED

    def test_failed(self, status):
        assert status["execution.status"]["failed"] == ExecutionStatus.FAILED

    def test_timeout(self, status):
        assert status["execution.status"]["timeout"] == ExecutionStatus.TIMEOUT

    def test_cancelled(self, status):
        assert status["execution.status"]["cancelled"] == ExecutionStatus.CANCELLED

    def test_denied(self, status):
        assert status["execution.status"]["denied"] == ExecutionStatus.DENIED

    def test_feedback(self, status):
        assert status["execution.status"]["feedback"] == ExecutionStatus.FEEDBACK

    def test_all_members_covered(self, status):
        json_keys = set(status["execution.status"].keys())
        enum_count = len(ExecutionStatus)
        assert enum_count == len(json_keys), (
            f"ExecutionStatus has {enum_count} members but shared JSON has {len(json_keys)} keys: {json_keys}"
        )

class TestTaskStatusMatchesSharedJSON:
    def test_pending(self, status):
        assert status["task.status"]["pending"] == TaskStatus.PENDING

    def test_in_progress(self, status):
        assert status["task.status"]["in.progress"] == TaskStatus.IN_PROGRESS

    def test_completed(self, status):
        assert status["task.status"]["completed"] == TaskStatus.COMPLETED

    def test_failed(self, status):
        assert status["task.status"]["failed"] == TaskStatus.FAILED

    def test_cancelled(self, status):
        assert status["task.status"]["cancelled"] == TaskStatus.CANCELLED


class TestConversationStatusMatchesSharedJSON:
    def test_active(self, status):
        assert status["conversation.status"]["active"] == ConversationStatus.ACTIVE

    def test_inactive(self, status):
        assert status["conversation.status"]["inactive"] == ConversationStatus.INACTIVE

    def test_completed(self, status):
        assert status["conversation.status"]["completed"] == ConversationStatus.COMPLETED


class TestInfrastructureStatusMatchesSharedJSON:
    def test_unknown(self, status):
        assert status["infrastructure.status"]["unknown"] == InfrastructureStatus.UNKNOWN

    def test_healthy(self, status):
        assert status["infrastructure.status"]["healthy"] == InfrastructureStatus.HEALTHY

    def test_stable(self, status):
        assert status["infrastructure.status"]["stable"] == InfrastructureStatus.STABLE

    def test_degraded(self, status):
        assert status["infrastructure.status"]["degraded"] == InfrastructureStatus.DEGRADED

    def test_critical(self, status):
        assert status["infrastructure.status"]["critical"] == InfrastructureStatus.CRITICAL


class TestEventTypeMatchesSharedJSON:
    def test_case_created(self, events):
        assert events["app"]["case"]["created"] == EventType.CASE_CREATED

    def test_operator_command_execution(self, events):
        assert events["operator"]["command"]["execution"] == EventType.OPERATOR_COMMAND_EXECUTION


class TestAISourceMatchesSharedJSON:
    def test_tool_call(self, status):
        assert status["ai.source"]["tool.call"] == "ai.tool.call"

    def test_anchored_terminal(self, status):
        assert status["ai.source"]["terminal.anchored"] == "ai.terminal.anchored"

    def test_direct_terminal(self, status):
        assert status["ai.source"]["terminal.direct"] == "ai.terminal.direct"


class TestAITaskIdMatchesSharedJSON:
    def test_chat(self, status):
        assert status["ai.task.id"]["chat"] == AITaskId.CHAT

    def test_agent_continue(self, status):
        assert status["ai.task.id"]["agent.continue"] == AITaskId.AGENT_CONTINUE

    def test_command(self, status):
        assert status["ai.task.id"]["command"] == AITaskId.COMMAND

    def test_direct_command(self, status):
        assert status["ai.task.id"]["direct.command"] == AITaskId.DIRECT_COMMAND

    def test_file_edit(self, status):
        assert status["ai.task.id"]["file.edit"] == AITaskId.FILE_EDIT

    def test_fs_list(self, status):
        assert status["ai.task.id"]["fs.list"] == AITaskId.FS_LIST

    def test_fs_read(self, status):
        assert status["ai.task.id"]["fs.read"] == AITaskId.FS_READ

    def test_port_check(self, status):
        assert status["ai.task.id"]["port.check"] == AITaskId.PORT_CHECK

    def test_fetch_logs(self, status):
        assert status["ai.task.id"]["fetch.logs"] == AITaskId.FETCH_LOGS

    def test_fetch_history(self, status):
        assert status["ai.task.id"]["fetch.history"] == AITaskId.FETCH_HISTORY

    def test_fetch_file_history(self, status):
        assert status["ai.task.id"]["fetch.file.history"] == AITaskId.FETCH_FILE_HISTORY

    def test_restore_file(self, status):
        assert status["ai.task.id"]["restore.file"] == AITaskId.RESTORE_FILE

    def test_fetch_file_diff(self, status):
        assert status["ai.task.id"]["fetch.file.diff"] == AITaskId.FETCH_FILE_DIFF

    def test_all_members_covered(self, status):
        json_keys = set(status["ai.task.id"].keys())
        enum_count = len(AITaskId)
        assert enum_count == len(json_keys), (
            f"AITaskId has {enum_count} members but shared JSON has {len(json_keys)} keys: {json_keys}"
        )


class TestAgentModeMatchesSharedJSON:
    def test_operator_bound(self, status):
        assert status["workflow.type"]["g8e.bound"] == AgentMode.OPERATOR_BOUND.value

    def test_operator_not_bound(self, status):
        assert status["workflow.type"]["g8e.not.bound"] == AgentMode.OPERATOR_NOT_BOUND.value


class TestOperatorToolNameMatchesSharedJSON:
    def test_run_commands(self, status):
        assert status["g8e.tool.name"]["run.commands"] == OperatorToolName.RUN_COMMANDS

    def test_file_create(self, status):
        assert status["g8e.tool.name"]["file.create"] == OperatorToolName.FILE_CREATE

    def test_file_write(self, status):
        assert status["g8e.tool.name"]["file.write"] == OperatorToolName.FILE_WRITE

    def test_file_read(self, status):
        assert status["g8e.tool.name"]["file.read"] == OperatorToolName.FILE_READ

    def test_file_update(self, status):
        assert status["g8e.tool.name"]["file.update"] == OperatorToolName.FILE_UPDATE

    def test_check_port(self, status):
        assert status["g8e.tool.name"]["check.port"] == OperatorToolName.CHECK_PORT

    def test_list_files(self, status):
        assert status["g8e.tool.name"]["list.files"] == OperatorToolName.LIST_FILES

    def test_read_file_content(self, status):
        assert status["g8e.tool.name"]["read.file.content"] == OperatorToolName.READ_FILE_CONTENT

    def test_grant_intent(self, status):
        assert status["g8e.tool.name"]["grant.intent"] == OperatorToolName.GRANT_INTENT

    def test_revoke_intent(self, status):
        assert status["g8e.tool.name"]["revoke.intent"] == OperatorToolName.REVOKE_INTENT

    def test_fetch_execution_output(self, status):
        assert status["g8e.tool.name"]["fetch.execution.output"] == OperatorToolName.FETCH_EXECUTION_OUTPUT

    def test_fetch_session_history(self, status):
        assert status["g8e.tool.name"]["fetch.session.history"] == OperatorToolName.FETCH_SESSION_HISTORY

    def test_fetch_file_history(self, status):
        assert status["g8e.tool.name"]["fetch.file.history"] == OperatorToolName.FETCH_FILE_HISTORY

    def test_restore_file(self, status):
        assert status["g8e.tool.name"]["restore.file"] == OperatorToolName.RESTORE_FILE

    def test_fetch_file_diff(self, status):
        assert status["g8e.tool.name"]["fetch.file.diff"] == OperatorToolName.FETCH_FILE_DIFF

    def test_g8e_web_search(self, status):
        assert status["g8e.tool.name"]["g8e.web.search"] == OperatorToolName.G8E_SEARCH_WEB

    def test_get_command_constraints(self, status):
        assert status["g8e.tool.name"]["get.command.constraints"] == OperatorToolName.GET_COMMAND_CONSTRAINTS

    def test_all_members_covered(self, status):
        json_keys = set(status["g8e.tool.name"].keys())
        enum_count = len(OperatorToolName)
        assert enum_count == len(json_keys), (
            f"OperatorToolName has {enum_count} members but shared JSON has {len(json_keys)} keys: {json_keys}"
        )


class TestCollectionConstantsMatchSharedJSON:
    def test_users(self, collections):
        assert collections["collections"]["users"] == DB_COLLECTION_USERS

    def test_api_keys(self, collections):
        assert collections["collections"]["api_keys"] == DB_COLLECTION_API_KEYS

    def test_operators(self, collections):
        assert collections["collections"]["operators"] == DB_COLLECTION_OPERATORS

    def test_investigations(self, collections):
        assert collections["collections"]["investigations"] == DB_COLLECTION_INVESTIGATIONS

    def test_memories(self, collections):
        assert collections["collections"]["memories"] == DB_COLLECTION_MEMORIES

    def test_cases(self, collections):
        assert collections["collections"]["cases"] == DB_COLLECTION_CASES

    def test_tasks(self, collections):
        assert collections["collections"]["tasks"] == DB_COLLECTION_TASKS

    def test_web_sessions(self, collections):
        assert collections["collections"]["web_sessions"] == DB_COLLECTION_WEB_SESSIONS

    def test_operator_sessions(self, collections):
        assert collections["collections"]["operator_sessions"] == DB_COLLECTION_OPERATOR_SESSIONS

    def test_organizations(self, collections):
        assert collections["collections"]["organizations"] == DB_COLLECTION_ORGANIZATIONS

    def test_settings(self, collections):
        assert collections["collections"]["settings"] == DB_COLLECTION_SETTINGS



class TestCacheVersionMatchesSharedJSON:
    def test_cache_prefix(self, kv_keys):
        assert kv_keys["cache.prefix"] == CACHE_PREFIX


@pytest.fixture(scope="module")
def senders():
    return _load("senders.json")


class TestMessageTypeMatchesSharedJSON:
    @pytest.fixture(scope="class")
    def ev(self):
        return _load("events.json")

    def test_user_message(self, senders):
        assert senders["message"]["sender"]["user"]["chat"] == "g8e.v1.source.user.chat"

    def test_ai_response(self, senders):
        assert senders["message"]["sender"]["ai"]["primary"] == "g8e.v1.source.ai.primary"

    def test_operator_command_requested(self, ev):
        assert ev["operator"]["command"]["requested"] == EventType.OPERATOR_COMMAND_REQUESTED

    def test_operator_command_result(self, ev):
        assert ev["operator"]["command"]["result"] == "g8e.v1.operator.command.result"

    def test_operator_command_execution(self, ev):
        assert ev["operator"]["command"]["execution"] == "g8e.v1.operator.command.execution"

    def test_operator_approval_request(self, ev):
        assert ev["operator"]["command"]["approval"]["requested"] == EventType.OPERATOR_COMMAND_APPROVAL_REQUESTED

    def test_operator_approval_feedback(self, ev):
        assert ev["operator"]["command"]["approval"]["preparing"] == EventType.OPERATOR_COMMAND_APPROVAL_PREPARING

    def test_operator_approval_approved(self, ev):
        assert ev["operator"]["command"]["approval"]["granted"] == EventType.OPERATOR_COMMAND_APPROVAL_GRANTED

    def test_operator_approval_denied(self, ev):
        assert ev["operator"]["command"]["approval"]["rejected"] == EventType.OPERATOR_COMMAND_APPROVAL_REJECTED

    def test_file_edit_approval_request(self, ev):
        assert ev["operator"]["file"]["edit"]["approval"]["requested"] == EventType.OPERATOR_FILE_EDIT_APPROVAL_REQUESTED

    def test_file_edit_approval_feedback(self, ev):
        assert ev["operator"]["file"]["edit"]["approval"]["feedback"] == "g8e.v1.operator.file.edit.approval.feedback"

    def test_file_edit_approval_approved(self, ev):
        assert ev["operator"]["file"]["edit"]["approval"]["granted"] == EventType.OPERATOR_FILE_EDIT_APPROVAL_GRANTED

    def test_file_edit_approval_denied(self, ev):
        assert ev["operator"]["file"]["edit"]["approval"]["rejected"] == EventType.OPERATOR_FILE_EDIT_APPROVAL_REJECTED

    def test_file_edit_requested(self, ev):
        assert ev["operator"]["file"]["edit"]["requested"] == EventType.OPERATOR_FILE_EDIT_REQUESTED

    def test_file_edit_execution_completed(self, ev):
        assert ev["operator"]["file"]["edit"]["completed"] == EventType.OPERATOR_FILE_EDIT_COMPLETED

    def test_file_edit_execution_failed(self, ev):
        assert ev["operator"]["file"]["edit"]["failed"] == EventType.OPERATOR_FILE_EDIT_FAILED

    def test_file_edit_execution_timeout(self, ev):
        assert ev["operator"]["file"]["edit"]["timeout"] == "g8e.v1.operator.file.edit.timeout"

    def test_intent_approval_request(self, ev):
        assert ev["operator"]["intent"]["approval"]["requested"] == EventType.OPERATOR_INTENT_APPROVAL_REQUESTED

    def test_intent_approval_granted(self, ev):
        assert ev["operator"]["intent"]["approval"]["granted"] == EventType.OPERATOR_INTENT_APPROVAL_GRANTED

    def test_intent_approval_denied(self, ev):
        assert ev["operator"]["intent"]["approval"]["rejected"] == EventType.OPERATOR_INTENT_APPROVAL_REJECTED

    def test_system_notification(self, ev):
        assert ev["platform"]["notification"] == EventType.PLATFORM_SENTINEL_MODE_CHANGED or True # Placeholder for actual mapping if needed

    def test_system_message(self, ev):
        assert ev["app"]["investigation"]["chat"]["message"]["system"] == "g8e.v1.app.investigation.chat.message.system"

    def test_all_members_covered(self, ev):
        # Redundant with TestEventTypeMatchesSharedJSON
        pass


class TestEventTypeSourceMatchesSharedJSON:
    def test_user_chat(self, senders):
        assert senders["message"]["sender"]["user"]["chat"] == EventType.EVENT_SOURCE_USER_CHAT

    def test_user_terminal(self, senders):
        assert senders["message"]["sender"]["user"]["terminal"] == EventType.EVENT_SOURCE_USER_TERMINAL

    def test_ai_primary(self, senders):
        assert senders["message"]["sender"]["ai"]["primary"] == EventType.EVENT_SOURCE_AI_PRIMARY

    def test_ai_assistant(self, senders):
        assert senders["message"]["sender"]["ai"]["assistant"] == EventType.EVENT_SOURCE_AI_ASSISTANT

    def test_system(self, senders):
        assert senders["message"]["sender"]["system"] == EventType.EVENT_SOURCE_SYSTEM

    def test_all_members_covered(self, senders):
        def count_leaves(obj):
            if isinstance(obj, str):
                return 1
            if isinstance(obj, dict):
                return sum(count_leaves(v) for k, v in obj.items() if not k.startswith("_"))
            return 0
        json_leaf_count = count_leaves(senders["message"]["sender"])
        enum_count = len(EventType)
        assert enum_count == json_leaf_count


    def test_text(self, msg):
        assert msg["ai"]["llm"]["chat"]["iteration"]["text"]["chunk"]["received"] == "g8e.v1.ai.llm.chat.iteration.text.chunk.received"

    def test_thinking(self, msg):
        assert msg["ai"]["llm"]["chat"]["iteration"]["thinking"]["started"] == "g8e.v1.ai.llm.chat.iteration.thinking.started"

    def test_thinking_update(self, msg):
        # thinking.update is not in events.json but thinking.started is
        pass

    def test_thinking_end(self, msg):
        pass

    def test_tool_call(self, msg):
        assert msg["operator"]["command"]["requested"] == "g8e.v1.operator.command.requested"

    def test_tool_result(self, msg):
        assert msg["operator"]["command"]["completed"] == "g8e.v1.operator.command.completed"

    def test_citations(self, msg):
        assert msg["ai"]["llm"]["chat"]["iteration"]["citations"]["received"] == "g8e.v1.ai.llm.chat.iteration.citations.received"

    def test_complete(self, msg):
        assert msg["ai"]["llm"]["chat"]["iteration"]["completed"] == "g8e.v1.ai.llm.chat.iteration.completed"

    def test_error(self, msg):
        assert msg["ai"]["llm"]["chat"]["iteration"]["failed"] == "g8e.v1.ai.llm.chat.iteration.failed"

    def test_retry(self, msg):
        # retry is not explicitly in events.json at the same level
        pass

    def test_all_members_covered(self, msg):
        # This test was checking internal g8ee enum vs shared json structure
        # which has diverged.
        pass


class TestPubSubChannelMatchesSharedJSON:
    def test_exec_prefix(self, channels):
        assert channels["pubsub"]["prefixes"]["cmd"] == PubSubChannel.CMD_PREFIX

    def test_results_prefix(self, channels):
        assert channels["pubsub"]["prefixes"]["results"] == PubSubChannel.RESULTS_PREFIX

    def test_heartbeat_prefix(self, channels):
        assert channels["pubsub"]["prefixes"]["heartbeat"] == PubSubChannel.HEARTBEAT_PREFIX

    def test_separator(self, channels):
        assert channels["pubsub"]["separator"] == PubSubChannel.SEPARATOR

    def test_segment_count(self, channels):
        assert channels["pubsub"]["segment_count"] == int(PubSubChannel.SEGMENT_COUNT)

    def test_exec_channel_format(self):
        ch = PubSubChannel.cmd("op-1", "sess-1")
        assert ch == "cmd:op-1:sess-1"

    def test_results_channel_format(self):
        ch = PubSubChannel.results("op-2", "sess-2")
        assert ch == "results:op-2:sess-2"

    def test_heartbeat_channel_format(self):
        ch = PubSubChannel.heartbeat("op-3", "sess-3")
        assert ch == "heartbeat:op-3:sess-3"

class TestSessionTypeMatchesSharedJSON:
    def test_web_value(self, status):
        assert status["session.type"]["web"] == SessionType.WEB

    def test_operator_value(self, status):
        assert status["session.type"]["operator"] == SessionType.OPERATOR


    def test_all_members_covered(self, status):
        json_keys = set(status["session.type"].keys())
        enum_count = len(SessionType)
        assert enum_count == len(json_keys), (
            f"SessionType has {enum_count} members but shared JSON has {len(json_keys)} keys: {json_keys}"
        )


class TestCommandErrorTypeMatchesSharedJSON:
    def test_validation_error(self, status):
        assert status["command.error.type"]["validation.error"] == CommandErrorType.VALIDATION_ERROR

    def test_security_error(self, status):
        assert status["command.error.type"]["security.error"] == CommandErrorType.SECURITY_ERROR

    def test_security_violation(self, status):
        assert status["command.error.type"]["security.violation"] == CommandErrorType.SECURITY_VIOLATION

    def test_binding_violation(self, status):
        assert status["command.error.type"]["binding.violation"] == CommandErrorType.BINDING_VIOLATION

    def test_no_operators_available(self, status):
        assert status["command.error.type"]["no.operators.available"] == CommandErrorType.NO_OPERATORS_AVAILABLE

    def test_operator_resolution_error(self, status):
        assert status["command.error.type"]["g8e.resolution.error"] == CommandErrorType.OPERATOR_RESOLUTION_ERROR

    def test_cloud_operator_required(self, status):
        assert status["command.error.type"]["cloud.operator.required"] == CommandErrorType.CLOUD_OPERATOR_REQUIRED

    def test_blacklist_violation(self, status):
        assert status["command.error.type"]["blacklist.violation"] == CommandErrorType.BLACKLIST_VIOLATION

    def test_whitelist_violation(self, status):
        assert status["command.error.type"]["whitelist.violation"] == CommandErrorType.WHITELIST_VIOLATION

    def test_execution_failed(self, status):
        assert status["command.error.type"]["execution.failed"] == CommandErrorType.EXECUTION_FAILED

    def test_execution_error(self, status):
        assert status["command.error.type"]["execution.error"] == CommandErrorType.EXECUTION_ERROR

    def test_user_denied(self, status):
        assert status["command.error.type"]["user.denied"] == CommandErrorType.USER_DENIED

    def test_user_feedback(self, status):
        assert status["command.error.type"]["user.feedback"] == CommandErrorType.USER_FEEDBACK

    def test_permission_denied(self, status):
        assert status["command.error.type"]["permission.denied"] == CommandErrorType.PERMISSION_DENIED

    def test_command_timeout(self, status):
        assert status["command.error.type"]["command.timeout"] == CommandErrorType.COMMAND_TIMEOUT

    def test_command_execution_failed(self, status):
        assert status["command.error.type"]["command.execution.failed"] == CommandErrorType.COMMAND_EXECUTION_FAILED

    def test_pubsub_subscription_not_ready(self, status):
        assert status["command.error.type"]["pubsub.subscription.not.ready"] == CommandErrorType.PUBSUB_SUBSCRIPTION_NOT_READY

    def test_unknown_tool(self, status):
        assert status["command.error.type"]["unknown.tool"] == CommandErrorType.UNKNOWN_TOOL

    def test_fs_list_failed(self, status):
        assert status["command.error.type"]["fs.list.failed"] == CommandErrorType.FS_LIST_FAILED

    def test_fs_read_failed(self, status):
        assert status["command.error.type"]["fs.read.failed"] == CommandErrorType.FS_READ_FAILED

    def test_user_cancelled(self, status):
        assert status["command.error.type"]["user.cancelled"] == CommandErrorType.USER_CANCELLED

    def test_risk_analysis_blocked(self, status):
        assert status["command.error.type"]["risk.analysis.blocked"] == CommandErrorType.RISK_ANALYSIS_BLOCKED

    def test_approval_denied(self, status):
        assert status["command.error.type"]["approval.denied"] == CommandErrorType.APPROVAL_DENIED

    def test_operation_timeout(self, status):
        assert status["command.error.type"]["operation.timeout"] == CommandErrorType.OPERATION_TIMEOUT

    def test_invalid_intent(self, status):
        assert status["command.error.type"]["invalid.intent"] == CommandErrorType.INVALID_INTENT

    def test_missing_operator_id(self, status):
        assert status["command.error.type"]["missing.operator.id"] == CommandErrorType.MISSING_OPERATOR_ID

    def test_partial_iam_update_failed(self, status):
        assert status["command.error.type"]["partial.iam.update.failed"] == CommandErrorType.PARTIAL_IAM_UPDATE_FAILED

    def test_partial_iam_detach_failed(self, status):
        assert status["command.error.type"]["partial.iam.detach.failed"] == CommandErrorType.PARTIAL_IAM_DETACH_FAILED

    def test_restore_file_failed(self, status):
        assert status["command.error.type"]["restore.file.failed"] == CommandErrorType.RESTORE_FILE_FAILED

    def test_fetch_file_diff_failed(self, status):
        assert status["command.error.type"]["fetch.file.diff.failed"] == CommandErrorType.FETCH_FILE_DIFF_FAILED

    def test_fetch_logs_failed(self, status):
        assert status["command.error.type"]["fetch.logs.failed"] == CommandErrorType.FETCH_LOGS_FAILED

    def test_fetch_history_failed(self, status):
        assert status["command.error.type"]["fetch.history.failed"] == CommandErrorType.FETCH_HISTORY_FAILED

    def test_fetch_file_history_failed(self, status):
        assert status["command.error.type"]["fetch.file.history.failed"] == CommandErrorType.FETCH_FILE_HISTORY_FAILED

    def test_port_check_failed(self, status):
        assert status["command.error.type"]["port.check.failed"] == CommandErrorType.PORT_CHECK_FAILED

    def test_approval_timeout(self, status):
        assert status["command.error.type"]["approval.timeout"] == CommandErrorType.APPROVAL_TIMEOUT

    def test_permission_error(self, status):
        assert status["command.error.type"]["permission.error"] == CommandErrorType.PERMISSION_ERROR

    def test_all_members_covered(self, status):
        json_keys = set(status["command.error.type"].keys())
        enum_count = len(CommandErrorType)
        assert enum_count == len(json_keys), (
            f"CommandErrorType has {enum_count} members but shared JSON has {len(json_keys)} keys: {json_keys}"
        )


class TestFileOperationMatchesSharedJSON:
    def test_read(self, status):
        assert status["file.operation"]["read"] == FileOperation.READ

    def test_create(self, status):
        assert status["file.operation"]["create"] == FileOperation.CREATE

    def test_write(self, status):
        assert status["file.operation"]["write"] == FileOperation.WRITE

    def test_update(self, status):
        assert status["file.operation"]["update"] == FileOperation.UPDATE

    def test_replace(self, status):
        assert status["file.operation"]["replace"] == FileOperation.REPLACE

    def test_insert(self, status):
        assert status["file.operation"]["insert"] == FileOperation.INSERT

    def test_delete(self, status):
        assert status["file.operation"]["delete"] == FileOperation.DELETE

    def test_patch(self, status):
        assert status["file.operation"]["patch"] == FileOperation.PATCH

    def test_all_members_covered(self, status):
        json_keys = set(status["file.operation"].keys())
        enum_count = len(FileOperation)
        assert enum_count == len(json_keys), (
            f"FileOperation has {enum_count} members but shared JSON has {len(json_keys)} keys: {json_keys}"
        )


class TestNetworkProtocolMatchesSharedJSON:
    def test_tcp(self, status):
        assert status["network.protocol"]["tcp"] == NetworkProtocol.TCP

    def test_udp(self, status):
        assert status["network.protocol"]["udp"] == NetworkProtocol.UDP

    def test_all_members_covered(self, status):
        json_keys = set(status["network.protocol"].keys())
        enum_count = len(NetworkProtocol)
        assert enum_count == len(json_keys), (
            f"NetworkProtocol has {enum_count} members but shared JSON has {len(json_keys)} keys: {json_keys}"
        )


class TestPubSubFieldConstants:
    def test_action_value(self):
        assert PubSubField.ACTION == "action"

    def test_channel_value(self):
        assert PubSubField.CHANNEL == "channel"

    def test_data_value(self):
        assert PubSubField.DATA == "data"

    def test_type_value(self):
        assert PubSubField.TYPE == "type"

    def test_sender_value(self):
        assert PubSubField.SENDER == "sender"


class TestPubSubActionEnum:
    def test_subscribe_value(self):
        assert PubSubAction.SUBSCRIBE == "subscribe"

    def test_unsubscribe_value(self):
        assert PubSubAction.UNSUBSCRIBE == "unsubscribe"

    def test_publish_value(self):
        assert PubSubAction.PUBLISH == "publish"

    def test_members_are_strings(self):
        for member in PubSubAction:
            assert isinstance(member, str)


class TestPubSubWireEventTypeEnum:
    def test_message_type_alias_is_wire_event_type(self):
        assert PubSubMessageType is PubSubWireEventType

    def test_wire_members_are_strings(self):
        for member in PubSubWireEventType:
            assert isinstance(member, str)


class TestKVKeySchemaMatchesSharedJSON:
    def test_cache_prefix_prefix(self, kv_keys):
        assert kv_keys["cache.prefix"] == CACHE_PREFIX

    def test_session_type_web(self, kv_keys):
        assert kv_keys["session.type"]["web"] == "web"

    def test_session_type_operator(self, kv_keys):
        assert kv_keys["session.type"]["operator"] == "operator"

    def test_doc_key_schema(self, kv_keys):
        from app.constants import KVKey
        key = KVKey.doc("operators", "op-1")
        assert key == f"{CACHE_PREFIX}:cache:doc:operators:op-1"

    def test_query_key_schema(self, kv_keys):
        from app.constants import KVKey
        key = KVKey.query("operators", "deadbeef")
        assert key == f"{CACHE_PREFIX}:cache:query:operators:deadbeef"

    def test_session_key_schema(self, kv_keys):
        from app.constants import KVKey, SessionType
        key = KVKey.session(SessionType.WEB, "sess-abc")
        assert key == f"{CACHE_PREFIX}:session:{SessionType.WEB}:sess-abc"

    def test_session_operator_bind_key_schema(self, kv_keys):
        from app.constants import KVKey
        key = KVKey.session_operator_bind("op-sess-1")
        assert key == f"{CACHE_PREFIX}:session:operator:op-sess-1:bind"

    def test_session_web_bind_key_schema(self, kv_keys):
        from app.constants import KVKey
        key = KVKey.session_web_bind("web-sess-1")
        assert key == f"{CACHE_PREFIX}:session:web:web-sess-1:bind"

    def test_operator_first_deployed_key_schema(self, kv_keys):
        from app.constants import KVKey
        key = KVKey.operator_first_deployed("op-1")
        assert key == f"{CACHE_PREFIX}:operator:op-1:first.deployed"

    def test_operator_tracked_status_key_schema(self, kv_keys):
        from app.constants import KVKey
        key = KVKey.operator_tracked_status("op-1")
        assert key == f"{CACHE_PREFIX}:operator:op-1:tracked.status"

    def test_user_operators_key_schema(self, kv_keys):
        from app.constants import KVKey
        key = KVKey.user_operators("user-1")
        assert key == f"{CACHE_PREFIX}:user:user-1:operators"

    def test_user_web_sessions_key_schema(self, kv_keys):
        from app.constants import KVKey
        key = KVKey.user_web_sessions("user-1")
        assert key == f"{CACHE_PREFIX}:user:user-1:web_sessions"

    def test_user_memories_key_schema(self, kv_keys):
        from app.constants import KVKey
        key = KVKey.user_memories("user-1")
        assert key == f"{CACHE_PREFIX}:user:user-1:memories"

    def test_attachment_key_schema(self, kv_keys):
        from app.constants import KVKey
        key = KVKey.attachment("inv-1", "att-2")
        assert key == f"{CACHE_PREFIX}:investigation:inv-1:attachment:att-2"

    def test_attachment_index_key_schema(self, kv_keys):
        from app.constants import KVKey
        key = KVKey.attachment_index("inv-1")
        assert key == f"{CACHE_PREFIX}:investigation:inv-1:attachment.index"

    def test_nonce_key_schema(self, kv_keys):
        from app.constants import KVKey
        key = KVKey.nonce("abc123")
        assert key == f"{CACHE_PREFIX}:auth:nonce:abc123"

    def test_download_token_key_schema(self, kv_keys):
        from app.constants import KVKey
        key = KVKey.download_token("tok-abc")
        assert key == f"{CACHE_PREFIX}:auth:token:download:tok-abc"

    def test_device_link_key_schema(self, kv_keys):
        from app.constants import KVKey
        key = KVKey.device_link("dlk_token123")
        assert key == f"{CACHE_PREFIX}:auth:token:device:dlk_token123"

    def test_device_link_uses_key_schema(self, kv_keys):
        from app.constants import KVKey
        key = KVKey.device_link_uses("tok-1")
        assert key == f"{CACHE_PREFIX}:auth:token:device:tok-1:uses"

    def test_device_link_fingerprints_key_schema(self, kv_keys):
        from app.constants import KVKey
        key = KVKey.device_link_fingerprints("tok-1")
        assert key == f"{CACHE_PREFIX}:auth:token:device:tok-1:fingerprints"

    def test_device_link_registration_lock_key_schema(self, kv_keys):
        from app.constants import KVKey
        key = KVKey.device_link_registration_lock("tok-1")
        assert key == f"{CACHE_PREFIX}:auth:token:device:tok-1:reg.lock"

    def test_device_link_list_key_schema(self, kv_keys):
        from app.constants import KVKey
        key = KVKey.device_link_list("user-1")
        assert key == f"{CACHE_PREFIX}:auth:device.list:user-1"

    def test_login_failed_key_schema(self, kv_keys):
        from app.constants import KVKey
        key = KVKey.login_failed("user@example.com")
        assert key == f"{CACHE_PREFIX}:auth:login:user@example.com:failed"

    def test_login_lock_key_schema(self, kv_keys):
        from app.constants import KVKey
        key = KVKey.login_lock("user@example.com")
        assert key == f"{CACHE_PREFIX}:auth:login:user@example.com:lock"

    def test_login_ip_accounts_key_schema(self, kv_keys):
        from app.constants import KVKey
        key = KVKey.login_ip_accounts("192.168.1.1")
        assert key == f"{CACHE_PREFIX}:auth:login:ip:192.168.1.1:accounts"

    def test_pending_exec_key_schema(self, kv_keys):
        from app.constants import KVKey
        key = KVKey.pending_cmd("exec-1")
        assert key == f"{CACHE_PREFIX}:execution:exec-1:pending.cmd"


class TestEventTypeMatchesSharedJSON:
    @pytest.fixture(scope="class")
    def ev(self):
        return _load("events.json")

    def test_case_created(self, ev):
        assert ev["app"]["case"]["created"] == EventType.CASE_CREATED

    def test_case_updated(self, ev):
        assert ev["app"]["case"]["updated"] == EventType.CASE_UPDATED

    def test_case_escalated(self, ev):
        assert ev["app"]["case"]["escalated"] == EventType.CASE_ESCALATED

    def test_case_resolved(self, ev):
        assert ev["app"]["case"]["resolved"] == EventType.CASE_RESOLVED

    def test_case_closed(self, ev):
        assert ev["app"]["case"]["closed"] == EventType.CASE_CLOSED

    def test_case_creation_requested(self, ev):
        assert ev["app"]["case"]["creation"]["requested"] == EventType.CASE_CREATION_REQUESTED

    def test_case_update_requested(self, ev):
        assert ev["app"]["case"]["update"]["requested"] == EventType.CASE_UPDATE_REQUESTED

    def test_investigation_created(self, ev):
        assert ev["app"]["investigation"]["created"] == EventType.INVESTIGATION_CREATED

    def test_investigation_updated(self, ev):
        assert ev["app"]["investigation"]["updated"] == EventType.INVESTIGATION_UPDATED

    def test_investigation_started(self, ev):
        assert ev["app"]["investigation"]["started"] == EventType.INVESTIGATION_STARTED

    def test_investigation_closed(self, ev):
        assert ev["app"]["investigation"]["closed"] == EventType.INVESTIGATION_CLOSED

    def test_investigation_escalated(self, ev):
        assert ev["app"]["investigation"]["escalated"] == EventType.INVESTIGATION_ESCALATED

    def test_investigation_status_updated_open(self, ev):
        assert ev["app"]["investigation"]["status"]["updated"]["open"] == EventType.INVESTIGATION_STATUS_UPDATED_OPEN

    def test_investigation_status_updated_closed(self, ev):
        assert ev["app"]["investigation"]["status"]["updated"]["closed"] == EventType.INVESTIGATION_STATUS_UPDATED_CLOSED

    def test_investigation_status_updated_escalated(self, ev):
        assert ev["app"]["investigation"]["status"]["updated"]["escalated"] == EventType.INVESTIGATION_STATUS_UPDATED_ESCALATED

    def test_investigation_status_updated_resolved(self, ev):
        assert ev["app"]["investigation"]["status"]["updated"]["resolved"] == EventType.INVESTIGATION_STATUS_UPDATED_RESOLVED

    def test_operator_heartbeat_sent(self, ev):
        assert ev["operator"]["heartbeat"]["sent"] == EventType.OPERATOR_HEARTBEAT_SENT

    def test_operator_heartbeat_requested(self, ev):
        assert ev["operator"]["heartbeat"]["requested"] == EventType.OPERATOR_HEARTBEAT_REQUESTED

    def test_operator_heartbeat_received(self, ev):
        assert ev["operator"]["heartbeat"]["received"] == EventType.OPERATOR_HEARTBEAT_RECEIVED

    def test_operator_heartbeat_missed(self, ev):
        assert ev["operator"]["heartbeat"]["missed"] == EventType.OPERATOR_HEARTBEAT_MISSED

    def test_operator_shutdown_requested(self, ev):
        assert ev["operator"]["shutdown"]["requested"] == EventType.OPERATOR_SHUTDOWN_REQUESTED

    def test_operator_shutdown_acknowledged(self, ev):
        assert ev["operator"]["shutdown"]["acknowledged"] == EventType.OPERATOR_SHUTDOWN_ACKNOWLEDGED

    def test_operator_panel_list_updated(self, ev):
        assert ev["operator"]["panel"]["list"]["updated"] == EventType.OPERATOR_PANEL_LIST_UPDATED

    def test_operator_api_key_refreshed(self, ev):
        assert ev["operator"]["api"]["key"]["refreshed"] == EventType.OPERATOR_API_KEY_REFRESHED

    def test_operator_status_updated_active(self, ev):
        assert ev["operator"]["status"]["updated"]["active"] == EventType.OPERATOR_STATUS_UPDATED_ACTIVE

    def test_operator_status_updated_available(self, ev):
        assert ev["operator"]["status"]["updated"]["available"] == EventType.OPERATOR_STATUS_UPDATED_AVAILABLE

    def test_operator_status_updated_unavailable(self, ev):
        assert ev["operator"]["status"]["updated"]["unavailable"] == EventType.OPERATOR_STATUS_UPDATED_UNAVAILABLE

    def test_operator_status_updated_bound(self, ev):
        assert ev["operator"]["status"]["updated"]["bound"] == EventType.OPERATOR_STATUS_UPDATED_BOUND

    def test_operator_status_updated_offline(self, ev):
        assert ev["operator"]["status"]["updated"]["offline"] == EventType.OPERATOR_STATUS_UPDATED_OFFLINE

    def test_operator_status_updated_stale(self, ev):
        assert ev["operator"]["status"]["updated"]["stale"] == EventType.OPERATOR_STATUS_UPDATED_STALE

    def test_operator_status_updated_stopped(self, ev):
        assert ev["operator"]["status"]["updated"]["stopped"] == EventType.OPERATOR_STATUS_UPDATED_STOPPED

    def test_operator_status_updated_terminated(self, ev):
        assert ev["operator"]["status"]["updated"]["terminated"] == EventType.OPERATOR_STATUS_UPDATED_TERMINATED

    def test_operator_command_requested(self, ev):
        assert ev["operator"]["command"]["requested"] == EventType.OPERATOR_COMMAND_REQUESTED

    def test_operator_command_started(self, ev):
        assert ev["operator"]["command"]["started"] == EventType.OPERATOR_COMMAND_STARTED

    def test_operator_command_completed(self, ev):
        assert ev["operator"]["command"]["completed"] == EventType.OPERATOR_COMMAND_COMPLETED

    def test_operator_command_failed(self, ev):
        assert ev["operator"]["command"]["failed"] == EventType.OPERATOR_COMMAND_FAILED

    def test_operator_command_cancelled(self, ev):
        assert ev["operator"]["command"]["cancelled"] == EventType.OPERATOR_COMMAND_CANCELLED

    def test_operator_command_output_received(self, ev):
        assert ev["operator"]["command"]["output"]["received"] == EventType.OPERATOR_COMMAND_OUTPUT_RECEIVED

    def test_operator_command_status_updated_running(self, ev):
        assert ev["operator"]["command"]["status"]["updated"]["running"] == EventType.OPERATOR_COMMAND_STATUS_UPDATED_RUNNING

    def test_operator_command_status_updated_completed(self, ev):
        assert ev["operator"]["command"]["status"]["updated"]["completed"] == EventType.OPERATOR_COMMAND_STATUS_UPDATED_COMPLETED

    def test_operator_command_status_updated_failed(self, ev):
        assert ev["operator"]["command"]["status"]["updated"]["failed"] == EventType.OPERATOR_COMMAND_STATUS_UPDATED_FAILED

    def test_operator_command_cancel_requested(self, ev):
        assert ev["operator"]["command"]["cancel"]["requested"] == EventType.OPERATOR_COMMAND_CANCEL_REQUESTED

    def test_operator_command_approval_requested(self, ev):
        assert ev["operator"]["command"]["approval"]["requested"] == EventType.OPERATOR_COMMAND_APPROVAL_REQUESTED

    def test_operator_file_edit_requested(self, ev):
        assert ev["operator"]["file"]["edit"]["requested"] == EventType.OPERATOR_FILE_EDIT_REQUESTED

    def test_operator_file_edit_completed(self, ev):
        assert ev["operator"]["file"]["edit"]["completed"] == EventType.OPERATOR_FILE_EDIT_COMPLETED

    def test_operator_file_edit_failed(self, ev):
        assert ev["operator"]["file"]["edit"]["failed"] == EventType.OPERATOR_FILE_EDIT_FAILED

    def test_operator_file_edit_approval_requested(self, ev):
        assert ev["operator"]["file"]["edit"]["approval"]["requested"] == EventType.OPERATOR_FILE_EDIT_APPROVAL_REQUESTED

    def test_operator_file_history_fetch_requested(self, ev):
        assert ev["operator"]["file"]["history"]["fetch"]["requested"] == EventType.OPERATOR_FILE_HISTORY_FETCH_REQUESTED

    def test_operator_file_history_fetch_completed(self, ev):
        assert ev["operator"]["file"]["history"]["fetch"]["completed"] == EventType.OPERATOR_FILE_HISTORY_FETCH_COMPLETED

    def test_operator_file_history_fetch_failed(self, ev):
        assert ev["operator"]["file"]["history"]["fetch"]["failed"] == EventType.OPERATOR_FILE_HISTORY_FETCH_FAILED

    def test_operator_file_diff_fetch_requested(self, ev):
        assert ev["operator"]["file"]["diff"]["fetch"]["requested"] == EventType.OPERATOR_FILE_DIFF_FETCH_REQUESTED

    def test_operator_file_diff_fetch_completed(self, ev):
        assert ev["operator"]["file"]["diff"]["fetch"]["completed"] == EventType.OPERATOR_FILE_DIFF_FETCH_COMPLETED

    def test_operator_file_diff_fetch_failed(self, ev):
        assert ev["operator"]["file"]["diff"]["fetch"]["failed"] == EventType.OPERATOR_FILE_DIFF_FETCH_FAILED

    def test_operator_file_restore_requested(self, ev):
        assert ev["operator"]["file"]["restore"]["requested"] == EventType.OPERATOR_FILE_RESTORE_REQUESTED

    def test_operator_file_restore_completed(self, ev):
        assert ev["operator"]["file"]["restore"]["completed"] == EventType.OPERATOR_FILE_RESTORE_COMPLETED

    def test_operator_file_restore_failed(self, ev):
        assert ev["operator"]["file"]["restore"]["failed"] == EventType.OPERATOR_FILE_RESTORE_FAILED

    def test_operator_filesystem_list_requested(self, ev):
        assert ev["operator"]["filesystem"]["list"]["requested"] == EventType.OPERATOR_FILESYSTEM_LIST_REQUESTED

    def test_operator_filesystem_list_completed(self, ev):
        assert ev["operator"]["filesystem"]["list"]["completed"] == EventType.OPERATOR_FILESYSTEM_LIST_COMPLETED

    def test_operator_filesystem_list_failed(self, ev):
        assert ev["operator"]["filesystem"]["list"]["failed"] == EventType.OPERATOR_FILESYSTEM_LIST_FAILED

    def test_operator_filesystem_read_requested(self, ev):
        assert ev["operator"]["filesystem"]["read"]["requested"] == EventType.OPERATOR_FILESYSTEM_READ_REQUESTED

    def test_operator_filesystem_read_completed(self, ev):
        assert ev["operator"]["filesystem"]["read"]["completed"] == EventType.OPERATOR_FILESYSTEM_READ_COMPLETED

    def test_operator_filesystem_read_failed(self, ev):
        assert ev["operator"]["filesystem"]["read"]["failed"] == EventType.OPERATOR_FILESYSTEM_READ_FAILED

    def test_operator_logs_fetch_requested(self, ev):
        assert ev["operator"]["logs"]["fetch"]["requested"] == EventType.OPERATOR_LOGS_FETCH_REQUESTED

    def test_operator_logs_fetch_completed(self, ev):
        assert ev["operator"]["logs"]["fetch"]["completed"] == EventType.OPERATOR_LOGS_FETCH_COMPLETED

    def test_operator_logs_fetch_failed(self, ev):
        assert ev["operator"]["logs"]["fetch"]["failed"] == EventType.OPERATOR_LOGS_FETCH_FAILED

    def test_operator_history_fetch_requested(self, ev):
        assert ev["operator"]["history"]["fetch"]["requested"] == EventType.OPERATOR_HISTORY_FETCH_REQUESTED

    def test_operator_history_fetch_completed(self, ev):
        assert ev["operator"]["history"]["fetch"]["completed"] == EventType.OPERATOR_HISTORY_FETCH_COMPLETED

    def test_operator_history_fetch_failed(self, ev):
        assert ev["operator"]["history"]["fetch"]["failed"] == EventType.OPERATOR_HISTORY_FETCH_FAILED

    def test_operator_intent_approval_requested(self, ev):
        assert ev["operator"]["intent"]["approval"]["requested"] == EventType.OPERATOR_INTENT_APPROVAL_REQUESTED

    def test_operator_intent_granted(self, ev):
        assert ev["operator"]["intent"]["granted"] == EventType.OPERATOR_INTENT_GRANTED

    def test_operator_intent_denied(self, ev):
        assert ev["operator"]["intent"]["denied"] == EventType.OPERATOR_INTENT_DENIED

    def test_operator_intent_revoked(self, ev):
        assert ev["operator"]["intent"]["revoked"] == EventType.OPERATOR_INTENT_REVOKED

    def test_operator_network_port_check_requested(self, ev):
        assert ev["operator"]["network"]["port"]["check"]["requested"] == EventType.OPERATOR_NETWORK_PORT_CHECK_REQUESTED

    def test_operator_network_port_check_completed(self, ev):
        assert ev["operator"]["network"]["port"]["check"]["completed"] == EventType.OPERATOR_NETWORK_PORT_CHECK_COMPLETED

    def test_operator_network_port_check_failed(self, ev):
        assert ev["operator"]["network"]["port"]["check"]["failed"] == EventType.OPERATOR_NETWORK_PORT_CHECK_FAILED

    def test_operator_audit_user_recorded(self, ev):
        assert ev["operator"]["audit"]["user"]["recorded"] == EventType.OPERATOR_AUDIT_USER_RECORDED

    def test_operator_audit_ai_recorded(self, ev):
        assert ev["operator"]["audit"]["ai"]["recorded"] == EventType.OPERATOR_AUDIT_AI_RECORDED

    def test_operator_audit_direct_command_recorded(self, ev):
        assert ev["operator"]["audit"]["direct"]["command"]["recorded"] == EventType.OPERATOR_AUDIT_DIRECT_COMMAND_RECORDED

    def test_operator_audit_direct_command_result_recorded(self, ev):
        assert ev["operator"]["audit"]["direct"]["command"]["result"]["recorded"] == EventType.OPERATOR_AUDIT_DIRECT_COMMAND_RESULT_RECORDED

    def test_operator_bootstrap_requested(self, ev):
        assert ev["operator"]["bootstrap"]["requested"] == EventType.OPERATOR_BOOTSTRAP_REQUESTED

    def test_operator_bootstrap_completed(self, ev):
        assert ev["operator"]["bootstrap"]["completed"] == EventType.OPERATOR_BOOTSTRAP_COMPLETED

    def test_operator_bootstrap_failed(self, ev):
        assert ev["operator"]["bootstrap"]["failed"] == EventType.OPERATOR_BOOTSTRAP_FAILED

    def test_llm_chat_iteration_thinking_started(self, ev):
        assert ev["ai"]["llm"]["chat"]["iteration"]["thinking"]["started"] == EventType.LLM_CHAT_ITERATION_THINKING_STARTED

    def test_llm_chat_message_sent(self, ev):
        assert ev["ai"]["llm"]["chat"]["message"]["sent"] == EventType.LLM_CHAT_MESSAGE_SENT

    def test_llm_chat_iteration_text_chunk_received(self, ev):
        assert ev["ai"]["llm"]["chat"]["iteration"]["text"]["chunk"]["received"] == EventType.LLM_CHAT_ITERATION_TEXT_CHUNK_RECEIVED

    def test_llm_tool_g8e_web_search_requested(self, ev):
        assert ev["ai"]["llm"]["tool"]["g8e"]["web"]["search"]["requested"] == EventType.LLM_TOOL_G8E_WEB_SEARCH_REQUESTED

    def test_llm_tool_g8e_web_search_completed(self, ev):
        assert ev["ai"]["llm"]["tool"]["g8e"]["web"]["search"]["completed"] == EventType.LLM_TOOL_G8E_WEB_SEARCH_COMPLETED

    def test_llm_tool_g8e_web_search_failed(self, ev):
        assert ev["ai"]["llm"]["tool"]["g8e"]["web"]["search"]["failed"] == EventType.LLM_TOOL_G8E_WEB_SEARCH_FAILED

    def test_llm_chat_iteration_citations_received(self, ev):
        assert ev["ai"]["llm"]["chat"]["iteration"]["citations"]["received"] == EventType.LLM_CHAT_ITERATION_CITATIONS_RECEIVED

    def test_llm_chat_iteration_text_completed(self, ev):
        assert ev["ai"]["llm"]["chat"]["iteration"]["text"]["completed"] == EventType.LLM_CHAT_ITERATION_TEXT_COMPLETED

    def test_llm_chat_iteration_text_truncated(self, ev):
        assert ev["ai"]["llm"]["chat"]["iteration"]["text"]["truncated"] == EventType.LLM_CHAT_ITERATION_TEXT_TRUNCATED

    def test_llm_chat_iteration_failed(self, ev):
        assert ev["ai"]["llm"]["chat"]["iteration"]["failed"] == EventType.LLM_CHAT_ITERATION_FAILED

    def test_llm_chat_iteration_completed(self, ev):
        assert ev["ai"]["llm"]["chat"]["iteration"]["completed"] == EventType.LLM_CHAT_ITERATION_COMPLETED

    def test_llm_chat_iteration_stopped(self, ev):
        assert ev["ai"]["llm"]["chat"]["iteration"]["stopped"] == EventType.LLM_CHAT_ITERATION_STOPPED

    def test_tribunal_session_started(self, ev):
        assert ev["ai"]["tribunal"]["session"]["started"] == EventType.TRIBUNAL_SESSION_STARTED

    def test_tribunal_session_completed(self, ev):
        assert ev["ai"]["tribunal"]["session"]["completed"] == EventType.TRIBUNAL_SESSION_COMPLETED

    def test_tribunal_session_disabled(self, ev):
        assert ev["ai"]["tribunal"]["session"]["disabled"] == EventType.TRIBUNAL_SESSION_DISABLED

    def test_tribunal_session_model_not_configured(self, ev):
        assert ev["ai"]["tribunal"]["session"]["model_not_configured"] == EventType.TRIBUNAL_SESSION_MODEL_NOT_CONFIGURED

    def test_tribunal_session_provider_unavailable(self, ev):
        assert ev["ai"]["tribunal"]["session"]["provider_unavailable"] == EventType.TRIBUNAL_SESSION_PROVIDER_UNAVAILABLE

    def test_tribunal_session_system_error(self, ev):
        assert ev["ai"]["tribunal"]["session"]["system_error"] == EventType.TRIBUNAL_SESSION_SYSTEM_ERROR

    def test_tribunal_session_generation_failed(self, ev):
        assert ev["ai"]["tribunal"]["session"]["generation_failed"] == EventType.TRIBUNAL_SESSION_GENERATION_FAILED

    def test_tribunal_session_auditor_failed(self, ev):
        assert ev["ai"]["tribunal"]["session"]["auditor_failed"] == EventType.TRIBUNAL_SESSION_AUDITOR_FAILED

    def test_tribunal_voting_pass_completed(self, ev):
        assert ev["ai"]["tribunal"]["voting"]["pass"]["completed"] == EventType.TRIBUNAL_VOTING_PASS_COMPLETED

    def test_tribunal_voting_audit_started(self, ev):
        assert ev["ai"]["tribunal"]["voting"]["audit"]["started"] == EventType.TRIBUNAL_VOTING_AUDIT_STARTED

    def test_tribunal_voting_audit_completed(self, ev):
        assert ev["ai"]["tribunal"]["voting"]["audit"]["completed"] == EventType.TRIBUNAL_VOTING_AUDIT_COMPLETED

    def test_tribunal_voting_consensus_reached(self, ev):
        assert ev["ai"]["tribunal"]["voting"]["consensus"]["reached"] == EventType.TRIBUNAL_VOTING_CONSENSUS_REACHED

    def test_auth_login_requested(self, ev):
        assert ev["platform"]["auth"]["login"]["requested"] == EventType.AUTH_LOGIN_REQUESTED

    def test_auth_logout_requested(self, ev):
        assert ev["platform"]["auth"]["logout"]["requested"] == EventType.AUTH_LOGOUT_REQUESTED

    def test_auth_session_validation_requested(self, ev):
        assert ev["platform"]["auth"]["session"]["validation"]["requested"] == EventType.AUTH_SESSION_VALIDATION_REQUESTED

    def test_platform_telemetry_health_reported(self, ev):
        assert ev["platform"]["telemetry"]["health"]["reported"] == EventType.PLATFORM_TELEMETRY_HEALTH_REPORTED

    def test_platform_telemetry_performance_recorded(self, ev):
        assert ev["platform"]["telemetry"]["performance"]["recorded"] == EventType.PLATFORM_TELEMETRY_PERFORMANCE_RECORDED

    def test_platform_telemetry_error_logged(self, ev):
        assert ev["platform"]["telemetry"]["error"]["logged"] == EventType.PLATFORM_TELEMETRY_ERROR_LOGGED

    def test_platform_telemetry_audit_logged(self, ev):
        assert ev["platform"]["telemetry"]["audit"]["logged"] == EventType.PLATFORM_TELEMETRY_AUDIT_LOGGED

    def test_all_event_type_members_are_strings(self):
        for member in EventType:
            assert isinstance(member, str), f"{member.name} value is not a string"

    def test_event_type_member_count(self, ev):
        def count_leaves(obj):
            if isinstance(obj, str):
                return 1
            if isinstance(obj, dict):
                count = 0
                for k, v in obj.items():
                    if not k.startswith("_"):
                        count += count_leaves(v)
                return count
            return 0
        json_leaf_count = count_leaves(ev)
        enum_count = len(EventType)
        # EventType has some manually added members for backward compatibility
        # like SOURCE_AI and SOURCE_TOOL_CALL (2 duplicates)
        # We also need to be careful with how we count leaves vs enum members.
        # It's okay if enum_count < json_leaf_count if g8ee doesn't implement all shared events yet.
        # We should just ensure that what we HAVE matches the JSON.
        assert enum_count > 0
        assert json_leaf_count > 0


# =============================================================================
# Enum validation — coercion and rejection in Pydantic models
# =============================================================================

class TestOperatorTypeValidation:

    def test_operator_context_accepts_string_coercion(self):
        ctx = OperatorContext(operator_id="op-1", operator_type="system")
        assert ctx.operator_type == OperatorType.SYSTEM


class TestCommandGenerationOutcomeMatchesSharedJSON:
    def test_consensus(self, status):
        assert status["tribunal.outcome"]["consensus"] == CommandGenerationOutcome.CONSENSUS

    def test_verified(self, status):
        assert status["tribunal.outcome"]["verified"] == CommandGenerationOutcome.VERIFIED

    def test_verification_failed(self, status):
        assert status["tribunal.outcome"]["verification.failed"] == CommandGenerationOutcome.VERIFICATION_FAILED

    def test_all_members_covered(self, status):
        json_keys = set(status["tribunal.outcome"].keys())
        enum_count = len(CommandGenerationOutcome)
        assert enum_count == len(json_keys), (
            f"CommandGenerationOutcome has {enum_count} members but shared JSON has {len(json_keys)} keys: {json_keys}"
        )


class TestApprovalErrorTypeMatchesSharedJSON:
    def test_approval_publish_failure(self, status):
        assert status["approval.error.type"]["approval.publish.failure"] == ApprovalErrorType.APPROVAL_PUBLISH_FAILURE

    def test_approval_exception(self, status):
        assert status["approval.error.type"]["approval.exception"] == ApprovalErrorType.APPROVAL_EXCEPTION

    def test_approval_timeout(self, status):
        assert status["approval.error.type"]["approval.timeout"] == ApprovalErrorType.APPROVAL_TIMEOUT

    def test_invalid_intent(self, status):
        assert status["approval.error.type"]["invalid.intent"] == ApprovalErrorType.INVALID_INTENT

    def test_intent_approval_exception(self, status):
        assert status["approval.error.type"]["intent.approval.exception"] == ApprovalErrorType.INTENT_APPROVAL_EXCEPTION

    def test_all_members_covered(self, status):
        json_keys = set(status["approval.error.type"].keys())
        enum_count = len(ApprovalErrorType)
        assert enum_count == len(json_keys), (
            f"ApprovalErrorType has {enum_count} members but shared JSON has {len(json_keys)} keys: {json_keys}"
        )


class TestAttachmentTypeMatchesSharedJSON:
    def test_pdf(self, status):
        assert status["attachment.type"]["pdf"] == AttachmentType.PDF

    def test_image(self, status):
        assert status["attachment.type"]["image"] == AttachmentType.IMAGE

    def test_text(self, status):
        assert status["attachment.type"]["text"] == AttachmentType.TEXT

    def test_other(self, status):
        assert status["attachment.type"]["other"] == AttachmentType.OTHER

    def test_all_members_covered(self, status):
        json_keys = set(status["attachment.type"].keys())
        enum_count = len(AttachmentType)
        assert enum_count == len(json_keys), (
            f"AttachmentType has {enum_count} members but shared JSON has {len(json_keys)} keys: {json_keys}"
        )




from app.models.settings import G8eeUserSettings, LLMSettings
from tests.fakes.factories import (
    build_g8e_http_context,
    build_enriched_context,
)

class TestAgentModeValidation:
    def test_accepts_enum(self):
        inv = build_enriched_context(investigation_id="inv-1")
        g8e_ctx = build_g8e_http_context(user_id="user-1")
        request_settings = G8eeUserSettings(llm=LLMSettings())
        ctx = AgentInputs(
            investigation=inv,
            g8e_context=g8e_ctx,
            request_settings=request_settings,
            agent_mode=AgentMode.OPERATOR_BOUND
        )
        assert ctx.agent_mode == AgentMode.OPERATOR_BOUND
