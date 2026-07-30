//fusa:test REQ-MDIO-008

package mdio_test

import (
	"bytes"
	"testing"

	"github.com/SoundMatt/go-RCP/acf"
	"github.com/SoundMatt/go-RCP/avtp"
	"github.com/SoundMatt/go-RCP/mdio"
)

// ── REQ-MDIO-008 (golden-vector half): frozen MDIO Config/Request/Response
// byte layouts ──
//
// These fixtures pin the exact wire bytes this package's encoders produce
// today, so later work can regression-test against a frozen MDIO encoding
// rather than re-deriving it from current behaviour, the same posture
// i2c/golden_test.go established. Every fixture below is hand-computed
// (Mode byte, PhyAddr byte, DevAddr byte, RegAddr as two big-endian bytes,
// then a DataWidth()-wide big-endian data field for write requests and
// responses), not merely round-tripped through the package's own encoders.

// goldenConfig is Enabled=1.
var goldenConfig = []byte{0x01}

func TestGolden_Config(t *testing.T) {
	cfg := mdio.Config{Enabled: true}
	got := mdio.EncodeConfig(cfg)
	if !bytes.Equal(got, goldenConfig) {
		t.Fatalf("EncodeConfig changed:\n got  % X\n want % X", got, goldenConfig)
	}
	decoded, err := mdio.DecodeConfig(goldenConfig)
	if err != nil {
		t.Fatalf("DecodeConfig(golden): %v", err)
	}
	if decoded != cfg {
		t.Errorf("DecodeConfig(golden) = %+v, want %+v", decoded, cfg)
	}
}

// goldenReadRequest is an MMD single-word read of PHY 1, MMD device 3,
// register 0x0006: Mode=0x00 (ModeMMDSingleWord), PhyAddr=0x01,
// DevAddr=0x03, RegAddr=0x0006 (big-endian 0x00,0x06).
var goldenReadRequest = []byte{0x00, 0x01, 0x03, 0x00, 0x06}

func TestGolden_ReadRequest(t *testing.T) {
	r := mdio.Request{Mode: mdio.ModeMMDSingleWord, PhyAddr: 1, DevAddr: 3, RegAddr: 0x0006}
	got := mdio.EncodeReadRequest(r)
	if !bytes.Equal(got, goldenReadRequest) {
		t.Fatalf("EncodeReadRequest changed:\n got  % X\n want % X", got, goldenReadRequest)
	}
	decoded, err := mdio.DecodeReadRequest(goldenReadRequest)
	if err != nil {
		t.Fatalf("DecodeReadRequest(golden): %v", err)
	}
	if decoded != r {
		t.Errorf("DecodeReadRequest(golden) = %+v, want %+v", decoded, r)
	}
}

// goldenWriteRequestMMD is an MMD multiple-byte write of PHY 5, MMD device
// 7, register 0x1234, value 0xBEEF: header Mode=0x01 (ModeMMDMultiByte),
// PhyAddr=0x05, DevAddr=0x07, RegAddr=0x12,0x34, then the 16-bit (MMD is
// always 16-bit) data field 0xBE,0xEF.
var goldenWriteRequestMMD = []byte{0x01, 0x05, 0x07, 0x12, 0x34, 0xBE, 0xEF}

// goldenResponseMMD is the 16-bit response value 0xCAFE for the same
// (MMD) request shape as goldenWriteRequestMMD: 0xCA,0xFE.
var goldenResponseMMD = []byte{0xCA, 0xFE}

func TestGolden_WriteRequestAndResponse_MMD(t *testing.T) {
	r := mdio.Request{Mode: mdio.ModeMMDMultiByte, PhyAddr: 5, DevAddr: 7, RegAddr: 0x1234}
	got := mdio.EncodeWriteRequest(r, 0xBEEF)
	if !bytes.Equal(got, goldenWriteRequestMMD) {
		t.Fatalf("EncodeWriteRequest(MMD) changed:\n got  % X\n want % X", got, goldenWriteRequestMMD)
	}
	decodedR, decodedData, err := mdio.DecodeWriteRequest(goldenWriteRequestMMD)
	if err != nil {
		t.Fatalf("DecodeWriteRequest(golden MMD): %v", err)
	}
	if decodedR != r || decodedData != 0xBEEF {
		t.Errorf("DecodeWriteRequest(golden MMD) = %+v/%#x, want %+v/0xBEEF", decodedR, decodedData, r)
	}

	respGot := mdio.EncodeResponse(r, 0xCAFE)
	if !bytes.Equal(respGot, goldenResponseMMD) {
		t.Fatalf("EncodeResponse(MMD) changed:\n got  % X\n want % X", respGot, goldenResponseMMD)
	}
	respDecoded, err := mdio.DecodeResponse(r, goldenResponseMMD)
	if err != nil {
		t.Fatalf("DecodeResponse(golden MMD): %v", err)
	}
	if respDecoded != 0xCAFE {
		t.Errorf("DecodeResponse(golden MMD) = %#x, want 0xCAFE", respDecoded)
	}
}

// goldenWriteRequestMMS0 is an MMS single-word write of PHY 2, MMS index 0
// ("MMS0"), register 0x0010, value 0xDEADBEEF: header Mode=0x02
// (ModeMMSSingleWord), PhyAddr=0x02, DevAddr=0x00, RegAddr=0x00,0x10, then
// the 32-bit (MMS0 is 32-bit — see Request.DataWidth) data field
// 0xDE,0xAD,0xBE,0xEF.
var goldenWriteRequestMMS0 = []byte{0x02, 0x02, 0x00, 0x00, 0x10, 0xDE, 0xAD, 0xBE, 0xEF}

// goldenResponseMMS0 is the 32-bit response value 0x12345678 for the same
// (MMS0) request shape as goldenWriteRequestMMS0: 0x12,0x34,0x56,0x78.
var goldenResponseMMS0 = []byte{0x12, 0x34, 0x56, 0x78}

func TestGolden_WriteRequestAndResponse_MMS0(t *testing.T) {
	r := mdio.Request{Mode: mdio.ModeMMSSingleWord, PhyAddr: 2, DevAddr: 0, RegAddr: 0x0010}
	got := mdio.EncodeWriteRequest(r, 0xDEADBEEF)
	if !bytes.Equal(got, goldenWriteRequestMMS0) {
		t.Fatalf("EncodeWriteRequest(MMS0) changed:\n got  % X\n want % X", got, goldenWriteRequestMMS0)
	}
	decodedR, decodedData, err := mdio.DecodeWriteRequest(goldenWriteRequestMMS0)
	if err != nil {
		t.Fatalf("DecodeWriteRequest(golden MMS0): %v", err)
	}
	if decodedR != r || decodedData != 0xDEADBEEF {
		t.Errorf("DecodeWriteRequest(golden MMS0) = %+v/%#x, want %+v/0xDEADBEEF", decodedR, decodedData, r)
	}

	respGot := mdio.EncodeResponse(r, 0x12345678)
	if !bytes.Equal(respGot, goldenResponseMMS0) {
		t.Fatalf("EncodeResponse(MMS0) changed:\n got  % X\n want % X", respGot, goldenResponseMMS0)
	}
	respDecoded, err := mdio.DecodeResponse(r, goldenResponseMMS0)
	if err != nil {
		t.Fatalf("DecodeResponse(golden MMS0): %v", err)
	}
	if respDecoded != 0x12345678 {
		t.Errorf("DecodeResponse(golden MMS0) = %#x, want 0x12345678", respDecoded)
	}
}

// goldenWriteRequestMMS1 is an MMS multi-word write of PHY 9, MMS index 1
// ("MMS1"), register 0x0020, value 0xAABBCCDD: header Mode=0x03
// (ModeMMSMultiWord), PhyAddr=0x09, DevAddr=0x01, RegAddr=0x00,0x20, then
// the 32-bit (MMS1 is also 32-bit) data field 0xAA,0xBB,0xCC,0xDD.
var goldenWriteRequestMMS1 = []byte{0x03, 0x09, 0x01, 0x00, 0x20, 0xAA, 0xBB, 0xCC, 0xDD}

func TestGolden_WriteRequest_MMS1(t *testing.T) {
	r := mdio.Request{Mode: mdio.ModeMMSMultiWord, PhyAddr: 9, DevAddr: 1, RegAddr: 0x0020}
	got := mdio.EncodeWriteRequest(r, 0xAABBCCDD)
	if !bytes.Equal(got, goldenWriteRequestMMS1) {
		t.Fatalf("EncodeWriteRequest(MMS1) changed:\n got  % X\n want % X", got, goldenWriteRequestMMS1)
	}
	decodedR, decodedData, err := mdio.DecodeWriteRequest(goldenWriteRequestMMS1)
	if err != nil {
		t.Fatalf("DecodeWriteRequest(golden MMS1): %v", err)
	}
	if decodedR != r || decodedData != 0xAABBCCDD {
		t.Errorf("DecodeWriteRequest(golden MMS1) = %+v/%#x, want %+v/0xAABBCCDD", decodedR, decodedData, r)
	}
}

// goldenWriteRequestMMSOther is an MMS single-word write of PHY 4, MMS
// index 5 (neither MMS0 nor MMS1), register 0x0008, value 0x1357: header
// Mode=0x02 (ModeMMSSingleWord), PhyAddr=0x04, DevAddr=0x05,
// RegAddr=0x00,0x08, then the 16-bit (every non-MMS0/MMS1 index is
// 16-bit) data field 0x13,0x57 — this is the case the pre-fix code got
// wrong by assuming every MDIO access was 16-bit-vs-fixed-Clause22/45
// shaped rather than mode-and-index-dependent.
var goldenWriteRequestMMSOther = []byte{0x02, 0x04, 0x05, 0x00, 0x08, 0x13, 0x57}

func TestGolden_WriteRequest_MMSOther(t *testing.T) {
	r := mdio.Request{Mode: mdio.ModeMMSSingleWord, PhyAddr: 4, DevAddr: 5, RegAddr: 0x0008}
	got := mdio.EncodeWriteRequest(r, 0x1357)
	if !bytes.Equal(got, goldenWriteRequestMMSOther) {
		t.Fatalf("EncodeWriteRequest(MMS, non-0/1) changed:\n got  % X\n want % X", got, goldenWriteRequestMMSOther)
	}
	decodedR, decodedData, err := mdio.DecodeWriteRequest(goldenWriteRequestMMSOther)
	if err != nil {
		t.Fatalf("DecodeWriteRequest(golden MMS other): %v", err)
	}
	if decodedR != r || decodedData != 0x1357 {
		t.Errorf("DecodeWriteRequest(golden MMS other) = %+v/%#x, want %+v/0x1357", decodedR, decodedData, r)
	}
}

func TestGolden_EndToEndDispatch(t *testing.T) {
	ep, root := newDeclaredEndpoint(t)
	if err := ep.Configure(root, mdio.Config{Enabled: true}); err != nil {
		t.Fatalf("Configure: %v", err)
	}

	req := acf.Message{
		Kind:      acf.KindShort,
		ByteBusID: avtp.ByteBusID(1),
		Control:   acf.FlagRead,
		Body:      goldenReadRequest,
	}
	resp, err := ep.HandleRequest(root, req)
	if err != nil {
		t.Fatalf("HandleRequest(golden read): %v", err)
	}
	r, err := mdio.DecodeReadRequest(goldenReadRequest)
	if err != nil {
		t.Fatalf("DecodeReadRequest(golden read): %v", err)
	}
	got, err := mdio.DecodeResponse(r, resp.Body)
	if err != nil {
		t.Fatalf("DecodeResponse: %v", err)
	}
	if got != 0 { // default store: unset register reads as zero
		t.Fatalf("HandleRequest(golden read) = %#x, want 0", got)
	}
}

// TestGolden_EndToEndDispatch_MMS0 checks a full write/read round trip
// through Endpoint.HandleRequest for an MMS0 (32-bit) access, so the
// 32-bit path is exercised end to end and not just at the Encode/Decode
// layer.
func TestGolden_EndToEndDispatch_MMS0(t *testing.T) {
	ep, root := newDeclaredEndpoint(t)
	if err := ep.Configure(root, mdio.Config{Enabled: true}); err != nil {
		t.Fatalf("Configure: %v", err)
	}

	writeReq := acf.Message{
		Kind:      acf.KindShort,
		ByteBusID: avtp.ByteBusID(1),
		Control:   acf.FlagWrite,
		Body:      goldenWriteRequestMMS0,
	}
	if _, err := ep.HandleRequest(root, writeReq); err != nil {
		t.Fatalf("HandleRequest(golden write MMS0): %v", err)
	}

	readReq := acf.Message{
		Kind:      acf.KindShort,
		ByteBusID: avtp.ByteBusID(1),
		Control:   acf.FlagRead,
		Body:      mdio.EncodeReadRequest(mdio.Request{Mode: mdio.ModeMMSSingleWord, PhyAddr: 2, DevAddr: 0, RegAddr: 0x0010}),
	}
	resp, err := ep.HandleRequest(root, readReq)
	if err != nil {
		t.Fatalf("HandleRequest(golden read MMS0): %v", err)
	}
	r := mdio.Request{Mode: mdio.ModeMMSSingleWord, PhyAddr: 2, DevAddr: 0, RegAddr: 0x0010}
	got, err := mdio.DecodeResponse(r, resp.Body)
	if err != nil {
		t.Fatalf("DecodeResponse: %v", err)
	}
	if got != 0xDEADBEEF {
		t.Fatalf("HandleRequest(golden read MMS0) = %#x, want 0xDEADBEEF", got)
	}
}
