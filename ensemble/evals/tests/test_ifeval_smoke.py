# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

import hashlib
import json
from pathlib import Path
from types import SimpleNamespace

import pytest

from g8e_evals.benchmarks.ifeval import import_subset
from g8e_evals.benchmarks.ifeval.import_subset import select_rows, write_subset
from g8e_evals.benchmarks.ifeval.loader import IFEvalLoader
from g8e_evals.benchmarks.ifeval.provenance import load_provenance
from g8e_evals.benchmarks.ifeval.verifier import IFEvalVerifier
from g8e_evals.schema import PolicyOutcome, RejectionLayer


@pytest.mark.integration
def test_ifeval_loader_validates_provenance():
    base_dir = Path(__file__).parent.parent
    gold_set = base_dir / "gold_sets/ifeval_subset/input_data.jsonl"
    loader = IFEvalLoader(gold_set)
    tasks = list(loader.load())
    assert len(tasks) == 5
    assert tasks[0].id == "1001"
    assert "not allowed to use any commas" in tasks[0].prompt


@pytest.mark.integration
def test_ifeval_loader_preserves_typed_policy_expectations(tmp_path: Path, monkeypatch):
    dataset = tmp_path / "input_data.jsonl"
    dataset.write_text(json.dumps({
        "key": 1,
        "prompt": "Do not edit the protected file.",
        "instruction_id_list": ["punctuation:no_comma"],
        "kwargs": [{}],
        "expected_action_class": "FILE_EDIT",
        "expected_allow_block_outcome": "block",
        "expected_rejection_layer": "l1_doctrine",
    }) + "\n")
    monkeypatch.setattr(
        "g8e_evals.benchmarks.ifeval.loader.load_provenance",
        lambda _path: SimpleNamespace(selected_keys=[1]),
    )
    monkeypatch.setattr(
        "g8e_evals.benchmarks.ifeval.loader.validate_dataset",
        lambda _path, _provenance: None,
    )

    task = next(iter(IFEvalLoader(dataset).load()))

    assert task.metadata.expected_allow_block_outcome == PolicyOutcome.BLOCK
    assert task.metadata.expected_rejection_layer == RejectionLayer.L1_DOCTRINE


@pytest.mark.integration
def test_ifeval_subset_import_is_reproducible(tmp_path: Path):
    base_dir = Path(__file__).parent.parent
    gold_set = base_dir / "gold_sets/ifeval_subset/input_data.jsonl"
    provenance = load_provenance(gold_set.with_name("provenance.json"))
    source_fixture = base_dir / provenance.transformation.fixture_path
    destination = tmp_path / "input_data.jsonl"

    assert hashlib.sha256(source_fixture.read_bytes()).hexdigest() == provenance.transformation.fixture_sha256
    write_subset(destination, select_rows(source_fixture.read_text().splitlines()))

    assert hashlib.sha256(destination.read_bytes()).hexdigest() == provenance.output.sha256
    assert destination.read_bytes() == gold_set.read_bytes()


@pytest.mark.integration
def test_ifeval_subset_provenance_matches_source_and_transformation():
    base_dir = Path(__file__).parent.parent
    provenance = load_provenance(base_dir / "gold_sets/ifeval_subset/provenance.json")
    transformation = base_dir / provenance.transformation.code_path

    assert provenance.source.url == import_subset.UPSTREAM_URL
    assert provenance.source.revision == import_subset.UPSTREAM_REVISION
    assert provenance.source.sha256 == import_subset.UPSTREAM_SHA256
    assert provenance.source.license_spdx == "Apache-2.0"
    assert provenance.selected_keys == list(import_subset.SELECTED_KEYS)
    assert hashlib.sha256(transformation.read_bytes()).hexdigest() == provenance.transformation.code_sha256


@pytest.mark.integration
def test_ifeval_loader_rejects_dataset_not_matching_provenance(tmp_path: Path):
    base_dir = Path(__file__).parent.parent
    gold_set_dir = base_dir / "gold_sets/ifeval_subset"
    dataset = tmp_path / "input_data.jsonl"
    dataset.write_bytes((gold_set_dir / "input_data.jsonl").read_bytes() + b'{"key": 9999}\n')
    dataset.with_name("provenance.json").write_bytes((gold_set_dir / "provenance.json").read_bytes())

    with pytest.raises(ValueError, match="SHA-256 mismatch"):
        list(IFEvalLoader(dataset).load())


@pytest.mark.unit
def test_ifeval_verifier_punctuation():
    verifier = IFEvalVerifier()
    # Task 1001: no comma
    score = verifier.verify("1001", "prompt", "This is fine.", ["punctuation:no_comma"], [{}])
    assert score.passed

    score = verifier.verify("1001", "prompt", "This is not fine, though.", ["punctuation:no_comma"], [{}])
    assert not score.passed

@pytest.mark.unit
def test_ifeval_verifier_uppercase():
    verifier = IFEvalVerifier()
    # Task 1002: uppercase
    score = verifier.verify("1002", "prompt", "ALL UPPERCASE", ["change_case:english_capital"], [{}])
    assert score.passed

    score = verifier.verify("1002", "prompt", "Not all uppercase", ["change_case:english_capital"], [{}])
    assert not score.passed

@pytest.mark.unit
def test_ifeval_verifier_lowercase():
    verifier = IFEvalVerifier()
    score = verifier.verify(
        "1019",
        "prompt",
        "is this a question?",
        ["change_case:english_lowercase"],
        [{}],
    )
    assert score.passed

    score = verifier.verify(
        "1019",
        "prompt",
        "Is this a question?",
        ["change_case:english_lowercase"],
        [{}],
    )
    assert not score.passed


@pytest.mark.unit
def test_ifeval_verifier_json():
    verifier = IFEvalVerifier()
    # Task 1003: JSON
    score = verifier.verify("1003", "prompt", '{"name": "test"}', ["detectable_format:json_format"], [{}])
    assert score.passed

    score = verifier.verify("1003", "prompt", "not json", ["detectable_format:json_format"], [{}])
    assert not score.passed

@pytest.mark.unit
def test_ifeval_verifier_min_words():
    verifier = IFEvalVerifier()
    # Task 1004: min 10 words
    answer = "one two three four five six seven eight nine ten"
    score = verifier.verify(
        "1004",
        "prompt",
        answer,
        ["length_constraints:number_words"],
        [{"relation": "at least", "num_words": 10}],
    )
    assert score.passed

    answer = "too short"
    score = verifier.verify(
        "1004",
        "prompt",
        answer,
        ["length_constraints:number_words"],
        [{"relation": "at least", "num_words": 10}],
    )
    assert not score.passed

@pytest.mark.unit
def test_ifeval_verifier_forbidden_words():
    verifier = IFEvalVerifier()
    # Task 1005: forbidden word 'apple'
    score = verifier.verify("1005", "prompt", "I like oranges", ["keywords:forbidden_words"], [{"forbidden_words": ["apple"]}])
    assert score.passed

    score = verifier.verify("1005", "prompt", "I like apple pie", ["keywords:forbidden_words"], [{"forbidden_words": ["apple"]}])
    assert not score.passed


@pytest.mark.unit
def test_ifeval_verifier_rejects_unsupported_language_instruction():
    verifier = IFEvalVerifier()
    score = verifier.verify(
        "1006",
        "prompt",
        "This answer is nonempty",
        ["language:response_language"],
        [{"language": "english"}],
    )

    assert not score.passed

