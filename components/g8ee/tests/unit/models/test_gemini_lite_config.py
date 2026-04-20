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

from app.models.model_configs import get_model_config
from app.constants import GEMINI_3_1_FLASH_LITE, ThinkingLevel

def test_gemini_3_1_flash_lite_config():
    """Verify that gemini-3.1-flash-lite-preview is correctly registered and configured."""
    config = get_model_config(GEMINI_3_1_FLASH_LITE)

    assert config.name == "gemini-3.1-flash-lite-preview"
    assert config.supports_tools is True
    assert config.context_window_input == 1_000_000
    assert config.context_window_output == 64_000

    # supported_thinking_levels is the single source of truth; a non-empty
    # list means the model supports thinking.
    assert len(config.supported_thinking_levels) > 0

    # OFF must be present so callers can explicitly disable thinking; MINIMAL,
    # LOW, MEDIUM, HIGH are the full intensity range the Flash-Lite preview
    # advertises. Order matches the model_configs.py declaration and is
    # relied on by _lowest_thinking_level / _highest_thinking_level helpers.
    assert config.supported_thinking_levels == [
        ThinkingLevel.OFF,
        ThinkingLevel.MINIMAL,
        ThinkingLevel.LOW,
        ThinkingLevel.MEDIUM,
        ThinkingLevel.HIGH,
    ]

def test_gemini_3_1_flash_lite_in_registry():
    """Verify the model is present in the global registry."""
    from app.models.model_configs import get_available_models
    models = get_available_models()
    assert GEMINI_3_1_FLASH_LITE in models
