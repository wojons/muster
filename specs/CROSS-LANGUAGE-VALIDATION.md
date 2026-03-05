# Cross-Language Validation Specification

**Status**: ✅ COMPLETE  
**Version**: 1.0.0  
**Last Updated**: 2026-03-02  

## Overview

This specification defines validation standards and patterns that must be consistently implemented across all programming languages used in the Muster system. Cross-language validation ensures data integrity, protocol compatibility, and consistent behavior regardless of the implementation language.

### Design Goals

- **Consistency**: Identical validation logic across all languages
- **Compatibility**: Valid data in one language is valid in all languages
- **Performance**: Efficient validation without excessive overhead
- **Clarity**: Clear error messages for all validation failures
- **Testability**: Comprehensive test coverage for validation rules

## Validation Framework

### Core Validation Principles

1. **Fail Fast**: Validate as early as possible in the data flow
2. **Be Strict**: Reject ambiguous or malformed data
3. **Provide Context**: Include field names and expected formats in errors
4. **Standardize**: Use consistent validation patterns across languages

### Validation Pipeline

```
Input Data
    ↓
[Syntax Validation] - JSON structure, encoding
    ↓
[Type Validation] - Field types match schema
    ↓
[Constraint Validation] - Min/max, patterns, enums
    ↓
[Business Logic Validation] - Domain-specific rules
    ↓
[Cross-Reference Validation] - Foreign keys, dependencies
    ↓
Validated Data
```

## Common Validation Patterns

### String Validation

#### Pattern Definition

| Constraint | Description | Example |
|------------|-------------|---------|
| `min_length` | Minimum character count | `min_length: 1` |
| `max_length` | Maximum character count | `max_length: 255` |
| `pattern` | Regular expression | `pattern: "^[a-z0-9-]+$"` |
| `format` | Predefined format | `format: "email"` |
| `enum` | Allowed values | `enum: ["active", "inactive"]` |

#### String Formats

| Format | Pattern | Example |
|--------|---------|---------|
| `email` | RFC 5322 compliant | `user@example.com` |
| `uuid` | UUID v4 | `550e8400-e29b-41d4-a716-446655440000` |
| `hostname` | RFC 1123 | `api.example.com` |
| `uri` | RFC 3986 | `https://example.com/path` |
| `datetime` | ISO 8601 | `2024-01-15T10:30:00Z` |
| `base64` | Base64 encoded | `SGVsbG8gV29ybGQ=` |

### Number Validation

| Constraint | Description | Example |
|------------|-------------|---------|
| `minimum` | Minimum value | `minimum: 0` |
| `maximum` | Maximum value | `maximum: 100` |
| `exclusive_minimum` | Strictly greater than | `exclusive_minimum: 0` |
| `exclusive_maximum` | Strictly less than | `exclusive_maximum: 100` |
| `multiple_of` | Divisible by | `multiple_of: 10` |

### Array Validation

| Constraint | Description | Example |
|------------|-------------|---------|
| `min_items` | Minimum item count | `min_items: 1` |
| `max_items` | Maximum item count | `max_items: 100` |
| `unique_items` | All items unique | `unique_items: true` |
| `items` | Schema for items | `items: {type: "string"}` |

### Object Validation

| Constraint | Description | Example |
|------------|-------------|---------|
| `required` | Required fields | `required: ["id", "name"]` |
| `properties` | Field schemas | `properties: {...}` |
| `additional_properties` | Allow extra fields | `additional_properties: false` |

## Language-Specific Implementations

### Go Implementation

```go
package validation

import (
    "fmt"
    "regexp"
    "strings"
    "time"
    "unicode/utf8"
)

// ValidationError represents a validation failure
type ValidationError struct {
    Field   string `json:"field"`
    Code    string `json:"code"`
    Message string `json:"message"`
    Value   interface{} `json:"value,omitempty"`
}

func (e *ValidationError) Error() string {
    return fmt.Sprintf("%s: %s", e.Field, e.Message)
}

// Validator is the interface for all validators
type Validator interface {
    Validate(value interface{}) *ValidationError
}

// StringValidator validates string constraints
type StringValidator struct {
    MinLength *int
    MaxLength *int
    Pattern   *regexp.Regexp
    Format    string
    Enum      []string
}

func (v *StringValidator) Validate(value interface{}) *ValidationError {
    str, ok := value.(string)
    if !ok {
        return &ValidationError{
            Field:   "",
            Code:    "TYPE_ERROR",
            Message: "expected string",
            Value:   value,
        }
    }
    
    length := utf8.RuneCountInString(str)
    
    if v.MinLength != nil && length < *v.MinLength {
        return &ValidationError{
            Field:   "",
            Code:    "MIN_LENGTH",
            Message: fmt.Sprintf("minimum length is %d", *v.MinLength),
            Value:   str,
        }
    }
    
    if v.MaxLength != nil && length > *v.MaxLength {
        return &ValidationError{
            Field:   "",
            Code:    "MAX_LENGTH",
            Message: fmt.Sprintf("maximum length is %d", *v.MaxLength),
            Value:   str,
        }
    }
    
    if v.Pattern != nil && !v.Pattern.MatchString(str) {
        return &ValidationError{
            Field:   "",
            Code:    "PATTERN",
            Message: "does not match required pattern",
            Value:   str,
        }
    }
    
    if v.Format != "" {
        if err := validateFormat(str, v.Format); err != nil {
            return &ValidationError{
                Field:   "",
                Code:    "FORMAT",
                Message: err.Error(),
                Value:   str,
            }
        }
    }
    
    if len(v.Enum) > 0 && !contains(v.Enum, str) {
        return &ValidationError{
            Field:   "",
            Code:    "ENUM",
            Message: fmt.Sprintf("must be one of: %s", strings.Join(v.Enum, ", ")),
            Value:   str,
        }
    }
    
    return nil
}

// NumberValidator validates number constraints
type NumberValidator struct {
    Minimum          *float64
    Maximum          *float64
    ExclusiveMinimum *float64
    ExclusiveMaximum *float64
    MultipleOf       *float64
}

func (v *NumberValidator) Validate(value interface{}) *ValidationError {
    var num float64
    switch n := value.(type) {
    case int:
        num = float64(n)
    case int64:
        num = float64(n)
    case float32:
        num = float64(n)
    case float64:
        num = n
    default:
        return &ValidationError{
            Field:   "",
            Code:    "TYPE_ERROR",
            Message: "expected number",
            Value:   value,
        }
    }
    
    if v.Minimum != nil && num < *v.Minimum {
        return &ValidationError{
            Field:   "",
            Code:    "MINIMUM",
            Message: fmt.Sprintf("minimum value is %v", *v.Minimum),
            Value:   num,
        }
    }
    
    if v.Maximum != nil && num > *v.Maximum {
        return &ValidationError{
            Field:   "",
            Code:    "MAXIMUM",
            Message: fmt.Sprintf("maximum value is %v", *v.Maximum),
            Value:   num,
        }
    }
    
    if v.ExclusiveMinimum != nil && num <= *v.ExclusiveMinimum {
        return &ValidationError{
            Field:   "",
            Code:    "EXCLUSIVE_MINIMUM",
            Message: fmt.Sprintf("must be greater than %v", *v.ExclusiveMinimum),
            Value:   num,
        }
    }
    
    if v.ExclusiveMaximum != nil && num >= *v.ExclusiveMaximum {
        return &ValidationError{
            Field:   "",
            Code:    "EXCLUSIVE_MAXIMUM",
            Message: fmt.Sprintf("must be less than %v", *v.ExclusiveMaximum),
            Value:   num,
        }
    }
    
    if v.MultipleOf != nil && int64(num/(*v.MultipleOf))*int64(*v.MultipleOf) != int64(num) {
        return &ValidationError{
            Field:   "",
            Code:    "MULTIPLE_OF",
            Message: fmt.Sprintf("must be multiple of %v", *v.MultipleOf),
            Value:   num,
        }
    }
    
    return nil
}

// ArrayValidator validates array constraints
type ArrayValidator struct {
    MinItems     *int
    MaxItems     *int
    UniqueItems  bool
    ItemValidator Validator
}

func (v *ArrayValidator) Validate(value interface{}) *ValidationError {
    arr, ok := value.([]interface{})
    if !ok {
        return &ValidationError{
            Field:   "",
            Code:    "TYPE_ERROR",
            Message: "expected array",
            Value:   value,
        }
    }
    
    if v.MinItems != nil && len(arr) < *v.MinItems {
        return &ValidationError{
            Field:   "",
            Code:    "MIN_ITEMS",
            Message: fmt.Sprintf("minimum %d items required", *v.MinItems),
            Value:   len(arr),
        }
    }
    
    if v.MaxItems != nil && len(arr) > *v.MaxItems {
        return &ValidationError{
            Field:   "",
            Code:    "MAX_ITEMS",
            Message: fmt.Sprintf("maximum %d items allowed", *v.MaxItems),
            Value:   len(arr),
        }
    }
    
    if v.UniqueItems {
        seen := make(map[string]bool)
        for _, item := range arr {
            key := fmt.Sprintf("%v", item)
            if seen[key] {
                return &ValidationError{
                    Field:   "",
                    Code:    "UNIQUE_ITEMS",
                    Message: "duplicate items found",
                }
            }
            seen[key] = true
        }
    }
    
    if v.ItemValidator != nil {
        for i, item := range arr {
            if err := v.ItemValidator.Validate(item); err != nil {
                err.Field = fmt.Sprintf("[%d].%s", i, err.Field)
                return err
            }
        }
    }
    
    return nil
}

// Format validation functions
func validateFormat(value string, format string) error {
    switch format {
    case "email":
        return validateEmail(value)
    case "uuid":
        return validateUUID(value)
    case "hostname":
        return validateHostname(value)
    case "uri":
        return validateURI(value)
    case "datetime":
        return validateDateTime(value)
    case "base64":
        return validateBase64(value)
    default:
        return fmt.Errorf("unknown format: %s", format)
    }
}

var emailRegex = regexp.MustCompile(`^[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}$`)

func validateEmail(value string) error {
    if !emailRegex.MatchString(value) {
        return fmt.Errorf("invalid email format")
    }
    return nil
}

var uuidRegex = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)

func validateUUID(value string) error {
    if !uuidRegex.MatchString(strings.ToLower(value)) {
        return fmt.Errorf("invalid UUID v4 format")
    }
    return nil
}

var hostnameRegex = regexp.MustCompile(`^([a-zA-Z0-9]|[a-zA-Z0-9][a-zA-Z0-9\-]{0,61}[a-zA-Z0-9])(\.([a-zA-Z0-9]|[a-zA-Z0-9][a-zA-Z0-9\-]{0,61}[a-zA-Z0-9]))*$`)

func validateHostname(value string) error {
    if len(value) > 253 || !hostnameRegex.MatchString(value) {
        return fmt.Errorf("invalid hostname format")
    }
    return nil
}

func validateURI(value string) error {
    // Basic URI validation
    if !strings.Contains(value, "://") {
        return fmt.Errorf("invalid URI format")
    }
    return nil
}

func validateDateTime(value string) error {
    _, err := time.Parse(time.RFC3339, value)
    if err != nil {
        return fmt.Errorf("invalid datetime format, expected RFC 3339")
    }
    return nil
}

func validateBase64(value string) error {
    if _, err := base64.StdEncoding.DecodeString(value); err != nil {
        return fmt.Errorf("invalid base64 encoding")
    }
    return nil
}

func contains(slice []string, item string) bool {
    for _, s := range slice {
        if s == item {
            return true
        }
    }
    return false
}
```

### TypeScript Implementation

```typescript
// validation/types.ts

export interface ValidationError {
  field: string;
  code: string;
  message: string;
  value?: unknown;
}

export interface StringConstraints {
  minLength?: number;
  maxLength?: number;
  pattern?: RegExp;
  format?: 'email' | 'uuid' | 'hostname' | 'uri' | 'datetime' | 'base64';
  enum?: string[];
}

export interface NumberConstraints {
  minimum?: number;
  maximum?: number;
  exclusiveMinimum?: number;
  exclusiveMaximum?: number;
  multipleOf?: number;
}

export interface ArrayConstraints {
  minItems?: number;
  maxItems?: number;
  uniqueItems?: boolean;
  itemValidator?: Validator;
}

export type Validator = (value: unknown) => ValidationError | null;

// validation/string-validator.ts

export function createStringValidator(constraints: StringConstraints): Validator {
  return (value: unknown): ValidationError | null => {
    if (typeof value !== 'string') {
      return {
        field: '',
        code: 'TYPE_ERROR',
        message: 'expected string',
        value,
      };
    }

    const str = value;

    if (constraints.minLength !== undefined && str.length < constraints.minLength) {
      return {
        field: '',
        code: 'MIN_LENGTH',
        message: `minimum length is ${constraints.minLength}`,
        value: str,
      };
    }

    if (constraints.maxLength !== undefined && str.length > constraints.maxLength) {
      return {
        field: '',
        code: 'MAX_LENGTH',
        message: `maximum length is ${constraints.maxLength}`,
        value: str,
      };
    }

    if (constraints.pattern !== undefined && !constraints.pattern.test(str)) {
      return {
        field: '',
        code: 'PATTERN',
        message: 'does not match required pattern',
        value: str,
      };
    }

    if (constraints.format !== undefined) {
      const formatError = validateFormat(str, constraints.format);
      if (formatError) {
        return {
          field: '',
          code: 'FORMAT',
          message: formatError,
          value: str,
        };
      }
    }

    if (constraints.enum !== undefined && !constraints.enum.includes(str)) {
      return {
        field: '',
        code: 'ENUM',
        message: `must be one of: ${constraints.enum.join(', ')}`,
        value: str,
      };
    }

    return null;
  };
}

// validation/number-validator.ts

export function createNumberValidator(constraints: NumberConstraints): Validator {
  return (value: unknown): ValidationError | null => {
    if (typeof value !== 'number' || isNaN(value)) {
      return {
        field: '',
        code: 'TYPE_ERROR',
        message: 'expected number',
        value,
      };
    }

    const num = value;

    if (constraints.minimum !== undefined && num < constraints.minimum) {
      return {
        field: '',
        code: 'MINIMUM',
        message: `minimum value is ${constraints.minimum}`,
        value: num,
      };
    }

    if (constraints.maximum !== undefined && num > constraints.maximum) {
      return {
        field: '',
        code: 'MAXIMUM',
        message: `maximum value is ${constraints.maximum}`,
        value: num,
      };
    }

    if (constraints.exclusiveMinimum !== undefined && num <= constraints.exclusiveMinimum) {
      return {
        field: '',
        code: 'EXCLUSIVE_MINIMUM',
        message: `must be greater than ${constraints.exclusiveMinimum}`,
        value: num,
      };
    }

    if (constraints.exclusiveMaximum !== undefined && num >= constraints.exclusiveMaximum) {
      return {
        field: '',
        code: 'EXCLUSIVE_MAXIMUM',
        message: `must be less than ${constraints.exclusiveMaximum}`,
        value: num,
      };
    }

    if (constraints.multipleOf !== undefined && num % constraints.multipleOf !== 0) {
      return {
        field: '',
        code: 'MULTIPLE_OF',
        message: `must be multiple of ${constraints.multipleOf}`,
        value: num,
      };
    }

    return null;
  };
}

// validation/format-validator.ts

const EMAIL_REGEX = /^[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}$/;
const UUID_REGEX = /^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/i;
const HOSTNAME_REGEX = /^([a-zA-Z0-9]|[a-zA-Z0-9][a-zA-Z0-9\-]{0,61}[a-zA-Z0-9])(\.([a-zA-Z0-9]|[a-zA-Z0-9][a-zA-Z0-9\-]{0,61}[a-zA-Z0-9]))*$/;

function validateFormat(value: string, format: string): string | null {
  switch (format) {
    case 'email':
      return EMAIL_REGEX.test(value) ? null : 'invalid email format';
    
    case 'uuid':
      return UUID_REGEX.test(value) ? null : 'invalid UUID v4 format';
    
    case 'hostname':
      return (value.length <= 253 && HOSTNAME_REGEX.test(value)) 
        ? null 
        : 'invalid hostname format';
    
    case 'uri':
      return value.includes('://') ? null : 'invalid URI format';
    
    case 'datetime': {
      const date = new Date(value);
      return isNaN(date.getTime()) ? 'invalid datetime format, expected RFC 3339' : null;
    }
    
    case 'base64': {
      try {
        atob(value);
        return null;
      } catch {
        return 'invalid base64 encoding';
      }
    }
    
    default:
      return `unknown format: ${format}`;
  }
}

// validation/object-validator.ts

export interface ObjectSchema {
  required?: string[];
  properties?: Record<string, Validator>;
  additionalProperties?: boolean;
}

export function createObjectValidator(schema: ObjectSchema): Validator {
  return (value: unknown): ValidationError | null => {
    if (typeof value !== 'object' || value === null || Array.isArray(value)) {
      return {
        field: '',
        code: 'TYPE_ERROR',
        message: 'expected object',
        value,
      };
    }

    const obj = value as Record<string, unknown>;

    // Check required fields
    if (schema.required) {
      for (const field of schema.required) {
        if (!(field in obj) || obj[field] === undefined) {
          return {
            field,
            code: 'REQUIRED',
            message: 'field is required',
          };
        }
      }
    }

    // Validate properties
    if (schema.properties) {
      for (const [field, validator] of Object.entries(schema.properties)) {
        if (field in obj) {
          const error = validator(obj[field]);
          if (error) {
            error.field = error.field ? `${field}.${error.field}` : field;
            return error;
          }
        }
      }
    }

    // Check additional properties
    if (schema.additionalProperties === false && schema.properties) {
      const allowedKeys = new Set(Object.keys(schema.properties));
      for (const key of Object.keys(obj)) {
        if (!allowedKeys.has(key)) {
          return {
            field: key,
            code: 'ADDITIONAL_PROPERTY',
            message: 'additional property not allowed',
          };
        }
      }
    }

    return null;
  };
}
```

### Python Implementation

```python
# validation/types.py
from typing import Any, Callable, Dict, List, Optional, Pattern, TypedDict, Union
import re
from datetime import datetime
import base64


class ValidationError(TypedDict):
    field: str
    code: str
    message: str
    value: Optional[Any]


Validator = Callable[[Any], Optional[ValidationError]]


class StringConstraints(TypedDict, total=False):
    min_length: int
    max_length: int
    pattern: Pattern
    format: str
    enum: List[str]


class NumberConstraints(TypedDict, total=False):
    minimum: float
    maximum: float
    exclusive_minimum: float
    exclusive_maximum: float
    multiple_of: float


class ArrayConstraints(TypedDict, total=False):
    min_items: int
    max_items: int
    unique_items: bool
    item_validator: Validator


# validation/string_validator.py

EMAIL_PATTERN = re.compile(r'^[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}$')
UUID_PATTERN = re.compile(r'^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$', re.IGNORECASE)
HOSTNAME_PATTERN = re.compile(r'^([a-zA-Z0-9]|[a-zA-Z0-9][a-zA-Z0-9\-]{0,61}[a-zA-Z0-9])(\.([a-zA-Z0-9]|[a-zA-Z0-9][a-zA-Z0-9\-]{0,61}[a-zA-Z0-9]))*$')


def create_string_validator(constraints: StringConstraints) -> Validator:
    def validate(value: Any) -> Optional[ValidationError]:
        if not isinstance(value, str):
            return {
                'field': '',
                'code': 'TYPE_ERROR',
                'message': 'expected string',
                'value': value
            }
        
        if 'min_length' in constraints and len(value) < constraints['min_length']:
            return {
                'field': '',
                'code': 'MIN_LENGTH',
                'message': f"minimum length is {constraints['min_length']}",
                'value': value
            }
        
        if 'max_length' in constraints and len(value) > constraints['max_length']:
            return {
                'field': '',
                'code': 'MAX_LENGTH',
                'message': f"maximum length is {constraints['max_length']}",
                'value': value
            }
        
        if 'pattern' in constraints and not constraints['pattern'].match(value):
            return {
                'field': '',
                'code': 'PATTERN',
                'message': 'does not match required pattern',
                'value': value
            }
        
        if 'format' in constraints:
            format_error = validate_format(value, constraints['format'])
            if format_error:
                return {
                    'field': '',
                    'code': 'FORMAT',
                    'message': format_error,
                    'value': value
                }
        
        if 'enum' in constraints and value not in constraints['enum']:
            return {
                'field': '',
                'code': 'ENUM',
                'message': f"must be one of: {', '.join(constraints['enum'])}",
                'value': value
            }
        
        return None
    
    return validate


def validate_format(value: str, format_name: str) -> Optional[str]:
    if format_name == 'email':
        return None if EMAIL_PATTERN.match(value) else 'invalid email format'
    
    elif format_name == 'uuid':
        return None if UUID_PATTERN.match(value) else 'invalid UUID v4 format'
    
    elif format_name == 'hostname':
        if len(value) > 253:
            return 'invalid hostname format'
        return None if HOSTNAME_PATTERN.match(value) else 'invalid hostname format'
    
    elif format_name == 'uri':
        return None if '://' in value else 'invalid URI format'
    
    elif format_name == 'datetime':
        try:
            datetime.fromisoformat(value.replace('Z', '+00:00'))
            return None
        except ValueError:
            return 'invalid datetime format, expected RFC 3339'
    
    elif format_name == 'base64':
        try:
            base64.b64decode(value)
            return None
        except Exception:
            return 'invalid base64 encoding'
    
    else:
        return f"unknown format: {format_name}"


# validation/number_validator.py

def create_number_validator(constraints: NumberConstraints) -> Validator:
    def validate(value: Any) -> Optional[ValidationError]:
        if not isinstance(value, (int, float)) or isinstance(value, bool):
            return {
                'field': '',
                'code': 'TYPE_ERROR',
                'message': 'expected number',
                'value': value
            }
        
        num = float(value)
        
        if 'minimum' in constraints and num < constraints['minimum']:
            return {
                'field': '',
                'code': 'MINIMUM',
                'message': f"minimum value is {constraints['minimum']}",
                'value': num
            }
        
        if 'maximum' in constraints and num > constraints['maximum']:
            return {
                'field': '',
                'code': 'MAXIMUM',
                'message': f"maximum value is {constraints['maximum']}",
                'value': num
            }
        
        if 'exclusive_minimum' in constraints and num <= constraints['exclusive_minimum']:
            return {
                'field': '',
                'code': 'EXCLUSIVE_MINIMUM',
                'message': f"must be greater than {constraints['exclusive_minimum']}",
                'value': num
            }
        
        if 'exclusive_maximum' in constraints and num >= constraints['exclusive_maximum']:
            return {
                'field': '',
                'code': 'EXCLUSIVE_MAXIMUM',
                'message': f"must be less than {constraints['exclusive_maximum']}",
                'value': num
            }
        
        if 'multiple_of' in constraints and num % constraints['multiple_of'] != 0:
            return {
                'field': '',
                'code': 'MULTIPLE_OF',
                'message': f"must be multiple of {constraints['multiple_of']}",
                'value': num
            }
        
        return None
    
    return validate


# validation/object_validator.py

class ObjectSchema(TypedDict, total=False):
    required: List[str]
    properties: Dict[str, Validator]
    additional_properties: bool


def create_object_validator(schema: ObjectSchema) -> Validator:
    def validate(value: Any) -> Optional[ValidationError]:
        if not isinstance(value, dict):
            return {
                'field': '',
                'code': 'TYPE_ERROR',
                'message': 'expected object',
                'value': value
            }
        
        # Check required fields
        if 'required' in schema:
            for field in schema['required']:
                if field not in value or value[field] is None:
                    return {
                        'field': field,
                        'code': 'REQUIRED',
                        'message': 'field is required',
                        'value': None
                    }
        
        # Validate properties
        if 'properties' in schema:
            for field, validator in schema['properties'].items():
                if field in value:
                    error = validator(value[field])
                    if error:
                        error['field'] = f"{field}.{error['field']}" if error['field'] else field
                        return error
        
        # Check additional properties
        if schema.get('additional_properties') is False and 'properties' in schema:
            allowed_keys = set(schema['properties'].keys())
            for key in value.keys():
                if key not in allowed_keys:
                    return {
                        'field': key,
                        'code': 'ADDITIONAL_PROPERTY',
                        'message': 'additional property not allowed',
                        'value': None
                    }
        
        return None
    
    return validate
```

## Cross-Language Test Suite

### Test Vectors

All implementations must pass the same test vectors:

```yaml
# test_vectors.yaml
string_tests:
  - description: "Valid email"
    value: "test@example.com"
    format: "email"
    should_pass: true
  
  - description: "Invalid email - missing @"
    value: "testexample.com"
    format: "email"
    should_pass: false
    expected_error: "FORMAT"
  
  - description: "Valid UUID"
    value: "550e8400-e29b-41d4-a716-446655440000"
    format: "uuid"
    should_pass: true
  
  - description: "Invalid UUID - wrong version"
    value: "550e8400-e29b-11d4-a716-446655440000"
    format: "uuid"
    should_pass: false
    expected_error: "FORMAT"

number_tests:
  - description: "Valid number in range"
    value: 50
    constraints:
      minimum: 0
      maximum: 100
    should_pass: true
  
  - description: "Number below minimum"
    value: -1
    constraints:
      minimum: 0
    should_pass: false
    expected_error: "MINIMUM"

array_tests:
  - description: "Valid array with unique items"
    value: ["a", "b", "c"]
    constraints:
      unique_items: true
      min_items: 1
    should_pass: true
  
  - description: "Array with duplicate items"
    value: ["a", "a", "b"]
    constraints:
      unique_items: true
    should_pass: false
    expected_error: "UNIQUE_ITEMS"
```

### Compliance Test Runner

```go
// tests/cross_language_test.go

package validation

import (
    "testing"
    "gopkg.in/yaml.v3"
    "os"
)

type TestVector struct {
    Description   string                 `yaml:"description"`
    Value         interface{}            `yaml:"value"`
    Format        string                 `yaml:"format,omitempty"`
    Constraints   map[string]interface{} `yaml:"constraints,omitempty"`
    ShouldPass    bool                   `yaml:"should_pass"`
    ExpectedError string                 `yaml:"expected_error,omitempty"`
}

func TestCrossLanguageValidation(t *testing.T) {
    data, err := os.ReadFile("test_vectors.yaml")
    if err != nil {
        t.Fatalf("Failed to read test vectors: %v", err)
    }
    
    var testCases struct {
        StringTests []TestVector `yaml:"string_tests"`
        NumberTests []TestVector `yaml:"number_tests"`
        ArrayTests  []TestVector `yaml:"array_tests"`
    }
    
    if err := yaml.Unmarshal(data, &testCases); err != nil {
        t.Fatalf("Failed to parse test vectors: %v", err)
    }
    
    // Run string tests
    for _, tc := range testCases.StringTests {
        t.Run(tc.Description, func(t *testing.T) {
            // Implementation-specific test
            validateTestCase(t, tc)
        })
    }
}
```

## Performance Benchmarks

All validation implementations must meet minimum performance standards:

| Operation | Max Latency (p99) | Throughput |
|-----------|-------------------|------------|
| String validation | 1ms | 100K ops/sec |
| Number validation | 0.5ms | 200K ops/sec |
| Object validation | 5ms | 20K ops/sec |
| Array validation (100 items) | 10ms | 10K ops/sec |

---

**Bible Standard Compliance**: This specification adheres to the Muster Bible Standard for cross-language compatibility, validation consistency, and data integrity. All implementations must pass the shared test vectors and meet performance benchmarks.
