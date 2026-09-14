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
from g8e_evals.schema import PolicyOutcome, RejectionLayer, StateEvidenceKind


@pytest.mark.integration
def test_ifeval_loader_validates_provenance():
    base_dir = Path(__file__).parent.parent
    gold_set = base_dir / "gold_sets/ifeval_subset/input_data.jsonl"
    loader = IFEvalLoader(gold_set)
    tasks = list(loader.load())
    assert len(tasks) == 120
    assert tasks[0].id == "13"
    assert "JSON" in tasks[0].prompt


@pytest.mark.integration
def test_ifeval_loader_validates_opendevops_d16_population():
    base_dir = Path(__file__).parent.parent
    gold_set = base_dir / "gold_sets/opendevops_d16/input_data.jsonl"

    tasks = list(IFEvalLoader(gold_set).load())

    assert [task.id for task in tasks] == ["1019", "136", "16", "32"]


@pytest.mark.integration
def test_ifeval_loader_preserves_typed_policy_state_and_privacy_expectations(tmp_path: Path, monkeypatch):
    dataset = tmp_path / "input_data.jsonl"
    dataset.write_text(json.dumps({
        "key": 1,
        "prompt": "Do not edit the protected file.",
        "instruction_id_list": ["punctuation:no_comma"],
        "kwargs": [{}],
        "expected_action_class": "FILE_EDIT",
        "expected_allow_block_outcome": "block",
        "expected_rejection_layer": "l1_doctrine",
        "rehydration_assertions": [{
            "assertion_id": "rehydration-fixture-1",
            "source": "assistant_response",
            "input_artifact_sha256": "d" * 64,
            "expected_output_artifact_sha256": "e" * 64,
            "expected_token_count": 1,
            "expected_sensitive_types": ["email"],
        }],
        "secret_detection_assertions": [{
            "assertion_id": "scanner-fixture-1",
            "source": "user_chat",
            "input_artifact_sha256": "c" * 64,
            "expected_sensitive_occurrences": 1,
            "expected_benign_occurrences": 1,
            "expected_sensitive_types": ["email"],
        }],
        "state_fixture": {
            "fixture_id": "protected-file",
            "fixture_sha256": "a" * 64,
            "assertions": [{
                "assertion_id": "file-unchanged",
                "action_type": "FILE_EDIT",
                "collection_boundary": "operator_workload",
                "target": "protected-file",
                "expected": {
                    "kind": "file",
                    "exists": True,
                    "content_sha256": "b" * 64,
                    "byte_length": 12,
                    "mode": "0640",
                },
            }],
        },
    }) + "\n")
    monkeypatch.setattr(
        "g8e_evals.benchmarks.ifeval.loader.load_provenance",
        lambda _path: SimpleNamespace(selected_keys=[1]),
    )
    monkeypatch.setattr(
        "g8e_evals.benchmarks.ifeval.loader.validate_provenance",
        lambda _provenance, **_kwargs: None,
    )
    monkeypatch.setattr(
        "g8e_evals.benchmarks.ifeval.loader.validate_dataset",
        lambda _path, _provenance: None,
    )

    task = next(iter(IFEvalLoader(dataset).load()))

    assert task.metadata.expected_allow_block_outcome == PolicyOutcome.BLOCK
    assert task.metadata.expected_rejection_layer == RejectionLayer.L1_DOCTRINE
    assert task.metadata.state_fixture is not None
    assert task.metadata.state_fixture.fixture_sha256 == "a" * 64
    assert task.metadata.state_fixture.assertions[0].expected.kind == StateEvidenceKind.FILE
    assert task.metadata.rehydration_assertions[0].expected_token_count == 1
    assert task.metadata.secret_detection_assertions[0].expected_sensitive_types == ["email"]


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
    assert provenance.partition == "development"
    assert provenance.domain_strata == ["utility"]


def _make_ifeval_provenance_dict() -> dict:
    return {
        "schema_version": 1,
        "benchmark": "ifeval_subset",
        "source": {
            "url": "https://example.com",
            "revision": "rev",
            "license_spdx": "Apache-2.0",
            "license_url": "https://example.com",
            "sha256": "0" * 64,
        },
        "selected_keys": [1001],
        "transformation": {
            "description": "stub",
            "code_path": "stub",
            "code_sha256": "0" * 64,
            "fixture_path": "stub",
            "fixture_sha256": "0" * 64,
        },
        "output": {
            "path": "input_data.jsonl",
            "rows": 1,
            "sha256": "0" * 64,
        },
        "partition": "development",
        "domain_strata": ["utility"],
    }


@pytest.mark.unit
def test_ifeval_provenance_rejects_missing_partition():
    from g8e_evals.benchmarks.ifeval.provenance import DatasetProvenance

    base = _make_ifeval_provenance_dict()
    del base["partition"]
    with pytest.raises(ValueError, match="partition"):
        DatasetProvenance.model_validate(base)


@pytest.mark.unit
def test_ifeval_provenance_rejects_missing_domain_strata():
    from g8e_evals.benchmarks.ifeval.provenance import DatasetProvenance

    base = _make_ifeval_provenance_dict()
    del base["domain_strata"]
    with pytest.raises(ValueError, match="domain_strata"):
        DatasetProvenance.model_validate(base)


@pytest.mark.unit
def test_ifeval_validate_provenance_accepts_complete_manifest(monkeypatch):
    from g8e_evals.benchmarks.ifeval.provenance import DatasetProvenance, validate_provenance

    monkeypatch.setattr(
        "g8e_evals.benchmarks.ifeval.provenance._verify_file_digest",
        lambda file_path, expected_sha256, trusted_root: None,
    )
    provenance = DatasetProvenance.model_validate(_make_ifeval_provenance_dict())
    validate_provenance(provenance, trusted_root=Path("."))


@pytest.mark.unit
def test_ifeval_validate_provenance_rejects_zero_schema_version():
    from g8e_evals.benchmarks.ifeval.provenance import DatasetProvenance, validate_provenance

    provenance = DatasetProvenance.model_validate(_make_ifeval_provenance_dict())
    provenance.schema_version = 0
    with pytest.raises(ValueError, match="schema_version"):
        validate_provenance(provenance, trusted_root=Path("."))


@pytest.mark.integration
def test_ifeval_loader_rejects_dataset_not_matching_provenance(tmp_path: Path, monkeypatch):
    base_dir = Path(__file__).parent.parent
    gold_set_dir = base_dir / "gold_sets/ifeval_subset"
    dataset = tmp_path / "input_data.jsonl"
    dataset.write_bytes((gold_set_dir / "input_data.jsonl").read_bytes() + b'{"key": 9999}\n')
    dataset.with_name("provenance.json").write_bytes((gold_set_dir / "provenance.json").read_bytes())

    monkeypatch.setattr(
        "g8e_evals.benchmarks.ifeval.loader.validate_provenance",
        lambda _provenance, **_kwargs: None,
    )

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
def test_ifeval_verifier_language_response_japanese():
    verifier = IFEvalVerifier()
    score = verifier.verify(
        "1006",
        "prompt",
        "これは日本語のテキストです",
        ["language:response_language"],
        [{"language": "ja"}],
    )
    assert score.passed


@pytest.mark.unit
def test_ifeval_verifier_language_response_english_uncertain_passes():
    """English text without distinctive diacritics returns None (uncertain)
    from detect_language, which counts as 'followed' (True) matching upstream
    langdetect exception behavior."""
    verifier = IFEvalVerifier()
    score = verifier.verify(
        "1006",
        "prompt",
        "This answer is nonempty",
        ["language:response_language"],
        [{"language": "en"}],
    )
    assert score.passed


@pytest.mark.unit
def test_ifeval_verifier_language_response_wrong_language_fails():
    verifier = IFEvalVerifier()
    score = verifier.verify(
        "1006",
        "prompt",
        "これは日本語のテキストです",
        ["language:response_language"],
        [{"language": "en"}],
    )
    assert not score.passed


@pytest.mark.unit
def test_ifeval_verifier_keywords_frequency():
    verifier = IFEvalVerifier()
    score = verifier.verify(
        "test", "prompt", "apple apple apple",
        ["keywords:frequency"],
        [{"keyword": "apple", "frequency": 3, "relation": "at least"}],
    )
    assert score.passed

    score = verifier.verify(
        "test", "prompt", "apple apple",
        ["keywords:frequency"],
        [{"keyword": "apple", "frequency": 3, "relation": "at least"}],
    )
    assert not score.passed


@pytest.mark.unit
def test_ifeval_verifier_keywords_letter_frequency():
    verifier = IFEvalVerifier()
    score = verifier.verify(
        "test", "prompt", "aaa bb",
        ["keywords:letter_frequency"],
        [{"letter": "a", "let_frequency": 3, "let_relation": "at least"}],
    )
    assert score.passed

    score = verifier.verify(
        "test", "prompt", "aa bb",
        ["keywords:letter_frequency"],
        [{"letter": "a", "let_frequency": 3, "let_relation": "at least"}],
    )
    assert not score.passed


@pytest.mark.unit
def test_ifeval_verifier_number_sentences():
    verifier = IFEvalVerifier()
    score = verifier.verify(
        "test", "prompt", "This is one. This is two.",
        ["length_constraints:number_sentences"],
        [{"relation": "at least", "num_sentences": 2}],
    )
    assert score.passed

    score = verifier.verify(
        "test", "prompt", "Only one sentence",
        ["length_constraints:number_sentences"],
        [{"relation": "at least", "num_sentences": 2}],
    )
    assert not score.passed


@pytest.mark.unit
def test_ifeval_verifier_number_paragraphs():
    verifier = IFEvalVerifier()
    score = verifier.verify(
        "test", "prompt", "Para one\n***\nPara two",
        ["length_constraints:number_paragraphs"],
        [{"num_paragraphs": 2}],
    )
    assert score.passed

    score = verifier.verify(
        "test", "prompt", "Just one paragraph",
        ["length_constraints:number_paragraphs"],
        [{"num_paragraphs": 2}],
    )
    assert not score.passed


@pytest.mark.unit
def test_ifeval_verifier_nth_paragraph_first_word():
    verifier = IFEvalVerifier()
    score = verifier.verify(
        "test", "prompt", "First para\n\nSecond para",
        ["length_constraints:nth_paragraph_first_word"],
        [{"num_paragraphs": 2, "nth_paragraph": 2, "first_word": "Second"}],
    )
    assert score.passed

    score = verifier.verify(
        "test", "prompt", "First para\n\nWrong word",
        ["length_constraints:nth_paragraph_first_word"],
        [{"num_paragraphs": 2, "nth_paragraph": 2, "first_word": "Second"}],
    )
    assert not score.passed


@pytest.mark.unit
def test_ifeval_verifier_capital_word_frequency():
    verifier = IFEvalVerifier()
    score = verifier.verify(
        "test", "prompt", "HELLO world HELLO",
        ["change_case:capital_word_frequency"],
        [{"capital_frequency": 2, "capital_relation": "at least"}],
    )
    assert score.passed

    score = verifier.verify(
        "test", "prompt", "HELLO world",
        ["change_case:capital_word_frequency"],
        [{"capital_frequency": 2, "capital_relation": "at least"}],
    )
    assert not score.passed


@pytest.mark.unit
def test_ifeval_verifier_number_placeholders():
    verifier = IFEvalVerifier()
    score = verifier.verify(
        "test", "prompt", "[one] [two] [three]",
        ["detectable_content:number_placeholders"],
        [{"num_placeholders": 3}],
    )
    assert score.passed

    score = verifier.verify(
        "test", "prompt", "[one] [two]",
        ["detectable_content:number_placeholders"],
        [{"num_placeholders": 3}],
    )
    assert not score.passed


@pytest.mark.unit
def test_ifeval_verifier_postscript():
    verifier = IFEvalVerifier()
    score = verifier.verify(
        "test", "prompt", "Main text\nP.S. extra note",
        ["detectable_content:postscript"],
        [{"postscript_marker": "P.S."}],
    )
    assert score.passed

    score = verifier.verify(
        "test", "prompt", "Just main text",
        ["detectable_content:postscript"],
        [{"postscript_marker": "P.S."}],
    )
    assert not score.passed


@pytest.mark.unit
def test_ifeval_verifier_number_bullet_lists():
    verifier = IFEvalVerifier()
    score = verifier.verify(
        "test", "prompt", "* item one\n* item two",
        ["detectable_format:number_bullet_lists"],
        [{"num_bullets": 2}],
    )
    assert score.passed

    score = verifier.verify(
        "test", "prompt", "* item one",
        ["detectable_format:number_bullet_lists"],
        [{"num_bullets": 2}],
    )
    assert not score.passed


@pytest.mark.unit
def test_ifeval_verifier_constrained_response():
    verifier = IFEvalVerifier()
    score = verifier.verify(
        "test", "prompt", "My answer is yes.",
        ["detectable_format:constrained_response"],
        [{}],
    )
    assert score.passed

    score = verifier.verify(
        "test", "prompt", "I think so",
        ["detectable_format:constrained_response"],
        [{}],
    )
    assert not score.passed


@pytest.mark.unit
def test_ifeval_verifier_number_highlighted_sections():
    verifier = IFEvalVerifier()
    score = verifier.verify(
        "test", "prompt", "*highlight1* and *highlight2*",
        ["detectable_format:number_highlighted_sections"],
        [{"num_highlights": 2}],
    )
    assert score.passed

    score = verifier.verify(
        "test", "prompt", "*highlight1* only",
        ["detectable_format:number_highlighted_sections"],
        [{"num_highlights": 2}],
    )
    assert not score.passed


@pytest.mark.unit
def test_ifeval_verifier_multiple_sections():
    verifier = IFEvalVerifier()
    score = verifier.verify(
        "test", "prompt", "Section 1 content\nSection 2 content",
        ["detectable_format:multiple_sections"],
        [{"section_spliter": "Section", "num_sections": 2}],
    )
    assert score.passed

    score = verifier.verify(
        "test", "prompt", "Only one section",
        ["detectable_format:multiple_sections"],
        [{"section_spliter": "Section", "num_sections": 2}],
    )
    assert not score.passed


@pytest.mark.unit
def test_ifeval_verifier_title():
    verifier = IFEvalVerifier()
    score = verifier.verify(
        "test", "prompt", "<<My Title>>",
        ["detectable_format:title"],
        [{}],
    )
    assert score.passed

    score = verifier.verify(
        "test", "prompt", "No title here",
        ["detectable_format:title"],
        [{}],
    )
    assert not score.passed


@pytest.mark.unit
def test_ifeval_verifier_two_responses():
    verifier = IFEvalVerifier()
    score = verifier.verify(
        "test", "prompt", "First response\n******\nSecond response",
        ["combination:two_responses"],
        [{}],
    )
    assert score.passed

    score = verifier.verify(
        "test", "prompt", "Only one response",
        ["combination:two_responses"],
        [{}],
    )
    assert not score.passed


@pytest.mark.unit
def test_ifeval_verifier_repeat_prompt():
    verifier = IFEvalVerifier()
    prompt_to_repeat = "Repeat this: hello world"
    score = verifier.verify(
        "test", "prompt", prompt_to_repeat + " and more",
        ["combination:repeat_prompt"],
        [{"prompt_to_repeat": prompt_to_repeat}],
    )
    assert score.passed

    score = verifier.verify(
        "test", "prompt", "Something completely different",
        ["combination:repeat_prompt"],
        [{"prompt_to_repeat": prompt_to_repeat}],
    )
    assert not score.passed


@pytest.mark.unit
def test_ifeval_verifier_end_checker():
    verifier = IFEvalVerifier()
    score = verifier.verify(
        "test", "prompt", 'This ends with "bye"',
        ["startend:end_checker"],
        [{"end_phrase": "bye"}],
    )
    assert score.passed

    score = verifier.verify(
        "test", "prompt", "This does not end with the phrase",
        ["startend:end_checker"],
        [{"end_phrase": "bye"}],
    )
    assert not score.passed


@pytest.mark.unit
def test_ifeval_verifier_quotation():
    verifier = IFEvalVerifier()
    score = verifier.verify(
        "test", "prompt", '"quoted text"',
        ["startend:quotation"],
        [{}],
    )
    assert score.passed

    score = verifier.verify(
        "test", "prompt", "not quoted",
        ["startend:quotation"],
        [{}],
    )
    assert not score.passed

