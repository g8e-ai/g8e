#!/usr/bin/env python3
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
Minimal FHIR PA API server for healthcare demo.
Accepts POST requests to /fhir/ClaimResponse endpoint.
"""
from http.server import HTTPServer, BaseHTTPRequestHandler
import json
from urllib.parse import urlparse, parse_qs

class FHIRHandler(BaseHTTPRequestHandler):
    def do_POST(self):
        parsed_path = urlparse(self.path)
        content_length = int(self.headers.get('Content-Length', 0))
        post_data = self.rfile.read(content_length)
        
        # Handle MCP JSON-RPC requests (Production Governance Path)
        try:
            rpc_data = json.loads(post_data.decode('utf-8'))
            if rpc_data.get('jsonrpc') == '2.0':
                method = rpc_data.get('method')
                if method == 'tools/list':
                    response = {
                        "jsonrpc": "2.0",
                        "id": rpc_data.get('id'),
                        "result": {
                            "tools": [
                                {
                                    "name": "submit_pa",
                                    "description": "Submit a FHIR R4 Prior Authorization request",
                                    "inputSchema": {
                                        "type": "object",
                                        "properties": {
                                            "resourceType": {"type": "string", "enum": ["ClaimResponse"]},
                                            "status": {"type": "string"},
                                            "use": {"type": "string"}
                                        },
                                        "required": ["resourceType", "status", "use"]
                                    }
                                }
                            ]
                        }
                    }
                    self.send_response(200)
                    self.send_header('Content-Type', 'application/json')
                    self.end_headers()
                    self.wfile.write(json.dumps(response).encode('utf-8'))
                    return
                elif method == 'tools/call':
                    params = rpc_data.get('params', {})
                    if params.get('name') == 'submit_pa':
                        # Forward to FHIR logic
                        fhir_data = params.get('arguments', {})
                        print(f"Received Governed FHIR Submission via MCP: {fhir_data}")
                        response = {
                            "jsonrpc": "2.0",
                            "id": rpc_data.get('id'),
                            "result": {
                                "content": [
                                    {
                                        "type": "text",
                                        "text": "PA-2026-0045: ClaimResponse received and queued via governed MCP endpoint."
                                    }
                                ]
                            }
                        }
                        self.send_response(200)
                        self.send_header('Content-Type', 'application/json')
                        self.end_headers()
                        self.wfile.write(json.dumps(response).encode('utf-8'))
                        return
        except:
            pass

        # Handle direct FHIR ClaimResponse endpoint (Fallback/Legacy Path)
        if parsed_path.path == '/fhir/ClaimResponse' or parsed_path.path == '/':
            try:
                data = json.loads(post_data.decode('utf-8'))
                print(f"Received Direct FHIR ClaimResponse: {data}")
                
                # Return success response
                response = {
                    "resourceType": "ClaimResponse",
                    "status": "active",
                    "use": "preauthorization",
                    "outcome": "queued",
                    "id": "PA-2026-0045"
                }
                
                self.send_response(200)
                self.send_header('Content-Type', 'application/fhir+json')
                self.end_headers()
                self.wfile.write(json.dumps(response).encode('utf-8'))
            except Exception as e:
                self.send_response(400)
                self.send_header('Content-Type', 'text/plain')
                self.end_headers()
                self.wfile.write(f"Error: {str(e)}".encode('utf-8'))
        else:
            self.send_response(404)
            self.send_header('Content-Type', 'text/plain')
            self.end_headers()
            self.wfile.write(b"Not Found")
    
    def do_GET(self):
        # Simple health check
        if self.path == '/health':
            self.send_response(200)
            self.send_header('Content-Type', 'application/json')
            self.end_headers()
            self.wfile.write(json.dumps({"status": "ok"}).encode('utf-8'))
        else:
            self.send_response(404)
            self.send_header('Content-Type', 'text/plain')
            self.end_headers()
            self.wfile.write(b"Not Found")
    
    def log_message(self, format, *args):
        # Suppress default logging
        pass

if __name__ == '__main__':
    server = HTTPServer(('0.0.0.0', 8000), FHIRHandler)
    print("FHIR PA API server listening on port 8000")
    server.serve_forever()
