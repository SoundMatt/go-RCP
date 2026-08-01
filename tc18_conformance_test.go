package rcp_test

//fusa:test REQ-TC18-002
//fusa:test REQ-TC18-006
//fusa:test REQ-TC18-044
//fusa:test REQ-TC18-045
//fusa:test REQ-TC18-051
//fusa:test REQ-TC18-055
//fusa:test REQ-TC18-080
//fusa:test REQ-TC18-117
//fusa:test REQ-TC18-121
//fusa:test REQ-TC18-122
//fusa:test REQ-TC18-131
//fusa:test REQ-TC18-135
//fusa:test REQ-TC18-146
//fusa:test REQ-TC18-150
//fusa:test REQ-TC18-152
//fusa:test REQ-TC18-153
//fusa:test REQ-TC18-189
//fusa:test REQ-TC18-199
//fusa:test REQ-TC18-203
//fusa:test REQ-TC18-227
//fusa:test REQ-TC18-228
//fusa:test REQ-TC18-242

import (
	"bytes"
	"encoding/binary"
	"errors"
	"hash/crc32"
	"net"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/SoundMatt/go-RCP/acf"
	"github.com/SoundMatt/go-RCP/avtp"
	"github.com/SoundMatt/go-RCP/can"
	"github.com/SoundMatt/go-RCP/discovery"
	"github.com/SoundMatt/go-RCP/e2e"
	"github.com/SoundMatt/go-RCP/gpio"
	"github.com/SoundMatt/go-RCP/i2c"
	"github.com/SoundMatt/go-RCP/mdio"
	"github.com/SoundMatt/go-RCP/pwm"
	"github.com/SoundMatt/go-RCP/regmap"
	"github.com/SoundMatt/go-RCP/request"
	"github.com/SoundMatt/go-RCP/server"
	"github.com/SoundMatt/go-RCP/udp"
)

// This file holds spec-derived conformance tests for behaviours the OPEN
// Alliance TC18 Remote Control Protocol Specification states normatively and
// this module already implements, but that no other test in this repository
// asserts against the specification's own wording. Every assertion below is
// deliberately literal — a numeric field value, a byte position, a wire
// constant, an error code — rather than a round trip through this module's
// own encoder and decoder, which would pass equally well against a wrong
// but self-consistent implementation.

// ── Shared fixtures ─────────────────────────────────────────────────────────

// tc18Stream builds a distinct avtp.StreamID per test, so a test that
// exercises stream-scoped behaviour (access grants, e2e safe points) cannot
// accidentally pass because two identities collided.
func tc18Stream(last byte) avtp.StreamID {
	return avtp.NewStreamID([6]byte{0x02, 0x54, 0x43, 0x31, 0x38, last}, 1)
}

// tc18UntimedHeader is the NTSCF (untimed) AVTPDU header every request in
// this file is framed in unless the test specifically needs the TSCF variant.
func tc18UntimedHeader(from avtp.StreamID) avtp.Header {
	return avtp.Header{Timed: false, StreamIDValid: true, StreamID: from}
}

// tc18StubHandler is a request.Handler test double: it records every call and
// answers with a fixed body, or fails with a fixed error. The mutex is needed
// because udp.Server's own serve goroutine calls HandleRequest concurrently
// with the test goroutine inspecting the recorded fields.
type tc18StubHandler struct {
	body []byte
	err  error

	mu       sync.Mutex
	calls    int
	lastReq  acf.Message
	lastFrom avtp.StreamID
}

func (h *tc18StubHandler) HandleRequest(requester avtp.StreamID, req acf.Message) (acf.Message, error) {
	h.mu.Lock()
	h.calls++
	h.lastReq = req
	h.lastFrom = requester
	h.mu.Unlock()
	if h.err != nil {
		return acf.Message{}, h.err
	}
	return acf.Message{
		Kind:           req.Kind,
		ByteBusID:      req.ByteBusID,
		TransactionNum: req.TransactionNum,
		Control:        acf.FlagResponse | (req.Control & (acf.FlagRead | acf.FlagWrite)),
		Body:           h.body,
	}, nil
}

func (h *tc18StubHandler) callCount() int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.calls
}

func (h *tc18StubHandler) last() acf.Message {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.lastReq
}

// tc18RootServer returns a server.Server with root already claimed by the
// returned stream — the starting point every endpoint-declaring test needs.
func tc18RootServer(t *testing.T, root avtp.StreamID) *server.Server {
	t.Helper()
	srv := server.NewServer()
	if err := srv.ClaimRoot(root); err != nil {
		t.Fatalf("ClaimRoot: %v", err)
	}
	return srv
}

// tc18GPIOEndpoint declares and configures a GPIO endpoint at addr.
func tc18GPIOEndpoint(t *testing.T, srv *server.Server, root avtp.StreamID, addr avtp.ByteBusID) *gpio.Endpoint {
	t.Helper()
	if err := srv.AddEndpoint(root, addr, gpio.EndpointType); err != nil {
		t.Fatalf("AddEndpoint(%d): %v", addr, err)
	}
	ep := gpio.NewEndpoint(srv, addr)
	if err := ep.Configure(root, gpio.Config{PinCount: 4, Direction: 0b1111}); err != nil {
		t.Fatalf("gpio Configure: %v", err)
	}
	return ep
}

// ── REQ-TC18-002 ────────────────────────────────────────────────────────────

// TestTC18_002_ResponseEchoesRequestByteBusIDAndTransactionNum pins TC18
// §10.1 (TC18.txt:1023-1024): a response is addressed by echoing the
// originating request's byte_bus_id and transaction_num, never by
// substituting the responding server's own address.
//
// The addressed endpoint deliberately sits at byte_bus_id 0x17F (383) — above
// 255, so an implementation that narrowed the 11-bit field to a byte would
// answer 0x7F instead — and the assertion is made both on the decoded
// acf.Message and on the encoded octets, where §11.2.1 splits byte_bus_id's
// top three bits into octet 2 and its low eight into octet 3.
//
//fusa:test REQ-TC18-002
func TestTC18_002_ResponseEchoesRequestByteBusIDAndTransactionNum(t *testing.T) {
	const addr = avtp.ByteBusID(0x17F)
	const txn = avtp.TransactionNum(0xA7)

	root := tc18Stream(0x02)
	srv := tc18RootServer(t, root)
	ep := tc18GPIOEndpoint(t, srv, root, addr)

	req := acf.Message{
		Kind:           acf.KindShort,
		ByteBusID:      addr,
		TransactionNum: txn,
		Control:        acf.FlagRead,
	}
	resp, err := ep.HandleRequest(root, req)
	if err != nil {
		t.Fatalf("gpio HandleRequest: %v", err)
	}
	if resp.ByteBusID != addr {
		t.Errorf("response ByteBusID = %#x, want %#x (echoed, not truncated to %#x and not replaced by the endpoint's own address)",
			resp.ByteBusID, addr, addr&0xFF)
	}
	if resp.TransactionNum != txn {
		t.Errorf("response TransactionNum = %d, want %d (echoed)", resp.TransactionNum, txn)
	}

	// Wire level: §11.2.1's byte_bus_id straddles octets 2 (low three bits)
	// and 3 (low eight bits). 0x17F encodes as octet2[2:0] = 0b001 and
	// octet3 = 0x7F.
	raw, err := acf.EncodeMessage(resp)
	if err != nil {
		t.Fatalf("EncodeMessage(response): %v", err)
	}
	if got := raw[2] & 0x07; got != 0x01 {
		t.Errorf("encoded byte_bus_id[10:8] = %#03b, want 0b001", got)
	}
	if raw[3] != 0x7F {
		t.Errorf("encoded byte_bus_id[7:0] = %#02x, want 0x7f", raw[3])
	}
	if raw[5] != byte(txn) {
		t.Errorf("encoded transaction_num = %#02x, want %#02x", raw[5], byte(txn))
	}

	// The same rule holds for the transport's error-response path, where no
	// endpoint exists at all to have supplied the address: the echoed
	// byte_bus_id can only have come from the request.
	router := udp.NewRouter(udp.NewEP0Handler(srv), true)
	errResp, shouldReply := router.Route(tc18UntimedHeader(root), req)
	if !shouldReply {
		t.Fatal("Route(unknown endpoint) reported no reply; want a wire-level error response")
	}
	if !errResp.Control.Has(acf.FlagError) {
		t.Fatalf("Route(unknown endpoint) Control = %#08b, want FlagError set", errResp.Control)
	}
	if errResp.ByteBusID != addr {
		t.Errorf("error response ByteBusID = %#x, want %#x", errResp.ByteBusID, addr)
	}
	if errResp.TransactionNum != txn {
		t.Errorf("error response TransactionNum = %d, want %d", errResp.TransactionNum, txn)
	}
}

// ── REQ-TC18-006 ────────────────────────────────────────────────────────────

// TestTC18_006_KindLongWithMTVIsNotAConditionalEnvelope pins TC18 §11.2
// (TC18.txt:1116-1122): the message_timestamp slot only carries
// conditional/cancel-request metadata when mtv is clear. An ACF_GBB
// (acf.KindLong) message with MTV set is an ordinary standard request whose
// byte_msg_payload belongs to the endpoint, and must reach the wrapped
// Handler verbatim.
//
// The payload here is a single byte equal to byte(request.KindCancelAll), the
// exact value an envelope decoder would read as "cancel every pending
// request": an implementation that keyed envelope routing off anything other
// than KindLong-with-mtv-clear would cancel the pending ticket instead of
// passing the byte through.
//
//fusa:test REQ-TC18-006
func TestTC18_006_KindLongWithMTVIsNotAConditionalEnvelope(t *testing.T) {
	stream := tc18Stream(0x06)
	handler := &tc18StubHandler{body: []byte{0xEE}}
	dispatcher := request.NewDispatcher(handler, 1, request.NewSequencer(), nil)

	// A genuine conditional envelope (KindLong, mtv clear) that stays
	// pending: its trigger source has no registered pump, so it can never
	// become ready on its own and can only leave the queue by cancellation.
	pending := acf.Message{
		Kind:           acf.KindLong,
		ByteBusID:      1,
		TransactionNum: 1,
		Body:           request.EncodeTriggered(2, acf.FlagWrite, []byte{0x01}),
	}
	pendingID, err := dispatcher.Submit(stream, pending)
	if err != nil {
		t.Fatalf("Submit(triggered envelope): %v", err)
	}

	payload := []byte{byte(request.KindCancelAll)}
	plain := acf.Message{
		Kind:           acf.KindLong,
		MTV:            true,
		ByteBusID:      1,
		TransactionNum: 2,
		Control:        acf.FlagWrite,
		Body:           payload,
	}
	plainID, err := dispatcher.Submit(stream, plain)
	if err != nil {
		t.Fatalf("Submit(KindLong with MTV set): %v", err)
	}
	dispatcher.Pump(0)

	if got := handler.callCount(); got != 1 {
		t.Fatalf("wrapped Handler call count = %d, want 1 (the mtv-set message must be dispatched as a standard request)", got)
	}
	if got := handler.last().Body; !bytes.Equal(got, payload) {
		t.Errorf("Handler received Body = % X, want % X (verbatim byte_msg_payload)", got, payload)
	}

	if _, err := dispatcher.Response(plainID); err != nil {
		t.Errorf("Response(mtv-set ticket) err = %v, want nil", err)
	}

	// Nothing was cancelled: the pending ticket is untouched.
	state, ok := dispatcher.StateOf(pendingID)
	if !ok {
		t.Fatal("pending ticket is no longer known to the Dispatcher")
	}
	if state == request.StateFinalized {
		t.Errorf("pending ticket state = %v, want a non-finalized state — the mtv-set message must not act as a cancellation request", state)
	}
	if _, err := dispatcher.Response(pendingID); !errors.Is(err, request.ErrPending) {
		t.Errorf("Response(pending ticket) err = %v, want ErrPending (not ErrTicketCancelled)", err)
	}
	if got := dispatcher.Pending(); got != 1 {
		t.Errorf("Pending() = %d, want 1", got)
	}
}

// ── REQ-TC18-044 ────────────────────────────────────────────────────────────

// TestTC18_044_ResponseControlBits pins TC18 §11.3 Table 15
// (TC18.txt:1881-1885): every response sets rsp, repeats its request's op
// bit, and leaves hs and cs at zero. The assertion is made both on the
// decoded acf.Message and on the encoded octets, where §11.2.1 places hs at
// bit 1 and cs at bit 0 of the descriptor's second word, and op/rsp at bits 7
// and 6 of that word's third octet.
//
//fusa:test REQ-TC18-044
func TestTC18_044_ResponseControlBits(t *testing.T) {
	root := tc18Stream(0x44)
	srv := tc18RootServer(t, root)
	ep := tc18GPIOEndpoint(t, srv, root, 1)

	cases := []struct {
		name    string
		control acf.ControlFlags
		body    []byte
		wantOp  byte // encoded op bit: 0 for a read, 1 for a write
	}{
		{name: "read request", control: acf.FlagRead, wantOp: 0},
		{name: "write request", control: acf.FlagWrite, body: gpio.EncodeWriteRequest(0b0011), wantOp: 1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := acf.Message{
				Kind:           acf.KindShort,
				ByteBusID:      1,
				TransactionNum: 5,
				Control:        tc.control,
				Body:           tc.body,
			}
			resp, err := ep.HandleRequest(root, req)
			if err != nil {
				t.Fatalf("HandleRequest: %v", err)
			}

			if !resp.Control.Has(acf.FlagResponse) {
				t.Errorf("response Control = %#08b, want FlagResponse set", resp.Control)
			}
			const opBits = acf.FlagRead | acf.FlagWrite
			if got, want := resp.Control&opBits, req.Control&opBits; got != want {
				t.Errorf("response op bits = %#08b, want %#08b (echoed from the request)", got, want)
			}
			if resp.HS {
				t.Error("response HS = true, want false")
			}
			if resp.CS {
				t.Error("response CS = true, want false")
			}

			raw, err := acf.EncodeMessage(resp)
			if err != nil {
				t.Fatalf("EncodeMessage(response): %v", err)
			}
			// KindShort has no message_timestamp slot, so the descriptor's
			// second word starts at octet 4.
			if got := raw[4] & 0x03; got != 0 {
				t.Errorf("encoded hs/cs bits = %#02b, want 0b00", got)
			}
			if got := (raw[6] >> 6) & 0x01; got != 1 {
				t.Errorf("encoded rsp bit = %d, want 1", got)
			}
			if got := (raw[6] >> 7) & 0x01; got != tc.wantOp {
				t.Errorf("encoded op bit = %d, want %d", got, tc.wantOp)
			}
		})
	}
}

// ── REQ-TC18-045 ────────────────────────────────────────────────────────────

// TestTC18_045_ErrorResponseBodyCarriesValidErrorCode pins TC18 §11.3.4 and
// §12.9.6: an error response's payload leads with one of Table 27's defined
// error codes. Every distinct internal failure this module can surface from
// the routing path is driven through it here, and the leading byte of each
// resulting body is required to decode to a udp.ErrorCode that Valid accepts
// — never a Go error's free text, and never an out-of-table numeric value.
//
//fusa:test REQ-TC18-045
func TestTC18_045_ErrorResponseBodyCarriesValidErrorCode(t *testing.T) {
	root := tc18Stream(0x45)

	cases := []struct {
		name string
		// handlerErr nil means "route to an address with no registered
		// Handler at all", i.e. udp.ErrUnknownEndpoint from the Router
		// itself rather than from a Handler.
		handlerErr error
	}{
		{name: "unknown endpoint", handlerErr: nil},
		{name: "invalid segment count", handlerErr: request.ErrInvalidSegmentCount},
		{name: "chained segment failed", handlerErr: request.ErrChainedSegmentFailed},
		{name: "unknown ticket", handlerErr: request.ErrUnknownTicket},
		{name: "ticket cancelled", handlerErr: request.ErrTicketCancelled},
		{name: "purged by watchdog", handlerErr: request.ErrPurgedByWatchdog},
		{name: "safe state not configured", handlerErr: request.ErrSafeStateNotConfigured},
		{name: "crc mismatch", handlerErr: e2e.ErrCRCMismatch},
		{name: "access denied", handlerErr: regmap.ErrAccessDenied},
		{name: "not root client", handlerErr: regmap.ErrNotRootClient},
		{name: "register locked", handlerErr: regmap.ErrRegisterLocked},
		{name: "unknown regmap endpoint", handlerErr: regmap.ErrUnknownEndpoint},
		{name: "pwm signal lost", handlerErr: pwm.ErrSignalLost},
		{name: "unclassified failure", handlerErr: errors.New("something this module cannot classify")},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv := tc18RootServer(t, root)
			router := udp.NewRouter(udp.NewEP0Handler(srv), true)

			addr := avtp.ByteBusID(9) // nothing registered here
			if tc.handlerErr != nil {
				addr = 1
				if err := router.Register(addr, &tc18StubHandler{err: tc.handlerErr}); err != nil {
					t.Fatalf("Register: %v", err)
				}
			}

			req := acf.Message{Kind: acf.KindShort, ByteBusID: addr, TransactionNum: 3, Control: acf.FlagRead}
			resp, shouldReply := router.Route(tc18UntimedHeader(root), req)
			if !shouldReply {
				t.Fatal("Route reported no reply; want a wire-level error response")
			}
			if !resp.Control.Has(acf.FlagError) {
				t.Fatalf("response Control = %#08b, want FlagError set", resp.Control)
			}
			if len(resp.Body) == 0 {
				t.Fatal("error response Body is empty; want a leading Table 27 error code")
			}

			code, _, err := udp.DecodeErrorBody(resp.Body)
			if err != nil {
				t.Fatalf("DecodeErrorBody: %v", err)
			}
			if code != udp.ErrorCode(resp.Body[0]) {
				t.Errorf("DecodeErrorBody code = %d, want the body's leading byte %d", code, resp.Body[0])
			}
			if !code.Valid() {
				t.Errorf("error code %d (%q) is not one of Table 27's seventeen defined codes", code, code)
			}
		})
	}
}

// ── REQ-TC18-051 ────────────────────────────────────────────────────────────

// TestTC18_051_ServerRepliesWithUntimedNTSCFHeader pins TC18 §11.4.3
// (TC18.txt:1979-1980): the RC Server always answers in an untimed (NTSCF)
// AVTPDU, never a presentation-timestamped (TSCF) one. The reply is captured
// off a real socket so the assertion lands on the octets the server actually
// transmitted: the subtype octet must be exactly 0x82, and the decoded header
// must report Timed false.
//
//fusa:test REQ-TC18-051
func TestTC18_051_ServerRepliesWithUntimedNTSCFHeader(t *testing.T) {
	root := tc18Stream(0x51)
	srv := tc18RootServer(t, root)
	router := udp.NewRouter(udp.NewEP0Handler(srv), true)

	us, err := udp.NewServer(tc18Stream(0xF1), "127.0.0.1:0", router)
	if err != nil {
		t.Fatalf("udp.NewServer: %v", err)
	}
	defer func() { _ = us.Close() }()

	conn, err := net.DialUDP("udp", nil, us.Addr())
	if err != nil {
		t.Fatalf("DialUDP: %v", err)
	}
	defer func() { _ = conn.Close() }()

	req := acf.Message{Kind: acf.KindShort, ByteBusID: regmap.EP0, TransactionNum: 7, Control: acf.FlagRead}
	frame, err := acf.EncodeFrame(tc18UntimedHeader(root), req)
	if err != nil {
		t.Fatalf("EncodeFrame: %v", err)
	}
	encapSeq := make([]byte, udp.AnnexJEncapSeqLen)
	binary.BigEndian.PutUint32(encapSeq, 1)
	if _, err := conn.Write(append(encapSeq, frame...)); err != nil {
		t.Fatalf("Write: %v", err)
	}

	_ = conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	buf := make([]byte, udp.MaxFrameLen)
	n, err := conn.Read(buf)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if n <= udp.AnnexJEncapSeqLen {
		t.Fatalf("reply too short to hold an AVTPDU: %d bytes", n)
	}
	avtpdu := buf[udp.AnnexJEncapSeqLen:n]

	if avtp.SubtypeNTSCF != 0x82 {
		t.Fatalf("avtp.SubtypeNTSCF = %#02x, want 0x82 (TC18 §11.1 Figure 6)", avtp.SubtypeNTSCF)
	}
	if avtpdu[0] != avtp.SubtypeNTSCF {
		t.Errorf("reply subtype octet = %#02x, want %#02x (NTSCF); %#02x is TSCF", avtpdu[0], avtp.SubtypeNTSCF, avtp.SubtypeTSCF)
	}
	hdr, _, err := avtp.DecodeHeader(avtpdu)
	if err != nil {
		t.Fatalf("DecodeHeader(reply): %v", err)
	}
	if hdr.Timed {
		t.Error("reply header Timed = true, want false — the RC Server never replies with a TSCF header")
	}
}

// ── REQ-TC18-055 ────────────────────────────────────────────────────────────

// TestTC18_055_MandatoryServerFeatures pins TC18 §12.2 (TC18.txt:2014-2019):
// a conformant RC Server implements NTSCF header processing, standard request
// handling, an endpoint at EP0, and the clear-all cancellation request. All
// three request shapes are exercised against one server here, in sequence.
//
//fusa:test REQ-TC18-055
func TestTC18_055_MandatoryServerFeatures(t *testing.T) {
	root := tc18Stream(0x55)
	srv := tc18RootServer(t, root)
	router := udp.NewRouter(udp.NewEP0Handler(srv), true)

	endpoint := &tc18StubHandler{body: []byte{0x77}}
	if err := router.Register(1, endpoint); err != nil {
		t.Fatalf("Register: %v", err)
	}
	hdr := tc18UntimedHeader(root)

	// (a) An NTSCF-framed standard read addressed to EP0 is answered.
	ep0Req := acf.Message{Kind: acf.KindShort, ByteBusID: regmap.EP0, TransactionNum: 1, Control: acf.FlagRead}
	ep0Resp, shouldReply := router.Route(hdr, ep0Req)
	if !shouldReply {
		t.Fatal("EP0 read: Route reported no reply; EP0 is mandatory")
	}
	if !ep0Resp.Control.Has(acf.FlagResponse) || ep0Resp.Control.Has(acf.FlagError) {
		t.Fatalf("EP0 read Control = %#08b, want FlagResponse set and FlagError clear", ep0Resp.Control)
	}
	if len(ep0Resp.Body) == 0 {
		t.Error("EP0 read Body is empty; want the encoded register map")
	}
	if _, err := regmap.DecodeRegisterMap(ep0Resp.Body); err != nil {
		t.Errorf("EP0 read body is not a decodable register map: %v", err)
	}

	// (b) An NTSCF-framed standard write addressed to a non-EP0 endpoint is
	// answered.
	epReq := acf.Message{Kind: acf.KindShort, ByteBusID: 1, TransactionNum: 2, Control: acf.FlagWrite, Body: []byte{0x01}}
	epResp, shouldReply := router.Route(hdr, epReq)
	if !shouldReply {
		t.Fatal("endpoint write: Route reported no reply")
	}
	if !epResp.Control.Has(acf.FlagResponse) || epResp.Control.Has(acf.FlagError) {
		t.Fatalf("endpoint write Control = %#08b, want FlagResponse set and FlagError clear", epResp.Control)
	}
	if epResp.TransactionNum != epReq.TransactionNum {
		t.Errorf("endpoint write response TransactionNum = %d, want %d", epResp.TransactionNum, epReq.TransactionNum)
	}
	if endpoint.callCount() != 1 {
		t.Errorf("endpoint Handler call count = %d, want 1", endpoint.callCount())
	}

	// (c) A clear-all cancellation request retires every pending request.
	dispatcher := request.NewDispatcher(endpoint, 1, request.NewSequencer(), nil)
	victim := acf.Message{
		Kind:           acf.KindLong,
		ByteBusID:      1,
		TransactionNum: 3,
		Body:           request.EncodeTriggered(2, acf.FlagWrite, []byte{0x01}),
	}
	victimID, err := dispatcher.Submit(root, victim)
	if err != nil {
		t.Fatalf("Submit(triggered): %v", err)
	}
	cancelAll := acf.Message{
		Kind:           acf.KindLong,
		ByteBusID:      1,
		TransactionNum: 4,
		Body:           request.EncodeCancelAll(),
	}
	cancelID, err := dispatcher.Submit(root, cancelAll)
	if err != nil {
		t.Fatalf("Submit(cancel-all): %v", err)
	}
	dispatcher.Pump(0)

	cancelResp, err := dispatcher.Response(cancelID)
	if err != nil {
		t.Fatalf("Response(cancel-all): %v", err)
	}
	cleared, err := request.DecodeCancelResponse(cancelResp.Body)
	if err != nil {
		t.Fatalf("DecodeCancelResponse: %v", err)
	}
	if cleared != 1 {
		t.Errorf("clear-all cancelled %d requests, want 1", cleared)
	}
	if _, err := dispatcher.Response(victimID); !errors.Is(err, request.ErrTicketCancelled) {
		t.Errorf("Response(cancelled ticket) err = %v, want ErrTicketCancelled", err)
	}
	if got := dispatcher.Pending(); got != 0 {
		t.Errorf("Pending() after clear-all = %d, want 0", got)
	}
}

// ── REQ-TC18-080 ────────────────────────────────────────────────────────────

// TestTC18_080_ACFGBBDiscoveryRequestIsDroppedWithoutReply pins TC18 §12.6.1
// Table 16 (TC18.txt:2367): a discovery request framed as ACF_GBB
// (acf.KindLong) is dropped without further response. Table 16 says "dropped",
// not "answered with an error", so the assertion is specifically that
// Router.Route reports no reply at all — an error response would be just as
// non-conformant as a successful one.
//
//fusa:test REQ-TC18-080
func TestTC18_080_ACFGBBDiscoveryRequestIsDroppedWithoutReply(t *testing.T) {
	root := tc18Stream(0x80)
	srv := tc18RootServer(t, root)
	router := udp.NewRouter(udp.NewEP0Handler(srv), true)
	hdr := tc18UntimedHeader(root)

	gbb := acf.Message{Kind: acf.KindLong, ByteBusID: regmap.EP0, TransactionNum: 1, Control: acf.FlagRead}
	resp, shouldReply := router.Route(hdr, gbb)
	if shouldReply {
		t.Errorf("Route(ACF_GBB discovery) shouldReply = true (Control %#08b, Body % X), want false — Table 16 requires the request to be dropped without further response",
			resp.Control, resp.Body)
	}
	if !reflect.DeepEqual(resp, acf.Message{}) {
		t.Errorf("Route(ACF_GBB discovery) response = %+v, want the zero Message", resp)
	}

	// The server layer names the condition explicitly, and returns no
	// register-map bytes for it.
	body, err := srv.HandleDiscoveryRequest(hdr, true)
	if !errors.Is(err, discovery.ErrDiscoveryRequestIsACFGBB) {
		t.Errorf("HandleDiscoveryRequest(isACFGBB=true) err = %v, want ErrDiscoveryRequestIsACFGBB", err)
	}
	if body != nil {
		t.Errorf("HandleDiscoveryRequest(isACFGBB=true) body = % X, want nil", body)
	}

	// Control: the identical request framed as ACF_ABB is answered normally,
	// so the drop above is attributable to the framing and nothing else.
	abb := acf.Message{Kind: acf.KindShort, ByteBusID: regmap.EP0, TransactionNum: 1, Control: acf.FlagRead}
	abbResp, shouldReply := router.Route(hdr, abb)
	if !shouldReply {
		t.Fatal("Route(ACF_ABB discovery) shouldReply = false, want true")
	}
	if abbResp.Control.Has(acf.FlagError) {
		t.Errorf("Route(ACF_ABB discovery) Control = %#08b, want FlagError clear", abbResp.Control)
	}
}

// ── REQ-TC18-117 ────────────────────────────────────────────────────────────

// TestTC18_117_UnknownSubtypeIsRejected pins TC18 §12.8.2 (TC18.txt:3170): a
// received frame that is not an IEEE 1722 AVTPDU of a subtype RCP uses is
// discarded rather than misinterpreted. avtp.DecodeHeader recognizes exactly
// two subtype octets — 0x82 (NTSCF) and 0x05 (TSCF) — and must report
// avtp.ErrUnknownSubtype for anything else, including 0x83, the value this
// module itself wrongly used for TSCF through v8.0.0.
//
//fusa:test REQ-TC18-117
func TestTC18_117_UnknownSubtypeIsRejected(t *testing.T) {
	// Long enough for either header variant, and zero everywhere else so the
	// only thing that can be rejected is the subtype octet itself.
	newBuf := func(subtype byte) []byte {
		b := make([]byte, 24)
		b[0] = subtype
		return b
	}

	for _, subtype := range []byte{0x83, 0x00, 0x01, 0x81, 0xFF} {
		if _, _, err := avtp.DecodeHeader(newBuf(subtype)); !errors.Is(err, avtp.ErrUnknownSubtype) {
			t.Errorf("DecodeHeader(subtype %#02x) err = %v, want ErrUnknownSubtype", subtype, err)
		}
	}

	// The two recognized subtypes must not be rejected as unknown, or the
	// assertion above would be vacuous.
	for _, subtype := range []byte{avtp.SubtypeNTSCF, avtp.SubtypeTSCF} {
		if _, _, err := avtp.DecodeHeader(newBuf(subtype)); err != nil {
			t.Errorf("DecodeHeader(subtype %#02x) err = %v, want nil", subtype, err)
		}
	}
}

// ── REQ-TC18-121 ────────────────────────────────────────────────────────────

// TestTC18_121_MultipleMessagesPerFrameAreRoutedIndividually pins TC18
// §12.9.1.1 (TC18.txt:3221-3223): "An RC Server shall support to handle
// multiple requests in one frame and check each of them individually".
// A frame carrying three independently-addressed ACF messages must decode to
// three messages in wire order, each reaching its own endpoint's Handler
// exactly once, and produce three responses.
//
//fusa:test REQ-TC18-121
func TestTC18_121_MultipleMessagesPerFrameAreRoutedIndividually(t *testing.T) {
	root := tc18Stream(0x21)
	srv := tc18RootServer(t, root)
	router := udp.NewRouter(udp.NewEP0Handler(srv), true)

	handlers := map[avtp.ByteBusID]*tc18StubHandler{
		1: {body: []byte{0xA1}},
		2: {body: []byte{0xA2}},
		3: {body: []byte{0xA3}},
	}
	for addr, h := range handlers {
		if err := router.Register(addr, h); err != nil {
			t.Fatalf("Register(%d): %v", addr, err)
		}
	}

	// Distinct body lengths so the decoded order cannot be an accident of
	// every message occupying the same number of quadlets.
	reqs := []acf.Message{
		{Kind: acf.KindShort, ByteBusID: 1, TransactionNum: 11, Control: acf.FlagWrite, Body: []byte{0x01}},
		{Kind: acf.KindShort, ByteBusID: 2, TransactionNum: 12, Control: acf.FlagWrite, Body: []byte{0x02, 0x02}},
		{Kind: acf.KindShort, ByteBusID: 3, TransactionNum: 13, Control: acf.FlagWrite, Body: []byte{0x03, 0x03, 0x03}},
	}
	hdr := tc18UntimedHeader(root)
	raw, err := acf.EncodeFrame(hdr, reqs...)
	if err != nil {
		t.Fatalf("EncodeFrame: %v", err)
	}
	frame, err := acf.DecodeFrame(raw)
	if err != nil {
		t.Fatalf("DecodeFrame: %v", err)
	}
	if len(frame.Messages) != 3 {
		t.Fatalf("len(frame.Messages) = %d, want 3", len(frame.Messages))
	}
	for i, want := range reqs {
		got := frame.Messages[i]
		if got.ByteBusID != want.ByteBusID || got.TransactionNum != want.TransactionNum {
			t.Errorf("frame.Messages[%d] = (byte_bus_id %d, transaction_num %d), want (%d, %d) — wire order must be preserved",
				i, got.ByteBusID, got.TransactionNum, want.ByteBusID, want.TransactionNum)
		}
		if !bytes.Equal(got.Body, want.Body) {
			t.Errorf("frame.Messages[%d].Body = % X, want % X", i, got.Body, want.Body)
		}
	}

	var responses []acf.Message
	for _, msg := range frame.Messages {
		resp, shouldReply := router.Route(frame.Header, msg)
		if !shouldReply {
			t.Fatalf("Route(byte_bus_id %d) reported no reply", msg.ByteBusID)
		}
		responses = append(responses, resp)
	}
	if len(responses) != 3 {
		t.Fatalf("routed %d responses, want 3", len(responses))
	}
	for i, want := range reqs {
		if responses[i].TransactionNum != want.TransactionNum {
			t.Errorf("responses[%d].TransactionNum = %d, want %d", i, responses[i].TransactionNum, want.TransactionNum)
		}
		if responses[i].Control.Has(acf.FlagError) {
			t.Errorf("responses[%d] carries FlagError, want a successful response", i)
		}
	}
	for addr, h := range handlers {
		if got := h.callCount(); got != 1 {
			t.Errorf("handler at byte_bus_id %d call count = %d, want 1 (each message checked and routed individually)", addr, got)
		}
	}
}

// ── REQ-TC18-122 ────────────────────────────────────────────────────────────

// TestTC18_122_OneHeaderGovernsEveryMessageInTheFrame pins TC18 §12.9.1.1
// (TC18.txt:3224-3226): an RCP frame carries exactly one AVTPDU header, so
// its timing disposition applies uniformly to every ACF message inside it —
// a frame cannot mix timestamped and untimestamped requests.
//
// The structural half is checked by reflection (acf.Frame has exactly one
// avtp.Header field); the behavioural half by routing all three messages of a
// TSCF frame through a Router with no time-synchronization support, where the
// single header's disposition must drop every one of them, and then through a
// Router that does support it, where the same header must admit every one.
//
//fusa:test REQ-TC18-122
func TestTC18_122_OneHeaderGovernsEveryMessageInTheFrame(t *testing.T) {
	frameType := reflect.TypeOf(acf.Frame{})
	headerType := reflect.TypeOf(avtp.Header{})
	headers := 0
	for i := 0; i < frameType.NumField(); i++ {
		if frameType.Field(i).Type == headerType {
			headers++
		}
	}
	if headers != 1 {
		t.Errorf("acf.Frame has %d avtp.Header fields, want exactly 1", headers)
	}

	const timestamp uint32 = 0xDEADBEEF
	root := tc18Stream(0x22)
	hdr := avtp.Header{
		Timed:           true,
		StreamIDValid:   true,
		StreamID:        root,
		Timestamp:       timestamp,
		TimestampStatus: avtp.TimestampValid,
	}
	reqs := []acf.Message{
		{Kind: acf.KindShort, ByteBusID: 1, TransactionNum: 31, Control: acf.FlagRead},
		{Kind: acf.KindShort, ByteBusID: 2, TransactionNum: 32, Control: acf.FlagRead},
		{Kind: acf.KindShort, ByteBusID: 3, TransactionNum: 33, Control: acf.FlagRead},
	}
	raw, err := acf.EncodeFrame(hdr, reqs...)
	if err != nil {
		t.Fatalf("EncodeFrame: %v", err)
	}
	frame, err := acf.DecodeFrame(raw)
	if err != nil {
		t.Fatalf("DecodeFrame: %v", err)
	}
	if len(frame.Messages) != 3 {
		t.Fatalf("len(frame.Messages) = %d, want 3", len(frame.Messages))
	}
	if !frame.Header.Timed {
		t.Error("frame.Header.Timed = false, want true")
	}
	if frame.Header.Timestamp != timestamp {
		t.Errorf("frame.Header.Timestamp = %#08x, want %#08x", frame.Header.Timestamp, timestamp)
	}
	if frame.Header.TimestampStatus != avtp.TimestampValid {
		t.Errorf("frame.Header.TimestampStatus = %v, want TimestampValid", frame.Header.TimestampStatus)
	}

	newRouter := func(timeSync bool) *udp.Router {
		srv := tc18RootServer(t, tc18Stream(0xC2))
		r := udp.NewRouter(udp.NewEP0Handler(srv), timeSync)
		for _, addr := range []avtp.ByteBusID{1, 2, 3} {
			if err := r.Register(addr, &tc18StubHandler{body: []byte{0x01}}); err != nil {
				t.Fatalf("Register(%d): %v", addr, err)
			}
		}
		return r
	}

	noSync := newRouter(false)
	for i, msg := range frame.Messages {
		if _, shouldReply := noSync.Route(frame.Header, msg); shouldReply {
			t.Errorf("message %d: shouldReply = true with no time-sync support, want false — the frame's single header governs every message it carries", i)
		}
	}
	withSync := newRouter(true)
	for i, msg := range frame.Messages {
		if _, shouldReply := withSync.Route(frame.Header, msg); !shouldReply {
			t.Errorf("message %d: shouldReply = false with time-sync support, want true", i)
		}
	}
}

// ── REQ-TC18-131 ────────────────────────────────────────────────────────────

// TestTC18_131_ErrorCodeEnumerationMatchesTable27 pins TC18 §12.9.6 Table 27
// (TC18.txt:3414-3446): the error-response enumeration has exactly seventeen
// entries at fixed numeric values, from UNSUPPORTED_CMD = 1 through
// CHAIN_ERROR = 17. The numeric value of each constant is asserted literally
// here — not derived from the constant itself — and Valid is swept across the
// whole byte range so 0 and 18..255 are rejected.
//
//fusa:test REQ-TC18-131
func TestTC18_131_ErrorCodeEnumerationMatchesTable27(t *testing.T) {
	table := []struct {
		specName string
		code     udp.ErrorCode
		want     uint8
	}{
		{"UNSUPPORTED_CMD", udp.ErrorCodeUnsupportedCommand, 1},
		{"SEQUENCER_NOT_KNOWN", udp.ErrorCodeSequencerNotKnown, 2},
		{"UNAUTHORIZED_ACCESS", udp.ErrorCodeUnauthorizedAccess, 3},
		{"LOCKED_MEM_ACCESS", udp.ErrorCodeLockedMemAccess, 4},
		{"REQUEST_CANCELED", udp.ErrorCodeRequestCancelled, 5},
		{"REQUEST_NOT_FOUND", udp.ErrorCodeRequestNotFound, 6},
		{"EP_ERROR", udp.ErrorCodeEPError, 7},
		{"EP_NOT_FOUND", udp.ErrorCodeEPNotFound, 8},
		{"PWM_IN_NO_SIGNAL", udp.ErrorCodePWMInNoSignal, 9},
		{"REQ_storage_OVFL", udp.ErrorCodeRequestStorageOverflow, 10},
		{"REQUEST_REJECTED", udp.ErrorCodeRequestRejected, 11},
		{"POCI_FAILURE", udp.ErrorCodePOCIFailure, 12},
		{"PRESENTATION_TIME_TOO_FAR", udp.ErrorCodePresentationTimeTooFarInFuture, 13},
		{"GPTP_FAIL", udp.ErrorCodeGPTPFailure, 14},
		{"INVALID_PARAMETER", udp.ErrorCodeInvalidParameter, 15},
		{"CHAIN_ABORTED", udp.ErrorCodeChainAborted, 16},
		{"CHAIN_ERROR", udp.ErrorCodeChainError, 17},
	}
	if len(table) != 17 {
		t.Fatalf("this test enumerates %d codes, want 17", len(table))
	}

	seen := make(map[uint8]string, len(table))
	for _, tc := range table {
		if uint8(tc.code) != tc.want {
			t.Errorf("%s = %d, want %d", tc.specName, uint8(tc.code), tc.want)
		}
		if prev, dup := seen[tc.want]; dup {
			t.Errorf("value %d assigned to both %s and %s", tc.want, prev, tc.specName)
		}
		seen[tc.want] = tc.specName
		if !tc.code.Valid() {
			t.Errorf("%s (%d).Valid() = false, want true", tc.specName, uint8(tc.code))
		}
	}
	for v := uint8(1); v <= 17; v++ {
		if _, ok := seen[v]; !ok {
			t.Errorf("Table 27 value %d has no corresponding udp.ErrorCode constant", v)
		}
	}

	for v := 0; v < 256; v++ {
		want := v >= 1 && v <= 17
		if got := udp.ErrorCode(v).Valid(); got != want {
			t.Errorf("udp.ErrorCode(%d).Valid() = %t, want %t", v, got, want)
		}
	}
}

// ── REQ-TC18-135 ────────────────────────────────────────────────────────────

// TestTC18_135_GrantedStreamWritesOnlyTheFunctionalBlock pins TC18 §13.1
// (TC18.txt:3516-3518): a non-root stream holding a grant for an endpoint may
// configure that endpoint's functional block, but the generic block
// (ep_type/ep_used and the address itself) stays server-owned. The generic
// block is the leading three octets of an endpoint's encoded registers
// (regmap's per-endpoint layout: generic block, then the length-prefixed
// functional block), and must be byte-identical either side of a successful
// WriteFunctional by that stream.
//
//fusa:test REQ-TC18-135
func TestTC18_135_GrantedStreamWritesOnlyTheFunctionalBlock(t *testing.T) {
	const addr = avtp.ByteBusID(4)
	const genericLen = 3 // address(1) + type(1) + enabled(1)

	root := tc18Stream(0x35)
	granted := tc18Stream(0xB5)
	srv := tc18RootServer(t, root)
	if err := srv.AddEndpoint(root, addr, gpio.EndpointType); err != nil {
		t.Fatalf("AddEndpoint: %v", err)
	}
	srv.Grant(granted, addr)

	before, err := srv.ReadEndpoint(granted, addr)
	if err != nil {
		t.Fatalf("ReadEndpoint(before): %v", err)
	}
	if len(before) < genericLen {
		t.Fatalf("encoded endpoint registers are %d bytes, too short to hold a generic block", len(before))
	}
	genericBefore := append([]byte(nil), before[:genericLen]...)
	if genericBefore[0] != byte(addr) {
		t.Errorf("generic block address octet = %#02x, want %#02x", genericBefore[0], byte(addr))
	}
	if genericBefore[1] != byte(gpio.EndpointType) {
		t.Errorf("generic block ep_type octet = %d, want %d (GPIO)", genericBefore[1], byte(gpio.EndpointType))
	}

	functional := gpio.EncodeConfig(gpio.Config{PinCount: 8, Direction: 0b1111_0000})
	if err := srv.WriteFunctional(granted, addr, functional); err != nil {
		t.Fatalf("WriteFunctional by granted non-root stream: %v", err)
	}

	after, err := srv.ReadEndpoint(granted, addr)
	if err != nil {
		t.Fatalf("ReadEndpoint(after): %v", err)
	}
	if !bytes.Equal(after[:genericLen], genericBefore) {
		t.Errorf("generic block changed across WriteFunctional: % X -> % X, want byte-identical",
			genericBefore, after[:genericLen])
	}
	// The write did land — otherwise the assertion above would hold trivially.
	if bytes.Equal(after, before) {
		t.Fatal("endpoint registers unchanged across WriteFunctional; the write did not take effect")
	}
	if !bytes.Contains(after[genericLen:], functional) {
		t.Errorf("functional block after write = % X, want it to contain % X", after[genericLen:], functional)
	}

	// The only whole-map write path is root-client-only, so a granted stream
	// has no route to the generic block through it either.
	if err := srv.WriteEP0(granted, regmap.EncodeRegisterMap(regmap.NewRegisterMap())); !errors.Is(err, regmap.ErrNotRootClient) {
		t.Errorf("WriteEP0 by granted non-root stream err = %v, want ErrNotRootClient", err)
	}
}

// ── REQ-TC18-146 ────────────────────────────────────────────────────────────

// tc18Reflect32 reverses the 32 bits of v — the refin/refout step CRC32P4's
// Table 31 parameter set requires. Derived here from the specification's
// parameters rather than imported from the e2e package, so the assertions
// below cross-check that package rather than restating it.
func tc18Reflect32(v uint32) uint32 {
	var r uint32
	for i := 0; i < 32; i++ {
		r = (r << 1) | (v & 1)
		v >>= 1
	}
	return r
}

// TestTC18_146_ComputeUsesCRC32P4Parameters pins TC18 §13.6 Table 31
// (TC18.txt:3792-3807): the safe-point checksum is CRC32P4 — polynomial
// 0xF4ACFB13, initial value 0xFFFFFFFF, both input and output reflected,
// final XOR 0xFFFFFFFF — and specifically not the IEEE 802.3 CRC-32.
//
// e2e/crc32p4_test.go already asserts the published CRC-32/AUTOSAR check
// value 0x1697D06A against the package's own internal table, so this test
// does not repeat that in-package assertion. It instead ties the check value
// to the exported e2e.Compute API: because Compute writes the message Body
// last over its covered-field layout, extending a message's Body by the
// standard check string "123456789" must advance Compute's result exactly as
// a locally built CRC32P4 table says it should — and must *not* advance it
// the way an IEEE 802.3 table would.
//
//fusa:test REQ-TC18-146
func TestTC18_146_ComputeUsesCRC32P4Parameters(t *testing.T) {
	// Table 31's polynomial in its normal (non-reflected) form.
	const normalPoly uint32 = 0xF4ACFB13
	const reflectedPoly uint32 = 0xC8DF352F
	if got := tc18Reflect32(normalPoly); got != reflectedPoly {
		t.Fatalf("reflect32(%#08x) = %#08x, want %#08x", normalPoly, got, reflectedPoly)
	}
	table := crc32.MakeTable(reflectedPoly)

	// crc32.Update over a reflected-polynomial table already applies Table
	// 31's init (0xFFFFFFFF) and final XOR (0xFFFFFFFF), so this is the full
	// parameter set, checked against CRC-32/AUTOSAR's published check value.
	const check = "123456789"
	const wantCheck uint32 = 0x1697D06A
	if got := crc32.Update(0, table, []byte(check)); got != wantCheck {
		t.Fatalf("locally built CRC32P4(%q) = %#08x, want %#08x — the local reference itself is wrong", check, got, wantCheck)
	}

	stream := tc18Stream(0x46)
	base := acf.Message{
		Kind:           acf.KindShort,
		ByteBusID:      3,
		TransactionNum: 9,
		Control:        acf.FlagWrite,
		Body:           []byte{0xDE, 0xAD},
	}
	extended := base
	extended.Body = append(append([]byte(nil), base.Body...), []byte(check)...)

	baseCRC := e2e.Compute(stream, base)
	gotExtended := e2e.Compute(stream, extended)

	if want := crc32.Update(baseCRC, table, []byte(check)); gotExtended != want {
		t.Errorf("e2e.Compute over a body extended by %q = %#08x, want %#08x — Compute does not implement Table 31's CRC32P4 parameters",
			check, gotExtended, want)
	}
	if ieee := crc32.Update(baseCRC, crc32.IEEETable, []byte(check)); gotExtended == ieee {
		t.Errorf("e2e.Compute advanced exactly as an IEEE 802.3 CRC-32 would (%#08x); Table 31 mandates polynomial %#08x, not IEEE's",
			ieee, normalPoly)
	}
}

// ── REQ-TC18-150 ────────────────────────────────────────────────────────────

// TestTC18_150_SafePointIsPerMessageNotPerFrame pins TC18 §13.6
// (TC18.txt:3789-3791): in an AVTPDU carrying several ACF messages the CRC32
// is computed per message, so corrupting one message's payload invalidates
// only that message. The corruption is applied to the encoded frame at a
// literal byte offset — the first payload octet of the middle message — so
// the test really does exercise the framing, not just three independent
// Verify calls.
//
//fusa:test REQ-TC18-150
func TestTC18_150_SafePointIsPerMessageNotPerFrame(t *testing.T) {
	stream := tc18Stream(0x50)
	payloads := [][]byte{
		{0x11, 0x12, 0x13, 0x14},
		{0x21, 0x22, 0x23, 0x24},
		{0x31, 0x32, 0x33, 0x34},
	}
	msgs := make([]acf.Message, len(payloads))
	for i, payload := range payloads {
		msgs[i] = e2e.Protect(stream, acf.Message{
			Kind:           acf.KindShort,
			ByteBusID:      avtp.ByteBusID(i + 1),
			TransactionNum: avtp.TransactionNum(40 + i),
			Control:        acf.FlagWrite,
			Body:           payload,
		})
	}

	raw, err := acf.EncodeFrame(tc18UntimedHeader(stream), msgs...)
	if err != nil {
		t.Fatalf("EncodeFrame: %v", err)
	}
	// NTSCF header (12) + three messages of descriptor (8) + protected body
	// (4 payload + 0 pad + 4 CRC = 8) = 12 + 3*16.
	const (
		headerLen     = 12
		messageLen    = 16
		descriptorLen = 8
	)
	if len(raw) != headerLen+3*messageLen {
		t.Fatalf("encoded frame is %d bytes, want %d — the byte offsets below assume this layout",
			len(raw), headerLen+3*messageLen)
	}
	corruptAt := headerLen + messageLen + descriptorLen // first payload octet of message 1
	raw[corruptAt] ^= 0xFF

	frame, err := acf.DecodeFrame(raw)
	if err != nil {
		t.Fatalf("DecodeFrame(corrupted): %v", err)
	}
	if len(frame.Messages) != 3 {
		t.Fatalf("len(frame.Messages) = %d, want 3", len(frame.Messages))
	}

	for i, msg := range frame.Messages {
		inner, err := e2e.Verify(stream, msg)
		if i == 1 {
			if !errors.Is(err, e2e.ErrCRCMismatch) {
				t.Errorf("message %d (corrupted): Verify err = %v, want ErrCRCMismatch", i, err)
			}
			continue
		}
		if err != nil {
			t.Errorf("message %d: Verify err = %v, want nil — a corrupt safe point on one message must not invalidate any other", i, err)
			continue
		}
		if !bytes.Equal(inner.Body, payloads[i]) {
			t.Errorf("message %d: recovered payload = % X, want % X", i, inner.Body, payloads[i])
		}
	}
}

// ── REQ-TC18-152 ────────────────────────────────────────────────────────────

// TestTC18_152_CRCMismatchSkipsExecutionAndReportsPOCIFailure pins TC18 §13.6
// (TC18.txt:3827-3828): a request whose CRC does not match is not executed,
// and is reported with Table 27's dedicated CRC code (POCI_FAILURE = 12)
// rather than the generic INVALID_PARAMETER fallback — the distinction a
// conformant client needs to treat an integrity failure as a safety event.
//
//fusa:test REQ-TC18-152
func TestTC18_152_CRCMismatchSkipsExecutionAndReportsPOCIFailure(t *testing.T) {
	client := tc18Stream(0x52)
	srv := tc18RootServer(t, client)
	router := udp.NewRouter(udp.NewEP0Handler(srv), true)

	endpoint := &tc18StubHandler{body: []byte{0x5A}}
	if err := router.Register(1, e2e.NewGuard(endpoint)); err != nil {
		t.Fatalf("Register: %v", err)
	}
	hdr := tc18UntimedHeader(client)

	req := acf.Message{
		Kind:           acf.KindShort,
		ByteBusID:      1,
		TransactionNum: 6,
		Control:        acf.FlagWrite,
		Body:           []byte{0x01, 0x02, 0x03, 0x04},
	}
	protected := e2e.Protect(client, req)

	// Control: an intact safe point does reach the endpoint.
	resp, shouldReply := router.Route(hdr, protected)
	if !shouldReply {
		t.Fatal("Route(intact) reported no reply")
	}
	if resp.Control.Has(acf.FlagError) {
		t.Fatalf("Route(intact) Control = %#08b, want FlagError clear", resp.Control)
	}
	if got := endpoint.callCount(); got != 1 {
		t.Fatalf("endpoint call count after intact request = %d, want 1", got)
	}

	corrupted := protected
	corrupted.Body = append([]byte(nil), protected.Body...)
	corrupted.Body[0] ^= 0xFF

	resp, shouldReply = router.Route(hdr, corrupted)
	if !shouldReply {
		t.Fatal("Route(corrupted) reported no reply; a CRC mismatch is reported, not dropped")
	}
	if !resp.Control.Has(acf.FlagError) {
		t.Fatalf("Route(corrupted) Control = %#08b, want FlagError set", resp.Control)
	}
	if got := endpoint.callCount(); got != 1 {
		t.Errorf("endpoint call count after corrupted request = %d, want 1 — the wrapped Handler must not be invoked at all", got)
	}

	code, _, err := udp.DecodeErrorBody(resp.Body)
	if err != nil {
		t.Fatalf("DecodeErrorBody: %v", err)
	}
	if code != udp.ErrorCodePOCIFailure {
		t.Errorf("error code = %d (%q), want %d (POCI_FAILURE)", uint8(code), code, uint8(udp.ErrorCodePOCIFailure))
	}
	if uint8(udp.ErrorCodePOCIFailure) != 12 {
		t.Errorf("udp.ErrorCodePOCIFailure = %d, want 12", uint8(udp.ErrorCodePOCIFailure))
	}
}

// ── REQ-TC18-153 ────────────────────────────────────────────────────────────

// TestTC18_153_ProtectedBodyIsPayloadThenPadThenCRC pins TC18 §13.6 Figures
// 19 and 20 (TC18.txt:3843-3881): a protected body is the real payload, then
// zero to three zero pad bytes rounding the payload up to a whole quadlet,
// then the four-byte big-endian CRC32 as the final quadlet — never CRC before
// pad. Byte positions are asserted literally for a payload needing two pad
// bytes and one needing a single pad byte.
//
//fusa:test REQ-TC18-153
func TestTC18_153_ProtectedBodyIsPayloadThenPadThenCRC(t *testing.T) {
	stream := tc18Stream(0x53)

	cases := []struct {
		name    string
		payload []byte
		wantPad int
	}{
		{name: "6-byte payload needs 2 pad bytes", payload: []byte{0xA0, 0xA1, 0xA2, 0xA3, 0xA4, 0xA5}, wantPad: 2},
		{name: "7-byte payload needs 1 pad byte", payload: []byte{0xB0, 0xB1, 0xB2, 0xB3, 0xB4, 0xB5, 0xB6}, wantPad: 1},
		{name: "4-byte payload needs no pad", payload: []byte{0xC0, 0xC1, 0xC2, 0xC3}, wantPad: 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m := acf.Message{
				Kind:           acf.KindShort,
				ByteBusID:      2,
				TransactionNum: 8,
				Control:        acf.FlagWrite,
				Body:           tc.payload,
			}
			crc := e2e.Compute(stream, m)
			got := e2e.Protect(stream, m)

			wantLen := len(tc.payload) + tc.wantPad + e2e.Len
			if len(got.Body) != wantLen {
				t.Fatalf("protected Body is %d bytes, want %d (payload %d + pad %d + CRC %d)",
					len(got.Body), wantLen, len(tc.payload), tc.wantPad, e2e.Len)
			}
			if len(got.Body)%4 != 0 {
				t.Errorf("protected Body length %d is not a whole number of quadlets", len(got.Body))
			}

			if !bytes.Equal(got.Body[:len(tc.payload)], tc.payload) {
				t.Errorf("protected Body[0:%d] = % X, want the payload % X", len(tc.payload), got.Body[:len(tc.payload)], tc.payload)
			}
			padRegion := got.Body[len(tc.payload) : len(tc.payload)+tc.wantPad]
			for i, b := range padRegion {
				if b != 0 {
					t.Errorf("pad byte %d (Body[%d]) = %#02x, want 0x00", i, len(tc.payload)+i, b)
				}
			}
			crcAt := len(got.Body) - e2e.Len
			if crcAt != len(tc.payload)+tc.wantPad {
				t.Errorf("CRC starts at Body[%d], want Body[%d] — the pad must precede the CRC, not follow it",
					crcAt, len(tc.payload)+tc.wantPad)
			}
			if trailer := binary.BigEndian.Uint32(got.Body[crcAt:]); trailer != crc {
				t.Errorf("trailing quadlet = %#08x, want the big-endian CRC32 %#08x", trailer, crc)
			}
			if _, err := e2e.Verify(stream, got); err != nil {
				t.Errorf("Verify(Protect(m)) err = %v, want nil", err)
			}
		})
	}
}

// ── REQ-TC18-189 ────────────────────────────────────────────────────────────

// TestTC18_189_PWMOutRejectsWrongLengthBodyAsInvalidParameter pins TC18
// §13.7.5.3 (TC18.txt:4630-4631): a PWM_OUT request whose byte_msg_payload is
// not exactly four bytes is rejected, and the rejection reaches the client as
// Table 27's INVALID_PARAMETER (15) — not as a dropped request and not as some
// other code.
//
//fusa:test REQ-TC18-189
func TestTC18_189_PWMOutRejectsWrongLengthBodyAsInvalidParameter(t *testing.T) {
	root := tc18Stream(0x89)
	srv := tc18RootServer(t, root)
	if err := srv.AddEndpoint(root, 1, pwm.EndpointType); err != nil {
		t.Fatalf("AddEndpoint: %v", err)
	}
	ep := pwm.NewEndpoint(srv, 1)
	if err := ep.Configure(root, pwm.Config{
		Enabled:            true,
		Role:               pwm.RoleOutput,
		DefaultActiveTicks: 10,
		DefaultPeriodTicks: 100,
	}); err != nil {
		t.Fatalf("pwm Configure: %v", err)
	}

	router := udp.NewRouter(udp.NewEP0Handler(srv), true)
	if err := router.Register(1, ep); err != nil {
		t.Fatalf("Register: %v", err)
	}
	hdr := tc18UntimedHeader(root)

	for _, body := range [][]byte{
		{0x00, 0x64, 0x0A},             // 3 bytes: one short
		{0x00, 0x64, 0x00, 0x0A, 0x00}, // 5 bytes: one long
	} {
		req := acf.Message{Kind: acf.KindShort, ByteBusID: 1, TransactionNum: 9, Control: acf.FlagWrite, Body: body}
		resp, shouldReply := router.Route(hdr, req)
		if !shouldReply {
			t.Fatalf("body of %d bytes: Route reported no reply; want an error response", len(body))
		}
		if !resp.Control.Has(acf.FlagError) {
			t.Fatalf("body of %d bytes: Control = %#08b, want FlagError set", len(body), resp.Control)
		}
		code, _, err := udp.DecodeErrorBody(resp.Body)
		if err != nil {
			t.Fatalf("body of %d bytes: DecodeErrorBody: %v", len(body), err)
		}
		if code != udp.ErrorCodeInvalidParameter {
			t.Errorf("body of %d bytes: error code = %d (%q), want %d (INVALID_PARAMETER)",
				len(body), uint8(code), code, uint8(udp.ErrorCodeInvalidParameter))
		}
		if uint8(udp.ErrorCodeInvalidParameter) != 15 {
			t.Errorf("udp.ErrorCodeInvalidParameter = %d, want 15", uint8(udp.ErrorCodeInvalidParameter))
		}
	}

	// Control: a body of exactly four bytes is accepted, so the rejections
	// above are attributable to the length and nothing else.
	ok := acf.Message{
		Kind: acf.KindShort, ByteBusID: 1, TransactionNum: 10, Control: acf.FlagWrite,
		Body: pwm.EncodeWaveform(10, 100),
	}
	resp, shouldReply := router.Route(hdr, ok)
	if !shouldReply || resp.Control.Has(acf.FlagError) {
		t.Errorf("4-byte body: shouldReply = %t, Control = %#08b; want a successful response", shouldReply, resp.Control)
	}
}

// ── REQ-TC18-199 ────────────────────────────────────────────────────────────

// TestTC18_199_I2CAcceptsBothMessageKindsAndEchoesKind pins TC18 §13.7.7.1
// (TC18.txt:4789-4791): an I2C transfer request may be framed as either
// ACF_ABB (acf.KindShort) or ACF_GBB (acf.KindLong), and the response uses the
// same message kind as its request.
//
//fusa:test REQ-TC18-199
func TestTC18_199_I2CAcceptsBothMessageKindsAndEchoesKind(t *testing.T) {
	root := tc18Stream(0x99)
	srv := tc18RootServer(t, root)
	if err := srv.AddEndpoint(root, 1, i2c.EndpointType); err != nil {
		t.Fatalf("AddEndpoint: %v", err)
	}
	ep := i2c.NewEndpoint(srv, 1)
	if err := ep.Configure(root, i2c.Config{Enabled: true, Speed: i2c.SpeedStandard}); err != nil {
		t.Fatalf("i2c Configure: %v", err)
	}

	cases := []struct {
		name string
		kind acf.MessageKind
	}{
		{name: "ACF_ABB", kind: acf.KindShort},
		{name: "ACF_GBB", kind: acf.KindLong},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := acf.Message{
				Kind:           tc.kind,
				ByteBusID:      1,
				TransactionNum: 12,
				Control:        acf.FlagWrite,
				Body:           i2c.EncodeTransferRequest([]byte{0x50, 0xAB}),
			}
			resp, err := ep.HandleRequest(root, req)
			if err != nil {
				t.Fatalf("HandleRequest(%v): %v", tc.name, err)
			}
			if resp.Kind != tc.kind {
				t.Errorf("response Kind = %d, want %d (the request's own kind)", resp.Kind, tc.kind)
			}
			if !resp.Control.Has(acf.FlagResponse) {
				t.Errorf("response Control = %#08b, want FlagResponse set", resp.Control)
			}
		})
	}
}

// ── REQ-TC18-203 ────────────────────────────────────────────────────────────

// tc18RecordingI2CTransport records the exact bytes handed to the bus and
// answers with a fixed reply, so a test can tell whether the endpoint
// rewrote, reordered or re-split the request payload on its way through.
type tc18RecordingI2CTransport struct {
	tx    []byte
	reply []byte
}

func (r *tc18RecordingI2CTransport) Transfer(tx []byte) ([]byte, error) {
	r.tx = append([]byte(nil), tx...)
	return append([]byte(nil), r.reply...), nil
}

// TestTC18_203_I2CIsAddressFormatTransparent pins TC18 §13.7.7.3
// (TC18.txt:4830-4832): the I2C endpoint places the request's
// byte_msg_payload on the bus verbatim, address bytes included, and never
// parses or rewrites them — so a 10-bit-addressed transfer (two address bytes
// ahead of the data) is handled identically to a 7-bit-addressed one.
//
//fusa:test REQ-TC18-203
func TestTC18_203_I2CIsAddressFormatTransparent(t *testing.T) {
	root := tc18Stream(0xA3)
	srv := tc18RootServer(t, root)
	if err := srv.AddEndpoint(root, 1, i2c.EndpointType); err != nil {
		t.Fatalf("AddEndpoint: %v", err)
	}
	ep := i2c.NewEndpoint(srv, 1)
	if err := ep.Configure(root, i2c.Config{Enabled: true, Speed: i2c.SpeedStandard}); err != nil {
		t.Fatalf("i2c Configure: %v", err)
	}

	// A 10-bit-address-shaped payload: the two-octet address prefix
	// (0b11110xx0 followed by the low eight address bits) and five data
	// bytes. Nothing in the endpoint may treat the first two octets
	// differently from the remaining five.
	payload := []byte{0xF4, 0x9C, 0x11, 0x22, 0x33, 0x44, 0x55}
	transport := &tc18RecordingI2CTransport{reply: []byte{0x01, 0x02}}
	ep.SetTransport(transport)

	req := acf.Message{
		Kind:           acf.KindShort,
		ByteBusID:      1,
		TransactionNum: 13,
		Control:        acf.FlagWrite,
		Body:           i2c.EncodeTransferRequest(payload),
	}
	resp, err := ep.HandleRequest(root, req)
	if err != nil {
		t.Fatalf("HandleRequest: %v", err)
	}
	if !bytes.Equal(transport.tx, payload) {
		t.Errorf("bytes presented to the transport = % X, want % X (byte-for-byte identical to the request body, no reordering and no address-length inference)",
			transport.tx, payload)
	}
	if !bytes.Equal(transport.tx, req.Body) {
		t.Errorf("bytes presented to the transport = % X, want the request Body % X", transport.tx, req.Body)
	}
	if got := i2c.DecodeTransferResponse(resp.Body); !bytes.Equal(got, transport.reply) {
		t.Errorf("response payload = % X, want the transport's reply % X", got, transport.reply)
	}
}

// ── REQ-TC18-227 ────────────────────────────────────────────────────────────

// TestTC18_227_StandardCANIdentifierIsRightAligned pins TC18 §13.7.11.3
// (TC18.txt:5471): an 11-bit standard identifier occupies the low bits of the
// encoded CAN ID field with the upper bits zero — not left-aligned, and not
// shifted into an extended-identifier position.
//
//fusa:test REQ-TC18-227
func TestTC18_227_StandardCANIdentifierIsRightAligned(t *testing.T) {
	const maxStandardID uint32 = 0x7FF

	f := can.Frame{Format: can.FormatClassical, Extended: false, ID: maxStandardID, Data: []byte{0xAA}}
	if err := f.Validate(); err != nil {
		t.Fatalf("Validate: %v", err)
	}
	raw := can.EncodeFrame(f)
	if len(raw) < 6 {
		t.Fatalf("encoded frame is %d bytes, too short to hold the CAN ID field", len(raw))
	}

	idField := binary.BigEndian.Uint32(raw[2:6])
	if idField != maxStandardID {
		t.Errorf("encoded CAN ID field = %#08x, want %#08x", idField, maxStandardID)
	}
	if idField>>11 != 0 {
		t.Errorf("encoded CAN ID field = %#08x, want the upper 21 bits zero", idField)
	}
	if raw[2] != 0x00 || raw[3] != 0x00 || raw[4] != 0x07 || raw[5] != 0xFF {
		t.Errorf("encoded CAN ID octets = % X, want 00 00 07 FF", raw[2:6])
	}

	// A small identifier is likewise right-aligned, not scaled or shifted.
	small := can.Frame{Format: can.FormatClassical, ID: 0x001, Data: []byte{0xAA}}
	rawSmall := can.EncodeFrame(small)
	if got := binary.BigEndian.Uint32(rawSmall[2:6]); got != 0x001 {
		t.Errorf("encoded CAN ID field for identifier 0x001 = %#08x, want 0x00000001", got)
	}

	decoded, err := can.DecodeFrame(raw)
	if err != nil {
		t.Fatalf("DecodeFrame: %v", err)
	}
	if decoded.ID != maxStandardID {
		t.Errorf("decoded ID = %#x, want %#x", decoded.ID, maxStandardID)
	}
	if decoded.Extended {
		t.Error("decoded Extended = true, want false")
	}
}

// ── REQ-TC18-228 ────────────────────────────────────────────────────────────

// TestTC18_228_CANFrameCannotExpressARemoteFrame pins TC18 §13.7.11.3
// (TC18.txt:5471): a CAN endpoint transmits data frames only — it has no way
// to send a remote transmission request. The assertion is structural: no field
// of can.Frame (or of the XL header it embeds) names an RTR/remote flag, so a
// remote frame is not representable at all rather than merely being rejected
// somewhere at runtime.
//
//fusa:test REQ-TC18-228
func TestTC18_228_CANFrameCannotExpressARemoteFrame(t *testing.T) {
	forbidden := []string{"RTR", "REMOTE"}

	check := func(typ reflect.Type) {
		for i := 0; i < typ.NumField(); i++ {
			name := typ.Field(i).Name
			upper := strings.ToUpper(name)
			for _, bad := range forbidden {
				if strings.Contains(upper, bad) {
					t.Errorf("%s has field %q, which names a remote-transmission-request flag; a CAN endpoint transmits data frames only",
						typ.Name(), name)
				}
			}
		}
	}

	frameType := reflect.TypeOf(can.Frame{})
	if frameType.NumField() == 0 {
		t.Fatal("can.Frame has no fields; the reflection sweep below would be vacuous")
	}
	check(frameType)
	check(reflect.TypeOf(can.XLHeader{}))
}

// ── REQ-TC18-242 ────────────────────────────────────────────────────────────

// TestTC18_242_MDIODataWidthFollowsTable57 pins TC18 §13.7.13.3 Table 57
// (TC18.txt:5682-5683): an MDIO access carries two payload bytes for either
// MMD mode, four bytes for an MMS mode whose selected MMS index is 0 or 1, and
// two bytes for every other MMS index. The widths are asserted both through
// Request.DataWidth and through the encoders' actual output lengths.
//
//fusa:test REQ-TC18-242
func TestTC18_242_MDIODataWidthFollowsTable57(t *testing.T) {
	const requestHeaderLen = 2 // reserved octet + packed mode/address octet

	cases := []struct {
		name    string
		mode    mdio.Mode
		devAddr uint8
		want    int
	}{
		{"MMD single word, addr 0", mdio.ModeMMDSingleWord, 0, 2},
		{"MMD single word, addr 1", mdio.ModeMMDSingleWord, 1, 2},
		{"MMD single word, addr 5", mdio.ModeMMDSingleWord, 5, 2},
		{"MMD single word, addr 31", mdio.ModeMMDSingleWord, 31, 2},
		{"MMD multi byte, addr 0", mdio.ModeMMDMultiByte, 0, 2},
		{"MMD multi byte, addr 1", mdio.ModeMMDMultiByte, 1, 2},
		{"MMD multi byte, addr 17", mdio.ModeMMDMultiByte, 17, 2},
		{"MMS single word, MMS0", mdio.ModeMMSSingleWord, 0, 4},
		{"MMS single word, MMS1", mdio.ModeMMSSingleWord, 1, 4},
		{"MMS single word, MMS2", mdio.ModeMMSSingleWord, 2, 2},
		{"MMS single word, MMS31", mdio.ModeMMSSingleWord, 31, 2},
		{"MMS multi word, MMS0", mdio.ModeMMSMultiWord, 0, 4},
		{"MMS multi word, MMS1", mdio.ModeMMSMultiWord, 1, 4},
		{"MMS multi word, MMS3", mdio.ModeMMSMultiWord, 3, 2},
		{"MMS multi word, MMS31", mdio.ModeMMSMultiWord, 31, 2},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := mdio.Request{Mode: tc.mode, DevAddr: tc.devAddr}
			if err := r.Validate(); err != nil {
				t.Fatalf("Validate: %v", err)
			}
			if got := r.DataWidth(); got != tc.want {
				t.Errorf("DataWidth() = %d, want %d", got, tc.want)
			}
			if got := len(mdio.EncodeResponse(r, 0x1234_5678)); got != tc.want {
				t.Errorf("len(EncodeResponse) = %d, want %d", got, tc.want)
			}
			if got := len(mdio.EncodeWriteRequest(r, 0x1234_5678)); got != requestHeaderLen+tc.want {
				t.Errorf("len(EncodeWriteRequest) = %d, want %d", got, requestHeaderLen+tc.want)
			}
			if got := len(mdio.EncodeReadRequest(r)); got != requestHeaderLen {
				t.Errorf("len(EncodeReadRequest) = %d, want %d", got, requestHeaderLen)
			}
		})
	}
}
