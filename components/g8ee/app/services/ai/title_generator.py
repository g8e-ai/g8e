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

from app.constants import LLM_DEFAULT_TEMPERATURE, LLM_DEFAULT_MAX_OUTPUT_TOKENS
from app.llm import get_llm_provider, Role
from app.models.settings import G8eeUserSettings
from app.models.agents.title_generator import CaseTitleResult
from app.llm.llm_types import Content, Part, LiteLLMSettings
from app.utils.agent_persona_loader import get_agent_persona

logger = logging.getLogger(__name__)


async def generate_case_title(
    description: str,
    *,
    max_length: int = 80,
    settings: G8eeUserSettings,
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
        provider = get_llm_provider(settings.llm, is_assistant=True)
        model = settings.llm.resolved_assistant_model
        if not model:
            logger.warning("[TITLE-GEN] No assistant_model configured, using fallback title")
            return CaseTitleResult(
                generated_title=_create_fallback_title(description, max_length),
                fallback=True
            )

        persona = get_agent_persona("title_generator")
        prompt = f"{persona.get_system_prompt()}\n\n<message>\n{description}\n</message>\n\nTitle:"

        logger.info("[TITLE-GEN] Generating case title, description_length=%d, description=%s", len(description), description)

        from app.models.model_configs import get_model_config
        model_config = get_model_config(model)
        temperature = model_config.default_temperature if model_config and model_config.default_temperature is not None else LLM_DEFAULT_TEMPERATURE
        max_output_tokens = model_config.max_output_tokens if model_config and model_config.max_output_tokens is not None else LLM_DEFAULT_MAX_OUTPUT_TOKENS
        settings = LiteLLMSettings(
            temperature=temperature,
            max_output_tokens=max_output_tokens,
            top_p_nucleus_sampling=model_config.top_p,
            top_k_filtering=model_config.top_k,
            stop_sequences=model_config.stop_sequences,
            system_instructions="",
            response_format=None,
        )
        response = await provider.generate_content_lite(
            model=model,
            contents=[Content(role=Role.USER, parts=[Part.from_text(prompt)])],
            lite_llm_settings=settings,
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
