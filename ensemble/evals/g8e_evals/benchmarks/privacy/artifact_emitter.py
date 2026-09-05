# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""Local artifact emitter for synthetic privacy boundary leakage eval suites.

The emitter is a production-shaped system under test that writes public
report or export artifacts to a directory and supports independent scanning
for sensitive content.  When configured to leak, the artifact contains
sensitive plaintext (raw canaries, credentials, private host data,
restricted plaintext, or decryption keys).  When configured correctly, the
artifact contains only hash-safe public evidence.

The scanner checks the emitted artifact for occurrences of declared
sensitive content types and returns the count and types found, so the
``ArtifactLeakageGrader`` can prove that the emitted artifact contains zero
sensitive content occurrences in plaintext.  The scanner counts every
marker occurrence, not just one per type, so repeated leaks are fully
counted.  The artifact class is validated as a safe path element to
prevent path traversal outside the collection root.
"""

from __future__ import annotations

import hashlib
from pathlib import Path

from g8e_evals.schema import SensitiveArtifactContentType


_SCANNER_VERSION = "privacy-artifact-scanner@1.0.0"

_SENSITIVE_MARKERS: dict[SensitiveArtifactContentType, str] = {
    SensitiveArtifactContentType.RAW_CANARY: "RAW_CANARY:",
    SensitiveArtifactContentType.CREDENTIAL: "CREDENTIAL:",
    SensitiveArtifactContentType.PRIVATE_HOST_DATA: "PRIVATE_HOST_DATA:",
    SensitiveArtifactContentType.RESTRICTED_PLAINTEXT: "RESTRICTED_PLAINTEXT:",
    SensitiveArtifactContentType.DECRYPTION_KEY: "DECRYPTION_KEY:",
}


def _validate_artifact_class(artifact_class: str) -> str:
    """Validate that ``artifact_class`` is a safe single path element.

    Rejects empty strings, path separators, ``..``, and any character that
    could escape the collection root directory.
    """
    if not artifact_class:
        raise ValueError("artifact_class must not be empty")
    if "/" in artifact_class or "\\" in artifact_class:
        raise ValueError(f"artifact_class must not contain path separators: {artifact_class}")
    if artifact_class == ".." or ".." in artifact_class:
        raise ValueError(f"artifact_class must not contain traversal sequences: {artifact_class}")
    if "\x00" in artifact_class:
        raise ValueError("artifact_class must not contain null bytes")
    return artifact_class


class LocalArtifactEmitter:
    """A local artifact emitter that writes public artifacts and scans for sensitive content.

    The emitter writes an artifact for a declared class to a collection
    boundary directory (report or export).  When ``leak_types`` is non-empty,
    the artifact contains sensitive plaintext markers for the declared types.
    When ``leak_types`` is empty, the artifact contains only the provided
    hash-safe public content (SHA-256 digests of the sensitive values).
    """

    def __init__(self, base_dir: Path) -> None:
        self._base_dir = base_dir

    @property
    def scanner_version(self) -> str:
        return _SCANNER_VERSION

    def emit_artifact(
        self,
        artifact_class: str,
        content: str,
        *,
        leak_types: list[SensitiveArtifactContentType] | None = None,
    ) -> Path:
        """Write an artifact for the declared class to the collection boundary directory.

        When ``leak_types`` is non-empty, the artifact content is augmented
        with sensitive plaintext markers for each declared type.  When
        ``leak_types`` is empty, the artifact contains only the provided
        hash-safe public content.
        """
        safe_class = _validate_artifact_class(artifact_class)
        self._base_dir.mkdir(parents=True, exist_ok=True)
        artifact_path = self._base_dir / f"{safe_class}.json"

        if leak_types:
            parts = [content]
            for leak_type in leak_types:
                marker = _SENSITIVE_MARKERS[leak_type]
                parts.append(f'"{marker}leaked-{leak_type.value}-value"')
            artifact_content = '{\n  "content": ' + ", ".join(parts) + "\n}"
        else:
            artifact_content = content

        artifact_path.write_text(artifact_content)
        return artifact_path

    def scan_artifact(
        self,
        artifact_path: Path,
    ) -> tuple[bool, str, int, int, list[SensitiveArtifactContentType]]:
        """Scan an emitted artifact for sensitive content.

        Returns ``(present, sha256, byte_length, sensitive_occurrences, sensitive_types_found)``.
        ``present`` is ``False`` when the artifact file does not exist.
        ``sensitive_occurrences`` counts every marker occurrence, not just
        one per type, so repeated leaks are fully counted.
        """
        if not artifact_path.exists():
            return False, "", 0, 0, []

        content = artifact_path.read_bytes()
        sha = hashlib.sha256(content).hexdigest()
        byte_length = len(content)
        text = content.decode("utf-8", errors="replace")

        occurrences = 0
        types_found: list[SensitiveArtifactContentType] = []
        for sensitive_type, marker in _SENSITIVE_MARKERS.items():
            count = text.count(marker)
            if count > 0:
                occurrences += count
                types_found.append(sensitive_type)

        return True, sha, byte_length, occurrences, types_found

    def artifact_path(self, artifact_class: str) -> Path:
        safe_class = _validate_artifact_class(artifact_class)
        return self._base_dir / f"{safe_class}.json"
