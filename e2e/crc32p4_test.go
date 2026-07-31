package e2e

import (
	"hash/crc32"
	"testing"
)

// TestCRC32P4_KnownAnswerVector cross-checks crc32P4Table (TC18 §13.6 Table
// 31: polynomial 0xF4ACFB13, initial value 0xFFFFFFFF, both input and
// output reflected, final XOR 0xFFFFFFFF) against a publicly cataloged,
// independently verifiable check value: this exact parameter set is also
// known in the public CRC parameter catalog (reveng's "catalogue of
// parametrised CRC algorithms") as "CRC-32/AUTOSAR", whose own published
// check value for the standard "123456789" check string is 0x1697D06A — not
// a value derived from any of this repository's own code. Cross-referenced
// against cpp-RCP/c-RCP's own independent implementations of the identical
// algorithm: c-RCP's test_e2e.c::test_crc32_known_answer_vector asserts the
// identical 0x1697D06A value for rcp_e2e_crc32("123456789", 9), and
// cpp-RCP's e2e.hpp derives its own working polynomial via the same
// reflect32(0xF4ACFB13) construction this package's crc32P4ReflectedPoly
// constant already hardcodes.
func TestCRC32P4_KnownAnswerVector(t *testing.T) {
	h := crc32.New(crc32P4Table)
	h.Write([]byte("123456789"))
	const wantAUTOSARCheck = 0x1697D06A
	if got := h.Sum32(); got != wantAUTOSARCheck {
		t.Fatalf("CRC32P4(%q) = %#08x, want %#08x (CRC-32/AUTOSAR published check value)", "123456789", got, wantAUTOSARCheck)
	}
}

// TestCRC32P4_EmptyInput checks the CRC of no data at all is the init value
// XORed with the final XOR value — both 0xFFFFFFFF per Table 31 — which
// cancel out to exactly 0.
func TestCRC32P4_EmptyInput(t *testing.T) {
	h := crc32.New(crc32P4Table)
	if got := h.Sum32(); got != 0 {
		t.Fatalf("CRC32P4(nil) = %#08x, want 0x00000000", got)
	}
}

// TestCRC32P4_MatchesBitLevelReference cross-checks crc32P4Table's
// table-driven implementation against a from-scratch, non-table bit-by-bit
// implementation of the same algorithm (poly 0xF4ACFB13 in its normal,
// non-reflected form; init 0xFFFFFFFF; both input and output reflected;
// final XOR 0xFFFFFFFF) over several inputs — empty input, a single byte, a
// handful of bytes, the standard "123456789" check string, and a
// multi-byte buffer — proving the table built from crc32P4ReflectedPoly
// implements the same algorithm as an independent, from-first-principles
// derivation, not merely a self-consistent but wrong one.
func TestCRC32P4_MatchesBitLevelReference(t *testing.T) {
	cases := [][]byte{
		nil,
		{0x00},
		{0xFF},
		{0xAA, 0xBB, 0xCC},
		[]byte("123456789"),
		{0x00, 0x11, 0x22, 0x33, 0x44, 0x55, 0x66, 0x77, 0x88, 0x99, 0xAA, 0xBB, 0xCC, 0xDD, 0xEE, 0xFF},
	}
	for i, data := range cases {
		h := crc32.New(crc32P4Table)
		h.Write(data)
		got := h.Sum32()
		want := bitLevelCRC32P4(data)
		if got != want {
			t.Errorf("case %d (% X): table-driven CRC32P4 = %#08x, from-scratch bit-level reference = %#08x", i, data, got, want)
		}
	}
}

// bitLevelCRC32P4 is a from-scratch, non-table-driven reference
// implementation of CRC32P4 (TC18 §13.6 Table 31: polynomial 0xF4ACFB13 in
// its normal/non-reflected form, initial value 0xFFFFFFFF, both input and
// output reflected, final XOR 0xFFFFFFFF), used only to independently
// cross-check crc32P4Table/Compute's table-driven implementation in tests —
// never used by non-test code.
func bitLevelCRC32P4(data []byte) uint32 {
	const polyNormal uint32 = 0xF4ACFB13
	crc := uint32(0xFFFFFFFF)
	for _, b := range data {
		crc ^= uint32(reflectByte(b)) << 24
		for i := 0; i < 8; i++ {
			if crc&0x80000000 != 0 {
				crc = (crc << 1) ^ polyNormal
			} else {
				crc <<= 1
			}
		}
	}
	return reflect32(crc) ^ 0xFFFFFFFF
}

// reflectByte reverses the 8 bits of b (refin=true's per-byte step).
// reflect32 (refout=true's final step, and also used by crc.go itself to
// derive crc32P4Table's polynomial from crc32P4NormalPoly) already exists
// as a package-level helper in crc.go — reused here rather than
// duplicated.
func reflectByte(b byte) byte {
	var r byte
	for i := 0; i < 8; i++ {
		r = (r << 1) | (b & 1)
		b >>= 1
	}
	return r
}
