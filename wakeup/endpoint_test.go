//fusa:test REQ-WAKEUP-005
//fusa:test REQ-WAKEUP-006
//fusa:test REQ-WAKEUP-007
//fusa:test REQ-WAKEUP-008

package wakeup_test

import (
	"errors"
	"testing"

	"github.com/SoundMatt/go-RCP/v9/acf"
	"github.com/SoundMatt/go-RCP/v9/avtp"
	"github.com/SoundMatt/go-RCP/v9/regmap"
	"github.com/SoundMatt/go-RCP/v9/wakeup"
)

func readReq() acf.Message {
	return acf.Message{Kind: acf.KindShort, ByteBusID: avtp.ByteBusID(1), Control: acf.FlagRead}
}

func writeReq(target wakeup.PowerState) acf.Message {
	return acf.Message{
		Kind:      acf.KindShort,
		ByteBusID: avtp.ByteBusID(1),
		Control:   acf.FlagWrite,
		Body:      wakeup.EncodePowerStateRequest(target),
	}
}

// TestHandleRequest_RequiresReadOrWriteWrongEndpointOrAccess checks a
// request with neither Read nor Write set, one addressed to the wrong
// endpoint, and one from a stream with no access grant are all rejected
// (REQ-WAKEUP-005).
func TestHandleRequest_RequiresReadOrWriteWrongEndpointOrAccess(t *testing.T) {
	ep, root := newDeclaredEndpoint(t)
	if err := ep.Configure(root, defaultConfig()); err != nil {
		t.Fatalf("Configure: %v", err)
	}

	neither := acf.Message{Kind: acf.KindShort, ByteBusID: avtp.ByteBusID(1)}
	if _, err := ep.HandleRequest(root, neither); !errors.Is(err, wakeup.ErrRequestMustReadOrWrite) {
		t.Errorf("HandleRequest(neither flag) err = %v, want ErrRequestMustReadOrWrite", err)
	}

	wrongAddr := readReq()
	wrongAddr.ByteBusID = 2
	if _, err := ep.HandleRequest(root, wrongAddr); !errors.Is(err, wakeup.ErrWrongEndpoint) {
		t.Errorf("HandleRequest(wrong addr) err = %v, want ErrWrongEndpoint", err)
	}

	stranger := avtp.NewStreamID([6]byte{0x07, 0xAA, 0xBB, 0xCC, 0xDD, 0xEE}, 9)
	if _, err := ep.HandleRequest(stranger, readReq()); !errors.Is(err, regmap.ErrAccessDenied) {
		t.Errorf("HandleRequest(no grant) err = %v, want regmap.ErrAccessDenied", err)
	}
}

// TestHandleRequest_RejectsDisabledAndUnpoweredTarget checks a request
// against a disabled endpoint, and a write request targeting
// PowerUnpowered, are both rejected (REQ-WAKEUP-006).
func TestHandleRequest_RejectsDisabledAndUnpoweredTarget(t *testing.T) {
	ep, root := newDeclaredEndpoint(t)
	if _, err := ep.HandleRequest(root, readReq()); !errors.Is(err, wakeup.ErrNotConfigured) {
		t.Errorf("HandleRequest(disabled) err = %v, want ErrNotConfigured", err)
	}

	if err := ep.Configure(root, defaultConfig()); err != nil {
		t.Fatalf("Configure: %v", err)
	}
	if _, err := ep.HandleRequest(root, writeReq(wakeup.PowerUnpowered)); !errors.Is(err, wakeup.ErrCannotRequestUnpowered) {
		t.Errorf("HandleRequest(write Unpowered) err = %v, want ErrCannotRequestUnpowered", err)
	}
	if got := ep.State(); got != wakeup.PowerNormal {
		t.Errorf("State() after rejected write = %v, want unchanged PowerNormal", got)
	}
}

// TestHandleRequest_ReadWriteRoundTrip checks a write request transitions
// state and echoes the target, a read reports the current state, and a
// same-state write is an idempotent no-op that still succeeds
// (REQ-WAKEUP-007).
func TestHandleRequest_ReadWriteRoundTrip(t *testing.T) {
	ep, root := newDeclaredEndpoint(t)
	if err := ep.Configure(root, defaultConfig()); err != nil {
		t.Fatalf("Configure: %v", err)
	}

	resp, err := ep.HandleRequest(root, readReq())
	if err != nil {
		t.Fatalf("HandleRequest(read, initial): %v", err)
	}
	if got, _ := wakeup.DecodePowerStateResponse(resp.Body); got != wakeup.PowerNormal {
		t.Errorf("HandleRequest(read, initial) = %v, want PowerNormal", got)
	}

	resp, err = ep.HandleRequest(root, writeReq(wakeup.PowerStandBy))
	if err != nil {
		t.Fatalf("HandleRequest(write StandBy): %v", err)
	}
	if got, _ := wakeup.DecodePowerStateResponse(resp.Body); got != wakeup.PowerStandBy {
		t.Errorf("HandleRequest(write StandBy) echoed = %v, want PowerStandBy", got)
	}
	if got := ep.State(); got != wakeup.PowerStandBy {
		t.Errorf("State() after write = %v, want PowerStandBy", got)
	}

	// Idempotent no-op re-request of the same state.
	if _, err := ep.HandleRequest(root, writeReq(wakeup.PowerStandBy)); err != nil {
		t.Errorf("HandleRequest(write same state again) err = %v, want nil", err)
	}
	if triggers := ep.DrainTriggers(); len(triggers) != 1 {
		t.Errorf("DrainTriggers() after one real + one no-op transition = %+v, want exactly 1 event", triggers)
	}
}

// TestHandleRequest_WakeFromSleepDeterminesStartKindAndQueuesHandshakes
// checks a Sleep→Normal wake determines Hot/Cold start per
// SetRetentionLost, queues the configured number of wake-handshake
// triggers, and that AcknowledgeWake discards any still-pending ones
// (REQ-WAKEUP-008).
func TestHandleRequest_WakeFromSleepDeterminesStartKindAndQueuesHandshakes(t *testing.T) {
	ep, root := newDeclaredEndpoint(t)
	cfg := defaultConfig() // WakeHandshakeRepeatCount: 3
	if err := ep.Configure(root, cfg); err != nil {
		t.Fatalf("Configure: %v", err)
	}

	if _, err := ep.HandleRequest(root, writeReq(wakeup.PowerSleep)); err != nil {
		t.Fatalf("HandleRequest(write Sleep): %v", err)
	}
	ep.DrainTriggers() // clear the Normal->Sleep transition's own event

	// Hot start: no SetRetentionLost call.
	if _, err := ep.HandleRequest(root, writeReq(wakeup.PowerNormal)); err != nil {
		t.Fatalf("HandleRequest(write Normal, hot wake): %v", err)
	}
	if got := ep.LastStartKind(); got != wakeup.StartHot {
		t.Errorf("LastStartKind() after hot wake = %v, want StartHot", got)
	}
	triggers := ep.DrainTriggers()
	if len(triggers) != 1+int(cfg.WakeHandshakeRepeatCount) {
		t.Fatalf("DrainTriggers() after hot wake len = %d, want %d", len(triggers), 1+cfg.WakeHandshakeRepeatCount)
	}
	if triggers[0].Kind != wakeup.TriggerPowerStateChanged || triggers[0].State != wakeup.PowerNormal {
		t.Errorf("triggers[0] = %+v, want TriggerPowerStateChanged/PowerNormal", triggers[0])
	}
	for i, seq := 1, uint16(0); i < len(triggers); i, seq = i+1, seq+1 {
		if triggers[i].Kind != wakeup.TriggerWakeHandshake || triggers[i].Handshake.Start != wakeup.StartHot || triggers[i].Handshake.Sequence != seq {
			t.Errorf("triggers[%d] = %+v, want TriggerWakeHandshake/StartHot/Sequence=%d", i, triggers[i], seq)
		}
	}

	// Cold start: SetRetentionLost called while asleep.
	if _, err := ep.HandleRequest(root, writeReq(wakeup.PowerSleep)); err != nil {
		t.Fatalf("HandleRequest(write Sleep again): %v", err)
	}
	ep.DrainTriggers()
	ep.SetRetentionLost()
	if _, err := ep.HandleRequest(root, writeReq(wakeup.PowerNormal)); err != nil {
		t.Fatalf("HandleRequest(write Normal, cold wake): %v", err)
	}
	if got := ep.LastStartKind(); got != wakeup.StartCold {
		t.Errorf("LastStartKind() after cold wake = %v, want StartCold", got)
	}

	// AcknowledgeWake discards remaining handshake events but not other
	// kinds queued alongside them.
	ep.AcknowledgeWake()
	if remaining := ep.DrainTriggers(); len(remaining) != 1 || remaining[0].Kind != wakeup.TriggerPowerStateChanged {
		t.Errorf("DrainTriggers() after AcknowledgeWake = %+v, want only the TriggerPowerStateChanged event", remaining)
	}
}
