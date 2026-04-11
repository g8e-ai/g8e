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

from pathlib import Path
from unittest.mock import patch

import pytest

from app.errors import ResourceNotFoundError
from app.constants import AGENT_MODE_PROMPT_FILES, AgentMode, PromptFile, PromptSection

from app.prompts_data.loader import (
    PROMPTS_DIR,
    clear_cache,
    list_prompts,
    load_mode_prompts,
    load_prompt,
)


@pytest.fixture(autouse=True)
def clear_prompt_caches():
    load_prompt.cache_clear()
    load_mode_prompts.cache_clear()
    yield
    load_prompt.cache_clear()
    load_mode_prompts.cache_clear()


class TestPromptsDir:

    def test_prompts_dir_exists(self):
        assert PROMPTS_DIR.exists()
        assert PROMPTS_DIR.is_dir()

    def test_prompts_dir_is_prompts_data(self):
        assert PROMPTS_DIR.name == "prompts_data"

    def test_required_subdirectories_exist(self):
        for subdir in ["core", "tools", "modes", "system"]:
            path = PROMPTS_DIR / subdir
            assert path.exists(), f"Missing required subdirectory: {subdir}"
            assert path.is_dir()


class TestLoadPrompt:

    def test_loads_core_identity(self):
        content = load_prompt(PromptFile.CORE_IDENTITY)

        assert isinstance(content, str)
        assert len(content) > 0
        assert "g8e.local" in content

    def test_loads_core_safety(self):
        content = load_prompt(PromptFile.CORE_SAFETY)

        assert isinstance(content, str)
        assert len(content) > 0

    def test_loads_response_constraints(self):
        content = load_prompt(PromptFile.SYSTEM_RESPONSE_CONSTRAINTS)

        assert isinstance(content, str)
        assert len(content) > 0

    def test_raises_resource_not_found_for_nonexistent_path(self):
        with pytest.raises(TypeError):
            load_prompt("nonexistent/fake_prompt.txt")

    def test_raises_for_empty_path(self):
        with pytest.raises(TypeError):
            load_prompt("")

    def test_caches_repeated_loads(self):
        first = load_prompt(PromptFile.CORE_IDENTITY)
        second = load_prompt(PromptFile.CORE_IDENTITY)

        assert first is second

    def test_cache_differentiates_paths(self):
        identity = load_prompt(PromptFile.CORE_IDENTITY)
        safety = load_prompt(PromptFile.CORE_SAFETY)

        assert identity != safety

    def test_loads_tool_description(self):
        content = load_prompt(PromptFile.TOOL_RUN_COMMANDS)

        assert isinstance(content, str)
        assert len(content) > 0

    def test_all_tool_prompts_load(self):
        for prompt_file in PromptFile:
            if prompt_file.name.startswith("TOOL_"):
                content = load_prompt(prompt_file)
                assert len(content) > 0

    def test_all_core_prompts_load(self):
        for prompt_file in PromptFile:
            if prompt_file.name.startswith("CORE_"):
                content = load_prompt(prompt_file)
                assert len(content) > 0


class TestListPrompts:

    def test_lists_core_prompts(self):
        prompts = list_prompts("core")

        assert isinstance(prompts, dict)
        assert len(prompts) >= 2
        assert "core_identity" in prompts
        assert "core_safety" in prompts

    def test_lists_tool_prompts(self):
        prompts = list_prompts("tools")

        assert isinstance(prompts, dict)
        assert len(prompts) > 0
        assert "tools_run_commands_with_operator" in prompts

    def test_lists_system_prompts(self):
        prompts = list_prompts("system")

        assert isinstance(prompts, dict)
        assert "system_response_constraints" in prompts

    def test_lists_all_prompts_from_root(self):
        prompts = list_prompts()

        assert isinstance(prompts, dict)
        assert len(prompts) > 0
        has_core = any(k.startswith("core_") for k in prompts)
        has_tools = any(k.startswith("tools_") for k in prompts)
        has_modes = any(k.startswith("modes_") for k in prompts)
        assert has_core
        assert has_tools
        assert has_modes

    def test_values_are_path_objects(self):
        prompts = list_prompts("core")

        for name, path in prompts.items():
            assert isinstance(path, Path)
            assert path.exists()
            assert path.suffix == ".txt"

    def test_nonexistent_subdirectory_returns_empty(self):
        prompts = list_prompts("nonexistent_directory")

        assert prompts == {}

    def test_prompt_names_use_underscores(self):
        prompts = list_prompts("modes")

        for name in prompts:
            assert "/" not in name
            assert name.startswith("modes_")


class TestClearCache:

    def test_clears_load_prompt_cache(self):
        first = load_prompt(PromptFile.CORE_IDENTITY)
        clear_cache()
        second = load_prompt(PromptFile.CORE_IDENTITY)

        assert first == second
        assert first is not second

    def test_clears_load_mode_prompts_cache(self):
        first = load_mode_prompts(operator_bound=True)
        clear_cache()
        second = load_mode_prompts(operator_bound=True)

        assert first == second
        assert first is not second


class TestLoadModePrompts:

    def test_operator_not_bound_mode(self):
        prompts = load_mode_prompts(operator_bound=False)

        assert isinstance(prompts, dict)
        assert "capabilities" in prompts
        assert "execution" in prompts
        assert "tools" in prompts

        for key in ["capabilities", "execution", "tools"]:
            assert isinstance(prompts[key], str)
            assert len(prompts[key]) > 0, f"Empty {key} prompt for operator_not_bound"

    def test_operator_bound_mode(self):
        prompts = load_mode_prompts(operator_bound=True)

        assert isinstance(prompts, dict)
        assert "capabilities" in prompts
        assert "execution" in prompts
        assert "tools" in prompts

        for key in ["capabilities", "execution", "tools"]:
            assert isinstance(prompts[key], str)
            assert len(prompts[key]) > 0, f"Empty {key} prompt for operator_bound"

    def test_cloud_operator_bound_mode(self):
        prompts = load_mode_prompts(operator_bound=True, is_cloud_operator=True)

        assert isinstance(prompts, dict)
        assert "capabilities" in prompts
        assert "execution" in prompts
        assert "tools" in prompts

        for key in ["capabilities", "execution", "tools"]:
            assert isinstance(prompts[key], str)
            assert len(prompts[key]) > 0, f"Empty {key} prompt for cloud_operator_bound"

    def test_modes_produce_different_content(self):
        not_bound = load_mode_prompts(operator_bound=False)
        bound = load_mode_prompts(operator_bound=True)
        cloud = load_mode_prompts(operator_bound=True, is_cloud_operator=True)

        assert not_bound != bound
        assert bound != cloud
        assert not_bound != cloud

    def test_cloud_false_same_as_default(self):
        default = load_mode_prompts(operator_bound=True)
        explicit = load_mode_prompts(operator_bound=True, is_cloud_operator=False)

        assert default == explicit

    def test_mode_prompts_are_cached(self):
        first = load_mode_prompts(operator_bound=True)
        second = load_mode_prompts(operator_bound=True)

        assert first is second

    def test_different_modes_cached_separately(self):
        bound = load_mode_prompts(operator_bound=True)
        not_bound = load_mode_prompts(operator_bound=False)

        assert bound is not not_bound

    def test_mode_directories_exist_on_disk(self):
        for mode_dir in ["operator_not_bound", "operator_bound", "cloud_operator_bound"]:
            mode_path = PROMPTS_DIR / "modes" / mode_dir
            assert mode_path.exists(), f"Missing mode directory: {mode_dir}"
            for prompt_file in ["capabilities.txt", "execution.txt", "tools.txt"]:
                file_path = mode_path / prompt_file
                assert file_path.exists(), f"Missing {prompt_file} in {mode_dir}"

    def test_missing_mode_prompt_returns_empty_string(self):
        with patch("app.prompts_data.loader.load_prompt") as mock_load:
            mock_load.side_effect = ResourceNotFoundError(
                "not found",
                resource_type="prompt_file",
                resource_id="test_missing",
                component="vse"
            )

            prompts = load_mode_prompts.__wrapped__(
                operator_bound=True, is_cloud_operator=False
            )

            assert prompts["capabilities"] == ""
            assert prompts["execution"] == ""
            assert prompts["tools"] == ""


class TestPromptFileIntegrity:

    def test_no_empty_prompt_files(self):
        all_txt = list(PROMPTS_DIR.rglob("*.txt"))
        assert len(all_txt) > 0

        empty_files = []
        for txt_file in all_txt:
            content = txt_file.read_text(encoding="utf-8").strip()
            if len(content) == 0:
                empty_files.append(str(txt_file.relative_to(PROMPTS_DIR)))

        assert empty_files == [], f"Empty prompt files found: {empty_files}"

    def test_prompt_files_are_utf8(self):
        all_txt = list(PROMPTS_DIR.rglob("*.txt"))

        for txt_file in all_txt:
            try:
                txt_file.read_text(encoding="utf-8")
            except UnicodeDecodeError:
                pytest.fail(f"Invalid UTF-8 in {txt_file.relative_to(PROMPTS_DIR)}")

    def test_total_prompt_file_count(self):
        all_txt = list(PROMPTS_DIR.rglob("*.txt"))
        assert len(all_txt) >= 20, f"Only found {len(all_txt)} prompt files, expected 20+"
