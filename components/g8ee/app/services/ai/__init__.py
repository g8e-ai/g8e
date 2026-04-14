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
AI Services

AI pipeline: chat pipeline, task management, request building,
response analysis, tool execution, streaming agent, memory,
and investigation context.
"""

from .agent import g8eEngine
from .chat_pipeline import ChatPipelineService
from .chat_task_manager import ChatTaskManager
from .generation_config_builder import AIGenerationConfigBuilder
from .grounding import AttachmentGroundingProvider, GroundingService, WebSearchProvider
from ..investigation.investigation_service import extract_all_operators_context, extract_system_context
from .request_builder import AIRequestBuilder
from .response_analyzer import AIResponseAnalyzer
from .title_generator import generate_case_title
from .tool_service import AIToolService

__all__ = [
    "AttachmentGroundingProvider",
    "ChatPipelineService",
    "ChatTaskManager",
    "g8eEngine",
    "extract_all_operators_context",
    "extract_system_context",
    "generate_case_title",
    "GroundingService",
    "AIGenerationConfigBuilder",
    "AIRequestBuilder",
    "AIResponseAnalyzer",
    "AIToolService",
    "WebSearchProvider",
]
