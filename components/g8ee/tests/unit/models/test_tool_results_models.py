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

from datetime import UTC, datetime

import pytest
from app.models.base import ValidationError, G8eBaseModel

from app.constants import ErrorAnalysisCategory, ExecutionStatus, RiskLevel
from app.models.tool_results import (
    CommandInternalResult,
    CommandRiskAnalysis,
    CommandRiskContext,
    ErrorAnalysisContext,
    ErrorAnalysisResult,
    FileOperationRiskAnalysis,
    FileOperationRiskContext,
    SshInventoryToolResult,
)

pytestmark = [pytest.mark.unit]


class TestCommandRiskContext:

    def test_defaults(self):
        ctx = CommandRiskContext()
        assert ctx.working_directory == ""
        assert ctx.git_status == ""

    def test_explicit_values(self):
        ctx = CommandRiskContext(working_directory="/app", git_status="clean")
        assert ctx.working_directory == "/app"
        assert ctx.git_status == "clean"

    def test_is_pydantic_model(self):
        assert issubclass(CommandRiskContext, G8eBaseModel)


    def test_partial_override_keeps_other_defaults(self):
        ctx = CommandRiskContext(working_directory="/srv")
        assert ctx.working_directory == "/srv"
        assert ctx.git_status == ""


class TestErrorAnalysisContext:

    def test_defaults(self):
        ctx = ErrorAnalysisContext()
        assert ctx.retry_count == 0
        assert ctx.working_directory == ""
        assert ctx.execution_id is None

    def test_retry_count_set(self):
        ctx = ErrorAnalysisContext(retry_count=2)
        assert ctx.retry_count == 2

    def testexecution_id_set(self):
        ctx = ErrorAnalysisContext(execution_id="exec-abc123")
        assert ctx.execution_id == "exec-abc123"

    def test_all_fields_set(self):
        ctx = ErrorAnalysisContext(
            retry_count=1,
            working_directory="/opt/app",
            execution_id="exec-xyz",
        )
        assert ctx.retry_count == 1
        assert ctx.working_directory == "/opt/app"
        assert ctx.execution_id == "exec-xyz"

    def test_is_pydantic_model(self):
        assert issubclass(ErrorAnalysisContext, G8eBaseModel)


    def test_retry_count_zero_does_not_trigger_escalation_flag(self):
        ctx = ErrorAnalysisContext(retry_count=0)
        assert ctx.retry_count < 2

    def test_retry_count_two_triggers_escalation_boundary(self):
        ctx = ErrorAnalysisContext(retry_count=2)
        assert ctx.retry_count >= 2


class TestFileOperationRiskContext:

    def test_defaults(self):
        ctx = FileOperationRiskContext()
        assert ctx.git_status == ""
        assert ctx.backup_available is False

    def test_explicit_values(self):
        ctx = FileOperationRiskContext(git_status="dirty", backup_available=True)
        assert ctx.git_status == "dirty"
        assert ctx.backup_available is True

    def test_is_pydantic_model(self):
        assert issubclass(FileOperationRiskContext, G8eBaseModel)

    def test_backup_available_defaults_false(self):
        ctx = FileOperationRiskContext(git_status="clean")
        assert ctx.backup_available is False

    def test_backup_available_coerces_truthy_string(self):
        ctx = FileOperationRiskContext(backup_available="yes")
        assert ctx.backup_available is True


class TestCommandInternalResultCompletedAt:

    def test_accepts_datetime(self):
        ts = datetime(2026, 3, 4, 12, 0, 0, tzinfo=UTC)
        result = CommandInternalResult(
            execution_id="exec-dt",
            status=ExecutionStatus.COMPLETED,
            completed_at=ts,
        )
        assert result.completed_at == ts
        assert isinstance(result.completed_at, datetime)

    def test_is_none_by_default(self):
        result = CommandInternalResult(
            execution_id="exec-none",
            status=ExecutionStatus.COMPLETED,
        )
        assert result.completed_at is None

    def test_accepts_iso_string_coerced_to_datetime(self):
        result = CommandInternalResult(
            execution_id="exec-iso",
            status=ExecutionStatus.COMPLETED,
            completed_at="2026-03-04T12:00:00Z",
        )
        assert isinstance(result.completed_at, datetime)

    def test_direct_assignment_from_datetime_source(self):
        ts = datetime(2026, 3, 4, 9, 0, 0, tzinfo=UTC)
        result = CommandInternalResult(
            execution_id="exec-direct",
            status=ExecutionStatus.COMPLETED,
            completed_at=ts,
        )
        assert isinstance(result.completed_at, datetime)
        assert result.completed_at.year == 2026


class TestCommandRiskAnalysis:

    def test_low_risk(self):
        analysis = CommandRiskAnalysis(risk_level=RiskLevel.LOW)
        assert analysis.risk_level == RiskLevel.LOW

    def test_medium_risk(self):
        analysis = CommandRiskAnalysis(risk_level=RiskLevel.MEDIUM)
        assert analysis.risk_level == RiskLevel.MEDIUM

    def test_high_risk(self):
        analysis = CommandRiskAnalysis(risk_level=RiskLevel.HIGH)
        assert analysis.risk_level == RiskLevel.HIGH

    def test_risk_level_stored_as_string_due_to_use_enum_values(self):
        analysis = CommandRiskAnalysis(risk_level=RiskLevel.HIGH)
        assert analysis.risk_level == RiskLevel.HIGH


    def test_risk_level_is_required(self):
        with pytest.raises(ValidationError):
            CommandRiskAnalysis()


class TestErrorAnalysisResult:

    def _valid_result(self, **overrides):
        defaults = {
            "error_category": ErrorAnalysisCategory.DEPENDENCY,
            "root_cause": "Missing package",
            "can_auto_fix": True,
            "should_escalate": False,
            "reasoning": "Package not installed",
            "user_message": "Installing missing dependency",
        }
        defaults.update(overrides)
        return ErrorAnalysisResult(**defaults)

    def test_all_fields_set(self):
        result = self._valid_result()
        assert result.error_category == ErrorAnalysisCategory.DEPENDENCY
        assert result.can_auto_fix is True

    def test_optional_fields_default_none(self):
        result = self._valid_result()
        assert result.suggested_fix is None
        assert result.suggested_command is None

    def test_suggested_fix_set(self):
        result = self._valid_result(suggested_fix="Run npm install")
        assert result.suggested_fix == "Run npm install"

    def test_suggested_command_set(self):
        result = self._valid_result(suggested_command="npm install")
        assert result.suggested_command == "npm install"


class TestFileOperationRiskAnalysis:

    def test_minimal_construction(self):
        analysis = FileOperationRiskAnalysis(
            risk_level=RiskLevel.LOW,
            safe_to_proceed=True,
        )
        assert analysis.risk_level == RiskLevel.LOW
        assert analysis.safe_to_proceed is True
        assert analysis.is_system_file is None
        assert analysis.blocking_issues == []
        assert analysis.approval_prompt is None

    def test_blocking_issues_default_empty_list(self):
        analysis = FileOperationRiskAnalysis(
            risk_level=RiskLevel.HIGH,
            safe_to_proceed=False,
        )
        assert isinstance(analysis.blocking_issues, list)
        assert len(analysis.blocking_issues) == 0

    def test_blocking_issues_populated(self):
        analysis = FileOperationRiskAnalysis(
            risk_level=RiskLevel.HIGH,
            safe_to_proceed=False,
            blocking_issues=["System file", "No backup"],
        )
        assert len(analysis.blocking_issues) == 2
        assert "System file" in analysis.blocking_issues

    def test_is_system_file_set(self):
        analysis = FileOperationRiskAnalysis(
            risk_level=RiskLevel.HIGH,
            safe_to_proceed=False,
            is_system_file=True,
        )
        assert analysis.is_system_file is True

    def test_safe_to_proceed_defaults_true(self):
        analysis = FileOperationRiskAnalysis(risk_level=RiskLevel.LOW)
        assert analysis.safe_to_proceed is True

    def test_risk_level_stored_as_string_due_to_use_enum_values(self):
        analysis = FileOperationRiskAnalysis(risk_level=RiskLevel.MEDIUM, safe_to_proceed=True)
        assert analysis.risk_level == RiskLevel.MEDIUM


class TestSshInventoryToolResult:

    def test_defaults(self):
        result = SshInventoryToolResult()
        assert result.success is True
        assert result.error is None
        assert result.error_type is None
        assert result.source_path is None
        assert result.hosts == []
        assert result.total_count == 0

    def test_explicit_values(self):
        result = SshInventoryToolResult(
            success=True,
            source_path="/etc/ssh/config",
            hosts=[{"host": "web-1", "hostname": "10.0.0.1"}],
            total_count=1,
        )
        assert result.success is True
        assert result.source_path == "/etc/ssh/config"
        assert len(result.hosts) == 1
        assert result.hosts[0]["host"] == "web-1"
        assert result.total_count == 1

    def test_is_pydantic_model(self):
        assert issubclass(SshInventoryToolResult, G8eBaseModel)

    def test_error_case(self):
        result = SshInventoryToolResult(
            success=False,
            error="SSH config not found",
            source_path=None,
            hosts=[],
            total_count=0,
        )
        assert result.success is False
        assert result.error == "SSH config not found"
        assert result.total_count == 0
