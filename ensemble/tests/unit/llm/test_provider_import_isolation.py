# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""Exercise startup in a fresh process so collected tests cannot mask eager imports."""

import os
import subprocess
import sys
import textwrap


def test_app_startup_validation_and_fake_calls_survive_broken_unused_sdks():
    env = os.environ.copy()
    env.update(NO_PROXY="localhost,127.0.0.1,[::1]", no_proxy="localhost,[::1]")
    result = subprocess.run(
        [
            sys.executable,
            "-c",
            textwrap.dedent("""
                import asyncio
                import importlib.abc
                import sys

                blocked = ("ollama", "openai", "anthropic", "google.genai")

                class BrokenSDKs(importlib.abc.MetaPathFinder):
                    def find_spec(self, fullname, path=None, target=None):
                        if any(fullname == name or fullname.startswith(name + ".") for name in blocked):
                            raise RuntimeError("broken SDK: " + fullname)

                sys.meta_path.insert(0, BrokenSDKs())
                import app.main
                from app.constants import LLMProvider
                from app.llm.factory import get_llm_provider, clear_provider_cache
                from app.models.settings import G8eeUserSettings, LLMSettings
                from app.services.ai.chat_pipeline import ChatPipelineService

                settings = LLMSettings(primary_provider=LLMProvider.FAKE, primary_model="fake")
                pipeline = ChatPipelineService.__new__(ChatPipelineService)
                pipeline.validate_llm_config(G8eeUserSettings(llm=settings))
                provider = get_llm_provider(settings)
                assert type(provider).__name__ == "FakeProvider"
                assert get_llm_provider(settings) is provider
                asyncio.run(clear_provider_cache())

                # Selected providers still fail explicitly; no fallback is cached.
                for selected in (LLMProvider.OLLAMA, LLMProvider.OPENAI,
                                 LLMProvider.ANTHROPIC, LLMProvider.GEMINI,
                                 LLMProvider.LLAMACPP):
                    settings = LLMSettings(primary_provider=selected,
                                           ollama_endpoint="http://localhost:11434")
                    try:
                        get_llm_provider(settings)
                    except RuntimeError as exc:
                        assert "broken SDK:" in str(exc), str(exc)
                    else:
                        raise AssertionError("selected broken provider did not fail: " + selected.value)
                assert not any(name in sys.modules for name in blocked)
                asyncio.run(clear_provider_cache())
            """),
        ],
        env=env,
        capture_output=True,
        text=True,
        timeout=30,
        check=False,
    )
    assert result.returncode == 0, result.stdout + result.stderr
