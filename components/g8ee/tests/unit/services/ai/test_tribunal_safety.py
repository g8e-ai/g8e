# Copyright (c) 2026 Lateralus Labs, LLC.
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

import pytest
from unittest.mock import AsyncMock, MagicMock, patch

from app.services.ai.command_generator import (
    _normalise_command,
    _validate_command_safety,
    _run_verifier,
    generate_command,
    TribunalEmitter,
)
from app.models.agent import OperatorContext
from app.models.agents.tribunal import TribunalGenerationFailedError, TribunalVerifierFailedError
from app.constants import VerifierReason, ComponentName
from app.utils.agent_persona_loader import get_agent_persona
from app.models.http_context import G8eHttpContext

def _make_mock_operator_context(os="linux"):
    return OperatorContext(
        operator_id="test-operator",
        os=os,
        shell="bash",
        username="testuser",
        uid=1000,
        working_directory="/home/testuser",
        hostname="testhost",
        architecture="x86_64",
    )


def _make_mock_g8e_context() -> G8eHttpContext:
    """Create a mock G8eHttpContext for tests."""
    return G8eHttpContext(
        web_session_id="test-session-id",
        user_id="test-user-id",
        case_id="test-case-id",
        investigation_id="test-investigation-id",
        source_component=ComponentName.G8EE,
    )

class TestNormaliseCommand:
    def test_markdown_fences(self):
        assert _normalise_command("```bash\nls -la\n```") == "ls -la"
        assert _normalise_command("```sh\nls -la\n```") == "ls -la"
        assert _normalise_command("```\nls -la\n```") == "ls -la"

    def test_prefixes(self):
        assert _normalise_command("Command: ls -la") == "ls -la"
        assert _normalise_command("The command is: ls -la") == "ls -la"
        assert _normalise_command("Final command: ls -la") == "ls -la"

    def test_multi_line(self):
        cmd = "cat <<EOF\nhello\nEOF"
        assert _normalise_command(f"```bash\n{cmd}\n```") == cmd

    def test_shell_syntax_validation(self):
        # Valid syntax
        assert _normalise_command("ls -la 'file with spaces'") == "ls -la 'file with spaces'"
        # Invalid syntax (unbalanced quote) - should fallback to first line or return empty
        assert _normalise_command("ls -la 'unbalanced") == ""
        # Valid first line, invalid second line explanatory text
        assert _normalise_command("ls -la\nThis is an explanation with an 'unbalanced quote") == "ls -la"

class TestValidateCommandSafety:
    def test_forbidden_patterns(self):
        is_safe, error = _validate_command_safety("sudo ls", False, False, None)
        assert not is_safe
        assert "forbidden pattern" in error.lower()

    @patch("app.services.ai.command_generator.validate_command_against_blacklist")
    def test_blacklist_enforcement(self, mock_blacklist):
        from app.utils.blacklist_validator import CommandBlacklistResult
        mock_blacklist.return_value = CommandBlacklistResult(is_allowed=False, reason="Blocked")
        
        is_safe, error = _validate_command_safety("ls /etc/shadow", False, True, None)
        assert not is_safe
        assert "blocked by blacklist" in error.lower()

    @patch("app.services.ai.command_generator.validate_command_against_whitelist")
    def test_whitelist_enforcement(self, mock_whitelist):
        from app.models.whitelist import CommandValidationResult
        mock_whitelist.return_value = CommandValidationResult(is_valid=False, command="unknown_cmd", reason="Not whitelisted")
        
        ctx = _make_mock_operator_context()
        is_safe, error = _validate_command_safety("unknown_cmd", True, False, ctx)
        assert not is_safe
        assert "not whitelisted" in error.lower()

class TestVerifierSafety:
    @pytest.mark.asyncio
    async def test_rejects_unsafe_revision(self):
        mock_response = MagicMock()
        mock_response.text = '{"status": "revised", "revised_command": "sudo rm -rf /"}'

        mock_provider = MagicMock()
        mock_provider.generate_content_lite = AsyncMock(return_value=mock_response)
        emitter = TribunalEmitter(None, _make_mock_g8e_context())
        
        with patch("app.services.ai.command_generator.get_model_config") as mock_config:
            mock_config.return_value.supports_structured_output = False
            
            from app.models.agents.tribunal import VoteBreakdown
            vote_breakdown = VoteBreakdown(
                candidates_by_member={},
                candidates_by_command={"ls": ["axiom"]},
                winner="ls",
                winner_supporters=["axiom"],
                dissenters_by_command={},
                consensus_strength=1.0,
            )

            with pytest.raises(TribunalVerifierFailedError) as exc_info:
                await _run_verifier(
                    provider=mock_provider,
                    model="test-model",
                    request="delete everything",
                    guidelines="",
                    mode="unanimous",
                    vote_winner="ls",
                    vote_breakdown=vote_breakdown,
                    tied_candidates=None,
                    operator_context=_make_mock_operator_context(),
                    emitter=emitter,
                    command_constraints_message="",
                    verifier_persona=get_agent_persona("auditor"),
                )
            
            assert exc_info.value.reason == VerifierReason.NO_VALID_REVISION
            assert "revision unsafe" in exc_info.value.error.lower()

class TestGenerateCommandSafety:
    @pytest.mark.asyncio
    async def test_rejects_unsafe_final_command(self):
        # Mock successful generation of an unsafe command
        mock_response = MagicMock()
        mock_response.text = "sudo ls"
        
        mock_provider = MagicMock()
        mock_provider.generate_content_lite = AsyncMock(return_value=mock_response)
        
        mock_settings = MagicMock()
        mock_settings.llm.llm_command_gen_enabled = True
        mock_settings.llm.llm_command_gen_verifier = False
        mock_settings.llm.llm_command_gen_passes = 1
        
        with patch("app.services.ai.command_generator.get_llm_provider", return_value=mock_provider), \
             patch("app.services.ai.command_generator._resolve_model", return_value="test-model"), \
             patch("app.services.ai.command_generator.get_model_config") as mock_config:
            
            mock_config.return_value.supports_structured_output = False
            
            with pytest.raises(TribunalGenerationFailedError) as exc_info:
                await generate_command(
                    request="run as root",
                    guidelines="",
                    operator_context=_make_mock_operator_context(),
                    g8ed_event_service=AsyncMock(),
                    web_session_id="ws-1",
                    user_id="user-1",
                    case_id="case-1",
                    investigation_id="inv-1",
                    settings=mock_settings,
                )
            
            assert "safety validation failed" in exc_info.value.pass_errors[0].lower()

class TestStructuredOutputSupport:
    @pytest.mark.asyncio
    async def test_handles_structured_json_response(self):
        import json
        mock_response = MagicMock()
        mock_response.text = json.dumps({"command": "ls -la"})
        
        mock_provider = MagicMock()
        mock_provider.generate_content_lite = AsyncMock(return_value=mock_response)
        emitter = TribunalEmitter(None, _make_mock_g8e_context())
        
        from app.services.ai.command_generator import _run_generation_pass
        
        with patch("app.services.ai.command_generator.get_model_config") as mock_config:
            mock_config.return_value.supports_structured_output = True
            
            result = await _run_generation_pass(
                provider=mock_provider,
                model="test-model",
                request="list files",
                guidelines="",
                operator_context=_make_mock_operator_context(),
                pass_index=0,
                emitter=emitter,
                pass_errors=[],
                command_constraints_message="",
            )
            
            assert result == "ls -la"
