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
from app.models.base import G8eBaseModel

class TriageAnswerRequest(G8eBaseModel):
    """Request model for answering a triage clarifying question."""
    investigation_id: str = Field(description="The investigation ID.")
    question_index: int = Field(description="The 0-indexed position of the question being answered.")
    answer: bool = Field(description="The yes/no answer.")

class TriageSkipRequest(G8eBaseModel):
    """Request model for skipping triage clarifying questions."""
    investigation_id: str = Field(description="The investigation ID.")

class TriageTimeoutRequest(G8eBaseModel):
    """Request model for triage clarifying questions timeout."""
    investigation_id: str = Field(description="The investigation ID.")
