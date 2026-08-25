# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

from app.models.base import G8eBaseModel, Field
from app.models.http_context import RequestContext
from app.models.internal_api import RequestOverrides


class TriageAnswerRequest(RequestOverrides):
    """Request model for answering a triage clarifying question.

    Identity and business context (case_id, investigation_id, web_session_id,
    user_id) come from the context field in the request body.
    """

    context: RequestContext = Field(
        ..., description="Request context with session/case/investigation identity"
    )
    question_index: int = Field(
        description="The 0-indexed position of the question being answered."
    )
    answer: bool = Field(description="The yes/no answer.")


class TriageSkipRequest(RequestOverrides):
    """Request model for skipping triage clarifying questions.

    Identity and business context (case_id, investigation_id, web_session_id,
    user_id) come from the context field in the request body.
    """

    context: RequestContext = Field(
        ..., description="Request context with session/case/investigation identity"
    )


class TriageTimeoutRequest(G8eBaseModel):
    """Request model for triage clarifying questions timeout.

    Identity and business context (case_id, investigation_id, web_session_id,
    user_id) come from the context field in the request body.
    """

    context: RequestContext = Field(
        ..., description="Request context with session/case/investigation identity"
    )
