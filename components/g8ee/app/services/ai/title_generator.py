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
Title Generator Utility

Generates concise, meaningful case titles from descriptions using the configured LLM provider.
Uses a lightweight model optimized for quick text generation tasks.
"""

import logging

from app.llm import get_llm_provider, Role
from app.models.settings import G8eeUserSettings
from app.models.agents.title_generator import CaseTitleResult
from app.llm.llm_types import Content, Part, AssistantLLMSettings

logger = logging.getLogger(__name__)


async def generate_case_title(
    description: str | None,
    max_length: int = 80,
    settings: G8eeUserSettings | None = None,
) -> CaseTitleResult:
    """
    Generate a concise case title from a description using the configured LLM.
    
    Args:
        description: The case description or initial message
        max_length: Maximum title length in characters (default: 80)
        settings: Optional Settings object
        
    Returns:
        CaseTitleResult containing generated title and fallback flag
    """
    if not description or not description.strip():
        return CaseTitleResult(generated_title="New Technical Support Case", fallback=True)

    if not settings:
        return CaseTitleResult(
            generated_title=_create_fallback_title(description, max_length), 
            fallback=True
        )

    try:
        async with get_llm_provider(settings.llm, is_assistant=True) as provider:
            model = settings.llm.resolved_assistant_model
            if not model:
                logger.warning("[TITLE-GEN] No assistant_model configured, using fallback title")
                return CaseTitleResult(
                    generated_title=_create_fallback_title(description, max_length),
                    fallback=True
                )

            prompt = f"""<task>
Generate a concise, specific title for this conversation.
</task>

<message>
{description}
</message>

<constraints>
- Output ONLY the title — a single complete sentence fragment, no trailing words cut off
- Short and specific (3-7 words), always fully formed and grammatically complete
- Describe the actual topic, not generic categories
- No quotes, no metadata, no explanations, no line breaks
- Base the title ONLY on the provided message content
</constraints>

Title:"""

            logger.info("[TITLE-GEN] Generating case title, description_length=%d, description=%s", len(description), description)

            assistant_settings = AssistantLLMSettings(
                temperature=None,
                max_output_tokens=None,
                stop_sequences=["\n"],
                system_instruction="",
            )
            response = await provider.generate_content_assistant(
                model=model,
                contents=[Content(role=Role.USER, parts=[Part(text=prompt)])],
                assistant_llm_settings=assistant_settings,
            )

            if not response or not response.text:
                logger.warning("[TITLE-GEN] No response from LLM, using fallback title")
                return CaseTitleResult(
                    generated_title=_create_fallback_title(description, max_length),
                    fallback=True
                )

            generated_title = response.text.strip()

            if generated_title.startswith('"') and generated_title.endswith('"'):
                generated_title = generated_title[1:-1]
            if generated_title.startswith("'") and generated_title.endswith("'"):
                generated_title = generated_title[1:-1]

            if len(generated_title) > max_length:
                generated_title = generated_title[:max_length-3] + "..."

            if not generated_title or len(generated_title.strip()) < 5:
                return CaseTitleResult(
                    generated_title=_create_fallback_title(description, max_length),
                    fallback=True
                )

            logger.info("[TITLE-GEN] Title generated: %s", generated_title)

            return CaseTitleResult(generated_title=generated_title, fallback=False)

    except Exception as e:
        logger.error("[TITLE-GEN] Failed to generate title: %s", e)
        return CaseTitleResult(
            generated_title=_create_fallback_title(description, max_length),
            fallback=True
        )


def _create_fallback_title(description: str, max_length: int) -> str:
    """
    Create a fallback title from the description by extracting the first line.
    
    Args:
        description: The case description
        max_length: Maximum title length
        
    Returns:
        Fallback title string
    """
    if not description or not description.strip():
        return "New Technical Support Case"

    first_line = description.split("\n")[0].strip()
    if not first_line:
        first_line = description.strip()

    prefixes_to_remove = ["hi", "hello", "hey", "i need help with", "can you help me with"]
    
    changed = True
    while changed:
        changed = False
        lower_line = first_line.lower()
        for prefix in prefixes_to_remove:
            if lower_line.startswith(prefix):
                first_line = first_line[len(prefix):].strip().lstrip(",:;-").strip()
                changed = True
                break

    if first_line:
        first_line = first_line[0].upper() + first_line[1:]

    if len(first_line) > max_length:
        first_line = first_line[:max_length-3] + "..."

    return first_line if first_line else "New Technical Support Case"
