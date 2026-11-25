# Starlark DSL Guide

Complete guide to using Starlark scripting language with OpenAPI CLI.

## Overview

The OpenAPI CLI includes a Starlark scripting engine for automating API interactions. Starlark is a Python-like language (a restricted Python dialect) that is deterministic and sandboxed by design.

The `dsl run` command executes a `.star` script with four HTTP builtins wired in. Everything else available to a script is the standard Starlark language and its core builtins — the interpreter does not load your OpenAPI specification, does not expose an `api.*` client, and does not bridge environment variables or JSON/time modules into the script environment.

## Command Reference

The `dsl` command is defined in `internal/builtin/dsl.go` and has exactly one form and one flag:

```bash
openapi-cli dsl run <file.star> [--verbose | -V]
```

| Flag | Short | Effect |
|------|-------|--------|
| `--verbose` | `-V` | Print all global variable bindings after script execution |

There are no other subcommands or flags:

- No `dsl test` subcommand exists.
- There is no `--spec` flag — the script environment does not load an OpenAPI specification.
- There is no `--var` flag — values are passed by defining variables in the script itself.
- There is no `--timeout` flag — scripts run until they finish or fail.

## Getting Started

### Basic Script Structure

Create a Starlark script file (`.star` extension):

```python
# A Starlark DSL script: define variables, functions,
# and call the HTTP builtins.

def describe(status, body):
    return "status=" + str(status) + " body=" + body

# Fetch data from an API
result = http_get("https://api.example.com/v1/pets/1")
print(describe(result[0], result[1]))
```

### Running a Script

```bash
# Run a script
openapi-cli dsl run demo/scripts/hello.star

# Run a script and print every global variable binding afterwards
openapi-cli dsl run demo/scripts/hello.star --verbose

# Short form of the verbose flag
openapi-cli dsl run demo/scripts/hello.star -V
```

`print()` output goes to plain stdout. With `--verbose`, each global variable defined by the script is printed as `name = value` after execution.

## Built-in Functions

### HTTP Functions

Four HTTP builtins are provided by the CLI. Each returns a tuple of `(status_code, body)`:

```python
# GET — returns (status_code, body)
status, body = http_get("https://api.example.com/v1/pets")
print(status)   # e.g. 200
print(body)     # response body as a string

# POST — sends the body with Content-Type: application/json
status, body = http_post("https://api.example.com/v1/pets", "{\"name\":\"Rex\"}")

# The body argument is optional for POST and PUT (an empty string is sent)
status, body = http_post("https://api.example.com/v1/ping")

# PUT — sends the body with Content-Type: application/json
status, body = http_put("https://api.example.com/v1/pets/1", "{\"status\":\"sold\"}")

# DELETE
status, body = http_delete("https://api.example.com/v1/pets/1")
```

If a request fails at the transport level (DNS failure, connection refused), the builtin returns an error and the script aborts. A completed HTTP request with a non-2xx status code is *not* an error — check `status` in your script.

### What Is NOT Provided

The following do **not** exist in the current implementation — scripts that use them will fail:

- No `api.pet.find_id(...)` style client functions. The script environment does not load an OpenAPI specification, so spec-derived operations cannot be called. If you need spec-driven requests, use the CLI's generated commands instead of the DSL, or build the request URL in the script.
- No `env.get("NAME")` — environment variables are not bridged into scripts. Use literal values or define them at the top of the script.
- No `json.loads` / `json.dumps` module.
- No `time.now()` or date/time functions.
- `hash(x)` is the Starlark builtin that hashes a value for use in sets/dicts — it is **not** SHA-256 and not usable for checksums.

### Starlark Core Builtins

Scripts run on the standard Starlark dialect (go.starlark.net) with no extra modules predeclared. Available core builtins include:

```python
# Types and construction
n = int("42")
s = str(99)
f = float("2.5")
d = dict(pairs=[("a", 1)])
t = tuple([1, 2, 3])
l = list(range(5))

# Aggregates and checks
total = len(l)
flag = all([True, False]) or any([True, False])

# Math
biggest = max(1, 2, 3)
smallest = min(4, 5, 6)
absolute = abs(-7)

# Iteration helpers
for i, name in enumerate(["a", "b"]):
    print(i, name)

# Sorting and reversing
sorted_names = sorted(["carl", "anna", "bea"])
backwards = reversed([1, 2, 3])

# Strings
upper = "hi".upper()
lower = "HI".lower()
parts = "a,b,c".split(",")
joined = "-".join(["x", "y"])
formatted = "pet={}".format("Rex")
starts = "hello".startswith("he")

# fail() aborts the script with an error message
# fail("invalid input: " + repr(value))
```

The full Starlark language reference: [Starlark Language Spec](https://github.com/bazelbuild/starlark).

Note that Starlark is more restricted than Python: there is no `try`/`except`, no f-strings, no `raise`, no lambdas, and no `while True` unbounded recursion.

## Script Examples

The runnable example scripts live in `demo/scripts/`: `hello.star`, `transform.star`, and `workflow.star`.

### Example 1: Fetch and Report

```python
# Fetch a list of pets and report the outcome
status, body = http_get("https://api.example.com/v1/pets")

if status == 200:
    print("Fetched pets OK")
    print("Body length:", len(body))
else:
    print("Request failed with status", status)
    fail("aborting: /pets returned " + str(status))
```

### Example 2: Batch Endpoint Checks

```python
# Check several endpoints and count successes
endpoints = [
    "https://api.example.com/v1/pets/1",
    "https://api.example.com/v1/pets/findByStatus?status=available",
    "https://api.example.com/v1/store/inventory",
]

success_count = 0
for url in endpoints:
    status, body = http_get(url)
    if status == 200:
        print("OK   " + url)
        success_count = success_count + 1
    else:
        print("FAIL " + url + " status=" + str(status))

print("Checks passed: " + str(success_count) + "/" + str(len(endpoints)))
```

### Example 3: Create a Resource

```python
# Create a pet, then confirm it can be fetched
payload = "{\"name\":\"Rex\",\"status\":\"available\"}"
status, body = http_post("https://api.example.com/v1/pets", payload)

if status != 200:
    fail("create failed with status " + str(status))

# Update the created pet's status
status, body = http_put("https://api.example.com/v1/pets/1", "{\"status\":\"sold\"}")
print("update status:", status)
```

### Example 4: Retrying on Non-2xx Responses

There is no `try`/`except` in Starlark, so retry loops must key off return values rather than exceptions. A transport-level failure aborts the script; the pattern below retries *unsuccessful HTTP statuses*:

```python
# Retry an endpoint until it answers 200 (bounded attempts)
max_attempts = 3

for attempt in range(1, max_attempts + 1):
    status, body = http_get("https://api.example.com/v1/health")
    if status == 200:
        print("healthy on attempt", attempt)
        break
    print("attempt " + str(attempt) + " got status " + str(status))

if status != 200:
    fail("endpoint unhealthy after " + str(max_attempts) + " attempts")
```

### Example 5: Verbose Variable Dump

The existing `demo/scripts/workflow.star` defines globals that never reach `print()`. Run it with `--verbose` to inspect them:

```bash
$ openapi-cli dsl run demo/scripts/workflow.star --verbose
Input: hello world
Output: HELLO WORLD
Success: Data validated and transformed
input_data = "hello world"
output = "HELLO WORLD"
transform_data = <function transform_data>
validate_data = <function validate_data>
```

All global bindings — functions as well as values — are listed after the script's own output (binding order may vary between runs). Without `--verbose`, only the `print()` lines appear.

## Configuration

### No Script-Level Configuration

The `dsl run` command reads no configuration: there is no `dsl:` config section, no execution-time limit, no memory cap, and no import allowlist for scripts (the `dsl:` block shown in [Configuration Guide](configuration.md) is not consumed by the `dsl run` command).

Behavior is fixed by the implementation:

- `print()` goes to plain stdout.
- Extra script output (only with `--verbose`): global variable bindings.
- No timeout: a script runs until it completes or an error aborts it.

### Environment Variables

The DSL runtime does **not** read environment variables into the script environment, and there is no `--var` flag. To parameterize a script, define variables at the top of the file:

```python
# check.star — edit these values per environment
base_url = "https://staging.example.com/v1"
api_key = "set-me"
```

Keep secrets out of committed scripts: define them in a local, untracked copy of the script.

## Advanced Topics

### Working with Lists

```python
status, body = http_get("https://api.example.com/v1/pets")

# List operations (pure Starlark)
names = ["anna", "carl", "bea"]
found = None
for name in names:
    if name == "carl":
        found = name
        break

# Transform a list
lengths = [len(name) for name in names]

# Filter
short_names = [n for n in names if len(n) < 5]
```

### Working with Dictionaries

```python
pet_data = {
    "name": "Fluffy",
    "status": "available",
}

# Update values
pet_data["status"] = "sold"

# Check if key exists
if "name" in pet_data:
    print(pet_data["name"])

# Iterate keys and values
for key in pet_data.keys():
    print(key, pet_data[key])

# Safe lookup
status_code = pet_data.get("status", "unknown")
```

### String Operations

```python
name = "Fluffy"

# Concatenation
full_name = name + " the Cat"

# Slicing
short_name = name[0:3]

# Formatting
formatted = "Pet name: {}".format(name)

# Case conversion
upper_name = name.upper()
lower_name = name.lower()
```

Note: there are no f-strings in Starlark — use `+` concatenation or `.format()`.

## Error Handling

Starlark has no `try`/`except`. Errors are handled by checking return values; an unhandled error aborts the script with a non-zero exit.

### Script Errors

```python
# Parse errors, execution errors, and failed builtin calls
# all abort the script. fail() raises one deliberately:

def require(condition, message):
    if not condition:
        fail(message)
    return condition

require(len("Rex") > 0, "name must not be empty")
```

### HTTP Status Handling

HTTP requests that complete are not errors, even for 4xx/5xx statuses — check the status code:

```python
status, body = http_get("https://api.example.com/v1/pets/99999")
if status != 200:
    print("Request failed with status:", status)
    print("Response body:", body)

# Validation errors are yours to detect:
payload = "{}"  # missing required fields
status, body = http_post("https://api.example.com/v1/pets", payload)
if status >= 400:
    print("Server rejected payload, status:", status)
```

### Custom Validation

```python
def validate_pet(pet):
    required = ["id", "name", "status"]
    for field in required:
        if field not in pet.keys():
            fail("Missing required field: " + field)
    return pet

new_pet = {"name": "Fluffy"}
# validate_pet(new_pet) would abort the script:
# "Missing required field: id"
```

## Testing Scripts

There is no `dsl test` subcommand. To test a script, run it with `dsl run` and inspect its stdout and exit code:

```bash
# Run the script normally
openapi-cli dsl run demo/scripts/hello.star

# Run with a dump of all global bindings — useful as a smoke check
openapi-cli dsl run demo/scripts/hello.star --verbose
```

### Writing Testable Scripts

Structure scripts so the interesting logic is in functions, and keep I/O calls explicit:

```python
# Functions that compute are easy to exercise with --verbose
def build_url(base, path):
    return base + "/" + path

def is_ok(status):
    return status >= 200 and status < 300

# Calls that touch the network stay at the bottom of the script
status, body = http_get(build_url("https://api.example.com/v1", "pets"))
print("ok:", is_ok(status))
```

Run with `--verbose` to see every binding, including intermediate values your functions produced.

## Best Practices

### 1. Keep Scripts Simple

```python
# Good - clear and linear
status, body = http_get("https://api.example.com/v1/pets/1")
print("status:", status)

# Avoid - dense one-liners hide control flow
ok = http_get("https://api.example.com/v1/pets/1")[0] == 200 and len(http_get("https://api.example.com/v1/pets/1")[1]) > 0
```

### 2. Use Functions

```python
# Good - reusable function
def fetch_pet(base_url, pet_id):
    return http_get(base_url + "/pets/" + str(pet_id))

status, body = fetch_pet("https://api.example.com/v1", 1)
status, body = fetch_pet("https://api.example.com/v1", 2)

# Avoid - repeated inline calls with drifted URLs
status1, body1 = http_get("https://api.example.com/v1/pets/1")
status2, body2 = http_get("https://api.example.com/v1/pet/2")
```

### 3. Check HTTP Statuses

```python
# Good - explicit status handling
status, body = http_get("https://api.example.com/v1/pets/1")
if status != 200:
    fail("unexpected status: " + str(status))
print("Pet:", body)

# Avoid - assuming success
status, body = http_get("https://api.example.com/v1/pets/1")
print("Pet:", body)  # body may hold an error document
```

### 4. Use Comments

```python
# Good - documented
# Fetch the pet, then flip its status to sold
status, body = http_get("https://api.example.com/v1/pets/1")
status, body = http_put("https://api.example.com/v1/pets/1", "{\"status\":\"sold\"}")

# Avoid - unclear intent
result = http_get("https://api.example.com/v1/pets/1")
result2 = http_put("https://api.example.com/v1/pets/1", "{\"status\":\"sold\"}")
```

### 5. Keep Secrets Out of Scripts

There is no environment-variable bridge and no `--var` flag, so anything written in the script is visible to anyone who can read the file.

```python
# Avoid - hardcoded secret in a committed file
api_key = "secret-key-12345"

# Better - keep the real value in a local untracked copy,
# or feed it from the API layer that invokes the script
```

## Resources

- [Starlark Language Spec](https://github.com/bazelbuild/starlark)
- [Configuration Guide](configuration.md) - CLI configuration
- [Command Reference](commands.md) - DSL commands

## Common Patterns

### Pattern 1: Batch Processing

```python
# Process multiple items with one request per item
item_ids = [1, 2, 3, 4, 5]

for item_id in item_ids:
    status, body = http_get("https://api.example.com/v1/items/" + str(item_id))
    if status != 200:
        print("item " + str(item_id) + " unavailable, skipping")
        continue
    update = http_put(
        "https://api.example.com/v1/items/" + str(item_id),
        "{\"status\":\"processed\"}",
    )
    print("item " + str(item_id) + " update status:", update[0])
```

### Pattern 2: Data Validation

```python
# Validate inputs before spending a request
def validate(item):
    if not item["name"]:
        fail("Name is required")
    if not (item["status"] == "available" or item["status"] == "pending"):
        fail("Invalid status")
    return item

new_item = {"name": "widget", "status": "available"}
validated = validate(new_item)
payload = "{\"name\":\"" + validated["name"] + "\",\"status\":\"" + validated["status"] + "\"}"
status, body = http_post("https://api.example.com/v1/items", payload)
print("create status:", status)
```

### Pattern 3: Fail Fast on Unhealthy Dependencies

```python
# Abort early if a dependency is down (no exception handling available,
# so gate on the status tuple)
status, body = http_get("https://api.example.com/health")
if status != 200:
    fail("dependency unhealthy, status=" + str(status))

# ... continue with the real workflow
```

## Troubleshooting

### Script Won't Run

**Issue**: `Error: parse error` or `Error: execution error`

**Solution**: Check script syntax and indentation. Starlark is stricter than Python — no `try`/`except`, no f-strings, no lambdas. Use spaces for indentation.

### Unknown Name Errors

**Issue**: `Error: execution error: ... undefined: api` (or `env`, `json`, `time`)

**Solution**: The script environment provides only the four HTTP builtins and the Starlark core. There is no `api.*` client, no environment-variable bridge, and no JSON or time modules — see [What Is NOT Provided](#what-is-not-provided).

### Too Many Arguments

**Issue**: `Error: accepts 2 arg(s), received 1`

**Solution**: `dsl run` requires both the subcommand and the script path: `openapi-cli dsl run demo/scripts/hello.star`, not `openapi-cli dsl demo/scripts/hello.star`.

### Script Hangs

**Issue**: A script never finishes (for example, an unbounded loop waiting on an endpoint that never answers).

**Solution**: There is no `--timeout` flag. Re-run after killing the process, and bound all loops explicitly:

```python
max_attempts = 5
for attempt in range(max_attempts):
    ...
```

## Related Documentation

- [Configuration Guide](configuration.md) - CLI configuration options
- [Command Reference](commands.md) - DSL commands
- [API Documentation](../api/README.md) - Complete API reference
- [Troubleshooting Guide](troubleshooting.md) - Common CLI issues
