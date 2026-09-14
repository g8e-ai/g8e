from __future__ import annotations

from pathlib import Path

import click
from pydantic import ValidationError

from g8e_evals.qualification import (
    QualificationBuildRequest,
    build_collection_candidate_qualification,
    render_qualification_json,
    resolve_qualification_input,
)


@click.group(name="qualification")
def qualification_cmd() -> None:
    """Build deterministic collection-candidate qualification drafts."""


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
