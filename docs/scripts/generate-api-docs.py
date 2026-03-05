#!/usr/bin/env python3
"""Generate API reference from OpenAPI specification."""

import json
import argparse
from pathlib import Path


def generate_from_openapi(spec_path: Path, output_dir: Path) -> None:
    """Generate Markdown docs from OpenAPI specification."""
    try:
        spec_data = json.loads(spec_path.read_text())
    except json.JSONDecodeError:
        raise ValueError(f"Invalid JSON in {spec_path}")
    
    output_dir.mkdir(parents=True, exist_ok=True)
    
    spec = spec_data.get('openapi') or spec_data.get('swagger')
    if not spec:
        raise ValueError("Not a valid OpenAPI specification")
    
    info = spec_data.get('info', {})
    servers = spec_data.get('servers', [])
    
    base_url = servers[0].get('url', 'https://api.example.com') if servers else ''
    
    overview = f"""# API Reference

{info.get('description', 'API documentation')}

## Base URL

```
{base_url}
```

## Authentication

"""
    
    components = spec_data.get('components', {})
    security_schemes = components.get('securitySchemes', {})
    
    if security_schemes:
        for name, scheme in security_schemes.items():
            overview += f"### {name}\n\n"
            overview += f"**Type**: {scheme.get('type', 'unknown')}\n\n"
            if scheme.get('description'):
                overview += f"{scheme['description']}\n\n"
    
    overview += "## Endpoints\n\n"
    
    paths = spec_data.get('paths', {})
    for path, methods in paths.items():
        for method, details in methods.items():
            if method.lower() in ['get', 'post', 'put', 'delete', 'patch', 'head', 'options']:
                overview += f"### {method.upper()} {path}\n\n"
                
                if details.get('summary'):
                    overview += f"**Summary**: {details['summary']}\n\n"
                
                if details.get('description'):
                    overview += f"{details['description']}\n\n"
                
                parameters = details.get('parameters', [])
                if parameters:
                    overview += "#### Parameters\n\n"
                    overview += "| Name | In | Type | Required | Description |\n"
                    overview += "|------|----|----|----|----------|\n"
                    for param in parameters:
                        name = param.get('name', '')
                        location = param.get('in', '')
                        param_type = param.get('schema', {}).get('type', 'string')
                        required = 'Yes' if param.get('required') else 'No'
                        description = param.get('description', '')
                        overview += f"| {name} | {location} | {param_type} | {required} | {description} |\n"
                    overview += "\n"
                
                request_body = details.get('requestBody')
                if request_body:
                    overview += "#### Request Body\n\n"
                    content = request_body.get('content', {})
                    for ct, details in content.items():
                        overview += f"**Content-Type**: {ct}\n\n"
                        schema = details.get('schema', {})
                        if schema:
                            if schema.get('type') == 'object' and schema.get('properties'):
                                overview += "**Properties**:\n\n"
                                overview += "| Name | Type | Required | Description |\n"
                                overview += "|------|----|----|----------|\n"
                                for prop_name, prop_schema in schema.get('properties', {}).items():
                                    prop_type = prop_schema.get('type', 'string')
                                    required = 'Yes' if prop_name in schema.get('required', []) else 'No'
                                    description = prop_schema.get('description', '')
                                    overview += f"| {prop_name} | {prop_type} | {required} | {description} |\n"
                        overview += "\n"
                
                responses = details.get('responses', {})
                if responses:
                    overview += "#### Responses\n\n"
                    for code, response in responses.items():
                        description = response.get('description', '')
                        overview += f"- **{code}**: {description}\n"
                    overview += "\n"
    
    output_file = output_dir / "reference.md"
    output_file.write_text(overview)
    
    print(f"✓ API documentation generated: {output_file}")


if __name__ == "__main__":
    parser = argparse.ArgumentParser(description="Generate API documentation")
    parser.add_argument("--spec", type=Path, required=True, help="OpenAPI spec file path")
    parser.add_argument("--output", type=Path, default=Path("docs/api"), help="Output directory")
    
    args = parser.parse_args()
    
    if not args.spec.exists():
        raise ValueError(f"Specification file not found: {args.spec}")
    
    generate_from_openapi(args.spec, args.output)
