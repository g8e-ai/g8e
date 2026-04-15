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
Pytest configuration for g8ee tests.

Fixtures only — no factory functions or plain classes here.
Factory functions and mock classes live in tests/fixtures/mocks.py.
Context/model builders live in tests/fixtures/investigations.py and
tests/fixtures/operators.py.

E2E fixtures are in tests/e2e/conftest.py.
"""

import logging
import os
import sys

import pytest
import pytest_asyncio

from app.clients.kv_cache_client import KVCacheClient
from app.clients.db_client import DBClient
from app.models.settings import G8eePlatformSettings
from app.services.infra.settings_service import SettingsService
from app.services.cache.cache_aside import CacheAsideService
from app.db.kv_service import KVService
from app.db.db_service import DBService
from app.constants import CloudSubtype, ComponentName, InvestigationStatus, LLMProvider, OperatorType
from app.models.settings import LLMSettings, SearchSettings
from app.models.model_configs import MODEL_REGISTRY, get_model_config
from tests.fakes.builder import (
    create_mock_cache_aside_service,
    build_command_service,
)
from tests.fakes.factories import (
    build_enriched_context,
)

# Global to store probed capabilities for the session
_PROBED_CAPABILITIES = {
    "supports_thinking": True,
    "supports_tools": True,
    "supports_web_search": True,
    "thinking_error": None,
    "tools_error": None,
    "web_search_error": None,
}


async def _probe_llm_capabilities(settings):
    """Probe the LLM for thinking and tool support."""
    from app.llm.factory import get_llm_provider, get_llm_settings
    from app.llm import llm_types as types
    from app.constants import ThinkingLevel

    llm = get_llm_settings()
    if not _has_llm_credentials(llm):
        return

    provider = get_llm_provider(llm)
    primary_model = llm.primary_model

    async with provider:
        # 1. Probe Thinking
        try:
            logger.info(f"[PROBE] Testing thinking support for {primary_model}...")
            thinking_config = types.ThinkingConfig(
                thinking_level=ThinkingLevel.MINIMAL,
                include_thoughts=True
            )
            thinking_llm_settings = types.PrimaryLLMSettings(
                max_output_tokens=1024,
                thinking_config=thinking_config,
                system_instruction="",
            )
            await provider.generate_content_primary(
                model=primary_model,
                contents=[types.Content(role="user", parts=[types.Part.from_text("Say 'ok'")])],
                primary_llm_settings=thinking_llm_settings,
            )
            logger.info(f"[PROBE] Thinking support confirmed for {primary_model}")
        except Exception as e:
            error_msg = str(e).lower()
            # Common error patterns for lack of thinking support
            if any(p in error_msg for p in ["thinking_config", "thinking is not supported", "invalid thinking_level", "400"]):
                _PROBED_CAPABILITIES["supports_thinking"] = False
                _PROBED_CAPABILITIES["thinking_error"] = str(e)
                logger.warning(f"[PROBE] Thinking support failed for {primary_model}: {e}")
            else:
                logger.info(f"[PROBE] Thinking probe returned non-capability error (ignoring): {e}")

        # 2. Probe Tools
        try:
            logger.info(f"[PROBE] Testing tool support for {primary_model}...")
            dummy_tool = types.ToolDeclaration(
                name="get_current_weather",
                description="Get the current weather in a given location",
                parameters={
                    "type": "object",
                    "properties": {
                        "location": {"type": "string", "description": "The city and state, e.g. San Francisco, CA"},
                    },
                    "required": ["location"],
                },
            )
            tool_group = types.ToolGroup(tools=[dummy_tool])
            tools_llm_settings = types.PrimaryLLMSettings(
                max_output_tokens=1024,
                tools=[tool_group],
                system_instruction="",
            )
            await provider.generate_content_primary(
                model=primary_model,
                contents=[types.Content(role="user", parts=[types.Part.from_text("What is the weather in London?")])],
                primary_llm_settings=tools_llm_settings,
            )
            logger.info(f"[PROBE] Tool support confirmed for {primary_model}")
        except Exception as e:
            error_msg = str(e).lower()
            # Common error patterns for lack of tool support
            if any(p in error_msg for p in ["tool", "function call", "not supported", "400"]):
                _PROBED_CAPABILITIES["supports_tools"] = False
                _PROBED_CAPABILITIES["tools_error"] = str(e)
                logger.warning(f"[PROBE] Tool support failed for {primary_model}: {e}")
            else:
                logger.info(f"[PROBE] Tool probe returned non-capability error (ignoring): {e}")

        # Update the registry for the duration of the session
        config = get_model_config(primary_model)
        config.supports_thinking = _PROBED_CAPABILITIES["supports_thinking"]
        config.supports_tools = _PROBED_CAPABILITIES["supports_tools"]

        # 3. Probe Web Search
        if settings.search and settings.search.enabled:
            from app.services.ai.grounding.web_search_provider import WebSearchProvider
            try:
                logger.info("[PROBE] Testing web search connectivity...")
                search_provider = WebSearchProvider(
                    project_id=settings.search.project_id,
                    engine_id=settings.search.engine_id,
                    api_key=settings.search.api_key,
                    location=settings.search.location or "global"
                )
                # Perform a minimal search for connectivity check
                result = await search_provider.search(query="connectivity-check", num=1)
                if result.success:
                    logger.info("[PROBE] Web search connectivity confirmed")
                else:
                    _PROBED_CAPABILITIES["supports_web_search"] = False
                    _PROBED_CAPABILITIES["web_search_error"] = result.error
                    logger.warning(f"[PROBE] Web search connectivity failed: {result.error}")
            except Exception as e:
                _PROBED_CAPABILITIES["supports_web_search"] = False
                _PROBED_CAPABILITIES["web_search_error"] = str(e)
                logger.warning(f"[PROBE] Web search probe failed with exception: {e}")

        # If the model is not in the registry yet, add it
        found = False
        for cfg in MODEL_REGISTRY.configs:
            if cfg.name == primary_model:
                cfg.supports_thinking = config.supports_thinking
                cfg.supports_tools = config.supports_tools
                found = True
                break
        if not found:
            MODEL_REGISTRY.configs.append(config)

logger = logging.getLogger(__name__)


def _has_llm_credentials(llm: LLMSettings | None) -> bool:
    """Return True if the given LLMSettings has the credentials it needs."""
    if llm is None:
        return False
    provider = llm.primary_provider
    if provider == LLMProvider.GEMINI:
        return bool(llm.gemini_api_key)
    if provider == LLMProvider.ANTHROPIC:
        return bool(llm.anthropic_api_key)
    if provider == LLMProvider.OPENAI:
        return bool(llm.openai_api_key and llm.openai_endpoint)
    if provider == LLMProvider.OLLAMA:
        return bool(llm.ollama_endpoint)
    return False


def _llm_settings_from_env() -> LLMSettings | None:
    """Build LLMSettings from TEST_LLM_* env vars set by ./g8e test flags.

    Returns None when no --llm-provider flag was supplied, which means
    ai_integration tests should be skipped.
    """
    provider_str = os.environ.get("TEST_LLM_PROVIDER", "").strip()
    if not provider_str:
        return None

    try:
        provider = LLMProvider(provider_str)
    except ValueError:
        logger.warning("TEST_LLM_PROVIDER=%s is not a valid provider", provider_str)
        return None

    assistant_provider_str = os.environ.get("TEST_LLM_ASSISTANT_PROVIDER", "").strip()
    if assistant_provider_str:
        try:
            assistant_provider = LLMProvider(assistant_provider_str)
        except ValueError:
            logger.warning("TEST_LLM_ASSISTANT_PROVIDER=%s is not a valid provider, falling back to primary", assistant_provider_str)
            assistant_provider = provider
    else:
        assistant_provider = provider

    api_key = os.environ.get("TEST_LLM_API_KEY", "").strip() or None
    endpoint = os.environ.get("TEST_LLM_ENDPOINT_URL", "").strip() or None
    assistant_api_key = os.environ.get("TEST_LLM_ASSISTANT_API_KEY", "").strip() or None
    assistant_endpoint = os.environ.get("TEST_LLM_ASSISTANT_ENDPOINT_URL", "").strip() or None
    primary = os.environ.get("TEST_LLM_PRIMARY_MODEL", "").strip() or None
    assistant = os.environ.get("TEST_LLM_ASSISTANT_MODEL", "").strip() or None
    temperature_str = os.environ.get("TEST_LLM_TEMPERATURE", "").strip() or None
    max_tokens_str = os.environ.get("TEST_LLM_MAX_TOKENS", "").strip() or None

    kwargs: dict = {"provider": provider, "assistant_provider": assistant_provider}
    if primary:
        kwargs["primary_model"] = primary
    if assistant:
        kwargs["assistant_model"] = assistant
    if temperature_str:
        try:
            kwargs["llm_temperature"] = float(temperature_str)
        except ValueError:
            logger.warning("TEST_LLM_TEMPERATURE=%s is not a valid float, ignoring", temperature_str)
    if max_tokens_str:
        try:
            kwargs["llm_max_tokens"] = int(max_tokens_str)
        except ValueError:
            logger.warning("TEST_LLM_MAX_TOKENS=%s is not a valid int, ignoring", max_tokens_str)

    _PROVIDER_KEY_FIELD = {
        LLMProvider.GEMINI: "gemini_api_key",
        LLMProvider.OPENAI: "openai_api_key",
        LLMProvider.ANTHROPIC: "anthropic_api_key",
        LLMProvider.OLLAMA: "ollama_api_key",
    }
    _PROVIDER_ENDPOINT_FIELD = {
        LLMProvider.OPENAI: "openai_endpoint",
        LLMProvider.ANTHROPIC: "anthropic_endpoint",
        LLMProvider.OLLAMA: "ollama_endpoint",
    }

    if api_key:
        field = _PROVIDER_KEY_FIELD.get(provider)
        if field:
            kwargs[field] = api_key
    if endpoint:
        field = _PROVIDER_ENDPOINT_FIELD.get(provider)
        if field:
            kwargs[field] = endpoint

    if assistant_api_key:
        field = _PROVIDER_KEY_FIELD.get(assistant_provider)
        if field:
            kwargs[field] = assistant_api_key
    if assistant_endpoint:
        field = _PROVIDER_ENDPOINT_FIELD.get(assistant_provider)
        if field:
            kwargs[field] = assistant_endpoint

    return LLMSettings(**kwargs)
    
    
def _web_search_settings_from_env() -> SearchSettings | None:
    """Build SearchSettings from TEST_WEB_SEARCH_* env vars set by ./g8e test flags.
    
    Returns None when no --web-search-* flags were supplied, which means
    requires_web_search tests should be skipped.
    """
    project_id = os.environ.get("TEST_WEB_SEARCH_PROJECT_ID", "").strip()
    engine_id = os.environ.get("TEST_WEB_SEARCH_ENGINE_ID", "").strip()
    api_key = os.environ.get("TEST_WEB_SEARCH_API_KEY", "").strip()
    location = os.environ.get("TEST_WEB_SEARCH_LOCATION", "").strip() or "global"

    if not project_id or not engine_id or not api_key:
        return None

    return SearchSettings(
        enabled=True,
        project_id=project_id,
        engine_id=engine_id,
        api_key=api_key,
        location=location
    )


async def _load_settings_from_g8es(timeout: float = 5.0) -> G8eePlatformSettings:
    """Load platform settings via SettingsService with a timeout."""
    import asyncio
    settings_service = SettingsService()
    bootstrap_settings = settings_service.get_local_settings()

    try:
        # Wrap the entire setup and fetch in a timeout
        async with asyncio.timeout(timeout):
            db_client = DBClient(
                ca_cert_path=bootstrap_settings.ca_cert_path,
                internal_auth_token=bootstrap_settings.auth.internal_auth_token
            )
            await db_client.connect()

            kv_client = KVCacheClient(
                component_name=ComponentName.G8EE,
                ca_cert_path=bootstrap_settings.ca_cert_path,
                internal_auth_token=bootstrap_settings.auth.internal_auth_token
            )
            await kv_client.connect()

            cache_aside = CacheAsideService(
                kv=KVService(kv_client),
                db=DBService(db_client),
                component_name=ComponentName.G8EE
            )
            settings_service._cache_aside = cache_aside

            try:
                return await settings_service.get_platform_settings()
            finally:
                await kv_client.close()
                await db_client.close()
    except TimeoutError:
        logger.warning("Timed out loading platform settings from g8es (limit %ds)", timeout)
        return bootstrap_settings
    except Exception as e:
        logger.warning("Failed to load platform settings from g8es: %s", e)
        return bootstrap_settings


def pytest_configure(config):
    import asyncio
    from app.llm.factory import set_settings, set_llm_settings, set_search_settings

    logger.info("Pytest configure started.")

    # Only load settings if they haven't been set yet
    try:
        settings = asyncio.run(_load_settings_from_g8es())
    except Exception as e:
        logger.warning(f"Failed to load settings from g8es: {e}")
        from app.services.infra.settings_service import SettingsService
        settings = SettingsService().get_local_settings()

    set_settings(settings)

    env_llm = _llm_settings_from_env()
    if env_llm is not None:
        logger.info(f"Overriding LLM settings from env: provider={env_llm.primary_provider}")
        set_llm_settings(env_llm)

    env_search = _web_search_settings_from_env()
    if env_search is not None:
        logger.info("Overriding web search settings from env")
        set_search_settings(env_search)

    # Probing is now deferred to the 'probed_capabilities' fixture to avoid startup latency.


def pytest_collection_modifyitems(config, items):
    from app.llm.factory import get_settings, get_llm_settings, get_search_settings
    settings = get_settings()
    llm = get_llm_settings()
    search_settings = get_search_settings()

    # NOTE: This hook runs before fixtures like 'probed_capabilities'.
    # It relies on _PROBED_CAPABILITIES which starts with optimistic defaults.
    # Capability-based skipping will only be fully accurate if the probe 
    # was already run in pytest_configure, but we moved it to a fixture 
    # to avoid latency. Tests requiring capabilities they don't have 
    # will now fail later during execution rather than being skipped here,
    # unless they are explicitly marked with capabilities.

    has_llm_credentials = _has_llm_credentials(llm)
    has_vertex_search = search_settings.enabled if search_settings else False
    has_web_search = (
        search_settings.enabled
        and bool(search_settings.project_id)
        and bool(search_settings.engine_id)
        and bool(search_settings.api_key)
    ) if search_settings else False

    logger.info(f"Collection: settings={settings is not None}, has_creds={has_llm_credentials}, has_vertex={has_vertex_search}, has_web_search={has_web_search}")
    if llm:
        logger.info(f"LLM Config: provider={llm.primary_provider}, key_set={bool(llm.gemini_api_key)}")

    skip_no_llm = pytest.mark.skip(reason=f"ai_integration tests require LLM flags. has_creds={has_llm_credentials}")
    skip_no_vertex = pytest.mark.skip(reason="requires_api tests require Vertex AI Search credentials (VERTEX_SEARCH_ENABLED, VERTEX_SEARCH_PROJECT_ID, VERTEX_SEARCH_ENGINE_ID, VERTEX_SEARCH_API_KEY)")
    skip_no_web_search = pytest.mark.skip(reason=f"requires_web_search tests require web search configuration (search.enabled, project_id, engine_id, api_key). has_web_search={has_web_search}")
    
    # Capability-based skips
    skip_no_thinking = pytest.mark.skip(reason=f"Model {llm.primary_model if llm else 'None'} proven not to support thinking: {_PROBED_CAPABILITIES['thinking_error']}")
    skip_no_tools = pytest.mark.skip(reason=f"Model {llm.primary_model if llm else 'None'} proven not to support tools: {_PROBED_CAPABILITIES['tools_error']}")
    skip_web_search_unreachable = pytest.mark.skip(reason=f"Web search service unreachable: {_PROBED_CAPABILITIES['web_search_error']}")

    for item in items:
        if item.get_closest_marker("ai_integration") and not has_llm_credentials:
            item.add_marker(skip_no_llm)
        if item.get_closest_marker("requires_api") and not has_vertex_search:
            item.add_marker(skip_no_vertex)
        if item.get_closest_marker("requires_web_search"):
            if not has_web_search:
                item.add_marker(skip_no_web_search)
            elif not _PROBED_CAPABILITIES["supports_web_search"]:
                item.add_marker(skip_web_search_unreachable)
            
        # Dynamically add markers based on scenario data for accuracy tests
        if "test_agent_accuracy" in item.name or "test_gemini_accuracy" in item.name:
            scenario = item.callspec.params.get("scenario") if hasattr(item, "callspec") else None
            if scenario:
                if scenario.get("agent_mode") == "operator_bound" or scenario.get("expected_tools"):
                    item.add_marker(pytest.mark.tools)
                
                # Assume all complex scenarios benefit from thinking
                if scenario.get("agent_mode") == "operator_bound":
                    item.add_marker(pytest.mark.thinking)

        # Dynamically skip based on probed capabilities
        if item.get_closest_marker("thinking") and not _PROBED_CAPABILITIES["supports_thinking"]:
            item.add_marker(skip_no_thinking)
        if item.get_closest_marker("tools") and not _PROBED_CAPABILITIES["supports_tools"]:
            item.add_marker(skip_no_tools)


# ---------------------------------------------------------------------------
# Unit test fixtures — mocks and fakes
# ---------------------------------------------------------------------------


class TaskTracker:
    """Helper for capturing and cleaning up coroutines and tasks created during tests.

    Provides a clean pattern for capturing asyncio objects to ensure they are properly
    closed or cancelled, avoiding RuntimeWarnings and side-effects.

    Usage:
        @pytest.mark.asyncio
        async def test_something(task_tracker):
            # 1. Track a coroutine manually
            coro = some_coro()
            task_tracker.track(coro)

            # 2. Patch create_task for a specific module
            with task_tracker.patch_create_task("app.routers.internal_router"):
                await function_under_test()
    """

    def __init__(self):
        import asyncio
        self._captured_coros = []
        self._captured_tasks = []
        # Store original create_task so we can still use it even if patched globally
        self._original_create_task = asyncio.create_task

    def track(self, coro_or_task):
        """Track a coroutine or task for automatic cleanup."""
        import asyncio
        if asyncio.iscoroutine(coro_or_task):
            self._captured_coros.append(coro_or_task)
        elif isinstance(coro_or_task, asyncio.Task):
            self._captured_tasks.append(coro_or_task)
        return coro_or_task

    def _fake_create_task(self, coro):
        import asyncio
        if not asyncio.iscoroutine(coro):
            # If it's not a real coroutine (e.g. it's a mock), 
            # we just return a new mock to represent the task.
            from unittest.mock import MagicMock
            return MagicMock()

        task = self._original_create_task(coro)
        self._captured_tasks.append(task)
        return task

    def patch_create_task(self, module_path):
        """Return a context manager that patches asyncio.create_task at the given path.

        The patch will create REAL tasks (so they actually run) but they will be
        automatically cancelled and awaited during cleanup.
        """
        from unittest.mock import patch

        class _TaskPatchContext:
            def __init__(self, tracker):
                self._tracker = tracker
                self._patch = None

            def __enter__(self):
                self._patch = patch(f"{module_path}.asyncio.create_task", side_effect=self._tracker._fake_create_task)
                self._patch.__enter__()
                return self

            def __exit__(self, exc_type, exc_val, exc_tb):
                self._patch.__exit__(exc_type, exc_val, exc_tb)

        return _TaskPatchContext(self)

    async def cleanup(self):
        """Close coroutines and cancel tasks."""
        import asyncio
        # 1. Close plain coroutines
        for coro in self._captured_coros:
            try:
                coro.close()
            except Exception:
                pass
        self._captured_coros.clear()

        # 2. Cancel and await tasks
        for task in self._captured_tasks:
            if not task.done():
                task.cancel()
        
        if self._captured_tasks:
            # Gather all tasks to ensure they are awaited, even if they were cancelled
            await asyncio.gather(*self._captured_tasks, return_exceptions=True)
        self._captured_tasks.clear()


@pytest_asyncio.fixture(scope="function")
async def task_tracker():
    """Provide a TaskTracker instance for capturing and cleaning up coroutines."""
    tracker = TaskTracker()
    yield tracker
    await tracker.cleanup()


@pytest.fixture(scope="function")
def unique_investigation_id():
    """Generate unique investigation ID for test isolation."""
    import uuid
    return f"test-inv-{uuid.uuid4().hex[:8]}"


@pytest.fixture(scope="function")
def unique_user_id():
    """Generate unique user ID for test isolation."""
    import uuid
    return f"test-user-{uuid.uuid4().hex[:8]}"


@pytest.fixture(scope="function")
def unique_case_id():
    """Generate unique case ID for test isolation."""
    import uuid
    return f"test-case-{uuid.uuid4().hex[:8]}"


@pytest.fixture(scope="function")
def unique_operator_id():
    """Generate unique operator ID for test isolation."""
    import uuid
    return f"test-op-{uuid.uuid4().hex[:8]}"


@pytest.fixture(scope="function")
def unique_session_id():
    """Generate unique session ID for test isolation."""
    import uuid
    return f"test-sess-{uuid.uuid4().hex[:8]}"


@pytest.fixture(scope="function")
def unique_web_session_id():
    """Generate unique web session ID for test isolation."""
    import uuid
    return f"test-ws-{uuid.uuid4().hex[:8]}"


@pytest.fixture(scope="function")
def mock_operator_document():
    """Mock OperatorDocument for evaluation tests.

    Returns a properly configured OperatorDocument with system info
    for agent accuracy testing scenarios.
    """
    from tests.fakes.factories import build_mock_operator_document
    return build_mock_operator_document()


@pytest_asyncio.fixture(scope="session", autouse=True)
async def probed_capabilities(test_settings):
    """Session-scoped fixture to probe LLM capabilities.

    This defers the expensive probing logic from pytest_configure to the
    actual start of the test session, allowing for graceful timeouts and
    better visibility.
    """
    import asyncio
    from app.llm.factory import get_llm_settings
    if _has_llm_credentials(get_llm_settings()):
        try:
            # Apply a 30s timeout to the entire probing session
            async with asyncio.timeout(30.0):
                await _probe_llm_capabilities(test_settings)
        except TimeoutError:
            logger.warning("LLM capability probing timed out after 30s")
        except Exception as e:
            logger.warning(f"LLM capability probing failed: {e}")
    return _PROBED_CAPABILITIES


@pytest.fixture(scope="session")
def test_settings():
    """Returns the globally configured Settings object.
    
    In a real test run, this is loaded from g8es by pytest_sessionstart.
    """
    from app.llm.factory import get_settings
    return get_settings()


@pytest.fixture
def mock_cache_aside_service():
    """Pure MagicMock spec'd to CacheAsideService for unit tests."""
    from tests.fakes.builder import create_pure_mock_cache_aside
    return create_pure_mock_cache_aside()


@pytest.fixture
def fake_cache_aside_service():
    """Real CacheAsideService backed by MagicMock clients for unit tests."""
    return create_mock_cache_aside_service()


@pytest.fixture
def mock_blob_service():
    """Pure MagicMock spec'd to BlobService for unit tests."""
    from unittest.mock import MagicMock
    from app.db.blob_service import BlobService
    mock = MagicMock(spec=BlobService)
    return mock


@pytest.fixture
def mock_event_service():
    from tests.fakes.fake_event_service import FakeEventService
    return FakeEventService()


@pytest.fixture
def mock_settings():
    return G8eePlatformSettings(port=443)


@pytest.fixture
def mock_g8ed_http_client():
    from tests.fakes.fake_g8ed_client import FakeG8edClient
    return FakeG8edClient()


@pytest.fixture
def mock_investigation_service():
    from tests.fakes.fake_investigation_service import FakeInvestigationService
    return FakeInvestigationService()


@pytest.fixture
def mock_db_service():
    from tests.fakes.fake_db_service import FakeDBService
    return FakeDBService()


# ---------------------------------------------------------------------------
# Unit test fixtures — domain objects (using consolidated factories)
# ---------------------------------------------------------------------------

@pytest.fixture
def enriched_investigation():
    return build_enriched_context()


@pytest.fixture
def cloud_operator_doc():
    from app.models.operators import (
        OperatorDocument,
        OperatorSystemInfo,
        OperatorHeartbeat,
        SystemInfoOSDetails,
        SystemInfoUserDetails,
        SystemInfoDiskDetails,
        SystemInfoMemoryDetails,
        HeartbeatEnvironment,
    )
    return OperatorDocument(
        operator_id="cloud-op-1",
        operator_session_id="session-cloud-op-1",
        operator_type=OperatorType.CLOUD,
        cloud_subtype=CloudSubtype.AWS,
        granted_intents=["ec2_discovery", "s3_read"],
        system_info=OperatorSystemInfo(
            hostname="ip-10-0-1-100.ec2.internal",
            os="Amazon Linux 2023",
            architecture="x86_64",
            cpu_count=4,
            memory_mb=8192,
            public_ip="54.123.45.67",
        ),
        latest_heartbeat_snapshot=OperatorHeartbeat(
            os_details=SystemInfoOSDetails(distro="Amazon Linux", kernel="6.1.0", version="2023"),
            user_details=SystemInfoUserDetails(username="ec2-user", home="/home/ec2-user", shell="/bin/bash"),
            disk_details=SystemInfoDiskDetails(percent=45.2, total_gb=100, free_gb=54.8),
            memory_details=SystemInfoMemoryDetails(percent=62.1, total_mb=8192, available_mb=3105),
            environment=HeartbeatEnvironment(pwd="/home/ec2-user", timezone="UTC"),
        ),
    )


@pytest.fixture
def binary_operator_doc():
    from app.models.operators import (
        OperatorDocument,
        OperatorSystemInfo,
        OperatorHeartbeat,
        SystemInfoOSDetails,
        SystemInfoUserDetails,
        SystemInfoDiskDetails,
        SystemInfoMemoryDetails,
        HeartbeatEnvironment,
    )
    return OperatorDocument(
        operator_id="binary-op-1",
        operator_session_id="session-binary-op-1",
        operator_type=OperatorType.SYSTEM,
        cloud_subtype=None,
        system_info=OperatorSystemInfo(
            hostname="web-server-1",
            os="Ubuntu 22.04",
            architecture="x86_64",
            cpu_count=8,
            memory_mb=16384,
        ),
        latest_heartbeat_snapshot=OperatorHeartbeat(
            os_details=SystemInfoOSDetails(distro="Ubuntu", kernel="5.15.0", version="22.04"),
            user_details=SystemInfoUserDetails(username="root", home="/root", shell="/bin/bash"),
            disk_details=SystemInfoDiskDetails(percent=10.0, total_gb=500, free_gb=450),
            memory_details=SystemInfoMemoryDetails(percent=20.0, total_mb=16384, available_mb=13107),
            environment=HeartbeatEnvironment(pwd="/root", timezone="UTC"),
        ),
    )


@pytest.fixture
def multi_operator_investigation(cloud_operator_doc, binary_operator_doc):
    return build_enriched_context(
        investigation_id="inv-test-123",
        case_id="case-test-123",
        user_id="user-test-123",
        status=InvestigationStatus.OPEN,
        operator_documents=[cloud_operator_doc, binary_operator_doc],
    )


@pytest.fixture
def provider_config():
    """GenerateContentConfig with default values for unit tests.
    
    This follows the documented pattern in testing.md and provides
    a default configuration for isolated unit tests.
    """
    from app.llm.llm_types import GenerateContentConfig
    from app.constants import LLM_DEFAULT_TEMPERATURE, LLM_DEFAULT_MAX_OUTPUT_TOKENS
    return GenerateContentConfig(
        temperature=LLM_DEFAULT_TEMPERATURE,
        max_output_tokens=LLM_DEFAULT_MAX_OUTPUT_TOKENS,
    )




# ---------------------------------------------------------------------------
# Real g8es fixtures for integration tests
# ---------------------------------------------------------------------------

@pytest_asyncio.fixture(scope="session", loop_scope="session")
async def cache_aside_service(test_settings):
    from app.db.db_service import DBService
    from app.db.kv_service import KVService
    from app.db import DBClient
    from app.services.cache.cache_aside import CacheAsideService
    
    settings = test_settings
    
    raw_kv = KVCacheClient(
        ca_cert_path=settings.ca_cert_path,
        internal_auth_token=settings.auth.internal_auth_token,
        component_name=ComponentName.G8EE,
    )
    await raw_kv.connect()

    raw_db = DBClient(
        ca_cert_path=settings.ca_cert_path,
        internal_auth_token=settings.auth.internal_auth_token
    )
    await raw_db.connect()
    
    kv = KVService(raw_kv)
    db = DBService(raw_db)
    
    yield CacheAsideService(
        kv=kv,
        db=db,
        component_name=ComponentName.G8EE,
        default_ttl=settings.listen.default_ttl,
    )
    await raw_db.close()
    await raw_kv.close()


@pytest_asyncio.fixture(scope="session", loop_scope="session")
async def kv_cache_client(cache_aside_service):
    # Returns the client from the shared cache service to ensure token consistency
    yield cache_aside_service.kv.client


@pytest_asyncio.fixture(scope="session", loop_scope="session")
async def db_client(cache_aside_service):
    yield cache_aside_service.db_client


@pytest_asyncio.fixture(scope="session", loop_scope="session")
async def pubsub_service(test_settings):
    from app.clients.pubsub_client import PubSubClient
    
    settings = test_settings
    
    client = PubSubClient(
        pubsub_url=settings.listen.pubsub_url,
        internal_auth_token=settings.auth.internal_auth_token,
        component_name=ComponentName.G8EE,
    )
    await client.connect()
    
    class FakeService:
        def __init__(self, c):
            self.pubsub_client = c
            
    yield FakeService(client)
    await client.close()


@pytest_asyncio.fixture(scope="session", loop_scope="session")
async def pubsub_client(pubsub_service):
    """Returns the shared PubSubClient instance from pubsub_service."""
    yield pubsub_service.pubsub_client


@pytest_asyncio.fixture(scope="session", loop_scope="session")
async def db_service(test_settings, cache_aside_service):
    from app.services.investigation.investigation_data_service import InvestigationDataService
    yield InvestigationDataService(cache=cache_aside_service)


@pytest_asyncio.fixture(scope="session", loop_scope="session")
async def llm_provider():
    """Returns a configured LLMProvider instance for the session."""
    from app.llm.factory import get_llm_provider, get_llm_settings
    llm = get_llm_settings()
    if llm is None:
        pytest.skip("No LLM settings configured")
    provider = get_llm_provider(llm)
    yield provider
    await provider.close()


@pytest.fixture(scope="session")
def memory_crud(cache_aside_service):
    from app.services.data.memory_service import MemoryCRUDService
    return MemoryCRUDService(cache_aside_service=cache_aside_service)


@pytest.fixture(scope="session")
def memory_service(memory_crud):
    from app.services.ai.memory_generation_service import MemoryGenerationService
    return MemoryGenerationService(memory_crud=memory_crud)


# ---------------------------------------------------------------------------
# End of fixtures
# ---------------------------------------------------------------------------
