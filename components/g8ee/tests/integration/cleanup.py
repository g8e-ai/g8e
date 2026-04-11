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

"""Integration test cleanup tracker.

Provides a centralized mechanism for tracking VSODB documents created
during integration tests so they can be reliably cleaned up after each
test, even on assertion failure or unexpected exceptions.

Usage via the ``cleanup`` autouse fixture in ``integration/conftest.py``::

    async def test_example(self, cache_aside_service, cleanup, all_services):
        inv_data_svc = all_services['investigation_data_service']
        created = await inv_data_svc.create_investigation(...)
        cleanup.track_investigation(created.id)
        # ... assertions ...
        # cleanup happens automatically after the test
"""

import logging
from app.services.cache.cache_aside import CacheAsideService

logger = logging.getLogger(__name__)


class IntegrationCleanupTracker:
    """Tracks VSODB documents for automatic post-test deletion."""

    def __init__(self, cache_aside_service: CacheAsideService) -> None:
        self._cache_aside = cache_aside_service
        self._tracked: list[tuple[str, str]] = []

    def track(self, collection: str, document_id: str) -> None:
        self._tracked.append((collection, document_id))

    def track_investigation(self, investigation_id: str) -> None:
        self.track("investigations", investigation_id)

    def track_operator(self, operator_id: str) -> None:
        self.track("operators", operator_id)

    def track_memory(self, investigation_id: str) -> None:
        self.track("memories", investigation_id)

    async def cleanup(self) -> None:
        for collection, doc_id in reversed(self._tracked):
            try:
                await self._cache_aside.delete_document(collection, doc_id)
            except Exception as exc:
                logger.debug(
                    "Cleanup: could not delete %s/%s: %s",
                    collection, doc_id, exc,
                )
        self._tracked.clear()
