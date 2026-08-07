//fusa:test REQ-SPI-012

package spi_test

import (
	"errors"
	"testing"

	"github.com/SoundMatt/go-RCP/v9/acf"
	"github.com/SoundMatt/go-RCP/v9/avtp"
	"github.com/SoundMatt/go-RCP/v9/spi"
)

// ── REQ-SPI-012: TC18 §13.5 Table 30 / §12.9.1 evt handling ──
//
// Table 30's SPI row, in full:
//
//	000b-101b  "selects channel 0 … 5 / the interface settings are to be
//	           applied according to this selection / the CSN pin assigned
//	           to this selection is to be asserted"
//	110b       "reserved – request to be rejected with error code =
//	           UNSUPPORTED_CMD"
//	111b       "The byte_msg_payload is not presented to the interface but
//	           used to change the configuration of the endpoint (see 12.7.1)"

// TestEVTClass_IsTable30ChannelSelectRow pins which row of Table 30 governs
// this endpoint type (REQ-SPI-012).
func TestEVTClass_IsTable30ChannelSelectRow(t *testing.T) {
	if spi.EVTClass != acf.EVTClassChannelSelect {
		t.Errorf("spi.EVTClass = %v, want %v", spi.EVTClass, acf.EVTClassChannelSelect)
	}
}

// TestEVT_SelectsEveryChannel checks all six channels are reachable, and
// only through evt[2:0] — the request body carries no channel byte
// (REQ-SPI-012).
func TestEVT_SelectsEveryChannel(t *testing.T) {
	ep, root := newDeclaredEndpoint(t)
	transports := make([]*echoTransport, spi.MaxChannels)
	for ch := spi.Channel(0); ch < spi.MaxChannels; ch++ {
		if err := ep.SetChannelConfig(root, ch, spi.ChannelConfig{Enabled: true, ClockHz: 1000, Mode: spi.Mode0}); err != nil {
			t.Fatalf("SetChannelConfig(%d): %v", ch, err)
		}
		transports[ch] = &echoTransport{}
		if err := ep.SetTransport(ch, transports[ch]); err != nil {
			t.Fatalf("SetTransport(%d): %v", ch, err)
		}
	}

	for ch := spi.Channel(0); ch < spi.MaxChannels; ch++ {
		if _, err := ep.HandleRequest(root, transferReq(ch, []byte{0x01})); err != nil {
			t.Fatalf("HandleRequest(channel %d): %v", ch, err)
		}
		if transports[ch].calls != 1 {
			t.Errorf("evt[2:0]=%03b: channel %d transport calls = %d, want 1", ch, ch, transports[ch].calls)
		}
	}
}

// TestEVT_ReservedChannelSelectorRejected checks evt[2:0] = 110b — the one
// reserved value in Table 30's SPI row — is rejected, and never reaches a
// bus (REQ-SPI-012).
func TestEVT_ReservedChannelSelectorRejected(t *testing.T) {
	ep, root := newDeclaredEndpoint(t)
	tr := &echoTransport{}
	for ch := spi.Channel(0); ch < spi.MaxChannels; ch++ {
		if err := ep.SetChannelConfig(root, ch, spi.ChannelConfig{Enabled: true, ClockHz: 1000, Mode: spi.Mode0}); err != nil {
			t.Fatalf("SetChannelConfig(%d): %v", ch, err)
		}
		if err := ep.SetTransport(ch, tr); err != nil {
			t.Fatalf("SetTransport(%d): %v", ch, err)
		}
	}

	req := acf.Message{
		Kind: acf.KindShort, ByteBusID: avtp.ByteBusID(1),
		EVT: 0b110, Control: acf.FlagWrite, Body: []byte{0xAA},
	}
	if _, err := ep.HandleRequest(root, req); !errors.Is(err, acf.ErrEVTReserved) {
		t.Fatalf("evt[2:0]=110b: err = %v, want acf.ErrEVTReserved", err)
	}
	if tr.calls != 0 {
		t.Errorf("reserved request reached a bus (transport calls = %d), want 0", tr.calls)
	}
}

// TestEVT_NonZeroWithoutPayloadRejected checks TC18 §12.9.1's general rule
// reaches this endpoint type: "If evt[2:0] ≠ 0 and no byte_msg_payload is
// present, then an error response shall be sent with the error code =
// UNSUPPORTED_CMD". Note this makes a zero-length transfer expressible only
// on channel 0 — a consequence of the specification, not of this package
// (REQ-SPI-012).
func TestEVT_NonZeroWithoutPayloadRejected(t *testing.T) {
	ep, root := newDeclaredEndpoint(t)
	for ch := spi.Channel(0); ch < spi.MaxChannels; ch++ {
		if err := ep.SetChannelConfig(root, ch, spi.ChannelConfig{Enabled: true, ClockHz: 1000, Mode: spi.Mode0}); err != nil {
			t.Fatalf("SetChannelConfig(%d): %v", ch, err)
		}
	}

	// Channel 0 (evt[2:0] = 000b) is unaffected by the rule.
	if _, err := ep.HandleRequest(root, transferReq(spi.Channel0, nil)); err != nil {
		t.Errorf("evt[2:0]=000b with empty body: err = %v, want nil", err)
	}
	for sel := acf.EVTSelector(1); sel <= 7; sel++ {
		req := acf.Message{
			Kind: acf.KindShort, ByteBusID: avtp.ByteBusID(1),
			EVT: uint8(sel), Control: acf.FlagWrite,
		}
		if _, err := ep.HandleRequest(root, req); !errors.Is(err, acf.ErrEVTMissingPayload) {
			t.Errorf("evt[2:0]=%03b with empty body: err = %v, want acf.ErrEVTMissingPayload", sel, err)
		}
	}
}

// TestEVT_ConfigChangeIsNotPresentedOnTheBus checks evt[2:0] = 111b routes
// the payload into the endpoint's §12.7.1 EP_func block instead of onto a
// SPI bus (REQ-SPI-012).
func TestEVT_ConfigChangeIsNotPresentedOnTheBus(t *testing.T) {
	ep, root := newDeclaredEndpoint(t)
	if err := ep.SetChannelConfig(root, spi.Channel0, spi.ChannelConfig{Enabled: true, ClockHz: 1000, Mode: spi.Mode0}); err != nil {
		t.Fatalf("SetChannelConfig: %v", err)
	}
	tr := &echoTransport{}
	if err := ep.SetTransport(spi.Channel0, tr); err != nil {
		t.Fatalf("SetTransport: %v", err)
	}

	// Byte 0 of spi's EP_func block is channel 0's Enabled flag; clearing it
	// is a §12.7.1 configuration write (Figure 18: the payload leads with
	// the relative EP_func start address).
	cfgReq := acf.Message{
		Kind: acf.KindShort, ByteBusID: avtp.ByteBusID(1),
		EVT: uint8(acf.EVTSelector7), Control: acf.FlagWrite,
		Body: acf.EncodeConfigRequestBody(0, []byte{0x00}),
	}
	if _, err := ep.HandleRequest(root, cfgReq); err != nil {
		t.Fatalf("HandleRequest(evt=111b): %v", err)
	}
	if tr.calls != 0 {
		t.Errorf("configuration payload reached the bus (transport calls = %d), want 0", tr.calls)
	}

	if _, err := ep.HandleRequest(root, transferReq(spi.Channel0, []byte{0x01})); !errors.Is(err, spi.ErrChannelNotConfigured) {
		t.Errorf("after configuration write: err = %v, want spi.ErrChannelNotConfigured", err)
	}
}
