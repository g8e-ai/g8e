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

from pydantic import Field
from app.models.base import VSOBaseModel
from app.constants import VerifierReason


class VerifierRequest(VSOBaseModel):
    """Request model for the Verifier agent."""
    intent: str = Field(description="The original user intent.")
    os: str = Field(description="The operating system of the target.")
    candidate_command: str = Field(description="The command string to be verified.")


class VerifierResult(VSOBaseModel):
    """The result of a Verifier agent operation."""
    passed: bool = Field(description="True if the verifier approves the candidate.")
    revision: str | None = Field(default=None, description="The revised command string if the verifier rejects the candidate.")
    reason: str = Field(description="Reasoning for the approval or rejection.")
    reason_enum: VerifierReason = Field(description="Canonical reason for the verifier result.")
