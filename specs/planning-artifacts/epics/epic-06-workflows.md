# Epic 6: Multi-Step Workflows

## Overview
Enable complex API workflows requiring multiple requests, response extraction, conditional logic, and state management.

## Business Value
- Handles complex real-world scenarios
- Reduces token usage by automating multi-step processes
- Enables sophisticated automation without scripting

## Acceptance Criteria
1. Define workflows in YAML/JSON format
2. Support request chaining (use response from step N in step N+1)
3. Conditional execution based on response data
4. Loop over paginated responses
5. Error handling and rollback
6. State persistence across workflow steps
7. Workflow validation before execution

## Technical Requirements
- Workflow DSL (Domain Specific Language)
- JSONPath or JMESPath for data extraction
- State machine implementation
- Transaction rollback support
- Parallel execution support
- Workflow debugging and dry-run

## Dependencies
- Epic 3: Request Execution (workflows compose requests)
- Epic 4: Module Registry (complex workflows may need modules)

## Stories
- Story 6.1: Workflow definition format
- Story 6.2: Request chaining implementation
- Story 6.3: Data extraction (JSONPath/JMESPath)
- Story 6.4: Conditional logic support
- Story 6.5: Pagination handling
- Story 6.6: State management
- Story 6.7: Error handling and rollback

## Estimation
- **Story Points**: 13
- **Duration**: 2-3 weeks

## Risks
- Workflow format complexity vs power tradeoff
- Debugging workflows can be difficult
- State management across distributed workflows

## Notes
- Start with simple sequential workflows
- Consider visual workflow builder in future
- Allow workflows to be shared as modules
