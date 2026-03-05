# Epic 3: HTTP Request Execution

## Overview
Build the execution engine that handles HTTP requests with proper error handling, retries, authentication, and response processing.

## Business Value
- Ensures reliable API communication
- Reduces model token usage by handling complexity internally
- Provides consistent error handling across all APIs

## Acceptance Criteria
1. Executes GET, POST, PUT, DELETE, PATCH requests
2. Handles request/response headers
3. Supports request body serialization (JSON, form-data)
4. Implements retry logic with exponential backoff
5. Handles authentication (Bearer, API Key, Basic Auth)
6. Validates responses against OpenAPI schemas
7. Formats output as JSON, table, or raw
8. Shows progress for long-running requests

## Technical Requirements
- HTTP client with timeout and cancellation
- Configurable retry strategies
- Auth middleware for different schemes
- Response validation using JSON Schema
- Output formatting with table support
- Streaming for large responses

## Dependencies
- Epic 1: Spec Ingestion (needs operations to execute)
- Epic 2: CLI Generation (needs commands to trigger execution)

## Stories
- Story 3.1: HTTP client setup
- Story 3.2: Request builder from operation
- Story 3.3: Response handler and formatter
- Story 3.4: Retry logic implementation
- Story 3.5: Authentication middleware
- Story 3.6: Request/response validation
- Story 3.7: Output formatting (JSON, table, yaml)

## Estimation
- **Story Points**: 13
- **Duration**: 2-3 weeks

## Risks
- Different APIs have varying auth schemes
- Large response bodies may cause memory issues
- Timeout handling across different network conditions

## Notes
- Use axios for HTTP client (well-tested, interceptor support)
- Consider pluggable auth providers
- Add request/response logging for debugging
