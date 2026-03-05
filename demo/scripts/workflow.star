# Multi-step Workflow Script (Starlark DSL)

def transform_data(data):
    """Transform data to uppercase"""
    return data.upper()

def validate_data(data):
    """Validate data has at least 3 characters"""
    return len(data) >= 3

# Workflow
input_data = "hello world"
print("Input:", input_data)

if validate_data(input_data):
    output = transform_data(input_data)
    print("Output:", output)
    print("Success: Data validated and transformed")
else:
    print("Error: Data validation failed")
