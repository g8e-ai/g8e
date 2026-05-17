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

from typing import Any
from .base import G8eBaseModel, Field, ConfigDict

class SearchSettings(G8eBaseModel):
    """Unified search configuration."""
    enabled: bool = Field(False)
    project_id: str | None = Field(None)
    engine_id: str | None = Field(None)
    location: str = Field("global")
    api_key: str | None = Field(None)

class EvalJudgeSettings(G8eBaseModel):
    """Evaluation judge configuration."""
    model_config = ConfigDict(
        populate_by_name=True,
        extra="ignore",
    )
    model: str | None = Field(None, alias="eval_judge_model")
    max_output_tokens: int = Field(4096, alias="eval_judge_max_tokens")

class CommandValidationSettings(G8eBaseModel):
    """Operator command safety and validation configuration."""
    enable_whitelisting: bool = Field(False)
    whitelisted_commands: str = Field("")
    enable_blacklisting: bool = Field(True)
    enable_auto_approve: bool = Field(True)
    auto_approved_commands: str = Field("")

class BatchExecutionSettings(G8eBaseModel):
    """Batch execution configuration for operator tools."""
    max_concurrency: int = Field(10, ge=1, le=64)
    fail_fast: bool = Field(False)

class LLMSettings(G8eBaseModel):
    """LLM provider configuration."""
    model_config = ConfigDict(
        populate_by_name=True,
        extra="ignore",
    )
    primary_provider: str | None = Field(default=None, alias="llm_primary_provider")
    assistant_provider: str | None = Field(default=None, alias="llm_assistant_provider")
    lite_provider: str | None = Field(default=None, alias="llm_lite_provider")
    primary_model: str | None = Field(default=None, alias="llm_model")
    assistant_model: str | None = Field(default=None, alias="llm_assistant_model")
    lite_model: str | None = Field(default=None, alias="llm_lite_model")
    primary_api_key: str | None = Field(default=None)
    primary_endpoint: str | None = Field(default=None)
    assistant_api_key: str | None = Field(default=None)
    assistant_endpoint: str | None = Field(default=None)
    lite_api_key: str | None = Field(default=None)
    lite_endpoint: str | None = Field(default=None)

class G8eeUserSettings(G8eBaseModel):
    """Per-user settings for g8ee Engine."""
    llm: LLMSettings
    search: SearchSettings = Field(default_factory=SearchSettings)
    eval_judge: EvalJudgeSettings = Field(default_factory=EvalJudgeSettings)
    command_validation: CommandValidationSettings = Field(default_factory=CommandValidationSettings)
    batch_execution: BatchExecutionSettings = Field(default_factory=BatchExecutionSettings)
