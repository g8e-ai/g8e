#!/usr/bin/env python3
"""
Sovereign Data Service — the L5 actuator for the DHS demo.

This is a MOCK EXTERNAL: it stands in for the sovereign data store / common
operating picture write-path that a partner fusion product would own. The g8e
governance path (envelope -> admission -> L1 doctrine -> L2 consensus -> L3
notary -> L5 execution -> signed receipt) is FULLY REAL; only this endpoint,
which records the resulting data operation, is simulated.

The operator reaches this service only after an envelope clears governance, so
the operations log here is, by construction, the set of data actions that the
sovereign data plane actually authorized.

Endpoints:
  POST /ingest    — record a governed multi-source ingest      {record_id, detail}
  POST /release   — record a governed cross-domain release      {record_id, detail}
  POST /cue       — record a governed interdiction cue          {record_id, detail}
  POST /purge     — record a governed retention destruction     {record_id, detail}
  GET  /operations — return the JSON array of recorded operations
  GET  /health    — health check
"""

import json
import os
from datetime import datetime, timezone
from http.server import HTTPServer, BaseHTTPRequestHandler

OPS_FILE = os.environ.get("DATASVC_OPS_FILE", "/var/vault/operations.jsonl")
PORT = int(os.environ.get("DATASVC_PORT", "9100"))

# Operation path -> human-readable action recorded in the sovereign log.
OPS = {
    "/ingest": "INGEST",
    "/release": "RELEASE",
    "/cue": "CUE",
    "/purge": "PURGE",
}


class DataSvcHandler(BaseHTTPRequestHandler):
    def _send_json(self, code, payload):
        body = json.dumps(payload).encode()
        self.send_response(code)
        self.send_header("Content-Type", "application/json")
        self.send_header("Content-Length", str(len(body)))
        self.end_headers()
        self.wfile.write(body)

    def do_POST(self):
        action = OPS.get(self.path)
        if action is None:
            self._send_json(404, {"error": "not found"})
            return

        length = int(self.headers.get("Content-Length", 0))
        data = self.rfile.read(length) if length > 0 else b"{}"
        try:
            req = json.loads(data) if data else {}
        except json.JSONDecodeError:
            self._send_json(400, {"error": "invalid JSON"})
            return

        ts = datetime.now(timezone.utc).isoformat().replace("+00:00", "Z")
        entry = {
            "action": action,
            "record_id": req.get("record_id", ""),
            "detail": req.get("detail", ""),
            "timestamp": ts,
        }
        os.makedirs(os.path.dirname(OPS_FILE), exist_ok=True)
        with open(OPS_FILE, "a") as f:
            f.write(json.dumps(entry) + "\n")

        print(f"[DATASVC] {action} recorded: record={entry['record_id']} "
              f"detail={entry['detail']} at {ts}")
        self._send_json(200, {"status": "ok", "operation": entry})

    def do_GET(self):
        if self.path == "/operations":
            ops = []
            if os.path.exists(OPS_FILE):
                with open(OPS_FILE) as f:
                    for line in f:
                        line = line.strip()
                        if line:
                            try:
                                ops.append(json.loads(line))
                            except json.JSONDecodeError:
                                pass
            self._send_json(200, {"operations": ops})
        elif self.path == "/health":
            self._send_json(200, {"status": "ok"})
        else:
            self._send_json(404, {"error": "not found"})

    def log_message(self, fmt, *args):
        pass


if __name__ == "__main__":
    os.makedirs(os.path.dirname(OPS_FILE), exist_ok=True)
    print(f"[DATASVC] Sovereign Data Service listening on :{PORT}")
    print(f"[DATASVC] Recording governed operations to {OPS_FILE}")
    HTTPServer(("0.0.0.0", PORT), DataSvcHandler).serve_forever()
