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

from app.services.ai.auditor_service import run_auditor
from app.services.ai.generator import TribunalEmitter
from app.services.ai.generator import generate_command
from app.utils.command import normalise_command
from app.utils.safety import validate_command_safety
from app.models.agent import OperatorContext
from app.models.agents.tribunal import TribunalGenerationFailedError, TribunalAuditorFailedError
from app.constants import AuditorReason, ComponentName
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
        assert normalise_command("```bash\nls -la\n```") == "ls -la"
        assert normalise_command("```sh\nls -la\n```") == "ls -la"
        assert normalise_command("```\nls -la\n```") == "ls -la"

    def test_prefixes(self):
        assert normalise_command("Command: ls -la") == "ls -la"
        assert normalise_command("The command is: ls -la") == "ls -la"
        assert normalise_command("Final command: ls -la") == "ls -la"

    def test_multi_line(self):
        cmd = "cat <<EOF\nhello\nEOF"
        # Heredocs are preserved as multi-line after space collapse
        result = normalise_command(f"```bash\n{cmd}\n```")
        assert "cat <<EOF" in result
        assert "hello" in result
        assert "EOF" in result

    def test_shell_syntax_validation(self):
        # Valid syntax
        assert normalise_command("ls -la 'file with spaces'") == "ls -la 'file with spaces'"
        # Invalid syntax (unbalanced quote) - should return empty
        assert normalise_command("ls -la 'unbalanced") == ""
        # Multi-line with invalid second line - returns first valid line
        assert normalise_command("ls -la\nThis is an explanation with an 'unbalanced quote") == "ls -la"

class TestValidateCommandSafety:
    def test_forbidden_patterns(self):
        is_safe, error = validate_command_safety("sudo ls", False, False, None)
        assert not is_safe
        assert "forbidden pattern" in error.lower()

    @patch("app.utils.safety.validate_command_against_blacklist")
    def test_blacklist_enforcement(self, mock_blacklist):
        from app.utils.blacklist_validator import CommandBlacklistResult
        mock_blacklist.return_value = CommandBlacklistResult(is_allowed=False, reason="Blocked")
        
        is_safe, error = validate_command_safety("ls /etc/shadow", False, True, None)
        assert not is_safe
        assert "blocked by blacklist" in error.lower()

    @patch("app.utils.safety.validate_command_against_whitelist")
    def test_whitelist_enforcement(self, mock_whitelist):
        from app.models.whitelist import CommandValidationResult
        mock_whitelist.return_value = CommandValidationResult(is_valid=False, command="unknown_cmd", reason="Not whitelisted")
        
        ctx = _make_mock_operator_context()
        is_safe, error = validate_command_safety("unknown_cmd", True, False, ctx)
        assert not is_safe
        assert "not whitelisted" in error.lower()

    @patch("app.utils.safety.validate_command_against_whitelist")
    def test_whitelist_override_forwarded_to_validator(self, mock_whitelist):
        """The CSV-derived override list must be threaded into the whitelist validator."""
        from app.models.whitelist import CommandValidationResult
        mock_whitelist.return_value = CommandValidationResult(is_valid=True, command="uptime")

        ctx = _make_mock_operator_context()
        is_safe, error = validate_command_safety(
            "uptime", True, False, ctx,
            whitelisted_commands_override=["uptime", "df"],
        )
        assert is_safe
        assert error is None
        # Validator must have been called with the override kwarg
        _, kwargs = mock_whitelist.call_args
        assert kwargs.get("allowed_commands_override") == ["uptime", "df"]

class TestAuditorSafety:
    @pytest.mark.asyncio
    async def test_rejects_unsafe_revision(self):
        mock_response = MagicMock()
        mock_response.text = '{"status": "revised", "revised_command": "sudo rm -rf /"}'

        mock_provider = MagicMock()
        mock_provider.generate_content_lite = AsyncMock(return_value=mock_response)
        emitter = TribunalEmitter(None, _make_mock_g8e_context())
        
        with patch("app.services.ai.auditor_service.get_model_config") as mock_config:
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

            with pytest.raises(TribunalAuditorFailedError) as exc_info:
                await run_auditor(
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
                    auditor_persona=get_agent_persona("auditor"),
                )
            
            assert exc_info.value.reason == AuditorReason.NO_VALID_REVISION
            assert "technical safety failure" in exc_info.value.error.lower()

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
        mock_settings.llm.llm_command_gen_auditor = False
        mock_settings.llm.llm_command_gen_passes = 1
        
        with patch("app.services.ai.generator.get_llm_provider", return_value=mock_provider), \
             patch("app.services.ai.generator._resolve_model", return_value="test-model"), \
             patch("app.services.ai.generator.get_model_config") as mock_config:
            
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
                    reputation_data_service=MagicMock(),
                    auditor_hmac_key="test-key",
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
        
        from app.services.ai.generator import _run_generation_pass
        
        with patch("app.services.ai.generator.get_model_config") as mock_config:
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
