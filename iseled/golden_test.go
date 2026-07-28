//fusa:test REQ-ISELED-009

package iseled_test

import (
	"bytes"
	"testing"

	"github.com/SoundMatt/go-RCP/acf"
	"github.com/SoundMatt/go-RCP/avtp"
	"github.com/SoundMatt/go-RCP/iseled"
)

// ── REQ-ISELED-009 (golden-vector half): frozen ISELED Config/Command byte
// layouts ──
//
// These fixtures pin the exact wire bytes this package's encoders produce
// today, so later work can regression-test against a frozen ISELED
// encoding rather than re-deriving it from current behaviour, the same
// posture i2c/golden_test.go established.

// goldenConfig is Enabled=1, DeviceCount=4, ResponseTimeoutMicros=500.
var goldenConfig = []byte{0x01, 0x04, 0x00, 0x00, 0x01, 0xF4}

func TestGolden_Config(t *testing.T) {
	cfg := iseled.Config{Enabled: true, DeviceCount: 4, ResponseTimeoutMicros: 500}
	got := iseled.EncodeConfig(cfg)
	if !bytes.Equal(got, goldenConfig) {
		t.Fatalf("EncodeConfig changed:\n got  % X\n want % X", got, goldenConfig)
	}
	decoded, err := iseled.DecodeConfig(goldenConfig)
	if err != nil {
		t.Fatalf("DecodeConfig(golden): %v", err)
	}
	if decoded != cfg {
		t.Errorf("DecodeConfig(golden) = %+v, want %+v", decoded, cfg)
	}
}

// goldenCommand is a command addressed to device 2, carrying two data
// bytes, with its ISELED-native CRC8 (computed by ComputeCRC over
// Address+Data) as the trailing byte.
var goldenCommand = []byte{0x02, 0x00, 0x02, 0x10, 0x20, iseled.ComputeCRC([]byte{0x02, 0x10, 0x20})}

func TestGolden_Command(t *testing.T) {
	cmd := iseled.Command{Address: 2, Data: []byte{0x10, 0x20}}
	got := iseled.EncodeCommand(cmd)
	if !bytes.Equal(got, goldenCommand) {
		t.Fatalf("EncodeCommand changed:\n got  % X\n want % X", got, goldenCommand)
	}
	decoded, err := iseled.DecodeCommand(goldenCommand)
	if err != nil {
		t.Fatalf("DecodeCommand(golden): %v", err)
	}
	if decoded.Address != cmd.Address || !bytes.Equal(decoded.Data, cmd.Data) {
		t.Errorf("DecodeCommand(golden) = %+v, want %+v", decoded, cmd)
	}
}

func TestGolden_EndToEndDispatch(t *testing.T) {
	ep, root := newDeclaredEndpoint(t)
	if err := ep.Configure(root, iseled.Config{Enabled: true, DeviceCount: 4}); err != nil {
		t.Fatalf("Configure: %v", err)
	}

	req := acf.Message{
		Kind:      acf.KindShort,
		ByteBusID: avtp.ByteBusID(1),
		Control:   acf.FlagWrite,
		Body:      goldenCommand,
	}
	resp, err := ep.HandleRequest(root, req)
	if err != nil {
		t.Fatalf("HandleRequest(golden command): %v", err)
	}
	got, err := iseled.DecodeAggregatedResponse(resp.Body)
	if err != nil {
		t.Fatalf("DecodeAggregatedResponse: %v", err)
	}
	if len(got) != 1 || got[0].Address != 2 || !bytes.Equal(got[0].Data, []byte{0x10, 0x20}) {
		t.Fatalf("HandleRequest(golden command) response = %+v, want one entry echoing device 2", got)
	}
}
