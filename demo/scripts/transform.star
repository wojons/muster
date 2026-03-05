# Data Transformation Script (Starlark DSL)

def uppercase(text):
    """Convert text to uppercase"""
    return text.upper()

def lowercase(text):
    """Convert text to lowercase"""
    return text.lower()

def reverse(text):
    """Reverse the given text"""
    return text[::-1]

# Test transformations
test = "Hello World"
print("Original:", test)
print("Uppercase:", uppercase(test))
print("Lowercase:", lowercase(test))
print("Reversed:", reverse(test))
