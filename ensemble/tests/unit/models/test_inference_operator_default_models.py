# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""The Inference Operator's default role models are registered with their capabilities.

g8ee decides thinking and tool use by model name. An unregistered name resolves to
``UNKNOWN_MODEL_CONFIG`` (no tools), so a chat on an Operator default model would
silently lose its tools. The names below mirror ``InferenceDefault*Model`` in
internal/constants/inference.go.
"""

import pytest

from app.constants import (
    OLLAMA_GEMMA4_E4B,
    OLLAMA_QWEN3_1_7B,
    OLLAMA_QWEN3_5_0_8B,
    ThinkingDialect,
)
from app.models.model_configs import UNKNOWN_MODEL_CONFIG, get_model_config

pytestmark = pytest.mark.unit

INFERENCE_OPERATOR_DEFAULT_MODELS = {
    "primary": OLLAMA_GEMMA4_E4B,
    "assistant": OLLAMA_QWEN3_1_7B,
    "lite": OLLAMA_QWEN3_5_0_8B,
}


@pytest.mark.parametrize(("role", "model"), sorted(INFERENCE_OPERATOR_DEFAULT_MODELS.items()))
def test_operator_default_model_is_registered_with_tools_and_thinking(role, model):
    config = get_model_config(model)
    assert config is not UNKNOWN_MODEL_CONFIG, f"{role} default {model} is not registered"
    assert config.name == model
    assert config.supports_tools is True
    assert config.thinking_dialect is ThinkingDialect.NATIVE_TOGGLE
