# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""Regression tests for Phase 7 — DB collection names sourced from g8e.constants."""

import pytest

from g8e.constants import collection as _g8e_collection

from app.constants.collections import (
    DB_COLLECTION_AGENT_ACTIVITY_METADATA,
    DB_COLLECTION_API_KEYS,
    DB_COLLECTION_CASES,
    DB_COLLECTION_CLI_SESSIONS,
    DB_COLLECTION_INVESTIGATIONS,
    DB_COLLECTION_MEMORIES,
    DB_COLLECTION_OPERATORS,
    DB_COLLECTION_OPERATOR_SESSIONS,
    DB_COLLECTION_OPERATOR_USAGE,
    DB_COLLECTION_ORGANIZATIONS,
    DB_COLLECTION_REPUTATION_COMMITMENTS,
    DB_COLLECTION_REPUTATION_STATE,
    DB_COLLECTION_REVOKED_CERTS,
    DB_COLLECTION_SETTINGS,
    DB_COLLECTION_STAKE_RESOLUTIONS,
    DB_COLLECTION_TASKS,
    DB_COLLECTION_TRIBUNAL_COMMANDS,
    DB_COLLECTION_USERS,
    DB_COLLECTION_WEB_SESSIONS,
)

pytestmark = pytest.mark.unit


class TestCollectionsFromG8e:
    """Verify shared collection names match g8e protocol constants."""

    @pytest.mark.parametrize(
        "local,g8e_key",
        [
            (DB_COLLECTION_SETTINGS, "settings"),
            (DB_COLLECTION_USERS, "users"),
            (DB_COLLECTION_WEB_SESSIONS, "web_sessions"),
            (DB_COLLECTION_OPERATOR_SESSIONS, "operator_sessions"),
            (DB_COLLECTION_CLI_SESSIONS, "cli_sessions"),
            (DB_COLLECTION_ORGANIZATIONS, "organizations"),
            (DB_COLLECTION_OPERATORS, "operators"),
            (DB_COLLECTION_OPERATOR_USAGE, "operator_usage"),
            (DB_COLLECTION_CASES, "cases"),
            (DB_COLLECTION_INVESTIGATIONS, "investigations"),
            (DB_COLLECTION_TASKS, "tasks"),
            (DB_COLLECTION_MEMORIES, "memories"),
            (DB_COLLECTION_REVOKED_CERTS, "revoked_certificates"),
            (DB_COLLECTION_AGENT_ACTIVITY_METADATA, "agent_activity_metadata"),
            (DB_COLLECTION_REPUTATION_STATE, "reputation_state"),
            (DB_COLLECTION_REPUTATION_COMMITMENTS, "reputation_commitments"),
            (DB_COLLECTION_STAKE_RESOLUTIONS, "stake_resolutions"),
        ],
    )
    def test_collection_matches_g8e(self, local: str, g8e_key: str):
        assert local == _g8e_collection(g8e_key)


class TestG8eeSpecificCollections:
    """Verify g8ee-specific collections are local strings not in g8e protocol."""

    def test_api_keys_is_local(self):
        assert DB_COLLECTION_API_KEYS == "api_keys"

    def test_tribunal_commands_is_local(self):
        assert DB_COLLECTION_TRIBUNAL_COMMANDS == "tribunal_commands"
