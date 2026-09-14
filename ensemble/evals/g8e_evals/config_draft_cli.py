# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""Narrow CLI entry point for draft generation invoked by the Go facade.

The Go ``./g8e eval diagnostic draft`` and ``./g8e eval campaign draft``
commands write a JSON request file and invoke::

    <eval-venv-python> -m g8e_evals.config_draft_cli <request-path>

This module reads the request, calls the appropriate draft generation
function, writes the config to the requested output path, and emits the
review summary as a single JSON object on stdout. It is not a public CLI;
operators discover draft through ``./g8e eval``.
"""

from __future__ import annotations

import json
import sys
from pathlib import Path
from typing import Literal

from pydantic import BaseModel, ConfigDict, Field, ValidationError

from g8e_evals.config_draft import (
    CampaignDraftOverrides,
    DiagnosticDraftOverrides,
    generate_campaign_draft,
    generate_diagnostic_draft,
)
from g8e_evals.operation_config import (
    AuthorityRef,
    EvidenceKeyRef,
    ProviderEndpointRef,
)


class DiagnosticDraftRequest(BaseModel):
    """Typed request for a diagnostic draft."""

    model_config = ConfigDict(extra="forbid", frozen=True)

    kind: Literal["diagnostic"] = "diagnostic"
    preset: str = Field(min_length=1)
    operation_id: str = Field(min_length=1)
    revision: str = Field(min_length=1)
    report_root: str = Field(min_length=1)
    gold_set: AuthorityRef
    evidence_key: EvidenceKeyRef
    provider_endpoint: ProviderEndpointRef
    model_variant_id: str = Field(min_length=1)
    output_path: str = Field(min_length=1)
    overrides: DiagnosticDraftOverrides | None = None


class CampaignDraftRequest(BaseModel):
    """Typed request for a campaign draft."""

    model_config = ConfigDict(extra="forbid", frozen=True)

    kind: Literal["campaign"] = "campaign"
    preset: str = Field(min_length=1)
    operation_id: str = Field(min_length=1)
    revision: str = Field(min_length=1)
    report_root: str = Field(min_length=1)
    gold_set: AuthorityRef
    evidence_key: EvidenceKeyRef
    provider_endpoint: ProviderEndpointRef
    campaign_id: str = Field(min_length=1)
    release_version: str = Field(min_length=1)
    preregistration: AuthorityRef
    profile: AuthorityRef
    model_registry: AuthorityRef
    cohort_ids: list[str] = Field(min_length=1)
    model_tags: AuthorityRef | None = None
    campaign_set_plan: AuthorityRef | None = None
    replacement_rule: AuthorityRef | None = None
    output_path: str = Field(min_length=1)
    overrides: CampaignDraftOverrides | None = None


def run_draft(request_path: str) -> int:
    """Load a draft request, generate the config, and emit the summary."""
    path = Path(request_path)
    if not path.is_file():
        sys.stderr.write(f"request file not found: {request_path}\n")
        return 1

    try:
        data = json.loads(path.read_text())
    except json.JSONDecodeError as exc:
        sys.stderr.write(f"invalid request JSON: {exc}\n")
        return 1

    kind = data.get("kind")
    try:
        if kind == "diagnostic":
            req = DiagnosticDraftRequest.model_validate(data)
            _, summary = generate_diagnostic_draft(
                preset_name=req.preset,
                operation_id=req.operation_id,
                revision=req.revision,
                report_root=req.report_root,
                gold_set=req.gold_set,
                evidence_key=req.evidence_key,
                provider_endpoint=req.provider_endpoint,
                model_variant_id=req.model_variant_id,
                overrides=req.overrides,
                output_path=Path(req.output_path),
            )
        elif kind == "campaign":
            req = CampaignDraftRequest.model_validate(data)
            _, summary = generate_campaign_draft(
                preset_name=req.preset,
                operation_id=req.operation_id,
                revision=req.revision,
                report_root=req.report_root,
                gold_set=req.gold_set,
                evidence_key=req.evidence_key,
                provider_endpoint=req.provider_endpoint,
                campaign_id=req.campaign_id,
                release_version=req.release_version,
                preregistration=req.preregistration,
                profile=req.profile,
                model_registry=req.model_registry,
                cohort_ids=req.cohort_ids,
                model_tags=req.model_tags,
                campaign_set_plan=req.campaign_set_plan,
                replacement_rule=req.replacement_rule,
                overrides=req.overrides,
                output_path=Path(req.output_path),
            )
        else:
            sys.stderr.write(f"unknown draft kind: {kind!r}\n")
            return 1
    except (ValidationError, KeyError, FileExistsError, ValueError) as exc:
        sys.stderr.write(f"draft generation failed: {exc}\n")
        return 1

    sys.stdout.write(summary.model_dump_json(exclude_none=True))
    sys.stdout.write("\n")
    sys.stdout.flush()
    return 0


def main(argv: list[str] | None = None) -> int:
    """Entry point for ``python -m g8e_evals.config_draft_cli <request-path>``."""
    args = argv if argv is not None else sys.argv[1:]
    if len(args) != 1:
        sys.stderr.write("usage: python -m g8e_evals.config_draft_cli <request-path>\n")
        sys.stderr.write(f"got {len(args)} argument(s)\n")
        return 1
    return run_draft(args[0])


if __name__ == "__main__":
    sys.exit(main())
