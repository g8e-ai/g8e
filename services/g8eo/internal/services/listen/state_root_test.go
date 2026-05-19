package listen

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStateRootSemantics(t *testing.T) {
	db := newTestDB(t)

	// 1. Initial state root must be deterministic
	root1, err := db.GetCurrentStateRoot()
	require.NoError(t, err)
	require.NotEmpty(t, root1)

	root1Again, err := db.GetCurrentStateRoot()
	require.NoError(t, err)
	assert.Equal(t, root1, root1Again, "State root must be deterministic for identical state")

	// 2. Document content change alters root
	err = db.DocSet("test", "d1", json.RawMessage(`{"val":1}`))
	require.NoError(t, err)
	root2, err := db.GetCurrentStateRoot()
	require.NoError(t, err)
	assert.NotEqual(t, root1, root2, "Content change must alter state root")

	// 3. Document metadata change (updated_at) does NOT alter root
	time.Sleep(2 * time.Millisecond) // Ensure updated_at is different
	err = db.DocSet("test", "d1", json.RawMessage(`{"val":1}`))
	require.NoError(t, err)
	root3, err := db.GetCurrentStateRoot()
	require.NoError(t, err)
	assert.Equal(t, root2, root3, "Metadata-only change (updated_at) must NOT alter state root")

	// 4. KV change alters root
	err = db.KVSet("k1", "v1", 0)
	require.NoError(t, err)
	root4, err := db.GetCurrentStateRoot()
	require.NoError(t, err)
	assert.NotEqual(t, root3, root4, "KV change must alter state root")

	// 5. Blob change alters root
	err = db.BlobPut("ns", "b1", []byte("data"), "text/plain", 0)
	require.NoError(t, err)
	root5, err := db.GetCurrentStateRoot()
	require.NoError(t, err)
	assert.NotEqual(t, root4, root5, "Blob change must alter state root")

	// 6. Nonce insert does NOT alter root
	replayed, err := db.CheckAndSetNonce("nonce1", time.Now().Add(time.Hour))
	require.NoError(t, err)
	assert.False(t, replayed)
	root6, err := db.GetCurrentStateRoot()
	require.NoError(t, err)
	assert.Equal(t, root5, root6, "Nonce insert must NOT alter state root")

	// 7. SSE event insert does NOT alter root
	err = db.SSEEventsAppend(SSERoute{WebSessionID: "session1"}, "type1", "payload1")
	require.NoError(t, err)
	root7, err := db.GetCurrentStateRoot()
	require.NoError(t, err)
	assert.Equal(t, root6, root7, "SSE event insert must NOT alter state root")

	// 8. Expired KV is excluded from root
	err = db.KVSet("exp1", "val", 1) // 1 second TTL
	require.NoError(t, err)
	rootWithExp, err := db.GetCurrentStateRoot()
	require.NoError(t, err)
	assert.NotEqual(t, root7, rootWithExp)

	time.Sleep(1100 * time.Millisecond)
	rootAfterExp, err := db.GetCurrentStateRoot()
	require.NoError(t, err)
	assert.Equal(t, root7, rootAfterExp, "Expired KV must be excluded from state root calculation")
}

func TestStateRootDeterministicOrder(t *testing.T) {
	db1 := newTestDB(t)
	db2 := newTestDB(t)

	// Wipe initial random platform settings to have a clean slate for order comparison
	_, err := db1.db.Exec("DELETE FROM documents")
	require.NoError(t, err)
	_, err = db1.db.Exec("DELETE FROM kv_store")
	require.NoError(t, err)

	_, err = db2.db.Exec("DELETE FROM documents")
	require.NoError(t, err)
	_, err = db2.db.Exec("DELETE FROM kv_store")
	require.NoError(t, err)

	// Insert in one order into db1
	require.NoError(t, db1.DocSet("test", "a", json.RawMessage(`{"v":1}`)))
	require.NoError(t, db1.DocSet("test", "b", json.RawMessage(`{"v":2}`)))
	require.NoError(t, db1.KVSet("k1", "v1", 0))
	require.NoError(t, db1.KVSet("k2", "v2", 0))
	root1, err := db1.GetCurrentStateRoot()
	require.NoError(t, err)

	// Insert in different order into db2
	require.NoError(t, db2.KVSet("k2", "v2", 0))
	require.NoError(t, db2.DocSet("test", "b", json.RawMessage(`{"v":2}`)))
	require.NoError(t, db2.KVSet("k1", "v1", 0))
	require.NoError(t, db2.DocSet("test", "a", json.RawMessage(`{"v":1}`)))
	root2, err := db2.GetCurrentStateRoot()
	require.NoError(t, err)

	assert.Equal(t, root1, root2, "State root must be deterministic regardless of insertion order")
}
