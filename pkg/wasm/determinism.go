package wasm

import (
	"encoding/binary"
	"math"
	"math/bits"
	"sort"
)

// IEEE754Config configures IEEE 754 floating point behavior.
type IEEE754Config struct {
	CanonicalizeNaN   bool `json:"canonicalize_nan"`
	DisableFMA        bool `json:"disable_fma"`
	DisableExceptions bool `json:"disable_exceptions"`
}

// DefaultIEEE754Config returns default IEEE 754 configuration.
func DefaultIEEE754Config() *IEEE754Config {
	return &IEEE754Config{
		CanonicalizeNaN:   true,
		DisableFMA:        true,
		DisableExceptions: true,
	}
}

// CanonicalNaN returns the canonical NaN value.
func CanonicalNaN() float64 {
	const canonicalNaN uint64 = 0x7FF8000000000000
	return math.Float64frombits(canonicalNaN)
}

// IsCanonicalNaN checks if a float64 is canonical NaN.
func IsCanonicalNaN(f float64) bool {
	bits := math.Float64bits(f)
	const canonicalValue = 0x7FF8000000000000
	const exponentMask = 0x7FF0000000000000
	const mantissaMask = 0x000FFFFFFFFFFFFF

	hasNaNExponent := (bits & exponentMask) == exponentMask
	hasMantissa := (bits & mantissaMask) != 0
	return hasNaNExponent && hasMantissa && bits == canonicalValue
}

// CanonicalizeFloat64 converts NaN to canonical form.
func CanonicalizeFloat64(f float64) float64 {
	if math.IsNaN(f) {
		return CanonicalNaN()
	}
	return f
}

// CanonicalizeFloat64Slice converts all NaNs in slice to canonical form.
func CanonicalizeFloat64Slice(s []float64) {
	for i := range s {
		s[i] = CanonicalizeFloat64(s[i])
	}
}

// FixedPoint represents decimal numbers as integers for deterministic arithmetic.
type FixedPoint int64

const (
	fixedPointScale = 1_000_000_000
)

// FromFloat converts float64 to fixed point.
func FromFloat(f float64) FixedPoint {
	return FixedPoint(f * fixedPointScale)
}

// Float converts fixed point to float64.
func (fp FixedPoint) Float() float64 {
	return float64(fp) / fixedPointScale
}

// Add adds two fixed point numbers.
func (fp FixedPoint) Add(other FixedPoint) FixedPoint {
	return fp + other
}

// Sub subtracts other from fp.
func (fp FixedPoint) Sub(other FixedPoint) FixedPoint {
	return fp - other
}

// Mul multiplies two fixed point numbers.
func (fp FixedPoint) Mul(other FixedPoint) FixedPoint {
	result := int64(fp) * int64(other) / fixedPointScale
	return FixedPoint(result)
}

// Div divides fp by other. Returns 0 if dividing by zero.
func (fp FixedPoint) Div(other FixedPoint) FixedPoint {
	if other == 0 {
		return 0
	}
	return FixedPoint(int64(fp) * fixedPointScale / int64(other))
}

// Cmp compares two fixed point numbers.
func (fp FixedPoint) Cmp(other FixedPoint) int {
	switch {
	case fp < other:
		return -1
	case fp > other:
		return 1
	default:
		return 0
	}
}

// DeterministicFloat provides deterministic floating point operations.
type DeterministicFloat struct {
	canonicalizeNaN bool
	disableFMA      bool
}

// NewDeterministicFloat creates a new deterministic float handler.
func NewDeterministicFloat(config *IEEE754Config) *DeterministicFloat {
	if config == nil {
		config = DefaultIEEE754Config()
	}
	return &DeterministicFloat{
		canonicalizeNaN: config.CanonicalizeNaN,
		disableFMA:      config.DisableFMA,
	}
}

// Add adds two floats deterministically.
func (df *DeterministicFloat) Add(a, b float64) float64 {
	result := a + b
	if df.canonicalizeNaN {
		result = CanonicalizeFloat64(result)
	}
	return result
}

// Sub subtracts b from a deterministically.
func (df *DeterministicFloat) Sub(a, b float64) float64 {
	result := a - b
	if df.canonicalizeNaN {
		result = CanonicalizeFloat64(result)
	}
	return result
}

// Mul multiplies two floats deterministically.
func (df *DeterministicFloat) Mul(a, b float64) float64 {
	result := a * b
	if df.canonicalizeNaN {
		result = CanonicalizeFloat64(result)
	}
	return result
}

// Div divides a by b deterministically.
func (df *DeterministicFloat) Div(a, b float64) float64 {
	result := a / b
	if df.canonicalizeNaN {
		result = CanonicalizeFloat64(result)
	}
	return result
}

// FMA performs fused multiply-add deterministically.
func (df *DeterministicFloat) FMA(a, b, c float64) float64 {
	if df.disableFMA {
		return df.Add(df.Mul(a, b), c)
	}
	result := math.FMA(a, b, c)
	if df.canonicalizeNaN {
		result = CanonicalizeFloat64(result)
	}
	return result
}

// SortMapKeys deterministically sorts map keys.
func SortMapKeys[K comparable](m map[K]bool) []K {
	keys := make([]K, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		return compareKeys(keys[i], keys[j])
	})
	return keys
}

// compareKeys compares two keys for deterministic sorting.
func compareKeys(a, any interface{}) bool {
	switch a := a.(type) {
	case string:
		if b, ok := any.(string); ok {
			return a < b
		}
	case int:
		if b, ok := any.(int); ok {
			return a < b
		}
	case int64:
		if b, ok := any.(int64); ok {
			return a < b
		}
	case uint64:
		if b, ok := any.(uint64); ok {
			return a < b
		}
	}
	return false
}

// DeterministicSeed provides a fixed seed for deterministic random.
type DeterministicSeed struct {
	seed uint64
}

// NewDeterministicSeed creates a new deterministic seed.
func NewDeterministicSeed(seed uint64) *DeterministicSeed {
	return &DeterministicSeed{seed: seed}
}

// Next advances the seed and returns next value.
func (ds *DeterministicSeed) Next() uint64 {
	ds.seed = bits.RotateLeft64(ds.seed, 41)
	ds.seed ^= ds.seed >> 6
	ds.seed ^= ds.seed << 29
	ds.seed ^= ds.seed >> 32
	return ds.seed
}

// Uint32 returns deterministic uint32.
func (ds *DeterministicSeed) Uint32() uint32 {
	return uint32(ds.Next())
}

// Uint64 returns deterministic uint64.
func (ds *DeterministicSeed) Uint64() uint64 {
	return ds.Next()
}

// Bytes returns deterministic random bytes.
func (ds *DeterministicSeed) Bytes(n int) []byte {
	buf := make([]byte, n*8)
	for i := 0; i < n; i++ {
		b := ds.Uint64()
		binary.LittleEndian.PutUint64(buf[i*8:(i+1)*8], b)
	}
	return buf[:n]
}

// Float64 returns deterministic float64 in [0, 1).
func (ds *DeterministicSeed) Float64() float64 {
	u := ds.Uint64()
	return float64(u>>11) / (1 << 53)
}
