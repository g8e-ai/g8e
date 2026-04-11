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

import pytest
from app.models.model_configs import get_model_config
from app.constants import GEMINI_3_1_FLASH_LITE_PREVIEW, ThinkingLevel

def test_gemini_3_1_flash_lite_preview_config():
    """Verify that gemini-3.1-flash-lite-preview is correctly registered and configured."""
    config = get_model_config(GEMINI_3_1_FLASH_LITE_PREVIEW)
    
    assert config.name == "gemini-3.1-flash-lite-preview"
    assert config.supports_thinking is True
    assert config.supports_tools is True
    assert config.context_window_input == 1_000_000
    assert config.context_window_output == 64_000
    
    # Verify thinking levels
    expected_levels = [
        ThinkingLevel.MINIMAL,
        ThinkingLevel.LOW,
        ThinkingLevel.MEDIUM,
        ThinkingLevel.HIGH
    ]
    assert all(level in config.supported_thinking_levels for level in expected_levels)
    assert len(config.supported_thinking_levels) == len(expected_levels)

def test_gemini_3_1_flash_lite_preview_in_registry():
    """Verify the model is present in the global registry."""
    from app.models.model_configs import get_available_models
    models = get_available_models()
    assert GEMINI_3_1_FLASH_LITE_PREVIEW in models
