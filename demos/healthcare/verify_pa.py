import json
import sys
import urllib.parse
import urllib.request
from dataclasses import dataclass


ACTUATOR_URL = "http://localhost:9200/observations"


@dataclass(frozen=True)
class ExpectedObservation:
    run_id: str
    scenario_id: str
    request_id: str
    action: str
    status: str
    measured_value: int
    threshold_value: int


@dataclass(frozen=True)
class ObservedState:
    action: str
    request_id: str
    resource_type: str
    subject: str
    measured_value: int
    threshold_value: int
    run_id: str
    scenario_id: str
    status: str
    auto_approved: bool
    reportable_to_oha: bool
    evaluated_at: str


def collect(expected: ExpectedObservation) -> ObservedState:
    query = urllib.parse.urlencode({
        "run_id": expected.run_id,
        "scenario_id": expected.scenario_id,
        "request_id": expected.request_id,
    })
    with urllib.request.urlopen(ACTUATOR_URL + "?" + query, timeout=5) as response:
        return ObservedState(**json.loads(response.read()))


def verify(expected: ExpectedObservation, observed: ObservedState) -> None:
    values = (
        ("run_id", observed.run_id, expected.run_id),
        ("scenario_id", observed.scenario_id, expected.scenario_id),
        ("request_id", observed.request_id, expected.request_id),
        ("action", observed.action, expected.action),
        ("status", observed.status, expected.status),
        ("measured_value", observed.measured_value, expected.measured_value),
        ("threshold_value", observed.threshold_value, expected.threshold_value),
    )
    for field, actual, wanted in values:
        if actual != wanted:
            raise ValueError(f"{field} mismatch: got {actual!r}, want {wanted!r}")


def main() -> None:
    if len(sys.argv) != 8:
        raise ValueError("usage: verify_pa.py RUN_ID SCENARIO_ID REQUEST_ID ACTION STATUS MEASURED_VALUE THRESHOLD_VALUE")
    expected = ExpectedObservation(
        run_id=sys.argv[1],
        scenario_id=sys.argv[2],
        request_id=sys.argv[3],
        action=sys.argv[4],
        status=sys.argv[5],
        measured_value=int(sys.argv[6]),
        threshold_value=int(sys.argv[7]),
    )
    observed = collect(expected)
    verify(expected, observed)
    print(json.dumps(observed.__dict__, sort_keys=True, separators=(",", ":")))


if __name__ == "__main__":
    try:
        main()
    except Exception as err:
        print(f"ERROR: {err}", file=sys.stderr)
        sys.exit(1)
