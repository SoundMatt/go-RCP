//fusa:test REQ-AVTP-021

package acf_test

import (
	"testing"

	"github.com/SoundMatt/go-RCP/v9/acf"
)

// ── REQ-AVTP-021: response classification (TC18 §11.3 Table 15) ───────────

func TestResponseKind_Acknowledge(t *testing.T) {
	m := acf.Message{EVT: 0x0F, Control: acf.FlagWrite | acf.FlagResponse}
	if got := m.ResponseKind(); got != acf.ResponseAcknowledge {
		t.Errorf("ResponseKind() = %v, want ResponseAcknowledge", got)
	}
}

func TestResponseKind_AcknowledgeFromReadOp(t *testing.T) {
	m := acf.Message{EVT: 0x0F, Control: acf.FlagRead | acf.FlagResponse}
	if got := m.ResponseKind(); got != acf.ResponseAcknowledge {
		t.Errorf("ResponseKind() = %v, want ResponseAcknowledge", got)
	}
}

// A rejected Acknowledge (evt = 0xF, err = 1) is still an Acknowledge per
// TC18 §11.3.1, not a ResponseError -- that's a distinct evt[3:0] < 0x9
// case (§11.3.4). err must not take priority over evt here.
func TestResponseKind_AcknowledgeRejectedIsNotError(t *testing.T) {
	m := acf.Message{EVT: 0x0F, Control: acf.FlagWrite | acf.FlagResponse | acf.FlagError}
	if got := m.ResponseKind(); got != acf.ResponseAcknowledge {
		t.Errorf("ResponseKind() = %v, want ResponseAcknowledge", got)
	}
}

func TestResponseKind_Write(t *testing.T) {
	m := acf.Message{EVT: 0, Control: acf.FlagWrite | acf.FlagResponse}
	if got := m.ResponseKind(); got != acf.ResponseWrite {
		t.Errorf("ResponseKind() = %v, want ResponseWrite", got)
	}
}

func TestResponseKind_Read(t *testing.T) {
	m := acf.Message{EVT: 0, Control: acf.FlagRead | acf.FlagResponse}
	if got := m.ResponseKind(); got != acf.ResponseRead {
		t.Errorf("ResponseKind() = %v, want ResponseRead", got)
	}
}

func TestResponseKind_Error(t *testing.T) {
	m := acf.Message{EVT: 0, Control: acf.FlagRead | acf.FlagResponse | acf.FlagError}
	if got := m.ResponseKind(); got != acf.ResponseError {
		t.Errorf("ResponseKind() = %v, want ResponseError", got)
	}
}

// evt[3:0] = 0x1..0x8 is the multi-response counter range (§11.3, not yet
// implemented as its own concept in this package) -- it must still fall
// through to the ordinary err/op classification below the Acknowledge
// check, not be misread as an Acknowledge.
func TestResponseKind_CounterRangeFallsThroughToWriteOrRead(t *testing.T) {
	m := acf.Message{EVT: 0x03, Control: acf.FlagWrite | acf.FlagResponse}
	if got := m.ResponseKind(); got != acf.ResponseWrite {
		t.Errorf("ResponseKind() = %v, want ResponseWrite", got)
	}
}
