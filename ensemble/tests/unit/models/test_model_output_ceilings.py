# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""A model's output ceiling is the provider's documented limit or unset (INV-LLM-MODELS-04).

The table below is the only place a ceiling is allowed to appear. Adding a ceiling
to `model_configs.py` without a documented source and a row here fails this test,
so an invented cap (such as the 8192 seen on a Lite call) cannot return silently.
"""

import pytest

from app.models.model_configs import MODEL_REGISTRY, get_model_config

pytestmark = pytest.mark.unit

# model name -> documented output token limit
DOCUMENTED_CEILINGS = {
    # https://platform.claude.com/docs/en/models/haiku-4-5/overview
    "claude-haiku-4-5": 64_000,
    "claude-sonnet-4-6": 128_000,
    "claude-opus-4-6": 128_000,
    # https://ai.google.dev/gemini-api/docs/models (per-model pages: 65,536)
    "gemini-3.1-pro-preview": 65_536,
    "gemini-3.1-pro-preview-customtools": 65_536,
    "gemini-3.1-flash-lite": 65_536,
    "gemini-3-flash-preview": 65_536,
}


def test_every_configured_ceiling_is_documented():
    configured = {
        name: get_model_config(name).max_output_tokens
        for name in set(MODEL_REGISTRY.available_models())
        if get_model_config(name).max_output_tokens is not None
    }
    assert configured == DOCUMENTED_CEILINGS


def test_models_without_a_documented_ceiling_stay_unset():
    for name in set(MODEL_REGISTRY.available_models()):
        if name not in DOCUMENTED_CEILINGS:
            assert get_model_config(name).max_output_tokens is None, name
