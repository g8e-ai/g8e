# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""Canonical serialization and content-hash helpers for g8e_evals.

This module is the single source of truth for model-to-dict
canonicalization, typed JSON/JSONL readers, and content-hash
computation. Every content hash in the package is SHA-256 over
canonical JSON (sorted keys, no extra whitespace, no NaN, UTF-8).
The helpers here guarantee byte-identical output to the previous
``json.loads(m.model_dump_json())`` and hand-rolled-dict patterns
they replace, so existing content hashes are preserved.

Conventions:

- ``canonical_model_dict`` is the ONLY sanctioned model-to-dict
  conversion for canonicalization. It calls
  ``m.model_dump(mode="json", by_alias=True)``, which produces
  exactly the dict that ``json.loads(m.model_dump_json())`` produces.
- ``canonical_model_list`` replaces every
  ``[json.loads(e.model_dump_json()) for e in ...]`` comprehension.
- ``read_jsonl_models`` / ``read_json_model`` replace raw
  ``json.loads`` reads on known-shape JSONL/JSON files with typed
  ``model_validate_json`` calls.
- ``content_hash_of`` computes the SHA-256 over canonical JSON of a
  model, excluding the ``content_hash`` field itself (the
  chicken-and-egg field). It accepts only ``BaseModel`` instances,
  never raw dicts.
- ``provisional_hash`` is the single sanctioned use of
  ``model_construct`` in the package. It computes the content hash
  for a not-yet-hashed frozen model by constructing a provisional
  instance with a placeholder ``content_hash`` and hashing its
  canonical JSON (excluding the placeholder). Callers pass typed
  field values as keyword arguments; the helper handles the two-phase
  construct-then-hash internally.
"""

from __future__ import annotations

import hashlib
import json
from collections.abc import Iterable
from pathlib import Path

from pydantic import BaseModel


_CONTENT_HASH_FIELD = "content_hash"
_PLACEHOLDER_HASH = "0" * 64


def canonical_json(data: object) -> str:
    """Serialize ``data`` to canonical JSON (sorted keys, compact, no NaN)."""
    return json.dumps(
        data,
        allow_nan=False,
        ensure_ascii=False,
        separators=(",", ":"),
        sort_keys=True,
    )


def canonical_model_dict(model: BaseModel) -> dict[str, object]:
    """Return the canonical dict form of ``model``.

    This is the ONLY sanctioned model-to-dict conversion for
    canonicalization. It produces exactly the dict that
    ``json.loads(model.model_dump_json())`` produces, so content
    hashes computed from the result are byte-identical to those
    computed from the previous ``json.loads`` pattern.
    """
    return model.model_dump(mode="json", by_alias=True)


def canonical_model_list(models: Iterable[BaseModel]) -> list[dict[str, object]]:
    """Return a list of canonical dicts for ``models``.

    Replaces every ``[json.loads(e.model_dump_json()) for e in ...]``
    comprehension. The output is byte-identical to the replaced
    pattern.
    """
    return [canonical_model_dict(m) for m in models]


def read_json_model[M: BaseModel](path: Path, model: type[M]) -> M:
    """Read and validate a JSON file as ``model``.

    Raises ``pydantic.ValidationError`` on malformed input. Callers
    that need a module-specific typed error wrap this call and
    re-raise.
    """
    return model.model_validate_json(path.read_text())


def read_jsonl_models[M: BaseModel](path: Path, model: type[M]) -> list[M]:
    """Read and validate a JSONL file as a list of ``model``.

    Blank lines are skipped. Each non-blank line is validated via
    ``model_validate_json``. Raises ``pydantic.ValidationError`` on
    malformed input. Callers that need a module-specific typed error
    wrap this call and re-raise.
    """
    records: list[M] = []
    for line in path.read_text().splitlines():
        line = line.strip()
        if line:
            records.append(model.model_validate_json(line))
    return records


def content_hash_of(model: BaseModel) -> str:
    """Compute the SHA-256 content hash of ``model``.

    The hash is over the canonical JSON of the model with the
    ``content_hash`` field excluded (the chicken-and-egg field).
    Accepts only ``BaseModel`` instances, never raw dicts.
    """
    data = canonical_model_dict(model)
    data.pop(_CONTENT_HASH_FIELD, None)
    return hashlib.sha256(canonical_json(data).encode()).hexdigest()


def provisional_hash[M: BaseModel](model_cls: type[M], /, **fields: object) -> str:
    """Compute the content hash for a not-yet-hashed frozen model.

    This is the single sanctioned use of ``model_construct`` in the
    package. Callers pass the model class and typed field values as
    keyword arguments. The helper constructs a provisional instance
    with a placeholder ``content_hash``, then computes the real hash
    over the canonical JSON of the provisional (excluding the
    placeholder).

    The returned hash is the value callers assign to the model's
    ``content_hash`` field when constructing the real instance via
    normal validation.
    """
    provisional = model_cls.model_construct(**fields, **{_CONTENT_HASH_FIELD: _PLACEHOLDER_HASH})
    return content_hash_of(provisional)


__all__ = [
    "canonical_json",
    "canonical_model_dict",
    "canonical_model_list",
    "content_hash_of",
    "provisional_hash",
    "read_json_model",
    "read_jsonl_models",
]
