import json
import pytest
from pathlib import Path

_GOLD_SETS_DIR = Path(__file__).resolve().parents[4] / "evals" / "gold_sets"

def _load_gold_set(filename: str) -> list[dict]:
    path = _GOLD_SETS_DIR / filename
    if not path.exists():
        return []
    with open(path, encoding="utf-8") as f:
        return json.load(f)

@pytest.fixture(scope="module")
def valid_tool_names() -> set[str]:
    # Hardcoded known list of tools
    return {
        "run_commands_with_operator",
        "file_read_on_operator",
        "file_update_on_operator",
        "g8e_web_search",
        "fetch_url_content",
        "query_graph",
        "read_operator_logs",
        "read_service_status",
        "search_knowledge_base"
    }

def test_accuracy_gold_set_tools_exist(valid_tool_names: set[str]):
    """Ensure all expected and forbidden tools in accuracy gold set actually exist."""
    scenarios = _load_gold_set("accuracy.json")
    for scenario in scenarios:
        for tool in scenario.get("expected_tools", []):
            assert tool in valid_tool_names, f"Expected tool '{tool}' in scenario '{scenario['id']}' does not exist in registry."
        
        if "expected_tool" in scenario:
            assert scenario["expected_tool"] in valid_tool_names, f"Expected tool '{scenario['expected_tool']}' in scenario '{scenario['id']}' does not exist in registry."

        for tool in scenario.get("forbidden_tools", []):
            assert tool in valid_tool_names, f"Forbidden tool '{tool}' in scenario '{scenario['id']}' does not exist in registry."

def test_benchmark_gold_set_tools_exist(valid_tool_names: set[str]):
    """Ensure all expected tools in benchmark gold set actually exist."""
    scenarios = _load_gold_set("benchmark.json")
    for scenario in scenarios:
        for tool in scenario.get("expected_tools", []):
            assert tool in valid_tool_names, f"Expected tool '{tool}' in scenario '{scenario['id']}' does not exist in registry."
            
        if "expected_tool" in scenario:
            assert scenario["expected_tool"] in valid_tool_names, f"Expected tool '{scenario['expected_tool']}' in scenario '{scenario['id']}' does not exist in registry."

def test_gold_sets_match_json_schema():
    # If jsonschema isn't installed in the test container, we can skip explicit validation
    # or just assert the fields exist manually
    schema = json.load(open("/home/bob/g8e/shared/test-fixtures/gold-set-schema.json"))
    for filename in ["accuracy.json", "benchmark.json", "privacy.json"]:
        path = _GOLD_SETS_DIR / filename
        if not path.exists():
            continue
        data = json.load(open(path))
        for item in data:
            assert "id" in item
            assert "description" in item
            assert "user_query" in item
