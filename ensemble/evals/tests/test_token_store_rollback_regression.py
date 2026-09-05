# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""Tier 2 integration regression tests for token-store persistence rollback.

These tests verify the P1-2 fix for ``LocalEncryptedTokenStore``: when a
persistence failure occurs (either via the ``fail_persist`` flag or a real
filesystem write failure through the injected ``writer`` callable), the
in-memory state is restored to the last successfully committed snapshot.
Pre-existing committed tokens survive the failure while uncommitted tokens
added after the last successful persist are rolled back.

The tests use a real local encrypted token store on disk (AES-256-GCM at
rest) with ``tmp_path`` isolation.  They prove state equality before and
after the failed mutation rather than merely asserting the reported
``rolled_back`` flag.
"""

from __future__ import annotations

from pathlib import Path

import pytest

from g8e_evals.benchmarks.privacy.token_store import (
    LocalEncryptedTokenStore,
    StorageError,
)


pytestmark = pytest.mark.integration

_KEY = b"r" * 32


def test_pre_existing_committed_tokens_survive_persistence_failure(
    tmp_path: Path,
) -> None:
    """Pre-existing committed tokens survive when a subsequent persist fails.

    The store is populated with two tokens, persisted successfully
    (establishing the committed snapshot), then a third uncommitted token
    is added and ``fail_persist`` is toggled on.  The failing persist
    restores the committed snapshot: the two pre-existing tokens survive
    and the uncommitted token is rolled back.
    """
    store_path = tmp_path / "store.json"
    store = LocalEncryptedTokenStore(store_path, _KEY)

    store.store("pre-existing-1", "value-1", "email", 300)
    store.store("pre-existing-2", "value-2", "email", 300)
    result = store.persist()
    assert result.persisted is True
    assert store.has_token("pre-existing-1")
    assert store.has_token("pre-existing-2")

    store.store("uncommitted", "value-3", "email", 300)
    assert store.has_token("uncommitted")

    store.set_fail_persist(True)
    fail_result = store.persist()
    assert fail_result.persisted is False
    assert fail_result.rolled_back is True
    assert fail_result.operation_refused is True

    assert store.has_token("pre-existing-1")
    assert store.has_token("pre-existing-2")
    assert not store.has_token("uncommitted")
    assert store.token_count() == 2


def test_uncommitted_tokens_rolled_back_with_no_prior_commit(
    tmp_path: Path,
) -> None:
    """When no prior commit exists, a failing persist empties the store.

    Without a committed snapshot to restore, the rollback path clears all
    in-memory tokens so no uncommitted state survives a persistence
    failure.
    """
    store_path = tmp_path / "store.json"
    store = LocalEncryptedTokenStore(store_path, _KEY, fail_persist=True)

    store.store("token-a", "value-a", "email", 300)
    store.store("token-b", "value-b", "email", 300)

    result = store.persist()
    assert result.persisted is False
    assert result.rolled_back is True

    assert not store.has_token("token-a")
    assert not store.has_token("token-b")
    assert store.token_count() == 0


def test_real_write_failure_via_injected_writer_triggers_rollback(
    tmp_path: Path,
) -> None:
    """A real filesystem write failure triggers the same rollback path.

    The ``writer`` callable raises ``OSError`` to simulate a real disk
    failure.  The store restores the committed snapshot: pre-existing
    tokens survive and the uncommitted token is rolled back, identical to
    the ``fail_persist`` path.
    """
    store_path = tmp_path / "store.json"

    def failing_writer(path: Path, data: bytes) -> None:
        raise OSError("simulated disk failure")

    store = LocalEncryptedTokenStore(store_path, _KEY, writer=failing_writer)

    store.store("committed-1", "value-1", "email", 300)
    store.store("committed-2", "value-2", "email", 300)

    # Establish a committed snapshot with a successful persist by
    # temporarily using the default writer path.  We create a second
    # store without the failing writer to commit, then re-inject the
    # failing writer on the same store instance by constructing a new
    # store that shares the same path and key but has the failing writer.
    commit_store = LocalEncryptedTokenStore(store_path, _KEY)
    commit_store.store("committed-1", "value-1", "email", 300)
    commit_store.store("committed-2", "value-2", "email", 300)
    commit_result = commit_store.persist()
    assert commit_result.persisted is True

    # Restore the committed state into the failing-writer store, then
    # add an uncommitted token and attempt to persist with the failing
    # writer.
    store.restore()
    assert store.has_token("committed-1")
    assert store.has_token("committed-2")

    store.store("uncommitted", "value-3", "email", 300)
    assert store.has_token("uncommitted")

    fail_result = store.persist()
    assert fail_result.persisted is False
    assert fail_result.rolled_back is True
    assert fail_result.operation_refused is True

    assert store.has_token("committed-1")
    assert store.has_token("committed-2")
    assert not store.has_token("uncommitted")
    assert store.token_count() == 2


def test_real_write_failure_via_storage_error_triggers_rollback(
    tmp_path: Path,
) -> None:
    """A ``StorageError`` raised by the writer triggers the rollback path.

    The ``writer`` callable raises ``StorageError`` (the store's own
    typed error) to simulate a storage-boundary failure.  The rollback
    behavior is identical to the ``OSError`` and ``fail_persist`` paths.
    """
    store_path = tmp_path / "store.json"

    def storage_error_writer(path: Path, data: bytes) -> None:
        raise StorageError("storage boundary failure")

    store = LocalEncryptedTokenStore(store_path, _KEY, writer=storage_error_writer)

    store.store("committed", "value-1", "email", 300)
    # No prior commit — the store will be emptied on failure.
    store.store("uncommitted", "value-2", "email", 300)

    result = store.persist()
    assert result.persisted is False
    assert result.rolled_back is True
    assert result.operation_refused is True

    assert not store.has_token("committed")
    assert not store.has_token("uncommitted")
    assert store.token_count() == 0


def test_successful_persist_after_rollback_restores_committed_snapshot(
    tmp_path: Path,
) -> None:
    """After a rollback, a subsequent successful persist re-commits the restored state.

    The store is committed with two tokens, a failure rolls back the
    uncommitted third token, then ``fail_persist`` is cleared and a new
    persist succeeds.  The committed snapshot now contains the two
    pre-existing tokens, and a new token can be added and committed.
    """
    store_path = tmp_path / "store.json"
    store = LocalEncryptedTokenStore(store_path, _KEY)

    store.store("pre-existing-1", "value-1", "email", 300)
    store.store("pre-existing-2", "value-2", "email", 300)
    assert store.persist().persisted is True

    store.store("uncommitted", "value-3", "email", 300)
    store.set_fail_persist(True)
    assert store.persist().persisted is False

    store.set_fail_persist(False)
    store.store("new-after-rollback", "value-4", "email", 300)
    result = store.persist()
    assert result.persisted is True

    assert store.has_token("pre-existing-1")
    assert store.has_token("pre-existing-2")
    assert not store.has_token("uncommitted")
    assert store.has_token("new-after-rollback")
    assert store.token_count() == 3


def test_restore_sets_committed_snapshot_for_future_rollback(
    tmp_path: Path,
) -> None:
    """``restore`` sets the committed snapshot so future failures roll back to the restored state.

    A store is committed with two tokens, restored from disk into a new
    store instance, then an uncommitted token is added and a failure is
    injected.  The rollback restores the two committed tokens from the
    restored snapshot, not an empty state.
    """
    store_path = tmp_path / "store.json"
    store = LocalEncryptedTokenStore(store_path, _KEY)

    store.store("restored-1", "value-1", "email", 300)
    store.store("restored-2", "value-2", "email", 300)
    assert store.persist().persisted is True

    restored_store = LocalEncryptedTokenStore(store_path, _KEY)
    count = restored_store.restore()
    assert count == 2
    assert restored_store.has_token("restored-1")
    assert restored_store.has_token("restored-2")

    restored_store.store("uncommitted", "value-3", "email", 300)
    restored_store.set_fail_persist(True)
    result = restored_store.persist()
    assert result.persisted is False
    assert result.rolled_back is True

    assert restored_store.has_token("restored-1")
    assert restored_store.has_token("restored-2")
    assert not restored_store.has_token("uncommitted")
    assert restored_store.token_count() == 2
