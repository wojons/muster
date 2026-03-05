# Basic Hello Script (Starlark DSL)

def greet(name):
    """Return a greeting for the given name"""
    return "Hello, " + name + "!"

# Main execution
result = greet("World")
print(result)
