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

"""g8el LLM provider implementation.

g8el is the platform's llama.cpp inference server component for g8e.
It inherits from LlamaCppProvider to reuse the OpenAI-compatible logic.
"""

import logging

from .llama_cpp import LlamaCppProvider

logger = logging.getLogger(__name__)


class G8elProvider(LlamaCppProvider):
    """g8el provider (platform's llama.cpp instance).

    g8el is the llama.cpp inference server component for g8e, providing an
    OpenAI-compatible HTTP API. This implementation inherits from LlamaCppProvider.
    """

    @property
    def service_name(self) -> str:
        return "g8el"

    def __init__(self, endpoint: str, api_key: str):
        super().__init__(endpoint=endpoint, api_key=api_key)
        logger.info("g8el provider initialized: %s", endpoint)
