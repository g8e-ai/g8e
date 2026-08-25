# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""Regression tests for Phase 14 — Architectural alignment audit.

Verifies:
- ``GovernanceClient`` uses ``GatewayAPIPaths.GOVERNANCE_ENVELOPES`` (no hardcoded URL)
- ``GovernanceClient`` passes mTLS cert paths to the HTTP session
- ``canonical_json`` produces sorted, no-whitespace output
- All business-critical data services route writes through governance envelopes

The dead ``PubSubGovernanceClient`` and its sibling ``*_pubsub.py`` client
variants were removed: they targeted a ``storage_type:operator_id:session_id``
routing scheme that does not exist on the operator side, were never imported
in production code, and bypassed the gateway's governance pipeline. The
ensemble now publishes raw command intent to ``cmd:`` and lets the gateway
construct the governed envelope.
"""

import hashlib
import json

import pytest

from app.constants.api_paths import GatewayAPIPaths
from app.utils.ledger_hash import canonical_json

pytestmark = pytest.mark.unit


class TestGovernanceClientUsesGatewayAPIPaths:
    """Verify GovernanceClient uses GatewayAPIPaths constant, not hardcoded URL."""

    def test_governance_envelopes_path_is_constant(self):
        assert GatewayAPIPaths.GOVERNANCE_ENVELOPES == "/api/v1/governance/envelopes"

    def test_governance_client_source_uses_gateway_api_paths(self):
        import inspect

        from app.clients.governance_client import GovernanceClient

        source = inspect.getsource(GovernanceClient.submit_envelope)
        assert "GatewayAPIPaths.GOVERNANCE_ENVELOPES" in source
        assert "/api/v1/governance/envelopes" not in source or "GatewayAPIPaths" in source


class TestGovernanceClientMTLS:
    """Verify GovernanceClient passes mTLS cert paths to session."""

    def test_governance_client_accepts_tls_config(self):
        import inspect

        from app.clients.governance_client import GovernanceClient
        from app.models.settings import TLSConfig

        sig = inspect.signature(GovernanceClient.__init__)
        assert "tls_config" in sig.parameters
        annotation = str(sig.parameters["tls_config"].annotation)
        assert "TLSConfig" in annotation

    def test_governance_client_passes_certs_to_session(self):
        import inspect

        from app.clients.governance_client import GovernanceClient

        source = inspect.getsource(GovernanceClient._get_http_session)
        assert "ca_cert_path" in source
        assert "client_cert_path" in source
        assert "client_key_path" in source
        assert "create_component_http_session" in source


class TestCanonicalJson:
    """Verify canonical_json produces sorted, no-whitespace output."""

    def test_sorted_keys(self):
        result = canonical_json({"b": 1, "a": 2})
        assert result == b'{"a":2,"b":1}'

    def test_no_whitespace(self):
        result = canonical_json({"a": 1, "b": 2})
        assert b" " not in result

    def test_nested_sorted_keys(self):
        result = canonical_json({"outer": {"z": 1, "a": 2}})
        assert result == b'{"outer":{"a":2,"z":1}}'

    def test_utf8_preserved(self):
        result = canonical_json({"name": "café"})
        assert "café".encode("utf-8") in result


class TestDeadPubSubClientsRemoved:
    """Regression: the dead ``*_pubsub.py`` client variants must stay deleted.

    These modules targeted a ``storage_type:operator_id:session_id`` routing
    scheme that does not exist on the operator side, were never imported in
    production code, and bypassed the gateway's governance pipeline. They
    must not be reintroduced.
    """

    @pytest.mark.parametrize(
        "module_name",
        [
            "app.clients.governance_client_pubsub",
            "app.clients.db_client_pubsub",
            "app.clients.blob_client_pubsub",
            "app.clients.kv_cache_client_pubsub",
        ],
    )
    def test_dead_pubsub_client_module_is_not_importable(self, module_name):
        import importlib

        with pytest.raises(ModuleNotFoundError):
            importlib.import_module(module_name)

    def test_clients_init_does_not_export_dead_pubsub_clients(self):
        import app.clients as clients_pkg

        exports = set(clients_pkg.__all__)
        assert "PubSubGovernanceClient" not in exports
        assert "PubSubDBClient" not in exports
        assert "PubSubBlobClient" not in exports
        assert "PubSubKvCacheClient" not in exports
        assert not hasattr(clients_pkg, "PubSubGovernanceClient")
        assert not hasattr(clients_pkg, "PubSubDBClient")
        assert not hasattr(clients_pkg, "PubSubBlobClient")
        assert not hasattr(clients_pkg, "PubSubKvCacheClient")


class TestPubSubClientDoesNotConstructEnvelopes:
    """Regression: PubSubClient must not construct governance envelopes.

    The gateway is the relay and enforcement point for operator command
    dispatch. The ensemble publishes a ``CommandIntent`` (protojson) to
    ``cmd:`` and the gateway intercepts, validates authorization, decodes
    the ``CommandIntent``, constructs the governed GovernanceEnvelope with
    the current state Merkle root, and forwards it to the operator. The
    ensemble must not import or call ``build_uap_envelope_json`` and must
    not expose ``check_operator_online`` or ``publish_storage_request``.
    """

    def test_pubsub_client_does_not_import_build_uap_envelope_json(self):
        import inspect

        import app.clients.pubsub_client as mod

        source = inspect.getsource(mod)
        assert "build_uap_envelope_json" not in source
        assert "build_uap_envelope" not in source

    def test_pubsub_client_has_no_check_operator_online(self):
        from app.clients.pubsub_client import PubSubClient

        assert not hasattr(PubSubClient, "check_operator_online")

    def test_pubsub_client_has_no_publish_storage_request(self):
        from app.clients.pubsub_client import PubSubClient

        assert not hasattr(PubSubClient, "publish_storage_request")

    def test_pubsub_client_sources_command_intent_from_g8e_models(self):
        """PubSubClient must source CommandIntent from g8e.models.governance."""
        import inspect

        import app.clients.pubsub_client as mod

        source = inspect.getsource(mod)
        assert "from g8e.models.governance import CommandIntent" in source


class TestDeadAbstractionsRemoved:
    """Regression: ``app.models.uap`` and ``app.utils.envelope_builder`` must stay deleted.

    These modules were removed in the Command Intent Protocol refactor.
    Envelope construction for the HTTP governance path now lives in
    ``app.clients.governance_client`` (using ``g8e.models.governance``
    directly), and inbound result decoding lives in
    ``app.utils.result_decoder``. They must not be reintroduced.
    """

    def test_uap_module_is_not_importable(self):
        import importlib

        with pytest.raises(ModuleNotFoundError):
            importlib.import_module("app.models.uap")

    def test_envelope_builder_module_is_not_importable(self):
        import importlib

        with pytest.raises(ModuleNotFoundError):
            importlib.import_module("app.utils.envelope_builder")

    def test_command_intent_sourced_from_g8e_models_governance(self):
        """CommandIntent must be sourced from g8e.models.governance, not redefined locally."""
        from g8e.models.governance import CommandIntent

        assert CommandIntent is not None
        assert hasattr(CommandIntent, "from_payload_bytes")
        assert hasattr(CommandIntent, "payload_bytes")


class TestDataServicesUseGovernanceEnvelopes:
    """Verify business-critical data services route writes through governance."""

    def test_case_data_service_uses_governance(self):
        import inspect

        from app.services.data.case_data_service import CaseDataService

        source = inspect.getsource(CaseDataService)
        assert "governance_client" in source
        assert "submit_envelope" in source
        assert "update_governed_doc" in source
        assert "delete_governed_doc" in source

    def test_investigation_data_service_uses_governance(self):
        import inspect

        from app.services.investigation.investigation_data_service import (
            InvestigationDataService,
        )

        source = inspect.getsource(InvestigationDataService)
        assert "governance_client" in source
        assert "submit_envelope" in source
        assert "update_governed_doc" in source
        assert "delete_governed_doc" in source

    def test_memory_data_service_uses_governance(self):
        import inspect

        from app.services.investigation.memory_data_service import MemoryDataService

        source = inspect.getsource(MemoryDataService)
        assert "governance_client" in source
        assert "submit_envelope" in source
        assert "update_governed_doc" in source

    def test_reputation_data_service_uses_governance(self):
        import inspect

        from app.services.data.reputation_data_service import ReputationDataService

        source = inspect.getsource(ReputationDataService)
        assert "governance_client" in source
        assert "update_governed_doc" in source
