#!/usr/bin/env python3
"""
Inspect a specific PA request from the seed data.
Usage: python3 inspect_pa_request.py <request_id>
"""
import json
import sys

def main():
    if len(sys.argv) != 2:
        print("Usage: python3 inspect_pa_request.py <request_id>", file=sys.stderr)
        sys.exit(1)
    
    request_id = sys.argv[1]
    
    try:
        with open('/var/g8e/target/pa_requests.json', 'r') as f:
            data = json.load(f)
        
        # Find the request in pa_queue
        for request in data.get('pa_queue', []):
            if request.get('id') == request_id:
                print(json.dumps(request, indent=2))
                sys.exit(0)
        
        print(f"Request {request_id} not found in pa_queue", file=sys.stderr)
        sys.exit(1)
    
    except FileNotFoundError:
        print("Error: /var/g8e/target/pa_requests.json not found", file=sys.stderr)
        sys.exit(1)
    except json.JSONDecodeError as e:
        print(f"Error parsing JSON: {e}", file=sys.stderr)
        sys.exit(1)
    except Exception as e:
        print(f"Error: {e}", file=sys.stderr)
        sys.exit(1)

if __name__ == '__main__':
    main()

# Made with Bob
