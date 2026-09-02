import json
import os
from dataclasses import asdict, dataclass
from datetime import datetime, timezone
from http.server import BaseHTTPRequestHandler, HTTPServer
from urllib.parse import parse_qs, urlparse


STATE_FILE = os.environ.get("HEALTHCARE_STATE_FILE", "/var/healthcare/observations.jsonl")
PORT = int(os.environ.get("HEALTHCARE_ACTUATOR_PORT", "9200"))


@dataclass(frozen=True)
class OperationRequest:
    action: str
    request_id: str
    resource_type: str
    subject: str
    measured_value: int
    threshold_value: int
    run_id: str
    scenario_id: str


@dataclass(frozen=True)
class StateObservation:
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


def evaluate_operation(request: OperationRequest, evaluated_at: str) -> StateObservation:
    if not all((request.action, request.request_id, request.resource_type, request.subject, request.run_id, request.scenario_id)):
        raise ValueError("operation request is incomplete")
    if request.measured_value < 0 or request.threshold_value < 0:
        raise ValueError("operation values must be non-negative")
    if request.action == "submit" and request.scenario_id == "healthcare-success":
        status = "SUBMITTED"
        auto_approved = False
        reportable_to_oha = False
    elif request.action == "gold-card" and request.scenario_id == "healthcare-gold-card":
        auto_approved = request.measured_value >= request.threshold_value
        status = "AUTO_APPROVED" if auto_approved else "PENDING_REVIEW"
        reportable_to_oha = False
    elif request.action == "sla-check" and request.scenario_id == "healthcare-sla-breach":
        reportable_to_oha = request.measured_value > request.threshold_value
        status = "SLA_BREACHED" if reportable_to_oha else "IN_REVIEW"
        auto_approved = False
    else:
        raise ValueError("unsupported action and scenario binding")
    return StateObservation(
        action=request.action,
        request_id=request.request_id,
        resource_type=request.resource_type,
        subject=request.subject,
        measured_value=request.measured_value,
        threshold_value=request.threshold_value,
        run_id=request.run_id,
        scenario_id=request.scenario_id,
        status=status,
        auto_approved=auto_approved,
        reportable_to_oha=reportable_to_oha,
        evaluated_at=evaluated_at,
    )


def append_observation(observation: StateObservation) -> None:
    os.makedirs(os.path.dirname(STATE_FILE), exist_ok=True)
    with open(STATE_FILE, "a", encoding="utf-8") as state:
        state.write(json.dumps(asdict(observation), sort_keys=True, separators=(",", ":")) + "\n")


def find_observation(run_id: str, scenario_id: str, request_id: str) -> StateObservation | None:
    if not os.path.exists(STATE_FILE):
        return None
    match = None
    with open(STATE_FILE, encoding="utf-8") as state:
        for line in state:
            candidate = StateObservation(**json.loads(line))
            if candidate.run_id == run_id and candidate.scenario_id == scenario_id and candidate.request_id == request_id:
                match = candidate
    return match


class HealthcareActuatorHandler(BaseHTTPRequestHandler):
    def send_json(self, status: int, payload: dict) -> None:
        body = json.dumps(payload, sort_keys=True, separators=(",", ":")).encode()
        self.send_response(status)
        self.send_header("Content-Type", "application/json")
        self.send_header("Content-Length", str(len(body)))
        self.end_headers()
        self.wfile.write(body)

    def do_POST(self) -> None:
        if self.path != "/operations":
            self.send_json(404, {"error": "not found"})
            return
        try:
            length = int(self.headers.get("Content-Length", "0"))
            values = parse_qs(self.rfile.read(length).decode(), strict_parsing=True)
            request = OperationRequest(
                action=values["action"][0],
                request_id=values["request_id"][0],
                resource_type=values["resource_type"][0],
                subject=values["subject"][0],
                measured_value=int(values["measured_value"][0]),
                threshold_value=int(values["threshold_value"][0]),
                run_id=values["run_id"][0],
                scenario_id=values["scenario_id"][0],
            )
            evaluated_at = datetime.now(timezone.utc).isoformat().replace("+00:00", "Z")
            observation = evaluate_operation(request, evaluated_at)
            append_observation(observation)
            self.send_json(200, asdict(observation))
        except (KeyError, ValueError, json.JSONDecodeError) as err:
            self.send_json(400, {"error": str(err)})

    def do_GET(self) -> None:
        parsed = urlparse(self.path)
        if parsed.path == "/health":
            self.send_json(200, {"status": "ok"})
            return
        if parsed.path != "/observations":
            self.send_json(404, {"error": "not found"})
            return
        values = parse_qs(parsed.query)
        observation = find_observation(
            values.get("run_id", [""])[0],
            values.get("scenario_id", [""])[0],
            values.get("request_id", [""])[0],
        )
        if observation is None:
            self.send_json(404, {"error": "observation not found"})
            return
        self.send_json(200, asdict(observation))

    def log_message(self, format: str, *args) -> None:
        return


if __name__ == "__main__":
    os.makedirs(os.path.dirname(STATE_FILE), exist_ok=True)
    HTTPServer(("0.0.0.0", PORT), HealthcareActuatorHandler).serve_forever()
