# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

# Agent constants - protocol sync removed, define locally if needed
from enum import StrEnum


class PersonaCapability(StrEnum):
    """Capabilities a persona declares it can exercise when the pipeline allows.

    Declared on ``AgentPersonaModel.capabilities``. A declaration is an upper
    bound, not a grant: the pipeline intersects it with user settings and with
    the resolved model's supported features before enabling the behavior.
    """

    LOCAL_SYNTAX_CHECK = "local_syntax_check"


__all__ = ["PersonaCapability"]
