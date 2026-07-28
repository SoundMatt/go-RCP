//fusa:test REQ-UART-010

package uart_test

import (
	"bytes"
	"testing"

	"github.com/SoundMatt/go-RCP/avtp"
	"github.com/SoundMatt/go-RCP/uart"
)

// ── REQ-UART-010 (golden-vector half): frozen UART Config/request/response
// byte layouts ──
//
// These fixtures pin the exact wire bytes this package's encoders produce
// today, so Phase 15's conditional-request work and later endpoint types can
// regression-test against a frozen UART encoding rather than re-deriving it
// from current behaviour, the same posture gpio/golden_test.go and
// spi/golden_test.go established.

// goldenConfig is Enabled=1, BaudRate=115200 (0x0001C200), DataBits=8,
// Parity=ParityEven(2), StopBits=StopBitsOne(0), FlowControl=0,
// ReadTimeoutMicros=50000 (0x0000C350).
var goldenConfig = []byte{
	0x01,
	0x00, 0x01, 0xC2, 0x00,
	0x08,
	0x02,
	0x00,
	0x00,
	0x00, 0x00, 0xC3, 0x50,
}

func TestGolden_Config(t *testing.T) {
	cfg := uart.Config{
		Enabled:           true,
		BaudRate:          115200,
		DataBits:          8,
		Parity:            uart.ParityEven,
		StopBits:          uart.StopBitsOne,
		ReadTimeoutMicros: 50000,
	}
	got := uart.EncodeConfig(cfg)
	if !bytes.Equal(got, goldenConfig) {
		t.Fatalf("EncodeConfig changed:\n got  % X\n want % X", got, goldenConfig)
	}
	decoded, err := uart.DecodeConfig(goldenConfig)
	if err != nil {
		t.Fatalf("DecodeConfig(golden): %v", err)
	}
	if decoded != cfg {
		t.Errorf("DecodeConfig(golden) = %+v, want %+v", decoded, cfg)
	}
}

// goldenReadResponse is a complete (0x01) read response carrying three bytes.
var goldenReadResponse = []byte{0x01, 0xDE, 0xAD, 0xBE}

func TestGolden_ReadResponse(t *testing.T) {
	got := uart.EncodeReadResponse(true, []byte{0xDE, 0xAD, 0xBE})
	if !bytes.Equal(got, goldenReadResponse) {
		t.Fatalf("EncodeReadResponse changed:\n got  % X\n want % X", got, goldenReadResponse)
	}
	complete, data, err := uart.DecodeReadResponse(goldenReadResponse)
	if err != nil {
		t.Fatalf("DecodeReadResponse(golden): %v", err)
	}
	if !complete || !bytes.Equal(data, []byte{0xDE, 0xAD, 0xBE}) {
		t.Errorf("DecodeReadResponse(golden) = (%v, % X), want (true, DE AD BE)", complete, data)
	}
}

func TestGolden_EndToEndDispatch(t *testing.T) {
	ep, root := newDeclaredEndpoint(t)
	cfg := uart.Config{Enabled: true, BaudRate: 9600, DataBits: 8, Parity: uart.ParityNone, StopBits: uart.StopBitsOne}
	if err := ep.Configure(root, cfg); err != nil {
		t.Fatalf("Configure: %v", err)
	}

	req := avtp.Message{
		Kind:      avtp.KindShort,
		ByteBusID: avtp.ByteBusID(1),
		Control:   avtp.FlagWrite,
		Body:      []byte{0xDE, 0xAD, 0xBE},
	}
	resp, err := ep.HandleRequest(root, req)
	if err != nil {
		t.Fatalf("HandleRequest(golden write): %v", err)
	}
	n, err := uart.DecodeWriteResponse(resp.Body)
	if err != nil || n != 3 {
		t.Fatalf("DecodeWriteResponse = (%d, %v), want (3, nil)", n, err)
	}

	readResp, err := ep.HandleRequest(root, avtp.Message{
		Kind: avtp.KindShort, ByteBusID: avtp.ByteBusID(1), Control: avtp.FlagRead, ReadSizeOrSegment: 3,
	})
	if err != nil {
		t.Fatalf("HandleRequest(golden read): %v", err)
	}
	if !bytes.Equal(readResp.Body, goldenReadResponse) {
		t.Fatalf("HandleRequest(golden read) response body = % X, want % X", readResp.Body, goldenReadResponse)
	}
}
