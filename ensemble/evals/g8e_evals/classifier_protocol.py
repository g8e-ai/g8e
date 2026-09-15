# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""Frozen classifier training and evaluation protocol for the campaign.

Defines the binary predicate, source corpus, annotation/adjudication
protocol, label-quality checks, class prevalence, subgroup policy, and
classifier train/development/test split by leakage-resistant grouping
where related examples or source documents exist. All split hashes are
frozen before tuning.

The classifier campaign (Phase 5) follows independently after its data
and training protocol pass review. It is not a blocker for the
generative campaign.

Every model is frozen with ``extra="forbid"``. Content hashes are
SHA-256 over canonical JSON (sorted keys, no extra whitespace).
"""

from __future__ import annotations

import hashlib
import json
from typing import Self

from pydantic import BaseModel, ConfigDict, Field, model_validator


CLASSIFIER_PROTOCOL_VERSION = "1.0.0"


def _sha256(data: str) -> str:
    return hashlib.sha256(data.encode()).hexdigest()


class LabelQualityCheck(BaseModel):
    """Typed label-quality check for the annotation protocol.

    Records the quality measurement method, the pass threshold, the
    measured value, and whether the check passed. The check must pass
    before the protocol is accepted.
    """

    model_config = ConfigDict(extra="forbid", frozen=True)

    method: str = Field(
        min_length=1,
        description="Quality measurement method (e.g. cohen_kappa, fleiss_kappa).",
    )
    threshold: float = Field(
        gt=0.0, lt=1.0,
        description="Minimum acceptable quality value.",
    )
    measured_value: float = Field(
        ge=0.0, le=1.0,
        description="Measured quality value.",
    )
    passed: bool = Field(description="Whether the quality check passed (measured_value >= threshold).")

    @model_validator(mode="after")
    def _validate_passed_consistency(self) -> Self:
        if self.passed and self.measured_value < self.threshold:
            raise ValueError(
                f"label_quality_check passed=True but measured_value "
                f"({self.measured_value}) < threshold ({self.threshold})"
            )
        if not self.passed:
            raise ValueError(
                f"label_quality_check passed must be True; "
                f"measured_value ({self.measured_value}) did not meet "
                f"threshold ({self.threshold})"
            )
        return self


class AnnotationProtocol(BaseModel):
    """Typed annotation and adjudication protocol.

    Records the adjudication method, number of annotators, agreement
    threshold, and label-quality check. The protocol is frozen before
    annotation begins.
    """

    model_config = ConfigDict(extra="forbid", frozen=True)

    adjudication_method: str = Field(
        min_length=1,
        description="Adjudication method (e.g. majority_vote, consensus).",
    )
    annotator_count: int = Field(ge=1, description="Number of annotators per task.")
    agreement_threshold: float = Field(
        gt=0.0, lt=1.0,
        description="Minimum agreement fraction required for adjudication.",
    )
    label_quality_check: LabelQualityCheck = Field(
        description="Label-quality check that must pass before the protocol is accepted.",
    )


class SubgroupPolicy(BaseModel):
    """Typed subgroup policy for the classifier protocol.

    Declares the subgroups of interest and the pooling rule. The policy
    is frozen before tuning.
    """

    model_config = ConfigDict(extra="forbid", frozen=True)

    declared_subgroups: list[str] = Field(
        min_length=1,
        description="Declared subgroup dimensions (e.g. weight_class, architecture).",
    )
    pooling_rule: str = Field(
        min_length=1,
        description="Pooling rule across subgroups (e.g. stratified, pooled).",
    )

    @model_validator(mode="after")
    def _validate_subgroups(self) -> Self:
        if self.declared_subgroups != sorted(self.declared_subgroups):
            raise ValueError(
                f"declared_subgroups must be sorted: {self.declared_subgroups}"
            )
        if len(self.declared_subgroups) != len(set(self.declared_subgroups)):
            raise ValueError(
                f"declared_subgroups must not contain duplicates: {self.declared_subgroups}"
            )
        return self


class LeakageResistantGroup(BaseModel):
    """One leakage-resistant grouping for the classifier split.

    Groups related examples or source documents so that all examples in
    a group are assigned to the same split (train, dev, or test). This
    prevents leakage between splits when examples share a source
    document or are otherwise related.
    """

    model_config = ConfigDict(extra="forbid", frozen=True)

    group_id: str = Field(min_length=1, description="Unique group identifier.")
    task_ids: list[str] = Field(
        min_length=1,
        description="Task IDs in this group.",
    )

    @model_validator(mode="after")
    def _validate_task_ids(self) -> Self:
        if len(self.task_ids) != len(set(self.task_ids)):
            raise ValueError(
                f"leakage_resistant_group {self.group_id!r} has duplicate task_ids"
            )
        return self


class TrainDevTestSplit(BaseModel):
    """Frozen train/development/test split by leakage-resistant grouping.

    Each split is defined by the leakage-resistant group IDs assigned to
    it. Group IDs must not overlap between splits. Split hashes are
    frozen before tuning.
    """

    model_config = ConfigDict(extra="forbid", frozen=True)

    split_id: str = Field(min_length=1, description="Unique split identifier.")
    grouping_method: str = Field(
        min_length=1,
        description="Grouping method (e.g. source_document, topic_cluster).",
    )
    train_group_ids: list[str] = Field(
        min_length=1,
        description="Group IDs assigned to the train split.",
    )
    dev_group_ids: list[str] = Field(
        min_length=1,
        description="Group IDs assigned to the development split.",
    )
    test_group_ids: list[str] = Field(
        min_length=1,
        description="Group IDs assigned to the test split.",
    )
    train_task_count: int = Field(ge=1, description="Number of tasks in the train split.")
    dev_task_count: int = Field(ge=1, description="Number of tasks in the development split.")
    test_task_count: int = Field(ge=1, description="Number of tasks in the test split.")
    train_split_hash: str = Field(
        min_length=64, max_length=64,
        description="SHA-256 of the train split content, frozen before tuning.",
    )
    dev_split_hash: str = Field(
        min_length=64, max_length=64,
        description="SHA-256 of the development split content, frozen before tuning.",
    )
    test_split_hash: str = Field(
        min_length=64, max_length=64,
        description="SHA-256 of the test split content, frozen before tuning.",
    )

    @model_validator(mode="after")
    def _validate_no_overlap(self) -> Self:
        train_set = set(self.train_group_ids)
        dev_set = set(self.dev_group_ids)
        test_set = set(self.test_group_ids)
        if train_set & dev_set:
            raise ValueError(
                f"train and dev group IDs overlap: {sorted(train_set & dev_set)}"
            )
        if train_set & test_set:
            raise ValueError(
                f"train and test group IDs overlap: {sorted(train_set & test_set)}"
            )
        if dev_set & test_set:
            raise ValueError(
                f"dev and test group IDs overlap: {sorted(dev_set & test_set)}"
            )
        return self


class ClassifierProtocol(BaseModel):
    """Frozen classifier training and evaluation protocol.

    Defines the binary predicate, source corpus, annotation/adjudication
    protocol, label-quality checks, class prevalence, subgroup policy,
    leakage-resistant groupings, and train/development/test split. All
    split hashes are frozen before tuning.

    The protocol is independently gated: the classifier campaign (Phase
    5) follows independently after its data and training protocol pass
    review. It is not a blocker for the generative campaign.
    """

    model_config = ConfigDict(extra="forbid", frozen=True)

    protocol_id: str = Field(min_length=1, description="Unique protocol identifier.")
    protocol_version: str = Field(min_length=1, description="Protocol schema version.")

    binary_predicate: str = Field(
        min_length=1,
        description="Binary predicate the classifier learns (e.g. allow_block_correctness).",
    )
    source_corpus: str = Field(
        min_length=1,
        description="Source corpus for the classification task.",
    )

    annotation_protocol: AnnotationProtocol = Field(
        description="Annotation and adjudication protocol.",
    )

    class_prevalence: dict[str, float] = Field(
        min_length=1,
        description="Class prevalence as a mapping from class label to proportion. Values must sum to 1.0.",
    )

    subgroup_policy: SubgroupPolicy = Field(
        description="Subgroup policy for the classifier protocol.",
    )

    leakage_resistant_groups: list[LeakageResistantGroup] = Field(
        min_length=1,
        description="Leakage-resistant groupings for the split.",
    )

    split: TrainDevTestSplit = Field(
        description="Frozen train/development/test split by leakage-resistant grouping.",
    )

    content_hash: str = Field(
        min_length=64, max_length=64,
        description="SHA-256 over canonical JSON of the protocol.",
    )

    @model_validator(mode="after")
    def _validate_protocol(self) -> Self:
        total = sum(self.class_prevalence.values())
        if abs(total - 1.0) > 1e-9:
            raise ValueError(
                f"class_prevalence values must sum to 1.0: got {total}"
            )

        group_ids = [g.group_id for g in self.leakage_resistant_groups]
        if len(group_ids) != len(set(group_ids)):
            seen: set[str] = set()
            dupes: list[str] = []
            for gid in group_ids:
                if gid in seen:
                    dupes.append(gid)
                seen.add(gid)
            raise ValueError(f"duplicate group_id: {sorted(set(dupes))}")

        expected = compute_classifier_protocol_hash(
            protocol_id=self.protocol_id,
            protocol_version=self.protocol_version,
            binary_predicate=self.binary_predicate,
            source_corpus=self.source_corpus,
            annotation_protocol=self.annotation_protocol,
            class_prevalence=self.class_prevalence,
            subgroup_policy=self.subgroup_policy,
            leakage_resistant_groups=self.leakage_resistant_groups,
            split=self.split,
        )
        if self.content_hash != expected:
            raise ValueError(
                f"classifier protocol content_hash mismatch: "
                f"declared {self.content_hash!r}, computed {expected!r}"
            )
        return self


def compute_classifier_protocol_hash(
    *,
    protocol_id: str,
    protocol_version: str,
    binary_predicate: str,
    source_corpus: str,
    annotation_protocol: AnnotationProtocol,
    class_prevalence: dict[str, float],
    subgroup_policy: SubgroupPolicy,
    leakage_resistant_groups: list[LeakageResistantGroup],
    split: TrainDevTestSplit,
) -> str:
    """Compute the content hash for a classifier protocol."""
    payload = json.dumps(
        {
            "protocol_id": protocol_id,
            "protocol_version": protocol_version,
            "binary_predicate": binary_predicate,
            "source_corpus": source_corpus,
            "annotation_protocol": annotation_protocol.model_dump(mode="json"),
            "class_prevalence": dict(sorted(class_prevalence.items())),
            "subgroup_policy": subgroup_policy.model_dump(mode="json"),
            "leakage_resistant_groups": [
                g.model_dump(mode="json")
                for g in sorted(leakage_resistant_groups, key=lambda g: g.group_id)
            ],
            "split": split.model_dump(mode="json"),
        },
        allow_nan=False,
        ensure_ascii=False,
        separators=(",", ":"),
        sort_keys=True,
    )
    return _sha256(payload)


__all__ = [
    "CLASSIFIER_PROTOCOL_VERSION",
    "AnnotationProtocol",
    "ClassifierProtocol",
    "LabelQualityCheck",
    "LeakageResistantGroup",
    "SubgroupPolicy",
    "TrainDevTestSplit",
    "compute_classifier_protocol_hash",
]
