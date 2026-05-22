#!/usr/bin/env python3
"""
Unit tests for OWASP CRS doctrine ingestion script.
"""

import json
import tempfile
from pathlib import Path
from unittest.mock import patch

import pytest

# Add parent directory to path for imports
import sys
sys.path.insert(0, str(Path(__file__).parent.parent))

from ingest_owasp_crs import CRSParser, Doctrine


@pytest.fixture
def sample_crs_conf():
    """Sample OWASP CRS rule content."""
    return """
SecRule REQUEST_URI "@rx (?i)nc\\s+.*-e\\s+(/bin/)?(sh|bash|zsh)" \\
    "id:932100,phase:1,log,deny,msg:'RCE: nc -e reverse shell',severity:'CRITICAL',t:none"

SecRule ARGS "@rx (?i)\\.\\.[\\/\\\\]" \\
    "id:930100,phase:2,log,deny,msg:'LFI: path traversal',severity:'ERROR',t:none"

SecRule REQUEST_BODY "@rx (?i)\\bUNION\\s+SELECT\\b" \\
    "id:942100,phase:1,log,deny,msg:'SQLi: UNION SELECT',severity:'WARNING',t:none"
"""


@pytest.fixture
def parser():
    """CRS parser instance."""
    return CRSParser()


def test_parse_single_rule(parser, sample_crs_conf):
    """Test parsing a single CRS rule."""
    with tempfile.NamedTemporaryFile(mode='w', suffix='.conf', delete=False) as f:
        f.write(sample_crs_conf)
        f.flush()
        conf_path = Path(f.name)
    
    try:
        doctrines = parser.parse_file(conf_path)
        
        assert len(doctrines) == 3
        
        # Check first doctrine (RCE)
        rce_doctrine = doctrines[0]
        assert rce_doctrine.id == "owasp_crs_932100"
        assert rce_doctrine.name == "RCE: nc -e reverse shell"
        assert rce_doctrine.category == "reverse_shell"
        assert rce_doctrine.severity == "critical"
        assert rce_doctrine.mitre_attack == "T1059.004"
        assert rce_doctrine.mitre_tactic == "Execution"
        assert rce_doctrine.confidence == 0.90
        assert rce_doctrine.enabled is True
        
        # Check second doctrine (LFI)
        lfi_doctrine = doctrines[1]
        assert lfi_doctrine.id == "owasp_crs_930100"
        assert lfi_doctrine.category == "path_traversal"
        assert lfi_doctrine.mitre_attack == "T1006"
        
        # Check third doctrine (SQLi)
        sqli_doctrine = doctrines[2]
        assert sqli_doctrine.id == "owasp_crs_942100"
        assert sqli_doctrine.category == "sql_injection"
        assert sqli_doctrine.severity == "high"
    finally:
        conf_path.unlink()


def test_parse_invalid_rule(parser):
    """Test parsing invalid CRS rule (missing ID)."""
    invalid_conf = """
SecRule REQUEST_URI "@rx (?i)test" \\
    "phase:1,log,deny,msg:'Test rule'"
"""
    
    with tempfile.NamedTemporaryFile(mode='w', suffix='.conf', delete=False) as f:
        f.write(invalid_conf)
        f.flush()
        conf_path = Path(f.name)
    
    try:
        doctrines = parser.parse_file(conf_path)
        assert len(doctrines) == 0
    finally:
        conf_path.unlink()


def test_pattern_cleanup(parser):
    """Test pattern cleanup (removing SecLanguage escapes)."""
    conf = """
SecRule REQUEST_URI "@rx (?i)test\\.pattern\\*" \\
    "id:999999,phase:1,log,deny,msg:'Test',severity:'INFO'"
"""
    
    with tempfile.NamedTemporaryFile(mode='w', suffix='.conf', delete=False) as f:
        f.write(conf)
        f.flush()
        conf_path = Path(f.name)
    
    try:
        doctrines = parser.parse_file(conf_path)
        assert len(doctrines) == 1
        assert doctrines[0].pattern == "(?i)test.pattern*"
    finally:
        conf_path.unlink()


def test_write_doctrine_file(parser):
    """Test writing doctrine file to JSON."""
    doctrines = [
        Doctrine(
            id="test_001",
            name="Test Doctrine",
            category="test",
            severity="critical",
            pattern="test.*pattern",
            mitre_attack="T1000",
            mitre_tactic="Execution",
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


def test_severity_mapping(parser):
    """Test CRS severity to g8e severity mapping."""
    conf = """
SecRule REQUEST_URI "@rx test" \\
    "id:999001,phase:1,log,deny,msg:'Critical',severity:'CRITICAL'"
SecRule REQUEST_URI "@rx test2" \\
    "id:999002,phase:1,log,deny,msg:'Error',severity:'ERROR'"
SecRule REQUEST_URI "@rx test3" \\
    "id:999003,phase:1,log,deny,msg:'Warning',severity:'WARNING'"
SecRule REQUEST_URI "@rx test4" \\
    "id:999004,phase:1,log,deny,msg:'Notice',severity:'NOTICE'"
SecRule REQUEST_URI "@rx test5" \\
    "id:999005,phase:1,log,deny,msg:'Info',severity:'INFO'"
"""
    
    with tempfile.NamedTemporaryFile(mode='w', suffix='.conf', delete=False) as f:
        f.write(conf)
        f.flush()
        conf_path = Path(f.name)
    
    try:
        doctrines = parser.parse_file(conf_path)
        assert len(doctrines) == 5
        
        severities = [d.severity for d in doctrines]
        assert "critical" in severities
        assert "high" in severities
        assert "medium" in severities
        assert "low" in severities
    finally:
        conf_path.unlink()


def test_category_mapping(parser):
    """Test CRS rule ID to g8e category mapping."""
    conf = """
SecRule REQUEST_URI "@rx test" "id:932100,phase:1,log,deny,msg:'RCE'"
SecRule REQUEST_URI "@rx test" "id:930100,phase:1,log,deny,msg:'LFI'"
SecRule REQUEST_URI "@rx test" "id:931100,phase:1,log,deny,msg:'RFI'"
SecRule REQUEST_URI "@rx test" "id:942100,phase:1,log,deny,msg:'SQLi'"
SecRule REQUEST_URI "@rx test" "id:913100,phase:1,log,deny,msg:'Scanner'"
"""
    
    with tempfile.NamedTemporaryFile(mode='w', suffix='.conf', delete=False) as f:
        f.write(conf)
        f.flush()
        conf_path = Path(f.name)
    
    try:
        doctrines = parser.parse_file(conf_path)
        assert len(doctrines) == 5
        
        categories = {d.id: d.category for d in doctrines}
        assert categories["owasp_crs_932100"] == "reverse_shell"
        assert categories["owasp_crs_930100"] == "path_traversal"
        assert categories["owasp_crs_931100"] == "remote_file_inclusion"
        assert categories["owasp_crs_942100"] == "sql_injection"
        assert categories["owasp_crs_913100"] == "reconnaissance"
    finally:
        conf_path.unlink()


def test_robust_parsing_variations(parser):
    """Test parsing with various quoting styles and optional @rx."""
    conf = """
    # Single quotes for pattern and actions
    SecRule REQUEST_URI '@rx (?i)nc' 'id:1001,msg:test,severity:CRITICAL'
    # Mixed quotes
    SecRule ARGS "@rx union" 'id:1002,msg:test,severity:ERROR'
    # Double quotes, no @rx
    SecRule REQUEST_BODY "select" "id:1003,msg:test,severity:WARNING"
    # Single quotes, no @rx
    SecRule ARGS 'delete' 'id:1004,msg:test,severity:NOTICE'
    # Quoted variable
    SecRule "ARGS|REQUEST_HEADERS" "@rx test" "id:1005,msg:test,severity:INFO"
    """
    
    with tempfile.NamedTemporaryFile(mode='w', suffix='.conf', delete=False) as f:
        f.write(conf)
        f.flush()
        conf_path = Path(f.name)
    
    try:
        doctrines = parser.parse_file(conf_path)
        assert len(doctrines) == 5
        
        # Verify specific variations
        ids = [d.id for d in doctrines]
        assert "owasp_crs_1001" in ids
        assert "owasp_crs_1002" in ids
        assert "owasp_crs_1003" in ids
        assert "owasp_crs_1004" in ids
        assert "owasp_crs_1005" in ids
        
        # Verify trimmed pattern
        assert doctrines[0].pattern == "(?i)nc"
    finally:
        conf_path.unlink()


def test_structural_validation_warning(parser, capsys):
    """Test that missed rules trigger a warning."""
    conf = """
    # This rule is valid
    SecRule ARGS "@rx valid" "id:2001,msg:test"
    # This rule is invalid (missing quotes around variable)
    SecRule ARGS invalid "id:2002,msg:test"
    """
    
    with tempfile.NamedTemporaryFile(mode='w', suffix='.conf', delete=False) as f:
        f.write(conf)
        f.flush()
        conf_path = Path(f.name)
    
    try:
        doctrines = parser.parse_file(conf_path)
        assert len(doctrines) == 1
        
        # Check stderr for warning
        captured = capsys.readouterr()
        assert "Warning: Missed 1 SecRule directives" in captured.err
    finally:
        conf_path.unlink()


def test_integration_full_run(tmp_path):
    """Integration test running the script end-to-end."""
    conf_content = 'SecRule ARGS "@rx pattern" "id:3001,msg:\'msg\',severity:\'CRITICAL\'"'
    conf_file = tmp_path / "test.conf"
    conf_file.write_text(conf_content)
    
    output_file = tmp_path / "doctrine.json"
    
    import subprocess
    script_path = Path(__file__).parent.parent / "ingest_owasp_crs.py"
    
    result = subprocess.run(
        [sys.executable, str(script_path), str(conf_file), "-o", str(output_file)],
        capture_output=True,
        text=True
    )
    
    assert result.returncode == 0
    assert "Successfully ingested 1 OWASP CRS doctrines" in result.stdout
    assert output_file.exists()
    
    with open(output_file, 'r') as f:
        data = json.load(f)
    
    assert data["source"] == "owasp_crs"
    assert len(data["doctrines"]) == 1
    assert data["doctrines"][0]["id"] == "owasp_crs_3001"
