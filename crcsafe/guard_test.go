//fusa:test REQ-CRC-005

package crcsafe_test

import (
	"errors"
	"testing"

	"github.com/SoundMatt/go-RCP/avtp"
	"github.com/SoundMatt/go-RCP/crcsafe"
)

// stubHandler is a minimal request.Handler double recording whether it was
// called at all, so tests can assert Guard skips it entirely on a CRC
// failure.
type stubHandler struct {
	called   bool
	lastReq  avtp.Message
	response avtp.Message
	err      error
}

func (s *stubHandler) HandleRequest(requester avtp.StreamID, req avtp.Message) (avtp.Message, error) {
	s.called = true
	s.lastReq = req
	return s.response, s.err
}

// TestGuard_ValidRequest checks Guard strips and verifies an inbound safe
// point, forwards the recovered inner message to the wrapped Handler
// unchanged, and re-Protects that Handler's response (REQ-CRC-005).
func TestGuard_ValidRequest(t *testing.T) {
	stream := testStream()
	inner := baseMessage()
	protectedReq := crcsafe.Protect(stream, inner)

	stub := &stubHandler{response: avtp.Message{Control: avtp.FlagResponse, Body: []byte{0x42}}}
	g := crcsafe.NewGuard(stub)

	resp, err := g.HandleRequest(stream, protectedReq)
	if err != nil {
		t.Fatalf("HandleRequest: %v", err)
	}
	if !stub.called {
		t.Fatalf("wrapped Handler was never called for a valid safe point")
	}
	if string(stub.lastReq.Body) != string(inner.Body) {
		t.Errorf("wrapped Handler received Body = % X, want % X (the recovered inner request)", stub.lastReq.Body, inner.Body)
	}

	// The response must itself carry a valid, freshly computed safe point.
	respInner, err := crcsafe.Verify(stream, resp)
	if err != nil {
		t.Fatalf("Verify(Guard response): %v", err)
	}
	if string(respInner.Body) != "\x42" {
		t.Errorf("Verify(Guard response).Body = % X, want 42", respInner.Body)
	}
}

// TestGuard_CRCFailureSkipsHandler checks a Message whose safe point does
// not verify never reaches the wrapped Handler at all, and that Guard
// reports the dedicated ErrCRCMismatch error instead (REQ-CRC-005).
func TestGuard_CRCFailureSkipsHandler(t *testing.T) {
	stream := testStream()
	protectedReq := crcsafe.Protect(stream, baseMessage())
	protectedReq.Body = append([]byte(nil), protectedReq.Body...)
	protectedReq.Body[len(protectedReq.Body)-1] ^= 0xFF // corrupt the trailing CRC

	stub := &stubHandler{}
	g := crcsafe.NewGuard(stub)

	if _, err := g.HandleRequest(stream, protectedReq); !errors.Is(err, crcsafe.ErrCRCMismatch) {
		t.Errorf("HandleRequest(corrupted) err = %v, want ErrCRCMismatch", err)
	}
	if stub.called {
		t.Errorf("wrapped Handler was called despite a CRC mismatch: execution must be skipped entirely")
	}
}
