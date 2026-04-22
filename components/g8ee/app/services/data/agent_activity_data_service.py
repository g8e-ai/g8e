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

import logging
from uuid import uuid4

from app.constants import (
    DB_COLLECTION_AGENT_ACTIVITY_METADATA,
    ComponentName,
    ErrorCode,
)
from app.errors import DatabaseError, ValidationError
from app.models.agent_activity import AgentActivityMetadata
from app.models.cache import FieldFilter
from app.services.cache.cache_aside import CacheAsideService

logger = logging.getLogger(__name__)


class AgentActivityDataService:
    """Data service for recording and querying AI agent activity metadata.
    
    Provides CRUD operations for agent activity metadata records, which are
    used for data science analysis and telemetry.
    """

    def __init__(self, cache: CacheAsideService):
        self.cache = cache
        self.collection = DB_COLLECTION_AGENT_ACTIVITY_METADATA

    async def record_activity(
        self,
        metadata: AgentActivityMetadata,
    ) -> AgentActivityMetadata:
        """Record a new agent activity metadata entry.
        
        Args:
            metadata: The activity metadata to record
            
        Returns:
            The recorded metadata with assigned ID
        """
        if not metadata.id:
            metadata.id = str(uuid4())

        logger.info(
            "Recording agent activity metadata",
            extra={
                "activity_id": metadata.id,
                "user_id": metadata.user_id,
                "investigation_id": metadata.investigation_id,
                "model": metadata.model_name,
            }
        )

        try:
            await self.cache.create_document(
                collection=self.collection,
                document_id=metadata.id,
                data=metadata.model_dump(mode="json"),
            )

            logger.info(f"Agent activity metadata recorded: {metadata.id}")
            return metadata

        except Exception as e:
            logger.error(f"Failed to record agent activity metadata {metadata.id}: {e}", exc_info=True)
            raise DatabaseError(
                message=f"Failed to record agent activity metadata: {e}",
                code=ErrorCode.DB_WRITE_ERROR,
                details={"activity_id": metadata.id},
                cause=e,
                component=ComponentName.G8EE
            )

    async def get_activity(self, activity_id: str) -> AgentActivityMetadata | None:
        """Retrieve a single activity metadata record by ID.
        
        Args:
            activity_id: The activity metadata ID
            
        Returns:
            The activity metadata, or None if not found
        """
        if not activity_id:
            raise ValidationError("Activity ID is required")

        logger.info(f"Retrieving agent activity metadata: {activity_id}")

        try:
            doc_data = await self.cache.get_document(
                collection=self.collection,
                document_id=activity_id,
            )
            if not doc_data:
                return None

            doc_data["id"] = activity_id
            return AgentActivityMetadata.model_validate(doc_data)

        except Exception as e:
            logger.error(f"Failed to retrieve agent activity metadata {activity_id}: {e}", exc_info=True)
            raise DatabaseError(
                message=f"Failed to retrieve agent activity metadata: {e}",
                code=ErrorCode.DB_QUERY_ERROR,
                details={"activity_id": activity_id},
                cause=e,
                component=ComponentName.G8EE
            )

    async def query_activities(
        self,
        user_id: str | None = None,
        investigation_id: str | None = None,
        case_id: str | None = None,
        model_name: str | None = None,
        agent_mode: str | None = None,
        limit: int = 100,
    ) -> list[AgentActivityMetadata]:
        """Query agent activity metadata with optional filters.
        
        Args:
            user_id: Filter by user ID
            investigation_id: Filter by investigation ID
            case_id: Filter by case ID
            model_name: Filter by model name
            agent_mode: Filter by agent mode
            limit: Maximum number of results to return
            
        Returns:
            List of matching activity metadata records
        """
        filters = []

        if user_id:
            filters.append(FieldFilter(field="user_id", op="==", value=user_id))
        if investigation_id:
            filters.append(FieldFilter(field="investigation_id", op="==", value=investigation_id))
        if case_id:
            filters.append(FieldFilter(field="case_id", op="==", value=case_id))
        if model_name:
            filters.append(FieldFilter(field="model_name", op="==", value=model_name))
        if agent_mode:
            filters.append(FieldFilter(field="agent_mode", op="==", value=agent_mode))

        try:
            results = await self.cache.query_documents(
                collection=self.collection,
                field_filters=[f.model_dump(mode="json") for f in filters],
                order_by={"created_at": "desc"},
                limit=limit,
            )

            return [AgentActivityMetadata.model_validate(data) for data in results]

        except Exception as e:
            logger.error(f"Failed to query agent activity metadata: {e}", exc_info=True)
            raise DatabaseError(
                message=f"Failed to query agent activity metadata: {e}",
                code=ErrorCode.DB_QUERY_ERROR,
                details={
                    "user_id": user_id,
                    "investigation_id": investigation_id,
                    "case_id": case_id,
                },
                cause=e,
                component=ComponentName.G8EE
            )

    async def delete_activity(self, activity_id: str) -> None:
        """Delete an agent activity metadata record.
        
        Args:
            activity_id: The activity metadata ID to delete
        """
        if not activity_id:
            raise ValidationError("Activity ID is required")

        logger.info(f"Deleting agent activity metadata: {activity_id}")

        try:
            await self.cache.delete_document(
                collection=self.collection,
                document_id=activity_id,
            )
            logger.info(f"Agent activity metadata deleted: {activity_id}")

        except Exception as e:
            logger.error(f"Failed to delete agent activity metadata {activity_id}: {e}", exc_info=True)
            raise DatabaseError(
                message=f"Failed to delete agent activity metadata: {e}",
                code=ErrorCode.DB_WRITE_ERROR,
                details={"activity_id": activity_id},
                cause=e,
                component=ComponentName.G8EE
            )
