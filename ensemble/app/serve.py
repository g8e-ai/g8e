# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""Ensemble launcher: explicit arguments become typed bootstrap settings.

``python -m app.serve`` is the container entrypoint. Platform configuration is
passed as checked-in command arguments (the same pattern the Go binary uses),
not through environment variables (INV-ENV-04). Every argument is optional and
defaults to the in-code default.
"""

import argparse
import sys
from collections.abc import Sequence
from dataclasses import dataclass

import uvicorn

from app.constants.bootstrap import (
    BootstrapSettings,
    configure_bootstrap,
    resolve_gateway_dial_urls,
)

DEFAULT_HOST = "0.0.0.0"
DEFAULT_PORT = 8000
APP_IMPORT_STRING = "app.main:app"


@dataclass(frozen=True)
class ServeArgs:
    """Parsed launcher arguments."""

    bootstrap: BootstrapSettings
    host: str
    port: int


def _build_parser() -> argparse.ArgumentParser:
    parser = argparse.ArgumentParser(prog="python -m app.serve", description=__doc__)
    parser.add_argument("--host", default=DEFAULT_HOST, help="Interface the API binds to")
    parser.add_argument("--port", type=int, default=DEFAULT_PORT, help="Port the API binds to")
    parser.add_argument(
        "--gateway-http-url", help="Gateway plain-HTTP bootstrap URL used for enrollment"
    )
    parser.add_argument(
        "--gateway-url", help="Gateway HTTPS base URL used by the internal HTTP client"
    )
    parser.add_argument(
        "--gateway-https-url", help="Gateway HTTPS URL for operator and app endpoints"
    )
    parser.add_argument("--gateway-pubsub-url", help="Gateway WebSocket pub/sub URL")
    parser.add_argument("--runtime-dir", help="Runtime (.g8e) directory")
    parser.add_argument("--pki-dir", help="PKI directory (default: <runtime-dir>/pki)")
    parser.add_argument(
        "--ca-cert-path",
        help="Gateway trust bundle path (default: <pki-dir>/trust/g8eg-ca-bundle.pem)",
    )
    return parser


def parse_args(argv: Sequence[str]) -> ServeArgs:
    """Parse launcher arguments. Exits with status 2 on an unknown argument."""
    ns = _build_parser().parse_args(list(argv))
    return ServeArgs(
        bootstrap=BootstrapSettings(
            gateway_http_url=ns.gateway_http_url,
            gateway_url=ns.gateway_url,
            gateway_https_url=ns.gateway_https_url,
            gateway_pubsub_url=ns.gateway_pubsub_url,
            runtime_dir=ns.runtime_dir,
            pki_dir=ns.pki_dir,
            ca_cert_path=ns.ca_cert_path,
        ),
        host=ns.host,
        port=ns.port,
    )


def main(argv: Sequence[str] | None = None) -> None:
    """Install bootstrap settings, then start the API server."""
    args = parse_args(sys.argv[1:] if argv is None else argv)
    # Bootstrap must be installed before uvicorn imports the application.
    configure_bootstrap(resolve_gateway_dial_urls(args.bootstrap))
    uvicorn.run(APP_IMPORT_STRING, host=args.host, port=args.port)


if __name__ == "__main__":
    main()
