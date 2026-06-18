#!/bin/sh
# Inspect a specific doctrine rule from the PHI/HIPAA doctrine file.
# Usage: sh inspect_doctrine_rule.sh <rule_id>

if [ $# -ne 1 ]; then
    echo "Usage: sh inspect_doctrine_rule.sh <rule_id>" >&2
    exit 1
fi

RULE_ID="$1"
DOCTRINE_FILE="/etc/g8e/doctrine/phi_hipaa_doctrine.json"

if [ ! -f "$DOCTRINE_FILE" ]; then
    echo "Error: $DOCTRINE_FILE not found" >&2
    exit 1
fi

# Use grep to find the rule and extract relevant fields
# This is a simple approach that works without jq or python
grep -A 10 "\"id\": \"$RULE_ID\"" "$DOCTRINE_FILE" | head -15 | \
    grep -E '"id"|"severity"|"confidence"|"pattern"' | \
    sed 's/^[[:space:]]*/  /' | \
    sed 's/,$//'

if [ $? -ne 0 ]; then
    echo "Doctrine rule $RULE_ID not found" >&2
    exit 1
fi

# Made with Bob
