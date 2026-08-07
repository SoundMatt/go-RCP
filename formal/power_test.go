//fusa:test REQ-FORM-012
//fusa:test REQ-FORM-013
//fusa:test REQ-FORM-014

package formal_test

import (
	"testing"

	"github.com/SoundMatt/go-RCP/v9/avtp"
	"github.com/SoundMatt/go-RCP/v9/formal"
	"github.com/SoundMatt/go-RCP/v9/wakeup"
)

func powerRoot() avtp.StreamID {
	return avtp.NewStreamID([6]byte{0x04, 0x11, 0x22, 0x33, 0x44, 0x55}, 1)
}

// REQ-FORM-012: a full StandBy→Normal→Sleep→Normal wake cycle, paced by
// powerstate.Driver, satisfies every PowerInvariants property.
func TestPowerInvariants_WakeCycle(t *testing.T) {
	ep, drv, actions, sent, err := formal.NewWakeCycleTrace(powerRoot(), avtp.ByteBusID(1))
	if err != nil {
		t.Fatalf("NewWakeCycleTrace: %v", err)
	}
	trace := formal.PowerTrace(ep, drv, actions)

	checker := formal.NewChecker()
	for _, inv := range formal.PowerInvariants() {
		checker.Add(inv)
	}
	if err := checker.Check(trace); err != nil {
		t.Errorf("Check: %v", err)
	}

	if len(*sent) != 3 {
		t.Errorf("transmitted %d wake handshakes, want 3 (WakeHandshakeRepeatCount)", len(*sent))
	}
	for i, h := range *sent {
		if h.Sequence != uint16(i) {
			t.Errorf("sent[%d].Sequence = %d, want %d", i, h.Sequence, i)
		}
		if h.Start != wakeup.StartHot {
			t.Errorf("sent[%d].Start = %v, want StartHot (no SetRetentionLost call)", i, h.Start)
		}
	}
}

// REQ-FORM-013: the endpoint's final observed power state after the cycle
// is Normal, and its final drv_pending is 0 — the trace actually reaches
// the state PowerInvariants only checks for at some point.
func TestPowerInvariants_FinalState(t *testing.T) {
	ep, drv, actions, _, err := formal.NewWakeCycleTrace(powerRoot(), avtp.ByteBusID(1))
	if err != nil {
		t.Fatalf("NewWakeCycleTrace: %v", err)
	}
	trace := formal.PowerTrace(ep, drv, actions)
	final := trace[len(trace)-1]

	if power, _ := final["power"].(string); power != wakeup.PowerNormal.String() {
		t.Errorf("final power = %q, want %q", power, wakeup.PowerNormal.String())
	}
	if pending, _ := final["drv_pending"].(int); pending != 0 {
		t.Errorf("final drv_pending = %d, want 0", pending)
	}
}

// REQ-FORM-014: PowerTrace produces exactly len(actions)+1 states,
// regardless of which package's types it is snapshotting — the same
// contract LifecycleTrace and SafeStateTrace each independently uphold.
func TestPowerTrace_Length(t *testing.T) {
	ep, drv, actions, _, err := formal.NewWakeCycleTrace(powerRoot(), avtp.ByteBusID(1))
	if err != nil {
		t.Fatalf("NewWakeCycleTrace: %v", err)
	}
	trace := formal.PowerTrace(ep, drv, actions)
	if len(trace) != len(actions)+1 {
		t.Errorf("len(trace) = %d, want %d", len(trace), len(actions)+1)
	}
}
