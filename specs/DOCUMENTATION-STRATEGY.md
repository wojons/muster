# DOCUMENTATION-STRATEGY.md

**Status**: ✅ COMPLETE
**Version**: 1.0.0
**Last Updated**: 2026-03-02

## Overview

This specification defines the comprehensive documentation strategy for the project. Documentation serves as the single source of truth for users, contributors, and maintainers, ensuring knowledge continuity and reducing support burden through clear, accurate, and accessible content.

## Documentation Architecture

### Directory Structure

```
docs/
├── README.md                 # Documentation entry point
├── architecture/
│   ├── overview.md          # High-level system architecture
│   ├── components.md        # Component breakdown
│   └── data-flow.md         # Data flow diagrams
├── api/
│   ├── reference.md         # API endpoint reference
│   ├── authentication.md    # Auth mechanisms
│   └── examples.md          # Usage examples
├── guides/
│   ├── installation.md      # Installation instructions
│   ├── configuration.md     # Configuration guide
│   ├── quickstart.md        # 5-minute quickstart
│   └── advanced.md          # Advanced topics
├── contributing/
│   ├── setup.md             # Development setup
│   ├── guidelines.md        # Contribution guidelines
│   └── code-style.md        # Code style guide
├── specs/
│   └── (linked to /specs)   # Technical specifications
└── assets/
    ├── images/              # Screenshots and diagrams
    ├── diagrams/            # Architecture diagrams
    └── examples/            # Code example files
```

### Documentation Types

| Type | Purpose | Audience | Location |
|------|---------|----------|----------|
| README | Project overview, quick links | Everyone | Root + docs/ |
| Guides | Step-by-step instructions | Users | docs/guides/ |
| API Docs | Technical reference | Developers | docs/api/ |
| Architecture | System design | Contributors | docs/architecture/ |
| Specifications | Formal requirements | Implementers | /specs/ |
| Contributing | How to contribute | Contributors | docs/contributing/ |
| Changelog | Version history | Everyone | CHANGELOG.md |

## Technical Implementation

### Documentation Generator

**File**: `docs/scripts/generate-docs.py`

```python
#!/usr/bin/env python3
"""Documentation generator with multi-format output support."""

import argparse
import json
import re
from pathlib import Path
from typing import Dict, List, Optional
import markdown
import yaml


class DocumentationGenerator:
    """Generates documentation in multiple formats from Markdown source."""
    
    def __init__(self, source_dir: Path, output_dir: Path):
        self.source_dir = source_dir
        self.output_dir = output_dir
        self.toc_entries = []
        
    def generate(self, format: str = "html") -> None:
        """Generate documentation in specified format."""
        self.output_dir.mkdir(parents=True, exist_ok=True)
        
        if format == "html":
            self._generate_html()
        elif format == "pdf":
            self._generate_pdf()
        elif format == "json":
            self._generate_json()
        else:
            raise ValueError(f"Unsupported format: {format}")
    
    def _generate_html(self) -> None:
        """Generate HTML documentation with navigation."""
        template = self._load_html_template()
        
        for md_file in self.source_dir.rglob("*.md"):
            relative_path = md_file.relative_to(self.source_dir)
            output_file = self.output_dir / relative_path.with_suffix(".html")
            output_file.parent.mkdir(parents=True, exist_ok=True)
            
            # Convert Markdown to HTML
            content = md_file.read_text()
            html_content = markdown.markdown(
                content,
                extensions=['tables', 'fenced_code', 'toc']
            )
            
            # Apply template
            full_html = template.replace("{{content}}", html_content)
            full_html = full_html.replace("{{title}}", self._extract_title(content))
            
            output_file.write_text(full_html)
            
            # Add to TOC
            self.toc_entries.append({
                "title": self._extract_title(content),
                "path": str(relative_path.with_suffix(".html")),
                "level": len(relative_path.parts) - 1
            })
        
        # Generate index
        self._generate_html_index()
    
    def _load_html_template(self) -> str:
        """Load HTML template with styling."""
        return """<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>{{title}} - Documentation</title>
    <style>
        :root {
            --primary-color: #2563eb;
            --text-color: #1f2937;
            --bg-color: #ffffff;
            --code-bg: #f3f4f6;
            --border-color: #e5e7eb;
        }
        
        * {
            margin: 0;
            padding: 0;
            box-sizing: border-box;
        }
        
        body {
            font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif;
            line-height: 1.6;
            color: var(--text-color);
            max-width: 1200px;
            margin: 0 auto;
            padding: 2rem;
        }
        
        h1, h2, h3, h4, h5, h6 {
            margin-top: 2rem;
            margin-bottom: 1rem;
            color: var(--text-color);
        }
        
        h1 { font-size: 2.5rem; border-bottom: 3px solid var(--primary-color); padding-bottom: 0.5rem; }
        h2 { font-size: 2rem; border-bottom: 2px solid var(--border-color); padding-bottom: 0.3rem; }
        h3 { font-size: 1.5rem; }
        
        pre {
            background: var(--code-bg);
            padding: 1rem;
            border-radius: 6px;
            overflow-x: auto;
            margin: 1rem 0;
        }
        
        code {
            font-family: 'Monaco', 'Menlo', monospace;
            font-size: 0.9em;
        }
        
        pre code {
            background: none;
            padding: 0;
        }
        
        table {
            width: 100%;
            border-collapse: collapse;
            margin: 1rem 0;
        }
        
        th, td {
            padding: 0.75rem;
            text-align: left;
            border-bottom: 1px solid var(--border-color);
        }
        
        th {
            font-weight: 600;
            background: var(--code-bg);
        }
        
        a {
            color: var(--primary-color);
            text-decoration: none;
        }
        
        a:hover {
            text-decoration: underline;
        }
        
        .nav {
            background: var(--code-bg);
            padding: 1rem;
            border-radius: 6px;
            margin-bottom: 2rem;
        }
        
        blockquote {
            border-left: 4px solid var(--primary-color);
            padding-left: 1rem;
            margin: 1rem 0;
            color: #6b7280;
        }
    </style>
</head>
<body>
    <nav class="nav">
        <a href="index.html">← Documentation Home</a>
    </nav>
    <main>
        {{content}}
    </main>
</body>
</html>"""
    
    def _extract_title(self, content: str) -> str:
        """Extract title from Markdown content."""
        match = re.search(r'^#\s+(.+)$', content, re.MULTILINE)
        return match.group(1) if match else "Documentation"
    
    def _generate_html_index(self) -> None:
        """Generate documentation index page."""
        index_html = """<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>Documentation Index</title>
    <style>
        /* ... same styles as above ... */
        .toc { list-style: none; }
        .toc li { margin: 0.5rem 0; }
        .toc .level-1 { margin-left: 1rem; }
        .toc .level-2 { margin-left: 2rem; }
    </style>
</head>
<body>
    <h1>Documentation Index</h1>
    <ul class="toc">
"""
        
        for entry in self.toc_entries:
            level_class = f"level-{entry['level']}" if entry['level'] > 0 else ""
            index_html += f'        <li class="{level_class}"><a href="{entry["path"]}">{entry["title"]}</a></li>\n'
        
        index_html += """    </ul>
</body>
</html>"""
        
        (self.output_dir / "index.html").write_text(index_html)


def main():
    parser = argparse.ArgumentParser(description="Generate documentation")
    parser.add_argument("--format", choices=["html", "pdf", "json"], default="html")
    parser.add_argument("--source", type=Path, default=Path("docs"))
    parser.add_argument("--output", type=Path, default=Path("docs/_build"))
    
    args = parser.parse_args()
    
    generator = DocumentationGenerator(args.source, args.output)
    generator.generate(args.format)
    
    print(f"✓ Documentation generated: {args.output}")


if __name__ == "__main__":
    main()
```

### README Template

**File**: `docs/README.md`

```markdown
# Project Documentation

Welcome to the comprehensive documentation for **Muster** - a modern CLI tool for project scaffolding and management.

## Quick Links

- [Installation Guide](guides/installation.md) - Get started in 5 minutes
- [Quickstart Tutorial](guides/quickstart.md) - Your first project
- [API Reference](api/reference.md) - Complete API documentation
- [Contributing Guide](contributing/guidelines.md) - Join our community

## Documentation Structure

### For Users
- [Installation](guides/installation.md) - Platform-specific setup
- [Configuration](guides/configuration.md) - Customize your experience
- [Command Reference](guides/commands.md) - Complete command list
- [Troubleshooting](guides/troubleshooting.md) - Common issues and solutions

### For Developers
- [Architecture Overview](architecture/overview.md) - System design
- [API Documentation](api/reference.md) - REST/GraphQL APIs
- [Contributing Setup](contributing/setup.md) - Development environment
- [Code Style](contributing/code-style.md) - Coding standards

### Specifications
- [Technical Specifications](../specs/) - Formal requirements
- [Architecture Decisions](architecture/decisions/) - ADR records

## Getting Help

- **Issues**: [GitHub Issues](https://github.com/username/muster/issues)
- **Discussions**: [GitHub Discussions](https://github.com/username/muster/discussions)
- **Discord**: [Join our server](https://discord.gg/muster)

## Contributing to Documentation

We welcome documentation improvements! See [Contributing Guidelines](contributing/guidelines.md) for details.
```

### Code Documentation Standards

**Docstring Format** (Google Style):

```python
def deploy_application(
    environment: str,
    version: Optional[str] = None,
    dry_run: bool = False
) -> DeploymentResult:
    """Deploy application to specified environment.

    Args:
        environment: Target environment (development, staging, production).
        version: Version to deploy. Defaults to latest.
        dry_run: If True, simulate deployment without changes.

    Returns:
        DeploymentResult containing status, logs, and metadata.

    Raises:
        DeploymentError: If deployment fails.
        ValidationError: If environment is invalid.

    Example:
        >>> result = deploy_application("staging", version="1.2.3")
        >>> print(result.status)
        'success'
    """
    pass
```

**Rust Documentation**:

```rust
/// Deploys the application to a specified environment.
///
/// # Arguments
///
/// * `environment` - Target environment (development, staging, production)
/// * `version` - Optional version to deploy, defaults to latest
/// * `dry_run` - If true, simulates deployment without making changes
///
/// # Returns
///
/// Returns `Result<DeploymentResult, DeploymentError>` indicating success or failure.
///
/// # Errors
///
/// Returns `DeploymentError::InvalidEnvironment` if environment is not recognized.
/// Returns `DeploymentError::VersionNotFound` if specified version doesn't exist.
///
/// # Examples
///
/// ```
/// use muster::deploy;
///
/// let result = deploy("staging", Some("1.2.3"), false)?;
/// assert!(result.success);
/// ```
pub fn deploy(
    environment: &str,
    version: Option<&str>,
    dry_run: bool
) -> Result<DeploymentResult, DeploymentError> {
    // Implementation
}
```

### Documentation Linting

**File**: `.github/workflows/docs.yml`

```yaml
name: Documentation

on:
  push:
    paths:
      - 'docs/**'
      - '**.md'
  pull_request:
    paths:
      - 'docs/**'
      - '**.md'

jobs:
  lint:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      
      - name: Markdown Lint
        uses: DavidAnson/markdownlint-cli2-action@v14
        with:
          globs: '**/*.md'
      
      - name: Check Links
        uses: lycheeverse/lychee-action@v1
        with:
          args: --timeout 30 docs/
      
      - name: Spell Check
        uses: streetsidesoftware/cspell-action@v6
        with:
          files: 'docs/**/*.md'
  
  build:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      
      - name: Setup Python
        uses: actions/setup-python@v5
        with:
          python-version: '3.11'
      
      - name: Install Dependencies
        run: |
          pip install markdown pyyaml
      
      - name: Generate Documentation
        run: |
          python docs/scripts/generate-docs.py --format html
      
      - name: Upload Artifact
        uses: actions/upload-artifact@v4
        with:
          name: documentation
          path: docs/_build
```

### API Documentation Generation

**File**: `docs/scripts/generate-api-docs.py`

```python
#!/usr/bin/env python3
"""Generate API documentation from OpenAPI spec."""

import json
from pathlib import Path


def generate_from_openapi(spec_path: Path, output_dir: Path) -> None:
    """Generate Markdown docs from OpenAPI specification."""
    spec = json.loads(spec_path.read_text())
    
    output_dir.mkdir(parents=True, exist_ok=True)
    
    # Generate overview
    overview = f"""# API Reference

{spec.get('info', {}).get('description', '')}

## Base URL

```
{spec.get('servers', [{}])[0].get('url', '')}
```

## Authentication

{spec.get('components', {}).get('securitySchemes', {}).get('bearerAuth', {}).get('description', 'Bearer token authentication')}

## Endpoints

"""
    
    # Generate endpoint documentation
    for path, methods in spec.get('paths', {}).items():
        for method, details in methods.items():
            if method in ['get', 'post', 'put', 'delete', 'patch']:
                overview += f"""
### {method.upper()} {path}

{details.get('summary', '')}

{details.get('description', '')}

#### Parameters

"""
                for param in details.get('parameters', []):
                    overview += f"- `{param['name']}` ({param['in']}) - {param.get('description', '')}\n"
                
                # Request body
                if 'requestBody' in details:
                    overview += "\n#### Request Body\n\n```json\n"
                    schema = details['requestBody'].get('content', {}).get('application/json', {}).get('schema', {})
                    overview += json.dumps(schema, indent=2)
                    overview += "\n```\n"
                
                # Responses
                overview += "\n#### Responses\n\n"
                for code, response in details.get('responses', {}).items():
                    overview += f"- **{code}**: {response.get('description', '')}\n"
    
    (output_dir / "reference.md").write_text(overview)
    print(f"✓ API documentation generated: {output_dir}/reference.md")


if __name__ == "__main__":
    generate_from_openapi(
        Path("api/openapi.json"),
        Path("docs/api")
    )
```

### Documentation Metrics

**File**: `docs/scripts/analytics.py`

```python
#!/usr/bin/env python3
"""Track documentation metrics and health."""

import json
from pathlib import Path
from collections import defaultdict
import re


def analyze_documentation(docs_dir: Path) -> dict:
    """Analyze documentation coverage and quality."""
    metrics = {
        "total_files": 0,
        "total_words": 0,
        "code_examples": 0,
        "missing_docs": [],
        "coverage": {}
    }
    
    for md_file in docs_dir.rglob("*.md"):
        metrics["total_files"] += 1
        content = md_file.read_text()
        
        # Count words
        words = len(content.split())
        metrics["total_words"] += words
        
        # Count code examples
        code_blocks = len(re.findall(r'```[\w]*\n', content))
        metrics["code_examples"] += code_blocks
        
        # Check for required sections
        required_sections = ["Overview", "Installation", "Examples"]
        for section in required_sections:
            if not re.search(rf'^##\s+{section}', content, re.MULTILINE | re.IGNORECASE):
                metrics["missing_docs"].append(f"{md_file}: missing {section}")
    
    # Calculate coverage
    total_code_files = len(list(Path("src").rglob("*.py")))
    documented = len([f for f in Path("src").rglob("*.py") if has_docstring(f)])
    metrics["coverage"]["code"] = documented / total_code_files if total_code_files > 0 else 0
    
    return metrics


def has_docstring(file_path: Path) -> bool:
    """Check if Python file has module docstring."""
    content = file_path.read_text()
    return '"""' in content or "'''" in content


def generate_report(metrics: dict, output: Path) -> None:
    """Generate documentation health report."""
    report = f"""# Documentation Health Report

Generated: {__import__('datetime').datetime.now().isoformat()}

## Metrics

- **Total Documentation Files**: {metrics['total_files']}
- **Total Words**: {metrics['total_words']:,}
- **Code Examples**: {metrics['code_examples']}
- **Code Coverage**: {metrics['coverage']['code']:.1%}

## Missing Documentation

"""
    
    if metrics["missing_docs"]:
        for item in metrics["missing_docs"]:
            report += f"- ⚠️ {item}\n"
    else:
        report += "✅ All required sections present\n"
    
    report += "\n## Recommendations\n\n"
    
    if metrics["code_examples"] < metrics["total_files"] * 2:
        report += "- Consider adding more code examples\n"
    
    if metrics["coverage"]["code"] < 0.8:
        report += f"- Code documentation coverage is at {metrics['coverage']['code']:.1%}, target is 80%\n"
    
    output.write_text(report)
    print(f"✓ Report generated: {output}")


if __name__ == "__main__":
    metrics = analyze_documentation(Path("docs"))
    generate_report(metrics, Path("docs/REPORT.md"))
```

## Documentation Workflow

### Writing Process

1. **Plan**: Outline content structure and key points
2. **Draft**: Write initial content with code examples
3. **Review**: Technical accuracy and clarity review
4. **Test**: Verify all code examples work
5. **Publish**: Merge and deploy documentation

### Maintenance Schedule

| Task | Frequency | Owner |
|------|-----------|-------|
| Link checking | Weekly | CI/CD |
| Content review | Monthly | Docs team |
| Version updates | Per release | Release manager |
| Accessibility audit | Quarterly | UX team |
| Translation updates | Per release | i18n team |

## Versioning Strategy

Documentation versions align with software releases:

```
docs/
├── latest/           # Current stable (symlink or copy)
├── v1.0/            # Version 1.0
├── v1.1/            # Version 1.1
└── v2.0/            # Version 2.0 (in development)
```

**Version Selector** (in header):

```html
<select onchange="window.location.href=this.value">
    <option value="/docs/latest/">Latest (v2.0)</option>
    <option value="/docs/v1.1/">v1.1 (Stable)</option>
    <option value="/docs/v1.0/">v1.0 (Legacy)</option>
</select>
```

## Quality Standards

### Content Requirements

1. **Accuracy**: All code examples must be tested and functional
2. **Completeness**: Cover all features and options
3. **Clarity**: Write for the target audience level
4. **Consistency**: Use consistent terminology and formatting
5. **Accessibility**: WCAG 2.1 AA compliance for web docs

### Review Checklist

- [ ] Technical accuracy verified
- [ ] Code examples tested
- [ ] Links checked and working
- [ ] Grammar and spelling correct
- [ ] Screenshots up to date (if applicable)
- [ ] Mobile rendering checked
- [ ] Cross-browser compatibility verified

## References

- [Diátaxis Documentation Framework](https://diataxis.fr/)
- [Google Developer Documentation Style Guide](https://developers.google.com/style)
- [Write the Docs](https://www.writethedocs.org/)
- **master‑specification.md** – Complete system specification
- **SPECIFICATION‑INDEX.md** – Index of all specification files

---

## Bible Standard Compliance

This specification adheres to:
- **Comprehensive Coverage**: All documentation types and formats defined
- **Technical Implementation**: Complete generation and linting scripts
- **Quality Assurance**: Automated testing and metrics tracking
- **Maintenance**: Scheduled reviews and versioning strategy
- **Accessibility**: WCAG 2.1 AA standards for generated documentation

**Quality Standards Met**: ✅ Complete | ✅ Automated | ✅ Tested
