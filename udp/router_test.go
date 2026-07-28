//fusa:test REQ-UDP-007
//fusa:test REQ-UDP-008
//fusa:test REQ-UDP-009
//fusa:test REQ-UDP-010

package udp_test

import (
	"errors"
	"testing"

	"github.com/SoundMatt/go-RCP/acf"
	"github.com/SoundMatt/go-RCP/avtp"
	"github.com/SoundMatt/go-RCP/regmap"
	"github.com/SoundMatt/go-RCP/server"
	"github.com/SoundMatt/go-RCP/udp"
)

// TestRouter_Register_Duplicate verifies a second Register call for the
// same address is rejected (REQ-UDP-007).
func TestRouter_Register_Duplicate(t *testing.T) {
	router := udp.NewRouter(udp.NewEP0Handler(server.NewServer()), true)
	h := &stubHandler{}
	if err := router.Register(1, h); err != nil {
		t.Fatalf("first Register: %v", err)
	}
	if err := router.Register(1, h); !errors.Is(err, udp.ErrDuplicateEndpoint) {
		t.Errorf("error = %v, want ErrDuplicateEndpoint", err)
	}
}

// TestRouter_Register_ReservedEP0 verifies EP0 cannot be claimed by a
// caller-registered Handler (REQ-UDP-008).
func TestRouter_Register_ReservedEP0(t *testing.T) {
	router := udp.NewRouter(udp.NewEP0Handler(server.NewServer()), true)
	if err := router.Register(regmap.EP0, &stubHandler{}); !errors.Is(err, udp.ErrReservedAddress) {
		t.Errorf("error = %v, want ErrReservedAddress", err)
	}
}

// TestRouter_Route_DropsTimedWithoutTimeSync verifies a timestamped (TSCF)
// header is dropped outright (no reply) when the Router is configured
// without time-sync support, per avtp.Header.Disposition (REQ-UDP-009).
func TestRouter_Route_DropsTimedWithoutTimeSync(t *testing.T) {
	router := udp.NewRouter(udp.NewEP0Handler(server.NewServer()), false)
	h := &stubHandler{body: []byte{0x01}}
	if err := router.Register(1, h); err != nil {
		t.Fatalf("Register: %v", err)
	}

	hdr := avtp.Header{Timed: true, TimestampStatus: avtp.TimestampValid}
	req := acf.Message{ByteBusID: 1, Control: acf.FlagRead}
	_, shouldReply := router.Route(hdr, req)
	if shouldReply {
		t.Errorf("shouldReply = true, want false (dropped)")
	}
	if h.callCount != 0 {
		t.Errorf("handler was called %d times, want 0 (dropped before dispatch)", h.callCount)
	}
}

// TestRouter_Route_ExecutesTimedWithTimeSync verifies a valid-timestamp
// TSCF header is still dispatched (immediately — see doc.go's non-goal
// note) when the Router is configured with time-sync support
// (REQ-UDP-010).
func TestRouter_Route_ExecutesTimedWithTimeSync(t *testing.T) {
	router := udp.NewRouter(udp.NewEP0Handler(server.NewServer()), true)
	h := &stubHandler{body: []byte{0x01}}
	if err := router.Register(1, h); err != nil {
		t.Fatalf("Register: %v", err)
	}

	hdr := avtp.Header{Timed: true, TimestampStatus: avtp.TimestampValid}
	req := acf.Message{ByteBusID: 1, Control: acf.FlagRead}
	_, shouldReply := router.Route(hdr, req)
	if !shouldReply {
		t.Errorf("shouldReply = false, want true (executed)")
	}
	if h.callCount != 1 {
		t.Errorf("handler was called %d times, want 1", h.callCount)
	}
}
