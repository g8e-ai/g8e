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
Prompt File Loader Utility

Loads prompt content from text files to eliminate triple-quoted strings
in Python code. Prompts are organized in subdirectories:
- system/: System-level prompts for AI agent configuration
- tools/: Tool description prompts for tool declarations
"""

import logging
from functools import lru_cache
from pathlib import Path

from ..constants import AGENT_MODE_PROMPT_FILES, AgentMode, PromptFile, PromptSection
from ..errors import ResourceNotFoundError

logger = logging.getLogger(__name__)

PROMPTS_DIR = Path(__file__).parent


@lru_cache(maxsize=128)
def load_prompt(prompt_file: PromptFile) -> str:
    """Load prompt content from a PromptFile enum member.

    Args:
        prompt_file: PromptFile enum member.

    Returns:
        Prompt content as string.

    Raises:
        ResourceNotFoundError: If the prompt file does not exist.
        TypeError: If prompt_file is not a PromptFile instance.
    """
    if not isinstance(prompt_file, PromptFile):
        raise TypeError(f"Expected PromptFile enum member, got {type(prompt_file)}")

    file_path = PROMPTS_DIR / prompt_file.path

    if not file_path.exists():
        logger.error("Prompt file not found: %s", file_path)
        raise ResourceNotFoundError(
            f"Prompt file not found: {file_path}",
            resource_type="prompt_file",
            resource_id=prompt_file.name,
            component="vse",
        )

    try:
        with open(file_path, encoding="utf-8") as f:
            content = f.read()

        logger.info("Loaded prompt from %s (%d chars)", prompt_file.name, len(content))
        return content

    except Exception as e:
        logger.error("Failed to load prompt from %s: %s", prompt_file.name, e)
        raise


def list_prompts(subdirectory: str = "") -> dict[str, Path]:
    """List all available prompt files in a subdirectory.

    Args:
        subdirectory: Optional subdirectory to search (e.g., 'system', 'functions').

    Returns:
        Dictionary mapping prompt names to file paths.
    """
    search_dir = PROMPTS_DIR / subdirectory if subdirectory else PROMPTS_DIR

    if not search_dir.exists():
        logger.warning("Prompt directory not found: %s", search_dir)
        return {}

    prompts = {}
    for file_path in search_dir.rglob("*.txt"):
        relative_path = file_path.relative_to(PROMPTS_DIR)
        prompt_name = str(relative_path).replace(".txt", "").replace("/", "_")
        prompts[prompt_name] = file_path

    return prompts


def clear_cache() -> None:
    """Clear the prompt loading cache."""
    load_prompt.cache_clear()
    load_mode_prompts.cache_clear()
    logger.info("Prompt cache cleared")


@lru_cache(maxsize=16)
def load_mode_prompts(
    operator_bound: bool,
    is_cloud_operator: bool = False,
    g8e_web_search_available: bool = True,
) -> dict[str, str]:
    """Load mode-specific prompts based on Operator binding status and type.

    Args:
        operator_bound: True for operator_bound mode, False for operator_not_bound.
        is_cloud_operator: True for Cloud Operator (AWS), uses cloud_operator_bound mode.
        g8e_web_search_available: When False and operator is not bound, loads the no-search
            variant prompt files instead of the standard ones.

    Returns:
        Dict keyed by PromptSection string with loaded prompt content.
    """
    if is_cloud_operator and operator_bound:
        mode = AgentMode.CLOUD_OPERATOR_BOUND
    elif operator_bound:
        mode = AgentMode.OPERATOR_BOUND
    else:
        mode = AgentMode.OPERATOR_NOT_BOUND

    section_files = dict(AGENT_MODE_PROMPT_FILES[mode])

    if mode == AgentMode.OPERATOR_NOT_BOUND and not g8e_web_search_available:
        section_files[PromptSection.CAPABILITIES] = PromptFile.MODE_OPERATOR_NOT_BOUND_CAPABILITIES_NO_SEARCH
        section_files[PromptSection.EXECUTION] = PromptFile.MODE_OPERATOR_NOT_BOUND_EXECUTION_NO_SEARCH

    prompts: dict[str, str] = {}

    for section, prompt_file in section_files.items():
        try:
            prompts[section] = load_prompt(prompt_file)
        except ResourceNotFoundError:
            logger.warning("Mode prompt not found: %s", prompt_file)
            prompts[section] = ""

    logger.info(
        "Loaded mode prompts for %s",
        mode,
        extra={
            "mode": str(mode),
            "operator_bound": operator_bound,
            "is_cloud_operator": is_cloud_operator,
            "g8e_web_search_available": g8e_web_search_available,
            "prompts_loaded": list(prompts.keys()),
        },
    )

    return prompts
