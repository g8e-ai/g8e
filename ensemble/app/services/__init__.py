# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""
g8ee Services

Business logic services for the application.

Service modules:
- ai/: AI-related services (chat pipeline, request builder, response analyzer, tool service)
- auth/: Authentication services
- cache/: Caching services
- data/: Data management services
- infra/: Infrastructure services (event service, etc.)
- investigation/: Investigation data services
- operator/: Operator-related services
- protocols.py: Service protocol definitions
- service_factory.py: Service factory for dependency injection
"""

from .ai.chat_pipeline import ChatPipelineService
from .ai.chat_task_manager import BackgroundTaskManager
from .ai.generation_config_builder import AIGenerationConfigBuilder
from .ai.request_builder import AIRequestBuilder
from .ai.response_analyzer import AIResponseAnalyzer
from .ai.tool_service import AIToolService
from .investigation.investigation_data_service import InvestigationDataService
from .infra.event_service import EventService
from .protocols import EventServiceProtocol

__all__ = [
    "AIGenerationConfigBuilder",
    "AIRequestBuilder",
    "AIResponseAnalyzer",
    "AIToolService",
    "BackgroundTaskManager",
    "ChatPipelineService",
    "EventService",
    "EventServiceProtocol",
    "InvestigationDataService",
]
