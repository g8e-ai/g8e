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

import re
from datetime import UTC, datetime, timedelta, timezone

import pytest

from app.utils.timestamp import (
    add_days,
    add_hours,
    add_minutes,
    add_seconds,
    ensure_utc,
    format_for_display,
    from_timestamp,
    is_expired,
    now,
    now_iso,
    parse_iso,
    time_ago,
    time_until,
    to_timestamp,
)

pytestmark = [pytest.mark.unit]

RFC3339NANO_PATTERN = re.compile(
    r"^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(\.\d+)?Z$"
)


class TestNow:

    def test_returns_datetime(self):
        result = now()
        assert isinstance(result, datetime)

    def test_is_timezone_aware(self):
        result = now()
        assert result.tzinfo is not None

    def test_is_utc(self):
        result = now()
        assert result.tzinfo == UTC

    def test_is_current_time(self):
        before = datetime.now(UTC)
        result = now()
        after = datetime.now(UTC)
        assert before <= result <= after


class TestNowIso:

    def test_returns_string(self):
        result = now_iso()
        assert isinstance(result, str)

    def test_matches_rfc3339nano_pattern(self):
        result = now_iso()
        assert RFC3339NANO_PATTERN.match(result), f"Does not match RFC3339Nano: {result!r}"

    def test_ends_with_z(self):
        result = now_iso()
        assert result.endswith("Z"), f"Expected Z suffix, got: {result!r}"

    def test_always_includes_microseconds(self):
        for _ in range(10):
            result = now_iso()
            assert "." in result, f"Fractional seconds always required, got: {result!r}"
            frac = result.split(".")[1].rstrip("Z")
            assert len(frac) == 6, f"Expected 6-digit microseconds, got {len(frac)} in: {result!r}"

    def test_is_parseable_as_utc_datetime(self):
        result = now_iso()
        normalised = result[:-1] + "+00:00"
        dt = datetime.fromisoformat(normalised)
        assert dt.tzinfo is not None

    def test_is_current_time(self):
        before = datetime.now(UTC)
        result = now_iso()
        after = datetime.now(UTC)
        normalised = result[:-1] + "+00:00"
        dt = datetime.fromisoformat(normalised)
        assert before <= dt <= after

    def test_no_offset_notation(self):
        result = now_iso()
        assert "+00:00" not in result
        assert "-00:00" not in result


class TestFromTimestamp:

    def test_converts_unix_float(self):
        unix = 1_700_000_000.0
        result = from_timestamp(unix)
        assert isinstance(result, datetime)
        assert result.tzinfo == UTC

    def test_converts_unix_int(self):
        unix = 1_700_000_000
        result = from_timestamp(unix)
        assert result.tzinfo == UTC

    def test_roundtrips_with_to_timestamp(self):
        original = datetime(2026, 1, 15, 12, 30, 0, tzinfo=UTC)
        unix = to_timestamp(original)
        recovered = from_timestamp(unix)
        assert abs((recovered - original).total_seconds()) < 1e-3


class TestToTimestamp:

    def test_tz_aware_datetime(self):
        dt = datetime(2026, 3, 3, 19, 5, 0, tzinfo=UTC)
        result = to_timestamp(dt)
        assert isinstance(result, float)

    def test_naive_datetime_treated_as_utc(self):
        naive = datetime(2026, 3, 3, 19, 5, 0)
        aware = datetime(2026, 3, 3, 19, 5, 0, tzinfo=UTC)
        assert abs(to_timestamp(naive) - to_timestamp(aware)) < 1e-6


class TestParseIso:

    def test_parses_z_suffix(self):
        s = "2026-03-03T19:05:00.123456Z"
        result = parse_iso(s)
        assert result.tzinfo is not None
        assert result.year == 2026
        assert result.month == 3
        assert result.day == 3

    def test_parses_offset_notation(self):
        s = "2026-03-03T14:05:00+00:00"
        result = parse_iso(s)
        assert result.tzinfo is not None

    def test_parses_non_utc_offset_and_normalises(self):
        s = "2026-03-03T21:05:00+02:00"
        result = parse_iso(s)
        utc = result.astimezone(UTC)
        assert utc.hour == 19
        assert utc.tzinfo is not None

    def test_parses_rfc3339nano(self):
        s = "2026-03-03T19:05:00.123456789Z"
        result = parse_iso(s)
        assert result.year == 2026

    def test_parses_no_fractional_seconds(self):
        s = "2026-03-03T19:05:00Z"
        result = parse_iso(s)
        assert result.second == 0

    def test_naive_string_treated_as_utc(self):
        s = "2026-03-03T19:05:00"
        result = parse_iso(s)
        assert result.tzinfo == UTC


class TestEnsureUtc:

    def test_naive_datetime_gets_utc(self):
        naive = datetime(2026, 1, 1, 0, 0, 0)
        result = ensure_utc(naive)
        assert result.tzinfo == UTC

    def test_utc_datetime_unchanged(self):
        dt = datetime(2026, 1, 1, 0, 0, 0, tzinfo=UTC)
        result = ensure_utc(dt)
        assert result == dt
        assert result.tzinfo == UTC

    def test_non_utc_tz_converted_to_utc(self):
        eastern = timezone(timedelta(hours=-5))
        dt = datetime(2026, 1, 1, 12, 0, 0, tzinfo=eastern)
        result = ensure_utc(dt)
        assert result.tzinfo == UTC
        assert result.hour == 17


class TestAddHelpers:

    def test_add_seconds(self):
        dt = datetime(2026, 1, 1, 0, 0, 0, tzinfo=UTC)
        result = add_seconds(dt, 90)
        assert result == datetime(2026, 1, 1, 0, 1, 30, tzinfo=UTC)

    def test_add_minutes(self):
        dt = datetime(2026, 1, 1, 0, 0, 0, tzinfo=UTC)
        result = add_minutes(dt, 5)
        assert result == datetime(2026, 1, 1, 0, 5, 0, tzinfo=UTC)

    def test_add_hours(self):
        dt = datetime(2026, 1, 1, 0, 0, 0, tzinfo=UTC)
        result = add_hours(dt, 3)
        assert result == datetime(2026, 1, 1, 3, 0, 0, tzinfo=UTC)

    def test_add_days(self):
        dt = datetime(2026, 1, 1, 0, 0, 0, tzinfo=UTC)
        result = add_days(dt, 10)
        assert result == datetime(2026, 1, 11, 0, 0, 0, tzinfo=UTC)

    def test_preserves_timezone(self):
        dt = datetime(2026, 1, 1, 0, 0, 0, tzinfo=UTC)
        assert add_seconds(dt, 1).tzinfo == UTC
        assert add_minutes(dt, 1).tzinfo == UTC
        assert add_hours(dt, 1).tzinfo == UTC
        assert add_days(dt, 1).tzinfo == UTC


class TestIsExpired:

    def test_past_datetime_is_expired(self):
        past = datetime.now(UTC) - timedelta(seconds=1)
        assert is_expired(past) is True

    def test_future_datetime_is_not_expired(self):
        future = datetime.now(UTC) + timedelta(seconds=60)
        assert is_expired(future) is False

    def test_naive_past_treated_as_expired(self):
        naive_past = datetime.now() - timedelta(seconds=10)
        assert is_expired(naive_past) is True


class TestTimeAgo:

    def test_seconds_ago(self):
        dt = datetime.now(UTC) - timedelta(seconds=30)
        result = time_ago(dt)
        assert "seconds ago" in result

    def test_one_minute_ago(self):
        dt = datetime.now(UTC) - timedelta(seconds=60)
        result = time_ago(dt)
        assert "minute" in result
        assert "ago" in result

    def test_plural_minutes_ago(self):
        dt = datetime.now(UTC) - timedelta(seconds=180)
        result = time_ago(dt)
        assert "minutes ago" in result

    def test_singular_minute_ago(self):
        dt = datetime.now(UTC) - timedelta(seconds=60)
        result = time_ago(dt)
        assert result == "1 minute ago"

    def test_one_hour_ago(self):
        dt = datetime.now(UTC) - timedelta(seconds=3600)
        result = time_ago(dt)
        assert "hour" in result
        assert "ago" in result

    def test_plural_hours_ago(self):
        dt = datetime.now(UTC) - timedelta(seconds=7200)
        result = time_ago(dt)
        assert "hours ago" in result

    def test_singular_hour_ago(self):
        dt = datetime.now(UTC) - timedelta(seconds=3600)
        result = time_ago(dt)
        assert result == "1 hour ago"

    def test_days_ago(self):
        dt = datetime.now(UTC) - timedelta(days=3)
        result = time_ago(dt)
        assert "days ago" in result

    def test_singular_day_ago(self):
        dt = datetime.now(UTC) - timedelta(days=1)
        result = time_ago(dt)
        assert result == "1 day ago"

    def test_naive_datetime_accepted(self):
        naive = datetime.now() - timedelta(seconds=10)
        result = time_ago(naive)
        assert "seconds ago" in result


class TestTimeUntil:

    def test_past_datetime_returns_in_the_past(self):
        dt = datetime.now(UTC) - timedelta(seconds=10)
        result = time_until(dt)
        assert result == "in the past"

    def test_seconds_in_future(self):
        dt = datetime.now(UTC) + timedelta(seconds=30)
        result = time_until(dt)
        assert "in" in result and "seconds" in result

    def test_minutes_in_future(self):
        dt = datetime.now(UTC) + timedelta(seconds=125)
        result = time_until(dt)
        assert "minutes" in result

    def test_singular_minute(self):
        dt = datetime.now(UTC) + timedelta(seconds=65)
        result = time_until(dt)
        assert result == "in 1 minute"

    def test_hours_in_future(self):
        dt = datetime.now(UTC) + timedelta(hours=2, seconds=5)
        result = time_until(dt)
        assert "hours" in result

    def test_singular_hour(self):
        dt = datetime.now(UTC) + timedelta(hours=1, seconds=5)
        result = time_until(dt)
        assert result == "in 1 hour"

    def test_days_in_future(self):
        dt = datetime.now(UTC) + timedelta(days=5)
        result = time_until(dt)
        assert "days" in result

    def test_singular_day(self):
        dt = datetime.now(UTC) + timedelta(days=1, seconds=5)
        result = time_until(dt)
        assert result == "in 1 day"

    def test_naive_datetime_accepted(self):
        future = datetime.now() + timedelta(seconds=60)
        result = time_until(future)
        assert isinstance(result, str)


class TestFormatForDisplay:

    def test_returns_string(self):
        dt = datetime(2026, 3, 15, 12, 30, 0, tzinfo=UTC)
        result = format_for_display(dt)
        assert isinstance(result, str)

    def test_default_format_contains_date(self):
        dt = datetime(2026, 3, 15, 12, 30, 0, tzinfo=UTC)
        result = format_for_display(dt)
        assert "2026-03-15" in result

    def test_default_format_contains_utc(self):
        dt = datetime(2026, 3, 15, 12, 30, 0, tzinfo=UTC)
        result = format_for_display(dt)
        assert "UTC" in result

    def test_custom_format_applied(self):
        dt = datetime(2026, 3, 15, 12, 30, 0, tzinfo=UTC)
        result = format_for_display(dt, "%d/%m/%Y")
        assert result == "15/03/2026"

    def test_naive_datetime_treated_as_utc(self):
        naive = datetime(2026, 3, 15, 12, 0, 0)
        result = format_for_display(naive)
        assert "2026-03-15" in result


class TestNowIsoCanonicalSpec:

    def test_output_parseable_by_parse_iso(self):
        s = now_iso()
        result = parse_iso(s)
        assert result.tzinfo is not None

    def test_microseconds_never_zero_padded_wrong(self):
        for _ in range(5):
            s = now_iso()
            frac = s.split(".")[1].rstrip("Z")
            assert len(frac) == 6, f"Expected exactly 6 microsecond digits, got: {s!r}"

    def test_no_plus_offset_in_canonical_output(self):
        for _ in range(5):
            s = now_iso()
            assert "+" not in s, f"UTC offset notation in canonical timestamp: {s!r}"
