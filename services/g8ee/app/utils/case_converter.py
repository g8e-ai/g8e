# Copyright (c) 2026 Lateralus Labs, LLC.
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

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
            result[snake_key] = [convert_dict_keys_to_snake_case(item) if isinstance(item, dict) else item for item in value]
        else:
            result[snake_key] = value

    return result
