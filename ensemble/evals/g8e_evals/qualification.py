from __future__ import annotations

import fnmatch
import hashlib
import json
from datetime import datetime
from enum import StrEnum
from pathlib import Path, PurePosixPath
from typing import Any, Literal, Self, cast

from cryptography import x509
from cryptography.hazmat.primitives import hashes, serialization
from pydantic import BaseModel, ConfigDict, Field, TypeAdapter, field_validator, model_validator

QUALIFICATION_SCHEMA_VERSION = "1.0.0"
_HASH_PATTERN = r"^[0-9a-f]{64}$"
_IMAGE_PATTERN = r"^sha256:[0-9a-f]{64}$"


class _FrozenModel(BaseModel):
    model_config = ConfigDict(extra="forbid", frozen=True, revalidate_instances="always")


def _payload(value: BaseModel | dict[str, Any]) -> dict[str, Any]:
    data = value.model_dump(mode="json", by_alias=True) if isinstance(value, BaseModel) else dict(value)
    data.pop("content_hash", None)
    return TypeAdapter(dict[str, Any]).dump_python(data, mode="json")


def content_hash(value: BaseModel | dict[str, Any]) -> str:
    encoded = json.dumps(
        _payload(value),
        allow_nan=False,
        ensure_ascii=False,
        separators=(",", ":"),
        sort_keys=True,
    ).encode()
    return hashlib.sha256(encoded).hexdigest()


def render_qualification_json(value: BaseModel) -> bytes:
    return (
        json.dumps(
            value.model_dump(mode="json", by_alias=True),
            allow_nan=False,
            ensure_ascii=False,
            indent=2,
            sort_keys=True,
        )
        + "\n"
    ).encode()


def _safe_relative_path(value: str) -> str:
    path = PurePosixPath(value)
    if not value or path.is_absolute() or ".." in path.parts or str(path) != value:
        raise ValueError("path must be a normalized relative POSIX path")
    return value


class ArtifactDigest(_FrozenModel):
    name: str = Field(min_length=1)
    path: str = Field(min_length=1)
    content_sha256: str = Field(pattern=_HASH_PATTERN)
    file_sha256: str = Field(pattern=_HASH_PATTERN)

    _validate_path = field_validator("path")(_safe_relative_path)

    @classmethod
    def from_json_file(cls, name: str, path: str, file_path: Path) -> ArtifactDigest:
        rendered = file_path.read_bytes()
        parsed = json.loads(rendered)
        if not isinstance(parsed, dict):
            raise ValueError("content-addressed JSON artifact must be an object")
        declared = parsed.get("content_hash")
        computed = content_hash(parsed)
        if declared != computed:
            raise ValueError("JSON artifact content_hash mismatch")
        return cls(
            name=name,
            path=path,
            content_sha256=computed,
            file_sha256=hashlib.sha256(rendered).hexdigest(),
        )


class SourceManifestAuthority(_FrozenModel):
    authority_id: str = Field(min_length=1)
    schema_version: str = QUALIFICATION_SCHEMA_VERSION
    scope: Literal["full_source", "execution_source"]
    mode: Literal["manifest"]
    entries: list[str] = Field(min_length=1)
    excludes: list[str]
    owner_approved_at: datetime
    content_hash: str = Field(pattern=_HASH_PATTERN)

    @model_validator(mode="after")
    def _validate_authority(self) -> Self:
        if len(self.entries) != len(set(self.entries)):
            raise ValueError("source manifest entries must be unique")
        for entry in self.entries:
            _safe_relative_path(entry)
        if len(self.excludes) != len(set(self.excludes)) or any(
            not value or "/" in value for value in self.excludes
        ):
            raise ValueError("source manifest excludes must be unique path-component patterns")
        if self.content_hash != content_hash(self):
            raise ValueError("source manifest authority content_hash mismatch")
        return self


class SourceManifestResult(_FrozenModel):
    schema_version: str = QUALIFICATION_SCHEMA_VERSION
    scope: Literal["full_source", "execution_source"]
    authority: ArtifactDigest
    source_tree_hash: str = Field(pattern=_HASH_PATTERN)
    content_hash: str = Field(pattern=_HASH_PATTERN)

    @classmethod
    def build(cls, **values: Any) -> SourceManifestResult:
        fields = {"schema_version": QUALIFICATION_SCHEMA_VERSION, **values}
        return cls(**fields, content_hash=content_hash(fields))

    @model_validator(mode="after")
    def _validate_result(self) -> Self:
        if self.content_hash != content_hash(self):
            raise ValueError("source manifest result content_hash mismatch")
        return self


def compute_source_manifest_result(
    authority_path: Path,
    source_root: Path,
    authority_record_path: str,
) -> SourceManifestResult:
    authority_digest = ArtifactDigest.from_json_file(
        authority_path.name, authority_record_path, authority_path
    )
    authority = SourceManifestAuthority.model_validate_json(authority_path.read_bytes())
    root = source_root.resolve(strict=True)
    if not root.is_dir():
        raise ValueError("source root must be a directory")
    files: dict[str, str] = {}
    for entry in authority.entries:
        if any(
            fnmatch.fnmatchcase(component, pattern)
            for component in PurePosixPath(entry).parts
            for pattern in authority.excludes
        ):
            continue
        current = root
        for part in PurePosixPath(entry).parts:
            current = current / part
            if current.is_symlink():
                raise ValueError(f"source manifest entry contains a symlink: {entry}")
        if not current.exists():
            raise ValueError(f"source manifest entry not found: {entry}")
        candidates = [current] if current.is_file() else current.rglob("*")
        for candidate in candidates:
            relative = candidate.relative_to(root)
            if any(
                fnmatch.fnmatchcase(component, pattern)
                for component in relative.parts
                for pattern in authority.excludes
            ):
                continue
            if candidate.is_symlink():
                raise ValueError(f"source manifest contains a symlink: {relative.as_posix()}")
            if not candidate.is_file():
                continue
            files[relative.as_posix()] = hashlib.sha256(candidate.read_bytes()).hexdigest()
    digest = hashlib.sha256()
    for path in sorted(files, key=lambda value: tuple(PurePosixPath(value).parts)):
        digest.update(path.encode())
        digest.update(b"\0")
        digest.update(files[path].encode())
        digest.update(b"\0")
    return SourceManifestResult.build(
        scope=authority.scope,
        authority=authority_digest,
        source_tree_hash=digest.hexdigest(),
    )


class ComponentImageIdentity(_FrozenModel):
    components: list[str] = Field(min_length=1)
    image_id: str = Field(pattern=_IMAGE_PATTERN)

    @model_validator(mode="after")
    def _validate_components(self) -> Self:
        if len(self.components) != len(set(self.components)) or any(not value for value in self.components):
            raise ValueError("image components must be non-empty and unique")
        return self


class CandidateIdentityEvidence(_FrozenModel):
    source_tree_hash: str = Field(pattern=_HASH_PATTERN)
    execution_source_manifest_hash: str = Field(pattern=_HASH_PATTERN)
    binary_sha256: str = Field(pattern=_HASH_PATTERN)
    images: list[ComponentImageIdentity] = Field(min_length=1)
    content_hash: str = Field(pattern=_HASH_PATTERN)

    @classmethod
    def build(
        cls,
        *,
        source_tree_hash: str,
        execution_source_manifest_hash: str,
        binary_sha256: str,
        images: list[ComponentImageIdentity],
    ) -> CandidateIdentityEvidence:
        fields = {
            "source_tree_hash": source_tree_hash,
            "execution_source_manifest_hash": execution_source_manifest_hash,
            "binary_sha256": binary_sha256,
            "images": images,
        }
        return cls(**fields, content_hash=content_hash(fields))

    @model_validator(mode="after")
    def _validate_candidate(self) -> Self:
        components = [component for image in self.images for component in image.components]
        image_ids = [image.image_id for image in self.images]
        if [image.components for image in self.images] != [
            ["gateway", "operator"],
            ["ensemble"],
            ["dashboard"],
        ]:
            raise ValueError("candidate image components must match the canonical stack")
        if len(components) != len(set(components)):
            raise ValueError("candidate image components must be unique")
        if len(image_ids) != len(set(image_ids)):
            raise ValueError("candidate image identities must be unique")
        if self.content_hash != content_hash(self):
            raise ValueError("candidate content_hash mismatch")
        return self


class EvidencePath(_FrozenModel):
    name: str = Field(min_length=1)
    path: str = Field(min_length=1)

    _validate_path = field_validator("path")(_safe_relative_path)


class CertificateIdentityEvidence(_FrozenModel):
    component: Literal["cli", "remote_operator", "ensemble", "dashboard"]
    certificate_sha256: str = Field(pattern=_HASH_PATTERN)
    spiffe_ids: list[str] = Field(min_length=1)

    @model_validator(mode="after")
    def _validate_spiffe_ids(self) -> Self:
        if len(self.spiffe_ids) != len(set(self.spiffe_ids)) or any(
            not value.startswith("spiffe://g8e.local/") for value in self.spiffe_ids
        ):
            raise ValueError("certificate SPIFFE IDs must be unique g8e.local identities")
        return self


class VersionStampEvidence(_FrozenModel):
    component: Literal["host", "gateway", "operator"]
    version: str = Field(pattern=r"^v[0-9]+\.[0-9]+\.[0-9]+$")
    source_tree_state_hash: str = Field(pattern=_HASH_PATTERN)


class RuntimeIdentityEvidence(_FrozenModel):
    candidate_content_hash: str = Field(pattern=_HASH_PATTERN)
    build_id: str = Field(min_length=1)
    build_time: datetime
    source_revision: str = Field(min_length=1)
    pki_root_sha256: str = Field(pattern=_HASH_PATTERN)
    owner_id: str = Field(min_length=1)
    cli_session_id: str = Field(min_length=1)
    embedded_operator_id: str = Field(min_length=1)
    embedded_operator_session_id: str = Field(min_length=1)
    remote_operator_id: str = Field(min_length=1)
    remote_operator_session_id: str = Field(min_length=1)
    certificates: list[CertificateIdentityEvidence] = Field(min_length=4, max_length=4)
    version_stamps: list[VersionStampEvidence] = Field(min_length=3, max_length=3)
    healthy_components: list[str] = Field(min_length=4, max_length=4)
    pending_enrollment_count: Literal[0]
    content_hash: str = Field(pattern=_HASH_PATTERN)

    @classmethod
    def build(cls, **values: Any) -> RuntimeIdentityEvidence:
        return cls(**values, content_hash=content_hash(values))

    @model_validator(mode="after")
    def _validate_runtime(self) -> Self:
        certificates = {certificate.component: certificate for certificate in self.certificates}
        if set(certificates) != {"cli", "remote_operator", "ensemble", "dashboard"}:
            raise ValueError("runtime certificate components are incomplete")
        expected_spiffe = {
            "cli": {f"spiffe://g8e.local/cli/{self.owner_id}/{self.cli_session_id}"},
            "remote_operator": {
                f"spiffe://g8e.local/operator//{self.remote_operator_id}/{self.remote_operator_session_id}"
            },
            "ensemble": {
                "spiffe://g8e.local/app/g8ee",
                f"spiffe://g8e.local/user/{self.owner_id}",
            },
            "dashboard": {
                "spiffe://g8e.local/app/g8ed",
                f"spiffe://g8e.local/user/{self.owner_id}",
            },
        }
        if any(set(certificates[name].spiffe_ids) != expected for name, expected in expected_spiffe.items()):
            raise ValueError("runtime certificate SPIFFE identity mismatch")
        if {stamp.component for stamp in self.version_stamps} != {"host", "gateway", "operator"}:
            raise ValueError("runtime version stamp components are incomplete")
        if self.healthy_components != ["gateway", "operator", "ensemble", "dashboard"]:
            raise ValueError("runtime healthy components must be complete and canonical")
        if self.content_hash != content_hash(self):
            raise ValueError("runtime identity content_hash mismatch")
        return self


class RuntimeCollectionRequest(_FrozenModel):
    schema_version: str = QUALIFICATION_SCHEMA_VERSION
    version: str = Field(pattern=r"^v[0-9]+\.[0-9]+\.[0-9]+$")
    candidate_path: str
    pki_root_path: str
    certificate_paths: list[EvidencePath] = Field(min_length=4, max_length=4)
    version_paths: list[EvidencePath] = Field(min_length=3, max_length=3)
    owner_id: str = Field(min_length=1)
    cli_session_id: str = Field(min_length=1)
    embedded_operator_id: str = Field(min_length=1)
    embedded_operator_session_id: str = Field(min_length=1)
    remote_operator_id: str = Field(min_length=1)
    remote_operator_session_id: str = Field(min_length=1)
    healthy_components: list[str] = Field(min_length=4, max_length=4)
    pending_enrollment_count: Literal[0]

    _validate_paths = field_validator("candidate_path", "pki_root_path")(_safe_relative_path)

    @model_validator(mode="after")
    def _validate_collection_request(self) -> Self:
        if [item.name for item in self.certificate_paths] != [
            "cli",
            "remote_operator",
            "ensemble",
            "dashboard",
        ]:
            raise ValueError("runtime certificate paths must use canonical component order")
        if [item.name for item in self.version_paths] != ["host", "gateway", "operator"]:
            raise ValueError("runtime version paths must use canonical component order")
        if self.healthy_components != ["gateway", "operator", "ensemble", "dashboard"]:
            raise ValueError("runtime health observations must use canonical component order")
        return self


def collect_runtime_identity(
    request: RuntimeCollectionRequest,
    base_dir: Path,
) -> RuntimeIdentityEvidence:
    _, candidate_bytes = _read_evidence_file(base_dir, request.candidate_path)
    candidate = CandidateIdentityEvidence.model_validate_json(candidate_bytes)
    root_path, root_bytes = _read_evidence_file(base_dir, request.pki_root_path)
    root_certificates = x509.load_pem_x509_certificates(root_bytes)
    if len(root_certificates) != 1:
        raise ValueError(f"PKI root file must contain exactly one certificate: {root_path.name}")
    certificates: list[CertificateIdentityEvidence] = []
    for binding in request.certificate_paths:
        _, certificate_bytes = _read_evidence_file(base_dir, binding.path)
        if b"PRIVATE KEY" in certificate_bytes:
            raise ValueError(f"certificate evidence contains private key material: {binding.path}")
        parsed = x509.load_pem_x509_certificates(certificate_bytes)
        if not parsed:
            raise ValueError(f"certificate file contains no certificate: {binding.path}")
        certificate = parsed[0]
        try:
            san = certificate.extensions.get_extension_for_class(x509.SubjectAlternativeName)
            spiffe_ids = san.value.get_values_for_type(x509.UniformResourceIdentifier)
        except x509.ExtensionNotFound as error:
            raise ValueError(f"certificate URI SAN missing: {binding.path}") from error
        certificates.append(
            CertificateIdentityEvidence(
                component=cast(
                    Literal["cli", "remote_operator", "ensemble", "dashboard"],
                    binding.name,
                ),
                certificate_sha256=certificate.fingerprint(hashes.SHA256()).hex(),
                spiffe_ids=spiffe_ids,
            )
        )
    stamps: list[VersionStampEvidence] = []
    host_version: dict[str, Any] | None = None
    for binding in request.version_paths:
        _, version_bytes = _read_evidence_file(base_dir, binding.path)
        version_data = json.loads(version_bytes)
        if not isinstance(version_data, dict):
            raise ValueError(f"version evidence must be a JSON object: {binding.path}")
        if version_data.get("version") != request.version:
            raise ValueError(f"version evidence mismatch: {binding.path}")
        source_hash = version_data.get("source_tree_state_hash")
        if not isinstance(source_hash, str):
            raise ValueError(f"version source hash missing: {binding.path}")
        stamps.append(
            VersionStampEvidence(
                component=cast(Literal["host", "gateway", "operator"], binding.name),
                version=request.version,
                source_tree_state_hash=source_hash,
            )
        )
        if binding.name == "host":
            host_version = version_data
    if host_version is None:
        raise ValueError("host version evidence missing")
    root_der = root_certificates[0].public_bytes(serialization.Encoding.DER)
    return RuntimeIdentityEvidence.build(
        candidate_content_hash=candidate.content_hash,
        build_id=host_version.get("build_id") or "unavailable",
        build_time=host_version.get("build_time"),
        source_revision=host_version.get("source_revision") or "unavailable",
        pki_root_sha256=hashlib.sha256(root_der).hexdigest(),
        owner_id=request.owner_id,
        cli_session_id=request.cli_session_id,
        embedded_operator_id=request.embedded_operator_id,
        embedded_operator_session_id=request.embedded_operator_session_id,
        remote_operator_id=request.remote_operator_id,
        remote_operator_session_id=request.remote_operator_session_id,
        certificates=certificates,
        version_stamps=stamps,
        healthy_components=request.healthy_components,
        pending_enrollment_count=request.pending_enrollment_count,
    )


class GateResultEvidence(_FrozenModel):
    gate_id: str = Field(min_length=1)
    candidate_content_hash: str = Field(pattern=_HASH_PATTERN)
    command: list[str] = Field(min_length=1)
    started_at: datetime
    completed_at: datetime
    exit_code: int
    stdout_sha256: str = Field(pattern=_HASH_PATTERN)
    stderr_sha256: str = Field(pattern=_HASH_PATTERN)
    tool_versions: dict[str, str] = Field(min_length=1)
    skipped: list[str]
    result: Literal["passed", "failed"]
    content_hash: str = Field(pattern=_HASH_PATTERN)

    @classmethod
    def build(cls, **values: Any) -> GateResultEvidence:
        fields = dict(values)
        fields["result"] = "passed" if fields["exit_code"] == 0 else "failed"
        return cls(**fields, content_hash=content_hash(fields))

    @model_validator(mode="after")
    def _validate_gate(self) -> Self:
        if self.completed_at < self.started_at:
            raise ValueError("gate completed_at must not precede started_at")
        if any(not argument for argument in self.command):
            raise ValueError("gate command arguments must be non-empty")
        if any(not name or not version for name, version in self.tool_versions.items()):
            raise ValueError("gate tool versions must be non-empty")
        if self.content_hash != content_hash(self):
            raise ValueError("gate content_hash mismatch")
        return self


class PublicLoopEvidence(_FrozenModel):
    schema_version: str = QUALIFICATION_SCHEMA_VERSION
    candidate_content_hash: str = Field(pattern=_HASH_PATTERN)
    source_id: str = Field(min_length=1)
    first_sequence: int = Field(ge=1)
    high_water_sequence: int = Field(ge=1)
    batch_count: int = Field(ge=1)
    feed_chain_hash: str = Field(pattern=_HASH_PATTERN)
    proof_artifact_sha256: str = Field(pattern=_HASH_PATTERN)
    proof_artifact_bytes: int = Field(gt=0)
    retry_count: int = Field(ge=1)
    old_key_id: str = Field(pattern=_HASH_PATTERN)
    replacement_key_id: str = Field(pattern=_HASH_PATTERN)
    old_key_revoked: Literal[True]
    replacement_key_accepted_after_restart: Literal[True]
    mirror_restart_recovered: Literal[True]
    inference_invocations: Literal[0]
    generated_at: datetime
    content_hash: str = Field(pattern=_HASH_PATTERN)

    @classmethod
    def build(cls, **values: Any) -> PublicLoopEvidence:
        fields = {"schema_version": QUALIFICATION_SCHEMA_VERSION, **values}
        return cls(**fields, content_hash=content_hash(fields))

    @model_validator(mode="after")
    def _validate_loop(self) -> Self:
        if self.first_sequence > self.high_water_sequence:
            raise ValueError("public loop sequence range is invalid")
        if self.content_hash != content_hash(self):
            raise ValueError("public loop content_hash mismatch")
        return self


class QualificationStatus(StrEnum):
    OWNER_REVIEW_REQUIRED = "owner_review_required"


class QualificationBuildRequest(_FrozenModel):
    schema_version: str = QUALIFICATION_SCHEMA_VERSION
    record_id: str = Field(min_length=1)
    version: str = Field(pattern=r"^v[0-9]+\.[0-9]+\.[0-9]+$")
    generated_at: datetime
    candidate_path: str
    runtime_path: str
    required_gate_ids: list[str] = Field(min_length=1)
    gate_paths: list[EvidencePath] = Field(min_length=1)
    public_loop_path: str
    authority_paths: list[EvidencePath] = Field(min_length=1)
    prior_qualification_content_hash: str = Field(pattern=_HASH_PATTERN)
    invalidation_authority_name: str = Field(min_length=1)
    endpoint_class: Literal["owner_operated_remote_ollama"]

    _validate_candidate_path = field_validator(
        "candidate_path", "runtime_path", "public_loop_path"
    )(_safe_relative_path)

    @model_validator(mode="after")
    def _validate_request(self) -> Self:
        if [binding.name for binding in self.gate_paths] != self.required_gate_ids:
            raise ValueError("gate paths must exactly match required gate IDs in canonical order")
        authority_names = [binding.name for binding in self.authority_paths]
        if len(authority_names) != len(set(authority_names)):
            raise ValueError("authority path names must be unique")
        if self.invalidation_authority_name not in authority_names:
            raise ValueError("invalidation authority must be present")
        return self


class QualificationInput(_FrozenModel):
    schema_version: str = QUALIFICATION_SCHEMA_VERSION
    record_id: str = Field(min_length=1)
    version: str = Field(pattern=r"^v[0-9]+\.[0-9]+\.[0-9]+$")
    generated_at: datetime
    candidate: CandidateIdentityEvidence
    runtime: RuntimeIdentityEvidence
    required_gate_ids: list[str] = Field(min_length=1)
    gates: list[GateResultEvidence] = Field(min_length=1)
    public_loop: PublicLoopEvidence
    authority_bindings: list[ArtifactDigest] = Field(min_length=1)
    prior_qualification_content_hash: str = Field(pattern=_HASH_PATTERN)
    invalidation_content_hash: str = Field(pattern=_HASH_PATTERN)
    endpoint_class: Literal["owner_operated_remote_ollama"]

    @model_validator(mode="after")
    def _validate_input(self) -> Self:
        gate_ids = [gate.gate_id for gate in self.gates]
        if len(gate_ids) != len(set(gate_ids)):
            raise ValueError("gate IDs must be unique")
        if gate_ids != self.required_gate_ids:
            raise ValueError("required gate IDs must exactly match supplied gates in canonical order")
        if any(gate.result != "passed" or gate.exit_code != 0 for gate in self.gates):
            raise ValueError("every qualification gate must have passed")
        if self.runtime.candidate_content_hash != self.candidate.content_hash:
            raise ValueError("runtime candidate identity mismatch")
        if any(
            stamp.source_tree_state_hash != self.candidate.execution_source_manifest_hash
            for stamp in self.runtime.version_stamps
        ):
            raise ValueError("runtime source stamp mismatch")
        if any(stamp.version != self.version for stamp in self.runtime.version_stamps):
            raise ValueError("runtime version mismatch")
        if any(gate.candidate_content_hash != self.candidate.content_hash for gate in self.gates):
            raise ValueError("gate candidate identity mismatch")
        if any(
            gate.started_at < self.runtime.build_time or gate.completed_at > self.generated_at
            for gate in self.gates
        ):
            raise ValueError("gate evidence is stale or outside the qualification window")
        if self.public_loop.candidate_content_hash != self.candidate.content_hash:
            raise ValueError("public loop candidate identity mismatch")
        if not self.runtime.build_time <= self.public_loop.generated_at <= self.generated_at:
            raise ValueError("public loop evidence is stale or outside the qualification window")
        binding_names = [binding.name for binding in self.authority_bindings]
        if len(binding_names) != len(set(binding_names)):
            raise ValueError("authority binding names must be unique")
        if self.invalidation_content_hash not in {
            binding.content_sha256 for binding in self.authority_bindings
        }:
            raise ValueError("invalidation content hash must be bound as an authority")
        return self


class CollectionCandidateQualification(_FrozenModel):
    schema_version: str
    record_id: str
    version: str
    generated_at: datetime
    candidate: CandidateIdentityEvidence
    runtime: RuntimeIdentityEvidence
    verification_gates: list[GateResultEvidence]
    public_loop: PublicLoopEvidence
    authority_bindings: list[ArtifactDigest]
    prior_qualification_content_hash: str
    invalidation_content_hash: str
    endpoint_class: Literal["owner_operated_remote_ollama"]
    status: Literal[QualificationStatus.OWNER_REVIEW_REQUIRED]
    owner_approval_required: Literal[True]
    live_operation_lease_status: Literal["no_active_lease"]
    model_inventory_status: Literal["not_queried"]
    content_hash: str = Field(pattern=_HASH_PATTERN)

    @model_validator(mode="after")
    def _validate_record(self) -> Self:
        if self.content_hash != content_hash(self):
            raise ValueError("qualification content_hash mismatch")
        return self


def _read_evidence_file(base_dir: Path, relative_path: str) -> tuple[Path, bytes]:
    base = base_dir.resolve(strict=True)
    current = base
    for part in PurePosixPath(relative_path).parts:
        current = current / part
        if current.is_symlink():
            raise ValueError(f"evidence path contains a symlink: {relative_path}")
    resolved = current.resolve(strict=True)
    if not resolved.is_relative_to(base) or not resolved.is_file():
        raise ValueError(f"evidence path is not a regular file under the input root: {relative_path}")
    return resolved, resolved.read_bytes()


def verify_historical_qualification(path: Path) -> ArtifactDigest:
    return ArtifactDigest.from_json_file(path.name, path.name, path)


def resolve_qualification_input(
    request: QualificationBuildRequest,
    base_dir: Path,
) -> QualificationInput:
    _, candidate_bytes = _read_evidence_file(base_dir, request.candidate_path)
    _, runtime_bytes = _read_evidence_file(base_dir, request.runtime_path)
    _, public_loop_bytes = _read_evidence_file(base_dir, request.public_loop_path)
    candidate = CandidateIdentityEvidence.model_validate_json(candidate_bytes)
    runtime = RuntimeIdentityEvidence.model_validate_json(runtime_bytes)
    public_loop = PublicLoopEvidence.model_validate_json(public_loop_bytes)
    gates: list[GateResultEvidence] = []
    for binding in request.gate_paths:
        _, gate_bytes = _read_evidence_file(base_dir, binding.path)
        gate = GateResultEvidence.model_validate_json(gate_bytes)
        if gate.gate_id != binding.name:
            raise ValueError(f"gate evidence name mismatch for {binding.path}")
        gates.append(gate)
    authorities: list[ArtifactDigest] = []
    invalidation_hash = ""
    for binding in request.authority_paths:
        authority_path, _ = _read_evidence_file(base_dir, binding.path)
        digest = ArtifactDigest.from_json_file(binding.name, binding.path, authority_path)
        authorities.append(digest)
        if binding.name == request.invalidation_authority_name:
            invalidation_hash = digest.content_sha256
    return QualificationInput(
        schema_version=request.schema_version,
        record_id=request.record_id,
        version=request.version,
        generated_at=request.generated_at,
        candidate=candidate,
        runtime=runtime,
        required_gate_ids=request.required_gate_ids,
        gates=gates,
        public_loop=public_loop,
        authority_bindings=authorities,
        prior_qualification_content_hash=request.prior_qualification_content_hash,
        invalidation_content_hash=invalidation_hash,
        endpoint_class=request.endpoint_class,
    )


def build_collection_candidate_qualification(
    inputs: QualificationInput,
) -> CollectionCandidateQualification:
    fields = {
        "schema_version": inputs.schema_version,
        "record_id": inputs.record_id,
        "version": inputs.version,
        "generated_at": inputs.generated_at,
        "candidate": inputs.candidate,
        "runtime": inputs.runtime,
        "verification_gates": inputs.gates,
        "public_loop": inputs.public_loop,
        "authority_bindings": inputs.authority_bindings,
        "prior_qualification_content_hash": inputs.prior_qualification_content_hash,
        "invalidation_content_hash": inputs.invalidation_content_hash,
        "endpoint_class": inputs.endpoint_class,
        "status": QualificationStatus.OWNER_REVIEW_REQUIRED,
        "owner_approval_required": True,
        "live_operation_lease_status": "no_active_lease",
        "model_inventory_status": "not_queried",
    }
    return CollectionCandidateQualification(**fields, content_hash=content_hash(fields))


__all__ = [
    "ArtifactDigest",
    "CandidateIdentityEvidence",
    "CertificateIdentityEvidence",
    "CollectionCandidateQualification",
    "ComponentImageIdentity",
    "EvidencePath",
    "GateResultEvidence",
    "PublicLoopEvidence",
    "QualificationBuildRequest",
    "QualificationInput",
    "QualificationStatus",
    "RuntimeCollectionRequest",
    "RuntimeIdentityEvidence",
    "SourceManifestAuthority",
    "SourceManifestResult",
    "VersionStampEvidence",
    "build_collection_candidate_qualification",
    "collect_runtime_identity",
    "compute_source_manifest_result",
    "content_hash",
    "render_qualification_json",
    "resolve_qualification_input",
    "verify_historical_qualification",
]
