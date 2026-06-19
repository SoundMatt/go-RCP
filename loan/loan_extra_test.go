//fusa:test REQ-LOAN-001
//fusa:test REQ-LOAN-002
//fusa:test REQ-LOAN-006

package loan_test

import (
	"context"
	"errors"
	"testing"

	rcp "github.com/SoundMatt/go-RCP"
)

// TestLoan_Zone_Delegates verifies Zone reports the inner controller's zone.
func TestLoan_Zone_Delegates(t *testing.T) {
	c := newLC(t, rcp.ZoneCentral, nil)
	if got := c.Zone(); got != rcp.ZoneCentral {
		t.Errorf("Zone() = %v, want ZoneCentral", got)
	}
}

// TestLoan_Subscribe_Delegates verifies Subscribe delegates to the inner
// controller and yields a usable channel.
func TestLoan_Subscribe_Delegates(t *testing.T) {
	c := newLC(t, rcp.ZoneFrontLeft, nil)
	ch, err := c.Subscribe(context.Background())
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	if ch == nil {
		t.Fatal("Subscribe returned nil channel")
	}
}

// TestLoan_Close_Delegates verifies Close marks the controller closed so that
// subsequent Loan/SendLoaned calls report ErrClosed.
func TestLoan_Close_Delegates(t *testing.T) {
	c := newLC(t, rcp.ZoneFrontLeft, nil)
	if err := c.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if _, err := c.Loan(8); !errors.Is(err, rcp.ErrClosed) {
		t.Errorf("Loan after Close = %v, want ErrClosed", err)
	}
	_, err := c.SendLoaned(context.Background(), &rcp.Command{Zone: rcp.ZoneFrontLeft})
	if !errors.Is(err, rcp.ErrClosed) {
		t.Errorf("SendLoaned after Close = %v, want ErrClosed", err)
	}
}

// TestLoan_Loan_LargerThanPoolBuffer covers the allocation path taken when the
// requested size exceeds the pooled buffer capacity (256 bytes).
func TestLoan_Loan_LargerThanPoolBuffer(t *testing.T) {
	c := newLC(t, rcp.ZoneFrontLeft, nil)
	l, err := c.Loan(1024)
	if err != nil {
		t.Fatalf("Loan(1024): %v", err)
	}
	defer l.Return()
	if len(l.Payload) != 1024 {
		t.Errorf("payload len = %d, want 1024", len(l.Payload))
	}
	for i, b := range l.Payload {
		if b != 0 {
			t.Fatalf("payload[%d] = %d, want 0", i, b)
		}
	}
}

// TestLoan_Loan_ReusesPooledBuffer covers the pool-hit path: a returned buffer
// is reused (and re-zeroed) by a subsequent same-or-smaller Loan.
func TestLoan_Loan_ReusesPooledBuffer(t *testing.T) {
	c := newLC(t, rcp.ZoneFrontLeft, nil)
	first, err := c.Loan(32)
	if err != nil {
		t.Fatalf("Loan: %v", err)
	}
	for i := range first.Payload {
		first.Payload[i] = 0xFF // dirty the buffer before returning it
	}
	first.Return()

	second, err := c.Loan(16)
	if err != nil {
		t.Fatalf("Loan: %v", err)
	}
	defer second.Return()
	for i, b := range second.Payload {
		if b != 0 {
			t.Fatalf("reused payload[%d] = %d, want 0 (must be re-zeroed)", i, b)
		}
	}
}
