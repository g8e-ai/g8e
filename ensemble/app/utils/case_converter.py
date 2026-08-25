# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

import re


def to_snake_case(string: str) -> str:
    return re.sub(r"(?<!^)(?=[A-Z])", "_", string).lower()


def to_camel_case(string: str) -> str:
    words = string.split("_")
    return words[0] + "".join(word.capitalize() for word in words[1:])


def convert_dict_keys_to_snake_case(d: dict[str, object]) -> dict[str, object]:
    if not isinstance(d, dict):
        return d

    result = {}
    for key, value in d.items():
        snake_key = to_snake_case(key)

        if isinstance(value, dict):
            result[snake_key] = convert_dict_keys_to_snake_case(value)
        elif isinstance(value, list):
            result[snake_key] = [
                convert_dict_keys_to_snake_case(item) if isinstance(item, dict) else item
                for item in value
            ]
        else:
            result[snake_key] = value

    return result
