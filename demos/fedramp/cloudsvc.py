#!/usr/bin/env python3
"""FedRAMP Sovereign Cloud Service (L5 actuator).

A minimal HTTP server that records governed cloud resource operations
(PROVISION, CONFIGURE, DESTROY, REVERT) to a JSONL audit log.
All operations are recorded with timestamps and caller metadata.
"""

import json
import os
import time
from http.server import BaseHTTPRequestHandler, HTTPServer

LOG_PATH = os.environ.get("CLOUDSVC_LOG", "/var/cloudsvc/operations.jsonl")
os.makedirs(os.path.dirname(LOG_PATH), exist_ok=True)

# Suppress all logging to stdout/stderr for clean demo output
import logging
logging.disable(logging.CRITICAL)


class CloudSvcHandler(BaseHTTPRequestHandler):
    def _send_json(self, code, body):
        self.send_response(code)
        self.send_header("Content-Type", "application/json")
        self.end_headers()
        self.wfile.write(json.dumps(body).encode())

    def _read_body(self):
        length = int(self.headers.get("Content-Length", 0))
        if length == 0:
            return {}
        raw = self.rfile.read(length)
        try:
            return json.loads(raw)
        except json.JSONDecodeError:
            return {}

    def _record(self, action, resource_id, detail, caller="governed-operator"):
        entry = {
            "timestamp": time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime()),
            "action": action,
            "resource_id": resource_id,
            "detail": detail,
            "caller": caller,
        }
        with open(LOG_PATH, "a") as f:
            f.write(json.dumps(entry) + "\n")
        return entry

    def do_POST(self):
        body = self._read_body()
        action = body.get("action", "")
        resource_id = body.get("resource_id", "")
        detail = body.get("detail", "")
        caller = body.get("caller", "governed-operator")

        if self.path == "/provision":
            entry = self._record("PROVISION", resource_id, detail, caller)
            self._send_json(200, {"status": "provisioned", "entry": entry})
        elif self.path == "/configure":
            entry = self._record("CONFIGURE", resource_id, detail, caller)
            self._send_json(200, {"status": "configured", "entry": entry})
        elif self.path == "/destroy":
            entry = self._record("DESTROY", resource_id, detail, caller)
            self._send_json(200, {"status": "destroyed", "entry": entry})
        elif self.path == "/revert":
            entry = self._record("REVERT", resource_id, detail, caller)
            self._send_json(200, {"status": "reverted", "entry": entry})
        else:
            self._send_json(404, {"error": "unknown endpoint"})

    def do_GET(self):
        if self.path == "/operations":
            ops = []
            if os.path.exists(LOG_PATH):
                with open(LOG_PATH) as f:
                    for line in f:
                        line = line.strip()
                        if line:
                            ops.append(json.loads(line))
            self._send_json(200, {"operations": ops})
        elif self.path == "/health":
            self._send_json(200, {"status": "ok"})
        else:
            self._send_json(404, {"error": "unknown endpoint"})

    def log_message(self, *args, **kwargs):
        pass


if __name__ == "__main__":
    server = HTTPServer(("0.0.0.0", 9100), CloudSvcHandler)
    server.serve_forever()
