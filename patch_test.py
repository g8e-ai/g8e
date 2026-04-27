with open('/home/bob/g8e/components/g8ee/tests/unit/security/test_sentinel_scrubber.py', 'r') as f:
    content = f.read()

new_test = """
    def test_ast_contract_no_placeholder_cannibalization(self):
        \"\"\"
        [Security] AST Contract Test
        Enforce that no RegexScrubber pattern uses vulnerable gap-matching (like `.{0,20}`)
        which could cannibalize already-inserted placeholders like `[AWS_KEY]`.
        They must use `[^\\[\\]]{0,20}` instead.
        \"\"\"
        import ast
        from pathlib import Path
        import app.security.sentinel_scrubber as ss_module
        
        filepath = Path(ss_module.__file__)
        with open(filepath, "r") as f:
            tree = ast.parse(f.read())
            
        for node in ast.walk(tree):
            if isinstance(node, ast.Call) and getattr(node.func, "id", "") == "RegexScrubber":
                if len(node.args) >= 2 and isinstance(node.args[1], ast.Constant) and isinstance(node.args[1].value, str):
                    pattern_str = node.args[1].value
                    if "{0," in pattern_str:
                        assert r"[^\\[\\]]" in pattern_str, (
                            f"Scrubber pattern {pattern_str!r} uses gap matching but lacks "
                            f"placeholder protection ([^\\[\\]]). This causes cannibalization."
                        )
                        assert r".{0," not in pattern_str, (
                            f"Scrubber pattern {pattern_str!r} uses dangerous `.` gap matching."
                        )
"""

if 'test_ast_contract_no_placeholder_cannibalization' not in content:
    content = content.replace('def test_placeholder_not_cannibalized_by_later_contextual_scrubber(self):', new_test.lstrip() + '\n    def test_placeholder_not_cannibalized_by_later_contextual_scrubber(self):')
    
    with open('/home/bob/g8e/components/g8ee/tests/unit/security/test_sentinel_scrubber.py', 'w') as f:
        f.write(content)
