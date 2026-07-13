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

    def test_g8ee_value(self):
        assert ComponentName.G8EE == "g8ee"

    def test_g8eo_value(self):
        assert ComponentName.G8EO == "g8eo"

    def test_operator_alias_equals_g8eo(self):
        assert ComponentName.OPERATOR == ComponentName.G8EO


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
