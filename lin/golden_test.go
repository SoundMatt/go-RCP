//fusa:test REQ-LINEP-008

package lin_test

import (
	"bytes"
	"testing"

	"github.com/SoundMatt/go-RCP/acf"
	"github.com/SoundMatt/go-RCP/avtp"
	"github.com/SoundMatt/go-RCP/lin"
)

// ── REQ-LINEP-008 (golden-vector half): frozen LIN Config/request/response
// byte layouts ──
//
// These fixtures pin the exact wire bytes this package's encoders produce
// today, so later work can regression-test against a frozen LIN encoding
// rather than re-deriving it from current behaviour, the same posture
// i2c/golden_test.go established.

// goldenConfig is Enabled=1, BaudRate=19200, TrailingTimeMicros=100.
var goldenConfig = []byte{0x01, 0x00, 0x00, 0x4B, 0x00, 0x00, 0x64}

func TestGolden_Config(t *testing.T) {
	cfg := lin.Config{Enabled: true, BaudRate: 19200, TrailingTimeMicros: 100}
	got := lin.EncodeConfig(cfg)
	if !bytes.Equal(got, goldenConfig) {
		t.Fatalf("EncodeConfig changed:\n got  % X\n want % X", got, goldenConfig)
	}
	decoded, err := lin.DecodeConfig(goldenConfig)
	if err != nil {
		t.Fatalf("DecodeConfig(golden): %v", err)
	}
	if decoded != cfg {
		t.Errorf("DecodeConfig(golden) = %+v, want %+v", decoded, cfg)
	}
}

// goldenTransferRequest is a commander-issued raw frame: two opaque bytes.
var goldenTransferRequest = []byte{0x50, 0x01}

func TestGolden_TransferRequest(t *testing.T) {
	got := lin.EncodeTransferRequest([]byte{0x50, 0x01})
	if !bytes.Equal(got, goldenTransferRequest) {
		t.Fatalf("EncodeTransferRequest changed:\n got  % X\n want % X", got, goldenTransferRequest)
	}
	if got := lin.DecodeTransferRequest(goldenTransferRequest); !bytes.Equal(got, goldenTransferRequest) {
		t.Errorf("DecodeTransferRequest(golden) = % X, want % X", got, goldenTransferRequest)
	}
}

func TestGolden_EndToEndDispatch(t *testing.T) {
	ep, root := newDeclaredEndpoint(t)
	if err := ep.Configure(root, lin.Config{Enabled: true, BaudRate: 19200}); err != nil {
		t.Fatalf("Configure: %v", err)
	}

	req := acf.Message{
		Kind:      acf.KindShort,
		ByteBusID: avtp.ByteBusID(1),
		Control:   acf.FlagWrite,
		Body:      goldenTransferRequest,
	}
	resp, err := ep.HandleRequest(root, req)
	if err != nil {
		t.Fatalf("HandleRequest(golden transfer): %v", err)
	}
	// Default loopback: response echoes the same bytes.
	if !bytes.Equal(resp.Body, goldenTransferRequest) {
		t.Fatalf("HandleRequest(golden transfer) response body = % X, want % X (loopback)", resp.Body, goldenTransferRequest)
	}
}
