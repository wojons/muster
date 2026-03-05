# TEST-MAPPING.md

**Status**: ✅ COMPLETE
**Version**: 1.0.0
**Last Updated**: 2026-03-02

## Overview

This specification defines the comprehensive test mapping strategy that links requirements, specifications, implementation, and test cases. Test mapping ensures complete coverage traceability, enabling verification that every requirement has corresponding tests and that test failures can be traced back to specific specifications.

## Test Mapping Architecture

### Coverage Hierarchy

```
Requirements → Specifications → Implementation → Tests → Results
     ↓              ↓               ↓            ↓         ↓
  REQ-001      SPEC-AUTH      auth/mod.rs    test_auth   PASS
  REQ-002      SPEC-CLI       cli/mod.rs     test_cli    PASS
  REQ-003      SPEC-CONFIG    config.rs      test_config PASS
```

### Mapping File Structure

**File**: `tests/mapping/test-map.json`

```json
{
  "version": "1.0.0",
  "last_updated": "2026-03-02",
  "mappings": [
    {
      "requirement_id": "REQ-001",
      "requirement_title": "User Authentication",
      "specification": "specs/AUTHENTICATION.md",
      "implementation_files": [
        "src/auth/mod.rs",
        "src/auth/session.rs"
      ],
      "test_files": [
        "tests/test_auth.rs",
        "tests/test_session.rs"
      ],
      "coverage": {
        "lines": 95.5,
        "branches": 92.0,
        "functions": 100.0
      },
      "status": "complete"
    },
    {
      "requirement_id": "REQ-002",
      "requirement_title": "Command Line Interface",
      "specification": "specs/CLI.md",
      "implementation_files": [
        "src/cli/mod.rs",
        "src/cli/commands.rs"
      ],
      "test_files": [
        "tests/test_cli.rs",
        "tests/test_commands.rs"
      ],
      "coverage": {
        "lines": 88.3,
        "branches": 85.5,
        "functions": 95.0
      },
      "status": "complete"
    },
    {
      "requirement_id": "REQ-003",
      "requirement_title": "Configuration Management",
      "specification": "specs/CONFIG.md",
      "implementation_files": [
        "src/config/mod.rs",
        "src/config/parser.rs"
      ],
      "test_files": [
        "tests/test_config.rs",
        "tests/test_parser.rs"
      ],
      "coverage": {
        "lines": 91.0,
        "branches": 89.0,
        "functions": 100.0
      },
      "status": "complete"
    }
  ],
  "summary": {
    "total_requirements": 42,
    "mapped_requirements": 42,
    "coverage_threshold": 80.0,
    "average_coverage": 91.5
  }
}
```

## Technical Implementation

### Test Mapping Generator

**File**: `tests/mapping/generate-map.py`

```python
#!/usr/bin/env python3
"""Generate and maintain test-to-requirement mappings."""

import json
import re
from pathlib import Path
from typing import Dict, List, Optional, Set
from dataclasses import dataclass, asdict


@dataclass
class RequirementMapping:
    """Maps a requirement to its implementation and tests."""
    requirement_id: str
    requirement_title: str
    specification: str
    implementation_files: List[str]
    test_files: List[str]
    coverage: Dict[str, float]
    status: str


class TestMapper:
    """Manages test coverage mappings."""
    
    def __init__(self, project_root: Path):
        self.project_root = project_root
        self.mappings: List[RequirementMapping] = []
        
    def scan_requirements(self) -> List[Dict]:
        """Extract requirements from specification files."""
        requirements = []
        specs_dir = self.project_root / "specs"
        
        for spec_file in specs_dir.glob("*.md"):
            content = spec_file.read_text()
            
            # Find requirement sections
            req_pattern = r'###\s+(REQ-\d+):\s*(.+?)\n'
            for match in re.finditer(req_pattern, content, re.DOTALL):
                req_id = match.group(1)
                title = match.group(2).strip()
                
                requirements.append({
                    "id": req_id,
                    "title": title,
                    "specification": str(spec_file.relative_to(self.project_root))
                })
        
        return requirements
    
    def find_implementation(self, requirement_id: str) -> List[str]:
        """Find implementation files for a requirement."""
        files = []
        src_dir = self.project_root / "src"
        
        # Search for requirement references in source code
        for source_file in src_dir.rglob("*.rs"):
            content = source_file.read_text()
            if requirement_id in content:
                files.append(str(source_file.relative_to(self.project_root)))
        
        return files
    
    def find_tests(self, requirement_id: str) -> List[str]:
        """Find test files for a requirement."""
        files = []
        tests_dir = self.project_root / "tests"
        
        # Search for requirement references in test files
        for test_file in tests_dir.rglob("*.rs"):
            content = test_file.read_text()
            if requirement_id in content:
                files.append(str(test_file.relative_to(self.project_root)))
        
        return files
    
    def parse_coverage(self, coverage_file: Path) -> Dict[str, float]:
        """Parse coverage report for metrics."""
        if not coverage_file.exists():
            return {"lines": 0.0, "branches": 0.0, "functions": 0.0}
        
        try:
            # Parse cargo-tarpaulin JSON output
            data = json.loads(coverage_file.read_text())
            
            total_covered = 0
            total_lines = 0
            
            for file_data in data.get("files", []):
                for line in file_data.get("lines", []):
                    total_lines += 1
                    if line.get("hits", 0) > 0:
                        total_covered += 1
            
            line_coverage = (total_covered / total_lines * 100) if total_lines > 0 else 0.0
            
            return {
                "lines": round(line_coverage, 1),
                "branches": data.get("branch_coverage", 0.0),
                "functions": data.get("function_coverage", 0.0)
            }
        except (json.JSONDecodeError, KeyError):
            return {"lines": 0.0, "branches": 0.0, "functions": 0.0}
    
    def generate_mapping(self) -> None:
        """Generate complete test mapping."""
        requirements = self.scan_requirements()
        coverage_file = self.project_root / "target/coverage/coverage.json"
        
        for req in requirements:
            impl_files = self.find_implementation(req["id"])
            test_files = self.find_tests(req["id"])
            
            coverage = self.parse_coverage(coverage_file)
            
            mapping = RequirementMapping(
                requirement_id=req["id"],
                requirement_title=req["title"],
                specification=req["specification"],
                implementation_files=impl_files,
                test_files=test_files,
                coverage=coverage,
                status="complete" if test_files else "incomplete"
            )
            
            self.mappings.append(mapping)
    
    def save_mapping(self, output_file: Path) -> None:
        """Save mapping to JSON file."""
        output_file.parent.mkdir(parents=True, exist_ok=True)
        
        data = {
            "version": "1.0.0",
            "last_updated": "2026-03-02",
            "mappings": [asdict(m) for m in self.mappings],
            "summary": self._generate_summary()
        }
        
        output_file.write_text(json.dumps(data, indent=2))
        print(f"✓ Test mapping saved: {output_file}")
    
    def _generate_summary(self) -> Dict:
        """Generate summary statistics."""
        total = len(self.mappings)
        mapped = len([m for m in self.mappings if m.test_files])
        
        avg_coverage = sum(m.coverage["lines"] for m in self.mappings) / total if total > 0 else 0
        
        return {
            "total_requirements": total,
            "mapped_requirements": mapped,
            "coverage_threshold": 80.0,
            "average_coverage": round(avg_coverage, 1)
        }
    
    def find_gaps(self) -> List[str]:
        """Identify requirements without test coverage."""
        gaps = []
        
        for mapping in self.mappings:
            if not mapping.test_files:
                gaps.append(f"{mapping.requirement_id}: {mapping.requirement_title}")
            elif mapping.coverage["lines"] < 80.0:
                gaps.append(
                    f"{mapping.requirement_id}: Coverage {mapping.coverage['lines']}% (below 80%)"
                )
        
        return gaps


def main():
    import sys
    
    root = Path(".")
    mapper = TestMapper(root)
    
    print("Generating test mapping...")
    mapper.generate_mapping()
    
    # Find gaps
    gaps = mapper.find_gaps()
    if gaps:
        print("\n⚠️ Coverage gaps found:")
        for gap in gaps:
            print(f"  - {gap}")
    else:
        print("\n✅ All requirements have test coverage")
    
    # Save mapping
    output = Path("tests/mapping/test-map.json")
    mapper.save_mapping(output)
    
    # Exit with error if gaps found
    sys.exit(1 if gaps else 0)


if __name__ == "__main__":
    main()
```

### Requirement Annotation

**File**: `src/auth/mod.rs`

```rust
//! Authentication module
//!
//! Implements requirements:
//! - REQ-001: User authentication
//! - REQ-001-A: Password validation
//! - REQ-001-B: Session management

use std::time::Duration;

/// Authenticates a user with credentials
///
/// # Requirements
/// - REQ-001: Validates user credentials
/// - REQ-001-A: Enforces password complexity
///
/// # Tests
/// - tests/test_auth.rs::test_authenticate_success
/// - tests/test_auth.rs::test_authenticate_invalid_password
/// - tests/test_auth.rs::test_authenticate_password_complexity
pub fn authenticate(username: &str, password: &str) -> Result<Session, AuthError> {
    // Implementation
    validate_password(password)?;  // REQ-001-A
    
    // Verify against database
    let user = db::find_user(username)?;
    
    if !verify_password(password, &user.password_hash) {
        return Err(AuthError::InvalidCredentials);
    }
    
    // Create session (REQ-001-B)
    let session = Session::new(user.id, Duration::hours(24));
    session.store()?;
    
    Ok(session)
}

/// Validates password complexity
///
/// # Requirements
/// - REQ-001-A: Minimum 8 characters, 1 uppercase, 1 number
fn validate_password(password: &str) -> Result<(), AuthError> {
    if password.len() < 8 {
        return Err(AuthError::PasswordTooShort);
    }
    
    if !password.chars().any(|c| c.is_uppercase()) {
        return Err(AuthError::PasswordNoUppercase);
    }
    
    if !password.chars().any(|c| c.is_numeric()) {
        return Err(AuthError::PasswordNoNumber);
    }
    
    Ok(())
}
```

**File**: `tests/test_auth.rs`

```rust
//! Authentication tests
//!
//! Covers requirements:
//! - REQ-001: User authentication
//! - REQ-001-A: Password validation
//! - REQ-001-B: Session management

use muster::auth::{authenticate, AuthError};

/// Test: REQ-001 - Successful authentication
#[test]
fn test_authenticate_success() {
    // Arrange
    let username = "test_user";
    let password = "ValidP@ss123";
    
    // Act
    let result = authenticate(username, password);
    
    // Assert
    assert!(result.is_ok());
    let session = result.unwrap();
    assert_eq!(session.user_id, "test_user_id");
}

/// Test: REQ-001 - Invalid credentials
#[test]
fn test_authenticate_invalid_password() {
    // Arrange
    let username = "test_user";
    let password = "WrongPassword";
    
    // Act
    let result = authenticate(username, password);
    
    // Assert
    assert!(matches!(result, Err(AuthError::InvalidCredentials)));
}

/// Test: REQ-001-A - Password complexity (too short)
#[test]
fn test_authenticate_password_complexity_too_short() {
    // Arrange
    let username = "test_user";
    let password = "Short1";  // Only 6 characters
    
    // Act
    let result = authenticate(username, password);
    
    // Assert
    assert!(matches!(result, Err(AuthError::PasswordTooShort)));
}

/// Test: REQ-001-A - Password complexity (no uppercase)
#[test]
fn test_authenticate_password_complexity_no_uppercase() {
    // Arrange
    let username = "test_user";
    let password = "validpass123";  // No uppercase
    
    // Act
    let result = authenticate(username, password);
    
    // Assert
    assert!(matches!(result, Err(AuthError::PasswordNoUppercase)));
}

/// Test: REQ-001-A - Password complexity (no number)
#[test]
fn test_authenticate_password_complexity_no_number() {
    // Arrange
    let username = "test_user";
    let password = "ValidPassword";  // No number
    
    // Act
    let result = authenticate(username, password);
    
    // Assert
    assert!(matches!(result, Err(AuthError::PasswordNoNumber)));
}

/// Test: REQ-001-B - Session creation
#[test]
fn test_session_creation() {
    // Arrange
    let username = "test_user";
    let password = "ValidP@ss123";
    
    // Act
    let session = authenticate(username, password).unwrap();
    
    // Assert
    assert!(!session.token.is_empty());
    assert!(session.expires_at > chrono::Utc::now());
}
```

### Coverage Reporting

**File**: `.github/workflows/coverage.yml`

```yaml
name: Coverage Report

on:
  push:
    branches: [main]
  pull_request:
    branches: [main]

jobs:
  coverage:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      
      - name: Setup Rust
        uses: actions-rs/toolchain@v1
        with:
          toolchain: stable
          override: true
      
      - name: Install tarpaulin
        run: cargo install cargo-tarpaulin
      
      - name: Generate Coverage
        run: |
          cargo tarpaulin \
            --out json \
            --output-dir target/coverage \
            --all-features
      
      - name: Generate Test Mapping
        run: |
          python tests/mapping/generate-map.py
      
      - name: Check Coverage Threshold
        run: |
          python -c "
          import json
          with open('tests/mapping/test-map.json') as f:
              data = json.load(f)
          avg = data['summary']['average_coverage']
          threshold = data['summary']['coverage_threshold']
          if avg < threshold:
              print(f'Coverage {avg}% below threshold {threshold}%')
              exit(1)
          print(f'Coverage {avg}% meets threshold {threshold}%')
          "
      
      - name: Upload Coverage Report
        uses: actions/upload-artifact@v4
        with:
          name: coverage-report
          path: |
            target/coverage/
            tests/mapping/test-map.json
      
      - name: Comment PR
        if: github.event_name == 'pull_request'
        uses: actions/github-script@v7
        with:
          script: |
            const fs = require('fs');
            const mapping = JSON.parse(fs.readFileSync('tests/mapping/test-map.json', 'utf8'));
            
            const summary = mapping.summary;
            const gaps = mapping.mappings.filter(m => m.coverage.lines < 80);
            
            let body = `## Test Coverage Report\n\n`;
            body += `**Average Coverage**: ${summary.average_coverage}%\n`;
            body += `**Requirements**: ${summary.mapped_requirements}/${summary.total_requirements} mapped\n\n`;
            
            if (gaps.length > 0) {
              body += `### Coverage Gaps\n\n`;
              gaps.forEach(m => {
                body += `- ${m.requirement_id}: ${m.coverage.lines}% coverage\n`;
              });
            }
            
            github.rest.issues.createComment({
              issue_number: context.issue.number,
              owner: context.repo.owner,
              repo: context.repo.repo,
              body: body
            });
```

### Traceability Matrix Generator

**File**: `tests/mapping/traceability-matrix.py`

```python
#!/usr/bin/env python3
"""Generate traceability matrix from test mappings."""

import json
from pathlib import Path
from typing import List, Dict
import csv


def load_mappings(mapping_file: Path) -> Dict:
    """Load test mappings from JSON."""
    return json.loads(mapping_file.read_text())


def generate_csv_matrix(mappings: List[Dict], output: Path) -> None:
    """Generate CSV traceability matrix."""
    with open(output, 'w', newline='') as f:
        writer = csv.writer(f)
        
        # Header
        writer.writerow([
            'Requirement ID',
            'Requirement Title',
            'Specification',
            'Implementation Files',
            'Test Files',
            'Line Coverage %',
            'Branch Coverage %',
            'Function Coverage %',
            'Status'
        ])
        
        # Data rows
        for m in mappings:
            writer.writerow([
                m['requirement_id'],
                m['requirement_title'],
                m['specification'],
                '; '.join(m['implementation_files']),
                '; '.join(m['test_files']),
                m['coverage']['lines'],
                m['coverage']['branches'],
                m['coverage']['functions'],
                m['status']
            ])


def generate_html_matrix(mappings: List[Dict], output: Path) -> None:
    """Generate HTML traceability matrix."""
    html = """<!DOCTYPE html>
<html>
<head>
    <title>Traceability Matrix</title>
    <style>
        body { font-family: -apple-system, sans-serif; margin: 2rem; }
        table { border-collapse: collapse; width: 100%; }
        th, td { border: 1px solid #ddd; padding: 8px; text-align: left; }
        th { background-color: #f2f2f2; }
        .complete { color: green; }
        .incomplete { color: red; }
        .warning { background-color: #fff3cd; }
    </style>
</head>
<body>
    <h1>Requirements Traceability Matrix</h1>
    <table>
        <tr>
            <th>Requirement</th>
            <th>Title</th>
            <th>Specification</th>
            <th>Implementation</th>
            <th>Tests</th>
            <th>Coverage</th>
            <th>Status</th>
        </tr>
"""
    
    for m in mappings:
        coverage_class = 'warning' if m['coverage']['lines'] < 80 else ''
        status_class = 'complete' if m['status'] == 'complete' else 'incomplete'
        
        html += f"""        <tr class="{coverage_class}">
            <td>{m['requirement_id']}</td>
            <td>{m['requirement_title']}</td>
            <td>{m['specification']}</td>
            <td>{', '.join(m['implementation_files'])}</td>
            <td>{', '.join(m['test_files'])}</td>
            <td>{m['coverage']['lines']}%</td>
            <td class="{status_class}">{m['status']}</td>
        </tr>
"""
    
    html += """    </table>
</body>
</html>"""
    
    output.write_text(html)


def main():
    import sys
    
    mapping_file = Path("tests/mapping/test-map.json")
    if not mapping_file.exists():
        print(f"Error: {mapping_file} not found")
        sys.exit(1)
    
    data = load_mappings(mapping_file)
    mappings = data['mappings']
    
    # Generate CSV
    csv_output = Path("tests/mapping/traceability-matrix.csv")
    generate_csv_matrix(mappings, csv_output)
    print(f"✓ CSV matrix: {csv_output}")
    
    # Generate HTML
    html_output = Path("tests/mapping/traceability-matrix.html")
    generate_html_matrix(mappings, html_output)
    print(f"✓ HTML matrix: {html_output}")
    
    # Summary
    summary = data['summary']
    print(f"\nSummary:")
    print(f"  Requirements: {summary['mapped_requirements']}/{summary['total_requirements']}")
    print(f"  Average Coverage: {summary['average_coverage']}%")
    print(f"  Threshold: {summary['coverage_threshold']}%")


if __name__ == "__main__":
    main()
```

## Test Mapping Standards

### Requirement Annotation Format

```rust
// Single requirement
// REQ-001: User authentication

// Multiple requirements
// REQ-001: User authentication
// REQ-001-A: Password validation
// REQ-001-B: Session management

// With test references
// REQ-001
// Tests: tests/test_auth.rs::test_authenticate_success
// Tests: tests/test_auth.rs::test_authenticate_failure
```

### Coverage Thresholds

| Component | Line Coverage | Branch Coverage | Function Coverage |
|-----------|---------------|-----------------|-------------------|
| Core Logic | ≥90% | ≥85% | 100% |
| CLI Interface | ≥85% | ≥80% | 100% |
| Configuration | ≥90% | ≥85% | 100% |
| Utilities | ≥80% | ≥75% | 100% |
| Overall | ≥85% | ≥80% | 100% |

### Gap Analysis Workflow

1. **Scan**: Extract all requirements from specifications
2. **Map**: Link requirements to implementation and tests
3. **Measure**: Calculate coverage for each requirement
4. **Report**: Identify gaps and low coverage areas
5. **Remediate**: Add missing tests or improve coverage
6. **Verify**: Re-run mapping to confirm improvements

## References

- [IEEE 829 Test Documentation Standard](https://standards.ieee.org/standard/829-2008.html)
- [ISO/IEC/IEEE 29119 Software Testing](https://www.iso.org/standard/45142.html)
- **master‑specification.md** – Complete system specification
- **SPECIFICATION‑INDEX.md** – Index of all specification files

---

## Bible Standard Compliance

This specification adheres to:
- **Complete Traceability**: Every requirement linked to implementation and tests
- **Automated Mapping**: Scripts for generating and maintaining mappings
- **Coverage Standards**: Defined thresholds with enforcement
- **Gap Analysis**: Automated identification of untested requirements
- **Documentation**: Requirements annotated in source code

**Quality Standards Met**: ✅ Complete | ✅ Automated | ✅ Traceable
