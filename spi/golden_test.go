//fusa:test REQ-SPI-011

package spi_test

import (
	"bytes"
	"testing"

	"github.com/SoundMatt/go-RCP/acf"
	"github.com/SoundMatt/go-RCP/avtp"
	"github.com/SoundMatt/go-RCP/spi"
)

// ── REQ-SPI-011 (golden-vector half): frozen SPI Config/request/response
// byte layouts ──
//
// These fixtures pin the exact wire bytes this package's encoders produce
// today, so Phase 15's conditional-request work and Phase 16's remaining
// endpoint types can regression-test against a frozen SPI encoding rather
// than re-deriving it from current behaviour, the same posture
// server/golden_test.go and gpio/golden_test.go established.

// goldenConfig has channel 0 enabled (1 MHz, Mode0, 5us delay) and every
// other channel left at its disabled zero value.
var goldenConfig = []byte{
	// Channel0: Enabled=1, ClockHz=0x000F4240 (1,000,000), Mode=0, delay=5.
	0x01, 0x00, 0x0F, 0x42, 0x40, 0x00, 0x00, 0x05,
	// Channel1-5: all zero (disabled).
	0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
	0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
	0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
	0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
	0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
}

func TestGolden_Config(t *testing.T) {
	var cfg spi.Config
	cfg.Channels[0] = spi.ChannelConfig{Enabled: true, ClockHz: 1_000_000, Mode: spi.Mode0, InterTransferDelayMicros: 5}

	got := spi.EncodeConfig(cfg)
	if !bytes.Equal(got, goldenConfig) {
		t.Fatalf("EncodeConfig changed:\n got  % X\n want % X", got, goldenConfig)
	}
	decoded, err := spi.DecodeConfig(goldenConfig)
	if err != nil {
		t.Fatalf("DecodeConfig(golden): %v", err)
	}
	if decoded != cfg {
		t.Errorf("DecodeConfig(golden) = %+v, want %+v", decoded, cfg)
	}
}

// goldenTransferRequest is a SPI transfer request's whole byte_msg_payload:
// tx = {0xDE, 0xAD, 0xBE, 0xEF}, with no leading channel byte. The channel
// travels in evt[2:0] instead (TC18 §13.5 Table 30's SPI row) — see
// goldenTransferEVT.
var goldenTransferRequest = []byte{0xDE, 0xAD, 0xBE, 0xEF}

// goldenTransferEVT is evt[2:0] = 010b, selecting chip-select channel 2.
const goldenTransferEVT = uint8(spi.Channel2)

func TestGolden_TransferRequest(t *testing.T) {
	got := spi.EncodeTransferRequest([]byte{0xDE, 0xAD, 0xBE, 0xEF})
	if !bytes.Equal(got, goldenTransferRequest) {
		t.Fatalf("EncodeTransferRequest changed:\n got  % X\n want % X", got, goldenTransferRequest)
	}
	if tx := spi.DecodeTransferRequest(goldenTransferRequest); !bytes.Equal(tx, []byte{0xDE, 0xAD, 0xBE, 0xEF}) {
		t.Errorf("DecodeTransferRequest(golden) = % X, want DE AD BE EF", tx)
	}
}

func TestGolden_EndToEndDispatch(t *testing.T) {
	ep, root := newDeclaredEndpoint(t)
	if err := ep.SetChannelConfig(root, spi.Channel2, spi.ChannelConfig{Enabled: true, ClockHz: 1000, Mode: spi.Mode0}); err != nil {
		t.Fatalf("SetChannelConfig: %v", err)
	}

	req := acf.Message{
		Kind:      acf.KindShort,
		ByteBusID: avtp.ByteBusID(1),
		EVT:       goldenTransferEVT,
		Control:   acf.FlagWrite,
		Body:      goldenTransferRequest,
	}
	resp, err := ep.HandleRequest(root, req)
	if err != nil {
		t.Fatalf("HandleRequest(golden transfer): %v", err)
	}
	// Default loopback: the response echoes the transmitted bytes.
	if !bytes.Equal(resp.Body, goldenTransferRequest) {
		t.Fatalf("HandleRequest(golden transfer) response body = % X, want % X (loopback)", resp.Body, goldenTransferRequest)
	}
}
