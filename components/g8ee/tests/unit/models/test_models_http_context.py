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

"""
Unit tests for app/models/http_context.py, app/models/health.py, app/models/auth.py

Covers: DependencyStatus, HealthCheckResult, WorkflowHealthResult,
        ServiceHealthResult, AuthenticatedUser, BoundOperator, G8eHttpContext
"""

import json
from datetime import UTC, datetime

import pytest
from app.models.base import ValidationError

from app.constants import (
    AuthMethod,
    ComponentName,
    HealthStatus,
    OperatorStatus,
)
from app.models.auth import AuthenticatedUser
from app.models.health import (
    DependencyStatus,
    HealthCheckResult,
    ServiceHealthResult,
    WorkflowHealthResult,
)
from app.models.http_context import (
    BoundOperator,
    G8eHttpContext,
)

pytestmark = [pytest.mark.unit]

_TS = datetime(2026, 1, 15, 10, 30, 0, tzinfo=UTC)


class TestDependencyStatus:

    def test_instantiation_healthy(self):
        dep = DependencyStatus(status=HealthStatus.HEALTHY)
        assert dep.status == HealthStatus.HEALTHY
        assert dep.error is None

    def test_instantiation_unhealthy_with_error(self):
        dep = DependencyStatus(status=HealthStatus.UNHEALTHY, error="Connection refused")
        assert dep.status == HealthStatus.UNHEALTHY
        assert dep.error == "Connection refused"

    def test_status_is_enum(self):
        dep = DependencyStatus(status=HealthStatus.HEALTHY)
        assert dep.status == HealthStatus.HEALTHY


    def test_status_required(self):
        with pytest.raises(ValidationError):
            DependencyStatus()

    def test_all_health_statuses_accepted(self):
        for status in HealthStatus:
            dep = DependencyStatus(status=status)
            assert dep.status == status

    def test_model_dump_serializes_enum(self):
        dep = DependencyStatus(status=HealthStatus.HEALTHY)
        dumped = dep.model_dump()
        assert dumped["status"] == "healthy"

    def test_model_dump_excludes_none_error_by_default(self):
        dep = DependencyStatus(status=HealthStatus.HEALTHY)
        dumped = dep.model_dump()
        assert "error" not in dumped

    def test_extra_fields_ignored(self):
        dep = DependencyStatus(status=HealthStatus.HEALTHY, injected="bad")
        assert not hasattr(dep, "injected")


class TestHealthCheckResult:

    def _make(self, **overrides):
        defaults = dict(
            timestamp=_TS,
            component="g8ee",
            dependencies={"g8es": DependencyStatus(status=HealthStatus.HEALTHY)},
            overall_status=HealthStatus.HEALTHY,
        )
        defaults.update(overrides)
        return HealthCheckResult(**defaults)

    def test_instantiation_with_required_fields(self):
        result = self._make()
        assert result.component == "g8ee"
        assert result.overall_status == HealthStatus.HEALTHY
        assert isinstance(result.timestamp, datetime)

    def test_overall_status_is_enum(self):
        result = self._make(overall_status=HealthStatus.UNHEALTHY)
        assert result.overall_status == HealthStatus.UNHEALTHY


    def test_dependencies_map_contains_dependency_status(self):
        result = self._make()
        assert "g8es" in result.dependencies
        assert isinstance(result.dependencies["g8es"], DependencyStatus)

    def test_unhealthy_dependencies_defaults_to_none(self):
        result = self._make()
        assert result.unhealthy_dependencies is None

    def test_unhealthy_dependencies_can_be_set(self):
        result = self._make(unhealthy_dependencies=["g8es", "redis"])
        assert result.unhealthy_dependencies == ["g8es", "redis"]

    def test_timestamp_is_datetime(self):
        result = self._make()
        assert isinstance(result.timestamp, datetime)

    def test_model_dump_excludes_none_by_default(self):
        result = self._make()
        dumped = result.model_dump()
        assert "unhealthy_dependencies" not in dumped

    def test_extra_fields_ignored(self):
        result = self._make(unexpected="value")
        assert not hasattr(result, "unexpected")


class TestWorkflowHealthResult:

    def _make(self, **overrides):
        defaults = dict(
            status=HealthStatus.HEALTHY,
            workflows={"chat": DependencyStatus(status=HealthStatus.HEALTHY)},
        )
        defaults.update(overrides)
        return WorkflowHealthResult(**defaults)

    def test_instantiation(self):
        result = self._make()
        assert result.status == HealthStatus.HEALTHY
        assert "chat" in result.workflows

    def test_status_is_enum(self):
        result = self._make(status=HealthStatus.UNHEALTHY)
        assert result.status == HealthStatus.UNHEALTHY


    def test_all_health_statuses_accepted(self):
        for status in HealthStatus:
            result = self._make(status=status)
            assert result.status == status

    def test_workflows_values_are_dependency_status(self):
        result = self._make(workflows={
            "chat": DependencyStatus(status=HealthStatus.HEALTHY),
            "command": DependencyStatus(status=HealthStatus.UNHEALTHY, error="timeout"),
        })
        assert result.workflows["command"].error == "timeout"

    def test_model_dump_serializes_enum(self):
        result = self._make()
        dumped = result.model_dump()
        assert dumped["status"] == "healthy"

    def test_extra_fields_ignored(self):
        result = self._make(injected="value")
        assert not hasattr(result, "injected")


class TestServiceHealthResult:

    def _make(self, **overrides):
        defaults = dict(
            service=HealthStatus.HEALTHY,
            timestamp=_TS,
            checks={"g8es": DependencyStatus(status=HealthStatus.HEALTHY)},
        )
        defaults.update(overrides)
        return ServiceHealthResult(**defaults)

    def test_instantiation_with_required_fields(self):
        result = self._make()
        assert result.service == HealthStatus.HEALTHY
        assert isinstance(result.timestamp, datetime)
        assert "g8es" in result.checks

    def test_service_is_enum(self):
        result = self._make(service=HealthStatus.UNHEALTHY)
        assert result.service == HealthStatus.UNHEALTHY


    def test_error_defaults_to_none(self):
        result = self._make()
        assert result.error is None

    def test_error_can_be_set(self):
        result = self._make(error="Database unreachable")
        assert result.error == "Database unreachable"

    def test_timestamp_is_datetime(self):
        result = self._make()
        assert isinstance(result.timestamp, datetime)

    def test_model_dump_excludes_none_error(self):
        result = self._make()
        dumped = result.model_dump()
        assert "error" not in dumped

    def test_model_dump_serializes_enum(self):
        result = self._make(service=HealthStatus.UNHEALTHY)
        dumped = result.model_dump()
        assert dumped["service"] == "unhealthy"

    def test_extra_fields_ignored(self):
        result = self._make(injected="value")
        assert not hasattr(result, "injected")


class TestAuthenticatedUser:

    def _make(self, **overrides):
        defaults = dict(
            uid="user-abc-123",
            user_id="user-abc-123",
            auth_method=AuthMethod.PROXY,
        )
        defaults.update(overrides)
        return AuthenticatedUser(**defaults)

    def test_instantiation_with_required_fields(self):
        user = self._make()
        assert user.uid == "user-abc-123"
        assert user.user_id == "user-abc-123"
        assert user.auth_method == AuthMethod.PROXY

    def test_uid_required(self):
        with pytest.raises(ValidationError):
            AuthenticatedUser(user_id="u", auth_method=AuthMethod.PROXY)

    def test_user_id_required(self):
        with pytest.raises(ValidationError):
            AuthenticatedUser(uid="u", auth_method=AuthMethod.PROXY)

    def test_auth_method_required(self):
        with pytest.raises(ValidationError):
            AuthenticatedUser(uid="u", user_id="u")

    def test_auth_method_is_enum(self):
        user = self._make(auth_method=AuthMethod.INTERNAL)
        assert user.auth_method == AuthMethod.INTERNAL


    def test_all_auth_methods_accepted(self):
        for method in AuthMethod:
            user = self._make(auth_method=method)
            assert user.auth_method == method

    def test_optional_fields_default_to_none(self):
        user = self._make()
        assert user.email is None
        assert user.name is None
        assert user.organization_id is None
        assert user.web_session_id is None

    def test_optional_fields_can_be_set(self):
        user = self._make(
            email="ops@example.com",
            name="Ops User",
            organization_id="org-999",
            web_session_id="sess-xyz",
        )
        assert user.email == "ops@example.com"
        assert user.name == "Ops User"
        assert user.organization_id == "org-999"
        assert user.web_session_id == "sess-xyz"

    def test_model_dump_serializes_auth_method_as_string(self):
        user = self._make()
        dumped = user.model_dump()
        assert dumped["auth_method"] == "proxy"

    def test_model_dump_excludes_none_by_default(self):
        user = self._make()
        dumped = user.model_dump()
        assert "email" not in dumped
        assert "name" not in dumped
        assert "organization_id" not in dumped
        assert "web_session_id" not in dumped

    def test_extra_fields_ignored(self):
        user = self._make(injected_token="evil")
        assert not hasattr(user, "injected_token")


class TestBoundOperator:

    def test_instantiation_with_required_fields(self):
        op = BoundOperator(operator_id="op-123")
        assert op.operator_id == "op-123"

    def test_operator_id_required(self):
        with pytest.raises(ValidationError):
            BoundOperator()

    def test_optional_fields_default_to_none(self):
        op = BoundOperator(operator_id="op-123")
        assert op.operator_session_id is None
        assert op.status is None

    def test_status_is_enum(self):
        op = BoundOperator(operator_id="op-123", status=OperatorStatus.BOUND)
        assert op.status == OperatorStatus.BOUND


    def test_all_operator_statuses_accepted(self):
        for status in OperatorStatus:
            op = BoundOperator(operator_id="op-123", status=status)
            assert op.status == status


    def test_full_instantiation(self):
        op = BoundOperator(
            operator_id="op-abc",
            operator_session_id="sess-xyz",
            status=OperatorStatus.BOUND,
        )
        assert op.operator_id == "op-abc"
        assert op.operator_session_id == "sess-xyz"
        assert op.status == OperatorStatus.BOUND

    def test_model_dump_serializes_enum_values(self):
        op = BoundOperator(
            operator_id="op-123",
            status=OperatorStatus.BOUND,
        )
        dumped = op.model_dump()
        assert dumped["status"] == "bound"

    def test_model_dump_excludes_none_by_default(self):
        op = BoundOperator(operator_id="op-123")
        dumped = op.model_dump()
        assert "status" not in dumped
        assert "operator_session_id" not in dumped

    def test_extra_fields_ignored(self):
        op = BoundOperator(operator_id="op-123", injected="value")
        assert not hasattr(op, "injected")


class TestG8eHttpContext:

    def _make(self, **overrides):
        defaults = dict(
            web_session_id="session_abc_123",
            user_id="user-uuid-456",
            case_id="case-test-001",
            investigation_id="inv-test-001",
            source_component=ComponentName.G8ED,
        )
        defaults.update(overrides)
        return G8eHttpContext(**defaults)

    def test_instantiation_with_required_fields(self):
        ctx = self._make()
        assert ctx.web_session_id == "session_abc_123"
        assert ctx.user_id == "user-uuid-456"
        assert ctx.source_component == ComponentName.G8ED

    def test_web_session_id_required(self):
        with pytest.raises(ValidationError):
            G8eHttpContext(user_id="u", case_id="c", investigation_id="i", source_component=ComponentName.G8ED)

    def test_user_id_required(self):
        with pytest.raises(ValidationError):
            G8eHttpContext(web_session_id="s", case_id="c", investigation_id="i", source_component=ComponentName.G8ED)

    def test_case_id_required(self):
        with pytest.raises(ValidationError):
            G8eHttpContext(web_session_id="s", user_id="u", investigation_id="i", source_component=ComponentName.G8ED)

    def test_investigation_id_required(self):
        with pytest.raises(ValidationError):
            G8eHttpContext(web_session_id="s", user_id="u", case_id="c", source_component=ComponentName.G8ED)

    def test_source_component_required(self):
        with pytest.raises(ValidationError):
            G8eHttpContext(web_session_id="s", user_id="u", case_id="c", investigation_id="i")

    def test_source_component_is_enum(self):
        ctx = self._make(source_component=ComponentName.G8EE)
        assert ctx.source_component == ComponentName.G8EE


    def test_all_component_names_accepted(self):
        for component in ComponentName:
            ctx = self._make(source_component=component)
            assert ctx.source_component == component

    def test_source_component_accepts_string_value_via_coercion(self):
        ctx = self._make(source_component=ComponentName.G8ED)
        assert ctx.source_component == ComponentName.G8ED

    def test_optional_fields_default_to_none(self):
        ctx = self._make()
        assert ctx.organization_id is None
        assert ctx.task_id is None

    def test_required_correlation_ids_are_set(self):
        ctx = self._make()
        assert ctx.case_id == "case-test-001"
        assert ctx.investigation_id == "inv-test-001"

    def test_correlation_ids_can_be_set(self):
        ctx = self._make(
            organization_id="org-111",
            case_id="case-222",
            investigation_id="inv-333",
            task_id="task-444",
        )
        assert ctx.organization_id == "org-111"
        assert ctx.case_id == "case-222"
        assert ctx.investigation_id == "inv-333"
        assert ctx.task_id == "task-444"

    def test_bound_operators_defaults_to_empty_list(self):
        ctx = self._make()
        assert ctx.bound_operators == []

    def test_bound_operators_accepts_list_of_bound_operator(self):
        ops = [
            BoundOperator(operator_id="op-1", status=OperatorStatus.BOUND),
            BoundOperator(operator_id="op-2", status=OperatorStatus.AVAILABLE),
        ]
        ctx = self._make(bound_operators=ops)
        assert len(ctx.bound_operators) == 2
        assert ctx.bound_operators[0].operator_id == "op-1"

    def test_bound_operators_accepts_list_of_dicts(self):
        ctx = self._make(bound_operators=[
            {"operator_id": "op-1", "status": "bound"},
            {"operator_id": "op-2", "status": "available"},
        ])
        assert len(ctx.bound_operators) == 2
        assert isinstance(ctx.bound_operators[0], BoundOperator)
        assert ctx.bound_operators[0].status == OperatorStatus.BOUND

    def test_bound_operators_accepts_json_string(self):
        payload = json.dumps([
            {"operator_id": "op-1", "operator_session_id": "sess-1", "status": "bound"},
        ])
        ctx = self._make(bound_operators=payload)
        assert len(ctx.bound_operators) == 1
        assert isinstance(ctx.bound_operators[0], BoundOperator)
        assert ctx.bound_operators[0].operator_id == "op-1"
        assert ctx.bound_operators[0].operator_session_id == "sess-1"
        assert ctx.bound_operators[0].status == OperatorStatus.BOUND


    def test_bound_operators_none_returns_empty(self):
        ctx = self._make(bound_operators=None)
        assert ctx.bound_operators == []

    def test_request_id_is_auto_generated(self):
        ctx = self._make()
        assert ctx.execution_id.startswith("exec_")
        assert len(ctx.execution_id) == len("exec_") + 12

    def test_each_instance_has_unique_request_id(self):
        ids = {self._make().execution_id for _ in range(20)}
        assert len(ids) == 20

    def test_request_id_can_be_overridden(self):
        ctx = self._make(execution_id="exec_custom123456")
        assert ctx.execution_id == "exec_custom123456"

    def test_timestamp_is_auto_generated_as_datetime(self):
        ctx = self._make()
        assert isinstance(ctx.timestamp, datetime)

    def test_timestamp_can_be_overridden(self):
        ctx = self._make(timestamp=_TS)
        assert ctx.timestamp == _TS

    def test_model_dump_serializes_source_component_as_string(self):
        ctx = self._make(source_component=ComponentName.G8ED)
        dumped = ctx.model_dump()
        assert dumped["source_component"] == "g8ed"

    def test_model_dump_excludes_none_by_default(self):
        ctx = self._make()
        dumped = ctx.model_dump()
        assert "organization_id" not in dumped
        assert "task_id" not in dumped
        assert "case_id" in dumped
        assert "investigation_id" in dumped

    def test_extra_fields_ignored(self):
        ctx = self._make(injected="malicious")
        assert not hasattr(ctx, "injected")

    def test_has_bound_operator_true_when_one_is_bound(self):
        ctx = self._make(bound_operators=[
            BoundOperator(operator_id="op-1", status=OperatorStatus.BOUND),
            BoundOperator(operator_id="op-2", status=OperatorStatus.AVAILABLE),
        ])
        assert ctx.has_bound_operator() is True

    def test_has_bound_operator_false_when_none_bound(self):
        ctx = self._make(bound_operators=[
            BoundOperator(operator_id="op-1", status=OperatorStatus.AVAILABLE),
            BoundOperator(operator_id="op-2", status=OperatorStatus.OFFLINE),
        ])
        assert ctx.has_bound_operator() is False

    def test_has_bound_operator_false_when_list_empty(self):
        ctx = self._make()
        assert ctx.has_bound_operator() is False

    def test_has_bound_operator_false_when_status_is_none(self):
        ctx = self._make(bound_operators=[
            BoundOperator(operator_id="op-1"),
        ])
        assert ctx.has_bound_operator() is False

    def test_no_for_logging_method(self):
        ctx = self._make()
        assert not hasattr(ctx, "for_logging")
