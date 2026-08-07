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
	"context"
	"errors"
	"testing"

	"github.com/SoundMatt/go-RCP/v9/avtp"
	"github.com/SoundMatt/go-RCP/v9/lin"
	"github.com/SoundMatt/go-RCP/v9/linbr"
	"github.com/SoundMatt/go-RCP/v9/server"
	"github.com/SoundMatt/go-RCP/v9/udp"
)

func clientStream() avtp.StreamID {
	return avtp.NewStreamID([6]byte{0x02, 0x11, 0x22, 0x33, 0x44, 0x55}, 1)
}

func serverStream() avtp.StreamID {
	return avtp.NewStreamID([6]byte{0x02, 0xAA, 0xBB, 0xCC, 0xDD, 0xEE}, 1)
}

const testAddr = avtp.ByteBusID(1)

// TestEncodeFrame_ClassicChecksum EncodeFrame produces PID+data+classic
// checksum wire bytes, decodable back to the same Frame (REQ-LIN-001).
func TestEncodeFrame_ClassicChecksum(t *testing.T) {
	f := linbr.Frame{ID: 0x10, Data: []byte{1, 2, 3, 4}, Checksum: linbr.ChecksumClassic}
	buf, err := linbr.EncodeFrame(f)
	if err != nil {
		t.Fatalf("EncodeFrame: %v", err)
	}
	if len(buf) != 6 { // pid(1) + data(4) + checksum(1)
		t.Fatalf("len(buf) = %d, want 6", len(buf))
	}
	got, err := linbr.DecodeFrame(buf, linbr.ChecksumClassic)
	if err != nil {
		t.Fatalf("DecodeFrame: %v", err)
	}
	if got.ID != f.ID || string(got.Data) != string(f.Data) {
		t.Errorf("DecodeFrame = %+v, want ID/Data matching %+v", got, f)
	}
}

// TestDecodeFrame_ChecksumMismatch a corrupted checksum byte is detected
// (REQ-LIN-002).
func TestDecodeFrame_ChecksumMismatch(t *testing.T) {
	f := linbr.Frame{ID: 0x05, Data: []byte{0x01, 0x02}, Checksum: linbr.ChecksumClassic}
	buf, err := linbr.EncodeFrame(f)
	if err != nil {
		t.Fatalf("EncodeFrame: %v", err)
	}
	buf[len(buf)-1] ^= 0xFF
	if _, err := linbr.DecodeFrame(buf, linbr.ChecksumClassic); !errors.Is(err, linbr.ErrChecksumMismatch) {
		t.Errorf("err = %v, want ErrChecksumMismatch", err)
	}
}

// TestChecksum_EnhancedDiffersFromClassic the enhanced checksum (which
// folds in the protected ID) is not interchangeable with the classic one
// for the same frame (REQ-LIN-003).
func TestChecksum_EnhancedDiffersFromClassic(t *testing.T) {
	f := linbr.Frame{ID: 0x21, Data: []byte{0xAA, 0xBB}, Checksum: linbr.ChecksumEnhanced}
	buf, err := linbr.EncodeFrame(f)
	if err != nil {
		t.Fatalf("EncodeFrame: %v", err)
	}
	if _, mismatchErr := linbr.DecodeFrame(buf, linbr.ChecksumClassic); !errors.Is(mismatchErr, linbr.ErrChecksumMismatch) {
		t.Error("decoding an enhanced-checksum frame as classic unexpectedly validated")
	}
	got, err := linbr.DecodeFrame(buf, linbr.ChecksumEnhanced)
	if err != nil {
		t.Fatalf("DecodeFrame(enhanced): %v", err)
	}
	if got.ID != f.ID {
		t.Errorf("ID = %#x, want %#x", got.ID, f.ID)
	}
}

// TestEncodeFrame_RejectsOversizedData a Data payload over 8 bytes is
// rejected before any wire bytes are produced (REQ-LIN-004).
func TestEncodeFrame_RejectsOversizedData(t *testing.T) {
	f := linbr.Frame{ID: 0x01, Data: make([]byte, 9)}
	if _, err := linbr.EncodeFrame(f); !errors.Is(err, linbr.ErrDataTooLong) {
		t.Errorf("err = %v, want ErrDataTooLong", err)
	}
}

// TestDecodeFrame_TooShort a buffer under 2 bytes cannot hold a PID and a
// checksum (REQ-LIN-005).
func TestDecodeFrame_TooShort(t *testing.T) {
	if _, err := linbr.DecodeFrame([]byte{0x01}, linbr.ChecksumClassic); !errors.Is(err, linbr.ErrFrameTooShort) {
		t.Errorf("err = %v, want ErrFrameTooShort", err)
	}
}

// TestScheduleTable_RoundRobin Next cycles through every entry in order and
// wraps back to the first (REQ-LIN-006).
func TestScheduleTable_RoundRobin(t *testing.T) {
	table := linbr.NewScheduleTable([]linbr.ScheduleEntry{
		{ID: 0x01, DelaySlots: 10},
		{ID: 0x02, DelaySlots: 20},
		{ID: 0x03, DelaySlots: 30},
	})
	var got []uint8
	for i := 0; i < 5; i++ {
		e, ok := table.Next()
		if !ok {
			t.Fatalf("Next() ok = false at i=%d", i)
		}
		got = append(got, e.ID)
	}
	want := []uint8{0x01, 0x02, 0x03, 0x01, 0x02}
	if len(got) != len(want) {
		t.Fatalf("got %d entries, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("slot %d = %#x, want %#x", i, got[i], want[i])
		}
	}
}

// TestScheduleTable_EmptyReportsNotOK an empty table's Next never reports ok
// (REQ-LIN-006, boundary case).
func TestScheduleTable_EmptyReportsNotOK(t *testing.T) {
	table := linbr.NewScheduleTable(nil)
	if _, ok := table.Next(); ok {
		t.Error("Next() on an empty table reported ok = true")
	}
}

// newHarness starts a udp.Server backed by a real lin.Endpoint at testAddr,
// configured to loop every transfer back unchanged (the native endpoint's
// default Transport, see lin.Endpoint.SetTransport), and dials a
// *udp.Controller against it.
func newHarness(t *testing.T) (*linbr.Controller, *udp.Controller) {
	t.Helper()
	srvSide := server.NewServer()
	if err := srvSide.ClaimRoot(serverStream()); err != nil {
		t.Fatalf("ClaimRoot: %v", err)
	}
	if err := srvSide.AddEndpoint(serverStream(), testAddr, lin.EndpointType); err != nil {
		t.Fatalf("AddEndpoint: %v", err)
	}
	srvSide.Grant(clientStream(), testAddr)
	ep := lin.NewEndpoint(srvSide, testAddr)
	if err := ep.Configure(serverStream(), lin.Config{Enabled: true, BaudRate: 19200}); err != nil {
		t.Fatalf("Configure: %v", err)
	}

	router := udp.NewRouter(udp.NewEP0Handler(srvSide), false)
	if err := router.Register(testAddr, ep); err != nil {
		t.Fatalf("Register: %v", err)
	}
	srv, err := udp.NewServer(serverStream(), "127.0.0.1:0", router)
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}
	t.Cleanup(func() { _ = srv.Close() })

	inner, err := udp.NewController(clientStream(), srv.Addr())
	if err != nil {
		t.Fatalf("NewController: %v", err)
	}
	t.Cleanup(func() { _ = inner.Close() })

	return linbr.NewController(inner, testAddr), inner
}

// TestController_Transfer_RoundTrip Transfer encodes a Frame, sends it
// through a real lin.Endpoint (raw byte pass-through, loopback by default),
// and decodes the echoed bytes back into the same Frame (REQ-LIN-007).
func TestController_Transfer_RoundTrip(t *testing.T) {
	c, _ := newHarness(t)
	f := linbr.Frame{ID: 0x1A, Data: []byte{0xAA, 0xBB, 0xCC}, Checksum: linbr.ChecksumClassic}
	got, err := c.Transfer(context.Background(), f)
	if err != nil {
		t.Fatalf("Transfer: %v", err)
	}
	if got.ID != f.ID || string(got.Data) != string(f.Data) {
		t.Errorf("Transfer() = %+v, want echo of %+v", got, f)
	}
}

// TestController_StreamIDAndClose StreamID/Close delegate to the wrapped
// Controller, and Close is idempotent (REQ-LIN-008).
func TestController_StreamIDAndClose(t *testing.T) {
	c, inner := newHarness(t)
	if got, want := c.StreamID(), inner.StreamID(); got != want {
		t.Errorf("StreamID() = %v, want %v", got, want)
	}
	if err := c.Close(); err != nil {
		t.Errorf("first Close: %v", err)
	}
	if err := c.Close(); err != nil {
		t.Errorf("second Close: %v", err)
	}
}
