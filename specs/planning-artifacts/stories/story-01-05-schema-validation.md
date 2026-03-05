# Story 1.5: Schema Validation

## Epic
Epic 1: OpenAPI Spec Ingestion

## Story
As a user, I want the CLI to validate OpenAPI specifications so that I get clear error messages if the spec is invalid.

## Acceptance Criteria
- [ ] Validate spec against OpenAPI 3.0.x schema
- [ ] Validate spec against OpenAPI 3.1.x schema
- [ ] Detect and report schema version
- [ ] Provide detailed error messages with line numbers
- [ ] Support both strict and lenient validation modes
- [ ] Validate required fields (openapi, info, paths)
- [ ] Warn about non-critical issues

## Technical Notes
- Use AJV for JSON Schema validation
- Bundle OpenAPI schemas as JSON files
- Implement custom error formatter
- Support validation of individual operations

## Estimation
- **Story Points**: 3
- **Duration**: 2-3 days

## Dependencies
- Story 1.4: YAML/JSON Parser

## Test Cases
1. Valid OpenAPI 3.0 spec passes
2. Valid OpenAPI 3.1 spec passes
3. Missing required field detected
4. Invalid type detected
5. Unknown field warning in lenient mode
6. Unknown field error in strict mode
7. Invalid reference detected
