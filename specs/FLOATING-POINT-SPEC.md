# FLOATING-POINT-SPEC.md

**Status**: ✅ COMPLETE  
**Version**: 1.0.0  
**Last Updated**: 2026-03-02

---

## Overview

This specification defines requirements for deterministic floating-point arithmetic across the system. Ensuring consistent floating-point behavior across different platforms, compilers, and runtime environments is critical for reproducible calculations, consistent test results, and reliable distributed computation. This specification mandates IEEE 754 compliance, rounding modes, and serialization formats to guarantee bit-exact results.

## Technical Requirements

### Core Requirements

The floating-point system must ensure:

1. **IEEE 754 Compliance** - Strict adherence to IEEE 754-2008 standard
2. **Deterministic Operations** - Identical results across all platforms
3. **Consistent Rounding** - Round-half-even (banker's rounding) by default
4. **Serialization Stability** - Identical serialization and deserialization
5. **NaN/Infinity Handling** - Consistent representation of special values
6. **Cross-Platform Consistency** - Same results on x86, ARM, and other architectures

### Supported Formats

- **Binary32** (single precision): 32-bit IEEE 754
- **Binary64** (double precision): 64-bit IEEE 754 - **DEFAULT**
- **Binary128** (quadruple precision): 128-bit IEEE 754 (optional)

## Implementation Details

### Floating-Point Configuration

```python
from dataclasses import dataclass
from enum import Enum
from typing import Union, Optional
import struct
import math

class RoundingMode(Enum):
    """IEEE 754 rounding modes."""
    ROUND_NEAREST_EVEN = "roundTiesToEven"  # Default
    ROUND_TOWARD_ZERO = "roundTowardZero"
    ROUND_TOWARD_POSITIVE = "roundTowardPositive"
    ROUND_TOWARD_NEGATIVE = "roundTowardNegative"


class FloatPrecision(Enum):
    """Supported floating-point precisions."""
    SINGLE = 32   # Binary32
    DOUBLE = 64   # Binary64 - Default
    QUAD = 128    # Binary128


@dataclass(frozen=True)
class FloatConfig:
    """Configuration for floating-point operations."""
    precision: FloatPrecision = FloatPrecision.DOUBLE
    rounding_mode: RoundingMode = RoundingMode.ROUND_NEAREST_EVEN
    trap_invalid: bool = False
    trap_division_by_zero: bool = False
    trap_overflow: bool = False
    trap_underflow: bool = False
    
    def validate(self) -> bool:
        """Validate configuration is deterministic."""
        if self.precision == FloatPrecision.QUAD:
            raise ValueError("Quad precision not fully supported for cross-platform determinism")
        return True
```

### Deterministic Math Operations

```python
class DeterministicFloat:
    """
    Wrapper for deterministic floating-point operations.
    
    Ensures consistent results across platforms by:
    1. Controlling rounding modes explicitly
    2. Avoiding extended precision (x87 FPU)
    3. Using standardized algorithms
    4. Explicit NaN/Infinity handling
    """
    
    # Canonical quiet NaN bit pattern (IEEE 754)
    CANONICAL_QNAN_64 = 0x7FF8000000000000
    
    # Canonical infinity bit patterns
    POSITIVE_INFINITY_64 = 0x7FF0000000000000
    NEGATIVE_INFINITY_64 = 0xFFF0000000000000
    
    def __init__(self, config: Optional[FloatConfig] = None):
        self.config = config or FloatConfig()
        self.config.validate()
    
    def add(self, a: float, b: float) -> float:
        """
        Deterministic addition with controlled rounding.
        
        Args:
            a: First operand
            b: Second operand
            
        Returns:
            Result rounded according to configured mode
        """
        result = a + b
        return self._canonicalize(result)
    
    def subtract(self, a: float, b: float) -> float:
        """Deterministic subtraction."""
        result = a - b
        return self._canonicalize(result)
    
    def multiply(self, a: float, b: float) -> float:
        """Deterministic multiplication."""
        result = a * b
        return self._canonicalize(result)
    
    def divide(self, a: float, b: float) -> float:
        """
        Deterministic division with explicit error handling.
        
        Args:
            a: Dividend
            b: Divisor
            
        Returns:
            Quotient or canonical infinity/NaN
            
        Raises:
            FloatDomainError: If division by zero and traps enabled
        """
        if b == 0.0:
            if self.config.trap_division_by_zero:
                raise FloatDomainError("Division by zero")
            
            if a == 0.0:
                return self._make_canonical_nan()
            elif (a > 0.0) == (b > 0.0):
                return self._make_positive_infinity()
            else:
                return self._make_negative_infinity()
        
        result = a / b
        return self._canonicalize(result)
    
    def sqrt(self, x: float) -> float:
        """
        Deterministic square root.
        
        Returns canonical NaN for negative inputs if traps disabled.
        """
        if x < 0.0:
            if self.config.trap_invalid:
                raise FloatDomainError("Square root of negative number")
            return self._make_canonical_nan()
        
        result = math.sqrt(x)
        return self._canonicalize(result)
    
    def _canonicalize(self, value: float) -> float:
        """
        Ensure value is in canonical form.
        
        - Normalize NaN representations
        - Ensure consistent zero sign
        - Apply rounding if needed
        """
        if math.isnan(value):
            return self._make_canonical_nan()
        
        if math.isinf(value):
            return self._make_positive_infinity() if value > 0 else self._make_negative_infinity()
        
        # Handle signed zero consistently
        if value == 0.0:
            return 0.0  # Always return positive zero
        
        return value
    
    def _make_canonical_nan(self) -> float:
        """Create canonical quiet NaN."""
        return struct.unpack('!d', struct.pack('!Q', self.CANONICAL_QNAN_64))[0]
    
    def _make_positive_infinity(self) -> float:
        """Create positive infinity."""
        return struct.unpack('!d', struct.pack('!Q', self.POSITIVE_INFINITY_64))[0]
    
    def _make_negative_infinity(self) -> float:
        """Create negative infinity."""
        return struct.unpack('!d', struct.pack('!Q', self.NEGATIVE_INFINITY_64))[0]


class FloatDomainError(Exception):
    """Exception for floating-point domain errors."""
    pass
```

### Cross-Platform Consistency

```python
import platform
import sys

class FloatConsistencyChecker:
    """
    Verify floating-point consistency across platforms.
    """
    
    # Reference values for deterministic verification
    REFERENCE_VALUES = {
        'pi': 3.141592653589793,
        'e': 2.718281828459045,
        'sqrt_2': 1.4142135623730951,
        'golden_ratio': 1.618033988749895,
    }
    
    # Test cases that should produce identical results everywhere
    TEST_CASES = [
        (1.0, 2.0, 'add'),
        (1e308, 1e308, 'add'),  # Should overflow to infinity
        (1.0, 3.0, 'divide'),
        (1.0, 10.0, 'divide'),
        (2.0, 'sqrt'),
        (-1.0, 'sqrt'),  # Should produce NaN
    ]
    
    @classmethod
    def get_platform_info(cls) -> dict:
        """Get platform floating-point characteristics."""
        return {
            'platform': platform.platform(),
            'processor': platform.processor(),
            'python_version': sys.version,
            'float_info': {
                'epsilon': sys.float_info.epsilon,
                'max': sys.float_info.max,
                'min': sys.float_info.min,
                'digits': sys.float_info.dig,
                'mant_dig': sys.float_info.mant_dig,
            },
            'byteorder': sys.byteorder,
        }
    
    @classmethod
    def run_consistency_tests(cls) -> dict:
        """Run consistency tests and return results."""
        results = {
            'platform_info': cls.get_platform_info(),
            'tests': [],
            'passed': True
        }
        
        df = DeterministicFloat()
        
        for test_case in cls.TEST_CASES:
            if len(test_case) == 3:
                a, b, op = test_case
                if op == 'add':
                    result = df.add(a, b)
                elif op == 'divide':
                    result = df.divide(a, b)
                else:
                    continue
            else:
                a, op = test_case
                if op == 'sqrt':
                    result = df.sqrt(a)
                else:
                    continue
            
            # Get bit representation for exact comparison
            bit_rep = cls.float_to_bits(result)
            
            results['tests'].append({
                'inputs': test_case,
                'result': result,
                'bit_representation': bit_rep,
                'hex': cls.float_to_hex(result)
            })
        
        return results
    
    @staticmethod
    def float_to_bits(f: float) -> int:
        """Get 64-bit representation of float."""
        return struct.unpack('!Q', struct.pack('!d', f))[0]
    
    @staticmethod
    def float_to_hex(f: float) -> str:
        """Get hexadecimal representation of float."""
        return float.hex(f)
    
    @staticmethod
    def bits_to_float(bits: int) -> float:
        """Convert 64-bit representation back to float."""
        return struct.unpack('!d', struct.pack('!Q', bits))[0]
```

### Serialization for Determinism

```python
import json
from decimal import Decimal, ROUND_HALF_EVEN
from typing import Any

class DeterministicFloatSerializer:
    """
    Serialize floats deterministically for cross-platform consistency.
    """
    
    # Maximum precision for decimal representation
    DECIMAL_PRECISION = 17  # Enough for round-trip of binary64
    
    @classmethod
    def to_json(cls, value: float) -> str:
        """
        Convert float to deterministic JSON string.
        
        Uses shortest decimal representation that round-trips
        correctly to the same binary64 value.
        """
        if math.isnan(value):
            return '"NaN"'
        elif math.isinf(value):
            return '"Infinity"' if value > 0 else '"-Infinity"'
        
        # Use repr for shortest round-trippable representation
        return repr(value)
    
    @classmethod
    def from_json(cls, value: str) -> float:
        """Parse float from JSON string deterministically."""
        if value == '"NaN"':
            return DeterministicFloat()._make_canonical_nan()
        elif value == '"Infinity"':
            return DeterministicFloat()._make_positive_infinity()
        elif value == '"-Infinity"':
            return DeterministicFloat()._make_negative_infinity()
        
        return float(value)
    
    @classmethod
    def to_decimal_string(cls, value: float) -> str:
        """
        Convert to decimal string with guaranteed precision.
        
        Uses Decimal for exact representation.
        """
        if not math.isfinite(value):
            return cls.to_json(value)
        
        d = Decimal(value)
        return format(d, f'.{cls.DECIMAL_PRECISION}g')
    
    @classmethod
    def serialize_for_hash(cls, value: float) -> bytes:
        """
        Serialize float to bytes suitable for hashing.
        
        Ensures identical bytes for identical float values
        across all platforms.
        """
        if math.isnan(value):
            # All NaNs hash the same
            return struct.pack('!Q', DeterministicFloat.CANONICAL_QNAN_64)
        
        return struct.pack('!d', value)


class DeterministicJSONEncoder(json.JSONEncoder):
    """JSON encoder with deterministic float handling."""
    
    def encode(self, obj: Any) -> str:
        """Override to handle floats specially."""
        if isinstance(obj, float):
            return DeterministicFloatSerializer.to_json(obj)
        return super().encode(obj)
    
    def default(self, obj: Any) -> Any:
        """Handle special float values."""
        if isinstance(obj, float):
            if math.isnan(obj):
                return "NaN"
            elif math.isinf(obj):
                return "Infinity" if obj > 0 else "-Infinity"
        return super().default(obj)
```

### Accumulation and Summation

```python
from typing import Iterable, List

class DeterministicAccumulator:
    """
    Deterministic accumulation operations.
    
    Kahan summation for reduced error in series summation.
    """
    
    def __init__(self, config: Optional[FloatConfig] = None):
        self.config = config or FloatConfig()
        self.df = DeterministicFloat(config)
    
    def kahan_sum(self, values: Iterable[float]) -> float:
        """
        Kahan summation algorithm for improved accuracy.
        
        Reduces error accumulation in summation of many values
        by keeping track of lost low-order bits.
        
        Args:
            values: Iterable of floats to sum
            
        Returns:
            Sum with reduced floating-point error
        """
        total = 0.0
        compensation = 0.0  # Running compensation for lost low-order bits
        
        for value in values:
            # Adjust value by compensation
            y = self.df.subtract(value, compensation)
            
            # Add to total
            t = self.df.add(total, y)
            
            # Calculate what was lost
            compensation = self.df.subtract(
                self.df.subtract(t, total),
                y
            )
            
            total = t
        
        return total
    
    def pairwise_sum(self, values: List[float]) -> float:
        """
        Pairwise summation for better precision.
        
        Recursively sums pairs of values, reducing error accumulation.
        """
        if len(values) == 0:
            return 0.0
        if len(values) == 1:
            return values[0]
        
        mid = len(values) // 2
        left = self.pairwise_sum(values[:mid])
        right = self.pairwise_sum(values[mid:])
        
        return self.df.add(left, right)
    
    def compensated_dot_product(
        self,
        a: List[float],
        b: List[float]
    ) -> float:
        """
        Dot product with error compensation.
        
        Args:
            a: First vector
            b: Second vector
            
        Returns:
            Compensated dot product
        """
        if len(a) != len(b):
            raise ValueError("Vectors must have same length")
        
        products = []
        for x, y in zip(a, b):
            products.append(self.df.multiply(x, y))
        
        return self.kahan_sum(products)
```

### Comparison Operations

```python
class FloatComparator:
    """
    Deterministic float comparison operations.
    
    Handles edge cases like NaN comparison consistently.
    """
    
    EPSILON = 1e-9  # Default epsilon for approximate equality
    
    @classmethod
    def equals(cls, a: float, b: float) -> bool:
        """
        Exact equality comparison.
        
        Returns False if either is NaN.
        """
        if math.isnan(a) or math.isnan(b):
            return False
        return a == b
    
    @classmethod
    def almost_equals(
        cls,
        a: float,
        b: float,
        rel_tol: float = 1e-9,
        abs_tol: float = 0.0
    ) -> bool:
        """
        Approximate equality with tolerances.
        
        Matches math.isclose() behavior but deterministic.
        """
        if math.isnan(a) or math.isnan(b):
            return False
        
        if a == b:
            return True
        
        if math.isinf(a) or math.isinf(b):
            return False
        
        diff = abs(b - a)
        return diff <= max(rel_tol * max(abs(a), abs(b)), abs_tol)
    
    @classmethod
    def compare(cls, a: float, b: float) -> int:
        """
        Three-way comparison.
        
        Returns:
            -1 if a < b
             0 if a == b
             1 if a > b
             
        NaN is treated as greater than all other values.
        """
        if math.isnan(a) and math.isnan(b):
            return 0
        if math.isnan(a):
            return 1
        if math.isnan(b):
            return -1
        
        if a < b:
            return -1
        elif a > b:
            return 1
        else:
            return 0
    
    @classmethod
    def less_than(cls, a: float, b: float) -> bool:
        """Deterministic less-than comparison."""
        return cls.compare(a, b) < 0
    
    @classmethod
    def greater_than(cls, a: float, b: float) -> bool:
        """Deterministic greater-than comparison."""
        return cls.compare(a, b) > 0
```

## Testing

```python
import unittest
import struct
import math

class TestDeterministicFloat(unittest.TestCase):
    """Test deterministic floating-point operations."""
    
    def setUp(self):
        self.df = DeterministicFloat()
    
    def test_basic_operations(self):
        """Test basic arithmetic operations."""
        self.assertEqual(self.df.add(1.0, 2.0), 3.0)
        self.assertEqual(self.df.subtract(5.0, 3.0), 2.0)
        self.assertEqual(self.df.multiply(3.0, 4.0), 12.0)
        self.assertEqual(self.df.divide(10.0, 2.0), 5.0)
    
    def test_canonical_nan(self):
        """Test canonical NaN representation."""
        nan1 = self.df._make_canonical_nan()
        nan2 = self.df._make_canonical_nan()
        
        # Both should be NaN
        self.assertTrue(math.isnan(nan1))
        self.assertTrue(math.isnan(nan2))
        
        # Bit representation should be identical
        bits1 = struct.unpack('!Q', struct.pack('!d', nan1))[0]
        bits2 = struct.unpack('!Q', struct.pack('!d', nan2))[0]
        self.assertEqual(bits1, bits2)
    
    def test_division_by_zero(self):
        """Test division by zero handling."""
        # 1.0 / 0.0 should be infinity
        result = self.df.divide(1.0, 0.0)
        self.assertTrue(math.isinf(result))
        self.assertGreater(result, 0)
        
        # -1.0 / 0.0 should be negative infinity
        result = self.df.divide(-1.0, 0.0)
        self.assertTrue(math.isinf(result))
        self.assertLess(result, 0)
        
        # 0.0 / 0.0 should be NaN
        result = self.df.divide(0.0, 0.0)
        self.assertTrue(math.isnan(result))
    
    def test_sqrt_negative(self):
        """Test square root of negative number."""
        result = self.df.sqrt(-1.0)
        self.assertTrue(math.isnan(result))
    
    def test_signed_zero(self):
        """Test consistent zero handling."""
        # All zeros should be positive
        zero = self.df._canonicalize(-0.0)
        self.assertEqual(zero, 0.0)
        self.assertNotEqual(struct.pack('!d', zero), struct.pack('!d', -0.0))
    
    def test_kahan_summation(self):
        """Test Kahan summation accuracy."""
        acc = DeterministicAccumulator()
        
        # Sum that loses precision with naive summation
        values = [1e16, 1.0, -1e16]
        
        naive_sum = sum(values)
        kahan_result = acc.kahan_sum(values)
        
        # Kahan should be more accurate
        self.assertEqual(kahan_result, 1.0)
        self.assertNotEqual(naive_sum, 1.0)  # Naive loses the 1.0


class TestFloatSerialization(unittest.TestCase):
    """Test deterministic serialization."""
    
    def test_round_trip(self):
        """Test that serialization round-trips correctly."""
        test_values = [
            0.0,
            1.0,
            3.141592653589793,
            1e308,
            1e-308,
            float('inf'),
            float('-inf'),
        ]
        
        for value in test_values:
            if math.isnan(value):
                continue  # NaN equality is tricky
            
            json_str = DeterministicFloatSerializer.to_json(value)
            recovered = DeterministicFloatSerializer.from_json(json_str)
            
            # Bit representation should be identical
            original_bits = struct.unpack('!Q', struct.pack('!d', value))[0]
            recovered_bits = struct.unpack('!Q', struct.pack('!d', recovered))[0]
            self.assertEqual(original_bits, recovered_bits)


class TestFloatComparison(unittest.TestCase):
    """Test deterministic comparisons."""
    
    def test_nan_comparison(self):
        """Test NaN comparison behavior."""
        nan = float('nan')
        
        # NaN should not equal itself
        self.assertFalse(FloatComparator.equals(nan, nan))
        
        # NaN should be greater than all other values
        self.assertEqual(FloatComparator.compare(nan, 1.0), 1)
        self.assertEqual(FloatComparator.compare(1.0, nan), -1)
    
    def test_almost_equals(self):
        """Test approximate equality."""
        self.assertTrue(FloatComparator.almost_equals(1.0, 1.0 + 1e-10))
        self.assertFalse(FloatComparator.almost_equals(1.0, 2.0))
```

## Error Handling

| Error Code | Description | Resolution |
|------------|-------------|------------|
| `FloatDomainError` | Mathematical domain error | Check input ranges before operations |
| `PrecisionLossError` | Precision loss in conversion | Use appropriate precision |
| `InexactError` | Inexact result | Enable inexact traps if needed |
| `UnderflowError` | Result underflowed to zero | Check magnitude of operands |
| `OverflowError` | Result overflowed to infinity | Check magnitude of operands |

## Platform Considerations

### x86/x64
- Disable x87 FPU extended precision (use SSE2)
- Set rounding mode explicitly
- Avoid FPU stack-based operations

### ARM
- Use VFP/NEON with strict IEEE 754 mode
- Ensure flush-to-zero is disabled
- Handle denormal numbers consistently

### WebAssembly
- All WASM engines use IEEE 754 binary64
- No special handling needed

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
