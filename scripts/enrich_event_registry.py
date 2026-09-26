#!/usr/bin/env python3
# Copyright (c) 2026 Lateralus Labs, LLC.
"""W1 registry enrichment for events.json (kind, transport, producers, persistence)."""

from __future__ import annotations

import json
import re
from pathlib import Path

REPO = Path(__file__).resolve().parents[1]
EVENTS_PATH = REPO / "protocol/constants/events.json"
BUNDLED_PATH = REPO / "protocol/python/g8e/_data/events.json"

DOCUMENT_GOV = {
    "action_type": "DOCUMENT_UPDATE",
    "payload": "g8e.operator.v1.DocumentUpdateRequested",
}
DOCUMENT_DELETE_GOV = {
    "action_type": "DOCUMENT_DELETE",
    "payload": "g8e.operator.v1.DocumentDeleteRequested",
}

GRAMMAR_ALLOWLIST = {
    "AiLLMChatFilterEvent": "lead",
    "PlatformNotification": "lead",
    "PlatformAuthInfo": "lead",
    "OperatorCommandExecution": "lead",
    "OperatorCommandResult": "lead",
    "AiLLMChatStopShow": "lead",
    "AiLLMChatStopHide": "lead",
    "AppInvestigationStatusUpdatedOpen": "lead",
    "AppInvestigationStatusUpdatedClosed": "lead",
    "AppInvestigationStatusUpdatedEscalated": "lead",
    "AppInvestigationStatusUpdatedResolved": "lead",
    "SourceAiAssistant": "w3",
    "SourceAiPrimary": "w3",
    "SourceAiTriage": "w3",
    "SourceSystem": "w3",
    "SourceUserChat": "w3",
    "SourceUserTerminal": "w3",
}

OUTCOME_TERMINALS = {
    "completed",
    "failed",
    "cancelled",
    "timeout",
    "created",
    "updated",
    "deleted",
    "granted",
    "rejected",
    "denied",
    "revoked",
    "started",
    "received",
    "acknowledged",
    "sent",
    "bound",
    "unbound",
    "opened",
    "closed",
    "established",
    "exported",
    "published",
    "rotated",
    "available",
    "invoked",
    "reached",
    "detected",
    "resolved",
    "appended",
    "truncated",
    "retry",
}

FACT_TERMINALS = {"recorded", "checkpointed", "heartbeat"}

PRODUCER_PATH_PREFIXES: tuple[tuple[str, str], ...] = (
    ("ensemble/app/", "ensemble"),
    ("ensemble/tests/", "ensemble"),
    ("internal/services/gateway/", "gateway"),
    ("internal/cli/", "cli"),
    ("internal/services/mcp/", "mcp"),
    ("internal/services/pubsub/", "operator"),
    ("internal/services/governance/", "operator"),
    ("internal/services/storage/", "operator"),
    ("internal/services/inference/", "operator"),
    ("dashboard/", "dashboard"),
    ("test/e2e/", "cli"),
    ("internal/tools/", "cli"),
)

SCAN_SUFFIXES = {".py", ".go", ".js", ".ts", ".tsx"}
SCAN_SKIP_PARTS = {
    "vendor",
    "node_modules",
    ".venv",
    "protocol/python/g8e/_data",
    "protocol/constants/events.json",
    "dashboard/public/js/constants/events.js",
}

ALLOWED_PRODUCERS = {"gateway", "operator", "ensemble", "dashboard", "cli", "mcp"}
ALLOWED_PERSISTENCE = {
    "operator.audit_log",
    "gateway.audit_log",
    "gateway.sse_store",
    "gateway.operator_docs",
    "gateway.docstore",
    "ephemeral",
}


def terminal(value: str) -> str:
    return value.removeprefix("g8e.v1.").split(".")[-1]


def infer_kind(value: str) -> str:
    if value.endswith(".requested"):
        return "request"
    if any(token in value for token in (".stream.", ".chunk.", ".delta.", ".keepalive.", ".thinking.")):
        return "stream"
    if value.endswith(".recorded") or value.endswith(".checkpointed") or ".status.updated" in value:
        return "fact"
    if terminal(value) in FACT_TERMINALS:
        return "fact"
    if terminal(value) in OUTCOME_TERMINALS or value.endswith(".progress.updated"):
        return "outcome"
    if value.startswith("g8e.v1.source."):
        return "fact"
    return "outcome"


def infer_transport(value: str, kind: str) -> list[str] | None:
    if kind == "stream" or value.startswith("g8e.v1.platform.sse."):
        return ["sse"]
    if kind == "request" and infer_governance_key(value) is not None:
        return ["governed"]
    if value.startswith("g8e.v1.operator.") and (
        ".command." in value
        or ".heartbeat." in value
        or ".audit." in value
        or value.endswith(".requested")
    ):
        return ["pubsub"]
    return None


def infer_governance_key(value: str) -> dict | None:
    mapping = {
        "g8e.v1.app.case.create.requested": DOCUMENT_GOV,
        "g8e.v1.app.case.update.requested": DOCUMENT_GOV,
        "g8e.v1.app.case.delete.requested": DOCUMENT_DELETE_GOV,
        "g8e.v1.app.investigation.create.requested": DOCUMENT_GOV,
        "g8e.v1.app.investigation.update.requested": DOCUMENT_GOV,
        "g8e.v1.app.investigation.delete.requested": DOCUMENT_DELETE_GOV,
        "g8e.v1.app.memory.create.requested": DOCUMENT_GOV,
        "g8e.v1.app.memory.update.requested": DOCUMENT_GOV,
        "g8e.v1.app.agent.activity.record.requested": DOCUMENT_GOV,
        "g8e.v1.operator.reputation.state.update.requested": DOCUMENT_GOV,
        "g8e.v1.app.document.update.requested": DOCUMENT_GOV,
        "g8e.v1.app.document.delete.requested": DOCUMENT_DELETE_GOV,
    }
    return mapping.get(value)


def new_entries() -> dict[str, dict]:
    audit_pairs = [
        ("OperatorAuditUserRecordRequested", "EventOperatorAuditUserRecordRequested", "g8e.v1.operator.audit.user.record.requested", "OperatorAuditUserRecorded"),
        ("OperatorAuditAiRecordRequested", "EventOperatorAuditAiRecordRequested", "g8e.v1.operator.audit.ai.record.requested", "OperatorAuditAiRecorded"),
        ("OperatorAuditCommandRecordRequested", "EventOperatorAuditCommandRecordRequested", "g8e.v1.operator.audit.command.record.requested", "OperatorAuditCommandRecorded"),
        ("OperatorAuditDirectCommandRecordRequested", "EventOperatorAuditDirectCommandRecordRequested", "g8e.v1.operator.audit.direct.command.record.requested", "OperatorAuditDirectCommandRecorded"),
        ("OperatorAuditDirectCommandResultRecordRequested", "EventOperatorAuditDirectCommandResultRecordRequested", "g8e.v1.operator.audit.direct.command.result.record.requested", "OperatorAuditDirectCommandResultRecorded"),
        ("OperatorAuditMcpCallRecordRequested", "EventOperatorAuditMcpCallRecordRequested", "g8e.v1.operator.audit.mcp.call.record.requested", "OperatorAuditMcpCallRecorded"),
    ]
    out: dict[str, dict] = {
        "AppCaseCreateRequested": {
            "_go_const": "EventAppCaseCreateRequested",
            "value": "g8e.v1.app.case.create.requested",
        },
        "AppCaseDeleteRequested": {
            "_go_const": "EventAppCaseDeleteRequested",
            "value": "g8e.v1.app.case.delete.requested",
        },
        "AppInvestigationCreateRequested": {
            "_go_const": "EventAppInvestigationCreateRequested",
            "value": "g8e.v1.app.investigation.create.requested",
        },
        "AppInvestigationUpdateRequested": {
            "_go_const": "EventAppInvestigationUpdateRequested",
            "value": "g8e.v1.app.investigation.update.requested",
        },
        "AppInvestigationDeleteRequested": {
            "_go_const": "EventAppInvestigationDeleteRequested",
            "value": "g8e.v1.app.investigation.delete.requested",
        },
        "AppMemoryCreateRequested": {
            "_go_const": "EventAppMemoryCreateRequested",
            "value": "g8e.v1.app.memory.create.requested",
        },
        "AppMemoryUpdateRequested": {
            "_go_const": "EventAppMemoryUpdateRequested",
            "value": "g8e.v1.app.memory.update.requested",
        },
        "AppAgentActivityRecordRequested": {
            "_go_const": "EventAppAgentActivityRecordRequested",
            "value": "g8e.v1.app.agent.activity.record.requested",
        },
        "OperatorReputationStateUpdateRequested": {
            "_go_const": "EventOperatorReputationStateUpdateRequested",
            "value": "g8e.v1.operator.reputation.state.update.requested",
        },
        "PlatformAuditChainCheckpointed": {
            "_go_const": "EventPlatformAuditChainCheckpointed",
            "value": "g8e.v1.platform.audit.chain.checkpointed",
        },
    }
    for key, go_const, wire, outcome in audit_pairs:
        out[key] = {
            "_go_const": go_const,
            "value": wire,
            "kind": "request",
            "producers": ["ensemble"],
            "persistence": "operator.audit_log",
            "outcomes": [outcome],
        }
    return out


def strip_governance(entry: dict) -> None:
    for field in ("governance", "transport", "kind", "producers", "persistence", "outcomes"):
        entry.pop(field, None)


def pascal_to_screaming_snake(name: str) -> str:
    s = re.sub(r"(?<=[a-z0-9])(?=[A-Z])", "_", name)
    s = re.sub(r"(?<=[A-Z])(?=[A-Z][a-z])", "_", s)
    return s.upper()


def producer_for_path(path: str) -> str | None:
    normalized = path.replace("\\", "/")
    for prefix, producer in PRODUCER_PATH_PREFIXES:
        if normalized.startswith(prefix):
            return producer
    return None


def should_scan_file(path: Path) -> bool:
    if path.suffix not in SCAN_SUFFIXES:
        return False
    normalized = str(path).replace("\\", "/")
    for skip in SCAN_SKIP_PARTS:
        if skip in normalized:
            return False
    if normalized.endswith("internal/constants/events.go"):
        return False
    return True


def scan_event_producers(events: dict[str, dict]) -> tuple[dict[str, set[str]], dict[str, set[str]]]:
    """Return (producers_by_key, reference_paths_by_key) from a single repo walk."""
    needles: dict[str, dict] = {}
    for key, entry in events.items():
        needles[key] = {
            "wire": entry["value"],
            "go_const": entry["_go_const"],
            "py_enum": f"EventType.{pascal_to_screaming_snake(key)}",
        }

    producers: dict[str, set[str]] = {key: set() for key in events}
    references: dict[str, set[str]] = {key: set() for key in events}

    for path in REPO.rglob("*"):
        if not path.is_file() or not should_scan_file(path):
            continue
        rel = str(path.relative_to(REPO)).replace("\\", "/")
        try:
            text = path.read_text(encoding="utf-8", errors="ignore")
        except OSError:
            continue
        producer = producer_for_path(rel)
        if producer is None:
            continue
        for key, meta in needles.items():
            if meta["wire"] in text or meta["go_const"] in text or meta["py_enum"] in text:
                producers[key].add(producer)
                references[key].add(rel)

    return producers, references


def infer_producers_fallback(key: str, entry: dict) -> list[str]:
    wire = entry["value"]
    kind = entry.get("kind", infer_kind(wire))
    transport = entry.get("transport") or infer_transport(wire, kind) or []

    if kind == "stream" or transport == ["sse"]:
        if wire.startswith("g8e.v1.platform."):
            return ["gateway"]
        return ["ensemble"]

    if "governed" in transport:
        if wire.startswith("g8e.v1.platform.enrollment."):
            return ["cli", "gateway"]
        if wire.startswith("g8e.v1.app."):
            return ["ensemble"]
        return ["ensemble", "cli", "mcp"]

    if "pubsub" in transport:
        if kind == "request":
            if ".audit." in wire:
                return ["ensemble"]
            return ["gateway"]
        return ["operator"]

    if wire.startswith("g8e.v1.operator."):
        if wire.startswith("g8e.v1.operator.status.updated"):
            return ["gateway"]
        if kind == "request":
            return ["gateway"]
        return ["operator"]

    if wire.startswith("g8e.v1.app."):
        return ["ensemble"]
    if wire.startswith("g8e.v1.ai."):
        return ["ensemble"]
    if wire.startswith("g8e.v1.platform."):
        return ["gateway"]
    if wire.startswith("g8e.v1.public."):
        return ["gateway"]
    if wire.startswith("g8e.v1.source."):
        return ["ensemble"]
    return ["gateway"]


def infer_persistence(key: str, entry: dict) -> str:
    wire = entry["value"]
    kind = entry.get("kind", infer_kind(wire))
    transport = entry.get("transport") or infer_transport(wire, kind) or []

    if kind == "stream":
        return "ephemeral"
    if wire == "g8e.v1.platform.audit.chain.checkpointed":
        return "operator.audit_log"
    if ".heartbeat." in wire or wire.endswith(".heartbeat"):
        return "gateway.operator_docs"
    if wire.startswith("g8e.v1.operator.status.updated"):
        return "gateway.operator_docs"
    if ".audit." in wire:
        return "operator.audit_log"
    if wire.startswith("g8e.v1.operator.receipt."):
        return "operator.audit_log"
    if wire.startswith("g8e.v1.public."):
        return "gateway.audit_log"
    if wire.startswith("g8e.v1.platform.enrollment."):
        return "gateway.docstore"
    if wire.startswith("g8e.v1.app.") and kind == "outcome":
        if any(token in wire for token in (".created", ".updated", ".deleted", ".document.")):
            return "gateway.docstore"
    if "sse" in transport:
        return "gateway.sse_store"
    if "governed" in transport and kind == "request" and wire.startswith("g8e.v1.app."):
        return "ephemeral"
    if "governed" in transport and kind == "outcome" and wire.startswith("g8e.v1.app."):
        return "gateway.docstore"
    if "governed" in transport:
        return "ephemeral"
    if "pubsub" in transport and kind == "fact":
        return "operator.audit_log"
    if kind == "outcome" and wire.startswith("g8e.v1.ai."):
        return "gateway.sse_store"
    if kind == "fact" and ".status.updated" in wire:
        return "gateway.sse_store"
    return "ephemeral"


def merge_producers(scanned: set[str], fallback: list[str]) -> list[str]:
    merged = set(fallback)
    merged.update(scanned)
    return sorted(merged, key=lambda p: sorted(ALLOWED_PRODUCERS).index(p))


def enrich() -> None:
    data = json.loads(EVENTS_PATH.read_text())
    events: dict[str, dict] = data["events"]

    events.pop("AppCaseCreationRequested", None)

    for key, entry in new_entries().items():
        events[key] = entry

    # Restore governed operator requests from mappings.go (wire -> action).
    governed_operator = {
        "g8e.v1.operator.eval.answer.requested": ("EVAL_ANSWER", "g8e.operator.v1.EvalAnswerRequested"),
        "g8e.v1.operator.heartbeat.requested": ("HEARTBEAT", "g8e.operator.v1.HeartbeatRequested"),
        "g8e.v1.operator.shutdown.requested": ("SHUTDOWN", "g8e.operator.v1.ShutdownRequested"),
        "g8e.v1.operator.command.requested": ("EXECUTE_BASH", "g8e.operator.v1.CommandRequested"),
        "g8e.v1.operator.command.cancel.requested": ("CANCEL", "g8e.operator.v1.CommandCancelRequested"),
        "g8e.v1.operator.file.edit.requested": ("FILE_EDIT", "g8e.operator.v1.FileEditRequested"),
        "g8e.v1.operator.fetch.file.history.requested": ("FETCH_FILE_HISTORY", "g8e.operator.v1.FetchFileHistoryRequested"),
        "g8e.v1.operator.restore.file.requested": ("RESTORE_FILE", "g8e.operator.v1.RestoreFileRequested"),
        "g8e.v1.operator.fs.list.requested": ("FS_LIST", "g8e.operator.v1.FsListRequested"),
        "g8e.v1.operator.fs.read.requested": ("FS_READ", "g8e.operator.v1.FsReadRequested"),
        "g8e.v1.operator.fs.grep.requested": ("FS_GREP", "g8e.operator.v1.FsGrepRequested"),
        "g8e.v1.operator.fetch.logs.requested": ("FETCH_LOGS", "g8e.operator.v1.FetchLogsRequested"),
        "g8e.v1.operator.fetch.history.requested": ("FETCH_HISTORY", "g8e.operator.v1.FetchHistoryRequested"),
        "g8e.v1.operator.mcp.call.requested": ("MCP_CALL", "g8e.operator.v1.McpCallRequested"),
        "g8e.v1.operator.a2a.call.requested": ("A2A_CALL", "g8e.operator.v1.A2ACallRequested"),
        "g8e.v1.operator.network.port.check.requested": ("PORT_CHECK", "g8e.operator.v1.CheckPortRequested"),
        "g8e.v1.operator.ollama.model.inventory.requested": ("OLLAMA_MODEL_INVENTORY", "g8e.operator.v1.OllamaModelInventoryRequested"),
        "g8e.v1.operator.ollama.model.residency.requested": ("OLLAMA_MODEL_RESIDENCY", "g8e.operator.v1.OllamaModelResidencyRequested"),
        "g8e.v1.operator.inference.requested": ("INFERENCE", "g8e.operator.v1.InferenceRequested"),
        "g8e.v1.operator.provider.boundary.observation.requested": ("PROVIDER_BOUNDARY_OBSERVATION", "g8e.eval.v1.ProviderBoundaryObservationCommand"),
        "g8e.v1.operator.model.provenance.observation.requested": ("MODEL_PROVENANCE_OBSERVATION", "g8e.eval.v1.ModelProvenanceObservationCommand"),
        "g8e.v1.platform.enrollment.create.requested": ("PLATFORM_ENROLLMENT_CREATE", "g8e.common.v1.PlatformEnrollmentGovernancePayload"),
        "g8e.v1.platform.enrollment.decide.requested": ("PLATFORM_ENROLLMENT_DECIDE", "g8e.common.v1.PlatformEnrollmentGovernancePayload"),
        "g8e.v1.platform.enrollment.issue.requested": ("PLATFORM_ENROLLMENT_ISSUE", "g8e.common.v1.PlatformEnrollmentGovernancePayload"),
        "g8e.v1.platform.enrollment.persist_policy.requested": ("PLATFORM_ENROLLMENT_PERSIST_POLICY", "g8e.common.v1.PlatformEnrollmentGovernancePayload"),
        "g8e.v1.platform.enrollment.create_session.requested": ("PLATFORM_ENROLLMENT_CREATE_SESSION", "g8e.common.v1.PlatformEnrollmentGovernancePayload"),
        "g8e.v1.platform.enrollment.revoke.requested": ("PLATFORM_ENROLLMENT_REVOKE", "g8e.common.v1.PlatformEnrollmentGovernancePayload"),
    }

    for key, entry in list(events.items()):
        wire = entry["value"]
        if "governance" in entry and not wire.endswith(".requested"):
            strip_governance(entry)

        kind = infer_kind(wire)
        entry["kind"] = kind

        gov = infer_governance_key(wire)
        if gov is None and wire in governed_operator:
            action, payload = governed_operator[wire]
            gov = {"action_type": action, "payload": payload}
        if gov is not None:
            entry["kind"] = "request"
            entry["governance"] = gov
            entry["transport"] = ["governed"]
        elif key in GRAMMAR_ALLOWLIST:
            entry["grammar_allowlist_owner"] = GRAMMAR_ALLOWLIST[key]
        else:
            transport = infer_transport(wire, kind)
            if transport:
                entry["transport"] = transport

        if key == "PlatformAuditChainCheckpointed":
            entry["kind"] = "fact"
            entry["producers"] = ["gateway", "operator"]
            entry["persistence"] = "operator.audit_log"

        for audit_key, meta in new_entries().items():
            if key == audit_key and "outcomes" in meta:
                entry.setdefault("outcomes", meta["outcomes"])

    scanned_producers, references = scan_event_producers(events)
    reserved_count = 0
    for key, entry in events.items():
        entry.pop("reserved", None)
        fallback = infer_producers_fallback(key, entry)
        entry["producers"] = merge_producers(scanned_producers.get(key, set()), fallback)
        entry["persistence"] = infer_persistence(key, entry)

        if not references.get(key):
            entry["reserved"] = True
            reserved_count += 1

        unknown = set(entry["producers"]) - ALLOWED_PRODUCERS
        if unknown:
            raise ValueError(f"{key}: unknown producers {sorted(unknown)}")
        if entry["persistence"] not in ALLOWED_PERSISTENCE:
            raise ValueError(f"{key}: unknown persistence {entry['persistence']}")

    data["events"] = dict(sorted(events.items()))
    EVENTS_PATH.write_text(json.dumps(data, indent=2) + "\n")
    BUNDLED_PATH.write_text(json.dumps(data, indent=2) + "\n")
    print(
        f"enriched {len(events)} registry entries "
        f"({reserved_count} reserved, {len(events) - reserved_count} referenced)"
    )


if __name__ == "__main__":
    enrich()
