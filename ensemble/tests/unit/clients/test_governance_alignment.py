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
- ``PubSubGovernanceClient.submit_envelope`` serializes dict correctly (not re-wrapping)
- All business-critical data services route writes through governance envelopes
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


class TestPubSubGovernanceClientSerialization:
    """Verify PubSubGovernanceClient.submit_envelope serializes dict correctly."""

    def test_submit_envelope_does_not_call_build_uap_envelope_json(self):
        import inspect

        from app.clients.governance_client_pubsub import PubSubGovernanceClient

        source = inspect.getsource(PubSubGovernanceClient.submit_envelope)
        assert "build_uap_envelope_json" not in source
        assert "json.dumps" in source

    def test_submit_envelope_accepts_dict(self):
        import inspect

        from app.clients.governance_client_pubsub import PubSubGovernanceClient

        sig = inspect.signature(PubSubGovernanceClient.submit_envelope)
        envelope_param = sig.parameters.get("envelope")
        assert envelope_param is not None
        assert envelope_param.annotation is dict or "dict" in str(envelope_param.annotation)


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
