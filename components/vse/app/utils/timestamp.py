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

"""
Timestamp utilities for VSO platform.

Provides standardized timezone-aware timestamp functions for consistent
time handling across all VSO components.
"""


from datetime import UTC, datetime, timedelta


def now() -> datetime:
    """Get current UTC timestamp with timezone info."""
    return datetime.now(UTC)


def now_iso() -> str:
    """Get current UTC timestamp as RFC3339Nano string with Z suffix (microsecond precision)."""
    dt = datetime.now(UTC)
    return dt.strftime("%Y-%m-%dT%H:%M:%S") + f".{dt.microsecond:06d}" + "Z"


def from_timestamp(timestamp: float | int) -> datetime:
    """Convert Unix timestamp to UTC datetime."""
    return datetime.fromtimestamp(timestamp, tz=UTC)


def to_timestamp(dt: datetime) -> float:
    """Convert datetime to Unix timestamp."""
    if dt.tzinfo is None:
        dt = dt.replace(tzinfo=UTC)
    return dt.timestamp()


def parse_iso(iso_string: str) -> datetime:
    """Parse ISO 8601 datetime string to timezone-aware UTC datetime."""
    if iso_string.endswith("Z"):
        iso_string = iso_string[:-1] + "+00:00"
    try:
        dt = datetime.fromisoformat(iso_string)
        if dt.tzinfo is None:
            dt = dt.replace(tzinfo=UTC)
        return dt
    except ValueError:
        try:
            dt = datetime.strptime(iso_string, "%Y-%m-%dT%H:%M:%S")
            return dt.replace(tzinfo=UTC)
        except ValueError:
            dt = datetime.strptime(iso_string, "%Y-%m-%d %H:%M:%S")
            return dt.replace(tzinfo=UTC)


def ensure_utc(dt: datetime) -> datetime:
    """Ensure datetime is in UTC timezone."""
    if dt.tzinfo is None:
        return dt.replace(tzinfo=UTC)
    if dt.tzinfo != UTC:
        return dt.astimezone(UTC)
    return dt


def add_seconds(dt: datetime, seconds: int) -> datetime:
    """Add seconds to datetime, preserving timezone."""
    return dt + timedelta(seconds=seconds)


def add_minutes(dt: datetime, minutes: int) -> datetime:
    """Add minutes to datetime, preserving timezone."""
    return dt + timedelta(minutes=minutes)


def add_hours(dt: datetime, hours: int) -> datetime:
    """Add hours to datetime, preserving timezone."""
    return dt + timedelta(hours=hours)


def add_days(dt: datetime, days: int) -> datetime:
    """Add days to datetime, preserving timezone."""
    return dt + timedelta(days=days)


def time_ago(dt: datetime) -> str:
    """Get human-readable time difference from now."""
    delta = datetime.now(UTC) - ensure_utc(dt)
    seconds = int(delta.total_seconds())

    if seconds < 60:
        return f"{seconds} seconds ago"
    if seconds < 3600:
        minutes = seconds // 60
        return f"{minutes} minute{'s' if minutes != 1 else ''} ago"
    if seconds < 86400:
        hours = seconds // 3600
        return f"{hours} hour{'s' if hours != 1 else ''} ago"
    days = seconds // 86400
    return f"{days} day{'s' if days != 1 else ''} ago"


def time_until(dt: datetime) -> str:
    """Get human-readable time until a future datetime."""
    delta = ensure_utc(dt) - datetime.now(UTC)
    seconds = int(delta.total_seconds())

    if seconds < 0:
        return "in the past"
    if seconds < 60:
        return f"in {seconds} seconds"
    if seconds < 3600:
        minutes = seconds // 60
        return f"in {minutes} minute{'s' if minutes != 1 else ''}"
    if seconds < 86400:
        hours = seconds // 3600
        return f"in {hours} hour{'s' if hours != 1 else ''}"
    days = seconds // 86400
    return f"in {days} day{'s' if days != 1 else ''}"


def is_expired(expiry_time: datetime) -> bool:
    """Check if a datetime has passed."""
    return ensure_utc(expiry_time) < datetime.now(UTC)


def format_for_display(dt: datetime, format_str: str = "%Y-%m-%d %H:%M:%S UTC") -> str:
    """Format datetime for display purposes."""
    return ensure_utc(dt).strftime(format_str)


ISO_FORMAT = "%Y-%m-%dT%H:%M:%S%z"
DISPLAY_FORMAT = "%Y-%m-%d %H:%M:%S UTC"
LOG_FORMAT = "%Y-%m-%d %H:%M:%S.%f"
