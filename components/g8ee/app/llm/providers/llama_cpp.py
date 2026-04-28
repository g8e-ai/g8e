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

"""llama.cpp LLM provider implementation.

llama.cpp server provides an OpenAI-compatible HTTP API, so this implementation
inherits from the OpenAI provider.
"""

import logging

from app.llm.providers.open_ai import OpenAIProvider

logger = logging.getLogger(__name__)


class LlamaCppProvider(OpenAIProvider):
    """llama.cpp provider using OpenAI-compatible API.

    llama.cpp server provides an OpenAI-compatible HTTP API, so we inherit from
    OpenAIProvider to reuse its implementation.
    """

    @property
    def service_name(self) -> str:
        return "llamacpp"

    def __init__(self, endpoint: str, api_key: str):
        super().__init__(endpoint=endpoint, api_key=api_key)
        logger.info("llama.cpp provider initialized: %s", endpoint)
