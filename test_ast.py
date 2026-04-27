import ast
from pathlib import Path

filepath = Path("/home/bob/g8e/components/g8ee/app/security/sentinel_scrubber.py")
with open(filepath, "r") as f:
    tree = ast.parse(f.read())

for node in ast.walk(tree):
    if isinstance(node, ast.Call) and getattr(node.func, "id", "") == "RegexScrubber":
        pattern_arg = node.args[1]
        if isinstance(pattern_arg, ast.Constant) and isinstance(pattern_arg.value, str):
            pattern_str = pattern_arg.value
            if "{0," in pattern_str and "{" not in pattern_str.split("{0,")[0]: # naive check
                # we just check if it's using . instead of [^\[\]]
                # specifically, we want to forbid `.{0,20}` or `.?` that can eat `[`
                if r".{0," in pattern_str:
                    print(f"FAILED: {pattern_str}")
                elif r"[^\[\]]" not in pattern_str:
                    print(f"FAILED (missing placeholder protection): {pattern_str}")
                else:
                    print(f"PASSED: {pattern_str}")
