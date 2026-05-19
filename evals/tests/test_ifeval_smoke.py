import os
import pytest
from pathlib import Path
from g8e_evals.benchmarks.ifeval.loader import IFEvalLoader
from g8e_evals.benchmarks.ifeval.verifier import IFEvalVerifier

def test_ifeval_loader():
    base_dir = Path(__file__).parent.parent
    gold_set = base_dir / "gold_sets/ifeval/input_data.jsonl"
    loader = IFEvalLoader(gold_set)
    tasks = list(loader.load())
    assert len(tasks) == 5
    assert tasks[0].id == "1001"
    assert "no punctuation" in tasks[0].prompt

def test_ifeval_verifier_punctuation():
    verifier = IFEvalVerifier()
    # Task 1001: no punctuation
    score = verifier.verify("1001", "prompt", "This is fine", ["punctuation:no_punctuation"], [{}])
    assert score.passed
    
    score = verifier.verify("1001", "prompt", "This is NOT fine.", ["punctuation:no_punctuation"], [{}])
    assert not score.passed

def test_ifeval_verifier_uppercase():
    verifier = IFEvalVerifier()
    # Task 1002: uppercase
    score = verifier.verify("1002", "prompt", "ALL UPPERCASE", ["case:uppercase"], [{}])
    assert score.passed
    
    score = verifier.verify("1002", "prompt", "Not all uppercase", ["case:uppercase"], [{}])
    assert not score.passed

def test_ifeval_verifier_json():
    verifier = IFEvalVerifier()
    # Task 1003: JSON
    score = verifier.verify("1003", "prompt", '{"name": "test"}', ["format:json"], [{}])
    assert score.passed
    
    score = verifier.verify("1003", "prompt", 'not json', ["format:json"], [{}])
    assert not score.passed

def test_ifeval_verifier_min_words():
    verifier = IFEvalVerifier()
    # Task 1004: min 10 words
    answer = "one two three four five six seven eight nine ten"
    score = verifier.verify("1004", "prompt", answer, ["length:min_words"], [{"num_words": 10}])
    assert score.passed
    
    answer = "too short"
    score = verifier.verify("1004", "prompt", answer, ["length:min_words"], [{"num_words": 10}])
    assert not score.passed

def test_ifeval_verifier_forbidden_words():
    verifier = IFEvalVerifier()
    # Task 1005: forbidden word 'apple'
    score = verifier.verify("1005", "prompt", "I like oranges", ["keywords:forbidden_words"], [{"forbidden_words": ["apple"]}])
    assert score.passed
    
    score = verifier.verify("1005", "prompt", "I like apple pie", ["keywords:forbidden_words"], [{"forbidden_words": ["apple"]}])
    assert not score.passed

