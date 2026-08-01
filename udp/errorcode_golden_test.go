//fusa:test REQ-UDP-019

package udp_test

import (
	"bytes"
	"testing"

	"github.com/SoundMatt/go-RCP/udp"
)

// ── REQ-UDP-007/008 (golden-vector half): frozen ErrorCode numeric values
// and error-response body byte layout ──
//
// Every expected value below is hand-computed directly against the
// governing OPEN Alliance TC18 Remote Control Protocol Specification's
// error-code enumeration (Table 27 in that specification), not merely
// round-tripped through this package's own EncodeErrorBody/DecodeErrorBody
// — the same posture acf/acf_golden_test.go established for wire layouts
// elsewhere in this repo. This exists because an earlier fix pass made
// ErrorCode numeric-shaped (a leading byte on the wire, replacing free
// text) without checking the actual numeric assignments the byte was
// supposed to carry, so a self-referential round-trip test alone could not
// have caught that the values were wrong.

// goldenErrorCodeValues pins each ErrorCode symbol this package defines to
// its exact numeric assignment in the specification's error-code
// enumeration.
var goldenErrorCodeValues = []struct {
	name string
	code udp.ErrorCode
	want uint8
}{
	{"ErrorCodeUnsupportedCommand", udp.ErrorCodeUnsupportedCommand, 1},
	{"ErrorCodeSequencerNotKnown", udp.ErrorCodeSequencerNotKnown, 2},
	{"ErrorCodeUnauthorizedAccess", udp.ErrorCodeUnauthorizedAccess, 3},
	{"ErrorCodeLockedMemAccess", udp.ErrorCodeLockedMemAccess, 4},
	{"ErrorCodeRequestCancelled", udp.ErrorCodeRequestCancelled, 5},
	{"ErrorCodeRequestNotFound", udp.ErrorCodeRequestNotFound, 6},
	{"ErrorCodeEPError", udp.ErrorCodeEPError, 7},
	{"ErrorCodeEPNotFound", udp.ErrorCodeEPNotFound, 8},
	{"ErrorCodePWMInNoSignal", udp.ErrorCodePWMInNoSignal, 9},
	{"ErrorCodeRequestStorageOverflow", udp.ErrorCodeRequestStorageOverflow, 10},
	{"ErrorCodeRequestRejected", udp.ErrorCodeRequestRejected, 11},
	{"ErrorCodePOCIFailure", udp.ErrorCodePOCIFailure, 12},
	{"ErrorCodePresentationTimeTooFarInFuture", udp.ErrorCodePresentationTimeTooFarInFuture, 13},
	{"ErrorCodeGPTPFailure", udp.ErrorCodeGPTPFailure, 14},
	{"ErrorCodeInvalidParameter", udp.ErrorCodeInvalidParameter, 15},
	{"ErrorCodeChainAborted", udp.ErrorCodeChainAborted, 16},
	{"ErrorCodeChainError", udp.ErrorCodeChainError, 17},
}

func TestGolden_ErrorCodeValues(t *testing.T) {
	for _, tt := range goldenErrorCodeValues {
		t.Run(tt.name, func(t *testing.T) {
			if got := uint8(tt.code); got != tt.want {
				t.Errorf("%s = %d, want %d (per the specification's error-code enumeration)", tt.name, got, tt.want)
			}
			if !tt.code.Valid() {
				t.Errorf("%s.Valid() = false, want true", tt.name)
			}
		})
	}
}

// TestGolden_ErrorCode_Invalid checks a code outside the specification's
// 17-entry enumeration (0, 6+11=nonexistent gaps don't exist here since the
// table is contiguous 1-17, so this checks just below and above the range)
// is reported invalid rather than silently accepted.
func TestGolden_ErrorCode_Invalid(t *testing.T) {
	for _, c := range []udp.ErrorCode{0, 18, 19, 100, 255} {
		if udp.ErrorCode(c).Valid() {
			t.Errorf("ErrorCode(%d).Valid() = true, want false", c)
		}
	}
}

// goldenErrorBodyChainError is EncodeErrorBody(ErrorCodeChainError, "")'s
// wire body: a single leading byte 0x11 (17 decimal, CHAIN_ERROR's
// specification-assigned value), no trailing diagnostic.
var goldenErrorBodyChainError = []byte{0x11}

// goldenErrorBodyGPTPFailure is
// EncodeErrorBody(ErrorCodeGPTPFailure, "no time sync")'s wire body: a
// single leading byte 0x0E (14 decimal, GPTP_FAIL's specification-assigned
// value), followed by the diagnostic's raw UTF-8 bytes.
var goldenErrorBodyGPTPFailure = append([]byte{0x0E}, []byte("no time sync")...)

func TestGolden_EncodeErrorBody(t *testing.T) {
	if got := udp.EncodeErrorBody(udp.ErrorCodeChainError, ""); !bytes.Equal(got, goldenErrorBodyChainError) {
		t.Errorf("EncodeErrorBody(ChainError, \"\") = % X, want % X", got, goldenErrorBodyChainError)
	}
	if got := udp.EncodeErrorBody(udp.ErrorCodeGPTPFailure, "no time sync"); !bytes.Equal(got, goldenErrorBodyGPTPFailure) {
		t.Errorf("EncodeErrorBody(GPTPFailure, \"no time sync\") = % X, want % X", got, goldenErrorBodyGPTPFailure)
	}
}

func TestGolden_DecodeErrorBody(t *testing.T) {
	code, diag, err := udp.DecodeErrorBody(goldenErrorBodyChainError)
	if err != nil {
		t.Fatalf("DecodeErrorBody(goldenErrorBodyChainError): %v", err)
	}
	if code != udp.ErrorCodeChainError || diag != "" {
		t.Errorf("DecodeErrorBody(goldenErrorBodyChainError) = (%v, %q), want (ErrorCodeChainError, \"\")", code, diag)
	}

	code, diag, err = udp.DecodeErrorBody(goldenErrorBodyGPTPFailure)
	if err != nil {
		t.Fatalf("DecodeErrorBody(goldenErrorBodyGPTPFailure): %v", err)
	}
	if code != udp.ErrorCodeGPTPFailure || diag != "no time sync" {
		t.Errorf("DecodeErrorBody(goldenErrorBodyGPTPFailure) = (%v, %q), want (ErrorCodeGPTPFailure, \"no time sync\")", code, diag)
	}
}
