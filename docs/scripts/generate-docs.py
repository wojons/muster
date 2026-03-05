#!/usr/bin/env python3
"""Documentation generator with multi-format output support."""

import argparse
import json
import re
from pathlib import Path
from typing import Dict, List, Optional

try:
    import markdown
except ImportError:
    markdown = None

try:
    import yaml
except ImportError:
    yaml = None


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
        if markdown is None:
            print("Warning: markdown module not available, skipping HTML generation")
            return
            
        template = self._load_html_template()
        
        for md_file in self.source_dir.rglob("*.md"):
            relative_path = md_file.relative_to(self.source_dir)
            output_file = self.output_dir / relative_path.with_suffix(".html")
            output_file.parent.mkdir(parents=True, exist_ok=True)
            
            content = md_file.read_text()
            html_content = markdown.markdown(
                content,
                extensions=['tables', 'fenced_code', 'toc']
            )
            
            full_html = template.replace("{{content}}", html_content)
            full_html = full_html.replace("{{title}}", self._extract_title(content))
            
            output_file.write_text(full_html)
            
            self.toc_entries.append({
                "title": self._extract_title(content),
                "path": str(relative_path.with_suffix(".html")),
                "level": len(relative_path.parts) - 1
            })
        
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
        body { font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif; line-height: 1.6; padding: 2rem; }
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
    
    def _generate_pdf(self) -> None:
        """Generate PDF documentation."""
        print("PDF generation not implemented")
    
    def _generate_json(self) -> None:
        """Generate JSON representation of docs."""
        docs_data = []
        
        for md_file in self.source_dir.rglob("*.md"):
            relative_path = md_file.relative_to(self.source_dir)
            docs_data.append({
                "path": str(relative_path),
                "title": self._extract_title(md_file.read_text()),
                "content": md_file.read_text()
            })
        
        (self.output_dir / "docs.json").write_text(json.dumps(docs_data, indent=2))


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
