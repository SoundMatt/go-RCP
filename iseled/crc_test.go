package iseled_test

import (
	"testing"

	"github.com/SoundMatt/go-RCP/v9/iseled"
)

// TestComputeCRC_Deterministic checks ComputeCRC is a pure, deterministic
// function of its input: same input always yields the same output, and
// different input yields (overwhelmingly likely) a different one.
func TestComputeCRC_Deterministic(t *testing.T) {
	a := iseled.ComputeCRC([]byte{0x01, 0x02, 0x03})
	b := iseled.ComputeCRC([]byte{0x01, 0x02, 0x03})
	if a != b {
		t.Errorf("ComputeCRC not deterministic: %#x != %#x", a, b)
	}
	c := iseled.ComputeCRC([]byte{0x01, 0x02, 0x04})
	if a == c {
		t.Errorf("ComputeCRC([0x01,0x02,0x03]) == ComputeCRC([0x01,0x02,0x04]) = %#x, want distinct values", a)
	}
	if empty := iseled.ComputeCRC(nil); empty == a {
		t.Errorf("ComputeCRC(nil) == ComputeCRC([0x01,0x02,0x03]), want distinct values")
	}
}
