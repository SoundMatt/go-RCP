package rcp_test

import (
	"testing"

	rcp "github.com/SoundMatt/go-RCP"
)

// TestZone_String_Unknown covers the default branch of Zone.String for the
// zero value and for any value outside the defined zone set.
func TestZone_String_Unknown(t *testing.T) {
	for _, z := range []rcp.Zone{rcp.ZoneUnknown, rcp.Zone(200)} {
		if got := z.String(); got != "Unknown" {
			t.Errorf("Zone(%d).String() = %q, want %q", z, got, "Unknown")
		}
	}
}

// TestNewLoan_RoundTrip verifies NewLoan exposes the payload and that Return
// invokes the release function exactly once.
func TestNewLoan_RoundTrip(t *testing.T) {
	released := 0
	payload := []byte("loaned")
	loan := rcp.NewLoan(payload, func() { released++ })
	if string(loan.Payload) != "loaned" {
		t.Errorf("loan.Payload = %q, want %q", loan.Payload, "loaned")
	}
	loan.Return()
	if released != 1 {
		t.Errorf("release called %d times, want 1", released)
	}
}

// TestLoan_Return_NilRelease confirms Return is safe when no release function
// was supplied (REQ-LOAN-004: Return must not panic).
func TestLoan_Return_NilRelease(t *testing.T) {
	loan := rcp.NewLoan([]byte("x"), nil)
	loan.Return() // must not panic
}
