# History Collection Migration Design

## Problem Statement

The current operator and investigation models store audit trails as unbounded arrays (`history_trail`, `conversation_history`) embedded within the main documents. This causes:

- **O(n) write amplification**: Every history append requires reading and rewriting the entire document
- **Document bloat**: As history grows, documents exceed size limits and impact query performance
- **Memory pressure**: Large documents consume more memory during reads/writes
- **Concurrency contention**: The KeyedAsyncLock helps serialize writes, but the underlying issue remains

## Current State

### OperatorDocument
```python
history_trail: list[OperatorHistoryEntry]  # Unbounded array
conversation_history: list[ConversationHistoryMessage]  # Also unbounded
heartbeat_history: list[OperatorHeartbeat]  # Capped at MAX_HEARTBEAT_HISTORY
command_results_history: list[CommandResultRecord]  # Capped at MAX_COMMAND_RESULTS_HISTORY
```

### InvestigationModel
```python
history_trail: list[InvestigationHistoryEntry]  # Unbounded array
conversation_history: list[ConversationHistoryMessage]  # Unbounded array
```

## Design Goals

1. **Separate collections**: Move history to dedicated collections with proper indexing
2. **Maintain hash chain integrity**: Ensure cryptographic chain remains intact during migration
3. **Backward compatibility**: Support reading from both old and new formats during transition
4. **Performance**: O(1) appends, efficient queries by time range/entity
5. **Atomicity**: Ensure history writes remain atomic per-entry

## Proposed Schema

### New Collections

#### `operator_history_entries`
```python
{
    "id": str,  # Unique entry ID
    "operator_id": str,  # Indexed
    "event_type": OperatorHistoryEventType,
    "summary": str,
    "actor": ComponentName,
    "details": dict,
    "timestamp": UTCDatetime,  # Indexed for time-range queries
    "prev_hash": str,  # For chain integrity
    "entry_hash": str,  # Computed hash
    "created_at": UTCDatetime
}
```

#### `investigation_history_entries`
```python
{
    "id": str,
    "investigation_id": str,  # Indexed
    "event_type": EventType,
    "summary": str,
    "actor": ComponentName,
    "details": dict,
    "timestamp": UTCDatetime,  # Indexed
    "prev_hash": str,
    "entry_hash": str,
    "created_at": UTCDatetime
}
```

#### `conversation_messages`
```python
{
    "id": str,
    "entity_id": str,  # operator_id or investigation_id
    "entity_type": str,  # "operator" or "investigation"
    "sender": str,
    "content": str,
    "metadata": dict,
    "timestamp": UTCDatetime,  # Indexed
    "prev_hash": str,
    "entry_hash": str,
    "created_at": UTCDatetime
}
```

## Migration Strategy

### Phase 1: Dual-Write (Non-Breaking)
- Add new collections
- Update all write paths to write to BOTH embedded arrays AND new collections
- Add feature flag to control new collection reads
- Verify data consistency between old and new formats

### Phase 2: Read Migration
- Add read methods that query new collections
- Update read paths to use new collections when feature flag enabled
- Add fallback to embedded arrays for missing data
- Monitor performance and correctness

### Phase 3: Data Backfill
- One-time migration job to copy existing history from embedded arrays to new collections
- Verify hash chain integrity after migration
- Validate record counts match

### Phase 4: Cleanup
- Remove embedded arrays from models (or mark as deprecated)
- Remove dual-write logic
- Remove feature flags
- Update documentation

## Implementation Plan

### Step 1: Create New Models and Collections
- [ ] Create `OperatorHistoryEntryDocument` model
- [ ] Create `InvestigationHistoryEntryDocument` model
- [ ] Create `ConversationMessageDocument` model
- [ ] Add collection constants
- [ ] Create data service methods for new collections

### Step 2: Dual-Write Implementation
- [ ] Update `OperatorDataService.add_history_entry` to write to both
- [ ] Update `InvestigationDataService.add_history_entry` to write to both
- [ ] Update `InvestigationDataService.add_chat_message` to write to both
- [ ] Add feature flag `USE_HISTORY_COLLECTIONS`

### Step 3: Read Implementation
- [ ] Create `get_operator_history` method that queries new collection
- [ ] Create `get_investigation_history` method that queries new collection
- [ ] Create `get_conversation_history` method that queries new collection
- [ ] Update model methods to use new collections when flag enabled
- [ ] Add fallback logic for backward compatibility

### Step 4: Migration Job
- [ ] Create migration script for operator history
- [ ] Create migration script for investigation history
- [ ] Create migration script for conversation history
- [ ] Add hash chain verification after migration
- [ ] Add rollback capability

### Step 5: Testing
- [ ] Unit tests for dual-write logic
- [ ] Unit tests for read methods with feature flag
- [ ] Integration tests for migration job
- [ ] Performance benchmarks (append/read operations)
- [ ] Hash chain integrity tests

### Step 6: Deployment
- [ ] Deploy dual-write code (no user impact)
- [ ] Run migration job during maintenance window
- [ ] Enable feature flag for reads (monitor)
- [ ] Remove dual-write and embedded arrays (final cleanup)

## Index Requirements

```python
# operator_history_entries
- operator_id (ascending)
- timestamp (descending for recent queries)
- (operator_id, timestamp) composite

# investigation_history_entries
- investigation_id (ascending)
- timestamp (descending)
- (investigation_id, timestamp) composite

# conversation_messages
- entity_id (ascending)
- entity_type (ascending)
- timestamp (descending)
- (entity_id, entity_type, timestamp) composite
```

## Hash Chain Considerations

The hash chain relies on `prev_hash` pointing to the previous entry. During migration:

1. **Preserve ordering**: Migrate entries in timestamp order
2. **Verify hashes**: After migration, re-verify chain integrity
3. **Handle gaps**: If embedded array is corrupted, log and continue
4. **Atomic migration**: Use transactions where supported

## Performance Expectations

- **Append operation**: O(1) instead of O(n) document rewrite
- **Recent history query**: O(log n) via index instead of full document read
- **Full history query**: Similar performance (still need to fetch all entries)
- **Document size**: Reduced by ~80% for long-lived entities

## Rollback Plan

If issues arise:
1. Disable feature flag to fall back to embedded arrays
2. New collections remain but unused
3. No data loss (embedded arrays still present)
4. Can retry migration after fix

## Open Questions

1. Should we keep capped arrays (heartbeat, command_results) in documents?
   - **Recommendation**: Yes, these are intentionally bounded and useful for quick access
2. Should conversation history be unified across operators and investigations?
   - **Recommendation**: Keep separate for now, unify later if pattern emerges
3. How to handle very old history (> 1 year)?
   - **Recommendation**: Add TTL or archival policy after migration is stable
