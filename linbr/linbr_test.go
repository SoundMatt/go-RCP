//fusa:test REQ-LIN-001
//fusa:test REQ-LIN-002
//fusa:test REQ-LIN-003
//fusa:test REQ-LIN-004
//fusa:test REQ-LIN-005
//fusa:test REQ-LIN-006
//fusa:test REQ-LIN-007
//fusa:test REQ-LIN-008

package linbr_test

import (
	"errors"
	"testing"
	"time"

	rcp "github.com/SoundMatt/go-RCP"
	"github.com/SoundMatt/go-RCP/linbr"
	"github.com/SoundMatt/go-RCP/mock"
)

// REQ-LIN-001: EncodeFrame produces a valid 11-byte LIN frame.
func TestEncodeFrame_Size(t *testing.T) {
	f := linbr.Frame{ID: 0x10, DataLen: 4, Data: [8]byte{1, 2, 3, 4}}
	buf := linbr.EncodeFrame(f)
	if len(buf) != 11 {
		t.Fatalf("EncodeFrame len = %d, want 11", len(buf))
	}
}

// REQ-LIN-002: DecodeFrame parses the header back correctly.
func TestEncodeDecodeRoundTrip(t *testing.T) {
	want := linbr.Frame{ID: 0x1A, DataLen: 3, Data: [8]byte{0xAA, 0xBB, 0xCC}}
	buf := linbr.EncodeFrame(want)
	got, err := linbr.DecodeFrame(buf)
	if err != nil {
		t.Fatalf("DecodeFrame: %v", err)
	}
	if got.ID != want.ID {
		t.Errorf("ID = %d, want %d", got.ID, want.ID)
	}
	if got.DataLen != want.DataLen {
		t.Errorf("DataLen = %d, want %d", got.DataLen, want.DataLen)
	}
	for i := uint8(0); i < want.DataLen; i++ {
		if got.Data[i] != want.Data[i] {
			t.Errorf("Data[%d] = %d, want %d", i, got.Data[i], want.Data[i])
		}
	}
}

// REQ-LIN-003: DecodeFrame returns ErrChecksumMismatch for corrupted frames.
func TestDecodeFrame_ChecksumMismatch(t *testing.T) {
	f := linbr.Frame{ID: 0x05, DataLen: 2, Data: [8]byte{0x01, 0x02}}
	buf := linbr.EncodeFrame(f)
	buf[10] ^= 0xFF // corrupt checksum
	_, err := linbr.DecodeFrame(buf)
	if !errors.Is(err, linbr.ErrChecksumMismatch) {
		t.Errorf("want ErrChecksumMismatch, got %v", err)
	}
}

// REQ-LIN-004: Bus.Send delivers frames to all registered Slaves with matching ID.
func TestBus_Send_Routes(t *testing.T) {
	bus := linbr.NewBus()
	s1 := linbr.NewSlave(bus, 0x20)
	s2 := linbr.NewSlave(bus, 0x20)
	defer s1.Close()
	defer s2.Close()

	f := linbr.Frame{ID: 0x20, DataLen: 1, Data: [8]byte{0x42}}
	bus.Send(f)

	for _, sl := range []*linbr.Slave{s1, s2} {
		select {
		case got := <-sl.Frames():
			if got.ID != 0x20 {
				t.Errorf("got ID %d, want 0x20", got.ID)
			}
		case <-time.After(time.Second):
			t.Error("timeout waiting for frame on slave")
		}
	}
}

// REQ-LIN-005: Bus.Send does not deliver frames with non-matching IDs.
func TestBus_Send_IDFilter(t *testing.T) {
	bus := linbr.NewBus()
	s := linbr.NewSlave(bus, 0x10)
	defer s.Close()

	bus.Send(linbr.Frame{ID: 0x20, DataLen: 1}) // different ID
	select {
	case <-s.Frames():
		t.Error("received frame with wrong ID")
	case <-time.After(50 * time.Millisecond):
		// expected: no frame
	}
}

// REQ-LIN-006: Slave receives frames dispatched by the Bus.
func TestSlave_ReceivesFrames(t *testing.T) {
	bus := linbr.NewBus()
	s := linbr.NewSlave(bus, 0x3F)
	defer s.Close()

	f := linbr.Frame{ID: 0x3F, DataLen: 2, Data: [8]byte{0xDE, 0xAD}}
	bus.Send(f)

	select {
	case got := <-s.Frames():
		if got.Data[0] != 0xDE || got.Data[1] != 0xAD {
			t.Errorf("wrong data: %v", got.Data[:2])
		}
	case <-time.After(time.Second):
		t.Error("timeout waiting for frame")
	}
}

// REQ-LIN-007: Bridge dispatches frames to rcp.Controller.Send.
func TestBridge_DispatchesFrames(t *testing.T) {
	bus := linbr.NewBus()
	dispatched := make(chan rcp.CommandType, 1)
	inner := mock.NewController(rcp.ZoneFrontLeft, func(cmd *rcp.Command) *rcp.Response {
		dispatched <- cmd.Type
		return &rcp.Response{CommandID: cmd.ID, Zone: cmd.Zone, Status: rcp.StatusOK}
	})
	defer inner.Close()

	slave := linbr.NewSlave(bus, 0x01)
	defer slave.Close()

	bridge := linbr.NewBridge(inner, slave)
	defer bridge.Close()

	// Data: Zone=FrontLeft, Type=CmdSet
	f := linbr.Frame{
		ID:      0x01,
		DataLen: 2,
		Data:    [8]byte{byte(rcp.ZoneFrontLeft), byte(rcp.CmdSet)},
	}
	bus.Send(f)

	select {
	case got := <-dispatched:
		if got != rcp.CmdSet {
			t.Errorf("dispatched type = %v, want CmdSet", got)
		}
	case <-time.After(3 * time.Second):
		t.Error("timeout waiting for dispatch")
	}
}

// REQ-LIN-008: Bridge.Close is idempotent.
func TestBridge_CloseIdempotent(t *testing.T) {
	bus := linbr.NewBus()
	inner := mock.NewController(rcp.ZoneFrontLeft, nil)
	defer inner.Close()

	slave := linbr.NewSlave(bus, 0x02)
	defer slave.Close()

	bridge := linbr.NewBridge(inner, slave)
	bridge.Close()
	bridge.Close() // must not panic
}
