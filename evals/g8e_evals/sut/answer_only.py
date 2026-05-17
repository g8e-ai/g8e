import json
import time
import uuid
from datetime import datetime, timedelta, timezone
from typing import Optional, Dict, Any

import httpx
from google.protobuf import json_format

from g8e_evals.harness import Task, Response, BindingType, SUTConfig
from g8e_evals.proto import operator_pb2
from g8e_evals.tls import resolve_trust_bundle
from g8e_evals.uap_utils import build_envelope


class AnswerOnlySUT:
    def __init__(self, config: SUTConfig):
        self.config = config
        
        # Backward compatibility for model_provider string (Primary)
        if config.primary.provider and config.primary.model:
            self.model_provider = f"{config.primary.provider}:{config.primary.model}"
        else:
            self.model_provider = config.primary.model or "openai:gpt-4"

    async def get_answer(self, task: Task) -> Response:
        # 1. Call LLM (stubbed for now)
        answer = await self._call_llm(task.prompt)

        if self.config.mode == "baseline":
            return Response(
                answer=answer,
                model=self.model_provider,
                binding=BindingType.UNBOUND,
                unbound_reason="baseline_no_operator"
            )

        # 2. Build EVAL_ANSWER envelope
        eval_req = operator_pb2.EvalAnswerRequested(
            prompt_id=task.id,
            benchmark=task.metadata.get("benchmark", "unknown"),
            answer=answer,
            model=self.model_provider
        )
        payload = eval_req.SerializeToString()
        
        nonce = str(uuid.uuid4())
        
        env = build_envelope(
            action_type="EVAL_ANSWER",
            payload=payload,
            operator_id=self.config.operator_id,
            operator_session_id=self.config.operator_session_id,
            state_root=self.config.state_root,
            nonce=nonce,
            l2_private_key=self.config.l2_private_key,
            l2_key_id=self.config.l2_key_id
        )
        
        # Convert to protojson for the wire
        envelope_json = json_format.MessageToDict(env, preserving_proto_field_name=True)
        
        channel = f"cmd::{self.config.operator_id}::{self.config.operator_session_id}"
        
        async with httpx.AsyncClient(verify=resolve_trust_bundle()) as client:
            # Publish to operator
            pub_resp = await client.post(
                f"{self.config.operator_url}/pubsub/publish",
                json={
                    "channel": channel,
                    "data": envelope_json
                }
            )
            
            if pub_resp.status_code != 200:
                return Response(
                    answer=answer,
                    model=self.model_provider,
                    binding=BindingType.UNBOUND,
                    unbound_reason=f"Operator submission failed: {pub_resp.text}"
                )

        return Response(
            answer=answer,
            model=self.model_provider,
            transaction_id=env.id,
            binding=BindingType.RECEIPT_BOUND
        )

    async def _call_llm(self, prompt: str) -> str:
        """Call the configured LLM using the g8ee provider stack."""
        from g8e_evals.llm_client import call_llm
        return await call_llm(
            model_provider=self.model_provider, 
            prompt=prompt,
            config=self.config
        )
