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
Integration tests: AI Context Gathering Deep Dive

These tests exercise the complete AI context gathering pipeline from raw HTTP
context through InvestigationService to the final EnrichedInvestigationContext
used by the AI agent. All tests use real services and infrastructure.

    Segment 1 — Basic investigation context resolution
      Resolve investigation by investigation_id and case_id with retry logic.

    Segment 2 — Memory context attachment
      Test memory attachment, missing memory handling, and memory data integrity.

    Segment 3 — Operator enrichment and context extraction
      Test bound operator resolution, system context extraction, and multi-operator scenarios.

    Segment 4 — Complete context assembly end-to-end
      Full pipeline from G8eHttpContext to EnrichedInvestigationContext with all components.

    Segment 5 — Error handling and edge cases
      Missing investigations, operator lookup failures, and partial context scenarios.

    Segment 6 — Context extraction for AI consumption
      Test extract_system_context and extract_all_operators_context functions.

Real code under test:
    InvestigationService (app/services/investigation/investigation_service.py)
    MemoryDataService (app/services/investigation/memory_data_service.py)
    OperatorDataService (app/services/operators/operator_data_service.py)
    CacheAsideService (app/services/cache/cache_aside.py)
    extract_system_context, extract_all_operators_context

All tests use real g8es and cache services — no mocks allowed per testing guidelines.
"""

import asyncio
import logging
import pytest
from datetime import datetime, UTC
import uuid

from app.constants import (
    CloudSubtype,
    OperatorStatus,
    OperatorType,
    InvestigationStatus,
    Priority,
)
from app.errors import ResourceNotFoundError
from app.models.http_context import BoundOperator
from app.models.investigations import (
    EnrichedInvestigationContext,
    InvestigationCreateRequest,
)
from app.models.memory import InvestigationMemory
from app.models.operators import (
    OperatorSystemInfo,
    SystemInfoOSDetails,
    SystemInfoUserDetails,
    SystemInfoMemoryDetails,
    SystemInfoDiskDetails,
    SystemInfoEnvironment,
)
from app.models.agent import OperatorContext
from app.services.investigation.investigation_service import (
    extract_system_context,
    extract_all_operators_context,
)
from tests.fakes.factories import (
    create_investigation_data,
    create_investigation_memory,
    create_investigation_request,
    build_production_operator_document,
    build_g8e_http_context,
)

pytestmark = [pytest.mark.integration]



# ---------------------------------------------------------------------------
# Segment 1 — Basic investigation context resolution
# ---------------------------------------------------------------------------

@pytest.mark.asyncio(loop_scope="session")
@pytest.mark.integration
class TestInvestigationContextResolution:
    """Test basic investigation context resolution with retry logic."""

    async def test_resolve_by_investigation_id_success(
        self, cache_aside_service, test_settings, all_services, cleanup
    ):
        """Happy path: resolve investigation by investigation_id."""
        # Setup services properly using real infrastructure
        service = all_services['investigation_service']
        investigation_data_service = all_services['investigation_data_service']

        # Create investigation data first to get the IDs
        investigation = create_investigation_data()
        
        # Create the actual investigation with the same IDs
        created_investigation = await investigation_data_service.create_investigation(
            InvestigationCreateRequest(
                case_id=investigation.case_id,
                user_id=investigation.user_id,
                case_title=investigation.case_title,
                case_description="Test case description",
            )
        )
        
        # Test - use the created investigation's ID
        result = await service.get_investigation_context(
            investigation_id=created_investigation.id,
            user_id=created_investigation.user_id
        )
        
        # Verify
        assert isinstance(result, EnrichedInvestigationContext)
        assert result.id == created_investigation.id
        assert result.case_id == created_investigation.case_id
        assert result.user_id == created_investigation.user_id
        assert result.case_title == created_investigation.case_title
        assert result.memory is None  # No memory attached yet
        assert result.operator_documents == []  # No operators enriched yet

    async def test_resolve_by_case_id_returns_latest(
        self, cache_aside_service, test_settings, all_services, cleanup
    ):
        """When resolving by case_id, return the most recently created investigation."""
        # Setup
        service = all_services['investigation_service']
        investigation_data_service = all_services['investigation_data_service']

        user_id = f"user-case-test-{uuid.uuid4().hex[:8]}"
        case_id = f"case-multi-test-{uuid.uuid4().hex[:8]}"  # Unique case_id for each run
        
        # Create multiple investigations using the factory
        inv1 = await investigation_data_service.create_investigation(
            create_investigation_request(
                case_id=case_id, 
                user_id=user_id, 
                case_title="First", 
                case_description="First case"
            )
        )
        await asyncio.sleep(0.01)  # Ensure different timestamps
        inv2 = await investigation_data_service.create_investigation(
            create_investigation_request(
                case_id=case_id, 
                user_id=user_id, 
                case_title="Second", 
                case_description="Second case"
            )
        )
        await asyncio.sleep(0.01)
        inv3 = await investigation_data_service.create_investigation(
            create_investigation_request(
                case_id=case_id, 
                user_id=user_id, 
                case_title="Third", 
                case_description="Third case"
            )
        )
        
        # Test
        result = await service.get_investigation_context(
            case_id=case_id,
            user_id=user_id
        )
        
        # Verify - should return the latest (inv3)
        assert isinstance(result, EnrichedInvestigationContext)
        # First check that we got the right investigation by ID
        assert result.id == inv3.id, f"Expected investigation {inv3.id} ({inv3.case_title}), got {result.id} ({result.case_title})"
        assert result.case_title == "Third"  # Should be the latest investigation
        # Verify it's actually the latest by checking the created_at timestamp
        assert result.created_at >= inv3.created_at

        # Cleanup
        cleanup.track_investigation(inv1.id)
        cleanup.track_investigation(inv2.id)
        cleanup.track_investigation(inv3.id)

    async def test_resolve_missing_investigation_raises_resource_not_found(
        self, cache_aside_service, test_settings, all_services, cleanup, unique_investigation_id, unique_user_id
    ):
        """Missing investigation_id raises ResourceNotFoundError after retries."""
        # Setup
        service = all_services['investigation_service']
        investigation_data_service = all_services['investigation_data_service']

        # Test & Verify
        with pytest.raises(ResourceNotFoundError) as exc_info:
            await service.get_investigation_context(
                investigation_id=unique_investigation_id,
                user_id=unique_user_id
            )
        
        assert "not found after" in str(exc_info.value)
        assert exc_info.value.resource_type == "investigation"
        assert exc_info.value.resource_id == unique_investigation_id

    async def test_resolve_case_id_no_investigations_raises_resource_not_found(
        self, cache_aside_service, test_settings, all_services, cleanup, unique_case_id, unique_user_id
    ):
        """Case ID with no investigations raises ResourceNotFoundError."""
        # Setup
        service = all_services['investigation_service']


        # Test & Verify
        with pytest.raises(ResourceNotFoundError) as exc_info:
            await service.get_investigation_context(
                case_id=unique_case_id,
                user_id=unique_user_id
            )
        
        assert "No investigations found" in str(exc_info.value)
        assert exc_info.value.resource_type == "investigation"

    async def test_resolve_without_user_id_logs_security_warning(
        self, cache_aside_service, test_settings, all_services, cleanup, caplog
    ):
        """Calling without user_id logs a security warning but still works."""
        # Setup
        service = all_services['investigation_service']
        investigation_data_service = all_services['investigation_data_service']

        # Create investigation data first to get the IDs
        investigation = create_investigation_data()
        
        # Create the actual investigation with the same IDs
        created_investigation = await investigation_data_service.create_investigation(
            InvestigationCreateRequest(
                case_id=investigation.case_id,
                user_id=investigation.user_id,
                case_title=investigation.case_title,
                case_description="Test case description",
            )
        )
        
        # Test (call without user_id for case-based lookup)
        with caplog.at_level(logging.WARNING):
            result = await service.get_investigation_context(
                case_id=created_investigation.case_id
                # Note: no user_id provided
            )
        
        # Verify
        assert isinstance(result, EnrichedInvestigationContext)
        assert result.case_id == created_investigation.case_id
        
        # Check security warning was logged
        security_warnings = [
            record for record in caplog.records
            if getattr(record, 'security', None) == "unscoped_query"
        ]
        assert len(security_warnings) > 0

        # Cleanup
        cleanup.track_investigation(created_investigation.id)


# ---------------------------------------------------------------------------
# Segment 2 — Memory context attachment
# ---------------------------------------------------------------------------

@pytest.mark.asyncio(loop_scope="session")
@pytest.mark.integration
class TestMemoryContextAttachment:
    """Test memory context attachment to investigations."""

    async def test_attach_existing_memory_to_investigation(
        self, cache_aside_service, test_settings, all_services, cleanup
    ):
        """Memory is successfully attached when it exists."""
        # Setup services properly using real infrastructure
        service = all_services['investigation_service']
        investigation_data_service = all_services['investigation_data_service']
        memory_data_service = all_services['memory_data_service']

        # Create investigation data first to get the IDs
        investigation = create_investigation_data()
        
        # Create the actual investigation with the same IDs
        created_investigation = await investigation_data_service.create_investigation(
            InvestigationCreateRequest(
                case_id=investigation.case_id,
                user_id=investigation.user_id,
                case_title=investigation.case_title,
                case_description="Test case description",
            )
        )
        
        # Create memory with the actual investigation ID
        memory = create_investigation_memory(investigation_id=created_investigation.id)
        await memory_data_service.save_memory(memory, is_new=True)
        
        # Test
        result = await service.get_investigation_context(
            investigation_id=created_investigation.id,
            user_id=created_investigation.user_id
        )
        
        # Verify
        assert isinstance(result, EnrichedInvestigationContext)
        assert result.memory is not None
        assert result.memory.investigation_id == created_investigation.id
        assert result.memory.investigation_summary == memory.investigation_summary
        assert result.memory.communication_preferences == memory.communication_preferences
        assert result.memory.technical_background == memory.technical_background

        # Cleanup
        cleanup.track_investigation(created_investigation.id)
        cleanup.track_memory(created_investigation.id)

    async def test_missing_memory_handled_gracefully(
        self, cache_aside_service, test_settings, all_services, cleanup
    ):
        """Investigation context works normally when no memory exists."""
        # Setup services properly using real infrastructure
        service = all_services['investigation_service']
        investigation_data_service = all_services['investigation_data_service']

        # Create investigation data first to get the IDs
        investigation = create_investigation_data()
        
        # Create the actual investigation with the same IDs
        created_investigation = await investigation_data_service.create_investigation(
            InvestigationCreateRequest(
                case_id=investigation.case_id,
                user_id=investigation.user_id,
                case_title=investigation.case_title,
                case_description="Test case description",
            )
        )
        
        # Test
        result = await service.get_investigation_context(
            investigation_id=created_investigation.id,
            user_id=created_investigation.user_id
        )
        
        # Verify
        assert isinstance(result, EnrichedInvestigationContext)
        assert result.memory is None  # No memory should be attached
        # Investigation should still be fully populated
        assert result.id == created_investigation.id
        assert result.case_id == created_investigation.case_id

        # Cleanup
        cleanup.track_investigation(created_investigation.id)

    async def test_memory_data_service_none_skips_attachment(
        self, cache_aside_service, test_settings, all_services, cleanup
    ):
        """When no memory exists for investigation, memory is None without error."""
        # Setup services properly using real infrastructure
        service = all_services['investigation_service']
        investigation_data_service = all_services['investigation_data_service']

        # Create investigation data first to get the IDs
        investigation = create_investigation_data()

        # Create the actual investigation with the same IDs
        created_investigation = await investigation_data_service.create_investigation(
            InvestigationCreateRequest(
                case_id=investigation.case_id,
                user_id=investigation.user_id,
                case_title=investigation.case_title,
                case_description="Test case description",
            )
        )

        # Test
        result = await service.get_investigation_context(
            investigation_id=created_investigation.id,
            user_id=created_investigation.user_id
        )

        # Verify
        assert isinstance(result, EnrichedInvestigationContext)
        assert result.memory is None  # No memory exists for this investigation

        # Cleanup
        cleanup.track_investigation(created_investigation.id)

    async def test_memory_data_integrity_preserved(
        self, cache_aside_service, test_settings, all_services, cleanup
    ):
        """All memory fields are preserved correctly during attachment."""
        # Setup services properly using real infrastructure
        service = all_services['investigation_service']
        investigation_data_service = all_services['investigation_data_service']
        memory_data_service = all_services['memory_data_service']

        # Create investigation data first to get the IDs
        investigation = create_investigation_data()
        
        # Create the actual investigation with the same IDs
        created_investigation = await investigation_data_service.create_investigation(
            InvestigationCreateRequest(
                case_id=investigation.case_id,
                user_id=investigation.user_id,
                case_title=investigation.case_title,
                case_description="Test case description",
            )
        )
        
        memory = InvestigationMemory(
            investigation_id=created_investigation.id,
            case_id=created_investigation.case_id,
            user_id=created_investigation.user_id,
            case_title=created_investigation.case_title,
            investigation_summary="Complex investigation involving microservices debugging",
            communication_preferences="Detailed explanations with diagrams and step-by-step guides",
            technical_background="Cloud architect with Kubernetes and Docker expertise",
            response_style="Comprehensive with context and background information",
            problem_solving_approach="Holistic system thinking, considers dependencies",
            interaction_style="Collaborative, asks clarifying questions before proceeding",
            status=InvestigationStatus.OPEN,
            created_at=datetime.now(UTC),
            updated_at=datetime.now(UTC),
        )
        await memory_data_service.save_memory(memory, is_new=True)
        
        # Test
        result = await service.get_investigation_context(
            investigation_id=created_investigation.id,
            user_id=created_investigation.user_id
        )
        
        # Verify all fields are preserved
        assert result.memory is not None
        assert result.memory.investigation_summary == memory.investigation_summary
        assert result.memory.communication_preferences == memory.communication_preferences
        assert result.memory.technical_background == memory.technical_background
        assert result.memory.response_style == memory.response_style
        assert result.memory.problem_solving_approach == memory.problem_solving_approach
        assert result.memory.interaction_style == memory.interaction_style

        # Cleanup
        cleanup.track_investigation(created_investigation.id)
        cleanup.track_memory(created_investigation.id)


# ---------------------------------------------------------------------------
# Segment 3 — Operator enrichment and context extraction
# ---------------------------------------------------------------------------

@pytest.mark.asyncio(loop_scope="session")
@pytest.mark.integration
class TestOperatorEnrichment:
    """Test operator enrichment and context extraction for AI."""

    async def test_enrich_single_bound_operator(
        self, cache_aside_service, test_settings, all_services, cleanup
    ):
        """Single bound operator is enriched and context extracted."""
        # Setup services properly using real infrastructure
        service = all_services['investigation_service']
        investigation_data_service = all_services['investigation_data_service']
        operator_data_service = all_services['operator_data_service']
        memory_data_service = all_services['memory_data_service']
        
        # Create investigation and operator
        investigation = create_investigation_data()
        operator = build_production_operator_document()
        
        # Create the actual investigation with the same IDs
        created_investigation = await investigation_data_service.create_investigation(
            InvestigationCreateRequest(
                case_id=investigation.case_id,
                user_id=investigation.user_id,
                case_title=investigation.case_title,
                case_description="Test case description",
            )
        )
        await operator_data_service.create_operator(operator)
        
        # Create g8e context with bound operator
        bound_operator = BoundOperator(
            operator_id=operator.id,
            operator_session_id=operator.operator_session_id,
            status=OperatorStatus.BOUND,
        )
        g8e_context = build_g8e_http_context(
            bound_operators=[bound_operator]
        )
        
        # Get base context first
        base_context = await service.get_investigation_context(
            investigation_id=created_investigation.id,
            user_id=created_investigation.user_id
        )
        
        # Test enrichment
        enriched_context = await service.get_enriched_investigation_context(
            investigation=base_context,
            user_id=created_investigation.user_id,
            g8e_context=g8e_context
        )
        
        # Verify
        assert isinstance(enriched_context, EnrichedInvestigationContext)
        assert len(enriched_context.operator_documents) == 1
        assert enriched_context.operator_documents[0].operator_id == operator.id
        assert enriched_context.operator_documents[0].current_hostname == operator.current_hostname

        # Cleanup
        cleanup.track_operator(operator.id)
        cleanup.track_investigation(created_investigation.id)

    async def test_enrich_multiple_bound_operators(
        self, cache_aside_service, test_settings, all_services, cleanup
    ):
        """Multiple bound operators are all enriched."""
        # Setup services properly using real infrastructure
        service = all_services['investigation_service']
        investigation_data_service = all_services['investigation_data_service']
        operator_data_service = all_services['operator_data_service']
        memory_data_service = all_services['memory_data_service']
        
        # Create investigation and multiple operators
        investigation = create_investigation_data()
        operator1 = build_production_operator_document(hostname="host-1")
        operator2 = build_production_operator_document(hostname="host-2")
        operator3 = build_production_operator_document(hostname="host-3")
        
        # Create the actual investigation with the same IDs
        created_investigation = await investigation_data_service.create_investigation(
            InvestigationCreateRequest(
                case_id=investigation.case_id,
                user_id=investigation.user_id,
                case_title=investigation.case_title,
                case_description="Test case description",
            )
        )
        await operator_data_service.create_operator(operator1)
        await operator_data_service.create_operator(operator2)
        await operator_data_service.create_operator(operator3)
        
        # Create g8e context with multiple bound operators
        bound_operators = [
            BoundOperator(
                operator_id=op.operator_id,
                operator_session_id=op.operator_session_id,
                status=OperatorStatus.BOUND,
            )
            for op in [operator1, operator2, operator3]
        ]
        g8e_context = build_g8e_http_context(
            bound_operators=bound_operators
        )
        
        # Get base context and enrich
        base_context = await service.get_investigation_context(
            investigation_id=created_investigation.id,
            user_id=created_investigation.user_id
        )
        enriched_context = await service.get_enriched_investigation_context(
            investigation=base_context,
            user_id=created_investigation.user_id,
            g8e_context=g8e_context
        )
        
        # Verify
        assert len(enriched_context.operator_documents) == 3
        operator_ids = [op.operator_id for op in enriched_context.operator_documents]
        assert operator1.operator_id in operator_ids
        assert operator2.operator_id in operator_ids
        assert operator3.operator_id in operator_ids

        # Cleanup
        cleanup.track_operator(operator1.operator_id)
        cleanup.track_operator(operator2.operator_id)
        cleanup.track_operator(operator3.operator_id)
        cleanup.track_investigation(created_investigation.id)

    async def test_non_bound_operators_filtered_out(
        self, cache_aside_service, test_settings, all_services, cleanup
    ):
        """Only BOUND status operators are enriched."""
        # Setup services properly using real infrastructure
        service = all_services['investigation_service']
        investigation_data_service = all_services['investigation_data_service']
        operator_data_service = all_services['operator_data_service']
        memory_data_service = all_services['memory_data_service']
        
        # Create investigation and operators with different statuses
        investigation = create_investigation_data()
        bound_operator = build_production_operator_document()
        claimed_operator = build_production_operator_document()
        offline_operator = build_production_operator_document()
        
        # Manually set different statuses since factory hardcodes BOUND
        claimed_operator.status = OperatorStatus.AVAILABLE
        offline_operator.status = OperatorStatus.OFFLINE
        
        # Create the actual investigation with the same IDs
        created_investigation = await investigation_data_service.create_investigation(
            InvestigationCreateRequest(
                case_id=investigation.case_id,
                user_id=investigation.user_id,
                case_title=investigation.case_title,
                case_description="Test case description",
            )
        )
        await operator_data_service.create_operator(bound_operator)
        await operator_data_service.create_operator(claimed_operator)
        await operator_data_service.create_operator(offline_operator)
        
        # Create g8e context with mixed status operators
        bound_operators = [
            BoundOperator(
                operator_id=op.operator_id,
                operator_session_id=op.operator_session_id,
                status=op.status,
            )
            for op in [bound_operator, claimed_operator, offline_operator]
        ]
        g8e_context = build_g8e_http_context(
            bound_operators=bound_operators
        )
        
        # Get base context and enrich
        base_context = await service.get_investigation_context(
            investigation_id=created_investigation.id,
            user_id=created_investigation.user_id
        )
        enriched_context = await service.get_enriched_investigation_context(
            investigation=base_context,
            user_id=created_investigation.user_id,
            g8e_context=g8e_context
        )
        
        # Verify - only BOUND operator should be enriched
        assert len(enriched_context.operator_documents) == 1
        assert enriched_context.operator_documents[0].operator_id == bound_operator.id

        # Cleanup
        cleanup.track_operator(bound_operator.id)
        cleanup.track_operator(claimed_operator.id)
        cleanup.track_operator(offline_operator.id)
        cleanup.track_investigation(created_investigation.id)

    async def test_missing_operator_handled_gracefully(
        self, cache_aside_service, test_settings, all_services, cleanup, unique_operator_id, unique_session_id
    ):
        """Bound operator not found in cache is handled gracefully."""
        # Setup services properly using real infrastructure
        service = all_services['investigation_service']
        investigation_data_service = all_services['investigation_data_service']

        # Create investigation but don't create operator
        investigation = create_investigation_data()
        created_investigation = await investigation_data_service.create_investigation(
            InvestigationCreateRequest(
                case_id=investigation.case_id,
                user_id=investigation.user_id,
                case_title=investigation.case_title,
                case_description="Test case description",
            )
        )
        
        # Create g8e context with non-existent operator
        bound_operator = BoundOperator(
            operator_id=unique_operator_id,
            operator_session_id=unique_session_id,
            status=OperatorStatus.BOUND,
        )
        g8e_context = build_g8e_http_context(
            bound_operators=[bound_operator]
        )
        
        # Get base context and enrich
        base_context = await service.get_investigation_context(
            investigation_id=created_investigation.id,
            user_id=created_investigation.user_id
        )
        enriched_context = await service.get_enriched_investigation_context(
            investigation=base_context,
            user_id=created_investigation.user_id,
            g8e_context=g8e_context
        )
        
        # Verify - no operators should be enriched
        assert len(enriched_context.operator_documents) == 0

        # Cleanup
        cleanup.track_investigation(created_investigation.id)

    async def test_cloud_operator_context_extraction(
        self, cache_aside_service, test_settings, all_services, cleanup
    ):
        """Cloud operator context includes cloud-specific fields."""
        # Setup services properly using real infrastructure
        service = all_services['investigation_service']
        investigation_data_service = all_services['investigation_data_service']
        operator_data_service = all_services['operator_data_service']

        # Create cloud operator with intents
        cloud_operator = build_production_operator_document(
            operator_type=OperatorType.CLOUD,
        )
        cloud_operator.cloud_subtype = CloudSubtype.AWS
        cloud_operator.granted_intents = ["ec2_discovery", "s3_read"]
        
        investigation = create_investigation_data()
        # Create the actual investigation with the same IDs
        created_investigation = await investigation_data_service.create_investigation(
            InvestigationCreateRequest(
                case_id=investigation.case_id,
                user_id=investigation.user_id,
                case_title=investigation.case_title,
                case_description="Test case description",
            )
        )
        await operator_data_service.create_operator(cloud_operator)
        
        # Create g8e context and enrich
        bound_operator = BoundOperator(
            operator_id=cloud_operator.id,
            operator_session_id=cloud_operator.operator_session_id,
            status=OperatorStatus.BOUND,
        )
        g8e_context = build_g8e_http_context(
            bound_operators=[bound_operator]
        )
        
        base_context = await service.get_investigation_context(
            investigation_id=created_investigation.id,
            user_id=created_investigation.user_id
        )
        enriched_context = await service.get_enriched_investigation_context(
            investigation=base_context,
            user_id=created_investigation.user_id,
            g8e_context=g8e_context
        )
        
        # Verify cloud-specific context
        assert len(enriched_context.operator_documents) == 1
        op_doc = enriched_context.operator_documents[0]
        assert op_doc.operator_type == OperatorType.CLOUD
        assert op_doc.cloud_subtype == "aws"
        assert op_doc.granted_intents == ["ec2_discovery", "s3_read"]

        # Cleanup
        cleanup.track_operator(cloud_operator.id)
        cleanup.track_investigation(created_investigation.id)


# ---------------------------------------------------------------------------
# Segment 4 — Complete context assembly end-to-end
# ---------------------------------------------------------------------------

@pytest.mark.asyncio(loop_scope="session")
@pytest.mark.integration
class TestCompleteContextAssembly:
    """Test complete context assembly from G8eHttpContext to final context."""

    async def test_full_pipeline_with_all_components(
        self, cache_aside_service, test_settings, all_services, cleanup
    ):
        """Complete pipeline: investigation + memory + operators + enrichment."""
        # Setup services properly using real infrastructure
        service = all_services['investigation_service']
        investigation_data_service = all_services['investigation_data_service']
        operator_data_service = all_services['operator_data_service']
        memory_data_service = all_services['memory_data_service']

        # Create complete test data
        investigation = create_investigation_data()
        
        # Create the actual investigation with the same IDs
        created_investigation = await investigation_data_service.create_investigation(
            InvestigationCreateRequest(
                case_id=investigation.case_id,
                user_id=investigation.user_id,
                case_title=investigation.case_title,
                case_description="Test case description",
            )
        )
        
        memory = InvestigationMemory(
            investigation_id=created_investigation.id,
            case_id=created_investigation.case_id,
            user_id=created_investigation.user_id,
            status=InvestigationStatus.OPEN,
            case_title=created_investigation.case_title,
            investigation_summary="Complex investigation involving microservices debugging",
            communication_preferences="Detailed explanations with diagrams and step-by-step guides",
            technical_background="Cloud architect with Kubernetes and Docker expertise",
            response_style="Comprehensive with context and background information",
            problem_solving_approach="Holistic system thinking, considers dependencies",
        )
        operator1 = build_production_operator_document(hostname="prod-server-01")
        operator2 = build_production_operator_document(hostname="prod-server-02")
        
        await memory_data_service.save_memory(memory, is_new=True)
        await operator_data_service.create_operator(operator1)
        await operator_data_service.create_operator(operator2)
        
        # Create g8e context with both operators
        bound_operators = [
            BoundOperator(
                operator_id=op.operator_id,
                operator_session_id=op.operator_session_id,
                status=OperatorStatus.BOUND,
            )
            for op in [operator1, operator2]
        ]
        g8e_context = build_g8e_http_context(
            user_id=created_investigation.user_id,
            case_id=created_investigation.case_id,
            investigation_id=created_investigation.id,
            bound_operators=bound_operators,
        )
        
        # Execute full pipeline
        base_context = await service.get_investigation_context(
            investigation_id=created_investigation.id,
            user_id=created_investigation.user_id
        )
        enriched_context = await service.get_enriched_investigation_context(
            investigation=base_context,
            user_id=created_investigation.user_id,
            g8e_context=g8e_context
        )
        
        # Verify complete assembly
        assert isinstance(enriched_context, EnrichedInvestigationContext)

        # Investigation fields
        assert enriched_context.id == created_investigation.id
        assert enriched_context.case_id == created_investigation.case_id
        assert enriched_context.user_id == created_investigation.user_id
        assert enriched_context.case_title == created_investigation.case_title

        # Memory attached
        assert enriched_context.memory is not None
        assert enriched_context.memory.investigation_summary == memory.investigation_summary
        assert enriched_context.memory.communication_preferences == memory.communication_preferences

        # Operators enriched
        assert len(enriched_context.operator_documents) == 2
        operator_hostnames = {op.hostname for op in enriched_context.operator_documents}
        assert "prod-server-01" in operator_hostnames
        assert "prod-server-02" in operator_hostnames

        # Cleanup
        cleanup.track_operator(operator1.operator_id)
        cleanup.track_operator(operator2.operator_id)
        cleanup.track_memory(memory.investigation_id)
        cleanup.track_investigation(created_investigation.id)

    async def test_context_with_no_operators_or_memory(
        self, cache_aside_service, test_settings, all_services, cleanup
    ):
        """Context assembly works with minimal data (no operators, no memory)."""
        # Setup services properly using real infrastructure
        service = all_services['investigation_service']
        investigation_data_service = all_services['investigation_data_service']

        # Create only investigation
        investigation = create_investigation_data()
        created_investigation = await investigation_data_service.create_investigation(
            InvestigationCreateRequest(
                case_id=investigation.case_id,
                user_id=investigation.user_id,
                case_title=investigation.case_title,
                case_description="Test case description",
            )
        )
        
        # Create g8e context with no bound operators
        g8e_context = build_g8e_http_context(
            user_id=created_investigation.user_id,
            case_id=created_investigation.case_id,
            investigation_id=created_investigation.id,
            bound_operators=[],
        )
        
        # Execute pipeline
        base_context = await service.get_investigation_context(
            investigation_id=created_investigation.id,
            user_id=created_investigation.user_id
        )
        enriched_context = await service.get_enriched_investigation_context(
            investigation=base_context,
            user_id=created_investigation.user_id,
            g8e_context=g8e_context
        )
        
        # Verify minimal context
        assert isinstance(enriched_context, EnrichedInvestigationContext)
        assert enriched_context.id == created_investigation.id
        assert enriched_context.memory is None  # No memory
        assert len(enriched_context.operator_documents) == 0  # No operators

        # Cleanup
        cleanup.track_investigation(created_investigation.id)



# ---------------------------------------------------------------------------
# Segment 5 — Error handling and edge cases (real integration tests only)
# ---------------------------------------------------------------------------

@pytest.mark.asyncio(loop_scope="session")
@pytest.mark.integration
class TestContextGatheringErrorHandling:
    """Test error handling and edge cases in context gathering."""

    async def test_context_with_null_investigation_id(
        self, cache_aside_service, test_settings, all_services
    ):
        """Context with null investigation_id handles gracefully."""
        service = all_services['investigation_service']
        
        # Create context with minimal investigation_id (using default generated id)
        context = EnrichedInvestigationContext(
            case_id="case-null-id",
            user_id="user-null-id",
            case_title="Test Case",
            status=InvestigationStatus.OPEN,
            priority=Priority.MEDIUM,
            sentinel_mode=False,
            created_at=datetime.now(UTC),
            updated_at=datetime.now(UTC),
            history_trail=[],
            conversation_history=[],
        )
        
        # Test memory attachment (should skip gracefully)
        result = await service._attach_memory_context(context)
        
        # Verify
        assert result.id is not None  # Should have generated an ID
        assert result.memory is None  # No memory attached

    async def test_context_without_required_parameters(
        self, cache_aside_service, test_settings, all_services
    ):
        """get_investigation_context without required params raises error."""
        service = all_services['investigation_service']
        
        # Test with no parameters
        with pytest.raises(ResourceNotFoundError) as exc_info:
            await service.get_investigation_context()
        
        assert "Investigation context could not be resolved" in str(exc_info.value)


# ---------------------------------------------------------------------------
# Segment 6 — Context extraction for AI consumption
# ---------------------------------------------------------------------------

@pytest.mark.asyncio(loop_scope="session")
@pytest.mark.integration
class TestAIContextExtraction:
    """Test context extraction functions used by AI agents."""

    async def test_extract_system_context_single_operator(
        self, cache_aside_service
    ):
        """extract_system_context returns primary operator context."""
        # Create test operator with full system details and specific ID
        operator = build_production_operator_document(
            operator_id="op-extract-001",
            hostname="extract-host",
        )
        
        # Add comprehensive system info for the test
        operator.system_info = OperatorSystemInfo(
            hostname="extract-host",
            os="linux",
            architecture="x86_64",
            cpu_count=4,
            memory_mb=8192,
            public_ip="192.168.1.100",
            os_details=SystemInfoOSDetails(
                distro="Ubuntu",
                kernel="5.15.0",
                version="22.04",
            ),
            user_details=SystemInfoUserDetails(
                username="testuser",
                home="/home/testuser",
                shell="/bin/bash",
            ),
            environment=SystemInfoEnvironment(
                pwd="/home/testuser",
                timezone="UTC",
                is_container=False,
                init_system="systemd",
            ),
            memory_details=SystemInfoMemoryDetails(
                total_mb=8192,
                available_mb=4485,
                percent=45.2,
            ),
            disk_details=SystemInfoDiskDetails(
                total_gb=100.0,
                free_gb=74.5,
                percent=25.5,
            ),
            is_container=False,
            init_system="systemd",
        )
        
        investigation = EnrichedInvestigationContext(
            id="inv-extract-001",
            case_id="case-extract-001",
            user_id="user-extract-001",
            case_title="Extraction Test",
            status=InvestigationStatus.OPEN,
            priority=Priority.MEDIUM,
            sentinel_mode=False,
            created_at=datetime.now(UTC),
            updated_at=datetime.now(UTC),
            history_trail=[],
            conversation_history=[],
            operator_documents=[operator],  # Single operator
        )
        
        # Test extraction
        context = extract_system_context(investigation)
        
        # Verify all fields are extracted correctly
        assert isinstance(context, OperatorContext)
        assert context.operator_id == "op-extract-001"
        assert context.hostname == "extract-host"
        assert context.os == "linux"
        assert context.architecture == "x86_64"
        assert context.cpu_count == 4
        assert context.memory_mb == 8192
        assert context.public_ip == "192.168.1.100"
        assert context.operator_type == OperatorType.SYSTEM
        assert context.is_cloud_operator is False
        
        # Heartbeat-derived fields
        assert context.distro == "Ubuntu"
        assert context.kernel == "5.15.0"
        assert context.os_version == "22.04"
        assert context.username == "testuser"
        assert context.home_directory == "/home/testuser"
        assert context.shell == "/bin/bash"
        assert context.working_directory == "/home/testuser"
        assert context.timezone == "UTC"
        assert context.is_container is False
        assert context.init_system == "systemd"
        assert context.disk_percent == 25.5
        assert context.disk_total_gb == 100.0
        assert context.disk_free_gb == 74.5
        assert context.memory_percent == 45.2
        assert context.memory_total_mb == 8192
        assert context.memory_available_mb == 4485

    async def test_extract_system_context_no_operators(
        self, cache_aside_service
    ):
        """extract_system_context returns None with no operators."""
        investigation = EnrichedInvestigationContext(
            id="inv-no-ops",
            case_id="case-no-ops",
            user_id="user-no-ops",
            case_title="No Operators Test",
            status=InvestigationStatus.OPEN,
            priority=Priority.MEDIUM,
            sentinel_mode=False,
            created_at=datetime.now(UTC),
            updated_at=datetime.now(UTC),
            history_trail=[],
            conversation_history=[],
            operator_documents=[],  # No operators
        )
        
        # Test extraction
        context = extract_system_context(investigation)
        
        # Verify
        assert context is None

    async def test_extract_system_context_none_investigation(
        self, cache_aside_service
    ):
        """extract_system_context returns None with None investigation."""
        context = extract_system_context(None)
        assert context is None

    async def test_extract_all_operators_context_multiple(
        self, cache_aside_service
    ):
        """extract_all_operators_context returns all operator contexts."""
        # Create multiple operators with different characteristics and specific IDs
        linux_operator = build_production_operator_document(
            operator_id="op-linux-001",
            hostname="linux-server",
            operator_type=OperatorType.SYSTEM,
        )
        linux_operator.granted_intents = []

        cloud_operator = build_production_operator_document(
            operator_id="op-cloud-001",
            hostname="aws-instance",
            operator_type=OperatorType.CLOUD,
        )
        cloud_operator.cloud_subtype = CloudSubtype.AWS
        cloud_operator.granted_intents = ["ec2_discovery", "s3_read"]
        
        investigation = EnrichedInvestigationContext(
            id="inv-multi-ops",
            case_id="case-multi-ops",
            user_id="user-multi-ops",
            case_title="Multi-Operator Test",
            status=InvestigationStatus.OPEN,
            priority=Priority.MEDIUM,
            sentinel_mode=False,
            created_at=datetime.now(UTC),
            updated_at=datetime.now(UTC),
            history_trail=[],
            conversation_history=[],
            operator_documents=[linux_operator, cloud_operator],
        )
        
        # Test extraction
        contexts = extract_all_operators_context(investigation)
        
        # Verify
        assert contexts is not None
        assert len(contexts) == 2
        
        # Check first operator (Linux)
        linux_ctx = contexts[0]
        assert isinstance(linux_ctx, OperatorContext)
        assert linux_ctx.operator_id == "op-linux-001"
        assert linux_ctx.hostname == "linux-server"
        assert linux_ctx.operator_type == OperatorType.SYSTEM
        assert linux_ctx.is_cloud_operator is False
        assert linux_ctx.granted_intents == []
        
        # Check second operator (Cloud)
        cloud_ctx = contexts[1]
        assert isinstance(cloud_ctx, OperatorContext)
        assert cloud_ctx.operator_id == "op-cloud-001"
        assert cloud_ctx.hostname == "aws-instance"
        assert cloud_ctx.operator_type == OperatorType.CLOUD
        assert cloud_ctx.is_cloud_operator is True
        assert cloud_ctx.cloud_subtype == "aws"
        assert cloud_ctx.granted_intents == ["ec2_discovery", "s3_read"]

    async def test_extract_all_operators_context_none_investigation(
        self, cache_aside_service
    ):
        """extract_all_operators_context returns None with None investigation."""
        contexts = extract_all_operators_context(None)
        assert contexts is None

    async def test_extract_all_operators_context_empty_operators(
        self, cache_aside_service
    ):
        """extract_all_operators_context returns None with no operators."""
        investigation = EnrichedInvestigationContext(
            id="inv-empty-ops",
            case_id="case-empty-ops",
            user_id="user-empty-ops",
            case_title="Empty Operators Test",
            status=InvestigationStatus.OPEN,
            priority=Priority.MEDIUM,
            sentinel_mode=False,
            created_at=datetime.now(UTC),
            updated_at=datetime.now(UTC),
            history_trail=[],
            conversation_history=[],
            operator_documents=[],
        )
        
        contexts = extract_all_operators_context(investigation)
        assert contexts is None

    async def test_context_extraction_with_missing_heartbeat_data(
        self, cache_aside_service
    ):
        """Context extraction handles missing heartbeat data gracefully."""
        # Create operator with no heartbeat snapshot
        operator = build_production_operator_document(operator_id="op-no-hb")
        operator.latest_heartbeat_snapshot = None  # No heartbeat data
        
        investigation = EnrichedInvestigationContext(
            id="inv-no-hb",
            case_id="case-no-hb",
            user_id="user-no-hb",
            case_title="No Heartbeat Test",
            status=InvestigationStatus.OPEN,
            priority=Priority.MEDIUM,
            sentinel_mode=False,
            created_at=datetime.now(UTC),
            updated_at=datetime.now(UTC),
            history_trail=[],
            conversation_history=[],
            operator_documents=[operator],
        )
        
        # Test extraction
        context = extract_system_context(investigation)
        
        # Verify - system info should be available, even if heartbeat-derived fields are from system_info fallback
        assert isinstance(context, OperatorContext)
        assert context.operator_id == "op-no-hb"
        assert context.os == "linux"  # From system_info
        assert context.hostname == "eval-node-01"  # From system_info
        
        # Fields that exist in system_info fallback should NOT be None
        assert context.working_directory == "/root"  # Fallback from system_info
        assert context.username == "root"  # Fallback from system_info
        
        # Truly heartbeat-only fields (not in default system_info) should be None
        assert context.distro is None
        assert context.kernel is None
        assert context.disk_percent is None
        assert context.memory_percent is None
