from __future__ import annotations

from pathlib import Path
from typing import Literal, Self
from urllib.parse import urlparse

from pydantic import BaseModel, ConfigDict, Field, model_validator

from g8e.constants import PORTS


class CampaignRunArgs(BaseModel):
    model_config = ConfigDict(extra="forbid", frozen=True)

    suite: str
    preregistration: Path
    campaign_id: str
    release_version: str
    seed: int
    output_dir: Path
    gold_set: Path
    max_retries: int
    max_requests: int
    max_usd: float
    max_tokens: int
    model_tags: Path | None
    profile: Path | None
    models: Path | None
    g8ee_url: str | None
    operator_url: str
    operator_session_id: None = None
    g8e_cli: str | None
    auth_project_root: Path | None
    task_offset: int
    task_limit: int | None
    campaign_set_plan: Path | None
    replacement_rule: Path | None


class CampaignValidationArgs(BaseModel):
    model_config = ConfigDict(extra="forbid", frozen=True)

    profile: Path
    models: Path


class CampaignRunConfig(BaseModel):
    model_config = ConfigDict(extra="forbid", frozen=True)

    schema_version: Literal["1.0.0"] = "1.0.0"
    suite: str = Field(min_length=1)
    preregistration: Path
    campaign_id: str = Field(min_length=1)
    release_version: str = Field(min_length=1)
    seed: int = Field(ge=0)
    output_dir: Path
    gold_set: Path
    max_retries: int = Field(default=1, ge=0)
    max_requests: int = Field(gt=0)
    max_usd: float = Field(ge=0)
    max_tokens: int = Field(gt=0)
    model_tags: Path | None = None
    profile: Path | None = None
    models: Path | None = None
    g8ee_url: str | None = None
    operator_url: str = f"https://localhost:{PORTS['ports']['OperatorHttps']['value']}"
    g8e_cli: str | None = None
    auth_project_root: Path | None = None
    task_offset: int = Field(default=0, ge=0)
    task_limit: int | None = Field(default=None, gt=0)
    campaign_set_plan: Path | None = None
    replacement_rule: Path | None = None

    @model_validator(mode="after")
    def validate_options(self) -> Self:
        if (self.profile is None) != (self.models is None):
            raise ValueError("profile and models must be provided together")
        if self.replacement_rule is not None and self.campaign_set_plan is None:
            raise ValueError("replacement_rule requires campaign_set_plan")
        for name, value in (("g8ee_url", self.g8ee_url), ("operator_url", self.operator_url)):
            if value is None:
                continue
            parsed = urlparse(value)
            if parsed.scheme not in {"http", "https"} or not parsed.hostname:
                raise ValueError(f"{name} must be an absolute HTTP URL")
        return self

    def resolved(self, config_path: Path) -> CampaignRunConfig:
        base = config_path.resolve().parent

        def resolve_path(path: Path | None) -> Path | None:
            if path is None or path.is_absolute():
                return path
            return (base / path).resolve()

        g8e_cli = self.g8e_cli
        if g8e_cli is not None and g8e_cli != "auto" and "/" in g8e_cli:
            cli_path = Path(g8e_cli)
            if not cli_path.is_absolute():
                g8e_cli = str((base / cli_path).resolve())
        return CampaignRunConfig(
            schema_version=self.schema_version,
            suite=self.suite,
            preregistration=resolve_path(self.preregistration),
            campaign_id=self.campaign_id,
            release_version=self.release_version,
            seed=self.seed,
            output_dir=resolve_path(self.output_dir),
            gold_set=resolve_path(self.gold_set),
            max_retries=self.max_retries,
            max_requests=self.max_requests,
            max_usd=self.max_usd,
            max_tokens=self.max_tokens,
            model_tags=resolve_path(self.model_tags),
            profile=resolve_path(self.profile),
            models=resolve_path(self.models),
            g8ee_url=self.g8ee_url,
            operator_url=self.operator_url,
            g8e_cli=g8e_cli,
            auth_project_root=resolve_path(self.auth_project_root),
            task_offset=self.task_offset,
            task_limit=self.task_limit,
            campaign_set_plan=resolve_path(self.campaign_set_plan),
            replacement_rule=resolve_path(self.replacement_rule),
        )

    def command_args(self) -> CampaignRunArgs:
        return CampaignRunArgs(
            suite=self.suite,
            preregistration=self.preregistration,
            campaign_id=self.campaign_id,
            release_version=self.release_version,
            seed=self.seed,
            output_dir=self.output_dir,
            gold_set=self.gold_set,
            max_retries=self.max_retries,
            max_requests=self.max_requests,
            max_usd=self.max_usd,
            max_tokens=self.max_tokens,
            model_tags=self.model_tags,
            profile=self.profile,
            models=self.models,
            g8ee_url=self.g8ee_url,
            operator_url=self.operator_url,
            operator_session_id=None,
            g8e_cli=self.g8e_cli,
            auth_project_root=self.auth_project_root,
            task_offset=self.task_offset,
            task_limit=self.task_limit,
            campaign_set_plan=self.campaign_set_plan,
            replacement_rule=self.replacement_rule,
        )


def load_campaign_run_config(path: Path) -> CampaignRunConfig:
    return CampaignRunConfig.model_validate_json(path.read_text()).resolved(path)
