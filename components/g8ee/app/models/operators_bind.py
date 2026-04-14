from pydantic import Field
from .base import G8eBaseModel

class BindOperatorsRequest(G8eBaseModel):
    """Request to bind multiple operators."""
    operator_ids: list[str] = Field(description="List of operator IDs to bind")

class BindOperatorsResponse(G8eBaseModel):
    """Response for bulk operator binding."""
    success: bool = Field(description="Whether the operation succeeded")
    bound_count: int = Field(default=0)
    failed_count: int = Field(default=0)
    bound_operator_ids: list[str] = Field(default_factory=list)
    failed_operator_ids: list[str] = Field(default_factory=list)
    errors: list[str] = Field(default_factory=list)
    statusCode: int = Field(default=200)
    error: str | None = Field(default=None)

class UnbindOperatorsRequest(G8eBaseModel):
    """Request to unbind multiple operators."""
    operator_ids: list[str] = Field(description="List of operator IDs to unbind")

class UnbindOperatorsResponse(G8eBaseModel):
    """Response for bulk operator unbinding."""
    success: bool = Field(description="Whether the operation succeeded")
    unbound_count: int = Field(default=0)
    failed_count: int = Field(default=0)
    unbound_operator_ids: list[str] = Field(default_factory=list)
    failed_operator_ids: list[str] = Field(default_factory=list)
    errors: list[str] = Field(default_factory=list)
    statusCode: int = Field(default=200)
    error: str | None = Field(default=None)
