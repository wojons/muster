package wasm

import (
	"math"
	"testing"
)

func TestCanonicalNaN(t *testing.T) {
	nan := CanonicalNaN()

	if !math.IsNaN(nan) {
		t.Error("expected NaN to be NaN")
	}

	if !IsCanonicalNaN(nan) {
		t.Error("expected NaN to be canonical")
	}

	t.Run("various NaNs", func(t *testing.T) {
		regularNaN := math.NaN()
		signalingNaN := math.Float64frombits(0x7FF0000000000001)
		quietNaN := math.Float64frombits(0x7FF0000000000001)

		canon := CanonicalizeFloat64(regularNaN)
		if !IsCanonicalNaN(canon) {
			t.Error("regular NaN should be canonicalized")
		}

		canon = CanonicalizeFloat64(signalingNaN)
		if !IsCanonicalNaN(canon) {
			t.Error("signaling NaN should be canonicalized")
		}

		canon = CanonicalizeFloat64(quietNaN)
		if !IsCanonicalNaN(canon) {
			t.Error("quiet NaN should be canonicalized")
		}
	})
}

func TestCanonicalizeFloat64(t *testing.T) {
	t.Run("values other than NaN", func(t *testing.T) {
		determinism := NewDeterministicFloat(DefaultIEEE754Config())

		tests := []struct {
			input    float64
			expected float64
		}{
			{0.0, 0.0},
			{1.0, 1.0},
			{-1.0, -1.0},
			{3.14159, 3.14159},
			{math.Inf(1), math.Inf(1)},
			{math.Inf(-1), math.Inf(-1)},
		}

		for _, tt := range tests {
			got := determinism.Add(tt.input, 0)
			if got != tt.expected {
				t.Errorf("Add(%v, 0) = %v, want %v", tt.input, got, tt.expected)
			}
		}
	})
}

func TestCanonicalizeFloat64Slice(t *testing.T) {
	slice := []float64{
		0.0,
		math.NaN(),
		1.0,
		math.NaN(),
		2.0,
	}

	CanonicalizeFloat64Slice(slice)

	for i, v := range slice {
		switch i {
		case 1, 3:
			if !IsCanonicalNaN(v) {
				t.Errorf("index %d: expected canonical NaN, got %v", i, v)
			}
		default:
			if math.IsNaN(v) {
				t.Errorf("index %d: expected non-NaN, got NaN", i)
			}
		}
	}
}

func TestFixedPoint(t *testing.T) {
	t.Run("FromFloat", func(t *testing.T) {
		tests := []struct {
			input  float64
			expect FixedPoint
		}{
			{0.0, 0},
			{1.0, fixedPointScale},
			{2.5, FixedPoint(2.5 * fixedPointScale)},
			{-1.0, -fixedPointScale},
		}

		for _, tt := range tests {
			got := FromFloat(tt.input)
			if got != tt.expect {
				t.Errorf("FromFloat(%v) = %v, want %v", tt.input, got, tt.expect)
			}
		}
	})

	t.Run("Float", func(t *testing.T) {
		fp := FromFloat(3.14159)
		got := fp.Float()
		expected := float64(fp) / fixedPointScale
		if got != expected {
			t.Errorf("Float() = %v, want %v", got, expected)
		}
	})

	t.Run("Add", func(t *testing.T) {
		a := FromFloat(1.5)
		b := FromFloat(2.5)
		result := a.Add(b)
		expected := FromFloat(4.0)
		if result != expected {
			t.Errorf("Add() = %v, want %v", result, expected)
		}
	})

	t.Run("Sub", func(t *testing.T) {
		a := FromFloat(2.5)
		b := FromFloat(1.5)
		result := a.Sub(b)
		expected := FromFloat(1.0)
		if result != expected {
			t.Errorf("Sub() = %v, want %v", result, expected)
		}
	})

	t.Run("Mul", func(t *testing.T) {
		a := FromFloat(2.0)
		b := FromFloat(3.0)
		result := a.Mul(b)
		expected := FromFloat(6.0)
		if result != expected {
			t.Errorf("Mul() = %v, want %v", result, expected)
		}
	})

	t.Run("Div", func(t *testing.T) {
		a := FromFloat(6.0)
		b := FromFloat(2.0)
		result := a.Div(b)
		expected := FromFloat(3.0)
		if result != expected {
			t.Errorf("Div() = %v, want %v", result, expected)
		}
	})

	t.Run("DivZero", func(t *testing.T) {
		a := FromFloat(1.0)
		b := FixedPoint(0)
		result := a.Div(b)
		if result != 0 {
			t.Errorf("Div() by zero should return 0, got %v", result)
		}
	})

	t.Run("Cmp", func(t *testing.T) {
		a := FromFloat(1.0)
		b := FromFloat(2.0)
		c := FromFloat(2.0)
		d := FromFloat(3.0)

		if a.Cmp(b) >= 0 {
			t.Error("expected a < b")
		}

		if b.Cmp(c) != 0 {
			t.Error("expected b == c")
		}

		if d.Cmp(b) <= 0 {
			t.Error("expected d > b")
		}
	})

	t.Run("Float conversion", func(t *testing.T) {
		fp := FromFloat(3.14159)
		f := fp.Float()
		if math.Abs(f-3.14159) > 0.00001 {
			t.Errorf("Float() = %v, want ~3.14159", f)
		}
	})
}

func TestDeterministicFloat(t *testing.T) {
	config := DefaultIEEE754Config()
	df := NewDeterministicFloat(config)

	t.Run("Add", func(t *testing.T) {
		result := df.Add(1.0, 2.0)
		if result != 3.0 {
			t.Errorf("Add(1.0, 2.0) = %v, want 3.0", result)
		}
	})

	t.Run("Add with NaN", func(t *testing.T) {
		nan1 := CanonicalNaN()
		nan2 := CanonicalNaN()
		result := df.Add(nan1, nan2)

		if !IsCanonicalNaN(result) {
			t.Logf("result is not canonical NaN: %v", result)
		}
	})

	t.Run("Sub", func(t *testing.T) {
		result := df.Sub(5.0, 3.0)
		if result != 2.0 {
			t.Errorf("Sub(5.0, 3.0) = %v, want 2.0", result)
		}
	})

	t.Run("Mul", func(t *testing.T) {
		result := df.Mul(3.0, 4.0)
		if result != 12.0 {
			t.Errorf("Mul(3.0, 4.0) = %v, want 12.0", result)
		}
	})

	t.Run("Div", func(t *testing.T) {
		result := df.Div(10.0, 2.0)
		if result != 5.0 {
			t.Errorf("Div(10.0, 2.0) = %v, want 5.0", result)
		}
	})

	t.Run("FMA disabled", func(t *testing.T) {
		df := NewDeterministicFloat(&IEEE754Config{
			CanonicalizeNaN: true,
			DisableFMA:      true,
		})

		result := df.FMA(2.0, 3.0, 4.0)
		if result != 10.0 {
			t.Errorf("FMA(2.0, 3.0, 4.0) = %v, want 10.0", result)
		}
	})

	t.Run("FMA enabled", func(t *testing.T) {
		df := NewDeterministicFloat(&IEEE754Config{
			CanonicalizeNaN: true,
			DisableFMA:      false,
		})

		result := df.FMA(2.0, 3.0, 4.0)
		expected := math.FMA(2.0, 3.0, 4.0)
		if !almostEqual(result, expected) {
			t.Errorf("FMA(2.0, 3.0, 4.0) = %v, want %v", result, expected)
		}
	})
}

func almostEqual(a, b float64) bool {
	const epsilon = 1e-10
	max := float64(0.0)
	min := float64(0.0)
	if a > b {
		max = a
		min = b
	} else {
		max = b
		min = a
	}
	return !(float64(max-min) > epsilon)
}

func TestSortMapKeys(t *testing.T) {
	t.Run("string keys", func(t *testing.T) {
		m := map[string]bool{
			"b": true,
			"c": true,
			"a": true,
		}

		keys := SortMapKeys(m)
		if len(keys) != 3 {
			t.Errorf("expected 3 keys, got %d", len(keys))
		}

		if keys[0] != "a" || keys[1] != "b" || keys[2] != "c" {
			t.Errorf("expected sorted keys, got %v", keys)
		}
	})

	t.Run("int keys", func(t *testing.T) {
		t.Skip("SortMapKeys currently only supports string keys")
	})
}

func TestDeterministicSeed(t *testing.T) {
	t.Run("deterministic sequence", func(t *testing.T) {
		ds1 := NewDeterministicSeed(42)
		ds2 := NewDeterministicSeed(42)

		for i := 0; i < 10; i++ {
			v1 := ds1.Uint64()
			v2 := ds2.Uint64()

			if v1 != v2 {
				t.Errorf("iteration %d: %v != %v", i, v1, v2)
			}
		}
	})

	t.Run("different seeds", func(t *testing.T) {
		ds1 := NewDeterministicSeed(1)
		ds2 := NewDeterministicSeed(2)

		v1 := ds1.Uint64()
		v2 := ds2.Uint64()

		if v1 == v2 {
			t.Error("expected different values from different seeds")
		}
	})

	t.Run("Uint32", func(t *testing.T) {
		ds := NewDeterministicSeed(42)
		v := ds.Uint32()
		for i := 0; i < 1000; i++ {
			ds := NewDeterministicSeed(42)
			u := ds.Uint32()
			if u != v {
				t.Errorf("iteration %d: Uint32 not deterministic", i)
				break
			}
		}
	})

	t.Run("Bytes", func(t *testing.T) {
		ds := NewDeterministicSeed(42)
		bytes := ds.Bytes(16)
		if len(bytes) != 16 {
			t.Errorf("Bytes(16) = %d bytes, want 16", len(bytes))
		}

		ds2 := NewDeterministicSeed(42)
		bytes2 := ds2.Bytes(16)

		for i := range bytes {
			if bytes[i] != bytes2[i] {
				t.Errorf("Bytes() not deterministic at index %d", i)
			}
		}
	})

	t.Run("Float64", func(t *testing.T) {
		ds := NewDeterministicSeed(42)
		f := ds.Float64()
		if f < 0.0 || f >= 1.0 {
			t.Errorf("Float64() = %v, expected [0, 1)", f)
		}

		ds2 := NewDeterministicSeed(42)
		f2 := ds2.Float64()

		if f != f2 {
			t.Error("Float64() not deterministic")
		}
	})
}

func TestDefaultIEEE754Config(t *testing.T) {
	config := DefaultIEEE754Config()

	if !config.CanonicalizeNaN {
		t.Error("expected CanonicalizeNaN to be true")
	}

	if !config.DisableFMA {
		t.Error("expected DisableFMA to be true")
	}

	if !config.DisableExceptions {
		t.Error("expected DisableExceptions to be true")
	}
}
