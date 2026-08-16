"""Tests for g8e protocol constants loading and integrity."""

import pytest

from g8e.constants import (
    EVENTS,
    STATUS,
    MSG,
    COLLECTIONS,
    KV,
    CHANNELS,
    PUBSUB,
    INTENTS,
    PROMPTS,
    TIMESTAMP,
    HEADERS,
    DOCUMENT_IDS,
    PLATFORM,
    AGENTS,
    NETWORK,
    API_PATHS,
    ComponentName,
    HTTP_AUTHORIZATION_HEADER,
    HTTP_CONTENT_TYPE_HEADER,
    HTTP_USER_AGENT_HEADER,
    WEB_SESSION_ID_HEADER,
    CLI_SESSION_ID_HEADER,
    OPERATOR_ID_HEADER,
    collection,
    channel,
    intent,
    prompt,
    kv_key,
    kv_session_type,
)


class TestProtocolConstantsLoad:
    """Verify that all protocol constant dicts load successfully and are non-empty."""

    @pytest.mark.parametrize(
        "name,constant",
        [
            ("EVENTS", EVENTS),
            ("STATUS", STATUS),
            ("MSG", MSG),
            ("COLLECTIONS", COLLECTIONS),
            ("KV", KV),
            ("CHANNELS", CHANNELS),
            ("PUBSUB", PUBSUB),
            ("INTENTS", INTENTS),
            ("PROMPTS", PROMPTS),
            ("TIMESTAMP", TIMESTAMP),
            ("HEADERS", HEADERS),
            ("DOCUMENT_IDS", DOCUMENT_IDS),
            ("PLATFORM", PLATFORM),
            ("AGENTS", AGENTS),
            ("NETWORK", NETWORK),
            ("API_PATHS", API_PATHS),
        ],
    )
    def test_constant_dict_is_non_empty(self, name, constant):
        assert isinstance(constant, dict), f"{name} must be a dict"
        assert len(constant) > 0, f"{name} must not be empty"

    def test_events_has_events_key(self):
        assert "events" in EVENTS
        assert len(EVENTS["events"]) > 0

    def test_status_has_status_key(self):
        assert "status" in STATUS
        assert len(STATUS["status"]) > 0

    def test_collections_has_collections_key(self):
        assert "collections" in COLLECTIONS
        assert len(COLLECTIONS["collections"]) > 0


class TestEventConstants:
    """Verify event constant structure and values."""

    def test_events_have_value_field(self):
        for key, meta in EVENTS["events"].items():
            assert "value" in meta, f"Event '{key}' missing 'value' field"
            assert isinstance(meta["value"], str), f"Event '{key}' value must be string"
            assert meta["value"], f"Event '{key}' value must not be empty"

    def test_event_values_are_dotted_namespaces(self):
        for key, meta in EVENTS["events"].items():
            value = meta["value"]
            assert "." in value, f"Event '{key}' value '{value}' should be dotted namespace"

    def test_event_values_start_with_g8e_prefix(self):
        for key, meta in EVENTS["events"].items():
            value = meta["value"]
            assert value.startswith("g8e.v1."), (
                f"Event '{key}' value '{value}' should start with 'g8e.v1.'"
            )


class TestStatusConstants:
    """Verify status constant structure and values."""

    def test_status_categories_have_entries(self):
        for cat_name, cat_vals in STATUS["status"].items():
            assert len(cat_vals) > 0, f"Status category '{cat_name}' has no entries"

    def test_status_entries_have_value_and_python_const(self):
        for cat_name, cat_vals in STATUS["status"].items():
            for key, meta in cat_vals.items():
                assert "value" in meta, (
                    f"Status '{cat_name}/{key}' missing 'value' field"
                )
                assert "_python_const" in meta, (
                    f"Status '{cat_name}/{key}' missing '_python_const' field"
                )


class TestCollectionConstants:
    """Verify collection constant structure."""

    def test_collections_have_value_field(self):
        for key, meta in COLLECTIONS["collections"].items():
            assert "value" in meta, f"Collection '{key}' missing 'value' field"
            assert isinstance(meta["value"], str), f"Collection '{key}' value must be string"
            assert meta["value"], f"Collection '{key}' value must not be empty"


class TestComponentName:
    """Verify ComponentName enum."""

    def test_client_value(self):
        assert ComponentName.CLIENT == "client"

    def test_g8eo_value(self):
        assert ComponentName.G8EO == "g8eo"

    def test_g8eo_gateway_value(self):
        assert ComponentName.G8EO_GATEWAY == "g8eo-gateway"


class TestHttpHeaderConstants:
    """Verify HTTP header constants are well-formed."""

    @pytest.mark.parametrize(
        "header",
        [
            HTTP_AUTHORIZATION_HEADER,
            HTTP_CONTENT_TYPE_HEADER,
            HTTP_USER_AGENT_HEADER,
            WEB_SESSION_ID_HEADER,
            CLI_SESSION_ID_HEADER,
            OPERATOR_ID_HEADER,
        ],
    )
    def test_header_is_non_empty_string(self, header):
        assert isinstance(header, str)
        assert len(header) > 0

    def test_g8e_headers_have_x_prefix(self):
        g8e_headers = [
            WEB_SESSION_ID_HEADER,
            CLI_SESSION_ID_HEADER,
            OPERATOR_ID_HEADER,
        ]
        for h in g8e_headers:
            assert h.startswith("X-G8E-"), f"Header '{h}' should start with 'X-G8E-'"


class TestAccessorFunctions:
    """Verify constants accessor utility functions."""

    def test_collection_accessor(self):
        assert collection("cases") == "cases"
        assert collection("account_locks") == "account_locks"
        assert collection("users") == "users"

    def test_channel_accessor(self):
        assert channel("Governance") == "governance"
        assert channel("SseEvent") == "sse_event"

    def test_intent_accessor(self):
        assert intent("ApigatewayDiscovery") == "apigateway_discovery"
        assert intent("Ec2Discovery") == "ec2_discovery"

    def test_prompt_accessor(self):
        assert prompt("SectionSafety") == "safety"
        assert prompt("SectionIdentity") == "identity"
        assert prompt("SectionVaultMode") == "sentinel_mode"

    def test_kv_key_accessor_without_kwargs(self):
        result = kv_key("CachePrefix")
        assert result == "g8e"

    def test_kv_key_accessor_with_kwargs(self):
        result = kv_key("SessionWeb", **{"session.type": "web", "session.id": "abc"})
        assert result == "g8e:sessions:web:abc"

    def test_kv_key_accessor_with_dot_kwargs(self):
        result = kv_key("SessionWeb", **{"session.type": "operator", "session.id": "xyz"})
        assert result == "g8e:sessions:operator:xyz"

    def test_kv_key_accessor_session_operator(self):
        result = kv_key("SessionOperator", **{"operator.session.id": "abc"})
        assert result == "g8e:sessions:operator:abc"

    def test_api_paths_has_grant_intent(self):
        assert "grant_intent" in API_PATHS
        assert API_PATHS["grant_intent"] == "/api/v1/operators/{operator_id}/intents/grant"

    def test_api_paths_has_revoke_intent(self):
        assert "revoke_intent" in API_PATHS
        assert API_PATHS["revoke_intent"] == "/api/v1/operators/{operator_id}/intents/revoke"

    def test_kv_session_type_accessor(self):
        assert kv_session_type("Web") == "web"
        assert kv_session_type("Operator") == "operator"

    def test_collection_invalid_key_raises(self):
        with pytest.raises(KeyError):
            collection("nonexistent")

    def test_channel_invalid_key_raises(self):
        with pytest.raises(KeyError):
            channel("nonexistent")

    def test_kv_key_invalid_name_raises(self):
        with pytest.raises(KeyError):
            kv_key("nonexistent")
