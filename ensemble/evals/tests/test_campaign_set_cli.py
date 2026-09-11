# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""Tier 1 unit tests for the campaign-set CLI entry points.

Verifies that ``campaign-set plan``, ``campaign-set validate``, and
``campaign-set verify`` commands parse arguments, load plan/index
files, and produce the expected output. No external dependencies
(no files beyond temp, network, or DB).
"""

from __future__ import annotations

import json
from pathlib import Path

import pytest
from click.testing import CliRunner

from g8e_evals.campaign_set import (
    build_campaign_set_plan,
    compute_campaign_set_index_hash,
    CampaignChildIndexEntry,
    CampaignSetIndex,
)
from g8e_evals.cli import main


pytestmark = pytest.mark.unit

_VALID_HASH = "a" * 64
_VALID_HASH_B = "b" * 64
_VALID_HASH_C = "c" * 64
_VALID_HASH_D = "d" * 64
_VALID_HASH_E = "e" * 64
_VALID_HASH_F = "f" * 64
_VALID_HASH_G = "g" * 64


def _make_population_task_ids() -> list[str]:
    return [f"ifeval-{i:04d}" for i in range(120)]


def _make_plan_kwargs() -> dict:
    return {
        "set_id": "ifeval-expanded-set",
        "parent_campaign_id": "ifeval-expanded-parent",
        "parent_campaign_revision": "v2.1.8",
        "population_task_ids": _make_population_task_ids(),
        "population_hash": _VALID_HASH,
        "model_registry_hash": _VALID_HASH_B,
        "campaign_profile_hash": _VALID_HASH_C,
        "repetition_ids": ["rep-1", "rep-2", "rep-3"],
        "seed": 42,
        "retry_policy_hash": _VALID_HASH_D,
        "budget_authority_hash": _VALID_HASH_E,
        "instrumentation_policy_hash": _VALID_HASH_F,
        "expected_record_policy_hash": _VALID_HASH_G,
        "orchestrator_environment_scope": "linux/amd64/cpu",
        "provider_environment_scope": "linux/amd64/remote-ollama",
        "child_revisions": [f"child-rev-{i}" for i in range(4)],
    }


def _write_plan(tmp_path: Path) -> Path:
    plan = build_campaign_set_plan(**_make_plan_kwargs())
    plan_path = tmp_path / "campaign-set-plan.json"
    plan_path.write_text(plan.model_dump_json(indent=2))
    return plan_path


def _write_index(tmp_path: Path, plan_path: Path) -> Path:
    plan_data = json.loads(plan_path.read_text())
    entries: list[CampaignChildIndexEntry] = []
    for cp in plan_data["child_plans"]:
        entries.append(CampaignChildIndexEntry(
            child_id=cp["child_id"],
            report_campaign_id=cp["child_id"],
            finalization_generation_hash=_VALID_HASH,
            child_verification_report_hash=_VALID_HASH_B,
            report_checksum=_VALID_HASH_C,
            assignment_count=cp["expected_assignment_count"],
        ))
    total = sum(e.assignment_count for e in entries)
    index = CampaignSetIndex.model_construct(
        set_id=plan_data["set_id"],
        set_plan_hash=plan_data["content_hash"],
        child_index_entries=entries,
        total_assignment_count=total,
        content_hash="0" * 64,
    )
    content_hash = compute_campaign_set_index_hash(index)
    final_index = CampaignSetIndex(
        set_id=plan_data["set_id"],
        set_plan_hash=plan_data["content_hash"],
        child_index_entries=entries,
        total_assignment_count=total,
        content_hash=content_hash,
    )
    index_path = tmp_path / "campaign-set-index.json"
    index_path.write_text(final_index.model_dump_json(indent=2))
    return index_path


class TestCampaignSetPlanCommand:
    def test_plan_command_produces_dry_run_output(self, tmp_path: Path):
        plan_path = _write_plan(tmp_path)
        runner = CliRunner()
        result = runner.invoke(main, ["campaign-set", "plan", "--plan", str(plan_path)])
        assert result.exit_code == 0
        assert "Campaign-set plan" in result.output
        assert "ifeval-expanded-set" in result.output
        assert "11160" in result.output
        assert "2790" in result.output

    def test_plan_command_shows_child_details(self, tmp_path: Path):
        plan_path = _write_plan(tmp_path)
        runner = CliRunner()
        result = runner.invoke(main, ["campaign-set", "plan", "--plan", str(plan_path)])
        assert result.exit_code == 0
        assert "child_count" in result.output
        assert "partition_index" in result.output

    def test_plan_command_rejects_invalid_plan(self, tmp_path: Path):
        bad_plan_path = tmp_path / "bad-plan.json"
        bad_plan_path.write_text('{"invalid": true}')
        runner = CliRunner()
        result = runner.invoke(main, ["campaign-set", "plan", "--plan", str(bad_plan_path)])
        assert result.exit_code != 0


class TestCampaignSetValidateCommand:
    def test_validate_command_validates_plan(self, tmp_path: Path):
        plan_path = _write_plan(tmp_path)
        runner = CliRunner()
        result = runner.invoke(main, ["campaign-set", "validate", "--plan", str(plan_path)])
        assert result.exit_code == 0
        assert "Campaign-set plan validation" in result.output
        assert "ok" in result.output

    def test_validate_command_validates_plan_and_index(self, tmp_path: Path):
        plan_path = _write_plan(tmp_path)
        index_path = _write_index(tmp_path, plan_path)
        runner = CliRunner()
        result = runner.invoke(main, [
            "campaign-set", "validate",
            "--plan", str(plan_path),
            "--index", str(index_path),
        ])
        assert result.exit_code == 0
        assert "Campaign-set plan validation" in result.output
        assert "Campaign-set index validation" in result.output

    def test_validate_command_rejects_missing_plan(self, tmp_path: Path):
        runner = CliRunner()
        result = runner.invoke(main, [
            "campaign-set", "validate",
            "--plan", str(tmp_path / "nonexistent.json"),
        ])
        assert result.exit_code != 0

    def test_validate_command_rejects_invalid_index(self, tmp_path: Path):
        plan_path = _write_plan(tmp_path)
        bad_index_path = tmp_path / "bad-index.json"
        bad_index_path.write_text('{"invalid": true}')
        runner = CliRunner()
        result = runner.invoke(main, [
            "campaign-set", "validate",
            "--plan", str(plan_path),
            "--index", str(bad_index_path),
        ])
        assert result.exit_code != 0


class TestCampaignSetVerifyCommand:
    def test_verify_command_requires_child_dirs(self, tmp_path: Path):
        plan_path = _write_plan(tmp_path)
        index_path = _write_index(tmp_path, plan_path)
        runner = CliRunner()
        result = runner.invoke(main, [
            "campaign-set", "verify",
            "--plan", str(plan_path),
            "--index", str(index_path),
        ])
        assert result.exit_code != 0

    def test_verify_command_rejects_missing_plan(self, tmp_path: Path):
        index_path = tmp_path / "index.json"
        index_path.write_text("{}")
        runner = CliRunner()
        result = runner.invoke(main, [
            "campaign-set", "verify",
            "--plan", str(tmp_path / "nonexistent.json"),
            "--index", str(index_path),
            "--child-dir", "child-0", str(tmp_path),
        ])
        assert result.exit_code != 0
