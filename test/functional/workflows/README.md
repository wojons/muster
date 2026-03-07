# Workflow Tests

This directory contains tests for complete user workflows through the system.

## What are Workflow Tests?

Workflow tests verify end-to-end user journeys, from start to finish. Unlike unit tests that test individual components, workflow tests test the COMPLETE user experience.

## Example User Workflows

### CLI Command Generation
1. User loads OpenAPI spec
2. User generates commands
3. User executes command
4. System returns response

### MCP Tool Execution  
1. Claude connects to MCP server
2. Claude lists available tools
3. Claude calls tool with parameters
4. System executes tool and returns result

### Authentication Flow
1. User initiates OAuth2 flow
2. User receives authorization code
3. User exchanges code for token
4. System stores token securely
5. Subsequent requests authenticated

## Guidelines

1. Test actual user workflows, not implementation details
2. Use mock servers for external services
3. Verify complete request/response cycles
4. Test error conditions realistically
5. Ensure tests complete in <5 seconds
6. Use real HTTP client calls (not mocked)

## Writing Workflow Tests

```go
func TestUserWorkflow_CLI_CommandExecution(t *testing.T) {
    // 1. Setup mock infrastructure
    server := httptest.NewServer(handler)
    defer server.Close()
    
    // 2. Perform user actions
    spec := loadSpec(server.URL)
    commands := generateCommands(spec, server.URL)
    
    // 3. Execute and verify
    result := executeCommand(commands[0], server.URL)
    require.NoError(t, result.error)
    require.NotNil(t, result.data)
    
    // 4. Complete user journey verified
}
```
