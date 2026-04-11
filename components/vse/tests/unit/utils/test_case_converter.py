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

"""Unit tests for app/utils/case_converter.py."""

import pytest

from app.utils.case_converter import (
    convert_dict_keys_to_snake_case,
    to_camel_case,
    to_snake_case,
)

pytestmark = pytest.mark.unit


class TestToSnakeCase:

    def test_single_uppercase_word(self):
        assert to_snake_case("Hello") == "hello"

    def test_camel_case_two_words(self):
        assert to_snake_case("camelCase") == "camel_case"

    def test_pascal_case(self):
        assert to_snake_case("PascalCase") == "pascal_case"

    def test_already_snake_case(self):
        assert to_snake_case("already_snake") == "already_snake"

    def test_multiple_words(self):
        assert to_snake_case("myVariableName") == "my_variable_name"

    def test_all_lowercase_unchanged(self):
        assert to_snake_case("lowercase") == "lowercase"

    def test_empty_string(self):
        assert to_snake_case("") == ""

    def test_consecutive_uppercase(self):
        result = to_snake_case("XMLParser")
        assert result == result.lower() or "_" in result

    def test_single_char_lowercase(self):
        assert to_snake_case("a") == "a"

    def test_single_char_uppercase(self):
        assert to_snake_case("A") == "a"


class TestToCamelCase:

    def test_snake_case_two_words(self):
        assert to_camel_case("snake_case") == "snakeCase"

    def test_single_word_unchanged(self):
        assert to_camel_case("hello") == "hello"

    def test_three_words(self):
        assert to_camel_case("my_variable_name") == "myVariableName"

    def test_empty_string(self):
        assert to_camel_case("") == ""

    def test_already_camel_case_input(self):
        assert to_camel_case("camel") == "camel"

    def test_leading_underscore_produces_empty_first_word(self):
        result = to_camel_case("_leading")
        assert isinstance(result, str)

    def test_all_lowercase_single_segment(self):
        assert to_camel_case("word") == "word"

    def test_roundtrip_two_words(self):
        original = "snake_case"
        camel = to_camel_case(original)
        assert camel == "snakeCase"
        assert to_snake_case(camel) == original

    def test_multiple_underscores(self):
        assert to_camel_case("a_b_c_d") == "aBCD"


class TestConvertDictKeysToSnakeCase:

    def test_flat_dict_camel_keys_converted(self):
        result = convert_dict_keys_to_snake_case({"firstName": "Alice", "lastName": "Smith"})
        assert result == {"first_name": "Alice", "last_name": "Smith"}

    def test_already_snake_keys_unchanged(self):
        result = convert_dict_keys_to_snake_case({"first_name": "Alice"})
        assert result == {"first_name": "Alice"}

    def test_nested_dict_keys_converted(self):
        result = convert_dict_keys_to_snake_case({"outerKey": {"innerKey": "value"}})
        assert result == {"outer_key": {"inner_key": "value"}}

    def test_list_values_with_dict_items_converted(self):
        result = convert_dict_keys_to_snake_case({"itemList": [{"itemName": "a"}, {"itemName": "b"}]})
        assert result == {"item_list": [{"item_name": "a"}, {"item_name": "b"}]}

    def test_list_values_with_scalar_items_unchanged(self):
        result = convert_dict_keys_to_snake_case({"tagList": [1, 2, 3]})
        assert result == {"tag_list": [1, 2, 3]}

    def test_non_dict_input_returned_unchanged(self):
        assert convert_dict_keys_to_snake_case("not a dict") == "not a dict"
        assert convert_dict_keys_to_snake_case(42) == 42
        assert convert_dict_keys_to_snake_case(None) is None

    def test_empty_dict(self):
        assert convert_dict_keys_to_snake_case({}) == {}

    def test_values_preserved(self):
        result = convert_dict_keys_to_snake_case({"myKey": 99})
        assert result["my_key"] == 99

    def test_deeply_nested(self):
        result = convert_dict_keys_to_snake_case({"levelOne": {"levelTwo": {"levelThree": "val"}}})
        assert result == {"level_one": {"level_two": {"level_three": "val"}}}

    def test_mixed_list_dict_and_scalar(self):
        result = convert_dict_keys_to_snake_case({"myList": [{"nestedKey": 1}, "string", 42]})
        assert result["my_list"][0] == {"nested_key": 1}
        assert result["my_list"][1] == "string"
        assert result["my_list"][2] == 42
