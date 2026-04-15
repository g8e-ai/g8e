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
g8ee Services

All g8ee-specific services that extend shared base services.

Modular AI Services (separation of concerns):
- ChatPipelineService: Chat context assembly and pipeline coordination
- BackgroundTaskManager: Asyncio task tracking and cancellation
- AIRequestBuilder: Builds request contents and generation config
- AIResponseAnalyzer: Analyzes AI responses (grounding, citations)
- AIToolService: Executes AI tool/tool calls
"""

from .ai.chat_pipeline import ChatPipelineService
from .ai.chat_task_manager import BackgroundTaskManager
from .ai.generation_config_builder import AIGenerationConfigBuilder
from .ai.request_builder import AIRequestBuilder
from .ai.response_analyzer import AIResponseAnalyzer
from .ai.tool_service import AIToolService
from .investigation.investigation_data_service import InvestigationDataService
from .infra.g8ed_event_service import EventService
from .protocols import EventServiceProtocol

__all__ = [
    "ChatPipelineService",
    "BackgroundTaskManager",
    "AIGenerationConfigBuilder",
    "AIRequestBuilder",
    "AIResponseAnalyzer",
    "AIToolService",
    "InvestigationDataService",
    "EventService",
    "EventServiceProtocol",
]
