# Shared Test Fixtures

This directory contains test fixtures shared across all g8e components to ensure consistency and prevent drift.

## SSE Events (`sse-events.json`)

Canonical SSE event structures used by both VSE (Python) and VSOD (Node.js) tests.

### Usage in VSE (Python)

```python
import json
from pathlib import Path

# Load shared fixtures
fixtures_path = Path(__file__).parent.parent / "shared" / "test-fixtures" / "sse-events.json"
with open(fixtures_path) as f:
    sse_events = json.load(f)

# Use in tests
text_chunk_event = sse_events["text_chunk_received"]
```

### Usage in VSOD (Node.js)

```javascript
import fs from 'fs';
import path from 'path';

// Load shared fixtures
const fixturesPath = path.resolve(__dirname, '../../../shared/test-fixtures/sse-events.json');
const sseEvents = JSON.parse(fs.readFileSync(fixturesPath, 'utf8'));

// Use in tests
const textChunkEvent = sseEvents.text_chunk_received;
```

## Fixture Structure

Each fixture includes:
- `type`: Event type constant
- `data`: Event payload with required routing fields (`investigation_id`, `case_id`, `web_session_id`)
- Realistic example values for testing

## Contract Tests

Both VSE and VSOD should include contract tests that verify:
1. Events emitted match the shared fixture structure
2. Required routing fields are present
3. Event types match constants in `shared/constants/events.json`

## Maintenance

When adding new SSE events:
1. Add the event structure to `sse-events.json`
2. Update any relevant contract tests
3. Document the event in component-specific testing guides
