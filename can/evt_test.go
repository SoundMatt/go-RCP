//fusa:test REQ-CANEP-011

package can_test

import (
	"errors"
	"testing"

	"github.com/SoundMatt/go-RCP/acf"
	"github.com/SoundMatt/go-RCP/avtp"
	"github.com/SoundMatt/go-RCP/can"
)

// ── REQ-CANEP-011: TC18 §13.5 Table 30 / §12.9.1 evt handling ──
//
// Table 30 puts CAN in the row it shares with ADC, PWM_IN, I²C, LIN, CAN,
// UART, ISELED and MDIO. The only evt[2:0] value with a defined
// payload-routing meaning in that row is 111b: "The byte_msg_payload is not
// presented to the interface but used to change the configuration of the
// endpoint (see 12.7.1)."
//
// This package reads that row as: 000b presents the payload at the interface
// as normal, 001b through 110b are reserved and rejected with
// UNSUPPORTED_CMD, and 111b is the configuration change. See acf/evt.go's
// ClassifyEVT for why the 000b entry departs from Table 30's literal text —
// §13.7.9's Figure 33 shows a conformant request with evt = 0000b, and a
// literal reading would leave this endpoint type unable to carry any traffic
// at all.

// evtReq builds a request for this endpoint carrying evt[2:0] = sel.
func evtReq(sel acf.EVTSelector, control acf.ControlFlags, body []byte) acf.Message {
	return acf.Message{
		Kind:      acf.KindShort,
		ByteBusID: avtp.ByteBusID(1),
		EVT:       uint8(sel),
		Control:   control,
		Body:      body,
	}
}

// TestEVTClass_IsTable30ConfigOnlyRow pins which row of Table 30 governs this
// endpoint type (REQ-CANEP-011).
func TestEVTClass_IsTable30ConfigOnlyRow(t *testing.T) {
	if can.EVTClass != acf.EVTClassConfigOnly {
		t.Errorf("can.EVTClass = %v, want %v", can.EVTClass, acf.EVTClassConfigOnly)
	}
}

// TestEVT_ReservedSelectorsRejected checks evt[2:0] = 001b through 110b are
// rejected with the reserved-selector error, which the transport layer
// answers with error code UNSUPPORTED_CMD, and that the payload never
// reaches the interface on the way (REQ-CANEP-011).
func TestEVT_ReservedSelectorsRejected(t *testing.T) {
	ep, root := newDeclaredEndpoint(t)
	if err := ep.Configure(root, can.Config{Enabled: true, NominalBitrateKbps: 500}); err != nil {
		t.Fatalf("Configure: %v", err)
	}
	tr := &recordingTransport{}
	ep.SetTransport(tr)

	for sel := acf.EVTSelector(1); sel <= 6; sel++ {
		if _, err := ep.HandleRequest(root, evtReq(sel, acf.FlagWrite, can.EncodeFrame(can.Frame{Format: can.FormatClassical, ID: 0x123, Data: []byte{0xDE, 0xAD}}))); !errors.Is(err, acf.ErrEVTReserved) {
			t.Errorf("evt[2:0]=%03b: err = %v, want acf.ErrEVTReserved", sel, err)
		}
	}
	if len(tr.got) != 0 {
		t.Errorf("reserved requests reached the interface (len(tr.got) = %v), want none", len(tr.got))
	}
}

// TestEVT_NonZeroWithoutPayloadRejected checks TC18 §12.9.1's general rule:
// "If evt[2:0] ≠ 0 and no byte_msg_payload is present, then an error
// response shall be sent with the error code = UNSUPPORTED_CMD" (REQ-CANEP-011).
func TestEVT_NonZeroWithoutPayloadRejected(t *testing.T) {
	ep, root := newDeclaredEndpoint(t)
	if err := ep.Configure(root, can.Config{Enabled: true, NominalBitrateKbps: 500}); err != nil {
		t.Fatalf("Configure: %v", err)
	}
	for sel := acf.EVTSelector(1); sel <= 7; sel++ {
		if _, err := ep.HandleRequest(root, evtReq(sel, acf.FlagWrite, nil)); !errors.Is(err, acf.ErrEVTMissingPayload) {
			t.Errorf("evt[2:0]=%03b with empty body: err = %v, want acf.ErrEVTMissingPayload", sel, err)
		}
	}
}

// TestEVT_ConfigChangeIsNotPresentedAtInterface checks evt[2:0] = 111b routes
// the payload into the endpoint's §12.7.1 EP_func block instead of onto the
// interface: the configuration takes effect, and the interface is never
// driven (REQ-CANEP-011).
func TestEVT_ConfigChangeIsNotPresentedAtInterface(t *testing.T) {
	ep, root := newDeclaredEndpoint(t)
	if err := ep.Configure(root, can.Config{Enabled: true, NominalBitrateKbps: 500}); err != nil {
		t.Fatalf("Configure: %v", err)
	}
	tr := &recordingTransport{}
	ep.SetTransport(tr)

	// Byte 0 of this endpoint type's EP_func block is its Enabled flag;
	// clearing it is a §12.7.1 configuration write whose effect is directly
	// observable through the endpoint's subsequent behaviour. §12.7.1
	// Figure 18: the payload leads with the relative EP_func start address.
	cfgReq := evtReq(acf.EVTSelector7, acf.FlagWrite, acf.EncodeConfigRequestBody(0, []byte{0x00}))
	if _, err := ep.HandleRequest(root, cfgReq); err != nil {
		t.Fatalf("HandleRequest(evt=111b): %v", err)
	}
	if len(tr.got) != 0 {
		t.Errorf("configuration payload reached the interface (len(tr.got) = %v), want none", len(tr.got))
	}

	// The configuration really was adopted: the endpoint now reports itself
	// unconfigured for ordinary traffic.
	if _, err := ep.HandleRequest(root, evtReq(acf.EVTSelector0, acf.FlagWrite, can.EncodeFrame(can.Frame{Format: can.FormatClassical, ID: 0x123, Data: []byte{0xDE, 0xAD}}))); !errors.Is(err, can.ErrNotConfigured) {
		t.Errorf("after configuration write: err = %v, want %v", err, can.ErrNotConfigured)
	}
}
