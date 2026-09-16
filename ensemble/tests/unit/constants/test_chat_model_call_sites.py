# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

import importlib
import inspect

import pytest

from app.constants.chat_model_call_sites import CALL_SITE_BY_NAME, CHAT_MODEL_CALL_SITES


@pytest.mark.parametrize("site", CHAT_MODEL_CALL_SITES, ids=lambda site: site.call_site)
def test_call_site_module_is_importable(site):
    module = importlib.import_module(site.module)
    assert module is not None


def test_call_site_inventory_covers_required_personas():
    required = {
        "triage",
        "active_agent_primary",
        "active_agent_assistant",
        "title_generation",
        "memory_codex",
        "tribunal_generation",
        "tribunal_auditor",
        "warden_command_risk",
        "warden_error_analysis",
        "warden_file_operation",
        "eval_judge",
    }
    assert required == set(CALL_SITE_BY_NAME)


def test_call_site_registry_has_unique_names():
    names = [site.call_site for site in CHAT_MODEL_CALL_SITES]
    assert len(names) == len(set(names))


def test_call_site_modules_use_expected_provider_method():
    for entry in CHAT_MODEL_CALL_SITES:
        module = importlib.import_module(entry.module)
        source = inspect.getsource(module)
        assert entry.provider_method in source, (
            f"{entry.call_site} expected {entry.provider_method} in {entry.module}"
        )
