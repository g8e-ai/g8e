# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""
g8ee Routers

API routers for g8ee endpoints.

Router modules:
- chat_router.py: Chat API endpoints
- health_router.py: Health check endpoints
- internal_router.py: Internal API endpoints for operator communication
"""

from .chat_router import router as chat_router
from .health_router import router as health_router

__all__ = [
    "chat_router",
    "health_router",
]
