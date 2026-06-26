#!/usr/bin/env python3
"""
Mock gimbal controller for the DoW tactical edge demo.

This is a MOCK EXTERNAL — it stands in for the physical camera gimbal.
The g8e governance path (envelope -> admission -> consensus -> L5 execution
-> receipt) is fully real; only this endpoint is simulated.

Exposes:
  POST /slew   — records a slew command {az, el} to /var/gimbal/slews.jsonl
  GET  /slews  — returns the JSON array of recorded slews
  GET  /health — simple health check
"""

import json
import os
from datetime import datetime, timezone
from http.server import HTTPServer, BaseHTTPRequestHandler

SLEWS_FILE = os.environ.get("GIMBAL_SLEWS_FILE", "/var/gimbal/slews.jsonl")
PORT = int(os.environ.get("GIMBAL_PORT", "9000"))


class GimbalHandler(BaseHTTPRequestHandler):
    def _send_json(self, code, payload):
        body = json.dumps(payload).encode()
        self.send_response(code)
        self.send_header("Content-Type", "application/json")
        self.send_header("Content-Length", str(len(body)))
        self.end_headers()
        self.wfile.write(body)

    def do_POST(self):
        if self.path == "/slew":
            length = int(self.headers.get("Content-Length", 0))
            data = self.rfile.read(length) if length > 0 else b"{}"
            try:
                req = json.loads(data) if data else {}
            except json.JSONDecodeError:
                self._send_json(400, {"error": "invalid JSON"})
                return

            az = float(req.get("az", 0))
            el = float(req.get("el", 0))
            ts = datetime.now(timezone.utc).isoformat().replace("+00:00", "Z")

            entry = {"az": az, "el": el, "timestamp": ts}
            os.makedirs(os.path.dirname(SLEWS_FILE), exist_ok=True)
            with open(SLEWS_FILE, "a") as f:
                f.write(json.dumps(entry) + "\n")

            print(f"[GIMBAL] Slew recorded: az={az}, el={el} at {ts}")
            self._send_json(200, {"status": "ok", "slew": entry})
        else:
            self._send_json(404, {"error": "not found"})

    def do_GET(self):
        if self.path == "/slews":
            slews = []
            if os.path.exists(SLEWS_FILE):
                with open(SLEWS_FILE) as f:
                    for line in f:
                        line = line.strip()
                        if line:
                            try:
                                slews.append(json.loads(line))
                            except json.JSONDecodeError:
                                pass
            self._send_json(200, {"slews": slews})
        elif self.path == "/health":
            self._send_json(200, {"status": "ok"})
        else:
            self._send_json(404, {"error": "not found"})

    def log_message(self, fmt, *args):
        pass


if __name__ == "__main__":
    os.makedirs(os.path.dirname(SLEWS_FILE), exist_ok=True)
    print(f"[GIMBAL] Mock gimbal controller listening on :{PORT}")
    print(f"[GIMBAL] Recording slews to {SLEWS_FILE}")
    HTTPServer(("0.0.0.0", PORT), GimbalHandler).serve_forever()
