#!/usr/bin/env python3
"""
Unit tests for Gitleaks doctrine ingestion script.
"""

import json
import tempfile
from pathlib import Path

import pytest

# Add parent directory to path for imports
import sys
sys.path.insert(0, str(Path(__file__).parent.parent))

from ingest_gitleaks import GitleaksParser, Doctrine


@pytest.fixture
def sample_gitleaks_toml():
    """Sample Gitleaks TOML configuration."""
    return """
title = "Gitleaks Configuration"

[[rules]]
id = "aws-access-key-id"
description = "AWS Access Key ID"
regex = '''(A3T[A-Z0-9]|AKIA|AGPA|AIDA|AROA|AIPA|ANPA|ANVA|ASIA)[A-Z0-9]{16}'''
severity = "critical"

[[rules]]
id = "github-token"
description = "GitHub Personal Access Token"
regex = '''ghp_[a-zA-Z0-9]{36}'''
severity = "critical"

[[rules]]
id = "slack-token"
description = "Slack Token"
regex = '''xox[baprs]-[a-zA-Z0-9-]+'''
severity = "high"

[[rules]]
id = "generic-api-key"
description = "Generic API Key"
regex = '''(?i)api[_-]?key\\s*["']?[=:]\\s*["']?[a-zA-Z0-9_\\-]{32,}'''
severity = "medium"
entropy = "3.5-4.5"
"""


@pytest.fixture
def parser():
    """Gitleaks parser instance."""
    return GitleaksParser()


def test_parse_single_rule(parser, sample_gitleaks_toml):
    """Test parsing a single Gitleaks rule."""
    with tempfile.NamedTemporaryFile(mode='w', suffix='.toml', delete=False) as f:
        f.write(sample_gitleaks_toml)
        f.flush()
        toml_path = Path(f.name)
    
    try:
        doctrines = parser.parse_file(toml_path)
        
        assert len(doctrines) == 4
        
        # Check first doctrine (AWS Access Key)
        aws_doctrine = doctrines[0]
        assert aws_doctrine.id == "gitleaks_aws-access-key-id"
        assert aws_doctrine.name == "AWS Access Key ID"
        assert aws_doctrine.category == "credential_access"
        assert aws_doctrine.severity == "critical"
        assert aws_doctrine.mitre_attack == "T1055.001"
        assert aws_doctrine.mitre_tactic == "Credential Access"
        assert aws_doctrine.confidence == 0.95
        assert aws_doctrine.enabled is True
        
        # Check second doctrine (GitHub Token)
        github_doctrine = doctrines[1]
        assert github_doctrine.id == "gitleaks_github-token"
        assert github_doctrine.name == "GitHub Personal Access Token"
        
        # Check third doctrine (Slack Token)
        slack_doctrine = doctrines[2]
        assert slack_doctrine.id == "gitleaks_slack-token"
        assert slack_doctrine.severity == "critical"  # high maps to critical
        
        # Check fourth doctrine (Generic API Key)
        generic_doctrine = doctrines[3]
        assert generic_doctrine.id == "gitleaks_generic-api-key"
        assert generic_doctrine.severity == "high"  # medium maps to high
        assert generic_doctrine.confidence == 0.90  # entropy-based
    finally:
        toml_path.unlink()


def test_parse_invalid_rule(parser):
    """Test parsing invalid Gitleaks rule (missing ID)."""
    invalid_toml = """
[[rules]]
description = "Invalid Rule"
regex = '''test'''
severity = "high"
"""
    
    with tempfile.NamedTemporaryFile(mode='w', suffix='.toml', delete=False) as f:
        f.write(invalid_toml)
        f.flush()
        toml_path = Path(f.name)
    
    try:
        doctrines = parser.parse_file(toml_path)
        assert len(doctrines) == 0
    finally:
        toml_path.unlink()


def test_parse_rule_without_regex(parser):
    """Test parsing rule without regex pattern."""
    toml = """
[[rules]]
id = "no-regex"
description = "Rule without regex"
severity = "high"
"""
    
    with tempfile.NamedTemporaryFile(mode='w', suffix='.toml', delete=False) as f:
        f.write(toml)
        f.flush()
        toml_path = Path(f.name)
    
    try:
        doctrines = parser.parse_file(toml_path)
        assert len(doctrines) == 0
    finally:
        toml_path.unlink()


def test_severity_mapping(parser):
    """Test Gitleaks severity to g8e severity mapping."""
    toml = """
[[rules]]
id = "critical-rule"
description = "Critical"
regex = '''test'''
severity = "critical"

[[rules]]
id = "high-rule"
description = "High"
regex = '''test'''
severity = "high"

[[rules]]
id = "medium-rule"
description = "Medium"
regex = '''test'''
severity = "medium"

[[rules]]
id = "low-rule"
description = "Low"
regex = '''test'''
severity = "low"
"""
    
    with tempfile.NamedTemporaryFile(mode='w', suffix='.toml', delete=False) as f:
        f.write(toml)
        f.flush()
        toml_path = Path(f.name)
    
    try:
        doctrines = parser.parse_file(toml_path)
        assert len(doctrines) == 4
        
        severities = {d.id: d.severity for d in doctrines}
        assert severities["gitleaks_critical-rule"] == "critical"
        assert severities["gitleaks_high-rule"] == "critical"
        assert severities["gitleaks_medium-rule"] == "high"
        assert severities["gitleaks_low-rule"] == "medium"
    finally:
        toml_path.unlink()


def test_entropy_confidence_calculation(parser):
    """Test confidence calculation based on entropy."""
    toml = """
[[rules]]
id = "high-entropy"
description = "High Entropy"
regex = '''test'''
severity = "high"
entropy = "6.0-8.0"

[[rules]]
id = "medium-entropy"
description = "Medium Entropy"
regex = '''test'''
severity = "high"
entropy = "4.0-5.0"

[[rules]]
id = "low-entropy"
description = "Low Entropy"
regex = '''test'''
severity = "high"
entropy = "2.0-3.0"

[[rules]]
id = "no-entropy"
description = "No Entropy"
regex = '''test'''
severity = "high"
"""
    
    with tempfile.NamedTemporaryFile(mode='w', suffix='.toml', delete=False) as f:
        f.write(toml)
        f.flush()
        toml_path = Path(f.name)
    
    try:
        doctrines = parser.parse_file(toml_path)
        assert len(doctrines) == 4
        
        confidences = {d.id: d.confidence for d in doctrines}
        assert confidences["gitleaks_high-entropy"] == 0.95
        assert confidences["gitleaks_medium-entropy"] == 0.90
        assert confidences["gitleaks_low-entropy"] == 0.85
        assert confidences["gitleaks_no-entropy"] == 0.95  # default
    finally:
        toml_path.unlink()


def test_write_doctrine_file(parser):
    """Test writing doctrine file to JSON."""
    doctrines = [
        Doctrine(
            id="test_001",
            name="Test Doctrine",
            category="credential_access",
            severity="critical",
            pattern="test.*pattern",
            mitre_attack="T1055.001",
            mitre_tactic="Credential Access",
            confidence=0.95,
            enabled=True
        )
    ]
    
    with tempfile.NamedTemporaryFile(mode='w', suffix='.json', delete=False) as f:
        output_path = Path(f.name)
    
    try:
        parser.write_doctrine_file(
            doctrines,
            output_path,
            source="test",
            version="1.0.0",
            license_str="MIT"
        )
        
        # Verify output
        with open(output_path, 'r') as f:
            output = json.load(f)
        
        assert output["source"] == "test"
        assert output["version"] == "1.0.0"
        assert output["license"] == "MIT"
        assert len(output["doctrines"]) == 1
        assert output["doctrines"][0]["id"] == "test_001"
        assert output["doctrines"][0]["pattern"] == "test.*pattern"
    finally:
        output_path.unlink()


def test_pattern_preservation(parser):
    """Test that regex patterns are preserved correctly."""
    toml = """
[[rules]]
id = "complex-pattern"
description = "Complex Regex Pattern"
regex = '''(A3T[A-Z0-9]|AKIA|AGPA|AIDA|AROA|AIPA|ANPA|ANVA|ASIA)[A-Z0-9]{16}'''
severity = "critical"
"""
    
    with tempfile.NamedTemporaryFile(mode='w', suffix='.toml', delete=False) as f:
        f.write(toml)
        f.flush()
        toml_path = Path(f.name)
    
    try:
        doctrines = parser.parse_file(toml_path)
        assert len(doctrines) == 1
        assert doctrines[0].pattern == "(A3T[A-Z0-9]|AKIA|AGPA|AIDA|AROA|AIPA|ANPA|ANVA|ASIA)[A-Z0-9]{16}"
    finally:
        toml_path.unlink()


if __name__ == "__main__":
    pytest.main([__file__, "-v"])
