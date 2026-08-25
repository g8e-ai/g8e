# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""Canonical database collection names.

Sourced from g8e.constants.COLLECTIONS to stay in sync with the protocol.
g8ee-specific collections not in the protocol are defined locally.
"""

from g8e.constants import collection as _g8e_collection
from g8e.constants import document_id as _g8e_document_id

# Collections sourced from g8e protocol constants
DB_COLLECTION_SETTINGS: str = _g8e_collection("settings")
DB_COLLECTION_USERS: str = _g8e_collection("users")
DB_COLLECTION_WEB_SESSIONS: str = _g8e_collection("web_sessions")
DB_COLLECTION_OPERATOR_SESSIONS: str = _g8e_collection("operator_sessions")
DB_COLLECTION_CLI_SESSIONS: str = _g8e_collection("cli_sessions")
DB_COLLECTION_ORGANIZATIONS: str = _g8e_collection("organizations")
DB_COLLECTION_OPERATORS: str = _g8e_collection("operators")
DB_COLLECTION_OPERATOR_USAGE: str = _g8e_collection("operator_usage")
DB_COLLECTION_CASES: str = _g8e_collection("cases")
DB_COLLECTION_INVESTIGATIONS: str = _g8e_collection("investigations")
DB_COLLECTION_TASKS: str = _g8e_collection("tasks")
DB_COLLECTION_MEMORIES: str = _g8e_collection("memories")
DB_COLLECTION_REVOKED_CERTS: str = _g8e_collection("revoked_certificates")
DB_COLLECTION_AGENT_ACTIVITY_METADATA: str = _g8e_collection("agent_activity_metadata")
DB_COLLECTION_REPUTATION_STATE: str = _g8e_collection("reputation_state")
DB_COLLECTION_REPUTATION_COMMITMENTS: str = _g8e_collection("reputation_commitments")
DB_COLLECTION_STAKE_RESOLUTIONS: str = _g8e_collection("stake_resolutions")

# g8ee-specific collections not in the g8e protocol
DB_COLLECTION_API_KEYS: str = "api_keys"
DB_COLLECTION_TRIBUNAL_COMMANDS: str = "tribunal_commands"

# Document IDs for settings collection (sourced from g8e protocol constants)
PLATFORM_SETTINGS_DOC: str = _g8e_document_id("platform_settings")
USER_SETTINGS_DOC_PREFIX: str = _g8e_document_id("user_settings_prefix")
