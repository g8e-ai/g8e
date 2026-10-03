# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

import importlib
import inspect

import pytest

from app.constants.chat_model_call_sites import (
    CALL_SITE_BY_NAME,
    CHAT_MODEL_CALL_SITES,
    classification_for_agent_role,
)
from app.errors import ValidationError


@pytest.mark.parametrize("site", CHAT_MODEL_CALL_SITES, ids=lambda site: site.call_site)
def test_call_site_module_is_importable(site):
    module = importlib.import_module(site.module)
    assert module is not None


def test_call_site_inventory_covers_required_personas():
    required = {
        "triage",
        "active_agent_primary",
        "active_agent_assistant",
        "active_agent_lite",
        "title_generation",
        "memory_codex",
        "tribunal_generation",
        "tribunal_auditor",
        "marshal_command",
        "marshal_error",
        "marshal_file",
        "eval_judge",
    }
    assert required == set(CALL_SITE_BY_NAME)


def test_call_site_registry_has_unique_names():
    names = [site.call_site for site in CHAT_MODEL_CALL_SITES]
    assert len(names) == len(set(names))


@pytest.mark.parametrize(
    ("call_site", "classification"),
    [
        ("triage", "scored_chain"),
        ("active_agent_primary", "scored_chain"),
        ("active_agent_assistant", "scored_chain"),
        ("active_agent_lite", "scored_chain"),
        ("tribunal_generation", "scored_chain"),
        ("tribunal_auditor", "scored_chain"),
        ("marshal_command", "scored_chain"),
        ("title_generation", "post_turn"),
        ("memory_codex", "post_turn"),
        ("eval_judge", "grader"),
    ],
)
def test_call_site_states_its_chain_classification(call_site, classification):
    assert CALL_SITE_BY_NAME[call_site].classification == classification


def test_classification_for_agent_role_covers_every_registered_role():
    for site in CHAT_MODEL_CALL_SITES:
        assert classification_for_agent_role(site.agent_role) == site.classification


def test_classification_for_unregistered_agent_role_fails_closed():
    with pytest.raises(ValidationError):
        classification_for_agent_role("unknown")


def test_call_site_modules_use_expected_provider_method():
    for entry in CHAT_MODEL_CALL_SITES:
        module = importlib.import_module(entry.module)
        source = inspect.getsource(module)
        assert entry.provider_method in source, (
            f"{entry.call_site} expected {entry.provider_method} in {entry.module}"
        )
