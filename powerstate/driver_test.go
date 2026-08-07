//fusa:test REQ-PWR-001
//fusa:test REQ-PWR-002
//fusa:test REQ-PWR-003
//fusa:test REQ-PWR-004
//fusa:test REQ-PWR-005

package powerstate_test

import (
	"errors"
	"sync"
	"testing"

	"github.com/SoundMatt/go-RCP/v9/acf"
	"github.com/SoundMatt/go-RCP/v9/avtp"
	"github.com/SoundMatt/go-RCP/v9/powerstate"
	"github.com/SoundMatt/go-RCP/v9/server"
	"github.com/SoundMatt/go-RCP/v9/wakeup"
)

func testStream() avtp.StreamID {
	return avtp.NewStreamID([6]byte{0x03, 0, 0, 0, 0, 1}, 1)
}

// writePowerState sends a plain write request commanding target against ep,
// failing the test on any error.
func writePowerState(t *testing.T, ep *wakeup.Endpoint, root avtp.StreamID, addr avtp.ByteBusID, target wakeup.PowerState, txn avtp.TransactionNum) {
	t.Helper()
	_, err := ep.HandleRequest(root, acf.Message{
		Kind:           acf.KindShort,
		ByteBusID:      addr,
		TransactionNum: txn,
		Control:        acf.FlagWrite,
		Body:           wakeup.EncodePowerStateRequest(target),
	})
	if err != nil {
		t.Fatalf("HandleRequest(write %v): %v", target, err)
	}
}

// newWakingEndpoint returns a configured wakeup.Endpoint already driven
// through a Sleep->Normal wake, so its trigger queue holds one
// TriggerPowerStateChanged plus repeatCount TriggerWakeHandshake events.
func newWakingEndpoint(t *testing.T, repeatCount uint16) (*wakeup.Endpoint, avtp.StreamID) {
	t.Helper()
	root := testStream()
	s := server.NewServer()
	if err := s.ClaimRoot(root); err != nil {
		t.Fatalf("ClaimRoot: %v", err)
	}
	addr := avtp.ByteBusID(1)
	if err := s.AddEndpoint(root, addr, wakeup.EndpointType); err != nil {
		t.Fatalf("AddEndpoint: %v", err)
	}
	ep := wakeup.NewEndpoint(s, addr)
	cfg := wakeup.Config{Enabled: true, WakeHandshakeIntervalMillis: 10, WakeHandshakeRepeatCount: repeatCount}
	if err := ep.Configure(root, cfg); err != nil {
		t.Fatalf("Configure: %v", err)
	}

	writePowerState(t, ep, root, addr, wakeup.PowerSleep, 1)
	writePowerState(t, ep, root, addr, wakeup.PowerNormal, 2)
	return ep, root
}

func testStreamB() avtp.StreamID {
	return avtp.NewStreamID([6]byte{0x03, 0, 0, 0, 0, 2}, 1)
}

// TestDriver_PumpRelaysPowerStateEvents checks Pump relays every drained
// TriggerPowerStateChanged event, oldest first (REQ-PWR-001).
func TestDriver_PumpRelaysPowerStateEvents(t *testing.T) {
	ep, _ := newWakingEndpoint(t, 2)
	target := testStreamB()

	var sent []wakeup.WakeHandshake
	d := powerstate.NewDriver(ep, target, func(to avtp.StreamID, h wakeup.WakeHandshake) error {
		if to != target {
			t.Errorf("Transmitter target = %v, want %v", to, target)
		}
		sent = append(sent, h)
		return nil
	})

	events, err := d.Pump()
	if err != nil {
		t.Fatalf("Pump: %v", err)
	}
	// newWakingEndpoint drives two transitions (Normal->Sleep, then
	// Sleep->Normal), each queuing its own TriggerPowerStateChanged — Pump
	// must relay both, oldest first.
	want := []wakeup.PowerState{wakeup.PowerSleep, wakeup.PowerNormal}
	if len(events) != len(want) {
		t.Fatalf("Pump() events = %+v, want %d events", events, len(want))
	}
	for i, w := range want {
		if events[i].State != w {
			t.Errorf("Pump() events[%d].State = %v, want %v", i, events[i].State, w)
		}
	}
}

// TestDriver_PumpPacesWakeHandshakeOnePerCall checks Pump queues every
// drained TriggerWakeHandshake repeat but transmits at most one per call
// (REQ-PWR-002).
func TestDriver_PumpPacesWakeHandshakeOnePerCall(t *testing.T) {
	ep, _ := newWakingEndpoint(t, 3)
	target := testStreamB()

	var sent []wakeup.WakeHandshake
	d := powerstate.NewDriver(ep, target, func(_ avtp.StreamID, h wakeup.WakeHandshake) error {
		sent = append(sent, h)
		return nil
	})

	if _, err := d.Pump(); err != nil {
		t.Fatalf("Pump(1): %v", err)
	}
	if len(sent) != 1 {
		t.Fatalf("after first Pump, sent = %v, want exactly 1 handshake", sent)
	}
	if d.Pending() != 2 {
		t.Fatalf("Pending() = %d, want 2 (3 queued, 1 sent)", d.Pending())
	}

	if _, err := d.Pump(); err != nil {
		t.Fatalf("Pump(2): %v", err)
	}
	if _, err := d.Pump(); err != nil {
		t.Fatalf("Pump(3): %v", err)
	}
	if len(sent) != 3 {
		t.Fatalf("after three Pump calls, sent = %v, want exactly 3 handshakes", sent)
	}
	for i, h := range sent {
		if h.Sequence != uint16(i) {
			t.Errorf("sent[%d].Sequence = %d, want %d", i, h.Sequence, i)
		}
	}

	// A fourth Pump call has nothing left to send.
	if _, err := d.Pump(); err != nil {
		t.Fatalf("Pump(4): %v", err)
	}
	if len(sent) != 3 {
		t.Errorf("after a fourth Pump call, sent = %v, want still exactly 3", sent)
	}
}

// TestDriver_PumpRetriesAfterTransmitterError checks a Transmitter error
// leaves the handshake at the front of the pending queue for the next Pump
// call to retry, rather than losing it (REQ-PWR-003).
func TestDriver_PumpRetriesAfterTransmitterError(t *testing.T) {
	ep, _ := newWakingEndpoint(t, 1)
	target := testStreamB()

	wantErr := errors.New("transport down")
	fail := true
	var sent []wakeup.WakeHandshake
	d := powerstate.NewDriver(ep, target, func(_ avtp.StreamID, h wakeup.WakeHandshake) error {
		if fail {
			return wantErr
		}
		sent = append(sent, h)
		return nil
	})

	if _, err := d.Pump(); !errors.Is(err, wantErr) {
		t.Fatalf("Pump() err = %v, want %v", err, wantErr)
	}
	if d.Pending() != 1 {
		t.Fatalf("Pending() after failed send = %d, want 1 (not lost)", d.Pending())
	}

	fail = false
	if _, err := d.Pump(); err != nil {
		t.Fatalf("Pump() after transport recovered: %v", err)
	}
	if len(sent) != 1 {
		t.Fatalf("sent = %v, want exactly 1 handshake once the retry succeeded", sent)
	}
	if d.Pending() != 0 {
		t.Errorf("Pending() after successful retry = %d, want 0", d.Pending())
	}
}

// TestDriver_AcknowledgeDiscardsAllPendingRepeats checks Acknowledge
// discards both repeats still queued on the Endpoint and repeats this
// Driver had already pulled into its own pending buffer (REQ-PWR-004).
func TestDriver_AcknowledgeDiscardsAllPendingRepeats(t *testing.T) {
	ep, _ := newWakingEndpoint(t, 5)
	target := testStreamB()

	var sendCount int
	d := powerstate.NewDriver(ep, target, func(_ avtp.StreamID, _ wakeup.WakeHandshake) error {
		sendCount++
		return nil
	})

	// Pull two repeats into the Driver's own buffer, only one of which is
	// transmitted.
	if _, err := d.Pump(); err != nil {
		t.Fatalf("Pump: %v", err)
	}
	if sendCount != 1 {
		t.Fatalf("sendCount = %d, want 1", sendCount)
	}

	d.Acknowledge()

	if d.Pending() != 0 {
		t.Errorf("Pending() after Acknowledge = %d, want 0", d.Pending())
	}
	if got := ep.DrainTriggers(); len(got) != 0 {
		t.Errorf("Endpoint still has %d queued triggers after Acknowledge, want 0", len(got))
	}

	// A further Pump call has nothing left to send.
	if _, err := d.Pump(); err != nil {
		t.Fatalf("Pump after Acknowledge: %v", err)
	}
	if sendCount != 1 {
		t.Errorf("sendCount after Acknowledge = %d, want still 1", sendCount)
	}
}

// TestDriver_ConcurrentUse checks Driver's exported methods are safe to call
// concurrently (REQ-PWR-005).
func TestDriver_ConcurrentUse(t *testing.T) {
	ep, _ := newWakingEndpoint(t, 20)
	target := testStreamB()

	var mu sync.Mutex
	var sendCount int
	d := powerstate.NewDriver(ep, target, func(_ avtp.StreamID, _ wakeup.WakeHandshake) error {
		mu.Lock()
		sendCount++
		mu.Unlock()
		return nil
	})

	var wg sync.WaitGroup
	for i := 0; i < 25; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _ = d.Pump()
			_ = d.Pending()
		}()
	}
	wg.Wait()
	d.Acknowledge()
}
