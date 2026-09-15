# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""Tests for typed operation config models, presets, and draft generation.

Covers: unknown field rejection, missing authority, hash mismatch,
path traversal, absolute path rejection, invalid endpoint, incoherent
task slice, zero/unbounded budget, concurrency, reused output root,
secret-like field rejection, deterministic rendering, create-new
semantics, and preset override behavior.
"""

from __future__ import annotations

import json

import pytest
from pydantic import ValidationError

pytestmark = pytest.mark.unit

from g8e_evals.config_draft import (
    CampaignDraftOverrides,
    DiagnosticDraftOverrides,
    generate_campaign_draft,
    generate_diagnostic_draft,
)
from g8e_evals.eval_presets import (
    get_campaign_preset,
    get_diagnostic_preset,
    list_campaign_presets,
    list_diagnostic_presets,
)
from g8e_evals.operation_config import (
    AuthorityRef,
    BudgetCeilings,
    CampaignConfig,
    DiagnosticConfig,
    EvidenceKeyRef,
    OperationKind,
    ProviderEndpointRef,
    StopConditions,
    load_operation_config,
    write_operation_config,
)


# ---------------------------------------------------------------------------
# Fixtures
# ---------------------------------------------------------------------------

GOLD_SHA = "a" * 64
PREREG_SHA = "b" * 64
PROFILE_SHA = "c" * 64
REGISTRY_SHA = "d" * 64


def _diag_kwargs(**overrides):
    base = {
        "operation_id": "diag-001",
        "revision": "rev-1",
        "suite": "ifeval_subset",
        "seed": 42,
        "report_root": ".local.dev/campaign/diag-test",
        "gold_set": AuthorityRef(path=".local.dev/campaign/gold.json", sha256=GOLD_SHA),
        "evidence_key": EvidenceKeyRef(path=".local.dev/campaign/key.json", key_id="key-001"),
        "provider_endpoint": ProviderEndpointRef(provider="ollama", endpoint_class="ollama-local"),
        "budget": BudgetCeilings(max_requests=30, max_tokens=491520, max_usd=0),
        "stop_conditions": StopConditions(idle_timeout_s=180.0),
        "model_variant_id": "gemma4:e4b",
        "arm": "ensemble_ungoverned",
    }
    base.update(overrides)
    return base


def _camp_kwargs(**overrides):
    base = {
        "operation_id": "camp-001",
        "revision": "rev-1",
        "suite": "ifeval_subset",
        "seed": 42,
        "report_root": ".local.dev/campaign/camp-test",
        "gold_set": AuthorityRef(path=".local.dev/campaign/gold.json", sha256=GOLD_SHA),
        "evidence_key": EvidenceKeyRef(path=".local.dev/campaign/key.json", key_id="key-001"),
        "provider_endpoint": ProviderEndpointRef(provider="ollama", endpoint_class="ollama-local"),
        "budget": BudgetCeilings(max_requests=100, max_tokens=1000000, max_usd=0, concurrency=1),
        "stop_conditions": StopConditions(idle_timeout_s=180.0),
        "campaign_id": "test-campaign",
        "release_version": "v2.1.8",
        "preregistration": AuthorityRef(path=".local.dev/campaign/prereg.json", sha256=PREREG_SHA),
        "profile": AuthorityRef(path=".local.dev/campaign/profile.json", sha256=PROFILE_SHA),
        "model_registry": AuthorityRef(path=".local.dev/campaign/registry.json", sha256=REGISTRY_SHA),
        "arms": ["direct", "ensemble_ungoverned"],
        "cohort_ids": ["cohort-qwen3-8b"],
        "repetitions": 3,
    }
    base.update(overrides)
    return base


# ---------------------------------------------------------------------------
# OperationConfigBase: content hash
# ---------------------------------------------------------------------------


class TestContentHash:
    def test_content_hash_is_none_during_construction(self):
        config = DiagnosticConfig(**_diag_kwargs())
        assert config.content_hash is None

    def test_finalize_sets_content_hash(self):
        config = DiagnosticConfig(**_diag_kwargs())
        finalized = config.finalized()
        assert finalized.content_hash is not None
        assert len(finalized.content_hash) == 64

    def test_content_hash_is_deterministic(self):
        c1 = DiagnosticConfig(**_diag_kwargs()).finalized()
        c2 = DiagnosticConfig(**_diag_kwargs()).finalized()
        assert c1.content_hash == c2.content_hash

    def test_content_hash_changes_on_field_change(self):
        c1 = DiagnosticConfig(**_diag_kwargs()).finalized()
        c2 = DiagnosticConfig(**_diag_kwargs(seed=99)).finalized()
        assert c1.content_hash != c2.content_hash

    def test_content_hash_excludes_content_hash_field(self):
        config = DiagnosticConfig(**_diag_kwargs())
        canonical = config.canonical_json_bytes()
        assert b"content_hash" not in canonical

    def test_load_verifies_content_hash(self, tmp_path):
        config = DiagnosticConfig(**_diag_kwargs())
        out = tmp_path / "diag.json"
        write_operation_config(config, out)
        loaded = load_operation_config(out)
        assert isinstance(loaded, DiagnosticConfig)
        assert loaded.content_hash == config.finalized().content_hash

    def test_load_rejects_tampered_content_hash(self, tmp_path):
        config = DiagnosticConfig(**_diag_kwargs())
        out = tmp_path / "diag.json"
        write_operation_config(config, out)
        data = json.loads(out.read_text())
        data["content_hash"] = "0" * 64
        out.write_text(json.dumps(data))
        with pytest.raises(ValidationError, match="content_hash mismatch"):
            load_operation_config(out)


# ---------------------------------------------------------------------------
# Path validation
# ---------------------------------------------------------------------------


class TestPathValidation:
    def test_authority_ref_rejects_absolute_path(self):
        with pytest.raises(ValidationError, match="repository-relative"):
            AuthorityRef(path="/etc/passwd", sha256=GOLD_SHA)

    def test_authority_ref_rejects_escaping_path(self):
        with pytest.raises(ValidationError, match="escape repository root"):
            AuthorityRef(path="../../etc/passwd", sha256=GOLD_SHA)

    def test_evidence_key_ref_rejects_absolute_path(self):
        with pytest.raises(ValidationError, match="repository-relative"):
            EvidenceKeyRef(path="/etc/key.json", key_id="key-001")

    def test_evidence_key_ref_rejects_escaping_path(self):
        with pytest.raises(ValidationError, match="escape repository root"):
            EvidenceKeyRef(path="../../key.json", key_id="key-001")

    def test_report_root_rejects_absolute_path(self):
        with pytest.raises(ValidationError, match="report_root must be repository-relative"):
            DiagnosticConfig(**_diag_kwargs(report_root="/tmp/eval"))

    def test_report_root_rejects_escaping_path(self):
        with pytest.raises(ValidationError, match="report_root must not escape"):
            DiagnosticConfig(**_diag_kwargs(report_root="../../tmp/eval"))


# ---------------------------------------------------------------------------
# Unknown field rejection
# ---------------------------------------------------------------------------


class TestUnknownFieldRejection:
    def test_diagnostic_rejects_unknown_field(self):
        with pytest.raises(ValidationError, match="extra"):
            DiagnosticConfig(**_diag_kwargs(secret_key="abc"))

    def test_campaign_rejects_unknown_field(self):
        with pytest.raises(ValidationError, match="extra"):
            CampaignConfig(**_camp_kwargs(api_key="secret"))

    def test_authority_ref_rejects_unknown_field(self):
        with pytest.raises(ValidationError, match="extra"):
            AuthorityRef.model_validate({"path": "ok.json", "sha256": GOLD_SHA, "extra_field": "bad"})

    def test_budget_rejects_unknown_field(self):
        with pytest.raises(ValidationError, match="extra"):
            BudgetCeilings.model_validate(
                {"max_requests": 1, "max_tokens": 1, "max_usd": 0, "secret": "abc"}
            )


# ---------------------------------------------------------------------------
# Budget validation
# ---------------------------------------------------------------------------


class TestBudgetValidation:
    def test_zero_max_requests_rejected(self):
        with pytest.raises(ValidationError):
            DiagnosticConfig(**_diag_kwargs(budget=BudgetCeilings(max_requests=0, max_tokens=100, max_usd=0)))

    def test_zero_max_tokens_rejected(self):
        with pytest.raises(ValidationError):
            DiagnosticConfig(**_diag_kwargs(budget=BudgetCeilings(max_requests=10, max_tokens=0, max_usd=0)))

    def test_negative_max_usd_rejected(self):
        with pytest.raises(ValidationError):
            DiagnosticConfig(**_diag_kwargs(budget=BudgetCeilings(max_requests=10, max_tokens=100, max_usd=-1)))

    def test_zero_concurrency_rejected(self):
        with pytest.raises(ValidationError):
            DiagnosticConfig(**_diag_kwargs(budget=BudgetCeilings(max_requests=10, max_tokens=100, max_usd=0, concurrency=0)))

    def test_negative_seed_rejected(self):
        with pytest.raises(ValidationError):
            DiagnosticConfig(**_diag_kwargs(seed=-1))


# ---------------------------------------------------------------------------
# Campaign-specific validation
# ---------------------------------------------------------------------------


class TestCampaignValidation:
    def test_replacement_rule_requires_campaign_set_plan(self):
        with pytest.raises(ValidationError, match="replacement_rule requires campaign_set_plan"):
            CampaignConfig(**_camp_kwargs(
                replacement_rule=AuthorityRef(path=".local.dev/campaign/rule.json", sha256="e" * 64),
            ))

    def test_replacement_rule_with_campaign_set_plan_succeeds(self):
        config = CampaignConfig(**_camp_kwargs(
            campaign_set_plan=AuthorityRef(path=".local.dev/campaign/plan.json", sha256="f" * 64),
            replacement_rule=AuthorityRef(path=".local.dev/campaign/rule.json", sha256="e" * 64),
        ))
        assert config.replacement_rule is not None
        assert config.campaign_set_plan is not None

    def test_empty_arms_rejected(self):
        with pytest.raises(ValidationError):
            CampaignConfig(**_camp_kwargs(arms=[]))

    def test_empty_cohort_ids_rejected(self):
        with pytest.raises(ValidationError):
            CampaignConfig(**_camp_kwargs(cohort_ids=[]))

    def test_zero_repetitions_rejected(self):
        with pytest.raises(ValidationError):
            CampaignConfig(**_camp_kwargs(repetitions=0))


# ---------------------------------------------------------------------------
# Canonical rendering
# ---------------------------------------------------------------------------


class TestCanonicalRendering:
    def test_canonical_json_is_sorted_and_compact(self):
        config = DiagnosticConfig(**_diag_kwargs())
        canonical = config.canonical_json_bytes()
        decoded = json.loads(canonical)
        keys = list(decoded.keys())
        assert keys == sorted(keys)

    def test_canonical_json_excludes_none_values(self):
        config = DiagnosticConfig(**_diag_kwargs())
        canonical = config.canonical_json_bytes()
        decoded = json.loads(canonical)
        assert "preregistration" not in decoded
        assert "model_tags" not in decoded

    def test_write_and_load_round_trip_diagnostic(self, tmp_path):
        config = DiagnosticConfig(**_diag_kwargs())
        out = tmp_path / "diag.json"
        write_operation_config(config, out)
        loaded = load_operation_config(out)
        assert isinstance(loaded, DiagnosticConfig)
        assert loaded == config.finalized()

    def test_write_and_load_round_trip_campaign(self, tmp_path):
        config = CampaignConfig(**_camp_kwargs())
        out = tmp_path / "camp.json"
        write_operation_config(config, out)
        loaded = load_operation_config(out)
        assert isinstance(loaded, CampaignConfig)
        assert loaded == config.finalized()


# ---------------------------------------------------------------------------
# Load dispatch
# ---------------------------------------------------------------------------


class TestLoadDispatch:
    def test_load_diagnostic_returns_diagnostic_config(self, tmp_path):
        config = DiagnosticConfig(**_diag_kwargs())
        out = tmp_path / "diag.json"
        write_operation_config(config, out)
        loaded = load_operation_config(out)
        assert isinstance(loaded, DiagnosticConfig)
        assert loaded.operation_kind == OperationKind.DIAGNOSTIC

    def test_load_campaign_returns_campaign_config(self, tmp_path):
        config = CampaignConfig(**_camp_kwargs())
        out = tmp_path / "camp.json"
        write_operation_config(config, out)
        loaded = load_operation_config(out)
        assert isinstance(loaded, CampaignConfig)
        assert loaded.operation_kind == OperationKind.CAMPAIGN

    def test_load_unknown_kind_raises(self, tmp_path):
        out = tmp_path / "bad.json"
        out.write_text(json.dumps({
            "schema_version": "1.0.0",
            "operation_kind": "unknown",
            "operation_id": "x",
            "revision": "r",
            "suite": "s",
            "seed": 0,
            "report_root": "r",
            "gold_set": {"path": "g", "sha256": GOLD_SHA},
            "evidence_key": {"path": "k", "key_id": "k"},
            "provider_endpoint": {"provider": "p", "endpoint_class": "e"},
            "budget": {"max_requests": 1, "max_tokens": 1, "max_usd": 0, "max_retries": 1, "concurrency": 1},
            "stop_conditions": {"idle_timeout_s": 1.0},
            "content_hash": None,
        }))
        with pytest.raises(ValueError, match="unknown operation_kind"):
            load_operation_config(out)


# ---------------------------------------------------------------------------
# Presets
# ---------------------------------------------------------------------------


class TestPresets:
    def test_list_diagnostic_presets_returns_names(self):
        names = list_diagnostic_presets()
        assert "ifeval-five-task" in names

    def test_list_campaign_presets_returns_names(self):
        names = list_campaign_presets()
        assert "opendevops-development" in names
        assert "ifeval-full-120" in names

    def test_get_diagnostic_preset_returns_preset(self):
        preset = get_diagnostic_preset("ifeval-five-task")
        assert preset.suite == "ifeval_subset"
        assert preset.arm == "ensemble_ungoverned"
        assert preset.task_limit == 5

    def test_get_campaign_preset_returns_preset(self):
        preset = get_campaign_preset("opendevops-development")
        assert preset.suite == "ifeval_subset"
        assert preset.repetitions == 3

    def test_all_roles_governed_preset_uses_full_stack_doctrine_arm(self):
        preset = get_campaign_preset("all-roles-governed")
        assert preset.arms == ["doctrine"]
        assert preset.task_limit == 5
        assert preset.repetitions == 3
        assert preset.budget.concurrency == 1

    def test_all_roles_governed_smoke_preset_is_minimal(self):
        preset = get_campaign_preset("all-roles-governed-smoke")
        assert preset.arms == ["doctrine"]
        assert preset.task_limit == 1
        assert preset.repetitions == 1

    def test_get_diagnostic_preset_unknown_raises(self):
        with pytest.raises(KeyError, match="unknown diagnostic preset"):
            get_diagnostic_preset("nonexistent")

    def test_get_campaign_preset_unknown_raises(self):
        with pytest.raises(KeyError, match="unknown campaign preset"):
            get_campaign_preset("nonexistent")


# ---------------------------------------------------------------------------
# Draft generation
# ---------------------------------------------------------------------------


class TestDiagnosticDraft:
    def test_draft_creates_config_at_new_path(self, tmp_path):
        out = tmp_path / "diag.json"
        config, summary = generate_diagnostic_draft(
            preset_name="ifeval-five-task",
            operation_id="diag-001",
            revision="rev-1",
            report_root=".local.dev/campaign/diag-test",
            gold_set=AuthorityRef(path=".local.dev/campaign/gold.json", sha256=GOLD_SHA),
            evidence_key=EvidenceKeyRef(path=".local.dev/campaign/key.json", key_id="key-001"),
            provider_endpoint=ProviderEndpointRef(provider="ollama", endpoint_class="ollama-local"),
            model_variant_id="gemma4:e4b",
            output_path=out,
        )
        assert out.exists()
        assert config.content_hash is not None
        assert summary.operation_kind == "diagnostic"
        assert summary.preset == "ifeval-five-task"

    def test_draft_rejects_existing_path(self, tmp_path):
        out = tmp_path / "diag.json"
        out.write_text("{}")
        with pytest.raises(FileExistsError, match="already exists"):
            generate_diagnostic_draft(
                preset_name="ifeval-five-task",
                operation_id="diag-001",
                revision="rev-1",
                report_root=".local.dev/campaign/diag-test",
                gold_set=AuthorityRef(path=".local.dev/campaign/gold.json", sha256=GOLD_SHA),
                evidence_key=EvidenceKeyRef(path=".local.dev/campaign/key.json", key_id="key-001"),
                provider_endpoint=ProviderEndpointRef(provider="ollama", endpoint_class="ollama-local"),
                model_variant_id="gemma4:e4b",
                output_path=out,
            )

    def test_draft_applies_overrides(self, tmp_path):
        config, _ = generate_diagnostic_draft(
            preset_name="ifeval-five-task",
            operation_id="diag-001",
            revision="rev-1",
            report_root=".local.dev/campaign/diag-test",
            gold_set=AuthorityRef(path=".local.dev/campaign/gold.json", sha256=GOLD_SHA),
            evidence_key=EvidenceKeyRef(path=".local.dev/campaign/key.json", key_id="key-001"),
            provider_endpoint=ProviderEndpointRef(provider="ollama", endpoint_class="ollama-local"),
            model_variant_id="qwen3:8b",
            overrides=DiagnosticDraftOverrides(
                arm="direct",
                budget=BudgetCeilings(max_requests=10, max_tokens=100000, max_usd=0),
            ),
        )
        assert config.arm == "direct"
        assert config.budget.max_requests == 10

    def test_draft_config_loads_back(self, tmp_path):
        out = tmp_path / "diag.json"
        generate_diagnostic_draft(
            preset_name="ifeval-five-task",
            operation_id="diag-001",
            revision="rev-1",
            report_root=".local.dev/campaign/diag-test",
            gold_set=AuthorityRef(path=".local.dev/campaign/gold.json", sha256=GOLD_SHA),
            evidence_key=EvidenceKeyRef(path=".local.dev/campaign/key.json", key_id="key-001"),
            provider_endpoint=ProviderEndpointRef(provider="ollama", endpoint_class="ollama-local"),
            model_variant_id="gemma4:e4b",
            output_path=out,
        )
        loaded = load_operation_config(out)
        assert isinstance(loaded, DiagnosticConfig)
        assert loaded.model_variant_id == "gemma4:e4b"

    def test_draft_summary_contains_authority_hashes(self, tmp_path):
        _, summary = generate_diagnostic_draft(
            preset_name="ifeval-five-task",
            operation_id="diag-001",
            revision="rev-1",
            report_root=".local.dev/campaign/diag-test",
            gold_set=AuthorityRef(path=".local.dev/campaign/gold.json", sha256=GOLD_SHA),
            evidence_key=EvidenceKeyRef(path=".local.dev/campaign/key.json", key_id="key-001"),
            provider_endpoint=ProviderEndpointRef(provider="ollama", endpoint_class="ollama-local"),
            model_variant_id="gemma4:e4b",
        )
        assert "gold_set" in summary.authority_hashes
        assert summary.authority_hashes["gold_set"] == GOLD_SHA


class TestCampaignDraft:
    def test_draft_creates_config_at_new_path(self, tmp_path):
        out = tmp_path / "camp.json"
        config, summary = generate_campaign_draft(
            preset_name="opendevops-development",
            operation_id="camp-001",
            revision="rev-1",
            report_root=".local.dev/campaign/camp-test",
            gold_set=AuthorityRef(path=".local.dev/campaign/gold.json", sha256=GOLD_SHA),
            evidence_key=EvidenceKeyRef(path=".local.dev/campaign/key.json", key_id="key-001"),
            provider_endpoint=ProviderEndpointRef(provider="ollama", endpoint_class="ollama-local"),
            campaign_id="test-campaign",
            release_version="v2.1.8",
            preregistration=AuthorityRef(path=".local.dev/campaign/prereg.json", sha256=PREREG_SHA),
            profile=AuthorityRef(path=".local.dev/campaign/profile.json", sha256=PROFILE_SHA),
            model_registry=AuthorityRef(path=".local.dev/campaign/registry.json", sha256=REGISTRY_SHA),
            cohort_ids=["cohort-qwen3-8b"],
            output_path=out,
        )
        assert out.exists()
        assert config.content_hash is not None
        assert summary.operation_kind == "campaign"

    def test_draft_rejects_existing_path(self, tmp_path):
        out = tmp_path / "camp.json"
        out.write_text("{}")
        with pytest.raises(FileExistsError, match="already exists"):
            generate_campaign_draft(
                preset_name="opendevops-development",
                operation_id="camp-001",
                revision="rev-1",
                report_root=".local.dev/campaign/camp-test",
                gold_set=AuthorityRef(path=".local.dev/campaign/gold.json", sha256=GOLD_SHA),
                evidence_key=EvidenceKeyRef(path=".local.dev/campaign/key.json", key_id="key-001"),
                provider_endpoint=ProviderEndpointRef(provider="ollama", endpoint_class="ollama-local"),
                campaign_id="test-campaign",
                release_version="v2.1.8",
                preregistration=AuthorityRef(path=".local.dev/campaign/prereg.json", sha256=PREREG_SHA),
                profile=AuthorityRef(path=".local.dev/campaign/profile.json", sha256=PROFILE_SHA),
                model_registry=AuthorityRef(path=".local.dev/campaign/registry.json", sha256=REGISTRY_SHA),
                cohort_ids=["cohort-qwen3-8b"],
                output_path=out,
            )

    def test_draft_applies_overrides(self):
        config, _ = generate_campaign_draft(
            preset_name="opendevops-development",
            operation_id="camp-001",
            revision="rev-1",
            report_root=".local.dev/campaign/camp-test",
            gold_set=AuthorityRef(path=".local.dev/campaign/gold.json", sha256=GOLD_SHA),
            evidence_key=EvidenceKeyRef(path=".local.dev/campaign/key.json", key_id="key-001"),
            provider_endpoint=ProviderEndpointRef(provider="ollama", endpoint_class="ollama-local"),
            campaign_id="test-campaign",
            release_version="v2.1.8",
            preregistration=AuthorityRef(path=".local.dev/campaign/prereg.json", sha256=PREREG_SHA),
            profile=AuthorityRef(path=".local.dev/campaign/profile.json", sha256=PROFILE_SHA),
            model_registry=AuthorityRef(path=".local.dev/campaign/registry.json", sha256=REGISTRY_SHA),
            cohort_ids=["cohort-qwen3-8b"],
            overrides=CampaignDraftOverrides(repetitions=5),
        )
        assert config.repetitions == 5

    def test_draft_summary_contains_all_authority_hashes(self):
        _, summary = generate_campaign_draft(
            preset_name="opendevops-development",
            operation_id="camp-001",
            revision="rev-1",
            report_root=".local.dev/campaign/camp-test",
            gold_set=AuthorityRef(path=".local.dev/campaign/gold.json", sha256=GOLD_SHA),
            evidence_key=EvidenceKeyRef(path=".local.dev/campaign/key.json", key_id="key-001"),
            provider_endpoint=ProviderEndpointRef(provider="ollama", endpoint_class="ollama-local"),
            campaign_id="test-campaign",
            release_version="v2.1.8",
            preregistration=AuthorityRef(path=".local.dev/campaign/prereg.json", sha256=PREREG_SHA),
            profile=AuthorityRef(path=".local.dev/campaign/profile.json", sha256=PROFILE_SHA),
            model_registry=AuthorityRef(path=".local.dev/campaign/registry.json", sha256=REGISTRY_SHA),
            cohort_ids=["cohort-qwen3-8b"],
        )
        assert "gold_set" in summary.authority_hashes
        assert "preregistration" in summary.authority_hashes
        assert "profile" in summary.authority_hashes
        assert "model_registry" in summary.authority_hashes
