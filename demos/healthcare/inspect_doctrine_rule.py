#!/usr/bin/env python3
"""
Inspect a specific doctrine rule from the PHI/HIPAA doctrine file.
Usage: python3 inspect_doctrine_rule.py <rule_id>
"""
import json
import sys

def main():
    if len(sys.argv) != 2:
        print("Usage: python3 inspect_doctrine_rule.py <rule_id>", file=sys.stderr)
        sys.exit(1)
    
    rule_id = sys.argv[1]
    
    try:
        with open('/etc/g8e/doctrine/phi_hipaa_doctrine.json', 'r') as f:
            data = json.load(f)
        
        # Find the doctrine rule
        for doctrine in data.get('doctrines', []):
            if doctrine.get('id') == rule_id:
                print(f"  id:         {doctrine.get('id', 'N/A')}")
                print(f"  severity:   {doctrine.get('severity', 'N/A')}")
                print(f"  confidence: {doctrine.get('confidence', 'N/A')}")
                print(f"  pattern:    {doctrine.get('pattern', 'N/A')}")
                sys.exit(0)
        
        print(f"Doctrine rule {rule_id} not found", file=sys.stderr)
        sys.exit(1)
    
    except FileNotFoundError:
        print("Error: /etc/g8e/doctrine/phi_hipaa_doctrine.json not found", file=sys.stderr)
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
