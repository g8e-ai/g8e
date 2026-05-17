import pytest
import os
from unittest.mock import AsyncMock, patch
from g8e_evals.sut.answer_only import AnswerOnlySUT
from g8e_evals.harness import SUTConfig, LLMRoleConfig, Task

@pytest.mark.asyncio
async def test_answer_only_sut_calls_real_llm():
    """Verify that AnswerOnlySUT calls g8e_evals.llm_client.call_llm and NOT a stub."""
    config = SUTConfig(
        primary=LLMRoleConfig(provider="openai", model="gpt-4o"),
        mode="baseline"
    )
    sut = AnswerOnlySUT(config)
    task = Task(id="test-1", prompt="What is 2+2?", metadata={"instruction_id_list": [], "kwargs": {}})
    
    # Use a sentinel to verify call_llm was hit
    with patch("g8e_evals.llm_client.call_llm", new_callable=AsyncMock) as mock_call:
        mock_call.return_value = "sentinel-response"
        response = await sut.get_answer(task)
        
        assert response.answer == "sentinel-response"
        mock_call.assert_called_once()
        # Verify provider:model was passed correctly
        _, kwargs = mock_call.call_args
        assert kwargs["model_provider"] == "openai:gpt-4o"
        assert kwargs["prompt"] == "What is 2+2?"

@pytest.mark.asyncio
async def test_llm_client_fails_closed_without_creds():
    """Verify that llm_client.call_llm raises RuntimeError when credentials are missing."""
    from g8e_evals.llm_client import call_llm
    
    config = SUTConfig(
        primary=LLMRoleConfig(provider="openai", model="gpt-4o"),
        mode="baseline"
    )
    
    # Ensure OPENAI_API_KEY is NOT set, but preserve protocol dir
    protocol_dir = os.environ.get("G8E_PROTOCOL_DIR")
    with patch.dict("os.environ", {"G8E_PROTOCOL_DIR": protocol_dir} if protocol_dir else {}, clear=True):
        with pytest.raises(RuntimeError) as excinfo:
            await call_llm(model_provider="openai:gpt-4o", prompt="hi", config=config)
        
        assert "openai requires OPENAI_API_KEY" in str(excinfo.value)

def test_provider_env_coverage():
    """Drift detector for LLM provider coverage."""
    # Ensure protocol dir is set before importing app.constants
    if "G8E_PROTOCOL_DIR" not in os.environ:
        import sys
        from pathlib import Path
        G8E_ROOT = Path(__file__).parent.parent.parent
        os.environ["G8E_PROTOCOL_DIR"] = str(G8E_ROOT / "protocol")

    from g8e_evals.llm_client import _PROVIDER_ENV
    from app.constants import LLMProvider
    
    # Get all provider values from the g8ee app enum
    enum_providers = {p.value for p in LLMProvider}
    
    # These are handled specifically or are aliases in g8ee
    ignored = {"mock", "bedrock", "vertex"} # If they aren't in _PROVIDER_ENV yet
    
    for p in enum_providers:
        if p in ignored:
            continue
        assert p in _PROVIDER_ENV, f"Provider {p} from app.constants.LLMProvider is missing from _PROVIDER_ENV"
