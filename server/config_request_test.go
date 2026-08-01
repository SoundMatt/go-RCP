//fusa:test REQ-RCS-032

package server_test

import (
	"bytes"
	"errors"
	"testing"

	"github.com/SoundMatt/go-RCP/acf"
	"github.com/SoundMatt/go-RCP/avtp"
	"github.com/SoundMatt/go-RCP/regmap"
	"github.com/SoundMatt/go-RCP/server"
)

// ── REQ-RCS-032: TC18 §12.7.1 offset-addressed functional configuration ───
//
// §12.7.1 defines the configuration request an endpoint answers when
// evt[2:0] = 111b: its byte_msg_payload leads with the "relative Register
// start address in EP_func" (Figure 18) and continues with the
// configuration data to write there. The rule this file cares most about is
// the overrun rule, stated verbatim: "Any byte_msg_payload for which the
// length plus the start_address results in a value larger than the EP_LEN,
// is to be ignored."

const cfgTestAddr = avtp.ByteBusID(7)

// newConfigServer returns a Server with one endpoint declared at
// cfgTestAddr, its functional block pre-populated with block, and root
// claimed by the returned stream.
func newConfigServer(t *testing.T, block []byte) (*server.Server, avtp.StreamID) {
	t.Helper()
	root := avtp.NewStreamID([6]byte{0x02, 0x99, 0x88, 0x77, 0x66, 0x55}, 3)
	s := server.NewServer()
	if err := s.ClaimRoot(root); err != nil {
		t.Fatalf("ClaimRoot: %v", err)
	}
	if err := s.AddEndpoint(root, cfgTestAddr, regmap.EndpointTypeGPIO); err != nil {
		t.Fatalf("AddEndpoint: %v", err)
	}
	if err := s.WriteFunctional(root, cfgTestAddr, block); err != nil {
		t.Fatalf("WriteFunctional: %v", err)
	}
	return s, root
}

// currentBlock is a small stateful stand-in for an endpoint package's
// EncodeConfig/DecodeConfig+Validate pair, so this test exercises
// ApplyConfigRequest's own contract rather than any one endpoint type's
// configuration layout.
type currentBlock struct {
	data   []byte
	reject error // if non-nil, adopt refuses every patched block
}

func (c *currentBlock) encode() []byte { return append([]byte(nil), c.data...) }

func (c *currentBlock) adopt(raw []byte) error {
	if c.reject != nil {
		return c.reject
	}
	c.data = append([]byte(nil), raw...)
	return nil
}

func cfgWrite(startAddr uint16, data []byte) acf.Message {
	return acf.Message{
		Kind: acf.KindShort, ByteBusID: cfgTestAddr,
		EVT: uint8(acf.EVTSelector7), Control: acf.FlagWrite,
		Body: acf.EncodeConfigRequestBody(startAddr, data),
	}
}

// TestApplyConfigRequest_PatchesAtStartAddress checks a configuration write
// lands at its relative EP_func start address, leaves every byte outside its
// range untouched, is adopted by the endpoint, and is persisted to the
// register map (REQ-RCS-032).
func TestApplyConfigRequest_PatchesAtStartAddress(t *testing.T) {
	block := []byte{0x11, 0x22, 0x33, 0x44, 0x55}
	s, root := newConfigServer(t, block)
	cur := &currentBlock{data: append([]byte(nil), block...)}

	body, err := s.ApplyConfigRequest(root, cfgTestAddr, cfgWrite(1, []byte{0xAA, 0xBB}), cur.encode, cur.adopt)
	if err != nil {
		t.Fatalf("ApplyConfigRequest: %v", err)
	}
	if body != nil {
		t.Errorf("write response body = % X, want nil", body)
	}

	want := []byte{0x11, 0xAA, 0xBB, 0x44, 0x55}
	if !bytes.Equal(cur.data, want) {
		t.Errorf("adopted block = % X, want % X", cur.data, want)
	}

	stored, err := s.ReadEndpoint(root, cfgTestAddr)
	if err != nil {
		t.Fatalf("ReadEndpoint: %v", err)
	}
	if !bytes.Contains(stored, want) {
		t.Errorf("persisted register map does not contain the patched block % X (got % X)", want, stored)
	}
}

// TestApplyConfigRequest_IgnoresOverrun checks §12.7.1's overrun rule
// verbatim: "Any byte_msg_payload for which the length plus the start_address
// results in a value larger than the EP_LEN, is to be ignored" — no error, no
// truncation, and no partial application (REQ-RCS-032).
func TestApplyConfigRequest_IgnoresOverrun(t *testing.T) {
	block := []byte{0x11, 0x22, 0x33, 0x44}
	s, root := newConfigServer(t, block)
	cur := &currentBlock{data: append([]byte(nil), block...)}

	tests := []struct {
		name  string
		start uint16
		data  []byte
	}{
		{"runs one byte past the end", 3, []byte{0xAA, 0xBB}},
		{"starts past the end", 9, []byte{0xAA}},
		{"whole block plus one", 0, []byte{1, 2, 3, 4, 5}},
	}
	for _, tt := range tests {
		if _, err := s.ApplyConfigRequest(root, cfgTestAddr, cfgWrite(tt.start, tt.data), cur.encode, cur.adopt); err != nil {
			t.Errorf("%s: err = %v, want nil (the request is to be ignored, not rejected)", tt.name, err)
		}
		if !bytes.Equal(cur.data, block) {
			t.Errorf("%s: block = % X, want unchanged % X", tt.name, cur.data, block)
		}
	}
}

// TestApplyConfigRequest_RejectsInvalidPatchAndShortBody checks a patched
// block the endpoint refuses is not persisted, and that a body too short to
// hold the Figure 18 start address is rejected rather than misread
// (REQ-RCS-032).
func TestApplyConfigRequest_RejectsInvalidPatchAndShortBody(t *testing.T) {
	block := []byte{0x11, 0x22, 0x33, 0x44}
	s, root := newConfigServer(t, block)

	sentinel := errors.New("not a valid configuration")
	cur := &currentBlock{data: append([]byte(nil), block...), reject: sentinel}
	if _, err := s.ApplyConfigRequest(root, cfgTestAddr, cfgWrite(0, []byte{0xFF}), cur.encode, cur.adopt); !errors.Is(err, sentinel) {
		t.Errorf("invalid patch: err = %v, want %v", err, sentinel)
	}
	stored, err := s.ReadEndpoint(root, cfgTestAddr)
	if err != nil {
		t.Fatalf("ReadEndpoint: %v", err)
	}
	if !bytes.Contains(stored, block) {
		t.Errorf("a refused configuration was persisted anyway (map = % X)", stored)
	}

	ok := &currentBlock{data: append([]byte(nil), block...)}
	short := acf.Message{
		Kind: acf.KindShort, ByteBusID: cfgTestAddr,
		EVT: uint8(acf.EVTSelector7), Control: acf.FlagWrite, Body: []byte{0x00},
	}
	if _, err := s.ApplyConfigRequest(root, cfgTestAddr, short, ok.encode, ok.adopt); !errors.Is(err, acf.ErrShortConfigRequest) {
		t.Errorf("short body: err = %v, want acf.ErrShortConfigRequest", err)
	}
}

// TestApplyConfigRequest_Read checks the read direction §12.7.1 also defines:
// the response carries the EP_func bytes at the requested start address,
// bounded by the request's read_size and clamped to the end of the block
// (REQ-RCS-032).
func TestApplyConfigRequest_Read(t *testing.T) {
	block := []byte{0x11, 0x22, 0x33, 0x44, 0x55}
	s, root := newConfigServer(t, block)
	cur := &currentBlock{data: append([]byte(nil), block...)}

	read := func(start uint16, readSize uint16) []byte {
		t.Helper()
		req := acf.Message{
			Kind: acf.KindShort, ByteBusID: cfgTestAddr,
			EVT: uint8(acf.EVTSelector7), Control: acf.FlagRead,
			ReadSizeOrSegment: readSize,
			Body:              acf.EncodeConfigRequestBody(start, nil),
		}
		body, err := s.ApplyConfigRequest(root, cfgTestAddr, req, cur.encode, cur.adopt)
		if err != nil {
			t.Fatalf("ApplyConfigRequest(read): %v", err)
		}
		return body
	}

	if got, want := read(1, 2), []byte{0x22, 0x33}; !bytes.Equal(got, want) {
		t.Errorf("read(1, 2) = % X, want % X", got, want)
	}
	if got, want := read(2, 0), []byte{0x33, 0x44, 0x55}; !bytes.Equal(got, want) {
		t.Errorf("read(2, 0) = % X, want % X (whole remainder)", got, want)
	}
	if got, want := read(3, 99), []byte{0x44, 0x55}; !bytes.Equal(got, want) {
		t.Errorf("read(3, 99) = % X, want % X (clamped to the block end)", got, want)
	}
	if got := read(9, 4); len(got) != 0 {
		t.Errorf("read past the end = % X, want empty", got)
	}
}

// TestWriteFunctionalAt_AccessAndBounds checks the offset-addressed write is
// gated by exactly the same access rule as WriteFunctional, rejects an
// undeclared endpoint, and applies §12.7.1's ignore-on-overrun rule
// (REQ-RCS-032).
func TestWriteFunctionalAt_AccessAndBounds(t *testing.T) {
	block := []byte{0x11, 0x22, 0x33, 0x44}
	s, root := newConfigServer(t, block)

	stranger := avtp.NewStreamID([6]byte{0x02, 0xAA, 0xBB, 0xCC, 0xDD, 0xEE}, 9)
	if err := s.WriteFunctionalAt(stranger, cfgTestAddr, 0, []byte{0xFF}); !errors.Is(err, regmap.ErrAccessDenied) {
		t.Errorf("ungranted stream: err = %v, want regmap.ErrAccessDenied", err)
	}
	if err := s.WriteFunctionalAt(root, avtp.ByteBusID(200), 0, []byte{0xFF}); !errors.Is(err, regmap.ErrUnknownEndpoint) {
		t.Errorf("undeclared endpoint: err = %v, want regmap.ErrUnknownEndpoint", err)
	}
	if err := s.WriteFunctionalAt(root, cfgTestAddr, 3, []byte{0xAA, 0xBB}); err != nil {
		t.Errorf("overrunning write: err = %v, want nil (ignored)", err)
	}

	if err := s.WriteFunctionalAt(root, cfgTestAddr, 2, []byte{0xAA}); err != nil {
		t.Fatalf("WriteFunctionalAt: %v", err)
	}
	stored, err := s.ReadEndpoint(root, cfgTestAddr)
	if err != nil {
		t.Fatalf("ReadEndpoint: %v", err)
	}
	if want := []byte{0x11, 0x22, 0xAA, 0x44}; !bytes.Contains(stored, want) {
		t.Errorf("persisted block does not contain % X (got % X)", want, stored)
	}
}
