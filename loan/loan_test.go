//fusa:test REQ-LOAN-001
//fusa:test REQ-LOAN-002
//fusa:test REQ-LOAN-003
//fusa:test REQ-LOAN-004
//fusa:test REQ-LOAN-005
//fusa:test REQ-LOAN-006

package loan_test

import (
	"bytes"
	"context"
	"errors"
	"testing"
	"time"

	rcp "github.com/SoundMatt/go-RCP"
	"github.com/SoundMatt/go-RCP/loan"
	mocktransport "github.com/SoundMatt/go-RCP/mock"
)

func newLC(t *testing.T, zone rcp.Zone, handler mocktransport.Handler) *loan.Controller {
	t.Helper()
	inner := mocktransport.NewController(zone, handler)
	t.Cleanup(func() { _ = inner.Close() })
	return loan.New(inner)
}

// TestLoan_Loan_ReturnsZeroedBuffer verifies Loan returns a zeroed buffer (REQ-LOAN-001).
func TestLoan_Loan_ReturnsZeroedBuffer(t *testing.T) {
	c := newLC(t, rcp.ZoneFrontLeft, nil)
	l, err := c.Loan(16)
	if err != nil {
		t.Fatalf("Loan: %v", err)
	}
	defer l.Return()

	if len(l.Payload) != 16 {
		t.Errorf("payload len = %d, want 16", len(l.Payload))
	}
	for i, b := range l.Payload {
		if b != 0 {
			t.Errorf("payload[%d] = %d, want 0 (not zeroed)", i, b)
		}
	}
}

// TestLoan_SendLoaned_RoundTrip verifies Send via loaned buffer succeeds (REQ-LOAN-002).
func TestLoan_SendLoaned_RoundTrip(t *testing.T) {
	c := newLC(t, rcp.ZoneFrontRight, nil)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	l, err := c.Loan(4)
	if err != nil {
		t.Fatalf("Loan: %v", err)
	}
	copy(l.Payload, []byte{0x01, 0x02, 0x03, 0x04})

	resp, err := c.SendLoaned(ctx, &rcp.Command{Zone: rcp.ZoneFrontRight, Type: rcp.CmdSet, Payload: l.Payload})
	if err != nil {
		t.Fatalf("SendLoaned: %v", err)
	}
	if resp.Status != rcp.StatusOK {
		t.Errorf("status = %v, want OK", resp.Status)
	}
}

// TestLoan_SendLoaned_PayloadReachesHandler verifies payload reaches the server handler (REQ-LOAN-003).
func TestLoan_SendLoaned_PayloadReachesHandler(t *testing.T) {
	want := []byte{0xDE, 0xAD}
	c := newLC(t, rcp.ZoneRearLeft, func(cmd *rcp.Command) *rcp.Response {
		return &rcp.Response{CommandID: cmd.ID, Zone: cmd.Zone, Status: rcp.StatusOK, Payload: cmd.Payload}
	})
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	l, err := c.Loan(len(want))
	if err != nil {
		t.Fatalf("Loan: %v", err)
	}
	copy(l.Payload, want)

	resp, err := c.SendLoaned(ctx, &rcp.Command{Zone: rcp.ZoneRearLeft, Type: rcp.CmdSet, Payload: l.Payload})
	if err != nil {
		t.Fatalf("SendLoaned: %v", err)
	}
	if !bytes.Equal(resp.Payload, want) {
		t.Errorf("response payload = %v, want %v", resp.Payload, want)
	}
}

// TestLoan_Return_DoesNotPanic verifies Return is safe to call (REQ-LOAN-004).
func TestLoan_Return_DoesNotPanic(t *testing.T) {
	c := newLC(t, rcp.ZoneCentral, nil)
	l, err := c.Loan(8)
	if err != nil {
		t.Fatalf("Loan: %v", err)
	}
	l.Return()
}

// TestLoan_Loan_NegativeSize verifies error on negative size (REQ-LOAN-005).
func TestLoan_Loan_NegativeSize(t *testing.T) {
	c := newLC(t, rcp.ZoneFrontLeft, nil)
	_, err := c.Loan(-1)
	if err == nil {
		t.Error("expected error for negative size, got nil")
	}
}

// TestLoan_Send_DelegatesCorrectly verifies Send delegates to inner (REQ-LOAN-006).
func TestLoan_Send_DelegatesCorrectly(t *testing.T) {
	c := newLC(t, rcp.ZoneRearRight, nil)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	// Sending to wrong zone proves delegation (inner's zone mismatch guard fires)
	_, err := c.Send(ctx, &rcp.Command{Zone: rcp.ZoneFrontLeft, Type: rcp.CmdNoop})
	if !errors.Is(err, rcp.ErrZoneMismatch) {
		t.Errorf("error = %v, want ErrZoneMismatch (proving delegation)", err)
	}
}
