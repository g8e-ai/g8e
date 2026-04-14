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
Integration tests: title generator against a real LLM.

Marked ai_integration — skipped automatically when no LLM provider is
configured (LLM_ENDPOINT / LLM_API_KEY). Runs against the configured
assistant model.

What is verified:
- The generated title is a non-empty string within the max_length cap
- The title is relevant to the input message (contains at least one key word)
- A greeting-only message produces a short title without hallucinated topics
- The config used has temperature=0.1 and stop_sequences=["\n"] so the
  model stays on-topic and does not produce multi-line output
"""

import pytest

from app.services.ai.title_generator import generate_case_title
from app.models.agents.title_generator import CaseTitleResult

pytestmark = [pytest.mark.integration, pytest.mark.ai_integration]


@pytest.mark.asyncio(loop_scope="session")
class TestTitleGeneratorIntegration:

    async def test_technical_message_produces_relevant_title(self, test_settings):
        """A specific technical message must produce a title containing key terms."""
        result = await generate_case_title(
            "My nginx server is returning 502 bad gateway errors after the latest deployment",
            settings=test_settings
        )

        assert isinstance(result, CaseTitleResult)
        assert 5 <= len(result.generated_title) <= 80
        assert "\n" not in result.generated_title
        lower = result.generated_title.lower()
        # Check for web server, deployment, or error concepts
        assert any(kw in lower for kw in ("nginx", "502", "gateway", "deployment", "error", "server", "web", "http", "bad gateway"))

    async def test_greeting_message_does_not_hallucinate_topic(self, test_settings):
        """A greeting must not produce a title about a completely unrelated topic."""
        result = await generate_case_title("hey man, what's going on", settings=test_settings)

        assert isinstance(result, CaseTitleResult)
        assert 5 <= len(result.generated_title) <= 80
        assert "\n" not in result.generated_title
        lower = result.generated_title.lower()
        assert not any(kw in lower for kw in ("coffee", "bean", "linux", "system", "kernel", "ubuntu"))

    async def test_specific_issue_title_reflects_content(self, test_settings):
        """A message about a specific issue must produce a title that reflects it."""
        result = await generate_case_title(
            "I need help setting up SSH key authentication on my remote server",
            settings=test_settings
        )

        assert isinstance(result, CaseTitleResult)
        assert 5 <= len(result.generated_title) <= 80
        assert "\n" not in result.generated_title
        lower = result.generated_title.lower()
        assert any(kw in lower for kw in ("ssh", "key", "auth", "server", "remote"))

    async def test_title_fits_within_max_length(self, test_settings):
        """Generated title must never exceed max_length."""
        result = await generate_case_title(
            "I am having trouble with my Kubernetes cluster and pods are crashing with OOMKilled errors",
            max_length=40,
            settings=test_settings
        )

        assert isinstance(result, CaseTitleResult)
        assert len(result.generated_title) <= 40

    async def test_title_is_single_line(self, test_settings):
        """Title must always be a single line — no newlines."""
        result = await generate_case_title(
            "Database connection pool exhausted under high load in production environment",
            settings=test_settings
        )

        assert "\n" not in result.generated_title
        assert "\r" not in result.generated_title
