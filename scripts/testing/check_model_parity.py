#!/usr/bin/env python3
import re
import sys
import os

# Paths to the model files relative to the script directory
SCRIPT_DIR = os.path.dirname(os.path.abspath(__file__))
PROJECT_ROOT = os.path.abspath(os.path.join(SCRIPT_DIR, "../.."))
PYTHON_MODEL_PATH = os.path.join(PROJECT_ROOT, "components/g8ee/app/models/http_context.py")
JS_MODEL_PATH = os.path.join(PROJECT_ROOT, "components/g8ed/models/request_models.js")

def extract_python_fields(class_name, content):
    pattern = r"class " + re.escape(class_name) + r"\(G8eBaseModel\):\n(.*?)(?=\nclass|\Z)"
    match = re.search(pattern, content, re.DOTALL)
    if not match:
        return set()
    
    body = match.group(1)
    fields = set()
    for line in body.split("\n"):
        line = line.strip()
        if not line or line.startswith("#") or line.startswith("@"):
            continue
        # Support both 'field: type = Field(...)' and 'field = Field(...)'
        field_match = re.match(r"^(\w+)(?::\s*.*?)?\s*=\s*Field\(", line)
        if field_match:
            fields.add(field_match.group(1))
    return fields

def extract_js_fields(class_name, content):
    pattern = r"export class " + re.escape(class_name) + r" extends G8eBaseModel \{\n\s*static fields = \{(.*?)\};"
    match = re.search(pattern, content, re.DOTALL)
    if not match:
        return set()
    
    body = match.group(1)
    fields = set()
    for line in body.split("\n"):
        line = line.strip()
        if not line or line.startswith("//"):
            continue
        field_match = re.match(r"^(\w+):\s*\{", line)
        if field_match:
            fields.add(field_match.group(1))
    return fields

def main():
    if not os.path.exists(PYTHON_MODEL_PATH):
        print(f"Error: Python model file not found at {PYTHON_MODEL_PATH}")
        sys.exit(1)
    if not os.path.exists(JS_MODEL_PATH):
        print(f"Error: JS model file not found at {JS_MODEL_PATH}")
        sys.exit(1)

    with open(PYTHON_MODEL_PATH, "r") as f:
        py_content = f.read()
    
    with open(JS_MODEL_PATH, "r") as f:
        js_content = f.read()

    # Models to compare
    models_to_check = [
        ("G8eHttpContext", "G8eHttpContext"),
        ("BoundOperator", "BoundOperatorContext"),
    ]

    mismatches = []

    for py_class, js_class in models_to_check:
        py_fields = extract_python_fields(py_class, py_content)
        js_fields = extract_js_fields(js_class, js_content)

        if not py_fields:
            print(f"Warning: No fields found for Python class {py_class}")
        if not js_fields:
            print(f"Warning: No fields found for JS class {js_class}")

        if py_fields != js_fields:
            only_in_py = py_fields - js_fields
            only_in_js = js_fields - py_fields
            
            mismatch_info = f"Model Mismatch: {py_class} (Py) vs {js_class} (JS)\n"
            if only_in_py:
                mismatch_info += f"  - Fields only in Python: {only_in_py}\n"
            if only_in_js:
                mismatch_info += f"  - Fields only in JS: {only_in_js}\n"
            mismatches.append(mismatch_info)

    if mismatches:
        print("\n".join(mismatches))
        print("\nFAILURE: Models are out of sync!")
        sys.exit(1)
    else:
        print("SUCCESS: Models are in sync.")
        sys.exit(0)

if __name__ == "__main__":
    main()
