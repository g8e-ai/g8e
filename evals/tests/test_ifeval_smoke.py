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

def test_llm_client_rejects_missing_openai_key():
    """Regression test: silent stub pattern must be detected.
    
    A _call_llm that returns a plausible-looking string and silently produces
    a "20% pass rate" is exactly the failure mode evals are supposed to detect.
    This test ensures the bench refuses to run if required API keys are unset.
    """
    from g8e_evals.llm_client import _build_settings, _parse_model
    
    # Save original env var if present
    original_key = os.environ.get("OPENAI_API_KEY")
    
    try:
        # Ensure OPENAI_API_KEY is unset
        if "OPENAI_API_KEY" in os.environ:
            del os.environ["OPENAI_API_KEY"]
        
        provider, model_name = _parse_model("openai:gpt-4")
        
        # Should raise RuntimeError when API key is missing
        with pytest.raises(RuntimeError) as exc_info:
            _build_settings(provider, model_name)
        
        assert "OPENAI_API_KEY" in str(exc_info.value)
        assert "to be set in the environment" in str(exc_info.value)
    finally:
        # Restore original env var
        if original_key is not None:
            os.environ["OPENAI_API_KEY"] = original_key

def test_llm_client_rejects_missing_anthropic_key():
    """Regression test: Anthropic requires API key."""
    from g8e_evals.llm_client import _build_settings, _parse_model
    
    original_key = os.environ.get("ANTHROPIC_API_KEY")
    
    try:
        if "ANTHROPIC_API_KEY" in os.environ:
            del os.environ["ANTHROPIC_API_KEY"]
        
        provider, model_name = _parse_model("anthropic:claude-3-5-sonnet-latest")
        
        with pytest.raises(RuntimeError) as exc_info:
            _build_settings(provider, model_name)
        
        assert "ANTHROPIC_API_KEY" in str(exc_info.value)
    finally:
        if original_key is not None:
            os.environ["ANTHROPIC_API_KEY"] = original_key

def test_llm_client_rejects_missing_gemini_key():
    """Regression test: Gemini requires API key (GEMINI_API_KEY or GOOGLE_API_KEY)."""
    from g8e_evals.llm_client import _build_settings, _parse_model
    
    original_gemini_key = os.environ.get("GEMINI_API_KEY")
    original_google_key = os.environ.get("GOOGLE_API_KEY")
    
    try:
        if "GEMINI_API_KEY" in os.environ:
            del os.environ["GEMINI_API_KEY"]
        if "GOOGLE_API_KEY" in os.environ:
            del os.environ["GOOGLE_API_KEY"]
        
        provider, model_name = _parse_model("gemini:gemini-2.0-flash")
        
        with pytest.raises(RuntimeError) as exc_info:
            _build_settings(provider, model_name)
        
        assert "GEMINI_API_KEY" in str(exc_info.value)
        assert "GOOGLE_API_KEY" in str(exc_info.value)
    finally:
        if original_gemini_key is not None:
            os.environ["GEMINI_API_KEY"] = original_gemini_key
        if original_google_key is not None:
            os.environ["GOOGLE_API_KEY"] = original_google_key

def test_llm_client_model_parsing_validates_format():
    """Regression test: ensure provider:model format is enforced."""
    from g8e_evals.llm_client import _parse_model
    
    # Missing colon
    with pytest.raises(ValueError) as exc_info:
        _parse_model("openai-gpt-4")
    assert "must be in 'provider:name' form" in str(exc_info.value)
    
    # Empty provider
    with pytest.raises(ValueError) as exc_info:
        _parse_model(":gpt-4")
    assert "missing provider or name" in str(exc_info.value)
    
    # Empty model name
    with pytest.raises(ValueError) as exc_info:
        _parse_model("openai:")
    assert "missing provider or name" in str(exc_info.value)
    
    # Unsupported provider
    with pytest.raises(ValueError) as exc_info:
        _parse_model("unknown:model")
    assert "Unsupported provider" in str(exc_info.value)
