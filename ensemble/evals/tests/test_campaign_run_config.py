from __future__ import annotations

import json
from pathlib import Path

import pytest
from click.testing import CliRunner
from pydantic import ValidationError

from g8e_evals import cli
from g8e_evals.campaign_run_config import CampaignRunConfig, load_campaign_run_config
from g8e_evals.cli import main

pytestmark = pytest.mark.unit


def _config() -> dict[str, object]:
    return {
        "schema_version": "1.0.0",
        "suite": "ifeval_subset",
        "preregistration": "authorities/preregistration.json",
        "campaign_id": "fresh-campaign",
        "release_version": "development",
        "seed": 42,
        "output_dir": "reports/fresh-campaign",
        "gold_set": "datasets/input_data.jsonl",
        "max_retries": 1,
        "max_requests": 30,
        "max_usd": 0.0,
        "max_tokens": 491520,
        "profile": "authorities/profile.json",
        "models": "authorities/models.json",
        "g8ee_url": "http://localhost:8000",
        "auth_project_root": "../../..",
    }


def test_campaign_run_config_resolves_paths_relative_to_config_file(tmp_path: Path) -> None:
    config_path = tmp_path / "operations" / "campaign.json"
    config_path.parent.mkdir()
    config_path.write_text(json.dumps(_config()))

    config = load_campaign_run_config(config_path)

    assert config.preregistration == config_path.parent / "authorities/preregistration.json"
    assert config.output_dir == config_path.parent / "reports/fresh-campaign"
    assert config.auth_project_root == tmp_path.parent.parent


def test_campaign_run_config_rejects_unknown_fields() -> None:
    with pytest.raises(ValidationError, match="extra_forbidden"):
        CampaignRunConfig.model_validate(_config() | {"api_key": "must-not-be-stored"})


def test_campaign_run_config_requires_profile_and_models_together() -> None:
    data = _config()
    del data["models"]

    with pytest.raises(ValidationError, match="profile and models must be provided together"):
        CampaignRunConfig.model_validate(data)


def test_campaign_check_prints_finite_budget_without_starting_provider(
    tmp_path: Path,
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    config_path = tmp_path / "campaign.json"
    config_path.write_text(json.dumps(_config()))
    for path in (
        tmp_path / "authorities/preregistration.json",
        tmp_path / "authorities/profile.json",
        tmp_path / "authorities/models.json",
        tmp_path / "datasets/input_data.jsonl",
    ):
        path.parent.mkdir(exist_ok=True)
        path.touch()
    monkeypatch.setattr(cli, "campaign_validate", lambda **kwargs: None)

    result = CliRunner().invoke(main, ["campaign", "check", str(config_path)])

    assert result.exit_code == 0, result.output
    assert "fresh-campaign" in result.output
    assert "30 requests" in result.output
    assert "491520 tokens" in result.output
    assert "provider calls: none" in result.output


def test_campaign_start_requires_explicit_confirmation(tmp_path: Path) -> None:
    config_path = tmp_path / "campaign.json"
    config_path.write_text(json.dumps(_config()))

    result = CliRunner().invoke(main, ["campaign", "start", str(config_path)])

    assert result.exit_code != 0
    assert "--yes" in result.output


def test_campaign_start_forwards_frozen_config_to_existing_runner(
    tmp_path: Path,
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    config_path = tmp_path / "campaign.json"
    config_path.write_text(json.dumps(_config()))
    received: dict[str, object] = {}

    def run(**kwargs: object) -> None:
        received.update(kwargs)

    monkeypatch.setattr(cli, "campaign_run", run)
    result = CliRunner().invoke(main, ["campaign", "start", str(config_path), "--yes"])

    assert result.exit_code == 0, result.output
    assert received["campaign_id"] == "fresh-campaign"
    assert received["max_requests"] == 30
    assert received["operator_session_id"] is None
    assert received["output_dir"] == tmp_path / "reports/fresh-campaign"
