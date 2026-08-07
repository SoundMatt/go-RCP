//fusa:test REQ-REC-001
//fusa:test REQ-REC-002
//fusa:test REQ-REC-003
//fusa:test REQ-REC-004
//fusa:test REQ-REC-005
//fusa:test REQ-REC-006
//fusa:test REQ-REC-007
//fusa:test REQ-REC-008

package record_test

import (
	"bytes"
	"errors"
	"sync"
	"testing"

	"github.com/SoundMatt/go-RCP/v9/acf"
	"github.com/SoundMatt/go-RCP/v9/avtp"
	"github.com/SoundMatt/go-RCP/v9/record"
)

func testStream() avtp.StreamID {
	return avtp.NewStreamID([6]byte{0x02, 0x11, 0x22, 0x33, 0x44, 0x55}, 1)
}

type stubHandler struct {
	resp acf.Message
	err  error
}

func (s *stubHandler) HandleRequest(_ avtp.StreamID, req acf.Message) (acf.Message, error) {
	if s.err != nil {
		return acf.Message{}, s.err
	}
	resp := s.resp
	resp.Kind = req.Kind
	resp.TransactionNum = req.TransactionNum
	return resp, nil
}

// TestRecorder_RingBuffer_KeepsMostRecent verifies ring-buffer mode retains
// only the most recent MaxEntries entries while Written still counts every
// append (REQ-REC-001).
func TestRecorder_RingBuffer_KeepsMostRecent(t *testing.T) {
	rec := record.New(2)
	h := record.NewHandler(&stubHandler{resp: acf.Message{Control: acf.FlagResponse}}, rec)
	for i := range 3 {
		_, _ = h.HandleRequest(testStream(), acf.Message{ByteBusID: 1, TransactionNum: avtp.TransactionNum(i)})
	}
	snap := rec.Snapshot()
	if len(snap) != 2 {
		t.Fatalf("Snapshot len = %d, want 2", len(snap))
	}
	if snap[0].Request.TransactionNum != 1 || snap[1].Request.TransactionNum != 2 {
		t.Errorf("ring buffer did not keep the two most recent entries: %+v", snap)
	}
	if rec.Written() != 3 {
		t.Errorf("Written() = %d, want 3", rec.Written())
	}
}

// TestRecorder_Unlimited_KeepsAll verifies maxEntries=0 retains every entry
// (REQ-REC-002).
func TestRecorder_Unlimited_KeepsAll(t *testing.T) {
	rec := record.New(0)
	h := record.NewHandler(&stubHandler{resp: acf.Message{Control: acf.FlagResponse}}, rec)
	for i := range 5 {
		_, _ = h.HandleRequest(testStream(), acf.Message{ByteBusID: 1, TransactionNum: avtp.TransactionNum(i)})
	}
	if len(rec.Snapshot()) != 5 {
		t.Errorf("Snapshot len = %d, want 5", len(rec.Snapshot()))
	}
}

// TestHandler_RecordsSuccessfulResponse verifies HandleRequest records a
// successful call's response and returns it unchanged (REQ-REC-003).
func TestHandler_RecordsSuccessfulResponse(t *testing.T) {
	rec := record.New(0)
	inner := &stubHandler{resp: acf.Message{Control: acf.FlagResponse, Body: []byte("ok")}}
	h := record.NewHandler(inner, rec)

	resp, err := h.HandleRequest(testStream(), acf.Message{ByteBusID: 1, Control: acf.FlagRead, Body: []byte("req")})
	if err != nil {
		t.Fatalf("HandleRequest: %v", err)
	}
	if string(resp.Body) != "ok" {
		t.Errorf("resp.Body = %q, want ok", resp.Body)
	}
	snap := rec.Snapshot()
	if len(snap) != 1 {
		t.Fatalf("Snapshot len = %d, want 1", len(snap))
	}
	if snap[0].Err != "" {
		t.Errorf("Err = %q, want empty", snap[0].Err)
	}
	if string(snap[0].Response.Body) != "ok" {
		t.Errorf("recorded Response.Body = %q, want ok", snap[0].Response.Body)
	}
	if string(snap[0].Request.Body) != "req" {
		t.Errorf("recorded Request.Body = %q, want req", snap[0].Request.Body)
	}
}

// TestHandler_RecordsError verifies HandleRequest records a failing call's
// error text and returns the error unchanged (REQ-REC-004).
func TestHandler_RecordsError(t *testing.T) {
	rec := record.New(0)
	wantErr := errors.New("boom")
	inner := &stubHandler{err: wantErr}
	h := record.NewHandler(inner, rec)

	_, err := h.HandleRequest(testStream(), acf.Message{ByteBusID: 1})
	if !errors.Is(err, wantErr) {
		t.Fatalf("HandleRequest err = %v, want %v", err, wantErr)
	}
	snap := rec.Snapshot()
	if len(snap) != 1 || snap[0].Err != "boom" {
		t.Fatalf("recorded entry = %+v, want Err=boom", snap)
	}
}

// TestWriteTo_ReadFrom_RoundTrips verifies entries survive a WriteTo/
// ReadFrom round trip intact (REQ-REC-005).
func TestWriteTo_ReadFrom_RoundTrips(t *testing.T) {
	rec := record.New(0)
	h := record.NewHandler(&stubHandler{resp: acf.Message{Control: acf.FlagResponse, Body: []byte("ack")}}, rec)
	stream := testStream()
	if _, err := h.HandleRequest(stream, acf.Message{Kind: acf.KindShort, ByteBusID: 3, Control: acf.FlagWrite, Body: []byte("hello")}); err != nil {
		t.Fatalf("HandleRequest: %v", err)
	}

	var buf bytes.Buffer
	if _, err := rec.WriteTo(&buf); err != nil {
		t.Fatalf("WriteTo: %v", err)
	}
	got, err := record.ReadFrom(&buf)
	if err != nil {
		t.Fatalf("ReadFrom: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("ReadFrom len = %d, want 1", len(got))
	}
	if got[0].Requester != stream {
		t.Errorf("Requester = %v, want %v", got[0].Requester, stream)
	}
	if got[0].Request.ByteBusID != 3 || string(got[0].Request.Body) != "hello" {
		t.Errorf("Request round-trip mismatch: %+v", got[0].Request)
	}
	if string(got[0].Response.Body) != "ack" {
		t.Errorf("Response round-trip mismatch: %+v", got[0].Response)
	}
}

// TestReadFrom_DetectsCorruption verifies a tampered log reports
// ErrCorrupted (REQ-REC-006).
func TestReadFrom_DetectsCorruption(t *testing.T) {
	rec := record.New(0)
	h := record.NewHandler(&stubHandler{resp: acf.Message{Control: acf.FlagResponse}}, rec)
	if _, err := h.HandleRequest(testStream(), acf.Message{Kind: acf.KindShort, ByteBusID: 1}); err != nil {
		t.Fatalf("HandleRequest: %v", err)
	}

	var buf bytes.Buffer
	if _, err := rec.WriteTo(&buf); err != nil {
		t.Fatalf("WriteTo: %v", err)
	}
	corrupted := buf.Bytes()
	corrupted[len(corrupted)-1] ^= 0xFF // flip a bit in the trailing CRC

	_, err := record.ReadFrom(bytes.NewReader(corrupted))
	if !errors.Is(err, record.ErrCorrupted) {
		t.Errorf("err = %v, want ErrCorrupted", err)
	}
}

// TestReplay_FeedsRecordedRequestsInOrder verifies Replay calls target once
// per recorded request, in order, and reports its responses (REQ-REC-007).
func TestReplay_FeedsRecordedRequestsInOrder(t *testing.T) {
	rec := record.New(0)
	h := record.NewHandler(&stubHandler{resp: acf.Message{Control: acf.FlagResponse}}, rec)
	stream := testStream()
	for i := range 3 {
		if _, err := h.HandleRequest(stream, acf.Message{ByteBusID: 1, TransactionNum: avtp.TransactionNum(i)}); err != nil {
			t.Fatalf("HandleRequest: %v", err)
		}
	}

	var order []avtp.TransactionNum
	target := &orderTrackingHandler{order: &order}
	resps, err := record.Replay(target, rec.Snapshot())
	if err != nil {
		t.Fatalf("Replay: %v", err)
	}
	if len(resps) != 3 {
		t.Fatalf("Replay returned %d responses, want 3", len(resps))
	}
	if order[0] != 0 || order[1] != 1 || order[2] != 2 {
		t.Errorf("replay order = %v, want [0 1 2]", order)
	}
}

type orderTrackingHandler struct {
	order *[]avtp.TransactionNum
}

func (o *orderTrackingHandler) HandleRequest(_ avtp.StreamID, req acf.Message) (acf.Message, error) {
	*o.order = append(*o.order, req.TransactionNum)
	return acf.Message{Control: acf.FlagResponse}, nil
}

// TestHandler_Concurrent verifies concurrent HandleRequest calls are
// data-race free (REQ-REC-008).
func TestHandler_Concurrent(t *testing.T) {
	rec := record.New(100)
	h := record.NewHandler(&stubHandler{resp: acf.Message{Control: acf.FlagResponse}}, rec)
	var wg sync.WaitGroup
	for i := range 30 {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			_, _ = h.HandleRequest(testStream(), acf.Message{ByteBusID: 1, TransactionNum: avtp.TransactionNum(id)})
		}(i)
	}
	wg.Wait()
}
