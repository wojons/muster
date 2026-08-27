# DSL Package Examples

This document provides comprehensive examples for using the `pkg/dsl` package, which provides Starlark-based scripting and automation.

## Overview

The `dsl` package integrates the Starlark interpreter with OpenAPI CLI, allowing users to write automation scripts in a Python-like syntax that runs in a secure sandbox.

## Basic Usage

### Running a Simple Script

```go
package main

import (
    "context"
    "fmt"

    "github.com/wojons/muster/pkg/dsl"
)

func main() {
    ctx := context.Background()

    // DSL engine
    engine, err := dsl.NewEngine(&dsl.EngineOptions{
        Timeout: 30 * time.Second,
        MemoryLimit: 100 * 1024 * 1024, // 100MB
    })
    if err != nil {
        panic(err)
    }

    // Run simple script
    script := `
def main():
    return {"message": "Hello from Starlark!"}
`
    result, err := engine.Execute(ctx, script)
    if err != nil {
        panic(err)
    }

    fmt.Printf("Result: %+v\n", result)
}
```

### Built-in Functions

#### HTTP Request Function

```starlark
# api_request.starlark
def fetch_user(user_id):
    """Fetch a user by ID"""
    url = "https://api.example.com/users/" + user_id
    response = http_get(url)
    return response.json()

def create_user(username, email):
    """Create a new user"""
    url = "https://api.example.com/users"
    data = {
        "username": username,
        "email": email,
    }
    response = http_post(url, data)
    return response.json()
```

```go
// execute script
script, _ := os.ReadFile("api_request.starlark")
result, err := engine.Execute(ctx, string(script))
```

#### Environment Variables

```starlark
def get_config():
    """Get configuration from environment"""
    api_key = env_get("API_KEY", "")
    api_url = env_get("API_URL", "https://api.example.com")

    return {
        "api_key": api_key,
        "api_url": api_url,
    }
```

#### Secrets Management

```starlark
def get_auth_token():
    """Get auth token from keychain"""
    token = secret_get("auth_token")
    return bearer_token(token)
```

## Advanced Examples

### API Automation Script

```starlark
def main():
    """Import users from API"""
    # Get authentication
    api_key = env_get("API_KEY")

    # Fetch all users
    users = []
    page = 1
    while True:
        response = http_get(
            "https://api.example.com/users",
            headers = {"Authorization": "Bearer " + api_key},
            params = {"page": str(page)}
        )

        if response.status_code != 200:
            fail("Failed to fetch users")

        data = response.json()
        users.extend(data["items"])

        if len(data["items"]) == 0:
            break

        page += 1

    return {
        "imported": len(users),
        "users": users,
    }
```

### Data Transformation Pipeline

```starlark
def transform_data(raw_data):
    """Transform raw API data"""
    transformed = []

    for item in raw_data:
        transformed.append({
            "id": item["id"],
            "name": item["name"].title(),
            "email": item["email"].lower(),
            "created_at": parse_timestamp(item["created_at"]),
        })

    return transformed

def validate_user(user):
    """Validate user data"""
    if not user.get("email"):
        return False
    if "@" not in user["email"]:
        return False
    return True

def process_users():
    """Process and validate users"""
    raw_data = http_get("https://api.example.com/users")
    users = raw_data.json()

    # Transform
    transformed = transform_data(users)

    # Validate
    valid_users = [u for u in transformed if validate_user(u)]

    return {
        "total": len(users),
        "valid": len(valid_users),
        "invalid": len(users) - len(valid_users),
        "users": valid_users,
    }
```

### Batch Operations

```starlark
def batch_update_users(user_updates):
    """Batch update multiple users"""
    results = {
        "succeeded": [],
        "failed": [],
    }

    for update in user_updates:
        url = "https://api.example.com/users/" + str(update["id"])
        response = http_put(url, update["data"])

        if response.status_code == 200:
            results["succeeded"].append(update["id"])
        else:
            results["failed"].append({
                "id": update["id"],
                "error": response.text,
            })

    return results

def main():
    """Batch update example"""
    updates = [
        {"id": 1, "data": {"name": "Updated Name 1"}},
        {"id": 2, "data": {"name": "Updated Name 2"}},
        {"id": 3, "data": {"name": "Updated Name 3"}},
    ]

    return batch_update_users(updates)
```

## Error Handling

### Try-Except in Starlark

```starlark
def safe_api_call(url, retries=3):
    """Make API call with retries"""
    for i in range(retries):
        try:
            response = http_get(url)

            if response.status_code == 200:
                return response.json()
            elif response.status_code == 404:
                return None
            else:
                fail("API error: " + str(response.status_code))
        except HttpError as e:
            log_warning(f"Request failed (attempt {i + 1}): {e}")
            if i == retries - 1:
                raise

    return None
```

### Validation Errors

```starlark
def create_user(username, email):
    """Create user with validation"""
    if len(username) < 3:
        fail("Username must be at least 3 characters")

    if "@" not in email:
        fail("Invalid email address")

    url = "https://api.example.com/users"
    data = {"username": username, "email": email}

    response = http_post(url, data)

    if response.status_code != 201:
        fail("Failed to create user")

    return response.json()
```

## Working with Files

### Reading JSON Files

```starlark
def load_config(path):
    """Load configuration from JSON file"""
    content = file_read(path)
    config = json_decode(content)
    return config

def main():
    config = load_config("config.json")

    url = config.get("api_url")
    api_key = config.get("api_key")

    return {"configured": bool(url and api_key)}
```

### Writing JSON Files

```starlark
def save_results(results, path):
    """Save results to JSON file"""
    content = json_encode(results)
    file_write(path, content)

def export_users_to_json():
    """Export users to JSON file"""
    response = http_get("https://api.example.com/users")
    users = response.json()

    output = {
        "exported_at": timestamp(),
        "count": len(users),
        "users": users,
    }

    save_results(output, "users_export.json")
    return output
```

## Custom Built-in Functions

### Registering Custom Functions

```go
// Register custom functions
engine.RegisterBuiltin("hash_data", func(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
    var data string
    if err := starlark.UnpackPositionalArgs(fn.Name(), args, kwargs, 1, &data); err != nil {
        return nil, err
    }

    hash := sha256.Sum256([]byte(data))
    return starlark.String(fmt.Sprintf("%x", hash)), nil
})

// Use in script
func main():
    data = "hello world"
    hash_data(data)
```

## Resource Limits

### Memory Limits

```go
engine, err := dsl.NewEngine(&dsl.EngineOptions{
    Timeout:     30 * time.Second,
    MemoryLimit: 100 * 1024 * 1024, // 100MB
    CPULimit:    50, // 50% CPU
})
```

### Execution Timeouts

```go
ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
defer cancel()

_, err = engine.Execute(ctx, script)
if err == context.DeadlineExceeded {
    fmt.Println("Script timed out")
}
```

## Integration with OpenAPI CLI

### Generating Commands from OpenAPI

```starlark
def generate_commands_from_spec():
    """Generate CLI commands from OpenAPI spec"""
    spec = load_openapi_spec("api.yaml")

    commands = []

    for path, methods in spec["paths"].items():
        for method, details in methods.items():
            if method in ["get", "post", "put", "delete"]:
                command = {
                    "name": details.get("operationId", method + "_" + path),
                    "method": method,
                    "path": path,
                    "description": details.get("summary", ""),
                    "parameters": details.get("parameters", []),
                }
                commands.append(command)

    return commands
```

### Dynamic Command Execution

```starlark
def execute_api_command(command_name, args):
    """Execute an API command dynamically"""
    command_map = {
        "get_user": {
            "method": "GET",
            "path": "/users/{id}",
        },
        "create_user": {
            "method": "POST",
            "path": "/users",
        },
    }

    command = command_map.get(command_name)
    if not command:
        fail("Unknown command: " + command_name)

    # Build URL
    url = command_url("https://api.example.com" + command["path"], args)

    # Execute request
    if command["method"] == "GET":
        response = http_get(url, params=args)
    elif command["method"] == "POST":
        response = http_post(url, args)

    return response.json()
```

## Testing DSL Scripts

### Unit Testing Framework

```starlark
def assert_equal(actual, expected, message=""):
    """Assert equality for testing"""
    if actual != expected:
        details = message + " " if message else ""
        fail(f"Expected {expected}, got {actual}. {details}")

def test_user_creation():
    """Test user creation endpoint"""
    # Create user
    response = http_post("https://api.example.com/users", {
        "username": "testuser",
        "email": "test@example.com",
    })

    assert_equal(response.status_code, 201, "User creation failed")

    user = response.json()
    assert_equal(user["username"], "testuser")
    assert_equal(user["email"], "test@example.com")

    return {"passed": True}
```

## Best Practices

### 1. Use Proper Error Handling

```starlark
# Good
def get_user(id):
    try:
        response = http_get(f"https://api.example.com/users/{id}")
        if response.status_code == 200:
            return response.json()
        fail(f"Failed to get user: {response.status_code}")
    except HttpError as e:
        fail(f"Network error: {e}")

# Bad
def get_user(id):
    response = http_get(f"https://api.example.com/users/{id}")
    return response.json()  # No error handling
```

### 2. Validate Inputs

```starlark
def create_user(username, email):
    if not username or len(username) < 3:
        fail("Invalid username")
    if not email or "@" not in email:
        fail("Invalid email")
    # Continue
```

### 3. Use Descriptive Names

```starlark
# Good
def fetch_user_by_id(user_id):
    """Fetch a user by their unique ID"""
    url = f"https://api.example.com/users/{user_id}"
    return http_get(url)

# Bad
def f(id):
    url = f"https://api.example.com/users/{id}"
    return http_get(url)
```

### 4. Keep Scripts Modular

```starlark
# Good - Modular
def validate_user(user):
    return bool(user.get("email"))

def transform_user(user):
    return {"id": user["id"], "name": user["name"].title()}

def process_user(user):
    if validate_user(user):
        return transform_user(user)
    return None

# Bad - Monolithic
def process(user):
    if user.get("email"):
        return {"id": user["id"], "name": user["name"].title()}
```

## Complete Example

```starlark
# user_management.starlark
"""
User management automation script for OpenAPI CLI.
Handles user creation, validation, and batch operations.
"""

# Configuration
API_BASE = "https://api.example.com"
API_KEY = env_get("API_KEY", "")
DEFAULT_HEADERS = {
    "Authorization": "Bearer " + API_KEY,
    "Content-Type": "application/json",
}

def user_url(path):
    """Build full API URL"""
    return API_BASE + path

def validate_user_data(data):
    """Validate user data for creation"""
    if not data.get("username"):
        return False, "Username is required"
    if len(data["username"]) < 3:
        return False, "Username must be at least 3 characters"
    if not data.get("email"):
        return False, "Email is required"
    if "@" not in data["email"]:
        return False, "Invalid email format"
    return True, ""

def create_user(user_data):
    """Create a new user"""
    valid, error = validate_user_data(user_data)
    if not valid:
        fail(error)

    response = http_post(
        user_url("/users"),
        user_data,
        headers=DEFAULT_HEADERS,
    )

    if response.status_code != 201:
        fail(f"Failed to create user: {response.text}")

    return response.json()

def get_user(user_id):
    """Fetch a user by ID"""
    response = http_get(
        user_url(f"/users/{user_id}"),
        headers=DEFAULT_HEADERS,
    )

    if response.status_code != 200:
        fail(f"Failed to get user: {response.status_code}")

    return response.json()

def update_user(user_id, updates):
    """Update a user"""
    response = http_put(
        user_url(f"/users/{user_id}"),
        updates,
        headers=DEFAULT_HEADERS,
    )

    if response.status_code != 200:
        fail(f"Failed to update user: {response.status_code}")

    return response.json()

def list_users(limit=50, offset=0):
    """List all users"""
    response = http_get(
        user_url("/users"),
        params={"limit": str(limit), "offset": str(offset)},
        headers=DEFAULT_HEADERS,
    )

    if response.status_code != 200:
        fail(f"Failed to list users: {response.status_code}")

    return response.json()

def batch_create_users(users_data):
    """Batch create multiple users"""
    results = {
        "created": [],
        "failed": [],
    }

    for user_data in users_data:
        try:
            user = create_user(user_data)
            results["created"].append(user)
        except Exception as e:
            results["failed"].append({
                "data": user_data,
                "error": str(e),
            })

    return results

def main():
    """Main entry point"""
    print("OpenAPI CLI User Management")
    print("=" * 40)

    # Example usage
    users = [
        {"username": "alice", "email": "alice@example.com"},
        {"username": "bob", "email": "bob@example.com"},
    ]

    results = batch_create_users(users)
    print(f"Created {len(results['created'])} users")
    print(f"Failed to create {len(results['failed'])} users")

    return results
```
