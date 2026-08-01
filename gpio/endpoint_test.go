//fusa:test REQ-GPIO-006
//fusa:test REQ-GPIO-007
//fusa:test REQ-GPIO-008
//fusa:test REQ-GPIO-009
//fusa:test REQ-GPIO-010

package gpio_test

import (
	"errors"
	"testing"

	"github.com/SoundMatt/go-RCP/acf"
	"github.com/SoundMatt/go-RCP/avtp"
	"github.com/SoundMatt/go-RCP/gpio"
	"github.com/SoundMatt/go-RCP/regmap"
)

// TestHandleRequest_ReadReturnsCurrentValue checks a read request returns the
// endpoint's current pin value, and a request with neither Read nor Write is
// rejected (REQ-GPIO-006).
func TestHandleRequest_ReadReturnsCurrentValue(t *testing.T) {
	cfg := gpio.Config{PinCount: 4, Direction: 0b0011}
	ep, root := newConfiguredEndpoint(t, cfg)
	writeAndGetValue(t, ep, root, acf.EVTSelector0, 0b0001)

	req := acf.Message{Kind: acf.KindShort, ByteBusID: avtp.ByteBusID(1), Control: acf.FlagRead}
	resp, err := ep.HandleRequest(root, req)
	if err != nil {
		t.Fatalf("HandleRequest: %v", err)
	}
	if !resp.Control.Has(acf.FlagResponse | acf.FlagRead) {
		t.Errorf("response Control = %v, want FlagResponse|FlagRead set", resp.Control)
	}
	v, err := gpio.DecodeValue(resp.Body)
	if err != nil {
		t.Fatalf("DecodeValue: %v", err)
	}
	if v != 0b0001 {
		t.Errorf("read value = %04b, want 0001", v)
	}

	noFlags := acf.Message{Kind: acf.KindShort, ByteBusID: avtp.ByteBusID(1)}
	if _, err := ep.HandleRequest(root, noFlags); !errors.Is(err, gpio.ErrRequestMustReadOrWrite) {
		t.Errorf("HandleRequest(no flags) err = %v, want ErrRequestMustReadOrWrite", err)
	}
}

// TestHandleRequest_WrongEndpointOrNoAccess checks a request addressed to a
// different byte_bus_id, and a request from a stream with no access grant,
// are both rejected (REQ-GPIO-007).
func TestHandleRequest_WrongEndpointOrNoAccess(t *testing.T) {
	cfg := gpio.Config{PinCount: 4, Direction: 0b1111}
	ep, root := newConfiguredEndpoint(t, cfg)

	wrongAddr := acf.Message{Kind: acf.KindShort, ByteBusID: avtp.ByteBusID(2), Control: acf.FlagRead}
	if _, err := ep.HandleRequest(root, wrongAddr); !errors.Is(err, gpio.ErrWrongEndpoint) {
		t.Errorf("HandleRequest(wrong addr) err = %v, want ErrWrongEndpoint", err)
	}

	stranger := avtp.NewStreamID([6]byte{0x02, 0xAA, 0xBB, 0xCC, 0xDD, 0xEE}, 9)
	req := acf.Message{Kind: acf.KindShort, ByteBusID: avtp.ByteBusID(1), Control: acf.FlagRead}
	if _, err := ep.HandleRequest(stranger, req); !errors.Is(err, regmap.ErrAccessDenied) {
		t.Errorf("HandleRequest(no grant) err = %v, want regmap.ErrAccessDenied", err)
	}
}

// TestTriggers_OnlyEnabledPinsQueue checks a Value change on a
// TriggerEnable-marked pin is queued, and a change on a non-trigger pin is
// not (REQ-GPIO-008).
func TestTriggers_OnlyEnabledPinsQueue(t *testing.T) {
	// Pin 0 triggers, pin 1 does not; both output.
	cfg := gpio.Config{PinCount: 2, Direction: 0b11, TriggerEnable: 0b01}
	ep, root := newConfiguredEndpoint(t, cfg)

	writeAndGetValue(t, ep, root, acf.EVTSelector1, 0b10) // pin 1 only: no trigger
	if got := ep.DrainTriggers(); got != nil {
		t.Errorf("DrainTriggers() after non-trigger pin change = %+v, want nil", got)
	}

	writeAndGetValue(t, ep, root, acf.EVTSelector1, 0b01) // pin 0: triggers
	got := ep.DrainTriggers()
	if len(got) != 1 {
		t.Fatalf("DrainTriggers() = %+v, want 1 event", got)
	}
	if got[0].ChangedMask != 0b01 || got[0].Value != 0b11 {
		t.Errorf("trigger event = %+v, want {ChangedMask:0b01 Value:0b11}", got[0])
	}
}

// TestSetInputs_OnlyAffectsInputPins checks SetInputs only drives
// input-direction pins and queues triggers the same way a write does
// (REQ-GPIO-009).
func TestSetInputs_OnlyAffectsInputPins(t *testing.T) {
	// Pin 0 output (trigger-enabled, to prove SetInputs can't touch it),
	// pin 1 input (trigger-enabled).
	cfg := gpio.Config{PinCount: 2, Direction: 0b01, TriggerEnable: 0b11}
	ep, root := newConfiguredEndpoint(t, cfg)
	writeAndGetValue(t, ep, root, acf.EVTSelector1, 0b01) // drive output pin 0 high
	ep.DrainTriggers()                                    // clear the write's own trigger

	got := ep.SetInputs(0b11) // attempt to drive both pins high externally
	if got != 0b11 {
		t.Fatalf("SetInputs value = %02b, want 11 (output pin retains its driven value)", got)
	}
	triggers := ep.DrainTriggers()
	if len(triggers) != 1 || triggers[0].ChangedMask != 0b10 {
		t.Errorf("triggers after SetInputs = %+v, want exactly pin 1 (0b10) changed", triggers)
	}
}

// TestDrainTriggers_FIFOAndClears checks DrainTriggers returns events in
// order and clears the queue (REQ-GPIO-010).
func TestDrainTriggers_FIFOAndClears(t *testing.T) {
	cfg := gpio.Config{PinCount: 2, Direction: 0b11, TriggerEnable: 0b11}
	ep, root := newConfiguredEndpoint(t, cfg)

	writeAndGetValue(t, ep, root, acf.EVTSelector1, 0b01)
	writeAndGetValue(t, ep, root, acf.EVTSelector1, 0b10)

	got := ep.DrainTriggers()
	if len(got) != 2 {
		t.Fatalf("DrainTriggers() = %+v, want 2 events", got)
	}
	if got[0].ChangedMask != 0b01 || got[1].ChangedMask != 0b10 {
		t.Errorf("DrainTriggers() order = %+v, want [0b01, 0b10]", got)
	}
	if again := ep.DrainTriggers(); again != nil {
		t.Errorf("DrainTriggers() after drain = %+v, want nil", again)
	}
}
