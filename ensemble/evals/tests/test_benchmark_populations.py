# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""Tier 1 tests for model-sensitive benchmark populations and the
classifier training protocol.

Verifies that benchmark populations carry source provenance,
redistribution licenses, content hashes, deduplication/contamination
checks, and minimum population sizes. Verifies that the classifier
training protocol defines the binary predicate, source corpus,
annotation/adjudication protocol, label-quality checks, class
prevalence, subgroup policy, and classifier train/development/test
split by leakage-resistant grouping with frozen split hashes.

No external dependencies (no files, network, or DB).
"""

# pyright: reportCallIssue=false

from __future__ import annotations

import pytest
from pydantic import ValidationError

from g8e_evals.benchmark_contract import (
    BenchmarkPopulation,
    BenchmarkPopulationKind,
    DeduplicationCheck,
    compute_population_hash,
)
from g8e_evals.classifier_protocol import (
    AnnotationProtocol,
    ClassifierProtocol,
    LabelQualityCheck,
    LeakageResistantGroup,
    SubgroupPolicy,
    TrainDevTestSplit,
    compute_classifier_protocol_hash,
)


pytestmark = pytest.mark.unit

_VALID_HASH = "a" * 64


def _make_dedup_check() -> DeduplicationCheck:
    return DeduplicationCheck(
        deduplication_method="exact_hash",
        contamination_check_method="ngram_overlap",
        duplicate_tasks_removed=0,
        contaminated_tasks_removed=0,
        check_report_hash=_VALID_HASH,
    )


class TestBenchmarkPopulation:
    def test_population_is_frozen_with_extra_forbid(self):
        """The population model rejects unknown fields and is frozen."""
        with pytest.raises(ValidationError, match="extra_forbidden"):
            BenchmarkPopulation(
                population_id="pop-1",
                population_version="1.0.0",
                kind=BenchmarkPopulationKind.INSTRUCTION_FOLLOWING,
                suite_id="ifeval_subset",
                description="IFEval instruction-following population",
                task_ids=["task-1", "task-2"],
                task_count=2,
                dataset_content_hash=_VALID_HASH,
                source_provenance_hash=_VALID_HASH,
                redistribution_license="apache-2.0",
                deduplication_check=_make_dedup_check(),
                minimum_population=1,
                content_hash=_VALID_HASH,
                unknown_field="rejected",
            )

    def test_population_task_count_must_match_task_ids(self):
        """The task_count must match the number of task_ids."""
        with pytest.raises(ValidationError, match="task_count"):
            BenchmarkPopulation(
                population_id="pop-1",
                population_version="1.0.0",
                kind=BenchmarkPopulationKind.INSTRUCTION_FOLLOWING,
                suite_id="ifeval_subset",
                description="IFEval",
                task_ids=["task-1", "task-2"],
                task_count=3,
                dataset_content_hash=_VALID_HASH,
                source_provenance_hash=_VALID_HASH,
                redistribution_license="apache-2.0",
                deduplication_check=_make_dedup_check(),
                minimum_population=1,
                content_hash=_VALID_HASH,
            )

    def test_population_task_ids_must_be_sorted(self):
        """Task IDs must be sorted."""
        with pytest.raises(ValidationError, match="task_ids must be sorted"):
            BenchmarkPopulation(
                population_id="pop-1",
                population_version="1.0.0",
                kind=BenchmarkPopulationKind.INSTRUCTION_FOLLOWING,
                suite_id="ifeval_subset",
                description="IFEval",
                task_ids=["task-2", "task-1"],
                task_count=2,
                dataset_content_hash=_VALID_HASH,
                source_provenance_hash=_VALID_HASH,
                redistribution_license="apache-2.0",
                deduplication_check=_make_dedup_check(),
                minimum_population=1,
                content_hash=_VALID_HASH,
            )

    def test_population_task_ids_must_be_unique(self):
        """Task IDs must not contain duplicates."""
        with pytest.raises(ValidationError, match="task_ids must not contain duplicates"):
            BenchmarkPopulation(
                population_id="pop-1",
                population_version="1.0.0",
                kind=BenchmarkPopulationKind.INSTRUCTION_FOLLOWING,
                suite_id="ifeval_subset",
                description="IFEval",
                task_ids=["task-1", "task-1"],
                task_count=2,
                dataset_content_hash=_VALID_HASH,
                source_provenance_hash=_VALID_HASH,
                redistribution_license="apache-2.0",
                deduplication_check=_make_dedup_check(),
                minimum_population=1,
                content_hash=_VALID_HASH,
            )

    def test_population_count_must_meet_minimum(self):
        """The task count must meet the minimum population for the kind."""
        with pytest.raises(ValidationError, match="minimum_population"):
            BenchmarkPopulation(
                population_id="pop-1",
                population_version="1.0.0",
                kind=BenchmarkPopulationKind.INSTRUCTION_FOLLOWING,
                suite_id="ifeval_subset",
                description="IFEval",
                task_ids=["task-1"],
                task_count=1,
                dataset_content_hash=_VALID_HASH,
                source_provenance_hash=_VALID_HASH,
                redistribution_license="apache-2.0",
                deduplication_check=_make_dedup_check(),
                minimum_population=10,
                content_hash=_VALID_HASH,
            )

    def test_population_content_hash_must_match(self):
        """The content hash must match the computed hash."""
        ch = compute_population_hash(
            population_id="pop-1",
            population_version="1.0.0",
            kind=BenchmarkPopulationKind.INSTRUCTION_FOLLOWING,
            suite_id="ifeval_subset",
            description="IFEval",
            task_ids=["task-1", "task-2"],
            dataset_content_hash=_VALID_HASH,
            source_provenance_hash=_VALID_HASH,
            redistribution_license="apache-2.0",
            deduplication_check=_make_dedup_check(),
            minimum_population=1,
        )
        with pytest.raises(ValueError, match="content_hash mismatch"):
            BenchmarkPopulation(
                population_id="pop-1",
                population_version="1.0.0",
                kind=BenchmarkPopulationKind.INSTRUCTION_FOLLOWING,
                suite_id="ifeval_subset",
                description="IFEval",
                task_ids=["task-1", "task-2"],
                task_count=2,
                dataset_content_hash=_VALID_HASH,
                source_provenance_hash=_VALID_HASH,
                redistribution_license="apache-2.0",
                deduplication_check=_make_dedup_check(),
                minimum_population=1,
                content_hash="0" * 64,
            )

        pop = BenchmarkPopulation(
            population_id="pop-1",
            population_version="1.0.0",
            kind=BenchmarkPopulationKind.INSTRUCTION_FOLLOWING,
            suite_id="ifeval_subset",
            description="IFEval",
            task_ids=["task-1", "task-2"],
            task_count=2,
            dataset_content_hash=_VALID_HASH,
            source_provenance_hash=_VALID_HASH,
            redistribution_license="apache-2.0",
            deduplication_check=_make_dedup_check(),
            minimum_population=1,
            content_hash=ch,
        )
        assert pop.content_hash == ch

    def test_population_round_trip_serialization(self):
        """The population round-trips through JSON without data loss."""
        import json

        ch = compute_population_hash(
            population_id="pop-1",
            population_version="1.0.0",
            kind=BenchmarkPopulationKind.STRUCTURED_OUTPUT,
            suite_id="tool_sequence",
            description="Tool sequence population",
            task_ids=["task-1", "task-2", "task-3"],
            dataset_content_hash=_VALID_HASH,
            source_provenance_hash=_VALID_HASH,
            redistribution_license="apache-2.0",
            deduplication_check=_make_dedup_check(),
            minimum_population=1,
        )
        pop = BenchmarkPopulation(
            population_id="pop-1",
            population_version="1.0.0",
            kind=BenchmarkPopulationKind.STRUCTURED_OUTPUT,
            suite_id="tool_sequence",
            description="Tool sequence population",
            task_ids=["task-1", "task-2", "task-3"],
            task_count=3,
            dataset_content_hash=_VALID_HASH,
            source_provenance_hash=_VALID_HASH,
            redistribution_license="apache-2.0",
            deduplication_check=_make_dedup_check(),
            minimum_population=1,
            content_hash=ch,
        )
        data = json.loads(pop.model_dump_json())
        restored = BenchmarkPopulation.model_validate(data)
        assert restored.content_hash == pop.content_hash
        assert restored.task_ids == pop.task_ids

    def test_population_deduplication_check_is_frozen(self):
        """The deduplication check rejects unknown fields and is frozen."""
        with pytest.raises(ValidationError, match="extra_forbidden"):
            DeduplicationCheck(
                deduplication_method="exact_hash",
                contamination_check_method="ngram_overlap",
                duplicate_tasks_removed=0,
                contaminated_tasks_removed=0,
                check_report_hash=_VALID_HASH,
                unknown_field="rejected",
            )


class TestClassifierProtocol:
    def _make_split(self) -> TrainDevTestSplit:
        return TrainDevTestSplit(
            split_id="split-1",
            grouping_method="source_document",
            train_group_ids=["doc-1", "doc-2"],
            dev_group_ids=["doc-3"],
            test_group_ids=["doc-4"],
            train_task_count=100,
            dev_task_count=20,
            test_task_count=30,
            train_split_hash=_VALID_HASH,
            dev_split_hash=_VALID_HASH,
            test_split_hash=_VALID_HASH,
        )

    def _make_protocol(
        self,
        split: TrainDevTestSplit | None = None,
    ) -> ClassifierProtocol:
        if split is None:
            split = self._make_split()
        ch = compute_classifier_protocol_hash(
            protocol_id="proto-1",
            protocol_version="1.0.0",
            binary_predicate="allow_block_correctness",
            source_corpus="governance_adversarial",
            annotation_protocol=AnnotationProtocol(
                adjudication_method="majority_vote",
                annotator_count=3,
                agreement_threshold=0.7,
                label_quality_check=LabelQualityCheck(
                    method="cohen_kappa",
                    threshold=0.6,
                    measured_value=0.75,
                    passed=True,
                ),
            ),
            class_prevalence={"positive": 0.5, "negative": 0.5},
            subgroup_policy=SubgroupPolicy(
                declared_subgroups=["weight_class"],
                pooling_rule="stratified",
            ),
            leakage_resistant_groups=[
                LeakageResistantGroup(
                    group_id="doc-1",
                    task_ids=["task-1", "task-2"],
                ),
            ],
            split=split,
        )
        return ClassifierProtocol(
            protocol_id="proto-1",
            protocol_version="1.0.0",
            binary_predicate="allow_block_correctness",
            source_corpus="governance_adversarial",
            annotation_protocol=AnnotationProtocol(
                adjudication_method="majority_vote",
                annotator_count=3,
                agreement_threshold=0.7,
                label_quality_check=LabelQualityCheck(
                    method="cohen_kappa",
                    threshold=0.6,
                    measured_value=0.75,
                    passed=True,
                ),
            ),
            class_prevalence={"positive": 0.5, "negative": 0.5},
            subgroup_policy=SubgroupPolicy(
                declared_subgroups=["weight_class"],
                pooling_rule="stratified",
            ),
            leakage_resistant_groups=[
                LeakageResistantGroup(
                    group_id="doc-1",
                    task_ids=["task-1", "task-2"],
                ),
            ],
            split=split,
            content_hash=ch,
        )

    def test_protocol_is_frozen_with_extra_forbid(self):
        """The protocol model rejects unknown fields and is frozen."""
        with pytest.raises(ValidationError, match="extra_forbidden"):
            ClassifierProtocol(
                protocol_id="proto-1",
                protocol_version="1.0.0",
                binary_predicate="allow_block_correctness",
                source_corpus="governance_adversarial",
                annotation_protocol=AnnotationProtocol(
                    adjudication_method="majority_vote",
                    annotator_count=3,
                    agreement_threshold=0.7,
                    label_quality_check=LabelQualityCheck(
                        method="cohen_kappa",
                        threshold=0.6,
                        measured_value=0.75,
                        passed=True,
                    ),
                ),
                class_prevalence={"positive": 0.5, "negative": 0.5},
                subgroup_policy=SubgroupPolicy(
                    declared_subgroups=["weight_class"],
                    pooling_rule="stratified",
                ),
                leakage_resistant_groups=[
                    LeakageResistantGroup(
                        group_id="doc-1",
                        task_ids=["task-1", "task-2"],
                    ),
                ],
                split=self._make_split(),
                content_hash=_VALID_HASH,
                unknown_field="rejected",
            )

    def test_protocol_content_hash_must_match(self):
        """The content hash must match the computed hash."""
        with pytest.raises(ValueError, match="content_hash mismatch"):
            ClassifierProtocol(
                protocol_id="proto-1",
                protocol_version="1.0.0",
                binary_predicate="allow_block_correctness",
                source_corpus="governance_adversarial",
                annotation_protocol=AnnotationProtocol(
                    adjudication_method="majority_vote",
                    annotator_count=3,
                    agreement_threshold=0.7,
                    label_quality_check=LabelQualityCheck(
                        method="cohen_kappa",
                        threshold=0.6,
                        measured_value=0.75,
                        passed=True,
                    ),
                ),
                class_prevalence={"positive": 0.5, "negative": 0.5},
                subgroup_policy=SubgroupPolicy(
                    declared_subgroups=["weight_class"],
                    pooling_rule="stratified",
                ),
                leakage_resistant_groups=[
                    LeakageResistantGroup(
                        group_id="doc-1",
                        task_ids=["task-1", "task-2"],
                    ),
                ],
                split=self._make_split(),
                content_hash="0" * 64,
            )

    def test_protocol_class_prevalence_must_sum_to_one(self):
        """Class prevalence values must sum to 1.0."""
        protocol = self._make_protocol()
        assert protocol.class_prevalence is not None
        total = sum(protocol.class_prevalence.values())
        assert abs(total - 1.0) < 1e-9

    def test_protocol_rejects_prevalence_not_summing_to_one(self):
        """Class prevalence values that do not sum to 1.0 are rejected."""
        with pytest.raises(ValidationError, match="class_prevalence"):
            ClassifierProtocol(
                protocol_id="proto-1",
                protocol_version="1.0.0",
                binary_predicate="allow_block_correctness",
                source_corpus="governance_adversarial",
                annotation_protocol=AnnotationProtocol(
                    adjudication_method="majority_vote",
                    annotator_count=3,
                    agreement_threshold=0.7,
                    label_quality_check=LabelQualityCheck(
                        method="cohen_kappa",
                        threshold=0.6,
                        measured_value=0.75,
                        passed=True,
                    ),
                ),
                class_prevalence={"positive": 0.6, "negative": 0.6},
                subgroup_policy=SubgroupPolicy(
                    declared_subgroups=["weight_class"],
                    pooling_rule="stratified",
                ),
                leakage_resistant_groups=[
                    LeakageResistantGroup(
                        group_id="doc-1",
                        task_ids=["task-1", "task-2"],
                    ),
                ],
                split=self._make_split(),
                content_hash=_VALID_HASH,
            )

    def test_protocol_split_group_ids_must_not_overlap(self):
        """Train, dev, and test group IDs must not overlap."""
        with pytest.raises(ValidationError, match="overlap"):
            TrainDevTestSplit(
                split_id="bad-split",
                grouping_method="source_document",
                train_group_ids=["doc-1", "doc-3"],
                dev_group_ids=["doc-3"],
                test_group_ids=["doc-4"],
                train_task_count=100,
                dev_task_count=20,
                test_task_count=30,
                train_split_hash=_VALID_HASH,
                dev_split_hash=_VALID_HASH,
                test_split_hash=_VALID_HASH,
            )

    def test_protocol_round_trip_serialization(self):
        """The protocol round-trips through JSON without data loss."""
        import json

        protocol = self._make_protocol()
        data = json.loads(protocol.model_dump_json())
        restored = ClassifierProtocol.model_validate(data)
        assert restored.content_hash == protocol.content_hash
        assert restored.split.train_group_ids == protocol.split.train_group_ids

    def test_protocol_label_quality_check_must_be_passed(self):
        """The label quality check must have passed=True."""
        with pytest.raises(ValidationError, match=r"label_quality_check.*passed"):
            AnnotationProtocol(
                adjudication_method="majority_vote",
                annotator_count=3,
                agreement_threshold=0.7,
                label_quality_check=LabelQualityCheck(
                    method="cohen_kappa",
                    threshold=0.6,
                    measured_value=0.4,
                    passed=False,
                ),
            )

    def test_protocol_leakage_resistant_group_ids_must_be_unique(self):
        """Leakage-resistant group IDs must be unique."""
        with pytest.raises(ValidationError, match=r"duplicate.*group_id"):
            ClassifierProtocol(
                protocol_id="proto-1",
                protocol_version="1.0.0",
                binary_predicate="allow_block_correctness",
                source_corpus="governance_adversarial",
                annotation_protocol=AnnotationProtocol(
                    adjudication_method="majority_vote",
                    annotator_count=3,
                    agreement_threshold=0.7,
                    label_quality_check=LabelQualityCheck(
                        method="cohen_kappa",
                        threshold=0.6,
                        measured_value=0.75,
                        passed=True,
                    ),
                ),
                class_prevalence={"positive": 0.5, "negative": 0.5},
                subgroup_policy=SubgroupPolicy(
                    declared_subgroups=["weight_class"],
                    pooling_rule="stratified",
                ),
                leakage_resistant_groups=[
                    LeakageResistantGroup(
                        group_id="doc-1",
                        task_ids=["task-1", "task-2"],
                    ),
                    LeakageResistantGroup(
                        group_id="doc-1",
                        task_ids=["task-3"],
                    ),
                ],
                split=self._make_split(),
                content_hash=_VALID_HASH,
            )

    def test_protocol_split_hashes_are_frozen_before_tuning(self):
        """Train, dev, and test split hashes are frozen before tuning."""
        split = self._make_split()
        assert split.train_split_hash is not None
        assert split.dev_split_hash is not None
        assert split.test_split_hash is not None
        assert len(split.train_split_hash) == 64
        assert len(split.dev_split_hash) == 64
        assert len(split.test_split_hash) == 64
