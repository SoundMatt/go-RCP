//fusa:test REQ-MDIO-005
//fusa:test REQ-MDIO-006
//fusa:test REQ-MDIO-007

package mdio_test

import (
	"errors"
	"testing"

	"github.com/SoundMatt/go-RCP/acf"
	"github.com/SoundMatt/go-RCP/avtp"
	"github.com/SoundMatt/go-RCP/mdio"
	"github.com/SoundMatt/go-RCP/regmap"
)

func readReq(r mdio.Request) acf.Message {
	return acf.Message{
		Kind:      acf.KindShort,
		ByteBusID: avtp.ByteBusID(1),
		Control:   acf.FlagRead,
		Body:      mdio.EncodeReadRequest(r),
	}
}

func writeReq(r mdio.Request, data uint32) acf.Message {
	return acf.Message{
		Kind:      acf.KindShort,
		ByteBusID: avtp.ByteBusID(1),
		Control:   acf.FlagWrite,
		Body:      mdio.EncodeWriteRequest(r, data),
	}
}

// recordingTransport is a Transport test double backed by its own register
// map, so tests can tell the configured Transport actually ran rather than
// the default in-memory store.
type recordingTransport struct {
	regs  map[uint8]uint32
	reads int
}

func newRecordingTransport() *recordingTransport {
	return &recordingTransport{regs: make(map[uint8]uint32)}
}

func (r *recordingTransport) ReadRegister(req mdio.Request) (uint32, error) {
	r.reads++
	return r.regs[req.DevAddr] + 1, nil // offset by 1 so tests can distinguish from the default store
}

func (r *recordingTransport) WriteRegister(req mdio.Request, data uint32) error {
	r.regs[req.DevAddr] = data
	return nil
}

// TestHandleRequest_RequiresReadOrWriteWrongEndpointOrAccess checks a
// request with neither Read nor Write set, one addressed to the wrong
// endpoint, and one from a stream with no access grant are all rejected
// (REQ-MDIO-005).
func TestHandleRequest_RequiresReadOrWriteWrongEndpointOrAccess(t *testing.T) {
	ep, root := newDeclaredEndpoint(t)
	if err := ep.Configure(root, mdio.Config{Enabled: true}); err != nil {
		t.Fatalf("Configure: %v", err)
	}

	neither := acf.Message{Kind: acf.KindShort, ByteBusID: avtp.ByteBusID(1)}
	if _, err := ep.HandleRequest(root, neither); !errors.Is(err, mdio.ErrRequestMustReadOrWrite) {
		t.Errorf("HandleRequest(neither flag) err = %v, want ErrRequestMustReadOrWrite", err)
	}

	wrongAddr := readReq(mdio.Request{})
	wrongAddr.ByteBusID = 2
	if _, err := ep.HandleRequest(root, wrongAddr); !errors.Is(err, mdio.ErrWrongEndpoint) {
		t.Errorf("HandleRequest(wrong addr) err = %v, want ErrWrongEndpoint", err)
	}

	stranger := avtp.NewStreamID([6]byte{0x06, 0xAA, 0xBB, 0xCC, 0xDD, 0xEE}, 9)
	if _, err := ep.HandleRequest(stranger, readReq(mdio.Request{})); !errors.Is(err, regmap.ErrAccessDenied) {
		t.Errorf("HandleRequest(no grant) err = %v, want regmap.ErrAccessDenied", err)
	}
}

// TestHandleRequest_RejectsDisabledEndpointAndInvalidRequest checks a
// request against a disabled endpoint, and an addressing-invalid Request,
// are both rejected (REQ-MDIO-006).
func TestHandleRequest_RejectsDisabledEndpointAndInvalidRequest(t *testing.T) {
	ep, root := newDeclaredEndpoint(t)
	if _, err := ep.HandleRequest(root, readReq(mdio.Request{})); !errors.Is(err, mdio.ErrNotConfigured) {
		t.Errorf("HandleRequest(disabled) err = %v, want ErrNotConfigured", err)
	}

	if err := ep.Configure(root, mdio.Config{Enabled: true}); err != nil {
		t.Fatalf("Configure: %v", err)
	}
	bad := mdio.Request{Mode: mdio.ModeMMDSingleWord, DevAddr: 0x20}
	if _, err := ep.HandleRequest(root, readReq(bad)); !errors.Is(err, mdio.ErrDevAddrOutOfRange) {
		t.Errorf("HandleRequest(invalid request) err = %v, want ErrDevAddrOutOfRange", err)
	}
}

// TestHandleRequest_DefaultStoreAndTransport checks a write/read round trip
// through the default in-memory register store, and through a configured
// Transport instead, and that each access queues a trigger (REQ-MDIO-007).
func TestHandleRequest_DefaultStoreAndTransport(t *testing.T) {
	ep, root := newDeclaredEndpoint(t)
	if err := ep.Configure(root, mdio.Config{Enabled: true}); err != nil {
		t.Fatalf("Configure: %v", err)
	}
	r := mdio.Request{Mode: mdio.ModeMMDSingleWord, DevAddr: 2}

	// Default store: reads as zero until written.
	resp, err := ep.HandleRequest(root, readReq(r))
	if err != nil {
		t.Fatalf("HandleRequest(read, default): %v", err)
	}
	if got, _ := mdio.DecodeResponse(r, resp.Body); got != 0 {
		t.Errorf("HandleRequest(read, unset) = %#x, want 0", got)
	}

	if _, werr := ep.HandleRequest(root, writeReq(r, 0x4242)); werr != nil {
		t.Fatalf("HandleRequest(write, default): %v", werr)
	}
	resp, err = ep.HandleRequest(root, readReq(r))
	if err != nil {
		t.Fatalf("HandleRequest(read after write, default): %v", err)
	}
	if got, _ := mdio.DecodeResponse(r, resp.Body); got != 0x4242 {
		t.Errorf("HandleRequest(read after write) = %#x, want 0x4242", got)
	}

	triggers := ep.DrainTriggers()
	if len(triggers) != 3 {
		t.Fatalf("DrainTriggers() len = %d, want 3", len(triggers))
	}
	if triggers[0].Write || triggers[1].Write != true || triggers[2].Write {
		t.Errorf("DrainTriggers() Write flags = %v, want [false true false]", triggers)
	}

	// Configured Transport.
	tr := newRecordingTransport()
	ep.SetTransport(tr)
	if _, werr := ep.HandleRequest(root, writeReq(r, 0x0100)); werr != nil {
		t.Fatalf("HandleRequest(write, transport): %v", werr)
	}
	resp, err = ep.HandleRequest(root, readReq(r))
	if err != nil {
		t.Fatalf("HandleRequest(read, transport): %v", err)
	}
	if got, _ := mdio.DecodeResponse(r, resp.Body); got != 0x0101 { // transport reads back written+1
		t.Errorf("HandleRequest(read, transport) = %#x, want 0x0101", got)
	}
	if tr.reads != 1 {
		t.Errorf("Transport.ReadRegister calls = %d, want 1", tr.reads)
	}
}

// TestHandleRequest_MMSWideWidth checks a write/read round trip against an
// MMS0 (32-bit) register carries the full 32-bit value through
// Endpoint.HandleRequest — the old fixed-16-bit-everywhere encoding could
// not represent this at all (REQ-MDIO-007).
func TestHandleRequest_MMSWideWidth(t *testing.T) {
	ep, root := newDeclaredEndpoint(t)
	if err := ep.Configure(root, mdio.Config{Enabled: true}); err != nil {
		t.Fatalf("Configure: %v", err)
	}
	r := mdio.Request{Mode: mdio.ModeMMSSingleWord, DevAddr: 0}

	if _, werr := ep.HandleRequest(root, writeReq(r, 0xFEEDFACE)); werr != nil {
		t.Fatalf("HandleRequest(write, MMS0): %v", werr)
	}
	resp, err := ep.HandleRequest(root, readReq(r))
	if err != nil {
		t.Fatalf("HandleRequest(read, MMS0): %v", err)
	}
	if got, err := mdio.DecodeResponse(r, resp.Body); err != nil || got != 0xFEEDFACE {
		t.Errorf("HandleRequest(read, MMS0) = %#x, %v, want 0xFEEDFACE, nil", got, err)
	}
}
