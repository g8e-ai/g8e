# Copyright (c) 2026 Lateralus Labs, LLC.
# Licensed under the Apache License, Version 2.0 (the "License");

import pytest
from typing import get_args, Union
from app.models.pubsub_messages import G8eOutboundPayload
import app.models.command_request_payloads as payloads

def test_g8ee_outbound_payload_union_is_exhaustive():
    """
    Contract test for Phase 8 of the MCP Rip.
    
    This test asserts that G8eOutboundPayload (the Union used in G8eMessage) 
    includes all defined *RequestPayload classes from command_request_payloads.py.
    
    If you add a new RequestPayload class, you MUST add it to the Union in 
    app/models/pubsub_messages.py, or this test will fail.
    """
    # Get all classes defined in the payloads module
    payload_classes = [
        cls for name, cls in payloads.__dict__.items()
        if isinstance(cls, type) and name.endswith("RequestPayload")
    ]
    
    # Get the types included in the Union
    union_types = get_args(G8eOutboundPayload)
    
    missing = []
    for cls in payload_classes:
        if cls not in union_types:
            missing.append(cls.__name__)
            
    assert not missing, (
        f"G8eOutboundPayload union is missing these request payload types: {', '.join(missing)}. "
        "Add them to G8eOutboundPayload in app/models/pubsub_messages.py to ensure "
        "they are properly serialized in G8eMessage."
    )

def test_g8ee_outbound_payload_has_discriminator():
    """
    Asserts that every member of G8eOutboundPayload has a payload_type field.
    """
    union_types = get_args(G8eOutboundPayload)
    for cls in union_types:
        assert "payload_type" in cls.model_fields, f"{cls.__name__} is missing 'payload_type' field"
