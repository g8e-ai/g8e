from __future__ import annotations

import hashlib
import subprocess
from datetime import UTC, datetime
from pathlib import Path

import click
from pydantic import ValidationError

from g8e_evals.qualification import (
    CandidateIdentityEvidence,
    ComponentImageIdentity,
    GateResultEvidence,
    QualificationBuildRequest,
    RuntimeCollectionRequest,
    SourceManifestResult,
    build_collection_candidate_qualification,
    collect_runtime_identity,
    compute_source_manifest_result,
    render_qualification_json,
    resolve_qualification_input,
)


@click.group(name="qualification")
def qualification_cmd() -> None:
    """Build deterministic collection-candidate qualification drafts."""


@qualification_cmd.command(name="hash-source")
@click.option("--authority", type=click.Path(exists=True, dir_okay=False, path_type=Path), required=True)
@click.option("--source-root", type=click.Path(exists=True, file_okay=False, path_type=Path), required=True)
@click.option("--authority-record-path", required=True)
@click.option("--output", type=click.Path(dir_okay=False, path_type=Path), required=True)
def qualification_hash_source(
    authority: Path,
    source_root: Path,
    authority_record_path: str,
    output: Path,
) -> None:
    """Hash an explicit owner-approved source manifest without Git."""
    try:
        result = compute_source_manifest_result(authority, source_root, authority_record_path)
        with output.open("xb") as result_file:
            result_file.write(result.model_dump_json(indent=2).encode() + b"\n")
    except (OSError, ValidationError, ValueError) as error:
        raise click.ClickException(str(error)) from error
    click.echo(str(output))


@qualification_cmd.command(name="candidate")
@click.option("--full-source", type=click.Path(exists=True, dir_okay=False, path_type=Path), required=True)
@click.option("--execution-source", type=click.Path(exists=True, dir_okay=False, path_type=Path), required=True)
@click.option("--binary", type=click.Path(exists=True, dir_okay=False, path_type=Path), required=True)
@click.option("--image", multiple=True, required=True)
@click.option("--output", type=click.Path(dir_okay=False, path_type=Path), required=True)
def qualification_candidate(
    full_source: Path,
    execution_source: Path,
    binary: Path,
    image: tuple[str, ...],
    output: Path,
) -> None:
    """Build a typed candidate identity from exact local artifacts."""
    images: list[ComponentImageIdentity] = []
    for item in image:
        components_text, separator, image_id = item.partition("=")
        components = components_text.split(",")
        if not separator or any(not component for component in components) or not image_id:
            raise click.UsageError("--image values must use component[,component]=sha256:<digest>")
        images.append(ComponentImageIdentity(components=components, image_id=image_id))
    try:
        if binary.is_symlink():
            raise ValueError("candidate binary must not be a symlink")
        full_source_result = SourceManifestResult.model_validate_json(full_source.read_bytes())
        execution_source_result = SourceManifestResult.model_validate_json(execution_source.read_bytes())
        if full_source_result.scope != "full_source" or execution_source_result.scope != "execution_source":
            raise ValueError("candidate source manifest scopes are invalid")
        identity = CandidateIdentityEvidence.build(
            source_tree_hash=full_source_result.source_tree_hash,
            execution_source_manifest_hash=execution_source_result.source_tree_hash,
            binary_sha256=hashlib.sha256(binary.read_bytes()).hexdigest(),
            images=images,
        )
        with output.open("xb") as identity_file:
            identity_file.write(identity.model_dump_json(indent=2).encode() + b"\n")
    except (OSError, ValidationError, ValueError) as error:
        raise click.ClickException(str(error)) from error
    click.echo(str(output))


@qualification_cmd.command(name="collect-runtime")
@click.option("--request", type=click.Path(exists=True, dir_okay=False, path_type=Path), required=True)
@click.option("--output", type=click.Path(dir_okay=False, path_type=Path), required=True)
def qualification_collect_runtime(request: Path, output: Path) -> None:
    """Collect public runtime identity evidence from explicit local inputs."""
    try:
        collection = RuntimeCollectionRequest.model_validate_json(request.read_bytes())
        evidence = collect_runtime_identity(collection, request.parent)
        with output.open("xb") as evidence_file:
            evidence_file.write(evidence.model_dump_json(indent=2).encode() + b"\n")
    except (OSError, ValidationError, ValueError) as error:
        raise click.ClickException(str(error)) from error
    click.echo(str(output))


@qualification_cmd.command(name="run-gate")
@click.option("--candidate", type=click.Path(exists=True, dir_okay=False, path_type=Path), required=True)
@click.option("--gate-id", required=True)
@click.option("--tool-version", multiple=True, required=True)
@click.option("--output", type=click.Path(dir_okay=False, path_type=Path), required=True)
@click.argument("command", nargs=-1, type=click.UNPROCESSED)
def qualification_run_gate(
    candidate: Path,
    gate_id: str,
    tool_version: tuple[str, ...],
    output: Path,
    command: tuple[str, ...],
) -> None:
    """Run one deterministic gate and emit candidate-bound evidence."""
    if not command:
        raise click.UsageError("gate command is required after --")
    versions: dict[str, str] = {}
    for item in tool_version:
        name, separator, version = item.partition("=")
        if not separator or not name or not version or name in versions:
            raise click.UsageError("--tool-version values must be unique name=version pairs")
        versions[name] = version
    try:
        identity = CandidateIdentityEvidence.model_validate_json(candidate.read_bytes())
        started_at = datetime.now(UTC)
        result = subprocess.run(command, check=False, capture_output=True)
        completed_at = datetime.now(UTC)
        evidence = GateResultEvidence.build(
            gate_id=gate_id,
            candidate_content_hash=identity.content_hash,
            command=list(command),
            started_at=started_at,
            completed_at=completed_at,
            exit_code=result.returncode,
            stdout_sha256=hashlib.sha256(result.stdout).hexdigest(),
            stderr_sha256=hashlib.sha256(result.stderr).hexdigest(),
            tool_versions=versions,
            skipped=[],
        )
        with output.open("xb") as evidence_file:
            evidence_file.write(evidence.model_dump_json(indent=2).encode() + b"\n")
    except (OSError, ValidationError, ValueError) as error:
        raise click.ClickException(str(error)) from error
    if result.returncode != 0:
        raise click.ClickException(f"qualification gate exited with status {result.returncode}")
    click.echo(str(output))


@qualification_cmd.command(name="build")
@click.option("--input", "input_path", type=click.Path(exists=True, dir_okay=False, path_type=Path), required=True)
@click.option("--output", type=click.Path(dir_okay=False, path_type=Path))
@click.option("--check", type=click.Path(exists=True, dir_okay=False, path_type=Path))
def qualification_build(input_path: Path, output: Path | None, check: Path | None) -> None:
    """Build an owner-review-required draft without approval or lease issuance."""
    if (output is None) == (check is None):
        raise click.UsageError("exactly one of --output or --check is required")
    try:
        request = QualificationBuildRequest.model_validate_json(input_path.read_bytes())
        inputs = resolve_qualification_input(request, input_path.parent)
        rendered = render_qualification_json(build_collection_candidate_qualification(inputs))
        if check is not None:
            if check.read_bytes() != rendered:
                raise click.ClickException("qualification draft does not reproduce")
            click.echo("qualification draft reproduces")
            return
        if output is None:
            raise click.ClickException("qualification output path is missing")
        with output.open("xb") as draft:
            draft.write(rendered)
    except (OSError, ValidationError, ValueError) as error:
        raise click.ClickException(str(error)) from error
    click.echo(str(output))


__all__ = ["qualification_cmd"]
