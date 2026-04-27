import json
import glob
import os

schema_path = "/home/bob/g8e/shared/test-fixtures/gold-set-schema.json"
with open(schema_path, "r") as f:
    schema = json.load(f)

schema["items"]["properties"]["expected_tools"]["items"]["enum"] = []
schema["items"]["properties"]["expected_tool"]["enum"] = []
schema["items"]["properties"]["forbidden_tools"]["items"]["enum"] = []

# Get all valid tool names
valid_tools = []
tools_dir = "/home/bob/g8e/components/g8ee/app/services/ai/tools"
if os.path.exists(tools_dir):
    for root, _, files in os.walk(tools_dir):
        for file in files:
            if file.endswith(".py") and not file.startswith("__"):
                with open(os.path.join(root, file)) as f:
                    content = f.read()
                    if "name=" in content and "description=" in content:
                        import re
                        matches = re.findall(r'name=["\']([^"\']+)["\']', content)
                        valid_tools.extend(matches)

# Since we don't have a reliable way to parse them all easily (some might be defined differently), 
# let's just create a test that verifies the gold sets against the actual tool registry in g8ee.
