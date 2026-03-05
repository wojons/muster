# Starlark DSL Guide

Complete guide to using Starlark scripting language with OpenAPI CLI.

## Overview

Starlark is a Python-like scripting language (Python dialect restricted for safety) that you can use to automate API interactions with OpenAPI CLI. Starlark is sandboxed by design, making it safe for scripting API workflows.

## Getting Started

### Basic Script Structure

Create a Starlark script file (`.star` extension):

```python
#!/usr/bin/env openapi-cli dsl

# Starlark DSL script
# Make API request
pet = api.pet.find_id(pet_id=1)
print(f"Found pet: {pet['name']}")

# Update status
pet['status'] = 'sold'
api.pet.update(id=1, body=pet)

# Create new pet
new_pet = api.pet.add(body={"name":"Rex","status":"available"})
print(f"Created pet with ID: {new_pet['id']}")
```

### Running a Script

```bash
# Run a script
openapi-cli dsl run script.star --spec openapi.yaml

# Pass variables
openapi-cli dsl run script.star --spec openapi.yaml --var API_KEY=secret

# With timeout
openapi-cli dsl run automation.star --timeout 120s
```

## Built-in Functions

### HTTP Functions

```python
# Make HTTP GET request
response = http.get("https://api.example.com/v1/pets")
print(response.status)

# Load JSON
data = json.loads(response.body)

# POST request
response = http.post("https://api.example.com/v1/pets",
                     body={"name":"Rex","status":"available"},
                     headers={"Content-Type":"application/json"})
```

### API Client Functions

The Starlark environment provides direct access to your OpenAPI specification as structured functions:

```python
# API calls mirror spec structure
pet = api.pet.find_id(pet_id=1)
pets = api.pet.find_status(status="available")

# Update
api.pet.update(id=1, body={"name":"Fluffy","status":"sold"})

# Delete
api.pet.delete(pet_id=1)
```

### Utility Functions

```python
# Environment variables
api_key = env.get("API_KEY")
project_dir = env.get("PROJECT_DIR")

# Hashing
hash_val = hash.sha256("data")

# JSON operations
obj = json.loads('{"name":"Fluffy"}')
json_str = json.dumps(obj)

# Time operations
now = time.now()
formatted = time.format(now, "%Y-%m-%d")
```

## Script Examples

### Example 1: Pet Store Workflow

```python
#!/usr/bin/env openapi-cli dsl

# Find available pets
pets = api.pet.find_status(status="available")

print(f"Found {len(pets)} available pets")

# Process each pet
for pet in pets:
    print(f"Pet {pet['id']}: {pet['name']}")

    # Update status to sold
    pet['status'] = 'sold'
    api.pet.update(id=pet['id'], body=pet)

print("All pets marked as sold")
```

### Example 2: Batch API Testing

```python
#!/usr/bin/env openapi-cli dsl

# Test endpoints
test_endpoints = [
    ("GET /pet/1", lambda: api.pet.find_id(pet_id=1)),
    ("GET /pet/findByStatus", lambda: api.pet.find_status(status="available")),
    ("GET /store/inventory", lambda: api.store.inventory()),
]

success_count = 0
for name, endpoint_call in test_endpoints:
    try:
        result = endpoint_call()
        print(f"✓ {name} - Success")
        success_count += 1
    except error as e:
        print(f"✗ {name} - Error: {e}")

print(f"\nTests passed: {success_count}/{len(test_endpoints)}")
```

### Example 3: Data Migration

```python
#!/usr/bin/env openapi-cli dsl

# Load old API data
old_pets = api_old.pet.list_all()

# Iterate and migrate
for old_pet in old_pets:
    # Transform data
    new_pet = {
        "id": old_pet['id'],
        "name": old_pet['name'],
        "status": old_pet['availability'].lower()
    }

    # Create in new API
    api_new.pet.add(body=new_pet)
    print(f"Migrated pet {old_pet['id']}")

print("Migration complete")
```

### Example 4: Error Handling

```python
#!/usr/bin/env openapi-cli dsl

# Function to get pet with retries
def get_pet_with_retry(pet_id, max_retries=3):
    for attempt in range(max_retries):
        try:
            return api.pet.find_id(pet_id=pet_id)
        except error as e:
            print(f"Attempt {attempt + 1} failed: {e}")
            if attempt + 1 == max_retries:
                fail(f"Failed to get pet {pet_id} after {max_retries} attempts")
    return None

# Use the function
pet = get_pet_with_retry(1)
print(f"Retrieved pet: {pet['name']}")
```

### Example 5: Parallel Execution

```python
#!/usr/bin/env openapi-cli dsl

# Load spec URLs from environment
base_url = env.get("API_BASE_URL", "https://api.example.com/v1")

# Process items concurrently
items = [1, 2, 3, 4, 5]

results = []
for item_id in items:
    # Make request
    response = http.get(f"{base_url}/items/{item_id}")
    if response.status == 200:
        data = json.loads(response.body)
        results.append(data)

print(f"Processed {len(results)} items")
```

## Configuration

### DSL Configuration Options

```yaml
dsl:
  enabled: true
  stdlib: true                # Enable standard library
  max_execution_time: 60s    # Maximum script execution time
  allowed_imports:           # Allowed modules
    - http
    - json
    - time
    - io
  max_memory: 64MB           # Maximum memory usage
```

### Environment Variables

Pass environment variables to scripts:

```bash
# Set API key
export API_KEY=your-secret

# URL
export API_BASE_URL=https://api.example.com/v1

# Run script
openapi-cli dsl run script.star
```

Access in script:

```python
api_key = env.get("API_KEY")
base_url = env.get("API_BASE_URL")
```

## Advanced Topics

### Working with Lists

```python
# List operations
pets = api.pet.find_status(status="available")

# Find specific pet
found_pet = None
for pet in pets:
    if pet['name'] == "Fluffy":
        found_pet = pet
        break

# Transform list
names = [pet['name'] for pet in pets]

# Filter
available_pets = [p for p in pets if p['status'] == 'available']
```

### Working with Dictionaries

```python
# Create dictionary
pet_data = {
    "name": "Fluffy",
    "status": "available"
}

# Update values
pet_data["status"] = "sold"

# Check if key exists
if "name" in pet_data:
    print(pet_data["name"])

# Iterate keys
for key in pet_data:
    print(f"{key}: {pet_data[key]}")
```

### String Operations

```python
# String manipulation
name = "Fluffy"

# Concatenation
full_name = name + " the Dog"

# Slicing
short_name = name[0:3]

# Format
formatted = f"Pet name: {name}"

# Case conversion
upper_name = name.upper()
lower_name = name.lower()
```

## Error Handling

### Built-in Error Types

```python
# HTTP errors
try:
    pet = api.pet.find_id(pet_id=99999)
except error as e:
    print(f"HTTP Error: {e}")

# Validation errors
try:
    api.pet.add(body={})  # Missing required fields
except error as e:
    print(f"Validation Error: {e}")

# Timeout errors
try:
    timeout_operation()
except error as e:
    print(f"Timeout: {e}")
```

### Custom Error Handling

```python
# Define validation function
def validate_pet(pet):
    required = ["id", "name", "status"]
    for field in required:
        if field not in pet:
            fail(f"Missing required field: {field}")
    return pet

# Use in workflow
new_pet = {"name":"Fluffy"}
try:
    validated = validate_pet(new_pet)
    api.pet.add(body=validated)
except error as e:
    print(f"Validation failed: {e}")
```

## Testing Scripts

### Test Mode

```bash
# Test script (dry run)
openapi-cli dsl test script.star --spec openapi.yaml

# With verbose output
openapi-cli dsl test automation.star --verbose
```

### Writing Testable Scripts

```python
#!/usr/bin/env openapi-cli dsl

# Function to test
def update_pet_status(pet_id, new_status):
    pet = api.pet.find_id(pet_id=pet_id)
    pet['status'] = new_status
    api.pet.update(id=pet_id, body=pet)

# Test if run directly (not via dsl import)
def main():
    update_pet_status(1, "sold")
    print("Pet status updated")

# Only run main if script is executed directly
if __name__ == "__main__":
    main()
```

## Best Practices

### 1. Keep Scripts Simple

```python
# ✅ Good - Clear and simple
pet = api.pet.find_id(pet_id=1)
print(f"Pet name: {pet['name']}")

# ❌ Bad - Too complex nested logic
result = api.pet.find_id(pet_id=1) if condition else api.pet.find_by_name(name="Fluffy")
```

### 2. Use Functions

```python
# ✅ Good - Reusable function
def find_pet(pet_id):
    return api.pet.find_id(pet_id=pet_id)

# ❌ Bad - Repeated code
pet1 = api.pet.find_id(pet_id=1)
pet2 = api.pet.find_id(pet_id=2)
```

### 3. Handle Errors

```python
# ✅ Good - Error handling
try:
    pet = api.pet.find_id(pet_id=1)
    print(f"Pet: {pet['name']}")
except error as e:
    print(f"Error: {e}")

# ❌ Bad - No error handling
pet = api.pet.find_id(pet_id=1)
print(f"Pet: {pet['name']}")
```

### 4. Use Comments

```python
# ✅ Good - Documented
# Load pet and update status
pet = api.pet.find_id(pet_id=1)
pet['status'] = 'sold'
api.pet.update(id=1, body=pet)

# ❌ Bad - Unclear intent
result = api.pet.find_id(pet_id=1)
result['status'] = 'sold'
api.pet.update(id=1, body=result)
```

### 5. Use Environment Variables

```python
# ✅ Good - Configurable
api_key = env.get("API_KEY")
base_url = env.get("API_BASE_URL")

# ❌ Bad - Hardcoded values
api_key = "secret-key-12345"
base_url = "https://api.example.com"
```

## Resources

- [Starlark Language Spec](https://github.com/bazelbuild/starlark)
- [OpenAPI Documentation](../api/README.md) - API reference
- [Configuration Guide](configuration.md) - DSL configuration
- [Command Reference](commands.md) - DSL commands

## Common Patterns

### Pattern 1: Batch Processing

```python
# Process multiple items
items = api.items.list_all()

for item in items:
    # Process item
    process(item)

    # Update status
    api.items.update(id=item['id'], body={"status":"processed"})
```

### Pattern 2: Data Validation

```python
# Validate before API call
def validate(item):
    if not item['name']:
        fail("Name is required")
    if item['status'] not in ['available', 'pending']:
        fail("Invalid status")
    return item

# Use
validated = validate(new_item)
api.items.add(body=validated)
```

### Pattern 3: Retry Pattern

```python
# Retry on failure
def with_retry(fn, max_attempts=3):
    for attempt in range(max_attempts):
        try:
            return fn()
        except error as e:
            if attempt + 1 == max_attempts:
                raise
            print(f"Attempt {attempt + 1} failed, retrying...")

# Use
result = with_retry(lambda: api.pet.find_id(pet_id=1))
```

## Troubleshooting

### Script Won't Run

**Issue**: `Error: Script execution failed`

**Solution**: Check script syntax and indentation. Starlark uses 2-space or 4-space indentation (no tabs).

### Import Errors

**Issue**: `Error: Module 'http' not found`

**Solution**: Ensure the module is in the allowed_imports list:

```yaml
dsl:
  allowed_imports:
    - http
```

### Timeout Issues

**Issue**: `Error: Script execution timeout`

**Solution**: Increase timeout in configuration:

```bash
openapi-cli dsl run script.star --timeout 180s
```

## Related Documentation

- [Configuration Guide](configuration.md) - DSL configuration options
- [Command Reference](commands.md) - DSL commands
- [API Documentation](../api/README.md) - Complete API reference
- [Troubleshooting Guide](troubleshooting.md) - Common DSL issues
