# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""
g8ee - g8e Engine component.

Application structure:
- clients/: Operator transport clients (DB, KV, PubSub, Blob, HTTP)
- constants/: Application constants, enums, and configuration values
- db/: Database layer (SQLite coordination store)
- llm/: LLM provider abstraction layer
- models/: Pydantic models and data structures
- routers/: FastAPI route handlers
- services/: Business logic services
- middleware/: FastAPI middleware
- security/: Authentication and authorization
- storage/: Storage abstractions
- utils/: Utility functions
- prompts_data/: Prompt templates and data
- proto/: Protocol buffer definitions
"""

# Normalize ambient NO_PROXY/no_proxy before any third-party import reads them:
# the ollama package builds an httpx client at import time and httpx crashes
# on bracketed IPv6 literals (e.g. [::1]). See app/utils/proxy_env.py.
from app.utils.proxy_env import sanitize_proxy_env

sanitize_proxy_env()
