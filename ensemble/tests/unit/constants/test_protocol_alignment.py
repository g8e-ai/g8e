# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""Protocol alignment tests for ensemble constants and models.

Verifies that the ensemble's re-exports and subclasses stay in sync with the
g8e protocol package (protocol/python/g8e/). The protocol package is the SSOT;
the ensemble must not hand-roll duplicates that drift from it. These tests fail
closed if the ensemble silently introduces a local copy where a protocol import
should be, or if a re-export breaks identity with the protocol symbol.
"""

import ast
import importlib
import json
from enum import Enum, StrEnum
from pathlib import Path

import pytest

import app.constants as app_constants
import app.models.base as app_base
import g8e.constants as g8e_constants
import g8e.enums as g8e_enums
from app.constants import generated_status as gs

pytestmark = pytest.mark.unit


# ---------------------------------------------------------------------------
# Enum re-exports (generated_status.py imports from g8e.enums)
# ---------------------------------------------------------------------------


def _g8e_enum_reexports() -> list[tuple[str, str, str]]:
    """Parse generated_status.py for every name imported from g8e.enums.

    Returns tuples of (g8e_attr_name, ensemble_alias, ensemble_attr_name).
    """
    src = Path(gs.__file__).read_text()
    tree = ast.parse(src)
    out: list[tuple[str, str, str]] = []
    for node in ast.walk(tree):
        if isinstance(node, ast.ImportFrom) and node.module == "g8e.enums":
            for alias in node.names:
                g8e_name = alias.name
                alias_name = alias.asname or g8e_name
                out.append((g8e_name, alias_name, alias_name))
    return out


class TestEnumReexportsAreProtocolIdentity:
    """Every enum re-exported from g8e.enums must be the same object (identity)."""

    @pytest.mark.parametrize(
        "g8e_name,alias_name,_ensemble_name",
        _g8e_enum_reexports(),
        ids=lambda v: v if isinstance(v, str) else "",
    )
    def test_enum_is_protocol_identity(self, g8e_name: str, alias_name: str, _ensemble_name: str):
        proto = getattr(g8e_enums, g8e_name)
        ens = getattr(gs, alias_name)
        assert ens is proto, (
            f"gs.{alias_name} is not g8e.enums.{g8e_name}; ensemble has a local copy"
        )

    def test_all_reexported_enums_are_enum_subclasses(self):
        for g8e_name, alias_name, _ in _g8e_enum_reexports():
            ens = getattr(gs, alias_name)
            assert issubclass(ens, Enum), f"gs.{alias_name} is not an Enum subclass"

    def test_reexport_count_matches_protocol_enums(self):
        reexports = _g8e_enum_reexports()
        assert len(reexports) >= 60, (
            f"Expected at least 60 enum re-exports from g8e.enums, got {len(reexports)}; "
            "if the protocol added enums the ensemble must re-export them"
        )


class TestEventTypeConsensusRename:
    """Verify the AI_TRIBUNAL_* → AI_CONSENSUS_* rename is fully applied.

    The protocol renamed all AI_TRIBUNAL_* members to AI_CONSENSUS_* and the
    underlying string values from g8e.v1.ai.tribunal.* to g8e.v1.ai.consensus.*.
    The ensemble must not reference the old prefix anywhere in its event values.
    """

    def test_no_tribunal_prefix_in_event_values(self):
        tribunal_values = [m.value for m in gs.EventType if "tribunal" in m.value]
        assert tribunal_values == [], (
            f"EventType still has tribunal-prefixed values: {tribunal_values}"
        )

    def test_consensus_prefix_present(self):
        consensus_values = [m.value for m in gs.EventType if "consensus" in m.value]
        assert len(consensus_values) >= 20, (
            f"Expected at least 20 consensus-prefixed EventType members, got {len(consensus_values)}"
        )

    def test_consensus_values_match_protocol(self):
        ens_values = {m.value for m in gs.EventType if "consensus" in m.value}
        proto_values = {m.value for m in g8e_enums.EventType if "consensus" in m.value}
        assert ens_values == proto_values, (
            f"Ensemble consensus EventType values differ from protocol: "
            f"ens-only={ens_values - proto_values}, proto-only={proto_values - ens_values}"
        )


# ---------------------------------------------------------------------------
# HTTP header re-exports (app.constants re-exports from g8e.constants)
# ---------------------------------------------------------------------------


HEADER_PAIRS = [
    ("AUTHORIZATION", "HTTP_AUTHORIZATION_HEADER"),
    ("ACCEPT", "HTTP_ACCEPT_HEADER"),
    ("ACCEPT_LANGUAGE", "HTTP_ACCEPT_LANGUAGE_HEADER"),
    ("ACCESS_CONTROL_ALLOW_CREDENTIALS", "HTTP_ACCESS_CONTROL_ALLOW_CREDENTIALS_HEADER"),
    ("ACCESS_CONTROL_ALLOW_ORIGIN", "HTTP_ACCESS_CONTROL_ALLOW_ORIGIN_HEADER"),
    ("ACCESS_CONTROL_REQUEST_HEADERS", "HTTP_ACCESS_CONTROL_REQUEST_HEADERS_HEADER"),
    ("ACCESS_CONTROL_REQUEST_METHOD", "HTTP_ACCESS_CONTROL_REQUEST_METHOD_HEADER"),
    ("CACHE_CONTROL", "HTTP_CACHE_CONTROL_HEADER"),
    ("CONTENT_LANGUAGE", "HTTP_CONTENT_LANGUAGE_HEADER"),
    ("CONTENT_TYPE", "HTTP_CONTENT_TYPE_HEADER"),
    ("COOKIE", "HTTP_COOKIE_HEADER"),
    ("EXECUTION_ID", "EXECUTION_ID_HEADER"),
    ("LAST_EVENT_ID", "HTTP_LAST_EVENT_ID_HEADER"),
    ("PRAGMA", "HTTP_PRAGMA_HEADER"),
    ("REQUESTED_WITH", "HTTP_REQUESTED_WITH_HEADER"),
    ("SET_COOKIE", "HTTP_SET_COOKIE_HEADER"),
    ("CLI_SESSION_ID", "CLI_SESSION_ID_HEADER"),
    ("WEB_SESSION_ID", "WEB_SESSION_ID_HEADER"),
    ("OPERATOR_ID", "OPERATOR_ID_HEADER"),
    ("CASE_ID", "CASE_ID_HEADER"),
    ("INVESTIGATION_ID", "INVESTIGATION_ID_HEADER"),
    ("TASK_ID", "TASK_ID_HEADER"),
    ("USER_ID", "USER_ID_HEADER"),
    ("ORGANIZATION_ID", "ORGANIZATION_ID_HEADER"),
    ("BOUND_OPERATORS", "BOUND_OPERATORS_HEADER"),
    ("SOURCE_COMPONENT", "COMPONENT_NAME_HEADER"),
    ("USER_AGENT", "HTTP_USER_AGENT_HEADER"),
    ("SYSTEM_FINGERPRINT", "HTTP_G8E_SYSTEM_FINGERPRINT_HEADER"),
    ("X_ACCEL_BUFFERING", "HTTP_ACCEL_BUFFERING_HEADER"),
    ("X_FORWARDED_FOR", "HTTP_X_FORWARDED_FOR_HEADER"),
    ("X_PROXY_ORGANIZATION_ID", "PROXY_ORGANIZATION_ID_HEADER"),
    ("X_PROXY_USER_ID", "PROXY_USER_ID_HEADER"),
]


class TestHttpHeaderReexports:
    """Every HTTP header re-exported in app.constants must match g8e.constants."""

    @pytest.mark.parametrize(
        "ensemble_name,protocol_name",
        HEADER_PAIRS,
        ids=[e for e, _ in HEADER_PAIRS],
    )
    def test_header_value_matches_protocol(self, ensemble_name: str, protocol_name: str):
        ens = getattr(app_constants, ensemble_name, None)
        proto = getattr(g8e_constants, protocol_name, None)
        assert ens is not None, f"app.constants.{ensemble_name} is missing"
        assert proto is not None, f"g8e.constants.{protocol_name} is missing"
        assert ens == proto, (
            f"app.constants.{ensemble_name}={ens!r} != g8e.constants.{protocol_name}={proto!r}"
        )

    def test_x_proxy_user_email_is_ensemble_local(self):
        """X_PROXY_USER_EMAIL is ensemble-local (protocol does not define it)."""
        assert not hasattr(g8e_constants, "PROXY_USER_EMAIL_HEADER"), (
            "g8e.constants now defines PROXY_USER_EMAIL_HEADER; the ensemble-local "
            "X_PROXY_USER_EMAIL should be replaced with a protocol re-export"
        )
        assert app_constants.X_PROXY_USER_EMAIL == "X-Proxy-User-Email"


# ---------------------------------------------------------------------------
# ComponentName and G8EE_COMPONENT
# ---------------------------------------------------------------------------


class TestComponentNameAlignment:
    """ComponentName comes from g8e.constants; G8EE_COMPONENT is ensemble-local."""

    def test_component_name_is_protocol_identity(self):
        assert app_constants.ComponentName is g8e_constants.ComponentName

    def test_component_name_members(self):
        names = {m.name for m in g8e_constants.ComponentName}
        assert names == {"CLIENT", "G8EO", "G8EO_GATEWAY"}

    def test_g8ee_component_is_local_string(self):
        """G8EE_COMPONENT is a local string constant, not a ComponentName member."""
        assert app_constants.G8EE_COMPONENT == "g8ee"
        protocol_values = {m.value for m in g8e_constants.ComponentName}
        assert app_constants.G8EE_COMPONENT not in protocol_values, (
            "G8EE_COMPONENT must not shadow a protocol ComponentName value"
        )


# ---------------------------------------------------------------------------
# Model base re-exports (app.models.base re-exports from g8e.models.base)
# ---------------------------------------------------------------------------


BASE_REEXPORT_PAIRS = [
    ("G8eBaseModel", "G8eBaseModel"),
    ("ConfigDict", "ConfigDict"),
    ("Field", "Field"),
    ("UTCDatetime", "UTCDatetime"),
    ("ValidationError", "ValidationError"),
    ("_to_iso_z", "_to_iso_z"),
    ("field_validator", "field_validator"),
    ("model_validator", "model_validator"),
]


class TestModelBaseReexports:
    """Model base symbols re-exported in app.models.base must be protocol identity."""

    @pytest.mark.parametrize(
        "ensemble_name,protocol_name",
        BASE_REEXPORT_PAIRS,
        ids=[e for e, _ in BASE_REEXPORT_PAIRS],
    )
    def test_base_symbol_is_protocol_identity(self, ensemble_name: str, protocol_name: str):
        import g8e.models.base as proto_base

        ens = getattr(app_base, ensemble_name)
        proto = getattr(proto_base, protocol_name)
        assert ens is proto, (
            f"app.models.base.{ensemble_name} is not g8e.models.base.{protocol_name}"
        )


# ---------------------------------------------------------------------------
# Model subclass relationships (ensemble extends protocol models)
# ---------------------------------------------------------------------------


class TestModelSubclassing:
    """Ensemble models that extend protocol models must subclass them."""

    def test_request_context_subclasses_protocol(self):
        from app.models.http_context import RequestContext
        from g8e.models.context import RequestContext as Proto

        assert issubclass(RequestContext, Proto)

    def test_chat_message_request_subclasses_protocol(self):
        from app.models.internal_api import ChatMessageRequest
        from g8e.models.internal_api import ChatMessageRequest as Proto

        assert issubclass(ChatMessageRequest, Proto)

    def test_chat_started_response_is_protocol(self):
        from app.models.internal_api import ChatStartedResponse
        from g8e.models.internal_api import ChatStartedResponse as Proto

        assert ChatStartedResponse is Proto

    def test_resource_creation_request_is_protocol(self):
        from app.models.internal_api import ResourceCreationRequest
        from g8e.models.internal_api import ResourceCreationRequest as Proto

        assert ResourceCreationRequest is Proto

    def test_session_event_wire_subclasses_protocol(self):
        from app.models.events import SessionEventWire
        from g8e.models.events import SessionEventWire as Proto

        assert issubclass(SessionEventWire, Proto)

    def test_background_event_wire_subclasses_protocol(self):
        from app.models.events import BackgroundEventWire
        from g8e.models.events import BackgroundEventWire as Proto

        assert issubclass(BackgroundEventWire, Proto)

    @pytest.mark.parametrize(
        "ensemble_name,protocol_path",
        [
            ("G8eeUserSettings", "g8e.models.settings:G8eeUserSettings"),
            ("LLMSettings", "g8e.models.settings:LLMSettings"),
            ("SearchSettings", "g8e.models.settings:SearchSettings"),
            ("BatchExecutionSettings", "g8e.models.settings:BatchExecutionSettings"),
            ("CommandValidationSettings", "g8e.models.settings:CommandValidationSettings"),
            ("EvalJudgeSettings", "g8e.models.settings:EvalJudgeSettings"),
        ],
        ids=["G8eeUserSettings", "LLMSettings", "SearchSettings",
             "BatchExecutionSettings", "CommandValidationSettings", "EvalJudgeSettings"],
    )
    def test_settings_model_subclasses_protocol(self, ensemble_name: str, protocol_path: str):
        from app.models import settings as ens_settings

        mod_path, cls_name = protocol_path.split(":")
        proto_mod = importlib.import_module(mod_path)
        proto_cls = getattr(proto_mod, cls_name)
        ens_cls = getattr(ens_settings, ensemble_name)
        assert issubclass(ens_cls, proto_cls), (
            f"{ensemble_name} must subclass {protocol_path}"
        )


# ---------------------------------------------------------------------------
# No hand-rolled enum duplicates in app.constants (non-generated_status)
# ---------------------------------------------------------------------------


class TestNoEnumDuplicates:
    """No ensemble-local enum (outside generated_status.py) may duplicate a
    protocol enum name. The ensemble must re-export, not redefine.
    """

    def test_no_ensemble_enum_shadows_protocol_enum(self):
        protocol_enum_names = {
            name for name in dir(g8e_enums)
            if isinstance(getattr(g8e_enums, name, None), type)
            and issubclass(getattr(g8e_enums, name), Enum)
        }
        # Scan app.constants submodules for Enum subclasses that shadow protocol names.
        # Uses importlib to check the actual base class (AST can't resolve bases).
        # A shadow is only a violation if the ensemble enum's members don't
        # include all protocol members with matching values — that indicates a
        # hand-rolled duplicate that can drift. A legitimate extension (built
        # from protocol values + extra ensemble-specific members) is allowed.
        constants_dir = Path(app_constants.__file__).parent
        duplicates = []
        for py_file in constants_dir.glob("*.py"):
            if py_file.name in ("generated_status.py", "__init__.py"):
                continue
            mod_name = f"app.constants.{py_file.stem}"
            try:
                mod = importlib.import_module(mod_name)
            except ImportError:
                continue
            for attr_name in dir(mod):
                if attr_name not in protocol_enum_names:
                    continue
                attr = getattr(mod, attr_name, None)
                if not (isinstance(attr, type) and issubclass(attr, Enum)):
                    continue
                if attr is getattr(g8e_enums, attr_name):
                    continue  # exact re-export, fine
                # Check if this is a legitimate extension: all protocol members
                # must be present with matching values.
                proto_enum = getattr(g8e_enums, attr_name)
                proto_members = {m.name: m.value for m in proto_enum}
                ens_members = {m.name: m.value for m in attr}
                if all(ens_members.get(k) == v for k, v in proto_members.items()):
                    continue  # legitimate extension, all protocol values match
                duplicates.append(f"{py_file.name}:{attr_name}")
        assert duplicates == [], (
            f"Ensemble constants redefine protocol enum names with drifting values: {duplicates}; "
            "re-export from g8e.enums or build from protocol values + ensemble-specific extras"
        )


# ---------------------------------------------------------------------------
# SSE event fixture file presence and shape
# ---------------------------------------------------------------------------

# Path to the protocol SSE fixtures file, resolved from this test file's
# location (ensemble/tests/unit/constants/) up to the workspace root.
_SSE_FIXTURES_PATH = (
    Path(__file__).resolve().parent.parent.parent.parent.parent
    / "protocol"
    / "test-fixtures"
    / "sse-events.json"
)

# Every event-type key the contract integration test suite expects. Kept in
# sync with required_event_types in
# ensemble/tests/integration/test_sse_event_contract_integration.py.
_REQUIRED_SSE_FIXTURE_KEYS = [
    "llm_chat_iteration_started",
    "text_chunk_received",
    "text_completed",
    "chat_iteration_failed",
    "g8e_web_search_requested",
    "g8e_web_search_completed",
    "g8e_web_search_failed",
    "port_check_requested",
    "port_check_completed",
    "port_check_failed",
    "citations_received",
    "operator_command_requested",
    "operator_command_started",
    "operator_command_completed",
    "operator_command_failed",
    "llm_lifecycle_started",
    "llm_lifecycle_completed",
    "platform_sse_connection_established",
    "platform_sse_keepalive_sent",
    "tribunal_voting_consensus_failed",
    "tribunal_voting_dissent_recorded",
]


class TestSSEEventFixturesPresence:
    """The protocol SSE fixtures file must exist and parse.

    The contract integration tests in
    ensemble/tests/integration/test_sse_event_contract_integration.py load
    protocol/test-fixtures/sse-events.json at module import and call
    pytest.skip(allow_module_level=True) when it is absent. That skip is
    silent: the suite reports as passed rather than failed, hiding drift
    between the ensemble's emitted events and the protocol contract. These
    Tier 1 guards fail closed if the fixture file is missing, unparseable,
    or incomplete, so a deletion or schema regression is caught immediately
    instead of being masked by the integration-tier skip.
    """

    def test_sse_fixtures_file_exists(self):
        assert _SSE_FIXTURES_PATH.exists(), (
            f"Protocol SSE fixtures file not found at {_SSE_FIXTURES_PATH}. "
            "The contract integration tests will silently skip without it."
        )

    def test_sse_fixtures_file_parses_as_json(self):
        assert _SSE_FIXTURES_PATH.exists(), (
            f"Protocol SSE fixtures file not found at {_SSE_FIXTURES_PATH}"
        )
        with _SSE_FIXTURES_PATH.open() as f:
            data = json.load(f)
        assert isinstance(data, dict), "SSE fixtures root must be a JSON object"

    @pytest.mark.parametrize("key", _REQUIRED_SSE_FIXTURE_KEYS)
    def test_required_fixture_key_present(self, key: str):
        with _SSE_FIXTURES_PATH.open() as f:
            data = json.load(f)
        assert key in data, (
            f"Missing required SSE fixture key: {key}. "
            "Update protocol/test-fixtures/sse-events.json to include it."
        )

    @pytest.mark.parametrize("key", _REQUIRED_SSE_FIXTURE_KEYS)
    def test_fixture_entry_has_type_and_data(self, key: str):
        with _SSE_FIXTURES_PATH.open() as f:
            data = json.load(f)
        entry = data[key]
        assert "type" in entry, f"Fixture {key} missing 'type' field"
        assert isinstance(entry["type"], str), f"Fixture {key} 'type' must be a string"
        assert "data" in entry, f"Fixture {key} missing 'data' field"
        assert isinstance(entry["data"], dict), f"Fixture {key} 'data' must be an object"

    def test_fixture_type_strings_match_event_type_constants(self):
        """Every fixture 'type' string must equal the canonical EventType value."""
        fixture_to_constant = {
            "llm_chat_iteration_started": gs.EventType.AI_LLM_CHAT_ITERATION_STARTED,
            "text_chunk_received": gs.EventType.AI_LLM_CHAT_ITERATION_TEXT_CHUNK_RECEIVED,
            "text_completed": gs.EventType.AI_LLM_CHAT_ITERATION_TEXT_COMPLETED,
            "chat_iteration_failed": gs.EventType.AI_LLM_CHAT_ITERATION_FAILED,
            "g8e_web_search_requested": gs.EventType.AI_LLM_TOOL_G8E_WEB_SEARCH_REQUESTED,
            "g8e_web_search_completed": gs.EventType.AI_LLM_TOOL_G8E_WEB_SEARCH_COMPLETED,
            "g8e_web_search_failed": gs.EventType.AI_LLM_TOOL_G8E_WEB_SEARCH_FAILED,
            "port_check_requested": gs.EventType.OPERATOR_NETWORK_PORT_CHECK_REQUESTED,
            "port_check_completed": gs.EventType.OPERATOR_NETWORK_PORT_CHECK_COMPLETED,
            "port_check_failed": gs.EventType.OPERATOR_NETWORK_PORT_CHECK_FAILED,
            "citations_received": gs.EventType.AI_LLM_CHAT_ITERATION_CITATIONS_RECEIVED,
            "operator_command_requested": gs.EventType.OPERATOR_COMMAND_REQUESTED,
            "operator_command_started": gs.EventType.OPERATOR_COMMAND_STARTED,
            "operator_command_completed": gs.EventType.OPERATOR_COMMAND_COMPLETED,
            "operator_command_failed": gs.EventType.OPERATOR_COMMAND_FAILED,
            "llm_lifecycle_started": gs.EventType.AI_LLM_LIFECYCLE_STARTED,
            "llm_lifecycle_completed": gs.EventType.AI_LLM_LIFECYCLE_COMPLETED,
            "platform_sse_connection_established": gs.EventType.PLATFORM_SSE_CONNECTION_ESTABLISHED,
            "platform_sse_keepalive_sent": gs.EventType.PLATFORM_SSE_KEEPALIVE_SENT,
            "tribunal_voting_consensus_failed": gs.EventType.AI_CONSENSUS_VOTING_CONSENSUS_FAILED,
            "tribunal_voting_dissent_recorded": gs.EventType.AI_CONSENSUS_VOTING_DISSENT_RECORDED,
        }
        with _SSE_FIXTURES_PATH.open() as f:
            data = json.load(f)
        mismatches = []
        for key, expected_constant in fixture_to_constant.items():
            actual = data[key]["type"]
            if actual != expected_constant.value:
                mismatches.append(
                    f"{key}: fixture={actual!r} != EventType={expected_constant.value!r}"
                )
        assert mismatches == [], (
            "SSE fixture 'type' strings drifted from EventType constants: "
            + "; ".join(mismatches)
        )
