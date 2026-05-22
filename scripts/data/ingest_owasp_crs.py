#!/usr/bin/env python3
"""
OWASP CRS Doctrine Ingestion Script

Parses OWASP Core Rule Set SecLanguage rules and converts them to g8e doctrine format.
"""

import argparse
import json
import re
import sys
from pathlib import Path
from typing import Dict, List, Optional
from dataclasses import dataclass, asdict


@dataclass
class Doctrine:
    id: str
    name: str
    category: str
    severity: str
    pattern: str
    mitre_attack: Optional[str] = None
    mitre_tactic: Optional[str] = None
    confidence: float = 0.95
    enabled: bool = True


class CRSParser:
    """Parser for OWASP CRS SecLanguage rules."""

    # MITRE ATT&CK mapping for common CRS rule IDs
    MITRE_MAPPING = {
        "932": "T1059.004",  # RCE → Command and Scripting Interpreter: Unix Shell
        "930": "T1006",      # LFI → Direct Access to System Files
        "931": "T1190",      # RFI → Exploit Public-Facing Application
        "942": "T1190",      # SQLi → Exploit Public-Facing Application
        "913": "T1595.001",  # Scanner → Scanning IP Blocks
        "941": "T1059.004",  # XSS → Command and Scripting Interpreter
    }

    # Tactic mapping
    TACTIC_MAPPING = {
        "T1059.004": "Execution",
        "T1006": "Credential Access",
        "T1190": "Initial Access",
        "T1595.001": "Reconnaissance",
    }

    # Category mapping from CRS rule IDs to g8e categories
    CATEGORY_MAPPING = {
        "932": "reverse_shell",
        "930": "path_traversal",
        "931": "remote_file_inclusion",
        "942": "sql_injection",
        "913": "reconnaissance",
        "941": "cross_site_scripting",
    }

    # Severity mapping from CRS to g8e
    SEVERITY_MAPPING = {
        "CRITICAL": "critical",
        "ERROR": "critical",
        "WARNING": "high",
        "NOTICE": "medium",
        "INFO": "low",
    }

    def __init__(self):
        self.doctrines: List[Doctrine] = []

    def parse_file(self, filepath: Path) -> List[Doctrine]:
        """Parse a single CRS .conf file and extract doctrines."""
        doctrines = []
        
        with open(filepath, 'r', encoding='utf-8', errors='ignore') as f:
            content = f.read()
        
        # Normalize multiline rules by removing backslash continuations and newlines
        # CRS rules span multiple lines with backslash continuations
        content = re.sub(r'\\\s*\n', ' ', content)
        
        # Extract SecRule directives with @rx patterns
        # Pattern: SecRule <variable> "@rx <pattern>" "id:...,msg:...,severity:..."
        # Use non-greedy match for pattern to capture everything up to the closing quote
        rule_pattern = re.compile(
            r'SecRule\s+([^\s]+(?:\|[^\s]+)*)\s+"@rx\s+([^"]+?)"\s+"([^"]*(?:id:\d+)[^"]*)"',
            re.MULTILINE | re.IGNORECASE
        )
        
        for match in rule_pattern.finditer(content):
            variable = match.group(1)
            pattern = match.group(2)
            actions = match.group(3)
            
            # Extract rule ID from actions
            rule_id_match = re.search(r'id:(\d+)', actions)
            if not rule_id_match:
                continue
            
            rule_id = rule_id_match.group(1)
            
            # Extract severity from actions
            severity_match = re.search(r'severity:(\'?[A-Z]+\'?)', actions)
            if severity_match:
                crs_severity = severity_match.group(1).strip("'")
                severity = self.SEVERITY_MAPPING.get(crs_severity.upper(), "medium")
            else:
                severity = "medium"
            
            # Extract message from actions for doctrine name
            msg_match = re.search(r'msg:\'([^\']+)\'', actions)
            if msg_match:
                name = msg_match.group(1)
            else:
                name = f"CRS Rule {rule_id}"
            
            # Map to g8e category based on rule ID prefix
            rule_prefix = rule_id[:3]
            category = self.CATEGORY_MAPPING.get(rule_prefix, "other")
            
            # Map to MITRE ATT&CK
            mitre_attack = self.MITRE_MAPPING.get(rule_prefix)
            mitre_tactic = self.TACTIC_MAPPING.get(mitre_attack) if mitre_attack else None
            
            # Clean up pattern (remove SecLanguage escapes and hex sequences)
            pattern = pattern.replace(r'\.', '.').replace(r'\*', '*')
            pattern = re.sub(r'\\x[0-9a-fA-F]{2}', '', pattern)
            
            doctrine = Doctrine(
                id=f"owasp_crs_{rule_id}",
                name=name,
                category=category,
                severity=severity,
                pattern=pattern,
                mitre_attack=mitre_attack,
                mitre_tactic=mitre_tactic,
                confidence=0.90,
                enabled=True
            )
            
            doctrines.append(doctrine)
        
        return doctrines

    def parse_directory(self, directory: Path) -> List[Doctrine]:
        """Parse all .conf files in a directory."""
        all_doctrines = []
        
        for conf_file in directory.glob("*.conf"):
            print(f"Parsing {conf_file.name}...")
            try:
                doctrines = self.parse_file(conf_file)
                all_doctrines.extend(doctrines)
                print(f"  Extracted {len(doctrines)} doctrines")
            except Exception as e:
                print(f"  Error parsing {conf_file.name}: {e}", file=sys.stderr)
        
        return all_doctrines

    def write_doctrine_file(
        self,
        doctrines: List[Doctrine],
        output_path: Path,
        source: str = "owasp_crs",
        version: str = "4.0.0",
        license_str: str = "Apache-2.0"
    ):
        """Write doctrines to canonical JSON format."""
        output = {
            "source": source,
            "version": version,
            "last_updated": "2026-05-22",
            "license": license_str,
            "doctrines": [asdict(d) for d in doctrines]
        }
        
        with open(output_path, 'w', encoding='utf-8') as f:
            json.dump(output, f, indent=2, ensure_ascii=False)
        
        print(f"Wrote {len(doctrines)} doctrines to {output_path}")


def main():
    parser = argparse.ArgumentParser(
        description="Ingest OWASP CRS rules into g8e doctrine format"
    )
    parser.add_argument(
        "input",
        type=Path,
        help="Path to OWASP CRS rules directory or .conf file"
    )
    parser.add_argument(
        "-o", "--output",
        type=Path,
        default=Path("protocol/constants/doctrine/owasp_crs_doctrine.json"),
        help="Output doctrine file path"
    )
    parser.add_argument(
        "--version",
        default="4.0.0",
        help="OWASP CRS version"
    )
    
    args = parser.parse_args()
    
    if not args.input.exists():
        print(f"Error: Input path {args.input} does not exist", file=sys.stderr)
        sys.exit(1)
    
    crs_parser = CRSParser()
    
    if args.input.is_file():
        doctrines = crs_parser.parse_file(args.input)
    else:
        doctrines = crs_parser.parse_directory(args.input)
    
    if not doctrines:
        print("No doctrines extracted", file=sys.stderr)
        sys.exit(1)
    
    # Ensure output directory exists
    args.output.parent.mkdir(parents=True, exist_ok=True)
    
    crs_parser.write_doctrine_file(
        doctrines,
        args.output,
        version=args.version
    )
    
    print(f"Successfully ingested {len(doctrines)} OWASP CRS doctrines")


if __name__ == "__main__":
    main()
