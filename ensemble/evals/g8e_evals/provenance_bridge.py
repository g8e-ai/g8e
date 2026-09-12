# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""Source/build provenance supplied by the stamped g8e CLI binary.

The g8e suite owns build provenance: the Makefile stamps the full source
revision and the canonical source-tree state hash into the binary via
ldflags (``main.sourceRevision`` / ``main.sourceTreeHash``), and the Go
toolchain additionally embeds VCS metadata at build time. ``g8e version
--json`` exposes that stamp, so the evals runner asks the platform binary
for provenance instead of requiring hand-set environment variables.

The bridge mirrors ``auth_bridge``: it shells out to the canonical CLI and
validates the JSON payload into a typed model. Environment variables
remain the CI contract and an explicit override; this channel is used only
when no provenance env vars are set.
"""

from __future__ import annotations

import json
import subprocess

from pydantic import BaseModel, ConfigDict, ValidationError

from g8e_evals.schema import SourceBuildProvenance


class ProvenanceBridgeError(Exception):
    """The g8e binary answered ``version --json`` with a malformed payload.

    Raised only when the channel is reachable but its output cannot be
    trusted: an unparseable or contract-violating response indicates a
    corrupted or incompatible binary, not a missing one.
    """


class FIPSStatus(BaseModel):
    """FIPS 140-3 block of ``g8e version --json`` (present only with --fips)."""

    model_config = ConfigDict(extra="forbid", frozen=True)

    enabled: bool
    enforced: bool
    module_version: str = ""


class CLIVersionProvenance(BaseModel):
    """Typed ``g8e version --json`` payload emitted by the platform CLI.

    All fields are optional because unstamped builds omit them; the bridge
    treats absent provenance fields as "not stamped" rather than as
    sentinel strings.
    """

    model_config = ConfigDict(extra="forbid", frozen=True)

    version: str = ""
    build_id: str = ""
    build_time: str = ""
    platform: str = ""
    source_revision: str = ""
    source_tree_state_hash: str = ""
    source_tree_modified: bool | None = None
    fips140: FIPSStatus | None = None


def load_cli_build_provenance(g8e_cli: str, *, timeout_s: float = 30.0) -> SourceBuildProvenance | None:
    """Load source/build provenance from the stamped g8e CLI binary.

    Returns ``None`` when the channel cannot supply provenance: the binary
    is missing, times out, exits non-zero (e.g. an older binary without
    ``--json``), or reports an unstamped development build. Callers fall
    back to the ``G8E_EVALS_*`` environment variables or fail preflight.

    Raises ``ProvenanceBridgeError`` when the binary responds but its
    payload is malformed — a reachable channel returning garbage is a
    defect, not an absence.
    """
    try:
        result = subprocess.run(
            [g8e_cli, "version", "--json"],
            capture_output=True,
            check=False,
            text=True,
            timeout=timeout_s,
        )
    except (OSError, subprocess.TimeoutExpired):
        return None
    if result.returncode != 0:
        return None
    try:
        info = CLIVersionProvenance.model_validate(json.loads(result.stdout))
    except (json.JSONDecodeError, ValidationError) as error:
        raise ProvenanceBridgeError(f"invalid JSON from {g8e_cli} version --json: {error}") from error

    source_revision = info.source_revision.strip()
    source_tree_state_hash = info.source_tree_state_hash.strip()
    if not source_revision or not source_tree_state_hash:
        return None
    try:
        return SourceBuildProvenance(
            source_revision=source_revision,
            source_tree_state_hash=source_tree_state_hash,
            build_id=info.build_id if info.build_id != "unknown" else "",
            build_system="g8e-cli",
        )
    except ValidationError as error:
        raise ProvenanceBridgeError(f"invalid provenance stamp from {g8e_cli}: {error}") from error
