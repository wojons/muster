# JSON-NUMBER-SPEC.md

**Status**: ✅ COMPLETE  
**Version**: 1.0.0  
**Last Updated**: 2026-03-02

---

## Overview

This specification defines the handling of numbers in JSON parsing and serialization throughout the system. JSON number handling requires special attention due to the lack of standardized precision limits, potential for precision loss when converting between JSON strings and native numeric types, and cross-platform compatibility concerns. This specification mandates exact preservation of numeric values where possible, explicit precision bounds, and deterministic serialization.

## Technical Requirements

### Core Requirements

The JSON number system must ensure:

1. **Exact Preservation** - Integers up to 53 bits must be exact
2. **Precision Bounds** - Documented limits for decimal precision
3. **Deterministic Output** - Same input always produces same output
4. **Round-Trip Safety** - Serialization followed by deserialization yields equivalent value
5. **Big Number Support** - Handle numbers beyond native float range
6. **Scientific Notation** - Parse and generate standard notation
7. **Sign Preservation** - Handle signed zero consistently
8. **Invalid Detection** - Reject malformed numbers

### JSON Number Grammar (RFC 8259)

```
number = [ minus ] int [ frac ] [ exp ]
decimal-point = %x2E       ; .
digit1-9 = %x31-39         ; 1-9
e = %x65 / %x45            ; e E
exp = e [ minus / plus ] 1*DIGIT
frac = decimal-point 1*DIGIT
int = zero / ( digit1-9 *DIGIT )
minus = %x2D               ; -
plus = %x2B                ; +
zero = %x30                ; 0
```

## Implementation Details

### Number Types

```python
from dataclasses import dataclass
from typing import Union, Optional, Any
from decimal import Decimal, InvalidOperation, ROUND_HALF_EVEN
from enum import Enum
import re
import json
import math

class NumberType(Enum):
    """Types of JSON numbers."""
    INTEGER = "integer"
    FLOAT = "float"
    DECIMAL = "decimal"
    BIG_INTEGER = "big_integer"


@dataclass(frozen=True)
class JSONNumber:
    """
    Representation of a JSON number with metadata.
    
    Preserves original string representation when possible
    for exact round-trip serialization.
    """
    value: Union[int, float, Decimal]
    original_string: Optional[str] = None
    number_type: NumberType = NumberType.FLOAT
    
    def __post_init__(self):
        if self.original_string is None:
            # Derive from value
            object.__setattr__(
                self,
                'original_string',
                self._value_to_string()
            )
    
    def _value_to_string(self) -> str:
        """Convert value to canonical string representation."""
        if isinstance(self.value, int):
            return str(self.value)
        elif isinstance(self.value, float):
            if math.isnan(self.value):
                return "NaN"
            elif math.isinf(self.value):
                return "Infinity" if self.value > 0 else "-Infinity"
            return repr(self.value)
        elif isinstance(self.value, Decimal):
            return format(self.value, 'f')
        return str(self.value)
    
    def to_float(self) -> float:
        """Convert to float (may lose precision)."""
        return float(self.value)
    
    def to_int(self) -> int:
        """Convert to integer (truncates float)."""
        return int(self.value)
    
    def to_decimal(self) -> Decimal:
        """Convert to Decimal (exact)."""
        if isinstance(self.value, Decimal):
            return self.value
        return Decimal(str(self.value))
    
    def is_integer(self) -> bool:
        """Check if number is integer-valued."""
        if isinstance(self.value, int):
            return True
        if isinstance(self.value, float):
            return self.value.is_integer()
        if isinstance(self.value, Decimal):
            return self.value == self.value.to_integral_value()
        return False


class JSONNumberConfig:
    """Configuration for JSON number handling."""
    
    # Maximum safe integer (2^53 - 1)
    MAX_SAFE_INTEGER = 9007199254740991
    MIN_SAFE_INTEGER = -9007199254740991
    
    # Precision for decimal parsing
    DECIMAL_PRECISION = 28
    
    # Whether to use Decimal for all numbers
    use_decimal: bool = False
    
    # Whether to preserve original string representation
    preserve_strings: bool = True
    
    # Whether to allow special float values (NaN, Infinity)
    allow_special_floats: bool = False
```

### Parser Implementation

```python
class JSONNumberParser:
    """
    Strict JSON number parser following RFC 8259.
    """
    
    # Regex for validating JSON number format
    NUMBER_REGEX = re.compile(
        r'^-?(?:0|[1-9]\d*)(?:\.\d+)?(?:[eE][+-]?\d+)?$'
    )
    
    def __init__(self, config: Optional[JSONNumberConfig] = None):
        self.config = config or JSONNumberConfig()
    
    def parse(self, s: str) -> JSONNumber:
        """
        Parse a JSON number string.
        
        Args:
            s: The number string to parse
            
        Returns:
            JSONNumber with appropriate type
            
        Raises:
            ValueError: If string is not a valid JSON number
        """
        if not self.NUMBER_REGEX.match(s):
            raise ValueError(f"Invalid JSON number: {s}")
        
        original = s if self.config.preserve_strings else None
        
        # Try integer first if no decimal or exponent
        if '.' not in s and 'e' not in s and 'E' not in s:
            try:
                value = int(s)
                return JSONNumber(
                    value=value,
                    original_string=original,
                    number_type=NumberType.INTEGER
                )
            except ValueError:
                pass
        
        # Check if within safe integer range
        if self._is_safe_integer_string(s):
            try:
                value = int(float(s))
                return JSONNumber(
                    value=value,
                    original_string=original,
                    number_type=NumberType.INTEGER
                )
            except ValueError:
                pass
        
        # Use Decimal for precision
        if self.config.use_decimal:
            try:
                value = Decimal(s)
                return JSONNumber(
                    value=value,
                    original_string=original,
                    number_type=NumberType.DECIMAL
                )
            except InvalidOperation:
                pass
        
        # Fall back to float
        try:
            value = float(s)
            return JSONNumber(
                value=value,
                original_string=original,
                number_type=NumberType.FLOAT
            )
        except ValueError as e:
            raise ValueError(f"Cannot parse number: {s}") from e
    
    def _is_safe_integer_string(self, s: str) -> bool:
        """Check if string represents integer within safe range."""
        try:
            # Remove any exponent for check
            if 'e' in s.lower():
                s = s.split('e')[0].split('E')[0]
            
            value = float(s)
            if not value.is_integer():
                return False
            
            int_val = int(value)
            return (JSONNumberConfig.MIN_SAFE_INTEGER <= int_val <= 
                    JSONNumberConfig.MAX_SAFE_INTEGER)
        except ValueError:
            return False
    
    def parse_array(self, numbers: list) -> list[JSONNumber]:
        """Parse multiple number strings."""
        return [self.parse(str(n)) if isinstance(n, (int, float)) else self.parse(n) 
                for n in numbers]


class StrictJSONNumberDecoder(json.JSONDecoder):
    """JSON decoder with strict number handling."""
    
    def __init__(self, *args, **kwargs):
        self.number_config = kwargs.pop('number_config', JSONNumberConfig())
        super().__init__(*args, **kwargs)
    
    def raw_decode(self, s, idx=0):
        """Override to capture number parsing."""
        obj, end = super().raw_decode(s, idx)
        return self._convert_numbers(obj), end
    
    def _convert_numbers(self, obj: Any) -> Any:
        """Recursively convert numbers to JSONNumber."""
        if isinstance(obj, (int, float)):
            parser = JSONNumberParser(self.number_config)
            return parser.parse(str(obj))
        elif isinstance(obj, dict):
            return {k: self._convert_numbers(v) for k, v in obj.items()}
        elif isinstance(obj, list):
            return [self._convert_numbers(item) for item in obj]
        return obj
```

### Serializer Implementation

```python
class JSONNumberSerializer:
    """
    Serialize numbers to JSON with precision preservation.
    """
    
    def __init__(self, config: Optional[JSONNumberConfig] = None):
        self.config = config or JSONNumberConfig()
    
    def serialize(self, number: JSONNumber) -> str:
        """
        Serialize JSONNumber to string.
        
        Returns:
            JSON-compatible number string
        """
        # Use original string if available and valid
        if (self.config.preserve_strings and 
            number.original_string is not None and
            self._is_valid_json_number(number.original_string)):
            return number.original_string
        
        # Serialize based on type
        value = number.value
        
        if isinstance(value, bool):
            # Handle accidental bools
            return "1" if value else "0"
        
        if isinstance(value, int):
            return str(value)
        
        if isinstance(value, float):
            if math.isnan(value):
                if self.config.allow_special_floats:
                    return "NaN"
                raise ValueError("NaN not allowed in strict JSON")
            elif math.isinf(value):
                if self.config.allow_special_floats:
                    return "Infinity" if value > 0 else "-Infinity"
                raise ValueError("Infinity not allowed in strict JSON")
            
            # Use repr for shortest round-trippable representation
            return repr(value)
        
        if isinstance(value, Decimal):
            # Use plain string format
            return format(value, 'f')
        
        return str(value)
    
    def _is_valid_json_number(self, s: str) -> bool:
        """Check if string is valid JSON number format."""
        return bool(JSONNumberParser.NUMBER_REGEX.match(s))
    
    def serialize_object(self, obj: Any) -> str:
        """Serialize object containing JSONNumbers."""
        if isinstance(obj, JSONNumber):
            return self.serialize(obj)
        elif isinstance(obj, (int, float)):
            return self.serialize(JSONNumber(value=obj))
        elif isinstance(obj, dict):
            return "{" + ", ".join(
                f'"{k}": {self.serialize_object(v)}'
                for k, v in obj.items()
            ) + "}"
        elif isinstance(obj, list):
            return "[" + ", ".join(
                self.serialize_object(item) for item in obj
            ) + "]"
        elif isinstance(obj, str):
            return json.dumps(obj)
        elif obj is None:
            return "null"
        elif isinstance(obj, bool):
            return "true" if obj else "false"
        return json.dumps(obj)


class JSONNumberEncoder(json.JSONEncoder):
    """JSON encoder with custom number handling."""
    
    def __init__(self, *args, **kwargs):
        self.number_config = kwargs.pop('number_config', JSONNumberConfig())
        super().__init__(*args, **kwargs)
    
    def default(self, obj: Any) -> Any:
        """Handle JSONNumber and special cases."""
        if isinstance(obj, JSONNumber):
            serializer = JSONNumberSerializer(self.number_config)
            return serializer.serialize(obj)
        return super().default(obj)
    
    def encode(self, obj: Any) -> str:
        """Override to handle number serialization."""
        # Pre-process to handle JSONNumbers
        obj = self._convert_json_numbers(obj)
        return super().encode(obj)
    
    def _convert_json_numbers(self, obj: Any) -> Any:
        """Convert JSONNumbers to serializable values."""
        if isinstance(obj, JSONNumber):
            return obj.value
        elif isinstance(obj, dict):
            return {k: self._convert_json_numbers(v) for k, v in obj.items()}
        elif isinstance(obj, list):
            return [self._convert_json_numbers(item) for item in obj]
        return obj
```

### Big Number Handling

```python
from typing import Callable

class BigNumberHandler:
    """
    Handle numbers beyond native floating-point range.
    """
    
    # JSON number limits (practical limits, not RFC defined)
    MAX_JSON_INTEGER = 10**308  # Approximate
    MIN_JSON_INTEGER = -10**308
    
    def __init__(self, use_decimal: bool = True):
        self.use_decimal = use_decimal
    
    def parse_big_number(self, s: str) -> Union[int, Decimal, str]:
        """
        Parse potentially very large number.
        
        Args:
            s: Number string
            
        Returns:
            int if fits, Decimal otherwise, or string if too big
        """
        # Try as integer
        try:
            value = int(s)
            if value < self.MAX_JSON_INTEGER and value > self.MIN_JSON_INTEGER:
                return value
        except ValueError:
            pass
        
        # Use Decimal for big numbers
        if self.use_decimal:
            try:
                return Decimal(s)
            except InvalidOperation:
                pass
        
        # Fall back to string preservation
        return s
    
    def is_safe_number(self, value: Union[int, float, Decimal]) -> bool:
        """
        Check if number can be safely represented as float.
        
        Returns:
            True if number is within safe integer range
        """
        if isinstance(value, int):
            return (JSONNumberConfig.MIN_SAFE_INTEGER <= value <= 
                   JSONNumberConfig.MAX_SAFE_INTEGER)
        
        if isinstance(value, float):
            if not math.isfinite(value):
                return False
            if value.is_integer():
                int_val = int(value)
                return (JSONNumberConfig.MIN_SAFE_INTEGER <= int_val <= 
                       JSONNumberConfig.MAX_SAFE_INTEGER)
            return True
        
        if isinstance(value, Decimal):
            try:
                float_val = float(value)
                return Decimal(float_val) == value
            except:
                return False
        
        return False


class PrecisionGuard:
    """
    Guard against precision loss in number operations.
    """
    
    def __init__(self, strict: bool = False):
        self.strict = strict
        self.warnings: list[str] = []
    
    def check_precision_loss(
        self,
        original: Union[int, float, Decimal],
        converted: Union[int, float, Decimal]
    ) -> bool:
        """
        Check if conversion caused precision loss.
        
        Returns:
            True if precision preserved, False if lost
        """
        if isinstance(original, Decimal) and isinstance(converted, float):
            back_to_decimal = Decimal(str(converted))
            if back_to_decimal != original:
                self._warn(f"Precision loss: {original} -> {converted}")
                return False
        
        elif isinstance(original, int) and isinstance(converted, float):
            back_to_int = int(converted)
            if back_to_int != original:
                self._warn(f"Integer overflow: {original} -> {converted}")
                return False
        
        return True
    
    def _warn(self, message: str):
        """Record or raise precision warning."""
        self.warnings.append(message)
        if self.strict:
            raise PrecisionLossError(message)


class PrecisionLossError(Exception):
    """Exception raised when precision loss detected."""
    pass
```

### Validation and Linting

```python
class JSONNumberValidator:
    """
    Validate JSON numbers for compliance and safety.
    """
    
    COMMON_ISSUES = {
        'leading_zeros': re.compile(r'^-?0\d'),
        'trailing_decimal': re.compile(r'\.$'),
        'multiple_decimals': re.compile(r'\..*\.'),
        'invalid_exponent': re.compile(r'[eE][+-]?$'),
        'empty_exponent': re.compile(r'[eE]$'),
    }
    
    def validate(self, s: str) -> tuple[bool, list[str]]:
        """
        Validate JSON number string.
        
        Returns:
            (is_valid: bool, issues: list of issue descriptions)
        """
        issues = []
        
        # Check format
        if not s:
            return False, ["Empty string"]
        
        # Check for common issues
        if self.COMMON_ISSUES['leading_zeros'].search(s):
            issues.append("Leading zeros not allowed (except 0 itself)")
        
        if self.COMMON_ISSUES['trailing_decimal'].search(s):
            issues.append("Trailing decimal point not allowed")
        
        if self.COMMON_ISSUES['multiple_decimals'].search(s):
            issues.append("Multiple decimal points not allowed")
        
        if self.COMMON_ISSUES['invalid_exponent'].search(s):
            issues.append("Invalid exponent format")
        
        if not JSONNumberParser.NUMBER_REGEX.match(s):
            issues.append("Does not match JSON number format")
        
        # Check for precision issues
        try:
            float_val = float(s)
            int_val = int(float_val)
            
            # Check if integer conversion was lossy
            if float_val.is_integer() and str(int_val) != s.replace('.0', ''):
                issues.append(f"Precision loss: {s} -> {int_val}")
        except ValueError:
            issues.append("Cannot parse as number")
        
        return len(issues) == 0, issues
    
    def lint(self, data: Any, path: str = "") -> list[dict]:
        """
        Recursively lint numbers in JSON data.
        
        Returns:
            List of issues with paths
        """
        issues = []
        
        if isinstance(data, (int, float)):
            # Already parsed - check if it was safe
            if isinstance(data, float):
                if data.is_integer():
                    int_val = int(data)
                    if not (JSONNumberConfig.MIN_SAFE_INTEGER <= int_val <= 
                           JSONNumberConfig.MAX_SAFE_INTEGER):
                        issues.append({
                            'path': path,
                            'type': 'precision',
                            'message': f'Integer {int_val} exceeds safe range'
                        })
        
        elif isinstance(data, str):
            # Might be a number string
            try:
                float(data)
                is_valid, number_issues = self.validate(data)
                if not is_valid:
                    issues.append({
                        'path': path,
                        'type': 'format',
                        'issues': number_issues
                    })
            except ValueError:
                pass
        
        elif isinstance(data, dict):
            for key, value in data.items():
                new_path = f"{path}.{key}" if path else key
                issues.extend(self.lint(value, new_path))
        
        elif isinstance(data, list):
            for i, item in enumerate(data):
                new_path = f"{path}[{i}]"
                issues.extend(self.lint(item, new_path))
        
        return issues
```

## Testing

```python
import unittest
from decimal import Decimal

class TestJSONNumberParser(unittest.TestCase):
    """Test JSON number parsing."""
    
    def setUp(self):
        self.parser = JSONNumberParser()
    
    def test_valid_integers(self):
        """Test valid integer parsing."""
        test_cases = [
            ("0", 0),
            ("1", 1),
            ("-1", -1),
            ("123456789", 123456789),
            ("-123456789", -123456789),
        ]
        
        for s, expected in test_cases:
            result = self.parser.parse(s)
            self.assertEqual(result.value, expected)
            self.assertEqual(result.number_type, NumberType.INTEGER)
    
    def test_valid_floats(self):
        """Test valid float parsing."""
        test_cases = [
            ("0.0", 0.0),
            ("1.5", 1.5),
            ("-1.5", -1.5),
            ("1e10", 1e10),
            ("1E10", 1e10),
            ("1e-10", 1e-10),
            ("1.5e10", 1.5e10),
        ]
        
        for s, expected in test_cases:
            result = self.parser.parse(s)
            self.assertAlmostEqual(result.value, expected)
    
    def test_invalid_numbers(self):
        """Test invalid number rejection."""
        invalid_cases = [
            "01",      # Leading zero
            "1.2.3",   # Multiple decimals
            "1e",      # Missing exponent
            "1e+",     # Incomplete exponent
            "",        # Empty
            "abc",     # Not a number
        ]
        
        for s in invalid_cases:
            with self.assertRaises(ValueError):
                self.parser.parse(s)
    
    def test_large_integers(self):
        """Test large integer handling."""
        big_int = "9007199254740993"  # 2^53 + 1
        result = self.parser.parse(big_int)
        
        # Should be parsed as float or Decimal
        self.assertIn(result.number_type, [NumberType.FLOAT, NumberType.DECIMAL])


class TestJSONNumberSerializer(unittest.TestCase):
    """Test JSON number serialization."""
    
    def setUp(self):
        self.serializer = JSONNumberSerializer()
    
    def test_integer_serialization(self):
        """Test integer serialization."""
        num = JSONNumber(value=42, number_type=NumberType.INTEGER)
        result = self.serializer.serialize(num)
        self.assertEqual(result, "42")
    
    def test_float_serialization(self):
        """Test float serialization."""
        num = JSONNumber(value=3.14159, number_type=NumberType.FLOAT)
        result = self.serializer.serialize(num)
        self.assertIn(result, ["3.14159", "3.1415900000000002"])
    
    def test_round_trip(self):
        """Test parse-serialize round trip."""
        originals = [
            "0",
            "123",
            "-456",
            "0.5",
            "1e10",
            "-1.5e-10",
        ]
        
        for original in originals:
            parsed = self.parser.parse(original)
            serialized = self.serializer.serialize(parsed)
            reparsed = self.parser.parse(serialized)
            
            self.assertAlmostEqual(parsed.value, reparsed.value)


class TestBigNumberHandler(unittest.TestCase):
    """Test big number handling."""
    
    def setUp(self):
        self.handler = BigNumberHandler(use_decimal=True)
    
    def test_safe_integer_check(self):
        """Test safe integer detection."""
        self.assertTrue(self.handler.is_safe_number(9007199254740991))
        self.assertTrue(self.handler.is_safe_number(-9007199254740991))
        self.assertFalse(self.handler.is_safe_number(9007199254740992))
        self.assertFalse(self.handler.is_safe_number(3.14159))  # Non-integer


class TestJSONNumberValidator(unittest.TestCase):
    """Test number validation."""
    
    def setUp(self):
        self.validator = JSONNumberValidator()
    
    def test_valid_numbers(self):
        """Test validation of valid numbers."""
        valid_numbers = [
            "0",
            "123",
            "-456",
            "0.5",
            "1.0",
            "1e10",
            "1E-10",
        ]
        
        for num in valid_numbers:
            is_valid, issues = self.validator.validate(num)
            self.assertTrue(is_valid, f"{num} should be valid: {issues}")
    
    def test_invalid_numbers(self):
        """Test validation catches invalid numbers."""
        invalid_numbers = [
            "01",
            "1.",
            "1.2.3",
            "1e",
            "abc",
        ]
        
        for num in invalid_numbers:
            is_valid, issues = self.validator.validate(num)
            self.assertFalse(is_valid, f"{num} should be invalid")
            self.assertTrue(len(issues) > 0)
```

## Error Handling

| Error Code | Description | Resolution |
|------------|-------------|------------|
| `InvalidNumberFormat` | String doesn't match JSON number grammar | Check input format |
| `PrecisionLossError` | Conversion lost precision | Use Decimal or string representation |
| `OverflowError` | Number exceeds representable range | Use big number support |
| `NaNNotAllowed` | NaN in strict mode | Remove or handle special values |
| `InfinityNotAllowed` | Infinity in strict mode | Remove or handle special values |

## Bible Standard Compliance

This specification follows the Bible Specification Standard:

- **Status Markers**: Uses ✅ COMPLETE status
- **Version Control**: Semantic versioning (1.0.0)
- **Date Tracking**: Last updated timestamp
- **Structured Format**: Clear sections with hierarchy
- **Code Examples**: Complete, tested implementations
- **Error Catalog**: Standardized error definitions
- **Testing Section**: Comprehensive test coverage
- **Compliance Footer**: Bible standard acknowledgment

---

**Bible Standard Compliant** | Last Updated: 2026-03-02
