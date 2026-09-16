# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

from g8e.models.events import (
    ModelBoundaryPrivacyAttestation,
    ModelCallTelemetry,
)

from app.models.base import G8eBaseModel

__all__ = [
    "GovernedDispatchEvidence",
    "ModelBoundaryPrivacyAttestation",
    "ModelCallTelemetry",
]


class GovernedDispatchEvidence(G8eBaseModel):
    """Evidence recorded by the G8E governed-dispatch provider per call.

    Captured from the verified ``InferenceDispatchResponse`` so downstream
    telemetry can bind each model call to the governed transaction, the
    result digest bound into the signed receipt, and the receipt's final
    status name. Internal to the ensemble; never crosses the wire as its
    own message - the fields flatten onto ``ModelCallTelemetry.governed_*``.
    """

    transaction_id: str = ""
    result_digest: str = ""
    receipt_status: str = ""
    provider_attempt_id: str = ""
    requested_model: str = ""
    served_model: str = ""
    model_digest: str = ""
    normalized_request_hash: str = ""
    output_hash: str = ""
    campaign_id: str = ""
    run_id: str = ""
    assignment_id: str = ""
    evaluation_attempt_id: str = ""
    scenario_id: str = ""
    model_registry_digest: str = ""
