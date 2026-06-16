//fusa:test REQ-SHMEM-001
//fusa:test REQ-SHMEM-002
//fusa:test REQ-SHMEM-003
//fusa:test REQ-SHMEM-004
//fusa:test REQ-SHMEM-005
//fusa:test REQ-SHMEM-006
//fusa:test REQ-SHMEM-007
//fusa:test REQ-SHMEM-008

package shmem_test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	rcp "github.com/SoundMatt/go-RCP"
	"github.com/SoundMatt/go-RCP/shmem"
)

func openZone(t *testing.T, zone rcp.Zone) (*shmem.ZoneServer, *shmem.Controller) {
	t.Helper()
	reg := shmem.NewRegistry()
	srv, ctrl, err := reg.Open(zone)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = reg.Close() })
	return srv, ctrl
}

// TestShmem_Send_RoundTrip verifies Send + Response over shared memory (REQ-SHMEM-001).
func TestShmem_Send_RoundTrip(t *testing.T) {
	_, ctrl := openZone(t, rcp.ZoneFrontLeft)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	resp, err := ctrl.Send(ctx, &rcp.Command{Zone: rcp.ZoneFrontLeft, Type: rcp.CmdNoop})
	if err != nil {
		t.Fatalf("Send: %v", err)
	}
	if resp.Status != rcp.StatusOK {
		t.Errorf("status = %v, want OK", resp.Status)
	}
}

// TestShmem_Send_CustomHandler verifies handler invocation (REQ-SHMEM-002).
func TestShmem_Send_CustomHandler(t *testing.T) {
	srv, ctrl := openZone(t, rcp.ZoneFrontRight)
	srv.SetHandler(func(cmd *rcp.Command) *rcp.Response {
		return &rcp.Response{CommandID: cmd.ID, Zone: cmd.Zone, Status: rcp.StatusError}
	})
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	resp, err := ctrl.Send(ctx, &rcp.Command{Zone: rcp.ZoneFrontRight, Type: rcp.CmdSet})
	if err != nil {
		t.Fatalf("Send: %v", err)
	}
	if resp.Status != rcp.StatusError {
		t.Errorf("status = %v, want Error", resp.Status)
	}
}

// TestShmem_Send_PayloadIsolation verifies that mutating payload after Send is safe (REQ-SHMEM-003).
func TestShmem_Send_PayloadIsolation(t *testing.T) {
	original := []byte{0xAA, 0xBB, 0xCC}
	want := []byte{0xAA, 0xBB, 0xCC}

	srv, ctrl := openZone(t, rcp.ZoneRearLeft)
	captured := make(chan []byte, 1)
	srv.SetHandler(func(cmd *rcp.Command) *rcp.Response {
		cp := make([]byte, len(cmd.Payload))
		copy(cp, cmd.Payload)
		captured <- cp
		return &rcp.Response{CommandID: cmd.ID, Zone: cmd.Zone, Status: rcp.StatusOK}
	})
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	payload := make([]byte, len(original))
	copy(payload, original)
	if _, err := ctrl.Send(ctx, &rcp.Command{Zone: rcp.ZoneRearLeft, Type: rcp.CmdSet, Payload: payload}); err != nil {
		t.Fatalf("Send: %v", err)
	}
	// Mutate original after send — server must have received original bytes
	payload[0] = 0xFF

	select {
	case got := <-captured:
		if !bytes.Equal(got, want) {
			t.Errorf("server payload = %v, want %v (isolation failed)", got, want)
		}
	case <-time.After(time.Second):
		t.Fatal("handler not called")
	}
}

// TestShmem_Send_ZoneMismatch verifies ErrZoneMismatch (REQ-SHMEM-004).
func TestShmem_Send_ZoneMismatch(t *testing.T) {
	_, ctrl := openZone(t, rcp.ZoneRearRight)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	_, err := ctrl.Send(ctx, &rcp.Command{Zone: rcp.ZoneFrontLeft, Type: rcp.CmdNoop})
	if !errors.Is(err, rcp.ErrZoneMismatch) {
		t.Errorf("error = %v, want ErrZoneMismatch", err)
	}
}

// TestShmem_Send_ContextCancelled verifies ErrTimeout (REQ-SHMEM-005).
func TestShmem_Send_ContextCancelled(t *testing.T) {
	_, ctrl := openZone(t, rcp.ZoneCentral)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := ctrl.Send(ctx, &rcp.Command{Zone: rcp.ZoneCentral, Type: rcp.CmdNoop})
	if !errors.Is(err, rcp.ErrTimeout) {
		t.Errorf("error = %v, want ErrTimeout", err)
	}
}

// TestShmem_Subscribe_ReceivesStatus verifies Publish → Subscribe fan-out (REQ-SHMEM-006).
func TestShmem_Subscribe_ReceivesStatus(t *testing.T) {
	srv, ctrl := openZone(t, rcp.ZoneFrontLeft)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	ch, err := ctrl.Subscribe(ctx)
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	srv.Publish([]byte{0x01})

	select {
	case st := <-ch:
		if st.Zone != rcp.ZoneFrontLeft {
			t.Errorf("zone = %v, want FrontLeft", st.Zone)
		}
	case <-time.After(time.Second):
		t.Fatal("no Status received within 1s")
	}
}

// TestShmem_Subscribe_ClosedOnContextCancel verifies channel closes on ctx cancel (REQ-SHMEM-007).
func TestShmem_Subscribe_ClosedOnContextCancel(t *testing.T) {
	_, ctrl := openZone(t, rcp.ZoneRearLeft)
	ctx, cancel := context.WithCancel(context.Background())

	ch, err := ctrl.Subscribe(ctx)
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	cancel()

	select {
	case _, ok := <-ch:
		if ok {
			t.Error("channel should be closed after context cancel")
		}
	case <-time.After(time.Second):
		t.Fatal("channel not closed within 1s")
	}
}

// TestShmem_Send_Concurrent verifies concurrent Sends are safe (REQ-SHMEM-008).
func TestShmem_Send_Concurrent(t *testing.T) {
	_, ctrl := openZone(t, rcp.ZoneFrontLeft)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	const n = 50
	var wg sync.WaitGroup
	errs := make(chan error, n)
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			resp, err := ctrl.Send(ctx, &rcp.Command{Zone: rcp.ZoneFrontLeft, Type: rcp.CmdNoop})
			if err != nil {
				errs <- err
				return
			}
			if resp.Status != rcp.StatusOK {
				errs <- fmt.Errorf("status = %v", resp.Status)
			}
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Error(err)
	}
}
