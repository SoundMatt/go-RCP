//fusa:test REQ-WIRE-001
//fusa:test REQ-WIRE-002
//fusa:test REQ-WIRE-003
//fusa:test REQ-WIRE-004
//fusa:test REQ-WIRE-005
//fusa:test REQ-WIRE-006
//fusa:test REQ-WIRE-007
//fusa:test REQ-WIRE-008
//fusa:test REQ-WIRE-009

package wire_test

import (
	"bytes"
	"errors"
	"testing"

	rcp "github.com/SoundMatt/go-RCP"
	"github.com/SoundMatt/go-RCP/wire"
)

// validHeader returns a minimal well-formed header of the given message type.
func validHeader(msgType byte) []byte {
	b := make([]byte, wire.HeaderLen)
	b[0] = wire.MagicByte0
	b[1] = wire.MagicByte1
	b[2] = wire.ProtoVer
	b[3] = msgType
	return b
}

// ── REQ-WIRE-001/002/003: header validation ───────────────────────────────────

func TestValidateHeader_Valid(t *testing.T) {
	if err := wire.ValidateHeader(validHeader(wire.TypeCommand)); err != nil {
		t.Fatalf("ValidateHeader(valid) = %v, want nil", err)
	}
}

func TestValidateHeader_ShortFrame(t *testing.T) {
	for n := 0; n < wire.HeaderLen; n++ {
		if err := wire.ValidateHeader(make([]byte, n)); !errors.Is(err, wire.ErrShortFrame) {
			t.Errorf("ValidateHeader(len %d) = %v, want ErrShortFrame", n, err)
		}
	}
}

func TestValidateHeader_BadMagic(t *testing.T) {
	b := validHeader(wire.TypeCommand)
	b[0] = 0x00
	if err := wire.ValidateHeader(b); !errors.Is(err, wire.ErrBadMagic) {
		t.Errorf("byte0 corrupt: got %v, want ErrBadMagic", err)
	}
	b = validHeader(wire.TypeCommand)
	b[1] = 0xFF
	if err := wire.ValidateHeader(b); !errors.Is(err, wire.ErrBadMagic) {
		t.Errorf("byte1 corrupt: got %v, want ErrBadMagic", err)
	}
}

func TestValidateHeader_BadVersion(t *testing.T) {
	b := validHeader(wire.TypeCommand)
	b[2] = wire.ProtoVer + 1
	if err := wire.ValidateHeader(b); !errors.Is(err, wire.ErrBadVersion) {
		t.Errorf("ValidateHeader(bad version) = %v, want ErrBadVersion", err)
	}
}

// ── REQ-WIRE-004: Command round-trip ───────────────────────────────────────────

func TestCommand_RoundTrip(t *testing.T) {
	cases := []struct {
		name string
		cmd  rcp.Command
	}{
		{"empty payload", rcp.Command{Zone: rcp.ZoneFrontLeft, Type: rcp.CmdGet, Priority: rcp.PriorityNormal, ID: 1}},
		{"with payload", rcp.Command{Zone: rcp.ZoneFrontRight, Type: rcp.CmdSet, Priority: rcp.PriorityHigh, ID: 0xDEADBEEF, Payload: []byte("hello wire")}},
		{"max-ish id", rcp.Command{Zone: rcp.ZoneFrontLeft, Type: rcp.CmdSet, Priority: rcp.PriorityHigh, ID: 0xFFFFFFFF, Payload: []byte{0x00, 0x01, 0xFF}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := wire.DecodeCommand(wire.EncodeCommand(&tc.cmd))
			if err != nil {
				t.Fatalf("DecodeCommand: %v", err)
			}
			if got.Zone != tc.cmd.Zone || got.Type != tc.cmd.Type ||
				got.Priority != tc.cmd.Priority || got.ID != tc.cmd.ID {
				t.Errorf("field mismatch: got %+v want %+v", got, tc.cmd)
			}
			if !bytes.Equal(got.Payload, normalize(tc.cmd.Payload)) {
				t.Errorf("payload mismatch: got %q want %q", got.Payload, tc.cmd.Payload)
			}
		})
	}
}

// ── REQ-WIRE-005: Response round-trip ──────────────────────────────────────────

func TestResponse_RoundTrip(t *testing.T) {
	cases := []struct {
		name string
		resp rcp.Response
	}{
		{"ok empty", rcp.Response{Zone: rcp.ZoneFrontLeft, Status: rcp.StatusOK, CommandID: 7}},
		{"error with payload", rcp.Response{Zone: rcp.ZoneFrontRight, Status: rcp.StatusError, CommandID: 0x01020304, Payload: []byte("fault detail")}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := wire.DecodeResponse(wire.EncodeResponse(&tc.resp))
			if err != nil {
				t.Fatalf("DecodeResponse: %v", err)
			}
			if got.Zone != tc.resp.Zone || got.Status != tc.resp.Status || got.CommandID != tc.resp.CommandID {
				t.Errorf("field mismatch: got %+v want %+v", got, tc.resp)
			}
			if !bytes.Equal(got.Payload, normalize(tc.resp.Payload)) {
				t.Errorf("payload mismatch: got %q want %q", got.Payload, tc.resp.Payload)
			}
		})
	}
}

// ── REQ-WIRE-006: Status round-trip ────────────────────────────────────────────

func TestStatus_RoundTrip(t *testing.T) {
	cases := []struct {
		name string
		st   rcp.Status
	}{
		{"healthy empty", rcp.Status{Zone: rcp.ZoneFrontLeft, Healthy: true, Seq: 42}},
		{"unhealthy payload", rcp.Status{Zone: rcp.ZoneFrontRight, Healthy: false, Seq: 0xCAFEBABE, Payload: []byte(`{"healthy":false}`)}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := wire.DecodeStatus(wire.EncodeStatus(&tc.st))
			if err != nil {
				t.Fatalf("DecodeStatus: %v", err)
			}
			if got.Zone != tc.st.Zone || got.Healthy != tc.st.Healthy || got.Seq != tc.st.Seq {
				t.Errorf("field mismatch: got %+v want %+v", got, tc.st)
			}
			if !bytes.Equal(got.Payload, normalize(tc.st.Payload)) {
				t.Errorf("payload mismatch: got %q want %q", got.Payload, tc.st.Payload)
			}
		})
	}
}

// ── REQ-WIRE-007: truncated body rejected ──────────────────────────────────────

func TestDecode_TruncatedBody(t *testing.T) {
	// Encode a frame with a 5-byte payload, then lop off the last 3 bytes so the
	// declared body length exceeds what is present.
	cmd := wire.EncodeCommand(&rcp.Command{Zone: rcp.ZoneFrontLeft, ID: 1, Payload: []byte("12345")})
	resp := wire.EncodeResponse(&rcp.Response{Zone: rcp.ZoneFrontLeft, CommandID: 1, Payload: []byte("12345")})
	st := wire.EncodeStatus(&rcp.Status{Zone: rcp.ZoneFrontLeft, Seq: 1, Payload: []byte("12345")})

	if _, err := wire.DecodeCommand(cmd[:len(cmd)-3]); !errors.Is(err, wire.ErrShortFrame) {
		t.Errorf("DecodeCommand(truncated) = %v, want ErrShortFrame", err)
	}
	if _, err := wire.DecodeResponse(resp[:len(resp)-3]); !errors.Is(err, wire.ErrShortFrame) {
		t.Errorf("DecodeResponse(truncated) = %v, want ErrShortFrame", err)
	}
	if _, err := wire.DecodeStatus(st[:len(st)-3]); !errors.Is(err, wire.ErrShortFrame) {
		t.Errorf("DecodeStatus(truncated) = %v, want ErrShortFrame", err)
	}
}

func TestDecode_ShortHeaderRejected(t *testing.T) {
	short := make([]byte, wire.HeaderLen-1)
	if _, err := wire.DecodeCommand(short); !errors.Is(err, wire.ErrShortFrame) {
		t.Errorf("DecodeCommand(short header) = %v, want ErrShortFrame", err)
	}
	if _, err := wire.DecodeResponse(short); !errors.Is(err, wire.ErrShortFrame) {
		t.Errorf("DecodeResponse(short header) = %v, want ErrShortFrame", err)
	}
	if _, err := wire.DecodeStatus(short); !errors.Is(err, wire.ErrShortFrame) {
		t.Errorf("DecodeStatus(short header) = %v, want ErrShortFrame", err)
	}
}

// ── REQ-WIRE-008: control frame ────────────────────────────────────────────────

func TestEncodeControlFrame(t *testing.T) {
	for _, mt := range []byte{wire.TypeSubscribe, wire.TypeUnsubscribe} {
		frame := wire.EncodeControlFrame(mt, rcp.ZoneFrontRight)
		if len(frame) != wire.HeaderLen {
			t.Fatalf("control frame len = %d, want %d", len(frame), wire.HeaderLen)
		}
		if err := wire.ValidateHeader(frame); err != nil {
			t.Errorf("control frame failed ValidateHeader: %v", err)
		}
		if frame[3] != mt {
			t.Errorf("msgType byte = %#x, want %#x", frame[3], mt)
		}
		if frame[4] != byte(rcp.ZoneFrontRight) {
			t.Errorf("zone byte = %#x, want %#x", frame[4], byte(rcp.ZoneFrontRight))
		}
	}
}

// ── REQ-WIRE-009: fuzz robustness of the decode trust boundary ─────────────────

func FuzzDecodeCommand(f *testing.F) {
	seedDecoders(f)
	f.Fuzz(func(t *testing.T, b []byte) {
		_, _ = wire.DecodeCommand(b) // must not panic
	})
}

func FuzzDecodeResponse(f *testing.F) {
	seedDecoders(f)
	f.Fuzz(func(t *testing.T, b []byte) {
		_, _ = wire.DecodeResponse(b) // must not panic
	})
}

func FuzzDecodeStatus(f *testing.F) {
	seedDecoders(f)
	f.Fuzz(func(t *testing.T, b []byte) {
		_, _ = wire.DecodeStatus(b) // must not panic
	})
}

func seedDecoders(f *testing.F) {
	f.Helper()
	f.Add([]byte{})
	f.Add(make([]byte, wire.HeaderLen))
	f.Add(wire.EncodeCommand(&rcp.Command{Zone: rcp.ZoneFrontLeft, ID: 1, Payload: []byte("seed")}))
	f.Add(wire.EncodeResponse(&rcp.Response{Zone: rcp.ZoneFrontLeft, CommandID: 1, Payload: []byte("seed")}))
	f.Add(wire.EncodeStatus(&rcp.Status{Zone: rcp.ZoneFrontLeft, Seq: 1, Payload: []byte("seed")}))
	// A header that declares a huge body but carries none — the classic
	// out-of-bounds trigger that ErrShortFrame must guard against.
	huge := make([]byte, wire.HeaderLen)
	huge[0], huge[1], huge[2] = wire.MagicByte0, wire.MagicByte1, wire.ProtoVer
	huge[12], huge[13], huge[14], huge[15] = 0xFF, 0xFF, 0xFF, 0xFF
	f.Add(huge)
}

// normalize maps a nil/empty payload to nil so round-trip comparisons treat an
// encoded empty payload (decoded as nil) and an input empty slice as equal.
func normalize(b []byte) []byte {
	if len(b) == 0 {
		return nil
	}
	return b
}
