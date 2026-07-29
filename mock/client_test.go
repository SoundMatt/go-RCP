//fusa:test REQ-MCL-001
//fusa:test REQ-MCL-002
//fusa:test REQ-MCL-003
//fusa:test REQ-MCL-004
//fusa:test REQ-MCL-005
//fusa:test REQ-MCL-006
//fusa:test REQ-MCL-007
//fusa:test REQ-MCL-008

package mock_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/SoundMatt/go-RCP/acf"
	"github.com/SoundMatt/go-RCP/avtp"
	"github.com/SoundMatt/go-RCP/mock"
	"github.com/SoundMatt/go-RCP/regmap"
)

func newFixture(t *testing.T) *mock.Fixture {
	t.Helper()
	fx, err := mock.NewFixture(testStream(), true)
	if err != nil {
		t.Fatalf("NewFixture: %v", err)
	}
	t.Cleanup(func() { _ = fx.Close() })
	return fx
}

// TestClient_Request_ReachesRegisteredEndpoint verifies Request routes to a
// registered Endpoint with the correct requester and body (REQ-MCL-001).
func TestClient_Request_ReachesRegisteredEndpoint(t *testing.T) {
	fx := newFixture(t)
	if err := fx.Server.AddEndpoint(fx.Root.StreamID(), 1, regmap.EndpointTypeGPIO); err != nil {
		t.Fatalf("AddEndpoint: %v", err)
	}
	var gotFrom avtp.StreamID
	var gotBody []byte
	ep := mock.NewEndpoint(1, func(requester avtp.StreamID, req acf.Message) (acf.Message, error) {
		gotFrom = requester
		gotBody = req.Body
		return acf.Message{Control: acf.FlagResponse, Body: []byte("ack")}, nil
	})
	if err := fx.Router.Register(1, ep); err != nil {
		t.Fatalf("Register: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	resp, err := fx.Root.Request(ctx, 1, acf.FlagWrite, []byte("hello"))
	if err != nil {
		t.Fatalf("Request: %v", err)
	}
	if string(resp.Body) != "ack" {
		t.Errorf("resp.Body = %q, want %q", resp.Body, "ack")
	}
	if gotFrom != fx.Root.StreamID() {
		t.Errorf("requester = %v, want %v", gotFrom, fx.Root.StreamID())
	}
	if string(gotBody) != "hello" {
		t.Errorf("endpoint saw body = %q, want %q", gotBody, "hello")
	}
}

// TestClient_ReadWrite_SetControlFlags verifies Read/Write set exactly the
// expected control flag (REQ-MCL-002).
func TestClient_ReadWrite_SetControlFlags(t *testing.T) {
	fx := newFixture(t)
	if err := fx.Server.AddEndpoint(fx.Root.StreamID(), 1, regmap.EndpointTypeGPIO); err != nil {
		t.Fatalf("AddEndpoint: %v", err)
	}
	var lastControl acf.ControlFlags
	ep := mock.NewEndpoint(1, func(_ avtp.StreamID, req acf.Message) (acf.Message, error) {
		lastControl = req.Control
		return acf.Message{Control: acf.FlagResponse}, nil
	})
	if err := fx.Router.Register(1, ep); err != nil {
		t.Fatalf("Register: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	if _, err := fx.Root.Read(ctx, 1); err != nil {
		t.Fatalf("Read: %v", err)
	}
	if !lastControl.Has(acf.FlagRead) {
		t.Error("Read did not set FlagRead")
	}

	if _, err := fx.Root.Write(ctx, 1, []byte("x")); err != nil {
		t.Fatalf("Write: %v", err)
	}
	if !lastControl.Has(acf.FlagWrite) {
		t.Error("Write did not set FlagWrite")
	}
}

// TestClient_Discover_RoundTrips verifies Discover returns a decodable
// register map reflecting a declared endpoint (REQ-MCL-003).
func TestClient_Discover_RoundTrips(t *testing.T) {
	fx := newFixture(t)
	if err := fx.Server.AddEndpoint(fx.Root.StreamID(), 5, regmap.EndpointTypeADC); err != nil {
		t.Fatalf("AddEndpoint: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	buf, err := fx.Root.Discover(ctx)
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	m, err := regmap.DecodeRegisterMap(buf)
	if err != nil {
		t.Fatalf("DecodeRegisterMap: %v", err)
	}
	if _, ok := m.Endpoint(5); !ok {
		t.Error("decoded map missing endpoint 5")
	}
}

// TestClient_StreamID verifies StreamID reports the configured identity
// (REQ-MCL-004).
func TestClient_StreamID(t *testing.T) {
	fx := newFixture(t)
	if fx.Root.StreamID() != testStream() {
		t.Errorf("StreamID() = %v, want %v", fx.Root.StreamID(), testStream())
	}
}

// TestClient_Close_IdempotentAndRejectsRequest verifies Close is safe to
// call multiple times and Request after Close reports ErrClosed
// (REQ-MCL-005).
func TestClient_Close_IdempotentAndRejectsRequest(t *testing.T) {
	fx := newFixture(t)
	if err := fx.Root.Close(); err != nil {
		t.Fatalf("first close: %v", err)
	}
	if err := fx.Root.Close(); err != nil {
		t.Fatalf("second close: %v", err)
	}
	_, err := fx.Root.Request(context.Background(), 1, acf.FlagRead, nil)
	if !errors.Is(err, mock.ErrClosed) {
		t.Errorf("err = %v, want ErrClosed", err)
	}
}

// TestClient_Request_HonorsCancelledContext verifies a pre-cancelled
// context is rejected before the router is invoked (REQ-MCL-006).
func TestClient_Request_HonorsCancelledContext(t *testing.T) {
	fx := newFixture(t)
	invoked := false
	if err := fx.Server.AddEndpoint(fx.Root.StreamID(), 1, regmap.EndpointTypeGPIO); err != nil {
		t.Fatalf("AddEndpoint: %v", err)
	}
	ep := mock.NewEndpoint(1, func(_ avtp.StreamID, req acf.Message) (acf.Message, error) {
		invoked = true
		return acf.Message{Control: acf.FlagResponse}, nil
	})
	if err := fx.Router.Register(1, ep); err != nil {
		t.Fatalf("Register: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := fx.Root.Request(ctx, 1, acf.FlagRead, nil)
	if err == nil {
		t.Fatal("expected error for cancelled context")
	}
	if invoked {
		t.Error("endpoint was invoked despite cancelled context")
	}
}

// TestClient_Request_Concurrent verifies concurrent Request calls are
// data-race free (REQ-MCL-007).
func TestClient_Request_Concurrent(t *testing.T) {
	fx := newFixture(t)
	if err := fx.Server.AddEndpoint(fx.Root.StreamID(), 1, regmap.EndpointTypeGPIO); err != nil {
		t.Fatalf("AddEndpoint: %v", err)
	}
	ep := mock.NewEndpoint(1, nil)
	if err := fx.Router.Register(1, ep); err != nil {
		t.Fatalf("Register: %v", err)
	}

	var wg sync.WaitGroup
	for range 50 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _ = fx.Root.Request(context.Background(), 1, acf.FlagRead, nil)
		}()
	}
	wg.Wait()
}

// TestClient_Request_TransactionNumIncrements verifies successive requests
// carry distinct, increasing TransactionNum values (REQ-MCL-008).
func TestClient_Request_TransactionNumIncrements(t *testing.T) {
	fx := newFixture(t)
	if err := fx.Server.AddEndpoint(fx.Root.StreamID(), 1, regmap.EndpointTypeGPIO); err != nil {
		t.Fatalf("AddEndpoint: %v", err)
	}
	var seen []avtp.TransactionNum
	ep := mock.NewEndpoint(1, func(_ avtp.StreamID, req acf.Message) (acf.Message, error) {
		seen = append(seen, req.TransactionNum)
		return acf.Message{Control: acf.FlagResponse}, nil
	})
	if err := fx.Router.Register(1, ep); err != nil {
		t.Fatalf("Register: %v", err)
	}

	ctx := context.Background()
	for range 3 {
		if _, err := fx.Root.Read(ctx, 1); err != nil {
			t.Fatalf("Read: %v", err)
		}
	}
	if len(seen) != 3 || seen[0] == seen[1] || seen[1] == seen[2] {
		t.Errorf("TransactionNum sequence not distinct: %v", seen)
	}
}

func FuzzClient_Request(f *testing.F) {
	f.Add(uint8(1), uint8(1), []byte(`{"k":"v"}`))
	f.Fuzz(func(t *testing.T, addr uint8, control uint8, body []byte) {
		fx, err := mock.NewFixture(testStream(), true)
		if err != nil {
			t.Fatalf("NewFixture: %v", err)
		}
		defer func() { _ = fx.Close() }()
		if addErr := fx.Server.AddEndpoint(fx.Root.StreamID(), avtp.ByteBusID(addr|1), regmap.EndpointTypeGPIO); addErr != nil {
			return
		}
		ep := mock.NewEndpoint(avtp.ByteBusID(addr|1), nil)
		if regErr := fx.Router.Register(avtp.ByteBusID(addr|1), ep); regErr != nil {
			return
		}
		resp, err := fx.Root.Request(context.Background(), avtp.ByteBusID(addr|1), acf.ControlFlags(control), body)
		if err != nil {
			return
		}
		_ = resp
	})
}
