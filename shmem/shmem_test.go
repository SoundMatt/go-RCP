//fusa:test REQ-SHMEM-001
//fusa:test REQ-SHMEM-002
//fusa:test REQ-SHMEM-003
//fusa:test REQ-SHMEM-004
//fusa:test REQ-SHMEM-005
//fusa:test REQ-SHMEM-006

package shmem_test

import (
	"bytes"
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/SoundMatt/go-RCP/v9/acf"
	"github.com/SoundMatt/go-RCP/v9/avtp"
	"github.com/SoundMatt/go-RCP/v9/server"
	"github.com/SoundMatt/go-RCP/v9/shmem"
	"github.com/SoundMatt/go-RCP/v9/udp"
)

type echoHandler struct{}

func (echoHandler) HandleRequest(_ avtp.StreamID, req acf.Message) (acf.Message, error) {
	return acf.Message{
		Kind:           req.Kind,
		ByteBusID:      req.ByteBusID,
		TransactionNum: req.TransactionNum,
		Control:        acf.FlagResponse | (req.Control & (acf.FlagRead | acf.FlagWrite)),
		Body:           req.Body,
	}, nil
}

func serverID() avtp.StreamID {
	return avtp.NewStreamID([6]byte{0x02, 0xAA, 0xBB, 0xCC, 0xDD, 0xEE}, 3)
}

func clientID() avtp.StreamID {
	return avtp.NewStreamID([6]byte{0x02, 0x11, 0x22, 0x33, 0x44, 0x55}, 3)
}

func newBus(t *testing.T) (*udp.Router, *shmem.Registry, *shmem.Controller) {
	t.Helper()
	router := udp.NewRouter(udp.NewEP0Handler(server.NewServer()), true)
	if err := router.Register(1, echoHandler{}); err != nil {
		t.Fatalf("Register: %v", err)
	}
	reg := shmem.NewRegistry()
	t.Cleanup(func() { _ = reg.Close() })
	_, ctrl, err := reg.Open("bus", router, serverID(), clientID())
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	return router, reg, ctrl
}

// TestController_Request_RoundTrips verifies a request reaches the
// Router-registered Handler over the shared bus and its response is
// returned to the caller (REQ-SHMEM-001).
func TestController_Request_RoundTrips(t *testing.T) {
	_, _, ctrl := newBus(t)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	resp, err := ctrl.Read(ctx, 1)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if !resp.Control.Has(acf.FlagResponse) {
		t.Errorf("response missing FlagResponse")
	}
}

// TestController_Request_HandlerInvoked verifies the registered Handler
// actually observes each request routed to it (REQ-SHMEM-002).
func TestController_Request_HandlerInvoked(t *testing.T) {
	router := udp.NewRouter(udp.NewEP0Handler(server.NewServer()), true)
	var mu sync.Mutex
	calls := 0
	h := handlerFunc(func(_ avtp.StreamID, req acf.Message) (acf.Message, error) {
		mu.Lock()
		calls++
		mu.Unlock()
		return acf.Message{Kind: req.Kind, ByteBusID: req.ByteBusID, TransactionNum: req.TransactionNum, Control: acf.FlagResponse}, nil
	})
	if err := router.Register(1, h); err != nil {
		t.Fatalf("Register: %v", err)
	}
	reg := shmem.NewRegistry()
	defer func() { _ = reg.Close() }()
	_, ctrl, err := reg.Open("bus", router, serverID(), clientID())
	if err != nil {
		t.Fatalf("Open: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if _, err := ctrl.Write(ctx, 1, []byte{0x01}); err != nil {
		t.Fatalf("Write: %v", err)
	}

	mu.Lock()
	defer mu.Unlock()
	if calls != 1 {
		t.Errorf("handler called %d times, want 1", calls)
	}
}

type handlerFunc func(avtp.StreamID, acf.Message) (acf.Message, error)

func (f handlerFunc) HandleRequest(requester avtp.StreamID, req acf.Message) (acf.Message, error) {
	return f(requester, req)
}

// TestController_Request_CopiesPayload verifies the caller's body slice is
// copied before being handed to the bus, so mutating it after the call
// does not affect what the Handler observed (REQ-SHMEM-003).
func TestController_Request_CopiesPayload(t *testing.T) {
	router := udp.NewRouter(udp.NewEP0Handler(server.NewServer()), true)
	var observed []byte
	h := handlerFunc(func(_ avtp.StreamID, req acf.Message) (acf.Message, error) {
		observed = append([]byte(nil), req.Body...)
		return acf.Message{Kind: req.Kind, ByteBusID: req.ByteBusID, TransactionNum: req.TransactionNum, Control: acf.FlagResponse}, nil
	})
	if err := router.Register(1, h); err != nil {
		t.Fatalf("Register: %v", err)
	}
	reg := shmem.NewRegistry()
	defer func() { _ = reg.Close() }()
	_, ctrl, err := reg.Open("bus", router, serverID(), clientID())
	if err != nil {
		t.Fatalf("Open: %v", err)
	}

	body := []byte{0xDE, 0xAD}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if _, err := ctrl.Write(ctx, 1, body); err != nil {
		t.Fatalf("Write: %v", err)
	}
	body[0] = 0xFF // mutate after send

	if !bytes.Equal(observed, []byte{0xDE, 0xAD}) {
		t.Errorf("observed body = % X, want DE AD (post-send mutation must not be visible)", observed)
	}
}

// TestController_Request_ContextExpired verifies ErrTimeout is returned for
// an already-cancelled context (REQ-SHMEM-004).
func TestController_Request_ContextExpired(t *testing.T) {
	_, _, ctrl := newBus(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := ctrl.Read(ctx, 1)
	if !errors.Is(err, udp.ErrTimeout) {
		t.Errorf("error = %v, want ErrTimeout", err)
	}
}

// TestController_Request_AfterClose verifies ErrClosed is returned once the
// Controller has been closed (REQ-SHMEM-005).
func TestController_Request_AfterClose(t *testing.T) {
	_, _, ctrl := newBus(t)
	_ = ctrl.Close()

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	_, err := ctrl.Read(ctx, 1)
	if !errors.Is(err, udp.ErrClosed) {
		t.Errorf("error = %v, want ErrClosed", err)
	}
}

// TestController_Request_ConcurrentCallers verifies many goroutines can
// issue requests over the same Controller/Bus concurrently without races
// or lost responses (REQ-SHMEM-006).
func TestController_Request_ConcurrentCallers(t *testing.T) {
	_, _, ctrl := newBus(t)
	const n = 50
	var wg sync.WaitGroup
	errs := make([]error, n)
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			_, err := ctrl.Read(ctx, 1)
			errs[i] = err
		}(i)
	}
	wg.Wait()
	for i, err := range errs {
		if err != nil {
			t.Errorf("caller %d: %v", i, err)
		}
	}
}
