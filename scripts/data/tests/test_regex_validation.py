#!/usr/bin/env python3
"""
Unit tests for regex complexity validation.
"""

# Add parent directory to path for imports
import sys
from pathlib import Path
sys.path.insert(0, str(Path(__file__).parent.parent))

from _lib import RegexValidationError, validate_regex_complexity

try:
    import pytest
    HAS_PYTEST = True
except ImportError:
    HAS_PYTEST = False


def test_safe_pattern_passes():
    """Test that safe regex patterns pass validation."""
    safe_patterns = [
        r'\b[A-Z0-9]{16}\b',
        r'ghp_[a-zA-Z0-9]{36}',
        r'(?i)api[_-]?key',
        r'\d{3}-\d{2}-\d{4}',
        r'[a-z0-9._%+-]+@[a-z0-9.-]+\.[a-z]{2,}',
    ]
    
    for pattern in safe_patterns:
        validate_regex_complexity(pattern)


def test_nested_quantifiers_rejected():
    """Test that nested quantifiers are rejected."""
    dangerous_patterns = [
        r'(a+)+',
        r'(a*)*',
        r'(a?)+',
        r'([a-z]+)+',
    ]

    for pattern in dangerous_patterns:
        if HAS_PYTEST:
            with pytest.raises(RegexValidationError, match="nested quantifiers"):
                validate_regex_complexity(pattern)
        else:
            try:
                validate_regex_complexity(pattern)
                raise AssertionError(f"Should have rejected nested quantifiers: {pattern}")
            except RegexValidationError:
                pass


def test_repeated_quantifiers_rejected():
    """Test that repeated quantifiers on groups are rejected."""
    dangerous_patterns = [
        r'(\w+)\s*\+',
        r'([a-z]*)\s*?',
    ]

    for pattern in dangerous_patterns:
        if HAS_PYTEST:
            with pytest.raises(RegexValidationError, match="repeated quantifiers"):
                validate_regex_complexity(pattern)
        else:
            try:
                validate_regex_complexity(pattern)
                raise AssertionError(f"Should have rejected repeated quantifiers: {pattern}")
            except RegexValidationError:
                pass


def test_excessive_alternation_rejected():
    """Test that excessive alternation is rejected."""
    pattern = 'a|b|c|d|e|f|g|h|i|j|k|l|m|n|o|p|q|r|s|t|u|v|w|x|y|z'

    if HAS_PYTEST:
        with pytest.raises(RegexValidationError, match="excessive alternations"):
            validate_regex_complexity(pattern)
    else:
        try:
            validate_regex_complexity(pattern)
            raise AssertionError("Should have rejected excessive alternation")
        except RegexValidationError:
            pass


def test_large_char_class_with_large_quantifier_rejected():
    """Test that large character classes with large quantifiers are rejected."""
    dangerous_patterns = [
        r'[a-zA-Z0-9]{100,}',
        r'[a-zA-Z0-9_\-\.]{500}',
    ]

    for pattern in dangerous_patterns:
        if HAS_PYTEST:
            with pytest.raises(RegexValidationError, match="large character class"):
                validate_regex_complexity(pattern)
        else:
            try:
                validate_regex_complexity(pattern)
                raise AssertionError(f"Should have rejected large char class: {pattern}")
            except RegexValidationError:
                pass


def test_overlapping_branches_rejected():
    """Test that overlapping alternation branches are rejected."""
    dangerous_patterns = [
        r'(a|ab)+',
        r'(test|testing)+',
        r'(abc|abcd)+',
    ]

    for pattern in dangerous_patterns:
        if HAS_PYTEST:
            with pytest.raises(RegexValidationError, match="overlapping alternation branches"):
                validate_regex_complexity(pattern)
        else:
            try:
                validate_regex_complexity(pattern)
                raise AssertionError(f"Should have rejected overlapping branches: {pattern}")
            except RegexValidationError:
                pass


def test_invalid_regex_syntax_rejected():
    """Test that invalid regex syntax is rejected."""
    invalid_patterns = [
        r'[unclosed',
        r'(unclosed',
        r'*invalid',
    ]

    for pattern in invalid_patterns:
        if HAS_PYTEST:
            with pytest.raises(RegexValidationError, match="Invalid regex syntax"):
                validate_regex_complexity(pattern)
        else:
            try:
                validate_regex_complexity(pattern)
                raise AssertionError(f"Should have rejected invalid regex: {pattern}")
            except RegexValidationError:
                pass


def test_custom_alternation_limit():
    """Test custom alternation limit."""
    pattern = 'a|b|c|d|e'

    # Should pass with default limit
    validate_regex_complexity(pattern)

    # Should fail with lower limit
    if HAS_PYTEST:
        with pytest.raises(RegexValidationError, match="excessive alternations"):
            validate_regex_complexity(pattern, max_alternations=3)
    else:
        try:
            validate_regex_complexity(pattern, max_alternations=3)
            raise AssertionError("Should have rejected excessive alternations")
        except RegexValidationError:
            pass


def test_realistic_gitleaks_patterns_pass():
    """Test that realistic Gitleaks patterns pass validation."""
    realistic_patterns = [
        r'(A3T[A-Z0-9]|AKIA|AGPA|AIDA|AROA|AIPA|ANPA|ANVA|ASIA)[A-Z0-9]{16}',
        r'ghp_[a-zA-Z0-9]{36}',
        r'xox[baprs]-[a-zA-Z0-9-]+',
        r'(?i)api[_-]?key\s*["\']?[=:]\s*["\']?[a-zA-Z0-9_\-]{32,}',
    ]
    
    for pattern in realistic_patterns:
        validate_regex_complexity(pattern)


def test_realistic_owasp_crs_patterns_pass():
    """Test that realistic OWASP CRS patterns pass validation."""
    realistic_patterns = [
        r'(?i)nc\s+.*-e\s+(/bin/)?(sh|bash|zsh)',
        r'(?i)\.\.[\/\\]',
        r'(?i)\bUNION\s+SELECT\b',
        r'(?i)<script[^>]*>.*?</script>',
    ]
    
    for pattern in realistic_patterns:
        validate_regex_complexity(pattern)


if __name__ == "__main__":
    # Standalone test runner for environments without pytest
    import sys
    
    test_count = 0
    passed_count = 0
    failed_count = 0
    
    def run_test(name, func):
        global test_count, passed_count, failed_count
        test_count += 1
        try:
            func()
            passed_count += 1
            print(f"✓ {name}")
        except Exception as e:
            failed_count += 1
            print(f"✗ {name}: {e}")
    
    def test_safe_pattern_passes():
        safe_patterns = [
            r'\b[A-Z0-9]{16}\b',
            r'ghp_[a-zA-Z0-9]{36}',
            r'(?i)api[_-]?key',
            r'\d{3}-\d{2}-\d{4}',
        ]
        for pattern in safe_patterns:
            validate_regex_complexity(pattern)
    
    def test_nested_quantifiers_rejected():
        try:
            validate_regex_complexity(r'(a+)+')
            raise AssertionError("Should have rejected nested quantifiers")
        except RegexValidationError:
            pass
    
    def test_excessive_alternation_rejected():
        pattern = 'a|b|c|d|e|f|g|h|i|j|k|l|m|n|o|p|q|r|s|t|u|v|w|x|y|z'
        try:
            validate_regex_complexity(pattern)
            raise AssertionError("Should have rejected excessive alternation")
        except RegexValidationError:
            pass
    
    def test_invalid_regex_rejected():
        try:
            validate_regex_complexity(r'[unclosed')
            raise AssertionError("Should have rejected invalid regex")
        except RegexValidationError:
            pass
    
    run_test("Safe patterns pass", test_safe_pattern_passes)
    run_test("Nested quantifiers rejected", test_nested_quantifiers_rejected)
    run_test("Excessive alternation rejected", test_excessive_alternation_rejected)
    run_test("Invalid regex rejected", test_invalid_regex_rejected)
    
    print(f"\n{passed_count}/{test_count} tests passed")
    sys.exit(0 if failed_count == 0 else 1)
