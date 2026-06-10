#!/usr/bin/env python3
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
        
        # Handle FHIR ClaimResponse endpoint
        if parsed_path.path == '/fhir/ClaimResponse' or parsed_path.path == '/':
            content_length = int(self.headers.get('Content-Length', 0))
            post_data = self.rfile.read(content_length)
            
            try:
                data = json.loads(post_data.decode('utf-8'))
                print(f"Received FHIR ClaimResponse: {data}")
                
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
