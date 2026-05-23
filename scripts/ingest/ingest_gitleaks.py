#!/usr/bin/env python3
"""
Gitleaks Doctrine Ingestion Script

Parses Gitleaks TOML configuration and converts secret detection rules to g8e doctrine format.
"""

import argparse
import json
import re
import sys
from pathlib import Path
from typing import Dict, List, Optional
from dataclasses import dataclass, asdict

from _lib import RegexValidationError, validate_regex_complexity

try:
    import tomllib
except ImportError:
    try:
        import tomli
    except ImportError:
        print("Error: tomllib (Python 3.11+) or tomli library required", file=sys.stderr)
        sys.exit(1)


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


class GitleaksParser:
    """Parser for Gitleaks TOML configuration."""

    # MITRE ATT&CK mapping for secret detection
    MITRE_ATTACK = "T1055.001"  # Process Injection: Dynamic API Invocation
    MITRE_TACTIC = "Credential Access"

    def __init__(self):
        self.doctrines: List[Doctrine] = []

    def parse_file(self, filepath: Path) -> List[Doctrine]:
        """Parse a Gitleaks TOML configuration file."""
        try:
            import tomllib
            with open(filepath, 'rb') as f:
                data = tomllib.load(f)
        except ImportError:
            import tomli
            with open(filepath, 'rb') as f:
                data = tomli.load(f)
        
        doctrines = []
        seen_ids = set()
        
        # Parse rules from [[rules]] sections
        for rule in data.get('rules', []):
            # Skip non-dict entries (some TOML structures may have mixed types)
            if not isinstance(rule, dict):
                continue
            try:
                doctrine = self._parse_rule(rule)
                if doctrine and doctrine.id not in seen_ids:
                    doctrines.append(doctrine)
                    seen_ids.add(doctrine.id)
            except Exception as e:
                print(f"Error parsing rule: {e}", file=sys.stderr)
                continue
        
        return doctrines

    def _parse_rule(self, rule: Dict) -> Optional[Doctrine]:
        """Parse a single Gitleaks rule into a doctrine."""
        rule_id = rule.get('id')
        if not rule_id:
            return None
        
        # Extract regex pattern
        regex = rule.get('regex')
        if not regex:
            return None

        # Clean up the regex (remove TOML string escaping if present)
        pattern = regex.strip()

        # Validate regex complexity to prevent ReDoS
        try:
            validate_regex_complexity(pattern)
        except RegexValidationError as e:
            print(f"Warning: Skipping rule {rule_id} due to regex validation error: {e}", file=sys.stderr)
            return None
        
        # Extract description/name
        description = rule.get('description', rule_id)
        
        # Map severity
        gitleaks_severity = rule.get('severity', 'medium')
        severity = self._map_severity(gitleaks_severity)
        
        # Calculate confidence based on entropy if available
        entropy = rule.get('entropy')
        confidence = self._calculate_confidence(entropy)
        
        doctrine = Doctrine(
            id=f"gitleaks_{rule_id}",
            name=description,
            category="credential_access",
            severity=severity,
            pattern=pattern,
            mitre_attack=self.MITRE_ATTACK,
            mitre_tactic=self.MITRE_TACTIC,
            confidence=confidence,
            enabled=True
        )
        
        return doctrine

    def _map_severity(self, gitleaks_severity: str) -> str:
        """Map Gitleaks severity to g8e severity."""
        severity_map = {
            'critical': 'critical',
            'high': 'critical',
            'medium': 'high',
            'low': 'medium',
        }
        return severity_map.get(gitleaks_severity.lower(), 'medium')

    def _calculate_confidence(self, entropy: Optional[any]) -> float:
        """Calculate confidence score based on entropy configuration."""
        if not entropy:
            return 0.95
        
        # Gitleaks can specify entropy as a float, string "min-max", or a table/dict
        if isinstance(entropy, dict):
            min_val = float(entropy.get('min', 0))
            max_val = float(entropy.get('max', 8))
        elif isinstance(entropy, str):
            # Try to parse "min-max" or just a single value
            try:
                if '-' in entropy:
                    parts = entropy.split('-')
                    min_val = float(parts[0])
                    max_val = float(parts[1])
                else:
                    min_val = 0
                    max_val = float(entropy)
            except (ValueError, IndexError):
                min_val = 0
                max_val = 8
        elif isinstance(entropy, (int, float)):
            min_val = 0
            max_val = float(entropy)
        else:
            return 0.95
        
        # Higher entropy ranges generally indicate higher confidence
        # Normalize to 0.8-0.95 range
        if max_val >= 6:
            return 0.95
        elif max_val >= 4:
            return 0.90
        else:
            return 0.85

    def write_doctrine_file(
        self,
        doctrines: List[Doctrine],
        output_path: Path,
        source: str = "gitleaks",
        version: str = "8.18.0",
        license_str: str = "MIT"
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
        description="Ingest Gitleaks rules into g8e doctrine format"
    )
    parser.add_argument(
        "input",
        type=Path,
        help="Path to Gitleaks TOML configuration file"
    )
    parser.add_argument(
        "-o", "--output",
        type=Path,
        default=Path("protocol/constants/doctrine/gitleaks_doctrine.json"),
        help="Output doctrine file path"
    )
    parser.add_argument(
        "--version",
        default="8.18.0",
        help="Gitleaks version"
    )
    
    args = parser.parse_args()
    
    if not args.input.exists():
        print(f"Error: Input path {args.input} does not exist", file=sys.stderr)
        sys.exit(1)
    
    gitleaks_parser = GitleaksParser()
    
    doctrines = gitleaks_parser.parse_file(args.input)
    
    if not doctrines:
        print("No doctrines extracted", file=sys.stderr)
        sys.exit(1)
    
    # Ensure output directory exists
    args.output.parent.mkdir(parents=True, exist_ok=True)
    
    gitleaks_parser.write_doctrine_file(
        doctrines,
        args.output,
        version=args.version
    )
    
    print(f"Successfully ingested {len(doctrines)} Gitleaks doctrines")


if __name__ == "__main__":
    main()
