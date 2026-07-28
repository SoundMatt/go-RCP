//fusa:test REQ-LOAN-001
//fusa:test REQ-LOAN-002
//fusa:test REQ-LOAN-003
//fusa:test REQ-LOAN-004
//fusa:test REQ-LOAN-005
//fusa:test REQ-LOAN-006

package loan_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/SoundMatt/go-RCP/acf"
	"github.com/SoundMatt/go-RCP/avtp"
	"github.com/SoundMatt/go-RCP/loan"
	"github.com/SoundMatt/go-RCP/server"
	"github.com/SoundMatt/go-RCP/udp"
)

type echoHandler struct{ lastBody []byte }

func (h *echoHandler) HandleRequest(_ avtp.StreamID, req acf.Message) (acf.Message, error) {
	h.lastBody = append([]byte(nil), req.Body...)
	return acf.Message{
		Kind:           req.Kind,
		ByteBusID:      req.ByteBusID,
		TransactionNum: req.TransactionNum,
		Control:        acf.FlagResponse | (req.Control & (acf.FlagRead | acf.FlagWrite)),
		Body:           req.Body,
	}, nil
}

// newLC starts a udp.Server with handler registered at endpoint 1 and
// returns a loan.Controller dialed against it.
func newLC(t *testing.T, handler *echoHandler) *loan.Controller {
	t.Helper()
	router := udp.NewRouter(udp.NewEP0Handler(server.NewServer()), true)
	if err := router.Register(1, handler); err != nil {
		t.Fatalf("Register: %v", err)
	}
	us, err := udp.NewServer(avtp.NewStreamID([6]byte{0x02, 0xAA, 0xBB, 0xCC, 0xDD, 0xEE}, 4), "127.0.0.1:0", router)
	if err != nil {
		t.Fatalf("udp.NewServer: %v", err)
	}
	t.Cleanup(func() { _ = us.Close() })

	inner, err := udp.NewController(avtp.NewStreamID([6]byte{0x02, 0x11, 0x22, 0x33, 0x44, 0x55}, 4), us.Addr())
	if err != nil {
		t.Fatalf("udp.NewController: %v", err)
	}
	c := loan.New(inner)
	t.Cleanup(func() { _ = c.Close() })
	return c
}

// TestLoan_ReturnsZeroedBuffer verifies Loan returns a buffer of exactly
// the requested size, zeroed (REQ-LOAN-001).
func TestLoan_ReturnsZeroedBuffer(t *testing.T) {
	c := newLC(t, &echoHandler{})
	l, err := c.Loan(16)
	if err != nil {
		t.Fatalf("Loan: %v", err)
	}
	defer l.Return()
	if len(l.Payload) != 16 {
		t.Errorf("len = %d, want 16", len(l.Payload))
	}
	for i, b := range l.Payload {
		if b != 0 {
			t.Fatalf("payload[%d] = %d, want 0", i, b)
		}
	}
}

// TestRequestLoaned_DeliversPayload verifies RequestLoaned delivers a
// loaned buffer's contents to the Handler on the other end (REQ-LOAN-002,
// REQ-LOAN-003).
func TestRequestLoaned_DeliversPayload(t *testing.T) {
	h := &echoHandler{}
	c := newLC(t, h)
	l, err := c.Loan(4)
	if err != nil {
		t.Fatalf("Loan: %v", err)
	}
	copy(l.Payload, []byte{0xDE, 0xAD, 0xBE, 0xEF})

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	resp, err := c.RequestLoaned(ctx, 1, acf.FlagWrite, l.Payload)
	if err != nil {
		t.Fatalf("RequestLoaned: %v", err)
	}
	want := []byte{0xDE, 0xAD, 0xBE, 0xEF}
	if len(resp.Body) != len(want) {
		t.Fatalf("resp.Body = % X, want % X", resp.Body, want)
	}
	for i := range want {
		if resp.Body[i] != want[i] {
			t.Errorf("resp.Body[%d] = %02X, want %02X", i, resp.Body[i], want[i])
		}
	}
}

// TestLoan_Return_NoPanic verifies Return is safe to call and does not
// panic, including for a zero-length loan (REQ-LOAN-004).
func TestLoan_Return_NoPanic(t *testing.T) {
	c := newLC(t, &echoHandler{})
	l, err := c.Loan(0)
	if err != nil {
		t.Fatalf("Loan(0): %v", err)
	}
	l.Return() // must not panic
}

// TestLoan_NegativeSize verifies Loan rejects a negative size (REQ-LOAN-005).
func TestLoan_NegativeSize(t *testing.T) {
	c := newLC(t, &echoHandler{})
	if _, err := c.Loan(-1); err == nil {
		t.Error("Loan(-1) = nil error, want an error")
	}
}

// TestController_Close_RejectsFurtherLoans verifies Close causes subsequent
// Loan/RequestLoaned calls to report ErrClosed (REQ-LOAN-006).
func TestController_Close_RejectsFurtherLoans(t *testing.T) {
	c := newLC(t, &echoHandler{})
	if err := c.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if _, err := c.Loan(8); !errors.Is(err, udp.ErrClosed) {
		t.Errorf("Loan after Close = %v, want ErrClosed", err)
	}
	_, err := c.RequestLoaned(context.Background(), 1, acf.FlagWrite, nil)
	if !errors.Is(err, udp.ErrClosed) {
		t.Errorf("RequestLoaned after Close = %v, want ErrClosed", err)
	}
}
