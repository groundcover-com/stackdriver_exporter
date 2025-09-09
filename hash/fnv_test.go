package hash

import (
	"testing"
)

func TestAddUint64ByteByByte(t *testing.T) {
	testCases := []struct {
		name     string
		value    uint64
		expected string // We'll verify the implementation is correct by testing consistency
	}{
		{"zero", 0, "consistent"},
		{"small_value", 42, "consistent"},
		{"large_value", 0xDEADBEEFCAFEBABE, "consistent"},
		{"max_uint64", ^uint64(0), "consistent"},
		{"timestamp_like", 1694174400000000000, "consistent"}, // 2023-09-08 12:00:00 UTC in nanoseconds
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			h1 := New()
			h2 := New()

			// Test that the function is deterministic
			result1 := AddUint64(h1, tc.value)
			result2 := AddUint64(h2, tc.value)

			if result1 != result2 {
				t.Errorf("AddUint64 is not deterministic: got %d and %d for value %d", result1, result2, tc.value)
			}

			// Test that different values produce different hashes (with high probability)
			if tc.value != 0 {
				h3 := New()
				differentResult := AddUint64(h3, tc.value+1)
				if result1 == differentResult {
					t.Errorf("AddUint64 produced same hash for different values: %d and %d both gave %d", tc.value, tc.value+1, result1)
				}
			}
		})
	}
}

func TestAddUint64ByteOrderConsistency(t *testing.T) {
	// Test that the function processes bytes in little-endian order (LSB first)
	testValue := uint64(0x0102030405060708)

	h := New()
	result := AddUint64(h, testValue)

	// Manually process the same value byte by byte to verify
	h2 := New()
	bytes := []byte{0x08, 0x07, 0x06, 0x05, 0x04, 0x03, 0x02, 0x01} // LSB first
	for _, b := range bytes {
		h2 = AddByte(h2, b)
	}

	if result != h2 {
		t.Errorf("AddUint64 does not process bytes in little-endian order: got %d, expected %d", result, h2)
	}
}

func TestAddUint64VsOldImplementation(t *testing.T) {
	// This test demonstrates why the old implementation was incorrect
	testValue := uint64(0x0102030405060708)

	// New implementation (correct FNV-1a)
	h1 := New()
	newResult := AddUint64(h1, testValue)

	// Old implementation (incorrect - XOR entire value at once)
	h2 := New()
	oldResult := h2 ^ testValue
	oldResult *= prime64

	// They should be different (the old way was wrong)
	if newResult == oldResult {
		t.Errorf("New and old implementations should produce different results, but both gave %d", newResult)
	}

	t.Logf("New (correct) implementation: %d", newResult)
	t.Logf("Old (incorrect) implementation: %d", oldResult)
}

func BenchmarkAddUint64(b *testing.B) {
	h := New()
	testValue := uint64(1694174400000000000)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		h = AddUint64(h, testValue)
	}
}

func BenchmarkAddUint64Different(b *testing.B) {
	testValue := uint64(1694174400000000000)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		h := New()
		AddUint64(h, testValue+uint64(i))
	}
}
